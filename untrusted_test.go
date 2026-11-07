package main

// Files from other people's mods, made up to be as unhelpful as possible: each must give a plain
// error or be skipped, never crash, hang or write outside the folder it is meant for.

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf16"
)

// engineFStr writes an FString the way the engine does: one byte per character when every
// character is ASCII, UTF-16 otherwise.
func engineFStr(s string) []byte {
	if s == "" {
		return []byte{0, 0, 0, 0}
	}
	ascii := true
	for _, r := range s {
		if r > 0x7F {
			ascii = false
		}
	}
	if ascii {
		b := binary.LittleEndian.AppendUint32(nil, uint32(len(s)+1))
		return append(append(b, s...), 0)
	}
	u := utf16.Encode([]rune(s))
	b := binary.LittleEndian.AppendUint32(nil, uint32(int32(-(len(u) + 1))))
	for _, c := range u {
		b = binary.LittleEndian.AppendUint16(b, c)
	}
	return append(b, 0, 0)
}

type synthImport struct {
	classPackage, className, object int32 // name indices
	outer                           int32
}

type synthExport struct {
	object int32 // name index
	data   []byte
}

// synthPackage builds a small cooked package (.uasset + .uexp) laid out the way ParsePackage reads one.
func synthPackage(name string, names []string, imports []synthImport, exports []synthExport) (ua, ue []byte) {
	le := binary.LittleEndian
	var h []byte
	h = le.AppendUint32(h, packageTag)
	h = le.AppendUint32(h, uint32(0xFFFFFFF8)) // legacy version -8
	h = append(h, make([]byte, 16)...)         // UE3, UE4, UE5 and licensee versions: unversioned
	h = le.AppendUint32(h, 0)                  // custom versions
	totalAt := len(h)
	h = le.AppendUint32(h, 0)
	h = append(h, engineFStr(name)...)
	h = le.AppendUint32(h, 0x80002000) // cooked, unversioned properties
	namesAt := len(h)
	h = append(h, make([]byte, 8)...)
	intsAt := len(h)
	h = append(h, make([]byte, 4*len(summaryInts))...)
	h = append(h, make([]byte, 16)...) // guid
	h = le.AppendUint32(h, 1)          // one generation
	genAt := len(h)
	h = append(h, make([]byte, 8)...)
	for i := 0; i < 2; i++ { // engine versions
		h = append(h, make([]byte, 10)...)
		h = append(h, engineFStr("")...)
	}
	h = append(h, make([]byte, 12)...) // compression flags, compressed chunks, package source
	h = le.AppendUint32(h, 0)          // additional packages to cook
	h = append(h, make([]byte, 4)...)  // asset registry data
	bulkAt := len(h)
	h = append(h, make([]byte, 8+4)...) // bulk data start, world tile info
	h = le.AppendUint32(h, 0)           // chunk ids
	h = append(h, make([]byte, 8)...)   // preload dependencies
	h = le.AppendUint32(h, uint32(len(names)))
	h = append(h, make([]byte, 8+4)...) // payload table, data resources
	nameOffset := len(h)
	for _, n := range names {
		h = append(h, engineFStr(n)...)
		h = append(h, 0, 0, 0, 0)
	}
	importOffset := len(h)
	for _, im := range imports {
		for _, v := range []int32{im.classPackage, 0, im.className, 0, im.outer, im.object, 0, 0} {
			h = le.AppendUint32(h, uint32(v))
		}
	}
	exportOffset := len(h)
	h = append(h, make([]byte, 96*len(exports))...)
	total := len(h)
	pos := total
	for i, e := range exports {
		at := exportOffset + 96*i
		le.PutUint32(h[at+16:], uint32(e.object))
		le.PutUint64(h[at+28:], uint64(len(e.data)))
		le.PutUint64(h[at+36:], uint64(pos))
		pos += len(e.data)
		ue = append(ue, e.data...)
	}
	ue = le.AppendUint32(ue, packageTag)
	le.PutUint32(h[totalAt:], uint32(total))
	le.PutUint32(h[namesAt:], uint32(len(names)))
	le.PutUint32(h[namesAt+4:], uint32(nameOffset))
	set := func(field string, v int) {
		for i, n := range summaryInts {
			if n == field {
				le.PutUint32(h[intsAt+4*i:], uint32(v))
			}
		}
	}
	set("ExportCount", len(exports))
	set("ExportOffset", exportOffset)
	set("ImportCount", len(imports))
	set("ImportOffset", importOffset)
	le.PutUint32(h[genAt:], uint32(len(exports)))
	le.PutUint32(h[genAt+4:], uint32(len(names)))
	le.PutUint64(h[bulkAt:], uint64(pos))
	return h, ue
}

// finishes runs f and fails the test if it has not returned after a few seconds (a loop that never ends).
func finishes(t *testing.T, what string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s did not finish", what)
	}
}

// ---- container tables

// tocWithIndex builds a container table with one chunk and the given folder table.
func tocWithIndex(dirs [][4]uint32, files [][3]uint32, names []string) []byte {
	le := binary.LittleEndian
	fstr := func(s string) []byte {
		b := le.AppendUint32(nil, uint32(len(s)+1))
		return append(append(b, s...), 0)
	}
	var d []byte
	d = append(d, fstr("../../../")...)
	d = le.AppendUint32(d, uint32(len(dirs)))
	for _, e := range dirs {
		for _, v := range e {
			d = le.AppendUint32(d, v)
		}
	}
	d = le.AppendUint32(d, uint32(len(files)))
	for _, e := range files {
		for _, v := range e {
			d = le.AppendUint32(d, v)
		}
	}
	d = le.AppendUint32(d, uint32(len(names)))
	for _, n := range names {
		d = append(d, fstr(n)...)
	}
	b := make([]byte, 144)
	copy(b, tocMagic)
	le.PutUint32(b[20:], 144)
	le.PutUint32(b[24:], 1) // one chunk
	le.PutUint32(b[48:], uint32(len(d)))
	b[80] = 8 // indexed
	b = append(b, make([]byte, 12+10)...)
	return append(b, d...)
}

// A folder table that leads back into itself (a folder that is its own child, two folders that
// are each other's child, a folder that is its own next folder, a file list that loops) must not
// crash the program: Go cannot recover from running out of stack, so a mod like that would stop
// the program on every start.
func TestTOCFolderLoops(t *testing.T) {
	const none = 0xFFFFFFFF
	cases := map[string][]byte{
		"own child": tocWithIndex([][4]uint32{{none, 0, none, none}}, nil, nil),
		"each other's child": tocWithIndex([][4]uint32{{none, 1, none, none}, {0, 0, none, 0}},
			[][3]uint32{{1, none, 0}}, []string{"Live", "A.uasset"}),
		"own next folder": tocWithIndex([][4]uint32{{none, 1, none, none}, {0, none, 1, 0}},
			[][3]uint32{{1, none, 0}}, []string{"Live", "A.uasset"}),
		"looping files": tocWithIndex([][4]uint32{{0, none, none, 0}}, [][3]uint32{{1, 0, 0}}, []string{"Live", "A.uasset"}),
	}
	mods := t.TempDir()
	for name, b := range cases {
		finishes(t, name, func() {
			info, err := parseTOC(b)
			if err != nil {
				t.Errorf("%s: %v", name, err)
				return
			}
			if name != "own child" && !strings.HasSuffix(info.Names[0], "A.uasset") {
				t.Errorf("%s: the name read before the loop was lost: %q", name, info.Names)
			}
		})
		dir := filepath.Join(mods, strings.ReplaceAll(name, " ", "_"))
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "Evil.utoc"), b, 0o644)
	}
	finishes(t, "scanning the mods folder", func() {
		if got := scanMods(mods); len(got) != len(cases) {
			t.Errorf("listed %d mods, want %d", len(got), len(cases))
		}
	})
	// a very long chain of folders, each the child of the one before
	var deep [][4]uint32
	for i := 0; i < 200000; i++ {
		deep = append(deep, [4]uint32{0, uint32(i + 1), none, none})
	}
	deep[len(deep)-1][1] = none
	deep[len(deep)-1][3] = 0
	finishes(t, "a deep folder chain", func() {
		parseTOC(tocWithIndex(deep, [][3]uint32{{1, none, 0}}, []string{"LongFolderName", "A.uasset"}))
	})
}

// Counts in a container header that multiply past what a number can hold must not make the
// program read before the start of the file.
func TestTOCHugeCounts(t *testing.T) {
	b := make([]byte, 144)
	copy(b, tocMagic)
	le := binary.LittleEndian
	le.PutUint32(b[20:], 144)
	le.PutUint32(b[24:], 1)
	le.PutUint32(b[36:], 0xFFFFFFFF) // compression methods
	le.PutUint32(b[40:], 0xFFFFFFFF) // length of a method's name
	le.PutUint32(b[44:], 0x10000)
	le.PutUint32(b[48:], 4)
	b[80] = 0x0C // signed, indexed
	b = append(b, make([]byte, 12+4)...)
	if _, err := parseTOC(b); err != nil {
		t.Fatal(err) // the chunk id is still there to compare mods with
	}
	p := filepath.Join(t.TempDir(), "Evil.utoc")
	os.WriteFile(p, b, 0o644)
	if _, err := OpenZenContainer(p); err == nil {
		t.Fatal("a container with an impossible header should be refused")
	}
}

// ---- packages read straight out of other mods' containers

func TestZenContainerDamaged(t *testing.T) {
	dir := t.TempDir()
	ucas := filepath.Join(dir, "x.ucas")
	os.WriteFile(ucas, make([]byte, 4096), 0o644)
	// a block compressed with a method the container does not list
	c := &ZenContainer{ucas: ucas, ids: make([][12]byte, 1), offs: []int64{0}, lens: []int64{16}, blockSize: 0x10000,
		blocks: []ioBlock{{offset: 0, compSize: 16, size: 16, method: 3}}, methods: []string{"Zlib"}}
	if _, err := c.ReadChunk(0); err == nil || !strings.Contains(err.Error(), "cannot read") {
		t.Fatalf("unknown method: %v", err)
	}
	// a chunk that starts past the end of its block
	c = &ZenContainer{ucas: ucas, ids: make([][12]byte, 1), offs: []int64{1000}, lens: []int64{10}, blockSize: 0x10000,
		blocks: []ioBlock{{offset: 0, compSize: 100, size: 100}}}
	if _, err := c.ReadChunk(0); err == nil {
		t.Fatal("a chunk past the end of its block should be refused")
	}
	// a chunk number the container does not have
	if _, err := c.ReadChunk(5); err == nil {
		t.Fatal("a chunk that is not there should be refused")
	}
	// a table of contents cut off right after the chunk id
	b := make([]byte, 144)
	copy(b, tocMagic)
	binary.LittleEndian.PutUint32(b[20:], 144)
	binary.LittleEndian.PutUint32(b[24:], 1)
	binary.LittleEndian.PutUint32(b[44:], 0x10000)
	b = append(b, make([]byte, 12)...)
	utoc := filepath.Join(dir, "cut.utoc")
	os.WriteFile(utoc, b, 0o644)
	if _, err := OpenZenContainer(utoc); err == nil {
		t.Fatal("a cut-short table of contents should be refused")
	}
	if _, _, err := readZenSkinTables(utoc, builtinSchema); err == nil {
		t.Fatal("reading the skin list of a damaged container should fail with an error")
	}
	// every damaged form of a container that holds a skin list
	good := synthTOC(5, "../../../", []string{"Live/Content/Datatables/Information/DT_Skin.uasset"})
	os.WriteFile(filepath.Join(dir, "good.ucas"), make([]byte, 600), 0o644)
	for i := 144; i < len(good); i++ {
		bad := append([]byte(nil), good...)
		bad[i] ^= 0xFF
		os.WriteFile(filepath.Join(dir, "good.utoc"), bad, 0o644)
		readZenSkinTables(filepath.Join(dir, "good.utoc"), builtinSchema) // must not panic
	}
}

// ---- packages

func TestSynthPackageRoundTrip(t *testing.T) {
	names := []string{"/Game/A/B", "B", "/Script/Engine", "Texture2D", "Package", "Scale×2", "Ünïcødé ✓"}
	ua, ue := synthPackage("/Game/A/B", names, []synthImport{{2, 4, 2, 0}, {2, 3, 3, -1}}, []synthExport{{1, []byte("data of B")}})
	p, err := ParsePackage(ua, ue)
	if err != nil {
		t.Fatal(err)
	}
	if p.Names[5] != "Scale×2" || p.ImportPath(1) != "/Script/Engine.Texture2D" {
		t.Fatalf("read wrongly: %q %q", p.Names, p.ImportPath(1))
	}
	// the engine stores "×" (U+00D7) as UTF-16; writing an unchanged package must give the same bytes
	ua2, ue2, err := p.Write(PackageEdit{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ua2, ua) || !bytes.Equal(ue2, ue) {
		t.Fatal("writing an unchanged package gave different bytes")
	}
	for _, s := range []string{"", "plain", "Scale×2", "é", "\u007f", "✓"} {
		got, _, err := readFStr(fstrBytes(s), 0)
		if err != nil || got != s || !bytes.Equal(fstrBytes(s), engineFStr(s)) {
			t.Errorf("%q: written as %v, read back as %q (%v)", s, fstrBytes(s), got, err)
		}
	}
}

func TestDamagedPackages(t *testing.T) {
	names := []string{"/Game/A/B", "B", "/Script/Engine", "Texture2D"}
	data := []byte("0123456789")
	// an import that is its own outer, two imports that are each other's outer, and an outer that
	// is not in the table
	for _, outers := range [][2]int32{{-1, 0}, {-2, -1}, {0, -9}} {
		ua, ue := synthPackage("/Game/A/B", names, []synthImport{{2, 3, 3, outers[0]}, {2, 3, 1, outers[1]}}, []synthExport{{1, data}})
		finishes(t, "reading imports", func() {
			p, err := ParsePackage(ua, ue)
			if err != nil {
				return // refused: fine
			}
			for i := range p.Imports {
				p.ImportPath(i)
			}
			p.Rename("/Game/A/C", map[string]string{"B": "C"})
		})
	}
	// an export whose size is negative, or so large that it wraps around
	for _, size := range []int64{-2, 0x7FFFFFFFFFFFFFFF} {
		ua, ue := synthPackage("/Game/A/B", names, nil, []synthExport{{1, data}})
		binary.LittleEndian.PutUint64(ua[len(ua)-96+28:], uint64(size))
		p, err := ParsePackage(ua, ue)
		if err == nil {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("size %d: accepted, then crashed: %v", size, r)
					}
				}()
				p.Write(PackageEdit{})
			}()
			t.Errorf("size %d: an export outside the .uexp was accepted", size)
		}
	}
	// names for export data past the end of the name map
	ua, ue := synthPackage("/Game/A/B", names, nil, []synthExport{{1, data}})
	p, _ := ParsePackage(ua, ue)
	binary.LittleEndian.PutUint32(ua[p.namesRefAt:], 99)
	if _, err := ParsePackage(ua, ue); err == nil {
		t.Error("a package saying 99 names are used by export data, with 4 names, was accepted")
	}
	// two thousand million generations: refused at once, not counted out one by one
	ua, ue = synthPackage("/Game/A/B", names, nil, []synthExport{{1, data}})
	binary.LittleEndian.PutUint32(ua[p.gensAt[0]-4:], 0x7FFFFFFF)
	finishes(t, "a huge generation count", func() {
		if _, err := ParsePackage(ua, ue); err == nil {
			t.Error("a package with more generations than bytes was accepted")
		}
	})
	// every damaged form of a small package: refused, or written without a crash
	good, goodUE := synthPackage("/Game/A/B", names, []synthImport{{2, 3, 3, 0}, {2, 3, 1, -1}}, []synthExport{{1, data}})
	for i := 0; i < len(good); i++ {
		for _, v := range []byte{0xFF, 0x80, 0x01} {
			bad := append([]byte(nil), good...)
			bad[i] ^= v
			p, err := ParsePackage(bad, goodUE)
			if err != nil {
				continue
			}
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("byte %d ^ %#x: accepted, then crashed: %v", i, v, r)
					}
				}()
				for k := range p.Imports {
					p.ImportPath(k)
				}
				p.Write(PackageEdit{})
				p.Rename("/Game/A/C", map[string]string{"B": "C"})
			}()
		}
	}
}

// ---- a skin's slot files from someone else's mod

// slotModWith lays out an installed slot mod (in top/a/b/Mods/Evil) whose skin s1 has the given
// package as its slot file.
func slotModWith(t *testing.T, top string, ua, ue []byte) InstalledSlotMod {
	t.Helper()
	dir := filepath.Join(top, "a", "b", "Mods", "Evil")
	if err := writeSlotFolder(filepath.Join(dir, slotFilesFolder), map[string][]byte{"s1/x.uasset": ua, "s1/x.uexp": ue}); err != nil {
		t.Fatal(err)
	}
	return InstalledSlotMod{Dir: dir, Manifest: &ModManifest{Key: "Evil_abcde", Container: "Evil_abcde_P"}}
}

func su57Template(material string) *SkinRow {
	return &SkinRow{Key: "I_EM_21001001", Obj: &Obj{Props: []Prop{
		{Name: "DecalMaterialRef", Val: material},
		{Name: "MenuIconImageRef", Val: "/Game/UI/Texture/Icon/Skin/T_UI_Icon_Skin_su57_001.T_UI_Icon_Skin_su57_001"}}}}
}

// filesOutside lists the package files under top that are neither under root nor in the mod.
func filesOutside(top, root string, im InstalledSlotMod) []string {
	var out []string
	filepath.Walk(top, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, ".uasset") && !within(root, p) && !within(im.Dir, p) {
			out = append(out, p)
		}
		return nil
	})
	return out
}

// The paths inside a shared mod's slot files, and in another mod's skin list, decide where the
// skin list's files are written; none of them may lead outside the folder being packed.
func TestSlotFilesStayInside(t *testing.T) {
	s := Settings{Project: "Live"}
	gameMat := "/Game/Vehicles/Aircraft/PP/PP0021_su57/Materials/001/MI_PP0021_su57_001_Decal.MI_PP0021_su57_001_Decal"
	sl := SlotDef{Key: "s1", Aircraft: 21, Template: 21001001, Name: "Evil", Material: "/Game/SkinStudio/Evil_abcde/s1/MI_x_001_Decal"}
	own := "/Game/SkinStudio/Evil_abcde/s1/MI_x_001_Decal"
	ownNames := []string{own, "MI_x_001_Decal", "/Script/Engine", "MaterialInstanceConstant"}
	ownExports := []synthExport{{1, []byte("material")}}
	type slotCase struct {
		what   string
		ua, ue []byte
		mat    string // the material of the skin it is based on
		ok     func(mat string, err error) bool
	}
	refused := func(mat string, err error) bool { return err != nil }
	var cases []slotCase
	// 1. a package path with ".." that cannot be renamed, so it would be written under its own path
	ua, ue := synthPackage("/Game/../../../../../escaped/x_001_Decal", []string{"None"}, nil, nil)
	cases = append(cases, slotCase{"a path with ..", ua, ue, gameMat, refused})
	// 2. backslashes, which Windows reads as folder separators
	ua, ue = synthPackage(`/Game/SkinStudio/Evil_abcde/s1/..\..\..\..\..\escaped\x_001_Decal`, []string{"None"}, nil, nil)
	cases = append(cases, slotCase{"backslashes", ua, ue, gameMat, refused})
	// 3. a package that is not the skin's own
	ua, ue = synthPackage("/Game/Vehicles/Aircraft/PP/PP0021_su57/Textures/001/T_PP0021_su57_001_C", []string{"None"}, nil, nil)
	cases = append(cases, slotCase{"a game file's path", ua, ue, gameMat, refused})
	// 4. the skin's own package, based on a skin whose material path (from another mod's list) has ".."
	ua, ue = synthPackage(own, ownNames, nil, ownExports)
	cases = append(cases, slotCase{"a template path with ..", ua, ue, "/Game/../../../../../escaped2/001/MI_PP0021_su57_001_Decal", refused})
	// 5. and the same package with a proper template: renamed to the skin's number
	cases = append(cases, slotCase{"a proper skin", ua, ue, gameMat, func(mat string, err error) bool {
		return err == nil && mat == "/Game/Vehicles/Aircraft/PP/PP0021_su57/Materials/603/MI_x_603_Decal"
	}})
	for _, c := range cases {
		top := t.TempDir()
		im := slotModWith(t, top, c.ua, c.ue)
		root := filepath.Join(top, "a", "b", "list", "input") // five folders up from its Live/Content is top/a
		mat, _, notes, err := numberedSlotFiles(s, root, su57Template(c.mat), im, sl, 603, nil)
		if !c.ok(mat, err) {
			t.Errorf("%s: material %q, notes %q, error %v", c.what, mat, notes, err)
		}
		if out := filesOutside(top, root, im); len(out) > 0 {
			t.Errorf("%s: wrote outside the folder being packed: %v", c.what, out)
		}
	}
}

// ---- moving mods out of the mods folder

// A rename that fails for any reason but a different drive (on Windows, most often the game
// holding a file open) must leave the mod as it was, not copy it and then delete what it can.
func TestMoveKeepsModWhole(t *testing.T) {
	if runtime.GOOS != "windows" && !otherDrive(&os.LinkError{Op: "rename", Old: "a", New: "b", Err: syscall.EXDEV}) {
		t.Fatal("a different drive should be recognised")
	}
	if otherDrive(&os.LinkError{Op: "rename", Old: "a", New: "b", Err: syscall.EACCES}) || otherDrive(nil) {
		t.Fatal("only a different drive may lead to a copy")
	}
	root := t.TempDir()
	src := filepath.Join(root, "Mods", "Navy")
	writeMod(t, src, "Navy_P", 1, []string{"Live/Content/A.uasset"})
	os.WriteFile(filepath.Join(src, manifestName), []byte("{}"), 0o644)
	dst := filepath.Join(root, "removed", "Navy")
	os.MkdirAll(dst, 0o755)
	os.WriteFile(filepath.Join(dst, "other.txt"), []byte("x"), 0o644) // a rename cannot replace a folder that holds something
	if err := moveDir(src, dst); err == nil {
		t.Fatal("the move should fail")
	}
	if es, _ := os.ReadDir(src); len(es) != 4 {
		t.Fatalf("the mod lost files: %v", es)
	}
	if es, _ := os.ReadDir(dst); len(es) != 1 {
		t.Fatalf("files were copied: %v", es)
	}
	// a loose mod: when one of its files cannot be moved, the ones already moved are put back
	mods := filepath.Join(root, "Mods")
	writeMod(t, mods, "Loose_P", 4, []string{"Live/Content/Other/Thing.uasset"})
	s := Settings{ModsDir: mods, Workspace: filepath.Join(root, "ws")}
	now := time.Now()
	for k := 0; k < 5; k++ { // in the way of its .ucas, wherever it goes in the next few seconds
		block := filepath.Join(s.removedModsDir(), "Loose_P_"+now.Add(time.Duration(k)*time.Second).Format("20060102-150405"), "Loose_P.ucas")
		os.MkdirAll(block, 0o755)
		os.WriteFile(filepath.Join(block, "x"), []byte("x"), 0o644)
	}
	if _, err := removeMod(s, "Loose_P", true); err == nil {
		t.Fatal("removing should fail")
	}
	for _, ext := range []string{".utoc", ".ucas", ".pak"} {
		if _, err := os.Stat(filepath.Join(mods, "Loose_P"+ext)); err != nil {
			t.Errorf("Loose_P%s is no longer in the mods folder", ext)
		}
	}
}

// ---- installing a shared mod

// shareZip writes a zip of the given files. A name starting with "!" is stored with a wrong
// checksum, so reading it fails at its end, the way a damaged download does.
func shareZip(t *testing.T, files map[string][]byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "mod.zip")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	var names []string
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		b := files[n]
		var w io.Writer
		if bad, ok := strings.CutPrefix(n, "!"); ok {
			w, err = zw.CreateRaw(&zip.FileHeader{Name: bad, Method: zip.Store, CRC32: 12345, CompressedSize64: uint64(len(b)), UncompressedSize64: uint64(len(b))})
		} else {
			w, err = zw.Create(n)
		}
		if err != nil {
			t.Fatal(err)
		}
		w.Write(b)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

func navyManifest() *ModManifest {
	return &ModManifest{Format: manifestFormat, Key: "Navy_abcde", Folder: "Navy", Title: "Navy", Container: "Navy_abcde_P",
		Slots: []SlotDef{{Key: "s1", Aircraft: 21, Template: 21001001, Name: "Navy", IconFile: "../../../../../../etc/passwd"}}}
}

// installNavy installs a zip holding navyManifest's mod into dest, the way doImport does.
func installNavy(t *testing.T, zipPath, dest string, plan ImportPlan) error {
	t.Helper()
	src, err := openSource(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	plan.Studio, plan.Manifest, plan.Folder, plan.Containers = true, navyManifest(), filepath.Base(dest), containersIn(src)
	return installStudioMod(testJob(), src, &plan, dest)
}

// dirState gives every file in a folder with its contents.
func dirState(dir string) map[string]string {
	out := map[string]string{}
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			b, _ := os.ReadFile(p)
			out[filepath.ToSlash(rel)] = string(b)
		}
		return nil
	})
	return out
}

func sameState(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

// An update copies the new copy in completely before the old one makes room: one that fails
// part-way (here a damaged file in the zip) leaves the installed copy exactly as it was, and a new
// import that fails leaves nothing.
func TestImportUpdateFailsSafely(t *testing.T) {
	mods := t.TempDir()
	dest := filepath.Join(mods, "Navy")
	old := map[string]string{"Navy_abcde_P.utoc": "old utoc", "Navy_abcde_P.ucas": "old ucas", "Navy_abcde_P.pak": "old pak",
		"slotfiles/s1/MI_old.uasset": "old material", "slotfiles/s1/MI_old.uexp": "old material data", manifestName: "old manifest", "icons/s1.png": "old icon"}
	for rel, b := range old {
		os.MkdirAll(filepath.Dir(filepath.Join(dest, rel)), 0o755)
		os.WriteFile(filepath.Join(dest, rel), []byte(b), 0o644)
	}
	mb, _ := json.Marshal(navyManifest())
	files := map[string][]byte{"skinstudio.json": mb, "Mods/Navy/Navy_abcde_P.utoc": []byte("new utoc"), "Mods/Navy/Navy_abcde_P.pak": []byte("new pak"),
		"Mods/Navy/slotfiles/s1/MI_new.uasset": []byte("new material"), "Mods/Navy/slotfiles/s1/MI_new.uexp": []byte("new material data")}
	broken := map[string][]byte{"!Mods/Navy/Navy_abcde_P.ucas": []byte("new ucas, damaged")}
	for n, b := range files {
		broken[n] = b
	}
	update := ImportPlan{Update: true, SlotDir: "Mods/Navy/slotfiles/"}
	if err := installNavy(t, shareZip(t, broken), dest, update); err == nil {
		t.Fatal("a damaged zip should not install")
	}
	if got := dirState(dest); !sameState(got, old) {
		t.Fatalf("the installed copy changed:\n%v\nwant\n%v", got, old)
	}
	fresh := filepath.Join(mods, "Navy_2")
	if err := installNavy(t, shareZip(t, broken), fresh, ImportPlan{SlotDir: update.SlotDir}); err == nil {
		t.Fatal("a damaged zip should not install")
	}
	if _, err := os.Stat(fresh); !os.IsNotExist(err) {
		t.Fatal("half a mod was left in the mods folder")
	}
	// a good zip replaces the old copy, with nothing left over
	files["Mods/Navy/Navy_abcde_P.ucas"] = []byte("new ucas")
	if err := installNavy(t, shareZip(t, files), dest, update); err != nil {
		t.Fatal(err)
	}
	got := dirState(dest)
	var m ModManifest
	if err := json.Unmarshal([]byte(got[manifestName]), &m); err != nil || m.Key != "Navy_abcde" || m.Folder != "Navy" {
		t.Fatalf("manifest: %v %+v", err, m)
	}
	// the icon file it names is where this program keeps icons, not the path in the zip
	if m.Slots[0].IconFile != "icons/s1.png" {
		t.Fatalf("icon file %q", m.Slots[0].IconFile)
	}
	delete(got, manifestName)
	want := map[string]string{"Navy_abcde_P.utoc": "new utoc", "Navy_abcde_P.ucas": "new ucas", "Navy_abcde_P.pak": "new pak",
		"slotfiles/s1/MI_new.uasset": "new material", "slotfiles/s1/MI_new.uexp": "new material data", "icons/s1.png": "old icon"}
	if !sameState(got, want) {
		t.Fatalf("after the update:\n%v\nwant\n%v", got, want)
	}
}

// The icon pictures a manifest names are only ever icons/<key>.png in the mod's folder.
func TestSharedIconFilesStayInMod(t *testing.T) {
	m := &ModManifest{Slots: []SlotDef{{Key: "s1", IconFile: "../../../Steam/config/loginusers.vdf"}, {Key: "s2"}},
		Emblems: []EmblemDef{{Key: "e1", IconFile: `..\..\x.png`}}}
	keepOwnIconFiles(m)
	if m.Slots[0].IconFile != "icons/s1.png" || m.Slots[1].IconFile != "" || m.Emblems[0].IconFile != "icons/e1.png" {
		t.Fatalf("%+v %+v", m.Slots, m.Emblems)
	}
}

type zeroReader struct{}

func (zeroReader) Read(b []byte) (int, error) {
	clear(b)
	return len(b), nil
}

// A zip can hold files of zeros that are tiny packed and huge unpacked: the skins' slot files are
// only read up to what a skin mod can need.
func TestImportSlotFilesLimited(t *testing.T) {
	mb, _ := json.Marshal(navyManifest())
	base := map[string][]byte{"skinstudio.json": mb, "Mods/Navy/Navy_abcde_P.utoc": []byte("utoc")}
	slots := ImportPlan{SlotDir: "Mods/Navy/slotfiles/"}
	// too many files for one skin
	many := map[string][]byte{}
	for n, b := range base {
		many[n] = b
	}
	for i := 0; i <= maxSlotFilesPerSkin; i++ {
		many[fmt.Sprintf("Mods/Navy/slotfiles/s1/MI_%d.uasset", i)] = []byte("x")
	}
	// and the same in a slotfiles.zip, as 2.0.2 to 2.0.8 shared them
	var inner bytes.Buffer
	izw := zip.NewWriter(&inner)
	for i := 0; i <= maxSlotFilesPerSkin; i++ {
		w, _ := izw.Create(fmt.Sprintf("s1/MI_%d.uasset", i))
		w.Write([]byte("x"))
	}
	izw.Close()
	oldZip := map[string][]byte{"Mods/Navy/" + slotFilesZip: inner.Bytes()}
	for n, b := range base {
		oldZip[n] = b
	}
	// a few files of 50 MB of zeros: a small zip, but more than any skin mod has
	big := filepath.Join(t.TempDir(), "big.zip")
	f, _ := os.Create(big)
	zw := zip.NewWriter(f)
	for n, b := range base {
		w, _ := zw.Create(n)
		w.Write(b)
	}
	for i := 0; i < 3; i++ {
		w, _ := zw.Create(fmt.Sprintf("Mods/Navy/slotfiles/s1/MI_%d.uasset", i))
		io.CopyN(w, zeroReader{}, 50<<20)
	}
	zw.Close()
	f.Close()
	mods := t.TempDir()
	cases := []struct {
		what string
		zip  string
		plan ImportPlan
	}{{"many files", shareZip(t, many), slots}, {"many files in slotfiles.zip", shareZip(t, oldZip), ImportPlan{SlotZip: "Mods/Navy/" + slotFilesZip}}, {"large files", big, slots}}
	for _, c := range cases {
		dest := filepath.Join(mods, "Navy")
		err := installNavy(t, c.zip, dest, c.plan)
		if err == nil || !strings.Contains(err.Error(), "than a skin mod needs") {
			t.Errorf("%s: %v", c.what, err)
		}
		if _, err := os.Stat(dest); !os.IsNotExist(err) {
			t.Errorf("%s: something was installed", c.what)
		}
	}
}

// A mod made some other way is installed as it is, but its text files never take the place of
// this program's own, and nothing in it lands outside the mods folder.
func TestImportedTextsKeepOwnFiles(t *testing.T) {
	root := t.TempDir()
	dataDir = root
	ue4ss := filepath.Join(root, "ue4ss", "Mods")
	mods := filepath.Join(ue4ss, "IoStoreLoaderMod")
	texts := filepath.Join(ue4ss, "SkinSlotText", "Scripts", "texts")
	os.MkdirAll(mods, 0o755)
	os.MkdirAll(texts, 0o755)
	own := []string{"SkinStudioNames.ini", "SkinStudio_Navy_abcde.ini", "SkinStudioRepack_Cool.ini"}
	for _, n := range own {
		os.WriteFile(filepath.Join(texts, n), []byte("mine"), 0o644)
	}
	s := Settings{ModsDir: mods, Workspace: filepath.Join(root, "ws"), UE4SSMods: ue4ss}
	td := "ue4ss/Mods/SkinSlotText/Scripts/texts/"
	files := map[string][]byte{
		"Cool/Cool_P.utoc":                synthTOC(7, "../../../", []string{"Live/Content/Cool/T_Cool.uasset"}),
		"Cool/Cool_P.ucas":                []byte("ucas"),
		"../../../escaped/Evil_P.utoc":    synthTOC(8, "../../../", []string{"Live/Content/Evil/T_Evil.uasset"}),
		td + "SkinStudioNames.ini":        []byte("theirs"),
		td + "skinstudio_Navy_abcde.ini":  []byte("theirs"),
		td + "SKINSTUDIOREPACK_Cool.ini":  []byte("theirs"),
		td + "Cool.ini":                   []byte("theirs"),
		td + "../../../../../escaped.ini": []byte("theirs"),
	}
	if _, err := doImport(testJob(), s, shareZip(t, files)); err != nil {
		t.Fatal(err)
	}
	for _, n := range own {
		if b, _ := os.ReadFile(filepath.Join(texts, n)); string(b) != "mine" {
			t.Errorf("%s was replaced", n)
		}
	}
	for _, n := range []string{"Imported_SkinStudioNames.ini", "Imported_skinstudio_Navy_abcde.ini", "Imported_SKINSTUDIOREPACK_Cool.ini", "Cool.ini"} {
		if b, _ := os.ReadFile(filepath.Join(texts, n)); string(b) != "theirs" {
			t.Errorf("%s was not written", n)
		}
	}
	if got := scanMods(mods); len(got) != 1 || got[0].Name != "Cool" {
		t.Fatalf("installed: %+v", got)
	}
	for _, p := range []string{filepath.Join(root, "escaped"), filepath.Join(root, "escaped.ini"), filepath.Join(ue4ss, "escaped.ini")} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s was written", p)
		}
	}
	// a copy that fails part-way leaves no half mod in the mods folder
	broken := map[string][]byte{"Hot/Hot_P.utoc": synthTOC(9, "../../../", []string{"Live/Content/Hot/T_Hot.uasset"}), "!Hot/Hot_P.ucas": []byte("damaged")}
	if _, err := doImport(testJob(), s, shareZip(t, broken)); err == nil {
		t.Fatal("a damaged zip should not install")
	}
	if _, err := os.Stat(filepath.Join(mods, "Hot")); !os.IsNotExist(err) {
		t.Fatal("half a mod was left in the mods folder")
	}
}

// Paths from a zip or folder that could lead outside it, on Linux or on Windows.
func TestUnsafeArchivePaths(t *testing.T) {
	for _, p := range []string{"", "/etc/x", "../x", "a/../x", "a/..", "C:/x", "c:x", `a\..\x`, `\\server\share\x`, "a/.../x", "a/.. /x", "a/x.",
		"a/x ", "con", "Mods/NUL/x.utoc", "aux.ini", "LPT1.utoc", "com9", "COM¹.txt", "conin$", "a/b\x00.utoc", "a/\x1fb"} {
		if safeRel(p) {
			t.Errorf("%q passed", p)
		}
	}
	for _, p := range []string{"a/b.utoc", "./Mods/x.utoc", "Mods/Console/x.utoc", "Mods/COM10/x", "Mods/.hidden/x", "My Mod/x.utoc", "a/b..c"} {
		if !safeRel(p) {
			t.Errorf("%q refused", p)
		}
	}
	if !safeGamePath("/Game/SkinStudio/A_abcde/s1/MI_X") || safeGamePath("/Game/SkinStudio//MI_X") || safeGamePath("/Game/A/") || safeGamePath("/Other/A") || safeGamePath("/Game/A/../B") {
		t.Error("game paths judged wrongly")
	}
	// a link inside a folder being imported is not followed
	dir := t.TempDir()
	secret := filepath.Join(t.TempDir(), "secret.txt")
	os.WriteFile(secret, []byte("secret"), 0o644)
	os.MkdirAll(filepath.Join(dir, "icons"), 0o755)
	os.WriteFile(filepath.Join(dir, "icons", "s2.png"), []byte("png"), 0o644)
	if err := os.Symlink(secret, filepath.Join(dir, "icons", "s1.png")); err != nil {
		t.Skip("links cannot be made here")
	}
	src, err := openSource(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if _, err := src.read("icons/s1.png", 1<<20); err == nil || len(src.files) != 1 {
		t.Fatalf("a link was followed: %v %v", err, src.files)
	}
	if b, err := src.read("icons/s2.png", 1<<20); err != nil || string(b) != "png" {
		t.Fatalf("a plain file could not be read: %v", err)
	}
}
