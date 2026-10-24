package main

// Unversioned property data: how cooked UE5 packages store an object's properties. Each object
// starts with a short header of "fragments" saying which of its properties are present (and which
// of those are zero), followed by the present values in property order. Reading it needs the
// property layout of every struct involved; the layouts this program needs are built in
// (schema.json, made from the game's mappings file), and a mappings file chosen in Settings can
// stand in for them after a game update.

import (
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf16"
)

//go:embed schema.json
var builtinSchemaJSON []byte

// PType is a property type: ["IntProperty"], ["ArrayProperty", inner], ["StructProperty", "Name"],
// ["EnumProperty", inner, "EnumName"], ["MapProperty", key, value].
type PType struct {
	Kind   string
	Inner  *PType
	Value  *PType // map values
	Struct string
	Enum   string
}

func (t *PType) UnmarshalJSON(b []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil || len(raw) == 0 {
		return errors.New("bad type")
	}
	if err := json.Unmarshal(raw[0], &t.Kind); err != nil {
		return err
	}
	switch t.Kind {
	case "StructProperty":
		return json.Unmarshal(raw[1], &t.Struct)
	case "EnumProperty":
		t.Inner = &PType{}
		if err := json.Unmarshal(raw[1], t.Inner); err != nil {
			return err
		}
		return json.Unmarshal(raw[2], &t.Enum)
	case "ArrayProperty", "SetProperty", "OptionalProperty":
		t.Inner = &PType{}
		return json.Unmarshal(raw[1], t.Inner)
	case "MapProperty":
		t.Inner, t.Value = &PType{}, &PType{}
		if err := json.Unmarshal(raw[1], t.Inner); err != nil {
			return err
		}
		return json.Unmarshal(raw[2], t.Value)
	}
	return nil
}

type Slot struct {
	Name string
	Type PType
}

type Schema struct {
	Structs map[string][]*Slot // unversioned slot order: own properties first, then the parent's
	Enums   map[string]map[int64]string
	Source  string
}

func (s *Schema) UnmarshalJSON(b []byte) error {
	var raw struct {
		Game    string
		Structs map[string][]json.RawMessage
		Enums   map[string][][2]json.RawMessage
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	s.Source = raw.Game
	s.Structs = map[string][]*Slot{}
	for name, slots := range raw.Structs {
		var out []*Slot
		for _, r := range slots {
			if string(r) == "null" {
				out = append(out, nil)
				continue
			}
			var pair []json.RawMessage
			if err := json.Unmarshal(r, &pair); err != nil || len(pair) != 2 {
				return fmt.Errorf("bad slot in %s", name)
			}
			sl := &Slot{}
			if err := json.Unmarshal(pair[0], &sl.Name); err != nil {
				return err
			}
			if err := json.Unmarshal(pair[1], &sl.Type); err != nil {
				return err
			}
			out = append(out, sl)
		}
		s.Structs[name] = out
	}
	s.Enums = map[string]map[int64]string{}
	for name, vals := range raw.Enums {
		m := map[int64]string{}
		for _, v := range vals {
			var num int64
			var n string
			json.Unmarshal(v[0], &num)
			json.Unmarshal(v[1], &n)
			m[num] = n
		}
		s.Enums[name] = m
	}
	return nil
}

var builtinSchema = func() *Schema {
	s := &Schema{}
	if err := json.Unmarshal(builtinSchemaJSON, s); err != nil {
		panic("built-in schema: " + err.Error())
	}
	return s
}()

// ------------------------------------------------------------------ values

// Obj is one struct's present properties, in slot order.
type Obj struct {
	Struct string
	Props  []Prop
}

type Prop struct {
	Name string
	Zero bool // stored as "zero" (no bytes): the game's own default
	Val  any
}

// SoftPath is a soft object reference: package and asset name plus a sub-path.
type SoftPath struct {
	Package, Asset, Sub string
}

func (s SoftPath) String() string {
	if s.Asset == "" || s.Asset == "None" {
		return s.Package
	}
	return s.Package + "." + s.Asset
}

// softPathTo is the reference to the main object of a package: /Game/A/B -> /Game/A/B.B.
func softPathTo(pkg string) SoftPath {
	return SoftPath{Package: pkg, Asset: pkg[strings.LastIndexByte(pkg, '/')+1:]}
}

// ObjRef is a reference to an import (negative) or export (positive).
type ObjRef int32

// Native is a struct stored as plain bytes (vectors, colours, GUIDs ...).
type Native []byte

func (o *Obj) Get(name string) (any, bool) {
	for _, p := range o.Props {
		if p.Name == name {
			if p.Zero {
				return nil, true
			}
			return p.Val, true
		}
	}
	return nil, false
}

// Set replaces (or adds, in slot order) a property's value.
func (o *Obj) Set(sc *Schema, name string, v any) error {
	slots := sc.Structs[o.Struct]
	idx := -1
	for i, s := range slots {
		if s != nil && s.Name == name {
			idx = i
		}
	}
	if idx < 0 {
		return fmt.Errorf("%s has no property %s", o.Struct, name)
	}
	np := Prop{Name: name, Val: v}
	for i, p := range o.Props {
		if p.Name == name {
			o.Props[i] = np
			return nil
		}
	}
	// insert in slot order
	pos := len(o.Props)
	for i, p := range o.Props {
		if slotIndex(slots, p.Name) > idx {
			pos = i
			break
		}
	}
	o.Props = append(o.Props[:pos], append([]Prop{np}, o.Props[pos:]...)...)
	return nil
}

func slotIndex(slots []*Slot, name string) int {
	for i, s := range slots {
		if s != nil && s.Name == name {
			return i
		}
	}
	return -1
}

func (o *Obj) Int(name string) int64 {
	v, _ := o.Get(name)
	switch x := v.(type) {
	case int32:
		return int64(x)
	case uint32:
		return int64(x)
	case int64:
		return x
	}
	return 0
}

func (o *Obj) Str(name string) string {
	v, _ := o.Get(name)
	switch x := v.(type) {
	case string:
		return x
	case SoftPath:
		return x.String()
	}
	return ""
}

// Clone copies an object deeply.
func (o *Obj) Clone() *Obj {
	n := &Obj{Struct: o.Struct, Props: make([]Prop, len(o.Props))}
	for i, p := range o.Props {
		n.Props[i] = Prop{Name: p.Name, Zero: p.Zero, Val: cloneVal(p.Val)}
	}
	return n
}

func cloneVal(v any) any {
	switch x := v.(type) {
	case *Obj:
		return x.Clone()
	case []any:
		out := make([]any, len(x))
		for i := range x {
			out[i] = cloneVal(x[i])
		}
		return out
	case Native:
		return append(Native(nil), x...)
	}
	return v
}

// ------------------------------------------------------------------ reading

var nativeSizes = map[string]int{"Vector": 24, "Vector2D": 16, "Vector4": 32, "Rotator": 24, "Quat": 32, "LinearColor": 16, "Color": 4,
	"Guid": 16, "DateTime": 8, "Timespan": 8, "IntPoint": 8, "IntVector": 12, "Vector3f": 12, "Vector2f": 8,
	"FrameNumber": 4, "Plane": 32, "Box2D": 33, "Box": 49, "Matrix": 128}

type PropReader struct {
	d     []byte
	p     int
	names []string
	sc    *Schema
}

func (r *PropReader) need(n int) {
	if n < 0 || r.p+n > len(r.d) {
		panic(fmt.Sprintf("data ends at %d, needed %d more bytes", r.p, n))
	}
}
func (r *PropReader) u8() byte { r.need(1); v := r.d[r.p]; r.p++; return v }
func (r *PropReader) u16() uint16 {
	r.need(2)
	v := binary.LittleEndian.Uint16(r.d[r.p:])
	r.p += 2
	return v
}
func (r *PropReader) u32() uint32 {
	r.need(4)
	v := binary.LittleEndian.Uint32(r.d[r.p:])
	r.p += 4
	return v
}
func (r *PropReader) i32() int32 { return int32(r.u32()) }
func (r *PropReader) u64() uint64 {
	r.need(8)
	v := binary.LittleEndian.Uint64(r.d[r.p:])
	r.p += 8
	return v
}
func (r *PropReader) fname() string {
	i, n := r.i32(), r.i32()
	if i < 0 || int(i) >= len(r.names) {
		panic(fmt.Sprintf("name %d out of range", i))
	}
	return joinFName(r.names[i], n)
}
func (r *PropReader) fstring() string {
	s, p, err := readFStr(r.d, r.p)
	if err != nil {
		panic(err.Error())
	}
	r.p = p
	return s
}

func (r *PropReader) value(t *PType) any {
	switch t.Kind {
	case "ByteProperty":
		return r.u8()
	case "BoolProperty":
		return r.u8() != 0
	case "IntProperty":
		return r.i32()
	case "UInt32Property":
		return r.u32()
	case "FloatProperty":
		return math.Float32frombits(r.u32())
	case "DoubleProperty":
		return math.Float64frombits(r.u64())
	case "Int64Property":
		return int64(r.u64())
	case "UInt64Property":
		return r.u64()
	case "Int16Property":
		return int16(r.u16())
	case "UInt16Property":
		return r.u16()
	case "Int8Property":
		return int8(r.u8())
	case "NameProperty":
		return r.fname()
	case "StrProperty":
		return r.fstring()
	case "ObjectProperty", "WeakObjectProperty", "LazyObjectProperty", "InterfaceProperty":
		return ObjRef(r.i32())
	case "SoftObjectProperty", "AssetObjectProperty":
		return SoftPath{r.fname(), r.fname(), r.fstring()}
	case "EnumProperty":
		v := r.value(t.Inner)
		if t.Inner.Kind == "NameProperty" {
			return v
		}
		var num int64
		switch x := v.(type) {
		case byte:
			num = int64(x)
		case int32:
			num = int64(x)
		case uint32:
			num = int64(x)
		}
		if n, ok := r.sc.Enums[t.Enum][num]; ok {
			return n
		}
		return num
	case "StructProperty":
		return r.structVal(t.Struct)
	case "ArrayProperty":
		n := int(r.i32())
		if n < 0 || n > 1_000_000 {
			panic(fmt.Sprintf("array of %d", n))
		}
		out := make([]any, n)
		for i := range out {
			out[i] = r.value(t.Inner)
		}
		return out
	}
	panic("property type " + t.Kind + " is not handled")
}

func (r *PropReader) structVal(name string) any {
	if n, ok := nativeSizes[name]; ok {
		r.need(n)
		v := Native(append([]byte(nil), r.d[r.p:r.p+n]...))
		r.p += n
		return v
	}
	if name == "SoftObjectPath" {
		return SoftPath{r.fname(), r.fname(), r.fstring()}
	}
	return r.object(name)
}

// object reads one struct or object in the unversioned form.
func (r *PropReader) object(name string) *Obj {
	slots, ok := r.sc.Structs[name]
	if !ok {
		panic("the layout of " + name + " is not known")
	}
	type frag struct {
		skip, num int
		hasZero   bool
	}
	var frags []frag
	zeros := 0
	for {
		v := r.u16()
		f := frag{skip: int(v & 0x7f), hasZero: v&0x80 != 0, num: int(v >> 9)}
		frags = append(frags, f)
		if f.hasZero {
			zeros += f.num
		}
		if v&0x100 != 0 {
			break
		}
	}
	var mask []bool
	if zeros > 0 {
		var bits uint64
		var nb int
		switch {
		case zeros <= 8:
			bits, nb = uint64(r.u8()), 8
		case zeros <= 16:
			bits, nb = uint64(r.u16()), 16
		default:
			words := (zeros + 31) / 32
			if words > 2 {
				panic("too many zero properties")
			}
			for w := 0; w < words; w++ {
				bits |= uint64(r.u32()) << (32 * w)
			}
			nb = words * 32
		}
		for i := 0; i < nb; i++ {
			mask = append(mask, bits>>i&1 != 0)
		}
	}
	o := &Obj{Struct: name}
	idx, mi := 0, 0
	for _, f := range frags {
		idx += f.skip
		for k := 0; k < f.num; k++ {
			zero := false
			if f.hasZero {
				zero = mask[mi]
				mi++
			}
			if idx >= len(slots) || slots[idx] == nil {
				panic(fmt.Sprintf("%s: property %d is not known (the game may have been updated)", name, idx))
			}
			s := slots[idx]
			p := Prop{Name: s.Name, Zero: zero}
			if !zero {
				p.Val = r.value(&s.Type)
			}
			o.Props = append(o.Props, p)
			idx++
		}
	}
	return o
}

// ReadObject reads a struct of the given layout from data at pos; it returns the object and where it ended.
func ReadObject(sc *Schema, names []string, data []byte, pos int, structName string) (o *Obj, end int, err error) {
	r := &PropReader{d: data, p: pos, names: names, sc: sc}
	defer func() {
		if x := recover(); x != nil {
			o, end, err = nil, r.p, fmt.Errorf("%v", x)
		}
	}()
	o = r.object(structName)
	return o, r.p, nil
}

// ------------------------------------------------------------------ names while writing

// NameTable is a package's name map that can grow. New names go in front of the names only the
// header uses, because retoc keeps only the names export data can refer to.
type NameTable struct {
	Names       []string
	ExportCount int // names[:ExportCount] may be used by export data (before additions)
	Added       []string
	index       map[string]int
}

func NewNameTable(names []string, exportCount int) *NameTable {
	nt := &NameTable{Names: append([]string(nil), names...), ExportCount: exportCount}
	nt.reindex()
	return nt
}

func (nt *NameTable) reindex() {
	nt.index = map[string]int{}
	for i, n := range nt.Names {
		if _, ok := nt.index[n]; !ok {
			nt.index[n] = i
		}
	}
}

func (nt *NameTable) usable(i int) bool { return i < nt.ExportCount+len(nt.Added) }

// Ref returns how a name text is stored, adding the text to the map when it is missing.
func (nt *NameTable) Ref(s string) (int32, int32) {
	base, num := splitFName(s)
	if i, ok := nt.index[base]; ok && nt.usable(i) {
		return int32(i), num
	}
	return int32(nt.add(base)), num
}

func (nt *NameTable) add(s string) int {
	pos := nt.ExportCount + len(nt.Added)
	nt.Added = append(nt.Added, s)
	nt.Names = append(nt.Names[:pos], append([]string{s}, nt.Names[pos:]...)...)
	nt.reindex()
	// a name that existed only among the header names is now also present in the usable part
	return pos
}

// ------------------------------------------------------------------ writing

type PropWriter struct {
	out []byte
	nt  *NameTable
	sc  *Schema
}

func (w *PropWriter) u8(v byte)    { w.out = append(w.out, v) }
func (w *PropWriter) u16(v uint16) { w.out = binary.LittleEndian.AppendUint16(w.out, v) }
func (w *PropWriter) u32(v uint32) { w.out = binary.LittleEndian.AppendUint32(w.out, v) }
func (w *PropWriter) u64(v uint64) { w.out = binary.LittleEndian.AppendUint64(w.out, v) }
func (w *PropWriter) fname(s string) {
	i, n := w.nt.Ref(s)
	w.u32(uint32(i))
	w.u32(uint32(n))
}
func (w *PropWriter) fstring(s string) { w.out = append(w.out, fstrBytes(s)...) }

func (w *PropWriter) value(t *PType, v any) {
	bad := func() { panic(fmt.Sprintf("value %v (%T) does not fit %s", v, v, t.Kind)) }
	switch t.Kind {
	case "ByteProperty", "Int8Property":
		switch x := v.(type) {
		case byte:
			w.u8(x)
		case int8:
			w.u8(byte(x))
		default:
			bad()
		}
	case "BoolProperty":
		b, ok := v.(bool)
		if !ok {
			bad()
		}
		if b {
			w.u8(1)
		} else {
			w.u8(0)
		}
	case "IntProperty":
		x, ok := v.(int32)
		if !ok {
			bad()
		}
		w.u32(uint32(x))
	case "UInt32Property":
		x, ok := v.(uint32)
		if !ok {
			bad()
		}
		w.u32(x)
	case "FloatProperty":
		x, ok := v.(float32)
		if !ok {
			bad()
		}
		w.u32(math.Float32bits(x))
	case "DoubleProperty":
		x, ok := v.(float64)
		if !ok {
			bad()
		}
		w.u64(math.Float64bits(x))
	case "Int64Property":
		x, ok := v.(int64)
		if !ok {
			bad()
		}
		w.u64(uint64(x))
	case "UInt64Property":
		x, ok := v.(uint64)
		if !ok {
			bad()
		}
		w.u64(x)
	case "Int16Property":
		x, ok := v.(int16)
		if !ok {
			bad()
		}
		w.u16(uint16(x))
	case "UInt16Property":
		x, ok := v.(uint16)
		if !ok {
			bad()
		}
		w.u16(x)
	case "NameProperty":
		s, ok := v.(string)
		if !ok {
			bad()
		}
		w.fname(s)
	case "StrProperty":
		s, ok := v.(string)
		if !ok {
			bad()
		}
		w.fstring(s)
	case "ObjectProperty", "WeakObjectProperty", "LazyObjectProperty", "InterfaceProperty":
		x, ok := v.(ObjRef)
		if !ok {
			bad()
		}
		w.u32(uint32(x))
	case "SoftObjectProperty", "AssetObjectProperty":
		x, ok := v.(SoftPath)
		if !ok {
			bad()
		}
		w.fname(x.Package)
		w.fname(x.Asset)
		w.fstring(x.Sub)
	case "EnumProperty":
		if t.Inner.Kind == "NameProperty" {
			w.value(t.Inner, v)
			return
		}
		var num int64 = -1
		switch x := v.(type) {
		case string:
			for k, n := range w.sc.Enums[t.Enum] {
				if n == x {
					num = k
				}
			}
			if num < 0 {
				panic(fmt.Sprintf("%s has no value %s", t.Enum, x))
			}
		case int64:
			num = x
		default:
			bad()
		}
		switch t.Inner.Kind {
		case "ByteProperty":
			w.u8(byte(num))
		case "IntProperty", "UInt32Property":
			w.u32(uint32(num))
		default:
			bad()
		}
	case "StructProperty":
		if n, ok := nativeSizes[t.Struct]; ok {
			x, ok := v.(Native)
			if !ok || len(x) != n {
				bad()
			}
			w.out = append(w.out, x...)
			return
		}
		if t.Struct == "SoftObjectPath" {
			x, ok := v.(SoftPath)
			if !ok {
				bad()
			}
			w.fname(x.Package)
			w.fname(x.Asset)
			w.fstring(x.Sub)
			return
		}
		o, ok := v.(*Obj)
		if !ok {
			bad()
		}
		w.object(o)
	case "ArrayProperty":
		arr, ok := v.([]any)
		if !ok {
			bad()
		}
		w.u32(uint32(len(arr)))
		for _, x := range arr {
			w.value(t.Inner, x)
		}
	default:
		panic("cannot write " + t.Kind)
	}
}

func (w *PropWriter) object(o *Obj) {
	slots, ok := w.sc.Structs[o.Struct]
	if !ok {
		panic("the layout of " + o.Struct + " is not known")
	}
	type pres struct {
		idx  int
		p    Prop
		slot *Slot
	}
	var present []pres
	for _, p := range o.Props {
		i := slotIndex(slots, p.Name)
		if i < 0 {
			panic(o.Struct + " has no property " + p.Name)
		}
		present = append(present, pres{i, p, slots[i]})
	}
	for i := 1; i < len(present); i++ {
		if present[i].idx <= present[i-1].idx {
			panic(o.Struct + ": properties out of order")
		}
	}
	type frag struct {
		skip, num int
		hasZero   bool
	}
	var frags []*frag
	prev := -1
	for _, x := range present {
		skip := x.idx - prev - 1
		if len(frags) > 0 && skip == 0 && frags[len(frags)-1].num < 127 {
			frags[len(frags)-1].num++
		} else {
			for skip > 127 {
				frags = append(frags, &frag{skip: 127})
				skip -= 127
			}
			frags = append(frags, &frag{skip: skip, num: 1})
		}
		prev = x.idx
	}
	if len(frags) == 0 {
		frags = []*frag{{}}
	}
	k := 0
	for _, f := range frags {
		for j := 0; j < f.num; j++ {
			if present[k+j].p.Zero {
				f.hasZero = true
			}
		}
		k += f.num
	}
	for n, f := range frags {
		v := uint16(f.skip) | uint16(f.num)<<9
		if f.hasZero {
			v |= 0x80
		}
		if n == len(frags)-1 {
			v |= 0x100
		}
		w.u16(v)
	}
	var mask []bool
	k = 0
	for _, f := range frags {
		if f.hasZero {
			for j := 0; j < f.num; j++ {
				mask = append(mask, present[k+j].p.Zero)
			}
		}
		k += f.num
	}
	if len(mask) > 0 {
		var bits uint64
		for i, z := range mask {
			if z {
				bits |= 1 << i
			}
		}
		switch {
		case len(mask) <= 8:
			w.u8(byte(bits))
		case len(mask) <= 16:
			w.u16(uint16(bits))
		default:
			for wd := 0; wd < (len(mask)+31)/32; wd++ {
				w.u32(uint32(bits >> (32 * wd)))
			}
		}
	}
	for _, x := range present {
		if !x.p.Zero {
			w.value(&x.slot.Type, x.p.Val)
		}
	}
}

// WriteObject serializes an object; names it needs are added to nt.
func WriteObject(sc *Schema, nt *NameTable, o *Obj) (out []byte, err error) {
	w := &PropWriter{nt: nt, sc: sc}
	defer func() {
		if x := recover(); x != nil {
			out, err = nil, fmt.Errorf("%v", x)
		}
	}()
	w.object(o)
	return w.out, nil
}

// utf16Len is used when checking that a text fits the game's limits.
func utf16Len(s string) int { return len(utf16.Encode([]rune(s))) }
