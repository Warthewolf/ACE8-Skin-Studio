package main

// Reading a .usmap mappings file (as made by UE4SS or Dumper-7), uncompressed. Only used when one is
// chosen in Settings, in case a game update changes the layouts built into the program.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
)

var usmapKinds = []string{"ByteProperty", "BoolProperty", "IntProperty", "FloatProperty", "ObjectProperty", "NameProperty", "DelegateProperty",
	"DoubleProperty", "ArrayProperty", "StructProperty", "StrProperty", "TextProperty", "InterfaceProperty",
	"MulticastDelegateProperty", "WeakObjectProperty", "LazyObjectProperty", "AssetObjectProperty", "SoftObjectProperty",
	"UInt64Property", "UInt32Property", "UInt16Property", "Int64Property", "Int16Property", "Int8Property", "MapProperty",
	"SetProperty", "EnumProperty", "FieldPathProperty", "OptionalProperty", "Utf8StrProperty", "AnsiStrProperty"}

func LoadUsmapSchema(path string) (sc *Schema, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if r := recover(); r != nil {
			sc, err = nil, fmt.Errorf("the mappings file could not be read (%v)", r)
		}
	}()
	if len(b) < 16 || binary.LittleEndian.Uint16(b) != 0x30C4 {
		return nil, errors.New("this is not a .usmap mappings file")
	}
	ver := b[2]
	o := 3
	if ver >= 1 {
		if binary.LittleEndian.Uint32(b[o:]) != 0 {
			return nil, errors.New("mappings files with package versions are not handled")
		}
		o += 4
	}
	method := b[o]
	o++
	csize, dsize := int(binary.LittleEndian.Uint32(b[o:])), int(binary.LittleEndian.Uint32(b[o+4:]))
	o += 8
	if method != 0 || csize != dsize {
		return nil, errors.New("compressed mappings files are not handled; save it uncompressed")
	}
	d := b[o : o+dsize]
	p := 0
	u8 := func() int { v := d[p]; p++; return int(v) }
	u16 := func() int { v := binary.LittleEndian.Uint16(d[p:]); p += 2; return int(v) }
	u32 := func() uint32 { v := binary.LittleEndian.Uint32(d[p:]); p += 4; return v }
	u64 := func() uint64 { v := binary.LittleEndian.Uint64(d[p:]); p += 8; return v }
	var names []string
	for i, n := 0, int(u32()); i < n; i++ {
		var l int
		if ver >= 2 {
			l = u16()
		} else {
			l = u8()
		}
		names = append(names, string(d[p:p+l]))
		p += l
	}
	nm := func(i uint32) string {
		if i == 0xFFFFFFFF {
			return ""
		}
		return names[i]
	}
	sc = &Schema{Structs: map[string][]*Slot{}, Enums: map[string]map[int64]string{}, Source: path}
	for i, n := 0, int(u32()); i < n; i++ {
		en := nm(u32())
		var cnt int
		if ver >= 3 {
			cnt = u16()
		} else {
			cnt = u8()
		}
		m := map[int64]string{}
		for k := 0; k < cnt; k++ {
			if ver >= 4 {
				v := int64(u64())
				m[v] = nm(u32())
			} else {
				m[int64(k)] = nm(u32())
			}
		}
		sc.Enums[en] = m
	}
	var ptype func() PType
	ptype = func() PType {
		k := u8()
		if k >= len(usmapKinds) {
			panic(fmt.Sprintf("property kind %d", k))
		}
		t := PType{Kind: usmapKinds[k]}
		switch t.Kind {
		case "EnumProperty":
			in := ptype()
			t.Inner = &in
			t.Enum = nm(u32())
		case "StructProperty":
			t.Struct = nm(u32())
		case "ArrayProperty", "SetProperty", "OptionalProperty":
			in := ptype()
			t.Inner = &in
		case "MapProperty":
			kt, vt := ptype(), ptype()
			t.Inner, t.Value = &kt, &vt
		}
		return t
	}
	type raw struct {
		super string
		count int
		props []struct {
			idx, arr int
			name     string
			t        PType
		}
	}
	all := map[string]*raw{}
	for i, n := 0, int(u32()); i < n; i++ {
		r := &raw{}
		sn := nm(u32())
		r.super = nm(u32())
		r.count = u16()
		spc := u16()
		for k := 0; k < spc; k++ {
			var x struct {
				idx, arr int
				name     string
				t        PType
			}
			x.idx = u16()
			x.arr = u8()
			x.name = nm(u32())
			x.t = ptype()
			r.props = append(r.props, x)
		}
		all[sn] = r
	}
	for name := range all {
		var slots []*Slot
		for s, guard := name, 0; s != "" && guard < 64; guard++ {
			r := all[s]
			if r == nil {
				break
			}
			local := make([]*Slot, r.count)
			for _, x := range r.props {
				for k := 0; k < x.arr; k++ {
					if x.idx+k < r.count {
						n := x.name
						if x.arr > 1 {
							n = fmt.Sprintf("%s[%d]", x.name, k)
						}
						local[x.idx+k] = &Slot{Name: n, Type: x.t}
					}
				}
			}
			slots = append(slots, local...)
			s = r.super
		}
		sc.Structs[name] = slots
	}
	return sc, nil
}
