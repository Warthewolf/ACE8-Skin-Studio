package main

// Preparing a model for painting: keep only the faces that use the skin material(s) of one or
// more .glb files (as exported by FModel) and write them as a single clean .glb.

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type gltfDoc struct {
	Accessors []struct {
		BufferView    *int   `json:"bufferView"`
		ByteOffset    int    `json:"byteOffset"`
		ComponentType int    `json:"componentType"`
		Normalized    bool   `json:"normalized"`
		Count         int    `json:"count"`
		Type          string `json:"type"`
	} `json:"accessors"`
	BufferViews []struct {
		Buffer     int `json:"buffer"`
		ByteOffset int `json:"byteOffset"`
		ByteLength int `json:"byteLength"`
		ByteStride int `json:"byteStride"`
	} `json:"bufferViews"`
	Meshes []struct {
		Name       string `json:"name"`
		Primitives []struct {
			Attributes map[string]int `json:"attributes"`
			Indices    *int           `json:"indices"`
			Material   *int           `json:"material"`
			Mode       *int           `json:"mode"`
		} `json:"primitives"`
	} `json:"meshes"`
	Materials []struct {
		Name string `json:"name"`
	} `json:"materials"`
}

type glbFile struct {
	doc gltfDoc
	bin []byte
}

func readGLB(path string) (*glbFile, error) {
	d, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	name := filepath.Base(path)
	if len(d) < 20 || string(d[:4]) != "glTF" {
		return nil, fmt.Errorf("%s is not a .glb model", name)
	}
	jl := int(binary.LittleEndian.Uint32(d[12:]))
	if string(d[16:20]) != "JSON" || 20+jl > len(d) {
		return nil, fmt.Errorf("%s is damaged", name)
	}
	g := &glbFile{}
	if err := json.Unmarshal(d[20:20+jl], &g.doc); err != nil {
		return nil, fmt.Errorf("%s is damaged (%v)", name, err)
	}
	p := 20 + jl
	if p+8 <= len(d) {
		bl := int(binary.LittleEndian.Uint32(d[p:]))
		if string(d[p+4:p+8]) == "BIN\x00" && p+8+bl <= len(d) {
			g.bin = d[p+8 : p+8+bl]
		}
	}
	if g.bin == nil {
		return nil, fmt.Errorf("%s has no embedded mesh data", name)
	}
	return g, nil
}

var gltfComps = map[string]int{"SCALAR": 1, "VEC2": 2, "VEC3": 3, "VEC4": 4}

// floats reads an accessor as float32 values, comps numbers per element.
func (g *glbFile) floats(idx, comps int) ([]float32, error) {
	if idx < 0 || idx >= len(g.doc.Accessors) {
		return nil, errors.New("model refers to missing data")
	}
	a := g.doc.Accessors[idx]
	if gltfComps[a.Type] != comps || a.BufferView == nil || *a.BufferView >= len(g.doc.BufferViews) {
		return nil, errors.New("model data has an unexpected layout")
	}
	bv := g.doc.BufferViews[*a.BufferView]
	size := map[int]int{5120: 1, 5121: 1, 5122: 2, 5123: 2, 5125: 4, 5126: 4}[a.ComponentType]
	if size == 0 {
		return nil, errors.New("model data uses an unsupported number type")
	}
	stride := bv.ByteStride
	if stride == 0 {
		stride = size * comps
	}
	base := bv.ByteOffset + a.ByteOffset
	if a.Count < 0 || base < 0 || (a.Count > 0 && base+(a.Count-1)*stride+size*comps > len(g.bin)) {
		return nil, errors.New("model data runs past the end of the file")
	}
	out := make([]float32, a.Count*comps)
	for i := 0; i < a.Count; i++ {
		for c := 0; c < comps; c++ {
			b := g.bin[base+i*stride+c*size:]
			var v float32
			switch a.ComponentType {
			case 5126:
				v = math.Float32frombits(binary.LittleEndian.Uint32(b))
			case 5121:
				v = float32(b[0])
				if a.Normalized {
					v /= 255
				}
			case 5120:
				v = float32(int8(b[0]))
				if a.Normalized {
					v = max(v/127, -1)
				}
			case 5123:
				v = float32(binary.LittleEndian.Uint16(b))
				if a.Normalized {
					v /= 65535
				}
			case 5122:
				v = float32(int16(binary.LittleEndian.Uint16(b)))
				if a.Normalized {
					v = max(v/32767, -1)
				}
			case 5125:
				v = float32(binary.LittleEndian.Uint32(b))
			}
			out[i*comps+c] = v
		}
	}
	return out, nil
}

func (g *glbFile) indices(idx int) ([]uint32, error) {
	if idx < 0 || idx >= len(g.doc.Accessors) {
		return nil, errors.New("model refers to missing data")
	}
	a := g.doc.Accessors[idx]
	if a.BufferView == nil || *a.BufferView >= len(g.doc.BufferViews) {
		return nil, errors.New("model data has an unexpected layout")
	}
	bv := g.doc.BufferViews[*a.BufferView]
	size := map[int]int{5121: 1, 5123: 2, 5125: 4}[a.ComponentType]
	if size == 0 {
		return nil, errors.New("model uses an unsupported index type")
	}
	stride := bv.ByteStride
	if stride == 0 {
		stride = size
	}
	base := bv.ByteOffset + a.ByteOffset
	if a.Count < 0 || base < 0 || (a.Count > 0 && base+(a.Count-1)*stride+size > len(g.bin)) {
		return nil, errors.New("model data runs past the end of the file")
	}
	out := make([]uint32, a.Count)
	for i := range out {
		b := g.bin[base+i*stride:]
		switch size {
		case 1:
			out[i] = uint32(b[0])
		case 2:
			out[i] = uint32(binary.LittleEndian.Uint16(b))
		default:
			out[i] = binary.LittleEndian.Uint32(b)
		}
	}
	return out, nil
}

// reLoosePart recognises pylon models by file name: DPTK or DPP, with or without a number
// (DPP1, DPP2, DPTK3 ...), or the word pylon. Upper and lower case are treated alike.
var reLoosePart = regexp.MustCompile(`(?i)(DPTK|DPP)[0-9]*|pylon`)

// ModelMaterial describes one material's share of a model file.
type ModelMaterial struct {
	File      string  `json:"file"`
	Name      string  `json:"name"`
	Vertices  int     `json:"vertices"`
	Triangles int     `json:"triangles"`
	UMin      float32 `json:"uMin"`
	UMax      float32 `json:"uMax"`
	VMin      float32 `json:"vMin"`
	VMax      float32 `json:"vMax"`
	Tiles     int     `json:"tiles"`
	Suggested bool    `json:"suggested"`
}

type modelPart struct {
	file, material string
	pos, nrm, uv   []float32
	idx            []uint32
}

func (g *glbFile) parts(file string) ([]modelPart, error) {
	var out []modelPart
	for _, m := range g.doc.Meshes {
		for _, p := range m.Primitives {
			if p.Mode != nil && *p.Mode != 4 {
				continue // triangles only
			}
			pa, ok := p.Attributes["POSITION"]
			if !ok {
				continue
			}
			part := modelPart{file: file, material: "(no material)"}
			if p.Material != nil && *p.Material < len(g.doc.Materials) && g.doc.Materials[*p.Material].Name != "" {
				part.material = g.doc.Materials[*p.Material].Name
			}
			var err error
			if part.pos, err = g.floats(pa, 3); err != nil {
				return nil, err
			}
			n := len(part.pos) / 3
			if na, ok := p.Attributes["NORMAL"]; ok {
				if part.nrm, err = g.floats(na, 3); err != nil || len(part.nrm) != n*3 {
					part.nrm = nil
				}
			}
			if ua, ok := p.Attributes["TEXCOORD_0"]; ok {
				if part.uv, err = g.floats(ua, 2); err != nil || len(part.uv) != n*2 {
					part.uv = nil
				}
			}
			if p.Indices != nil {
				if part.idx, err = g.indices(*p.Indices); err != nil {
					return nil, err
				}
			} else {
				part.idx = make([]uint32, n)
				for i := range part.idx {
					part.idx[i] = uint32(i)
				}
			}
			for _, i := range part.idx {
				if int(i) >= n {
					return nil, errors.New("model has faces that point at missing corners")
				}
			}
			out = append(out, part)
		}
	}
	return out, nil
}

func describePart(p modelPart) ModelMaterial {
	mm := ModelMaterial{File: filepath.Base(p.file), Name: p.material, Vertices: len(p.pos) / 3, Triangles: len(p.idx) / 3}
	if len(p.uv) >= 2 {
		mm.UMin, mm.UMax, mm.VMin, mm.VMax = p.uv[0], p.uv[0], p.uv[1], p.uv[1]
		for i := 0; i < len(p.uv); i += 2 {
			mm.UMin, mm.UMax = min(mm.UMin, p.uv[i]), max(mm.UMax, p.uv[i])
			mm.VMin, mm.VMax = min(mm.VMin, p.uv[i+1]), max(mm.VMax, p.uv[i+1])
		}
		mm.Tiles = int(math.Ceil(float64(mm.UMax) - 1e-4))
	}
	mm.Suggested = strings.Contains(strings.ToLower(p.material), "decal")
	return mm
}

// InspectModels lists the materials of the given .glb files.
func InspectModels(paths []string) ([]ModelMaterial, error) {
	var out []ModelMaterial
	for _, p := range paths {
		g, err := readGLB(p)
		if err != nil {
			return nil, err
		}
		parts, err := g.parts(p)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", filepath.Base(p), err)
		}
		for _, part := range parts {
			out = append(out, describePart(part))
		}
	}
	return out, nil
}

// BuildPaintModel writes one .glb holding only the chosen materials' faces.
// keep is a set of "file|material" keys. Returns the number of tiles the skin spans and notes.
func BuildPaintModel(paths []string, keep map[string]bool, moveLoose bool, outPath string) (int, []string, error) {
	var chosen []modelPart
	for _, p := range paths {
		g, err := readGLB(p)
		if err != nil {
			return 0, nil, err
		}
		parts, err := g.parts(p)
		if err != nil {
			return 0, nil, fmt.Errorf("%s: %v", filepath.Base(p), err)
		}
		for _, part := range parts {
			if keep[filepath.Base(p)+"|"+part.material] {
				if part.uv == nil {
					return 0, nil, fmt.Errorf("%s: material %s has no texture coordinates, so it cannot be painted", filepath.Base(p), part.material)
				}
				chosen = append(chosen, part)
			}
		}
	}
	if len(chosen) == 0 {
		return 0, nil, errors.New("tick at least one material to keep")
	}
	// The biggest part sets how many tiles the skin has; strays beyond that are wrapped back in,
	// which is where the game reads them from.
	big := chosen[0]
	for _, c := range chosen {
		if len(c.pos) > len(big.pos) {
			big = c
		}
	}
	tiles := max(describePart(big).Tiles, 1)
	var notes []string
	for ci := range chosen {
		moved := 0
		uv := chosen[ci].uv
		for i := 0; i < len(uv); i += 2 {
			if uv[i] > float32(tiles)+1e-4 || uv[i] < -1e-4 {
				w := float32(math.Mod(float64(uv[i]), float64(tiles)))
				if w < 0 {
					w += float32(tiles)
				}
				uv[i] = w
				moved++
			}
		}
		if moved > 0 {
			notes = append(notes, fmt.Sprintf("%s: %d corners had texture coordinates outside the %d tiles and were wrapped back in", filepath.Base(chosen[ci].file), moved, tiles))
		}
	}
	// Pylons and similar loose parts are modelled at the origin, inside the fuselage.
	// Drop them below the aircraft so they can be reached with a brush.
	if moveLoose {
		lowest := float32(math.Inf(1))
		for i := 1; i < len(big.pos); i += 3 {
			lowest = min(lowest, big.pos[i])
		}
		for ci := range chosen {
			if chosen[ci].file == big.file || !reLoosePart.MatchString(filepath.Base(chosen[ci].file)) {
				continue
			}
			top := float32(math.Inf(-1))
			bottom := float32(math.Inf(1))
			for i := 1; i < len(chosen[ci].pos); i += 3 {
				top, bottom = max(top, chosen[ci].pos[i]), min(bottom, chosen[ci].pos[i])
			}
			shift := lowest - 0.5 - top
			pos := append([]float32(nil), chosen[ci].pos...)
			for i := 1; i < len(pos); i += 3 {
				pos[i] += shift
			}
			chosen[ci].pos = pos
			lowest = bottom + shift
			notes = append(notes, fmt.Sprintf("%s was moved below the aircraft so it can be painted", filepath.Base(chosen[ci].file)))
		}
	}
	// group by source file: one object per file
	byFile := map[string][]modelPart{}
	var order []string
	for _, c := range chosen {
		if _, ok := byFile[c.file]; !ok {
			order = append(order, c.file)
		}
		byFile[c.file] = append(byFile[c.file], c)
	}
	sort.Strings(order)

	var bin []byte
	type view struct {
		Buffer     int `json:"buffer"`
		ByteOffset int `json:"byteOffset"`
		ByteLength int `json:"byteLength"`
		Target     int `json:"target"`
	}
	type accessor struct {
		BufferView    int       `json:"bufferView"`
		ComponentType int       `json:"componentType"`
		Count         int       `json:"count"`
		Type          string    `json:"type"`
		Min           []float32 `json:"min,omitempty"`
		Max           []float32 `json:"max,omitempty"`
	}
	var views []view
	var accs []accessor
	addF := func(data []float32, comps int, typ string, minmax bool) int {
		for len(bin)%4 != 0 {
			bin = append(bin, 0)
		}
		off := len(bin)
		for _, v := range data {
			bin = binary.LittleEndian.AppendUint32(bin, math.Float32bits(v))
		}
		views = append(views, view{0, off, len(data) * 4, 34962})
		a := accessor{BufferView: len(views) - 1, ComponentType: 5126, Count: len(data) / comps, Type: typ}
		if minmax {
			a.Min, a.Max = make([]float32, comps), make([]float32, comps)
			copy(a.Min, data[:comps])
			copy(a.Max, data[:comps])
			for i := 0; i < len(data); i++ {
				a.Min[i%comps], a.Max[i%comps] = min(a.Min[i%comps], data[i]), max(a.Max[i%comps], data[i])
			}
		}
		accs = append(accs, a)
		return len(accs) - 1
	}
	addI := func(data []uint32) int {
		for len(bin)%4 != 0 {
			bin = append(bin, 0)
		}
		off := len(bin)
		for _, v := range data {
			bin = binary.LittleEndian.AppendUint32(bin, v)
		}
		views = append(views, view{0, off, len(data) * 4, 34963})
		accs = append(accs, accessor{BufferView: len(views) - 1, ComponentType: 5125, Count: len(data), Type: "SCALAR"})
		return len(accs) - 1
	}
	type prim struct {
		Attributes map[string]int `json:"attributes"`
		Indices    int            `json:"indices"`
		Material   int            `json:"material"`
		Mode       int            `json:"mode"`
	}
	type mesh struct {
		Name       string `json:"name"`
		Primitives []prim `json:"primitives"`
	}
	type node struct {
		Name string `json:"name"`
		Mesh int    `json:"mesh"`
	}
	var meshes []mesh
	var nodes []node
	var sceneNodes []int
	for _, f := range order {
		name := strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))
		m := mesh{Name: name}
		for _, part := range byFile[f] {
			attrs := map[string]int{"POSITION": addF(part.pos, 3, "VEC3", true)}
			if part.nrm != nil {
				attrs["NORMAL"] = addF(part.nrm, 3, "VEC3", false)
			}
			attrs["TEXCOORD_0"] = addF(part.uv, 2, "VEC2", false)
			m.Primitives = append(m.Primitives, prim{Attributes: attrs, Indices: addI(part.idx), Material: 0, Mode: 4})
		}
		meshes = append(meshes, m)
		nodes = append(nodes, node{Name: name, Mesh: len(meshes) - 1})
		sceneNodes = append(sceneNodes, len(nodes)-1)
	}
	for len(bin)%4 != 0 {
		bin = append(bin, 0)
	}
	doc := map[string]any{
		"asset":       map[string]string{"version": "2.0", "generator": appName},
		"scene":       0,
		"scenes":      []map[string]any{{"nodes": sceneNodes}},
		"nodes":       nodes,
		"meshes":      meshes,
		"materials":   []map[string]string{{"name": big.material}},
		"buffers":     []map[string]int{{"byteLength": len(bin)}},
		"bufferViews": views,
		"accessors":   accs,
	}
	js, err := json.Marshal(doc)
	if err != nil {
		return 0, nil, err
	}
	for len(js)%4 != 0 {
		js = append(js, ' ')
	}
	out := make([]byte, 0, 28+len(js)+len(bin))
	out = append(out, "glTF"...)
	out = binary.LittleEndian.AppendUint32(out, 2)
	out = binary.LittleEndian.AppendUint32(out, uint32(28+len(js)+len(bin)))
	out = binary.LittleEndian.AppendUint32(out, uint32(len(js)))
	out = append(out, "JSON"...)
	out = append(out, js...)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(bin)))
	out = append(out, "BIN\x00"...)
	out = append(out, bin...)
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return 0, nil, err
	}
	if err := os.WriteFile(outPath, out, 0o644); err != nil {
		return 0, nil, err
	}
	return tiles, notes, nil
}
