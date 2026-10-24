package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func loadRawVT(t *testing.T, base string) (*VTex, []byte, []byte, []byte) {
	t.Helper()
	needFile(t, base+".ubulk")
	f := texFilesFor(base + ".uexp")
	vt, ue, ub, err := LoadVTex(f)
	if err != nil {
		t.Fatal(err)
	}
	ua, err := os.ReadFile(f.UAsset)
	if err != nil {
		t.Fatal(err)
	}
	return vt, ua, ue, ub
}

// The Su-57's colour map is DXT1 and its normal map BC7, at the same size. Laying the colour map out
// as BC7 must give exactly the layout the game's own BC7 file has.
func TestReshapeMatchesGameBC7(t *testing.T) {
	c, cua, cue, cub := loadRawVT(t, su57Raw+"/Textures/001/T_PP0021_su57_001_C")
	no, noua, noue, noub := loadRawVT(t, su57Raw+"/Textures/001/T_PP0021_su57_001_NO")
	nua, nexp, nbulk, nt, err := reshapeVT(c, cua, cue, cub, TexOptions{BC7: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(nbulk) != len(noub) {
		t.Fatalf(".ubulk %d bytes, the game's BC7 file has %d", len(nbulk), len(noub))
	}
	if nt.Format != "PF_BC7" || nt.TileBytes != no.TileBytes || !reflect.DeepEqual(nt.MipBase, no.MipBase) || !reflect.DeepEqual(nt.ChunkOfMip, no.ChunkOfMip) || !reflect.DeepEqual(nt.Tables, no.Tables) {
		t.Fatalf("layout differs from the game's BC7 file:\n%+v\n%+v", nt.MipBase, no.MipBase)
	}
	for i := range nt.Chunks {
		a, b := nt.Chunks[i], no.Chunks[i]
		if a.Start != b.Start || a.Size != b.Size || a.PayloadSize != b.PayloadSize || a.BulkIndex != b.BulkIndex || !bytes.Equal(a.codecRaw, b.codecRaw) {
			t.Fatalf("chunk %d: %+v, the game has %+v", i, a, b)
		}
		if !bytes.Equal(nbulk[a.Start:a.Start+int(a.PayloadSize)], noub[b.Start:b.Start+int(b.PayloadSize)]) {
			t.Fatalf("chunk %d header differs", i)
		}
	}
	// the description is two bytes shorter (PF_BC7 against PF_DXT1, twice), the skip offset follows
	if len(nexp) != len(cue)-2 {
		t.Fatalf(".uexp %d bytes from %d", len(nexp), len(cue))
	}
	np, err := ParsePackage(nua, nexp)
	if err != nil {
		t.Fatal(err)
	}
	gp, err := ParsePackage(noua, noue)
	if err != nil {
		t.Fatal(err)
	}
	if np.Names[1] != "PF_BC7" || gp.Names[1] != "PF_BC7" {
		t.Fatalf("names %v / %v", np.Names, gp.Names)
	}
	nr, _ := np.DataResources()
	gr, _ := gp.DataResources()
	if !reflect.DeepEqual(nr, gr) {
		t.Fatalf("data resources\n%+v\nthe game's\n%+v", nr, gr)
	}
	skip := func(ue []byte, v *VTex) int64 { return int64(binary.LittleEndian.Uint64(ue[v.pfAt-36:])) }
	if skip(nexp, nt) != skip(cue, c)-2 || skip(noue, no) != skip(cue, c)-2 {
		t.Fatalf("skip offsets %d, %d (DXT1 %d)", skip(nexp, nt), skip(noue, no), skip(cue, c))
	}
	// everything else in the description is the colour map's own
	if !bytes.Equal(nexp[:c.pfAt-36], cue[:c.pfAt-36]) || !bytes.Equal(nexp[c.pfAt-28:c.pfAt], cue[c.pfAt-28:c.pfAt]) {
		t.Fatal("the properties before the pixel data changed")
	}
}

// standIn is the Su-57 colour map's real header with small stand-in pixels (five squares of px).
func standIn(t *testing.T, px int, smallBy map[int]int) (*VTex, []byte, []byte, []byte) {
	t.Helper()
	needFile(t, su57Raw+"/Textures/001/T_PP0021_su57_001_C.uasset")
	ue, ub := synthVTMixed(5, 1, px, 128, 4, "PF_DXT1", 8, smallBy)
	p, err := LoadPackage(su57Raw + "/Textures/001/T_PP0021_su57_001_C")
	if err != nil {
		t.Fatal(err)
	}
	// the stand-in's description, behind the real properties
	_, at, _ := findPixelFormat(p.UExp, 0)
	_, sat, _ := findPixelFormat(ue, 0)
	exp := append(append([]byte(nil), p.UExp[:at]...), ue[sat:len(ue)-12]...)
	// it ends like the real one: no more pixel formats (8 bytes) after the chunks
	exp = append(exp, make([]byte, 8)...)
	vt, err := ParseVTex(exp)
	if err != nil {
		t.Fatal(err)
	}
	if err := vt.ResolveChunks(exp, ub); err != nil {
		t.Fatal(err)
	}
	binary.LittleEndian.PutUint64(exp[at-36:], uint64(vt.chunkEnd-(at-36)))
	ua, ue2, err := p.Write(PackageEdit{Exports: [][]byte{exp}, UExpTail: p.UExp[len(p.UExp)-4:]})
	if err != nil {
		t.Fatal(err)
	}
	vt, err = ParseVTex(ue2)
	if err != nil {
		t.Fatal(err)
	}
	if err := vt.ResolveChunks(ue2, ub); err != nil {
		t.Fatal(err)
	}
	return vt, ua, ue2, ub
}

func TestReshapeRoundTrip(t *testing.T) {
	for _, opts := range []TexOptions{{BC7: true}, {NoMips: true}, {BC7: true, NoMips: true}} {
		vt, ua, ue, ub := standIn(t, 256, nil)
		pic := smoothPicture(vt.Width, vt.Height, 5, 1, 3)
		nua, nexp, nbulk, err := rebuildTexture(vt, ua, ue, ub, pic, opts, nil)
		if err != nil {
			t.Fatalf("%+v: %v", opts, err)
		}
		p, err := ParsePackage(nua, nexp)
		if err != nil {
			t.Fatalf("%+v: %v", opts, err)
		}
		nt, err := ParseVTex(nexp)
		if err != nil {
			t.Fatal(err)
		}
		if err := nt.ResolveChunks(nexp, nbulk); err != nil { // checksums are right
			t.Fatalf("%+v: %v", opts, err)
		}
		wantFmt, wantMips := "PF_DXT1", vt.Mips
		if opts.BC7 {
			wantFmt = "PF_BC7"
		}
		if opts.NoMips {
			wantMips = 1
		}
		if nt.Format != wantFmt || nt.Mips != wantMips || len(nt.Chunks) != len(nt.chunkSet()) {
			t.Fatalf("%+v: %s with %d sizes, %d chunks", opts, nt.Format, nt.Mips, len(nt.Chunks))
		}
		rs, err := p.DataResources()
		if err != nil || len(rs) != len(nt.Chunks) {
			t.Fatalf("%+v: %d data resources for %d chunks (%v)", opts, len(rs), len(nt.Chunks), err)
		}
		for i, c := range nt.Chunks {
			if rs[c.BulkIndex].SerialOffset != int64(c.Start) || rs[c.BulkIndex].SerialSize != int64(c.Size) || rs[c.BulkIndex].Flags != 0 || rs[c.BulkIndex].LegacyFlags&bulkInSeparateFile == 0 {
				t.Fatalf("%+v: chunk %d at %d (%d bytes), its entry says %+v", opts, i, c.Start, c.Size, rs[c.BulkIndex])
			}
		}
		got, err := nt.DecodeMip(nbulk, 0)
		if err != nil {
			t.Fatal(err)
		}
		limit := 2.5
		if opts.BC7 {
			limit = 1.0
		}
		if d := meanAbsDiff(got, pic); d > limit {
			t.Fatalf("%+v: full size differs from the picture by %.2f", opts, d)
		}
		// a new slot moves it to a package path of its own
		q, err := ParsePackage(nua, nexp)
		if err != nil {
			t.Fatal(err)
		}
		mua, mue, err := q.Retarget("/Game/SkinStudio/Test/s1/T_PP0021_su57_001_C", nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParsePackage(mua, mue); err != nil {
			t.Fatal(err)
		}
		mt, err := ParseVTex(mue)
		if err != nil || mt.ResolveChunks(mue, nbulk) != nil {
			t.Fatalf("%+v: moved copy does not read back", opts)
		}
	}
}

func (t *VTex) chunkSet() map[uint32]bool {
	m := map[uint32]bool{}
	for _, c := range t.ChunkOfMip {
		m[c] = true
	}
	return m
}

// A square the game keeps smaller than the rest gets full-size tiles when only the full size is kept.
func TestNoMipsSmallSquare(t *testing.T) {
	vt, ua, ue, ub := standIn(t, 256, map[int]int{3: 1})
	if vt.BlockMip(3, 0) != 1 {
		t.Fatalf("stand-in square 4 is stored at size %d", vt.BlockMip(3, 0))
	}
	pic := smoothPicture(vt.Width, vt.Height, 5, 1, 4)
	nua, nexp, nbulk, err := rebuildTexture(vt, ua, ue, ub, pic, TexOptions{NoMips: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePackage(nua, nexp); err != nil {
		t.Fatal(err)
	}
	nt, err := ParseVTex(nexp)
	if err != nil || nt.ResolveChunks(nexp, nbulk) != nil {
		t.Fatal("does not read back")
	}
	for b := 0; b < 5; b++ {
		if nt.BlockMip(b, 0) != 0 {
			t.Fatalf("square %d stored at size %d", b, nt.BlockMip(b, 0))
		}
	}
	if want := fullTable(vt.Tables[0].W, vt.Tables[0].H, vt.Tables[0].MaxAddr); !reflect.DeepEqual(nt.Tables[0], want) {
		t.Fatalf("table %+v", nt.Tables[0])
	}
	got, err := nt.DecodeMip(nbulk, 0)
	if err != nil {
		t.Fatal(err)
	}
	if d := meanAbsDiff(got, pic); d > 2.5 {
		t.Fatalf("differs by %.2f", d)
	}
}

// fullTable gives the same table as the game's for squares that are all full size.
func TestFullTableMatchesGame(t *testing.T) {
	c, _, _, _ := loadRawVT(t, su57Raw+"/Textures/001/T_PP0021_su57_001_C")
	for m, tb := range c.Tables {
		if got := fullTable(tb.W, tb.H, tb.MaxAddr); !reflect.DeepEqual(got, tb) {
			t.Fatalf("size %d: %+v, the game has %+v", m, got, tb)
		}
	}
}

// Both builds pass the choice through: a new skin slot (kept when the skin is changed without new
// pictures) and a replaced game skin.
func TestBuildsStoreColourAsAsked(t *testing.T) {
	e := newSlotEnv(t)
	// the raw colour texture with the game's real description in front of stand-in pixels
	orig, ua, ue, ub := standIn(t, 256, nil)
	if err := writePackageFiles(filepath.Join(e.rawDir, "T_PP0021_su57_001_C"), ua, ue, ub); err != nil {
		t.Fatal(err)
	}
	j := testJob()
	red := [3]uint8{200, 40, 40}
	paint := e.paint(t, red)
	want := TexOptions{BC7: true, NoMips: true}
	res, err := doBuildSlot(j, e.s, slotReq{planReq: planReq{SkinDir: e.rawDir, PaintDir: paint, Textures: []string{"T_PP0021_su57_001_C"}, Tex: want},
		ModTitle: "Red", Name: "Red", Unlock: UnlockAlways, Icon: "base"})
	if err != nil {
		t.Fatal(err)
	}
	m, err := loadProject(e.s, res.(slotResult).Mod)
	if err != nil {
		t.Fatal(err)
	}
	if m.Slots[0].Tex != want {
		t.Fatalf("slot records %+v", m.Slots[0].Tex)
	}
	own := slotAssetDir(m.Key, "s1")
	check := func(path, format string, mips int) {
		t.Helper()
		vt, uexp, ubulk, err := LoadVTex(texFilesFor(path + ".uexp"))
		if err != nil {
			t.Fatal(err)
		}
		if vt.Format != format || vt.Mips != mips {
			t.Fatalf("%s: %s with %d sizes", path, vt.Format, vt.Mips)
		}
		if _, err := LoadPackage(path); err != nil {
			t.Fatal(err)
		}
		if err := vt.ResolveChunks(uexp, ubulk); err != nil {
			t.Fatal(err)
		}
		px, err := vt.decodeRect(ubulk, 0, 300, 100, 4, 4)
		if err != nil {
			t.Fatal(err)
		}
		if c := px.Pix[:3]; int(c[0])-int(red[0]) > 4 || int(red[0])-int(c[0]) > 4 || c[1] > 48 {
			t.Fatalf("%s: colour %v", path, c)
		}
	}
	packed := filepath.Join(e.s.projectDir(m.Key), "packed", m.Container+".input")
	check(gameFile(packed, "Live", own+"T_PP0021_su57_001_C"), "PF_BC7", 1)
	// changing only the name keeps the texture, and what it records
	if _, err := doBuildSlot(j, e.s, slotReq{planReq: planReq{SkinDir: e.rawDir}, Mod: m.Key, Slot: "s1", Name: "Red Two", Unlock: UnlockAlways, Icon: "base"}); err != nil {
		t.Fatal(err)
	}
	if m, _ = loadProject(e.s, m.Key); m.Slots[0].Tex != want || m.Slots[0].Name != "Red Two" {
		t.Fatalf("after the change: %+v", m.Slots[0])
	}
	check(gameFile(packed, "Live", own+"T_PP0021_su57_001_C"), "PF_BC7", 1)
	// replacing the game's skin, BC7 with its smaller sizes
	if _, err := doBuild(j, e.s, planReq{SkinDir: e.rawDir, PaintDir: paint, Textures: []string{"T_PP0021_su57_001_C"}, ModName: "RedReplace", KeepInput: true, Tex: TexOptions{BC7: true}}); err != nil {
		t.Fatal(err)
	}
	in := filepath.Join(e.s.modsOut(), "RedReplace", "pack_input")
	check(gameFile(in, "Live", "/Game/Vehicles/Aircraft/PP/PP0021_su57/Textures/001/T_PP0021_su57_001_C"), "PF_BC7", orig.Mips)
}
