package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// synthVT builds an empty streamed texture (.uexp and .ubulk bytes) laid out the way the game's
// files are, for any number of squares, so layouts other than five-in-a-row can be exercised.
func synthVT(blocksW, blocksH, blockPx, tile, border int, format string, blockBytes int) ([]byte, []byte) {
	return synthVTMixed(blocksW, blocksH, blockPx, tile, border, format, blockBytes, nil)
}

// synthVTMixed is synthVT with some squares stored smaller than the others, the way the game
// stores a 1024-pixel tile next to 4096-pixel ones: smallBy[square] = 2 leaves that square
// without tiles in the two largest sizes.
func synthVTMixed(blocksW, blocksH, blockPx, tile, border int, format string, blockBytes int, smallBy map[int]int) ([]byte, []byte) {
	P := tile + 2*border
	tileBytes := (P / 4) * (P / 4) * blockBytes
	W, H := blocksW*blockPx, blocksH*blockPx
	mips := 0
	for s := max(W, H); s >= 1; s /= 2 {
		mips++
		if max(W>>uint(mips), 1) == 1 && max(H>>uint(mips), 1) == 1 {
			break
		}
	}
	mips = min(mips, 12)
	le := binary.LittleEndian
	var ux bytes.Buffer
	w32 := func(v uint32) { binary.Write(&ux, le, v) }
	fstr := func(s string) { w32(uint32(len(s) + 1)); ux.WriteString(s); ux.WriteByte(0) }
	ux.Write(bytes.Repeat([]byte{0xAB}, 96)) // stand-in for the properties before the pixel data
	fstr(format)
	w32(0)
	w32(0)
	w32(1) // virtual
	w32(1) // cooked
	w32(1) // layers
	w32(uint32(blocksW))
	w32(uint32(blocksH))
	w32(uint32(tile))
	w32(uint32(border))
	w32(1)
	w32(uint32(tileBytes))
	w32(uint32(mips))
	w32(uint32(W))
	w32(uint32(H))
	// chunk per mip: the first four sizes get their own chunk, the rest share the last
	chunkOf := make([]int, mips)
	for m := range chunkOf {
		chunkOf[m] = min(m, 4)
	}
	nChunks := chunkOf[mips-1] + 1
	type tab struct {
		w, h       int
		addr, offs []uint32
		count      int
	}
	tabs := make([]tab, mips)
	chunkSize := make([]int, nChunks)
	for c := range chunkSize {
		chunkSize[c] = 4
	}
	base := make([]int, mips)
	for m := 0; m < mips; m++ {
		mw, mh := max(W>>uint(m), 1), max(H>>uint(m), 1)
		tw, th := (mw+tile-1)/tile, (mh+tile-1)/tile
		var codes []uint32
		for y := 0; y < th; y++ {
			for x := 0; x < tw; x++ {
				if bpx := blockPx >> uint(m); bpx >= tile && smallBy[(y*tile/bpx)*blocksW+x*tile/bpx] > m {
					continue // this square is not stored at this size
				}
				codes = append(codes, morton(x, y))
			}
		}
		sort.Slice(codes, func(i, j int) bool { return codes[i] < codes[j] })
		t := tab{w: tw, h: th, count: len(codes)}
		for i, c := range codes {
			if i == 0 || c != codes[i-1]+1 {
				if i > 0 {
					t.addr = append(t.addr, codes[i-1]+1)
					t.offs = append(t.offs, 0xFFFFFFFF)
				}
				t.addr = append(t.addr, c)
				t.offs = append(t.offs, uint32(i))
			}
		}
		t.addr = append(t.addr, codes[len(codes)-1]+1)
		t.offs = append(t.offs, 0xFFFFFFFF)
		tabs[m] = t
		base[m] = chunkSize[chunkOf[m]]
		chunkSize[chunkOf[m]] += len(codes) * tileBytes
	}
	w32(uint32(mips))
	for _, c := range chunkOf {
		w32(uint32(c))
	}
	w32(uint32(mips))
	for _, b := range base {
		w32(uint32(b))
	}
	w32(uint32(mips))
	for _, t := range tabs {
		w32(uint32(t.w))
		w32(uint32(t.h))
		w32(t.addr[len(t.addr)-1])
		w32(uint32(len(t.addr)))
		for _, a := range t.addr {
			w32(a)
		}
		w32(uint32(len(t.offs)))
		for _, o := range t.offs {
			w32(o)
		}
	}
	w32(0)
	w32(0)
	w32(0)
	fstr(format)
	for _, f := range []float32{0.5, 0.5, 0.5, 1} {
		w32(math.Float32bits(f))
	}
	var bulk []byte
	w32(uint32(nChunks))
	for c := 0; c < nChunks; c++ {
		chunk := make([]byte, chunkSize[c])
		sum := sha1.Sum(chunk)
		ux.Write(sum[:])
		w32(uint32(chunkSize[c]))
		w32(4)
		ux.WriteByte(4)
		w32(4)
		w32(uint32(c))
		bulk = append(bulk, chunk...)
	}
	ux.Write(make([]byte, 12))
	return ux.Bytes(), bulk
}

func smoothPicture(w, h, bw, bh int, seed int64) *Img {
	rng := rand.New(rand.NewSource(seed))
	im := NewImg(w, h)
	fx, fy := (2+rng.Float64()*2)*math.Pi/float64(w), (1+rng.Float64())*math.Pi/float64(h) // a few slow waves across the picture
	// Grey only, so block compression can follow it at every size: slow waves plus a step per square.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			v := uint8(70 + (x*bw/w)*12 + (y*bh/h)*30 + int(35*math.Sin(float64(x)*fx)+25*math.Cos(float64(y)*fy)))
			im.Pix[i], im.Pix[i+1], im.Pix[i+2], im.Pix[i+3] = v, v, v, 255
		}
	}
	return im
}

func meanAbsDiff(a, b *Img) float64 {
	if a.W != b.W || a.H != b.H {
		return math.Inf(1)
	}
	s := 0.0
	for i := 0; i < len(a.Pix); i += 4 {
		for c := 0; c < 3; c++ {
			s += math.Abs(float64(a.Pix[i+c]) - float64(b.Pix[i+c]))
		}
	}
	return s / float64(a.W*a.H*3)
}

// Textures are not always five squares in a row. Check the whole path for 7x1, 3x1, 1x1 and 4x2.
func TestOtherTileCounts(t *testing.T) {
	cases := []struct {
		bw, bh, px int
		format     string
		bb         int
	}{{7, 1, 512, "PF_DXT1", 8}, {7, 1, 256, "PF_BC7", 16}, {3, 1, 512, "PF_DXT1", 8}, {1, 1, 1024, "PF_DXT1", 8}, {4, 2, 256, "PF_DXT1", 8}, {10, 1, 256, "PF_DXT1", 8}}
	for _, c := range cases {
		uexp, ubulk := synthVT(c.bw, c.bh, c.px, 128, 4, c.format, c.bb)
		vt, err := ParseVTex(uexp)
		if err != nil {
			t.Fatalf("%dx%d: parse: %v", c.bw, c.bh, err)
		}
		if vt.BlocksW != c.bw || vt.BlocksH != c.bh || vt.Width != c.bw*c.px || vt.Height != c.bh*c.px {
			t.Fatalf("%dx%d: wrong layout read: %+v", c.bw, c.bh, vt)
		}
		if err := vt.ResolveChunks(uexp, ubulk); err != nil {
			t.Fatalf("%dx%d: chunks: %v", c.bw, c.bh, err)
		}
		src := smoothPicture(vt.Width, vt.Height, c.bw, c.bh, int64(c.bw*10+c.bh))
		ne, nb, err := vt.Rebuild(uexp, ubulk, src, nil)
		if err != nil {
			t.Fatalf("%dx%d: rebuild: %v", c.bw, c.bh, err)
		}
		if len(ne) != len(uexp) || len(nb) != len(ubulk) {
			t.Fatalf("%dx%d: file sizes changed", c.bw, c.bh)
		}
		vt2, err := ParseVTex(ne)
		if err != nil {
			t.Fatal(err)
		}
		if err := vt2.ResolveChunks(ne, nb); err != nil {
			t.Fatalf("%dx%d: rebuilt files do not verify: %v", c.bw, c.bh, err)
		}
		full, err := vt2.DecodeFull(nb, nil)
		if err != nil {
			t.Fatal(err)
		}
		if d := meanAbsDiff(full, src); d > 4 {
			t.Fatalf("%dx%d %s: full-size picture differs from the source by %.2f", c.bw, c.bh, c.format, d)
		}
		want := src
		for m := 1; m < vt2.Mips; m++ {
			want = halve(want, vt2.SRGB)
			got, err := vt2.DecodeMip(nb, m)
			if err != nil {
				t.Fatalf("%dx%d: size %d: %v", c.bw, c.bh, m, err)
			}
			if got.W != want.W || got.H != want.H {
				t.Fatalf("%dx%d: size %d is %dx%d, want %dx%d", c.bw, c.bh, m, got.W, got.H, want.W, want.H)
			}
			// below 16 pixels the picture is too small for block compression to follow; only the shape is checked
			if d := meanAbsDiff(got, want); d > 6 && min(want.W/c.bw, want.H/c.bh) >= 8 {
				t.Fatalf("%dx%d: size %d differs by %.2f", c.bw, c.bh, m, d)
			}
		}
		// tiles out, tiles back in
		dir := t.TempDir()
		outs, err := WriteTiles(full, vt2.BlocksW, vt2.BlocksH, KindColour, true, dir, "T_Test_001_C", ".tga", nil)
		if err != nil || len(outs) != c.bw*c.bh {
			t.Fatalf("%dx%d: wrote %d tile files (%v)", c.bw, c.bh, len(outs), err)
		}
		if c.bh == 1 {
			last := filepath.Join(dir, "T_Test_001_C_BaseColor."+itoa(1000+c.bw)+".tga")
			if _, err := os.Stat(last); err != nil {
				t.Fatalf("%dx%d: last tile %s missing", c.bw, c.bh, filepath.Base(last))
			}
		}
		files, _ := listImages(dir)
		sets := FindPainted(files, []string{"T_Test_001_C"}, nil)
		rec := PlanRecipe(KindColour, "T_Test_001_C", vt2, sets)
		if !rec.Ready {
			t.Fatalf("%dx%d: tiles not recognised: %s", c.bw, c.bh, rec.Missing)
		}
		back, _, err := Compose(KindColour, "T_Test_001_C", vt2, nb, sets)
		if err != nil || meanAbsDiff(back, full) != 0 { // colour channels must match exactly
			t.Fatalf("%dx%d: tiles did not come back as the same picture (%v)", c.bw, c.bh, err)
		}
		// one missing tile must be reported, not silently accepted
		os.Remove(outs[len(outs)-1].Path)
		files, _ = listImages(dir)
		if rec := PlanRecipe(KindColour, "T_Test_001_C", vt2, FindPainted(files, nil, nil)); rec.Ready && c.bw*c.bh > 1 {
			t.Fatalf("%dx%d: a missing tile went unnoticed", c.bw, c.bh)
		}
	}
}

func itoa(n int) string {
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

// A stitched picture is cut into width / height pieces whatever that number is.
func TestSplitAutoCount(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []int{1, 2, 5, 7, 10} {
		src := smoothPicture(64*n, 64, n, 1, int64(n))
		p := filepath.Join(dir, "T_Wide"+itoa(n)+"_001_C.png")
		if err := SaveImage(p, src, SaveRGB); err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(dir, "out"+itoa(n))
		files, err := SplitPicture(p, 0, false, out, "", nil)
		if err != nil || len(files) != n {
			t.Fatalf("%d pieces: got %d files (%v)", n, len(files), err)
		}
		for i := 0; i < n; i++ {
			tile, err := LoadImage(filepath.Join(out, "T_Wide"+itoa(n)+"_001_C."+itoa(1001+i)+".png"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(tile.Pix, src.Sub(i*64, 0, 64, 64).Pix) {
				t.Fatalf("%d pieces: tile %d has the wrong pixels", n, i)
			}
		}
	}
	// a width that is not a whole number of squares needs an explicit count
	odd := filepath.Join(dir, "odd.png")
	SaveImage(odd, smoothPicture(100, 30, 1, 1, 1), SaveRGB)
	if _, err := SplitPicture(odd, 0, false, dir, "", nil); err == nil {
		t.Fatal("an odd-sized picture was split without asking for a count")
	}
	if files, err := SplitPicture(odd, 4, false, filepath.Join(dir, "odd4"), "", nil); err != nil || len(files) != 4 {
		t.Fatalf("explicit count failed: %v", err)
	}
}
