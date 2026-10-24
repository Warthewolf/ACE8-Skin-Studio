package main

import (
	"math"
	"testing"
)

const pilotTex = gameExports + "/Character/PI/PI003/Texture/Acce/T_PI003_Helmet01_01"
const portraitTex = gameExports + "/UI/Texture/HUD/SubtitleSpeakerPortrait/T_HUD_SubtitleSpeakerPortrait_00_Tasha00_001"

// psnr compares the given channels of two pictures of the same size.
func psnr(a, b *Img, chans ...int) float64 {
	var se float64
	n := 0
	for i := 0; i < a.W*a.H; i++ {
		for _, c := range chans {
			d := float64(a.Pix[i*4+c]) - float64(b.Pix[i*4+c])
			se += d * d
			n++
		}
	}
	if se == 0 {
		return 99
	}
	return 10 * math.Log10(255*255/(se/float64(n)))
}

func TestTex2DHelmetLayout(t *testing.T) {
	for suffix, want := range map[string]string{"_C": "PF_DXT1", "_MRO": "PF_DXT1", "_N": "PF_BC5"} {
		needFile(t, pilotTex+suffix+".uasset")
		tx, _, _, err := LoadTex2D(pilotTex + suffix)
		if err != nil {
			t.Fatal(suffix, err)
		}
		if tx.Format != want || len(tx.Mips) != 13 || tx.W() != 4096 || tx.Mips[12].W != 1 {
			t.Errorf("%s: %s %d mips %dx%d", suffix, tx.Format, len(tx.Mips), tx.W(), tx.H())
		}
		if tx.SRGB != (suffix == "_C") {
			t.Errorf("%s: colour %v", suffix, tx.SRGB)
		}
		bulk := 0
		for _, m := range tx.Mips {
			if m.InBulk {
				bulk++
			}
		}
		if bulk != 6 {
			t.Errorf("%s: %d mips in the .ubulk", suffix, bulk)
		}
	}
}

// Rebuilding with the texture's own picture gives back nearly the same texture, and only pixel
// bytes change.
func TestTex2DRoundTrip(t *testing.T) {
	for _, c := range []struct {
		suffix string
		chans  []int
		min    float64
	}{{"_C", []int{0, 1, 2}, 38}, {"_N", []int{0, 1}, 40}} {
		needFile(t, pilotTex+c.suffix+".uasset")
		tx, p, ubulk, err := LoadTex2D(pilotTex + c.suffix)
		if err != nil {
			t.Fatal(err)
		}
		orig, err := tx.DecodeMip(p.UExp, ubulk, 0)
		if err != nil {
			t.Fatal(err)
		}
		ue, ub, err := tx.Rebuild(p.UExp, ubulk, orig, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(ue) != len(p.UExp) || len(ub) != len(ubulk) {
			t.Fatalf("%s: sizes changed", c.suffix)
		}
		// outside the pixels, the .uexp is unchanged
		inPix := make([]bool, len(ue))
		for _, m := range tx.Mips {
			if !m.InBulk {
				for i := m.Off; i < m.Off+m.Size; i++ {
					inPix[i] = true
				}
			}
		}
		for i := range ue {
			if !inPix[i] && ue[i] != p.UExp[i] {
				t.Fatalf("%s: byte %d of the .uexp outside the pixels changed", c.suffix, i)
			}
		}
		q, _ := ParsePackage(p.UAsset, ue)
		tx2, err := ParseTex2D(q)
		if err != nil {
			t.Fatal(err)
		}
		again, err := tx2.DecodeMip(ue, ub, 0)
		if err != nil {
			t.Fatal(err)
		}
		if v := psnr(orig, again, c.chans...); v < c.min {
			t.Errorf("%s: %.1f dB after a round trip", c.suffix, v)
		} else {
			t.Logf("%s: %.1f dB", c.suffix, v)
		}
		// the smaller mips follow the picture too (made with a plain 2x2 average, so they differ a
		// little from the game's own)
		for _, mip := range []int{4, 8} {
			small, _ := tx2.DecodeMip(ue, ub, mip)
			origSmall, _ := tx.DecodeMip(p.UExp, ubulk, mip)
			if v := psnr(origSmall, small, c.chans...); v < 25 {
				t.Errorf("%s: mip %d %.1f dB", c.suffix, mip, v)
			} else {
				t.Logf("%s: mip %d %.1f dB", c.suffix, mip, v)
			}
		}
	}
}

func TestTex2DPortrait(t *testing.T) {
	needFile(t, portraitTex+".uasset")
	tx, p, ub, err := LoadTex2D(portraitTex)
	if err != nil {
		t.Fatal(err)
	}
	if tx.Format != "PF_BC7" || len(tx.Mips) != 1 || tx.W() != 428 || tx.H() != 540 || tx.NeedsBulk() || !tx.SRGB {
		t.Fatalf("%+v", tx)
	}
	img, err := tx.DecodeMip(p.UExp, ub, 0)
	if err != nil {
		t.Fatal(err)
	}
	ue, _, err := tx.Rebuild(p.UExp, ub, img, nil)
	if err != nil {
		t.Fatal(err)
	}
	q, _ := ParsePackage(p.UAsset, ue)
	again, _ := DecodeIcon(q)
	if v := psnr(img, again, 0, 1, 2, 3); v < 38 {
		t.Errorf("portrait %.1f dB", v)
	}
}

func TestBC4Codec(t *testing.T) {
	src := NewImg(4, 4)
	vals := []uint8{0, 17, 34, 51, 68, 85, 102, 119, 136, 153, 170, 187, 204, 221, 238, 255}
	for i, v := range vals {
		src.Pix[i*4+1] = v
	}
	var blk [8]byte
	encodeBC4Block(src.Pix, 16, 1, blk[:])
	out := NewImg(4, 4)
	decodeBC4Block(blk[:], out.Pix, 16, 1)
	for i, v := range vals {
		if d := int(out.Pix[i*4+1]) - int(v); d < -20 || d > 20 {
			t.Errorf("pixel %d: %d for %d", i, out.Pix[i*4+1], v)
		}
	}
	// flat blocks come back exact
	for i := range src.Pix {
		src.Pix[i] = 77
	}
	encodeBC4Block(src.Pix, 16, 0, blk[:])
	decodeBC4Block(blk[:], out.Pix, 16, 0)
	if out.Pix[0] != 77 || out.Pix[60] != 77 {
		t.Errorf("flat block: %d", out.Pix[0])
	}
}

// A block of white text fading into a transparent background: one RGBA line (mode 6) cannot follow
// it, so the encoder keeps colour and transparency apart (mode 4 or 5).
func TestBC7TextBlock(t *testing.T) {
	alpha := []int{132, 129, 73, 1, 6, 2, 0, 0, 5, 1, 0, 0, 3, 0, 0, 0}
	src := make([]uint8, 64)
	for i, a := range alpha {
		v := uint8(255)
		if a == 0 {
			v = 0 // transparent pixels come out of a browser canvas black
		}
		src[i*4], src[i*4+1], src[i*4+2], src[i*4+3] = v, v, v, uint8(a)
	}
	var blk [16]byte
	var out [64]uint8
	encodeBC7Block(src, 16, blk[:])
	decodeBC7Block(blk[:], out[:], 16)
	for i, a := range alpha {
		if d := int(out[i*4+3]) - a; d < -8 || d > 8 {
			t.Errorf("pixel %d: transparency %d for %d", i, out[i*4+3], a)
		}
		if a > 50 && out[i*4] < 245 {
			t.Errorf("pixel %d: colour %d", i, out[i*4])
		}
	}
	// blocks that are not see-through stay fully opaque, and never come out worse than mode 6 alone
	for i := 0; i < 16; i++ {
		src[i*4], src[i*4+1], src[i*4+2], src[i*4+3] = uint8(i*16), 90, uint8(255-i*16), 255
	}
	encodeBC7Block(src, 16, blk[:])
	var one [16]byte
	encodeBC7Mode6(src, 16, one[:])
	if bc7Error(src, 16, blk[:]) > bc7Error(src, 16, one[:]) {
		t.Errorf("opaque block came out worse than mode 6 (mode bits %#x)", blk[0])
	}
	decodeBC7Block(blk[:], out[:], 16)
	for i := 0; i < 16; i++ {
		if out[i*4+3] != 255 {
			t.Errorf("pixel %d of an opaque block decodes with transparency %d", i, out[i*4+3])
		}
	}
}
