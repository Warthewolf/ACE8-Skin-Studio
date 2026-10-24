package main

// Cooked game packages in the legacy layout (.uasset header + .uexp export data), as retoc writes
// them when extracting and reads them when packing. UE 5.4, unversioned properties.
//
// What this file can do with a package:
//   - read its header: package name, name map, imports, exports
//   - give an export's data
//   - write it again with a different package name, changed or added names and new export data.
// Everything else in the header is kept byte for byte; offsets are moved to match.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode/utf16"
)

const packageTag = 0x9E2A83C1

// The thirteen 32-bit values after the name map's count and offset (UE 5.4 cooked).
var summaryInts = []string{"SoftObjectPathsCount", "SoftObjectPathsOffset", "GatherableTextDataCount", "GatherableTextDataOffset",
	"ExportCount", "ExportOffset", "ImportCount", "ImportOffset", "DependsOffset",
	"SoftPackageReferencesCount", "SoftPackageReferencesOffset", "SearchableNamesOffset", "ThumbnailTableOffset"}

type PkgImport struct {
	ClassPackage, ClassName, ObjectName FNameRef
	Outer                               int32
	at                                  int
}

type PkgExport struct {
	ClassIndex, SuperIndex, TemplateIndex, OuterIndex int32
	ObjectName                                        FNameRef
	SerialSize, SerialOffset                          int64
	at                                                int
}

// FNameRef is a name as stored: an index into the name map and a number (0 = no number).
type FNameRef struct {
	Index, Number int32
}

type Package struct {
	UAsset, UExp        []byte
	Name                string // the package's own path, e.g. /Game/Vehicles/.../T_X_001_C
	Names               []string
	nameHashes          [][4]byte
	Imports             []PkgImport
	Exports             []PkgExport
	NamesFromExportData int // names [0:this) may be used by export data

	// positions in UAsset
	totalHeaderAt, nameAt, nameEnd int // nameAt..nameEnd: the package name FString
	namesFieldsAt, intsAt          int
	nameOffset, namesEnd           int
	totalHeader                    int
	gensAt                         []int // each generation's (export count, name count)
	arAt, bulkAt, worldTileAt      int
	preloadAt, namesRefAt          int
	payloadAt, dataResAt           int
}

func le32(b []byte, o int) int32     { return int32(binary.LittleEndian.Uint32(b[o:])) }
func le64(b []byte, o int) int64     { return int64(binary.LittleEndian.Uint64(b[o:])) }
func put32(b []byte, o int, v int32) { binary.LittleEndian.PutUint32(b[o:], uint32(v)) }
func put64(b []byte, o int, v int64) { binary.LittleEndian.PutUint64(b[o:], uint64(v)) }

// readFStr reads an Unreal FString at o and returns it and the position after it.
func readFStr(b []byte, o int) (string, int, error) {
	if o+4 > len(b) {
		return "", o, errors.New("text cut short")
	}
	n := int(le32(b, o))
	o += 4
	switch {
	case n == 0:
		return "", o, nil
	case n > 0:
		if n > 1<<20 || o+n > len(b) {
			return "", o, errors.New("text cut short")
		}
		return string(latin1(b[o : o+n-1])), o + n, nil
	default:
		n = -n
		if n > 1<<20 || o+2*n > len(b) {
			return "", o, errors.New("text cut short")
		}
		u := make([]uint16, n-1)
		for i := range u {
			u[i] = binary.LittleEndian.Uint16(b[o+2*i:])
		}
		return string(utf16.Decode(u)), o + 2*n, nil
	}
}

func latin1(b []byte) []rune {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return r
}

// fstrBytes encodes an FString the way the engine does: Latin-1 when every character fits, UTF-16 otherwise.
func fstrBytes(s string) []byte {
	if s == "" {
		return []byte{0, 0, 0, 0}
	}
	narrow := true
	for _, r := range s {
		if r > 0xFF {
			narrow = false
			break
		}
	}
	var out []byte
	if narrow {
		rs := []rune(s)
		out = binary.LittleEndian.AppendUint32(nil, uint32(len(rs)+1))
		for _, r := range rs {
			out = append(out, byte(r))
		}
		return append(out, 0)
	}
	u := utf16.Encode([]rune(s))
	out = binary.LittleEndian.AppendUint32(nil, uint32(int32(-(len(u) + 1))))
	for _, c := range u {
		out = binary.LittleEndian.AppendUint16(out, c)
	}
	return append(out, 0, 0)
}

// LoadPackage reads base.uasset and base.uexp (base is the path without the ending).
func LoadPackage(base string) (*Package, error) {
	ua, err := os.ReadFile(base + ".uasset")
	if err != nil {
		return nil, err
	}
	ue, err := os.ReadFile(base + ".uexp")
	if err != nil {
		return nil, err
	}
	return ParsePackage(ua, ue)
}

func ParsePackage(ua, ue []byte) (p *Package, err error) {
	defer func() {
		if r := recover(); r != nil {
			p, err = nil, fmt.Errorf("unexpected package layout (%v)", r)
		}
	}()
	p = &Package{UAsset: ua, UExp: ue}
	if len(ua) < 64 || binary.LittleEndian.Uint32(ua) != packageTag {
		return nil, errors.New("not a game package (.uasset)")
	}
	o := 4
	if v := le32(ua, o); v > -8 {
		return nil, fmt.Errorf("package version %d is not handled", v)
	}
	o += 4 + 4 + 12 // legacy version, UE3 version, UE4/UE5/licensee versions (0 = unversioned)
	if le32(ua, 12) != 0 || le32(ua, 16) != 0 {
		return nil, errors.New("this package carries version numbers; only cooked, unversioned packages are handled")
	}
	nc := int(le32(ua, o)) // custom versions
	o += 4 + nc*20
	p.totalHeaderAt = o
	p.totalHeader = int(le32(ua, o))
	o += 4
	p.nameAt = o
	if p.Name, o, err = readFStr(ua, o); err != nil {
		return nil, err
	}
	p.nameEnd = o
	flags := binary.LittleEndian.Uint32(ua[o:])
	if flags&0x2000 == 0 {
		return nil, errors.New("this package does not use unversioned properties")
	}
	o += 4
	p.namesFieldsAt = o
	nameCount := int(le32(ua, o))
	p.nameOffset = int(le32(ua, o+4))
	o += 8
	p.intsAt = o
	o += 4 * len(summaryInts)
	o += 16 // package guid
	ng := int(le32(ua, o))
	o += 4
	for i := 0; i < ng; i++ {
		p.gensAt = append(p.gensAt, o)
		o += 8
	}
	for i := 0; i < 2; i++ { // saved-by and compatible-with engine versions
		o += 10
		if _, o, err = readFStr(ua, o); err != nil {
			return nil, err
		}
	}
	o += 4 // compression flags
	if le32(ua, o) != 0 {
		return nil, errors.New("compressed package chunks are not handled")
	}
	o += 4
	o += 4 // package source
	na := int(le32(ua, o))
	o += 4
	for i := 0; i < na; i++ {
		if _, o, err = readFStr(ua, o); err != nil {
			return nil, err
		}
	}
	p.arAt = o
	o += 4
	p.bulkAt = o
	o += 8
	p.worldTileAt = o
	o += 4
	nch := int(le32(ua, o))
	o += 4 + 4*nch
	o += 4 // preload count
	p.preloadAt = o
	o += 4
	p.namesRefAt = o
	o += 4
	p.payloadAt = o
	o += 8
	p.dataResAt = o
	o += 4
	if o != p.nameOffset {
		return nil, fmt.Errorf("package header ends at %d but the name map starts at %d", o, p.nameOffset)
	}
	p.NamesFromExportData = int(le32(ua, p.namesRefAt))
	o = p.nameOffset
	for i := 0; i < nameCount; i++ {
		var s string
		if s, o, err = readFStr(ua, o); err != nil {
			return nil, err
		}
		var h [4]byte
		copy(h[:], ua[o:o+4])
		o += 4
		p.Names = append(p.Names, s)
		p.nameHashes = append(p.nameHashes, h)
	}
	p.namesEnd = o
	ic, io := int(p.sumInt("ImportCount")), int(p.sumInt("ImportOffset"))
	for i := 0; i < ic; i++ {
		at := io + 32*i
		p.Imports = append(p.Imports, PkgImport{
			ClassPackage: FNameRef{le32(ua, at), le32(ua, at+4)},
			ClassName:    FNameRef{le32(ua, at+8), le32(ua, at+12)},
			Outer:        le32(ua, at+16),
			ObjectName:   FNameRef{le32(ua, at+20), le32(ua, at+24)},
			at:           at})
	}
	ec, eo := int(p.sumInt("ExportCount")), int(p.sumInt("ExportOffset"))
	for i := 0; i < ec; i++ {
		at := eo + 96*i
		e := PkgExport{ClassIndex: le32(ua, at), SuperIndex: le32(ua, at+4), TemplateIndex: le32(ua, at+8), OuterIndex: le32(ua, at+12),
			ObjectName: FNameRef{le32(ua, at+16), le32(ua, at+20)}, SerialSize: le64(ua, at+28), SerialOffset: le64(ua, at+36), at: at}
		start := e.SerialOffset - int64(p.totalHeader)
		if start < 0 || start+e.SerialSize > int64(len(ue)) {
			return nil, fmt.Errorf("export %d lies outside the .uexp file", i+1)
		}
		p.Exports = append(p.Exports, e)
	}
	if p.totalHeader != len(ua) {
		return nil, fmt.Errorf("header size %d does not match the .uasset size %d", p.totalHeader, len(ua))
	}
	return p, nil
}

func (p *Package) sumInt(name string) int32 {
	for i, n := range summaryInts {
		if n == name {
			return le32(p.UAsset, p.intsAt+4*i)
		}
	}
	panic("unknown summary field " + name)
}

// ExportData returns export i's bytes (a copy).
func (p *Package) ExportData(i int) []byte {
	e := p.Exports[i]
	start := e.SerialOffset - int64(p.totalHeader)
	return append([]byte(nil), p.UExp[start:start+e.SerialSize]...)
}

// NameText gives a stored name as text: "X" with number 5 is X_4.
func (p *Package) NameText(r FNameRef) string {
	if r.Index < 0 || int(r.Index) >= len(p.Names) {
		return fmt.Sprintf("<bad name %d>", r.Index)
	}
	return joinFName(p.Names[r.Index], r.Number)
}

// ObjectPath is the full path of an import or export: imports give "/Game/X/Y.Y", exports "Y".
func (p *Package) ImportPath(i int) string {
	var parts []string
	for idx := int32(-(i + 1)); idx < 0; {
		imp := p.Imports[-idx-1]
		parts = append([]string{p.NameText(imp.ObjectName)}, parts...)
		idx = imp.Outer
	}
	return strings.Join(parts, ".")
}

// DataResource is one entry of the package's data-resource table: where a piece of bulk data (for a
// streamed texture, one chunk of tiles) is kept.
type DataResource struct {
	Flags                               uint32
	SerialOffset, DuplicateSerialOffset int64
	SerialSize, RawSize                 int64
	OuterIndex                          int32
	LegacyFlags                         uint32
}

const dataResourceSize = 44

// dataResTable gives where the data-resource table starts and how many entries it has.
func (p *Package) dataResTable() (at, n int, err error) {
	if p.dataResAt <= 0 {
		return 0, 0, errors.New("the package has no data-resource table")
	}
	at = int(le32(p.UAsset, p.dataResAt))
	if at <= 0 || at+8 > len(p.UAsset) {
		return 0, 0, errors.New("the package has no data-resource table")
	}
	n = int(le32(p.UAsset, at+4))
	if n < 0 || n > 4096 || at+8+n*dataResourceSize > len(p.UAsset) {
		return 0, 0, fmt.Errorf("the package lists %d data resources", n)
	}
	return at, n, nil
}

// DataResources reads the data-resource table.
func (p *Package) DataResources() ([]DataResource, error) {
	at, n, err := p.dataResTable()
	if err != nil {
		return nil, err
	}
	out := make([]DataResource, n)
	for i := range out {
		e := at + 8 + i*dataResourceSize
		ua := p.UAsset
		out[i] = DataResource{Flags: uint32(le32(ua, e)), SerialOffset: le64(ua, e+4), DuplicateSerialOffset: le64(ua, e+12),
			SerialSize: le64(ua, e+20), RawSize: le64(ua, e+28), OuterIndex: le32(ua, e+36), LegacyFlags: uint32(le32(ua, e+40))}
	}
	return out, nil
}

// PackageEdit describes a rewrite of a package.
type PackageEdit struct {
	Name  string   // new package name ("" = keep)
	Names []string // the full new name map (nil = keep); names may change text but the first
	//                  NamesFromExportData entries keep their places unless AddedForExports says otherwise
	AddedForExports int      // how many names were inserted at index NamesFromExportData (they move later names up)
	Exports         [][]byte // new data for each export (nil = keep)
	UExpTail        []byte   // what follows the export data in the .uexp (nil = keep the original 4 bytes)
	// DataResources replaces the data-resource table (nil = keep). The table must be the last thing
	// in the .uasset, as it is in cooked packages.
	DataResources []DataResource
}

// Write returns new .uasset and .uexp bytes.
func (p *Package) Write(ed PackageEdit) ([]byte, []byte, error) {
	ua := p.UAsset
	names := p.Names
	if ed.Names != nil {
		names = ed.Names
	}
	if len(names) != len(p.Names)+ed.AddedForExports {
		return nil, nil, fmt.Errorf("name map of %d names, expected %d", len(names), len(p.Names)+ed.AddedForExports)
	}
	remap := func(i int32) int32 {
		if int(i) >= p.NamesFromExportData {
			return i + int32(ed.AddedForExports)
		}
		return i
	}
	pkgName := p.Name
	if ed.Name != "" {
		pkgName = ed.Name
	}
	// 1. summary up to and including the package name
	var out bytes.Buffer
	out.Write(ua[:p.nameAt])
	out.Write(fstrBytes(pkgName))
	d1 := out.Len() - p.nameEnd // how far everything after the package name moves
	out.Write(ua[p.nameEnd:p.nameOffset])
	// 2. name map
	// retoc writes zeros where the editor writes two hashes; new names follow whichever the package uses
	oldHash := map[string][4]byte{}
	zeros := true
	for i, n := range p.Names {
		oldHash[n] = p.nameHashes[i]
		if p.nameHashes[i] != [4]byte{} {
			zeros = false
		}
	}
	for _, n := range names {
		out.Write(fstrBytes(n))
		h, ok := oldHash[n]
		if !ok && !zeros {
			h = nameHashes(n)
		}
		out.Write(h[:])
	}
	d := out.Len() - p.namesEnd // how far everything after the name map moves
	if ed.DataResources != nil {
		at, n, err := p.dataResTable()
		if err != nil {
			return nil, nil, err
		}
		if at < p.namesEnd || at+8+n*dataResourceSize != len(ua) {
			return nil, nil, errors.New("the data-resource table is not at the end of the package; it cannot be rewritten")
		}
		out.Write(ua[p.namesEnd : at+4]) // up to and including the table's version
		w := func(v any) { binary.Write(&out, binary.LittleEndian, v) }
		w(int32(len(ed.DataResources)))
		for _, r := range ed.DataResources {
			w(r.Flags)
			w(r.SerialOffset)
			w(r.DuplicateSerialOffset)
			w(r.SerialSize)
			w(r.RawSize)
			w(r.OuterIndex)
			w(r.LegacyFlags)
		}
	} else {
		out.Write(ua[p.namesEnd:])
	}
	nb := out.Bytes()
	// 3. fix the summary's numbers (positions before the package name do not move)
	at := func(o int) int {
		if o >= p.nameEnd {
			return o + d1
		}
		return o
	}
	exportBytes := make([][]byte, len(p.Exports))
	total := 0
	for i := range p.Exports {
		if ed.Exports != nil && ed.Exports[i] != nil {
			exportBytes[i] = ed.Exports[i]
		} else {
			exportBytes[i] = p.ExportData(i)
		}
		total += len(exportBytes[i])
	}
	newTotal := len(nb)
	put32(nb, at(p.totalHeaderAt), int32(newTotal))
	put32(nb, at(p.namesFieldsAt), int32(len(names)))
	put32(nb, at(p.namesFieldsAt)+4, int32(p.nameOffset+d1))
	for i, key := range summaryInts {
		o := at(p.intsAt + 4*i)
		v := le32(ua, p.intsAt+4*i)
		if strings.HasSuffix(key, "Offset") && v > 0 {
			put32(nb, o, v+int32(d))
		}
	}
	for _, g := range p.gensAt {
		put32(nb, at(g)+4, int32(len(names)))
	}
	if v := le32(ua, p.arAt); v > 0 {
		put32(nb, at(p.arAt), v+int32(d))
	}
	put64(nb, at(p.bulkAt), int64(newTotal+total))
	if v := le32(ua, p.worldTileAt); v > 0 {
		put32(nb, at(p.worldTileAt), v+int32(d))
	}
	if v := le32(ua, p.preloadAt); v > 0 {
		put32(nb, at(p.preloadAt), v+int32(d))
	}
	put32(nb, at(p.namesRefAt), int32(p.NamesFromExportData+ed.AddedForExports))
	if v := le64(ua, p.payloadAt); v > 0 {
		put64(nb, at(p.payloadAt), v+int64(d))
	}
	if v := le32(ua, p.dataResAt); v > 0 {
		put32(nb, at(p.dataResAt), v+int32(d))
	}
	// 4. imports and exports: name indices and where each export's data now is
	for _, imp := range p.Imports {
		o := imp.at + d
		for _, f := range []int{0, 8, 20} {
			put32(nb, o+f, remap(le32(nb, o+f)))
		}
	}
	pos := int64(newTotal)
	for i, e := range p.Exports {
		o := e.at + d
		put32(nb, o+16, remap(le32(nb, o+16)))
		put64(nb, o+28, int64(len(exportBytes[i])))
		put64(nb, o+36, pos)
		pos += int64(len(exportBytes[i]))
	}
	tail := ed.UExpTail
	if tail == nil {
		last := int64(0)
		for _, e := range p.Exports {
			if end := e.SerialOffset - int64(p.totalHeader) + e.SerialSize; end > last {
				last = end
			}
		}
		tail = p.UExp[last:]
	}
	var ue bytes.Buffer
	for _, b := range exportBytes {
		ue.Write(b)
	}
	ue.Write(tail)
	return nb, ue.Bytes(), nil
}

// Retarget returns a copy of the package moved to newName, with some imported packages pointing
// elsewhere: moves maps old package paths ("/Game/A/B") to new ones. Object names inside stay the
// same. A name whose text is shared by something that must not move makes it fail rather than guess.
func (p *Package) Retarget(newName string, moves map[string]string) ([]byte, []byte, error) {
	return p.RetargetWith(newName, moves, PackageEdit{})
}

// RetargetWith is Retarget with new export data as well (ed.Name and ed.Names are ignored).
func (p *Package) RetargetWith(newName string, moves map[string]string, ed PackageEdit) ([]byte, []byte, error) {
	all := map[string]string{}
	for k, v := range moves {
		all[k] = v
	}
	if newName != "" && newName != p.Name {
		all[p.Name] = newName
	}
	names := append([]string(nil), p.Names...)
	// which (base name, number) pairs each move covers
	type key struct {
		base string
		num  int32
	}
	want := map[key]string{}
	for old, nw := range all {
		ob, on := splitFName(old)
		nbase, nn := splitFName(nw)
		if on != nn {
			return nil, nil, fmt.Errorf("cannot move %s to %s: the numbers at the end differ", old, nw)
		}
		want[key{ob, on}] = nbase
	}
	// every use of a name in the header: a name that is moved must only ever be used with the moved number
	uses := map[int32]map[int32]bool{}
	note := func(r FNameRef) {
		if uses[r.Index] == nil {
			uses[r.Index] = map[int32]bool{}
		}
		uses[r.Index][r.Number] = true
	}
	for _, imp := range p.Imports {
		note(imp.ClassPackage)
		note(imp.ClassName)
		note(imp.ObjectName)
	}
	for _, e := range p.Exports {
		note(e.ObjectName)
	}
	found := map[string]bool{}
	for i, n := range p.Names {
		for k, nbase := range want {
			if n != k.base {
				continue
			}
			for num := range uses[int32(i)] {
				if num != k.num {
					return nil, nil, fmt.Errorf("the name %s is used for more than one thing in %s; it cannot be moved safely", n, p.Name)
				}
			}
			if names[i] != n && names[i] != nbase {
				return nil, nil, fmt.Errorf("the name %s would have to move to two places in %s", n, p.Name)
			}
			names[i] = nbase
			found[joinFName(k.base, k.num)] = true
		}
	}
	for old := range moves {
		if !found[old] {
			return nil, nil, fmt.Errorf("%s does not refer to %s", p.Name, old)
		}
	}
	ed.Name, ed.Names, ed.AddedForExports = newName, names, 0
	return p.Write(ed)
}

// Rename is Retarget for any change of name, including the number the engine keeps apart at the end
// of a name: T_X_101 is stored as T_X with the number 102, so moving T_X_101 to T_X_603 changes the
// number as well as (perhaps) the text. renames maps old names to new ones: paths of packages it
// imports and names of objects (its own or imported); newName is the package's own new path. Every
// use in the import and export tables changes. It fails rather than guess when a name whose number
// changes may also be used somewhere it does not read (inside export data or other tables), or when
// a name whose text changes is also used for something that is not renamed.
func (p *Package) Rename(newName string, renames map[string]string) ([]byte, []byte, error) {
	all := map[string]string{}
	for k, v := range renames {
		if k != v {
			all[k] = v
		}
	}
	if newName != "" && newName != p.Name {
		all[p.Name] = newName
	}
	type ref struct {
		off      int // where the name index is in the .uasset; the number follows it
		idx, num int32
	}
	var refs []ref
	for _, imp := range p.Imports {
		for _, f := range []int{0, 8, 20} {
			refs = append(refs, ref{imp.at + f, le32(p.UAsset, imp.at+f), le32(p.UAsset, imp.at+f+4)})
		}
	}
	for _, e := range p.Exports {
		refs = append(refs, ref{e.at + 16, le32(p.UAsset, e.at+16), le32(p.UAsset, e.at+20)})
	}
	ua := append([]byte(nil), p.UAsset...)
	names := append([]string(nil), p.Names...)
	changed := map[int]string{}
	olds := make([]string, 0, len(all))
	for k := range all {
		olds = append(olds, k)
	}
	sort.Strings(olds)
	for _, old := range olds {
		nw := all[old]
		ob, on := splitFName(old)
		nb, nn := splitFName(nw)
		idx := -1
		for i, n := range p.Names {
			if n == ob {
				idx = i
				break
			}
		}
		if idx < 0 {
			if old == p.Name {
				continue // its own path need not be in the name map
			}
			return nil, nil, fmt.Errorf("%s does not refer to %s", p.Name, old)
		}
		var mine []ref
		other := false
		for _, r := range refs {
			if int(r.idx) == idx {
				if r.num == on {
					mine = append(mine, r)
				} else {
					other = true
				}
			}
		}
		if len(mine) == 0 && old != p.Name {
			return nil, nil, fmt.Errorf("%s does not refer to %s", p.Name, old)
		}
		if nb != ob {
			if other {
				return nil, nil, fmt.Errorf("the name %s is used for more than one thing in %s; it cannot be renamed safely", ob, p.Name)
			}
			for i, n := range p.Names {
				if i != idx && n == nb {
					return nil, nil, fmt.Errorf("%s already has a name %s", p.Name, nb)
				}
			}
		}
		if prev, ok := changed[idx]; ok && prev != nb {
			return nil, nil, fmt.Errorf("the name %s would have to change to two names in %s", ob, p.Name)
		}
		if on != nn {
			// The only other tables of a cooked header that hold names are the soft object paths and
			// soft package references; this does not rewrite those, so a package that has them is left.
			if p.sumInt("SoftObjectPathsCount") > 0 || p.sumInt("SoftPackageReferencesCount") > 0 {
				return nil, nil, fmt.Errorf("%s has soft references; %s cannot be renamed safely", p.Name, old)
			}
			// Export data may use the first NamesFromExportData names. A use of this one with its old
			// number would be the 8 bytes (index, number); with a number that is not 0 those bytes are
			// distinctive enough to look for. (Number 0 is everywhere in data, so it says nothing: a
			// game texture does not use its own name in its data, as icons with numbered names show.)
			pat := make([]byte, 8)
			binary.LittleEndian.PutUint32(pat, uint32(idx))
			binary.LittleEndian.PutUint32(pat[4:], uint32(on))
			if on != 0 && idx < p.NamesFromExportData && bytes.Contains(p.UExp, pat) {
				return nil, nil, fmt.Errorf("%s may use %s inside its data; it cannot be renamed safely", p.Name, old)
			}
			for _, r := range mine {
				put32(ua, r.off+4, nn)
			}
		}
		names[idx] = nb
		changed[idx] = nb
	}
	q := *p
	q.UAsset = ua
	return q.Write(PackageEdit{Name: newName, Names: names})
}
