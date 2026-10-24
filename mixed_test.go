package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A skin like the F-14's: eight tiles, of which 1007 and 1008 are stored at a quarter of the
// size of the others. Scaled down here: 512-pixel tiles, with 1007 and 1008 at 128 pixels.
func TestMixedTileSizes(t *testing.T) {
	for _, c := range []struct {
		format string
		bb     int
		kind   Kind
		name   string
	}{{"PF_DXT1", 8, KindColour, "T_PP0023_f14d_101_C"}, {"PF_BC7", 16, KindNormal, "T_PP0023_f14d_101_NO"}} {
		small := map[int]int{6: 2, 7: 2}
		uexp, ubulk := synthVTMixed(8, 1, 512, 128, 4, c.format, c.bb, small)
		vt, err := ParseVTex(uexp)
		if err != nil {
			t.Fatalf("%s: %v", c.format, err)
		}
		if err := vt.ResolveChunks(uexp, ubulk); err != nil {
			t.Fatal(err)
		}
		for bx := 0; bx < 8; bx++ {
			if got := vt.BlockMip(bx, 0); got != small[bx] {
				t.Fatalf("tile %d: stored %d steps smaller, want %d", 1001+bx, got, small[bx])
			}
		}
		if note := vt.SmallTilesNote(); note != "In the game, tiles 1007 and 1008 are 128 x 128 instead of 512 x 512." {
			t.Fatalf("note: %q", note)
		}
		// 2 squares x 16 tiles are absent at full size, 2 x 4 at half size
		absent := 0
		for ty := 0; ty < vt.Tables[0].H; ty++ {
			for tx := 0; tx < vt.Tables[0].W; tx++ {
				if !vt.tileStored(0, tx, ty) {
					absent++
				}
			}
		}
		if absent != 32 {
			t.Fatalf("expected 32 tiles absent at full size, found %d", absent)
		}

		// a picture whose two small squares hold 128-pixel content, each pixel repeated 4 x 4
		pic := smoothPicture(vt.Width, vt.Height, 8, 1, 7)
		native := map[int]*Img{}
		for _, bx := range []int{6, 7} {
			native[bx] = smoothPicture(128, 128, 1, 1, int64(20+bx))
			pic.Paste(repeatEach(native[bx], 4), bx*512, 0)
		}
		ne, nb, err := vt.Rebuild(uexp, ubulk, pic, nil)
		if err != nil {
			t.Fatalf("%s: rebuild: %v", c.format, err)
		}
		vt2, err := ParseVTex(ne)
		if err != nil {
			t.Fatal(err)
		}
		if err := vt2.ResolveChunks(ne, nb); err != nil {
			t.Fatalf("%s: rebuilt files do not verify: %v", c.format, err)
		}
		if len(nb) != len(ubulk) || len(ne) != len(uexp) {
			t.Fatal("rebuilt files changed size")
		}
		// reading it back: no error any more, and the small squares come back enlarged
		full, err := vt2.DecodeFull(nb, nil)
		if err != nil {
			t.Fatalf("%s: full-size picture: %v", c.format, err)
		}
		if d := meanAbsDiff(full, pic); d > 4 {
			t.Fatalf("%s: picture differs from the source by %.2f", c.format, d)
		}
		for _, bx := range []int{6, 7} {
			sq := full.Sub(bx*512, 0, 512, 512)
			if meanAbsDiff(repeatEach(everyNth(sq, 4), 4), sq) != 0 {
				t.Fatalf("%s: tile %d is not a clean enlargement of what is stored", c.format, 1001+bx)
			}
			if d := meanAbsDiff(everyNth(sq, 4), native[bx]); d > 4 {
				t.Fatalf("%s: tile %d differs from its source by %.2f", c.format, 1001+bx, d)
			}
		}
		// every stored size can be read, including the two where the small squares are filled in
		want := pic
		for m := 0; m < vt2.Mips; m++ {
			if m > 0 {
				want = halve(want, vt2.SRGB)
			}
			got, err := vt2.DecodeMip(nb, m)
			if err != nil {
				t.Fatalf("%s: size %d: %v", c.format, m, err)
			}
			if d := meanAbsDiff(got, want); d > 6 && want.W/8 >= 8 {
				t.Fatalf("%s: size %d differs by %.2f", c.format, m, d)
			}
		}

		// tiles out: 1007 and 1008 are saved at the size the game stores
		dir := t.TempDir()
		outs, err := WriteTiles(full, 8, 1, KindColour, false, dir, c.name, ".png", nil, vt2.BlockMip)
		if err != nil || len(outs) != 8 {
			t.Fatalf("wrote %d tiles (%v)", len(outs), err)
		}
		for bx := 0; bx < 8; bx++ {
			w, h, err := ImageSize(filepath.Join(dir, c.name+"."+itoa(1001+bx)+".png"))
			wantPx := 512 >> uint(small[bx])
			if err != nil || w != wantPx || h != wantPx {
				t.Fatalf("tile %d saved at %d x %d, want %d (%v)", 1001+bx, w, h, wantPx, err)
			}
		}
		// ... and back in again unchanged, with no complaint about their size
		files, _ := listImages(dir)
		sets := FindPainted(files, []string{c.name}, nil)
		rec := PlanRecipe(c.kind, c.name, vt2, sets)
		if !rec.Ready || len(rec.Notes) == 0 || !strings.Contains(rec.Notes[0], "tiles 1007 and 1008 are 128 x 128") {
			t.Fatalf("%s: plan: %+v", c.format, rec)
		}
		back, notes, err := Compose(c.kind, c.name, vt2, nb, sets)
		if err != nil || meanAbsDiff(back, full) != 0 || len(notes) != 0 {
			t.Fatalf("%s: tiles did not come back as the same picture (%v, notes %v)", c.format, err, notes)
		}
		again, bulk2, err := vt2.Rebuild(ne, nb, back, nil)
		if err != nil {
			t.Fatalf("%s: second rebuild: %v", c.format, err)
		}
		vt3, _ := ParseVTex(again)
		if err := vt3.ResolveChunks(again, bulk2); err != nil {
			t.Fatal(err)
		}
		full3, err := vt3.DecodeFull(bulk2, nil)
		if err != nil || meanAbsDiff(full3, full) > 3 {
			t.Fatalf("%s: second round drifted (%v)", c.format, err)
		}

		// a full-size picture for a small tile is accepted and reduced, and the log says so
		big := resize(native[6], 512, 512)
		if err := SaveImage(filepath.Join(dir, c.name+".1007.png"), big, SaveRGB); err != nil {
			t.Fatal(err)
		}
		files, _ = listImages(dir)
		_, notes, err = Compose(c.kind, c.name, vt2, nb, FindPainted(files, []string{c.name}, nil))
		if err != nil || len(notes) != 1 || !strings.Contains(notes[0], "the game stores this tile at 128 x 128") {
			t.Fatalf("%s: full-size picture for a small tile: %v %v", c.format, err, notes)
		}
		// an odd size is still resized, with the usual note
		SaveImage(filepath.Join(dir, c.name+".1007.png"), resize(native[6], 64, 64), SaveRGB)
		files, _ = listImages(dir)
		_, notes, err = Compose(c.kind, c.name, vt2, nb, FindPainted(files, []string{c.name}, nil))
		if err != nil || len(notes) != 1 || !strings.Contains(notes[0], "was resized from 64 x 64") {
			t.Fatalf("%s: odd-size picture for a small tile: %v %v", c.format, err, notes)
		}
		os.RemoveAll(dir)
	}
}

// A hole that is not a whole small square is still a fault and is still reported.
func TestUnexpectedHoleIsReported(t *testing.T) {
	uexp, ubulk := synthVTMixed(4, 1, 512, 128, 4, "PF_DXT1", 8, nil)
	vt, err := ParseVTex(uexp)
	if err != nil {
		t.Fatal(err)
	}
	if err := vt.ResolveChunks(uexp, ubulk); err != nil {
		t.Fatal(err)
	}
	// knock one tile range out of the full-size table by hand
	tb := &vt.Tables[0]
	tb.Addr = append([]uint32{tb.Addr[0], tb.Addr[0] + 5, tb.Addr[0] + 6}, tb.Addr[1:]...)
	tb.Offs = append([]uint32{tb.Offs[0], 0xFFFFFFFF, tb.Offs[0] + 6}, tb.Offs[1:]...)
	if _, err := vt.DecodeFull(ubulk, nil); err == nil || !strings.Contains(err.Error(), "not stored in this file") {
		t.Fatalf("a stray hole should be reported, got %v", err)
	}
	if _, _, err := vt.Rebuild(uexp, ubulk, NewImg(vt.Width, vt.Height), nil); err == nil {
		t.Fatal("rebuilding over a stray hole should fail")
	}
}
