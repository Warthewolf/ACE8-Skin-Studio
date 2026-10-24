package main

// Reading and writing the game's save files (Campaign.sav, System.sav).
//
// They are Unreal "GVAS" save games (UE 5.4 tagged properties with full type names). At the top
// level there are three properties: SavedVersion, PackedData (an array of bytes) and Checksum.
// PackedData holds the real save: a run of property lists, each ending with "None" (Campaign.sav
// has two: the common data, with the unlock lists, then the campaign data). Checksum is a CRC-32
// of the PackedData bytes, started from saveChecksumSeed instead of 0.
//
// Only what is needed is read into a tree: property lists, structs and arrays of structs. Every
// other value is kept as its bytes, and each tag keeps its own bytes, so a save that is read and
// written again without a change comes out byte for byte the same. Changing a value fixes the sizes
// above it and the checksum when the save is written.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"strings"
	"unicode/utf16"
)

const saveChecksumSeed = 0x41916EBD

// gvType is a property's full type name: ArrayProperty(StructProperty(LiveUnlockSaveData(/Script/Live))).
type gvType struct {
	Name string
	Sub  []gvType
}

func (t gvType) String() string {
	if len(t.Sub) == 0 {
		return t.Name
	}
	parts := make([]string, len(t.Sub))
	for i, s := range t.Sub {
		parts[i] = s.String()
	}
	return t.Name + "(" + strings.Join(parts, ",") + ")"
}

func (t gvType) sub(i int) gvType {
	if i < len(t.Sub) {
		return t.Sub[i]
	}
	return gvType{}
}

// Structs the engine writes as plain bytes rather than as a property list.
var gvNativeStructs = map[string]bool{"Guid": true, "DateTime": true, "Timespan": true, "Vector": true, "Vector2D": true,
	"Vector4": true, "Rotator": true, "Quat": true, "LinearColor": true, "Color": true, "IntPoint": true, "IntVector": true,
	"Box": true, "Box2D": true, "Plane": true, "Transform": true, "SoftObjectPath": true, "SoftClassPath": true,
	"GameplayTagContainer": true}

type gvProp struct {
	Name   string
	Type   gvType
	Flags  byte
	head   []byte    // the tag as it was read, from the name to the value
	sizeAt int       // where the value's size is in head
	Val    []byte    // the value's bytes, when it is not read as a tree
	Fields *gvList   // a struct read as a property list
	Elems  []*gvList // an array of structs read as property lists
}

// gvList is a property list and the "None" that ends it.
type gvList struct {
	Props []*gvProp
	end   []byte
}

func (l *gvList) find(name string) *gvProp {
	if l == nil {
		return nil
	}
	for _, p := range l.Props {
		if p.Name == name {
			return p
		}
	}
	return nil
}

// ------------------------------------------------------------------ reading

type gvReader struct {
	b []byte
	p int
}

var errSaveShort = errors.New("the save ends too early")

func (r *gvReader) take(n int) ([]byte, error) {
	if n < 0 || r.p+n > len(r.b) {
		return nil, errSaveShort
	}
	v := r.b[r.p : r.p+n]
	r.p += n
	return v, nil
}

func (r *gvReader) i32() (int32, error) {
	v, err := r.take(4)
	if err != nil {
		return 0, err
	}
	return int32(binary.LittleEndian.Uint32(v)), nil
}

func (r *gvReader) u8() (byte, error) {
	v, err := r.take(1)
	if err != nil {
		return 0, err
	}
	return v[0], nil
}

func (r *gvReader) fstr() (string, error) {
	n, err := r.i32()
	if err != nil {
		return "", err
	}
	switch {
	case n == 0:
		return "", nil
	case n > 0:
		if n > 1<<20 {
			return "", errors.New("a text in the save is too long")
		}
		v, err := r.take(int(n))
		if err != nil {
			return "", err
		}
		return string(bytes.TrimRight(v, "\x00")), nil
	default:
		if -n > 1<<20 {
			return "", errors.New("a text in the save is too long")
		}
		v, err := r.take(int(-n) * 2)
		if err != nil {
			return "", err
		}
		u := make([]uint16, len(v)/2)
		for i := range u {
			u[i] = binary.LittleEndian.Uint16(v[2*i:])
		}
		for len(u) > 0 && u[len(u)-1] == 0 {
			u = u[:len(u)-1]
		}
		return string(utf16.Decode(u)), nil
	}
}

func (r *gvReader) typeName(depth int) (gvType, error) {
	if depth > 16 {
		return gvType{}, errors.New("a type in the save is nested too deeply")
	}
	name, err := r.fstr()
	if err != nil {
		return gvType{}, err
	}
	n, err := r.i32()
	if err != nil {
		return gvType{}, err
	}
	if n < 0 || n > 64 {
		return gvType{}, fmt.Errorf("unexpected type name in the save (%s)", name)
	}
	t := gvType{Name: name}
	for i := 0; i < int(n); i++ {
		s, err := r.typeName(depth + 1)
		if err != nil {
			return gvType{}, err
		}
		t.Sub = append(t.Sub, s)
	}
	return t, nil
}

// readList reads one property list up to and including its "None".
func readGVList(r *gvReader, depth int) (*gvList, error) {
	if depth > 32 {
		return nil, errors.New("the save is nested too deeply")
	}
	l := &gvList{}
	for {
		start := r.p
		name, err := r.fstr()
		if err != nil {
			return nil, err
		}
		if name == "None" {
			l.end = append([]byte(nil), r.b[start:r.p]...)
			return l, nil
		}
		if name == "" {
			return nil, fmt.Errorf("unexpected data in the save at %d", start)
		}
		t, err := r.typeName(0)
		if err != nil {
			return nil, err
		}
		sizeAt := r.p - start
		size, err := r.i32()
		if err != nil {
			return nil, err
		}
		flags, err := r.u8()
		if err != nil {
			return nil, err
		}
		if flags&1 != 0 { // array index
			if _, err := r.take(4); err != nil {
				return nil, err
			}
		}
		if flags&2 != 0 { // property guid
			if _, err := r.take(16); err != nil {
				return nil, err
			}
		}
		if flags&4 != 0 {
			return nil, fmt.Errorf("the save uses a property layout this program does not know (%s)", name)
		}
		head := append([]byte(nil), r.b[start:r.p]...)
		val, err := r.take(int(size))
		if err != nil {
			return nil, err
		}
		p := &gvProp{Name: name, Type: t, Flags: flags, head: head, sizeAt: sizeAt, Val: append([]byte(nil), val...)}
		p.readTree(depth)
		l.Props = append(l.Props, p)
	}
}

// readTree reads a struct or an array of structs into lists when it can; when it cannot, the
// value stays as bytes (it is still written back as it was).
func (p *gvProp) readTree(depth int) {
	switch p.Type.Name {
	case "StructProperty":
		if gvNativeStructs[p.Type.sub(0).Name] {
			return
		}
		r := &gvReader{b: p.Val}
		if l, err := readGVList(r, depth+1); err == nil && r.p == len(p.Val) {
			p.Fields, p.Val = l, nil
		}
	case "ArrayProperty":
		in := p.Type.sub(0)
		if in.Name != "StructProperty" || gvNativeStructs[in.sub(0).Name] {
			return
		}
		r := &gvReader{b: p.Val}
		n, err := r.i32()
		if err != nil || n < 0 || int(n) > len(p.Val) {
			return
		}
		elems := make([]*gvList, 0, n)
		for i := 0; i < int(n); i++ {
			l, err := readGVList(r, depth+1)
			if err != nil {
				return
			}
			elems = append(elems, l)
		}
		if r.p == len(p.Val) {
			p.Elems, p.Val = elems, nil
		}
	}
}

// ------------------------------------------------------------------ writing

func (l *gvList) write(w *bytes.Buffer) {
	for _, p := range l.Props {
		p.write(w)
	}
	w.Write(l.end)
}

func (l *gvList) bytes() []byte {
	var w bytes.Buffer
	l.write(&w)
	return w.Bytes()
}

func (p *gvProp) value() []byte {
	switch {
	case p.Fields != nil:
		return p.Fields.bytes()
	case p.Elems != nil:
		var w bytes.Buffer
		w.Write(binary.LittleEndian.AppendUint32(nil, uint32(len(p.Elems))))
		for _, e := range p.Elems {
			e.write(&w)
		}
		return w.Bytes()
	}
	return p.Val
}

func (p *gvProp) write(w *bytes.Buffer) {
	v := p.value()
	h := append([]byte(nil), p.head...)
	binary.LittleEndian.PutUint32(h[p.sizeAt:], uint32(len(v)))
	w.Write(h)
	w.Write(v)
}

func (p *gvProp) clone() *gvProp {
	q := *p
	q.head = append([]byte(nil), p.head...)
	if p.Val != nil {
		q.Val = append([]byte{}, p.Val...)
	}
	q.Fields = p.Fields.clone()
	if p.Elems != nil {
		q.Elems = make([]*gvList, len(p.Elems))
		for i, e := range p.Elems {
			q.Elems[i] = e.clone()
		}
	}
	return &q
}

func (l *gvList) clone() *gvList {
	if l == nil {
		return nil
	}
	c := &gvList{end: append([]byte(nil), l.end...)}
	for _, p := range l.Props {
		c.Props = append(c.Props, p.clone())
	}
	return c
}

// ------------------------------------------------------------------ values

func (p *gvProp) isArrayOf(inner string) bool {
	return p != nil && p.Type.Name == "ArrayProperty" && p.Type.sub(0).Name == inner && p.Val != nil
}

// uint32s reads an ArrayProperty(UInt32Property) (or IntProperty: the bits are the same).
func (p *gvProp) uint32s() ([]uint32, error) {
	if !p.isArrayOf("UInt32Property") && !p.isArrayOf("IntProperty") {
		return nil, fmt.Errorf("%s is not a list of numbers", p.Name)
	}
	if len(p.Val) < 4 {
		return nil, fmt.Errorf("%s is too short", p.Name)
	}
	n := int(binary.LittleEndian.Uint32(p.Val))
	if n < 0 || 4+4*n != len(p.Val) {
		return nil, fmt.Errorf("%s does not have the size it should", p.Name)
	}
	out := make([]uint32, n)
	for i := range out {
		out[i] = binary.LittleEndian.Uint32(p.Val[4+4*i:])
	}
	return out, nil
}

func (p *gvProp) setUint32s(v []uint32) {
	b := make([]byte, 4+4*len(v))
	binary.LittleEndian.PutUint32(b, uint32(len(v)))
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[4+4*i:], x)
	}
	p.Val = b
}

// int32Val reads an IntProperty or UInt32Property.
func (p *gvProp) int32Val() (int32, bool) {
	if p == nil || (p.Type.Name != "IntProperty" && p.Type.Name != "UInt32Property") || len(p.Val) != 4 {
		return 0, false
	}
	return int32(binary.LittleEndian.Uint32(p.Val)), true
}

func (p *gvProp) setInt32(v int32) {
	p.Val = binary.LittleEndian.AppendUint32(nil, uint32(v))
}

// text reads a StrProperty, NameProperty or EnumProperty.
func (p *gvProp) text() string {
	if p == nil || p.Val == nil {
		return ""
	}
	s, _ := (&gvReader{b: p.Val}).fstr()
	return s
}

// boolVal reads a BoolProperty (its value is in the tag's flags).
func (p *gvProp) boolVal() bool { return p != nil && p.Flags&0x10 != 0 }

// walk calls fn for every property in the list and inside its structs and arrays of structs.
func (l *gvList) walk(fn func(p *gvProp)) {
	if l == nil {
		return
	}
	for _, p := range l.Props {
		fn(p)
		p.Fields.walk(fn)
		for _, e := range p.Elems {
			e.walk(fn)
		}
	}
}

// ------------------------------------------------------------------ the file

type saveFile struct {
	head   []byte    // the file header, up to the first property
	top    *gvList   // SavedVersion, PackedData, Checksum
	tail   []byte    // what comes after the top list
	Class  string    // the save game's class (/Script/Live.LiveCampaignSaveGame)
	Packed []*gvList // the property lists inside PackedData
}

func parseSave(b []byte) (*saveFile, error) {
	r := &gvReader{b: b}
	if magic, err := r.take(4); err != nil || string(magic) != "GVAS" {
		return nil, errors.New("this is not one of the game's save files")
	}
	ver, err := r.i32()
	if err != nil {
		return nil, err
	}
	n := 4 // UE4 version
	if ver >= 3 {
		n += 4 // UE5 version
	}
	n += 2 + 2 + 2 + 4 // engine major, minor, patch, changelist
	if _, err := r.take(n); err != nil {
		return nil, err
	}
	if _, err := r.fstr(); err != nil { // engine branch
		return nil, err
	}
	if _, err := r.i32(); err != nil { // custom version format
		return nil, err
	}
	cv, err := r.i32()
	if err != nil {
		return nil, err
	}
	if cv < 0 || cv > 4096 {
		return nil, errors.New("this save's header is not one this program knows")
	}
	if _, err := r.take(int(cv) * 20); err != nil {
		return nil, err
	}
	f := &saveFile{}
	if f.Class, err = r.fstr(); err != nil {
		return nil, err
	}
	if r.p < len(b) && b[r.p] == 0 { // UE 5.4 adds a 0 byte here
		r.p++
	}
	f.head = append([]byte(nil), b[:r.p]...)
	if f.top, err = readGVList(r, 0); err != nil {
		return nil, fmt.Errorf("the save could not be read: %v", err)
	}
	f.tail = append([]byte(nil), b[r.p:]...)
	pd := f.top.find("PackedData")
	if !pd.isArrayOf("ByteProperty") || len(pd.Val) < 4 {
		return nil, errors.New("this save has no PackedData; it is not one this program knows")
	}
	data := pd.Val[4:]
	if int(binary.LittleEndian.Uint32(pd.Val)) != len(data) {
		return nil, errors.New("this save's PackedData does not have the size it says")
	}
	sum, ok := f.top.find("Checksum").int32Val()
	if !ok {
		return nil, errors.New("this save has no checksum; it is not one this program knows")
	}
	if crc32.Update(saveChecksumSeed, crc32.IEEETable, data) != uint32(sum) {
		return nil, errors.New("this save's checksum does not match (the file is damaged, or the game has changed how it saves)")
	}
	pr := &gvReader{b: data}
	for pr.p < len(data) {
		l, err := readGVList(pr, 0)
		if err != nil {
			return nil, fmt.Errorf("the save's data could not be read: %v", err)
		}
		f.Packed = append(f.Packed, l)
	}
	return f, nil
}

// bytes writes the save, with the sizes and the checksum worked out again.
func (f *saveFile) bytes() []byte {
	var data bytes.Buffer
	for _, l := range f.Packed {
		l.write(&data)
	}
	pd := f.top.find("PackedData")
	pd.Val = binary.LittleEndian.AppendUint32(nil, uint32(data.Len()))
	pd.Val = append(pd.Val, data.Bytes()...)
	f.top.find("Checksum").setInt32(int32(crc32.Update(saveChecksumSeed, crc32.IEEETable, data.Bytes())))
	var w bytes.Buffer
	w.Write(f.head)
	f.top.write(&w)
	w.Write(f.tail)
	return w.Bytes()
}

// prop finds a property at the top of the save's data lists.
func (f *saveFile) prop(name string) *gvProp {
	for _, l := range f.Packed {
		if p := l.find(name); p != nil {
			return p
		}
	}
	return nil
}

func (f *saveFile) walk(fn func(p *gvProp)) {
	for _, l := range f.Packed {
		l.walk(fn)
	}
}
