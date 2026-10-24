package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

const texDir = "/mnt/user-data/uploads/PP0021_su57/Textures/"

// Damaged or truncated files must be refused with an error, never crash the program.
func TestDamagedInputsDoNotPanic(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	uexp, err := os.ReadFile(texDir + "001/T_PP0021_su57_001_C.uexp")
	if err != nil {
		t.Skip("sample files not present")
	}
	for i := 0; i < 20000; i++ {
		d := append([]byte(nil), uexp...)
		switch i % 3 {
		case 0:
			d = d[:rng.Intn(len(d))]
		case 1:
			for k := 0; k < 1+rng.Intn(6); k++ {
				d[rng.Intn(len(d))] = byte(rng.Intn(256))
			}
		default:
			p := rng.Intn(len(d) - 4)
			copy(d[p:], []byte{0xff, 0xff, 0xff, 0x7f})
		}
		if vt, err := ParseVTex(d); err == nil {
			vt.ResolveChunks(d, make([]byte, 4096))
		}
	}
	// TGA
	im := NewImg(37, 21)
	for i := range im.Pix {
		im.Pix[i] = byte(rng.Intn(4) * 60)
	}
	for _, mode := range []SaveMode{SaveRGB, SaveRGBA, SaveGrey} {
		var buf bytes.Buffer
		if err := encodeTGA(&buf, im, mode); err != nil {
			t.Fatal(err)
		}
		back, err := decodeTGA(buf.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < im.W*im.H; i++ {
			want := [4]byte{im.Pix[i*4], im.Pix[i*4+1], im.Pix[i*4+2], im.Pix[i*4+3]}
			if mode == SaveGrey {
				want = [4]byte{want[0], want[0], want[0], 255}
			} else if mode == SaveRGB {
				want[3] = 255
			}
			got := [4]byte{back.Pix[i*4], back.Pix[i*4+1], back.Pix[i*4+2], back.Pix[i*4+3]}
			if got != want {
				t.Fatalf("tga mode %d pixel %d: got %v want %v", mode, i, got, want)
			}
		}
		for k := 0; k < 3000; k++ {
			d := append([]byte(nil), buf.Bytes()...)
			if k%2 == 0 {
				d = d[:rng.Intn(len(d))]
			} else {
				d[rng.Intn(len(d))] = byte(rng.Intn(256))
			}
			decodeTGA(d)
		}
	}
	// GLB
	if g, err := os.ReadFile("/mnt/user-data/uploads/Content/Vehicles/Aircraft/PP/PP0021_su57/Model/SM_PP0021_su57_RefuelOff.glb"); err == nil {
		dir := t.TempDir()
		for k := 0; k < 400; k++ {
			d := append([]byte(nil), g...)
			if k%2 == 0 {
				d = d[:rng.Intn(len(d))]
			} else {
				for q := 0; q < 4; q++ {
					d[20+rng.Intn(1500)] = byte(rng.Intn(256))
				}
			}
			p := filepath.Join(dir, "x.glb")
			os.WriteFile(p, d, 0o644)
			InspectModels([]string{p})
		}
	}
}

// Every PNG flavour must load to the same pixels.
func TestPNGFlavours(t *testing.T) {
	dir := t.TempDir()
	w, h := 9, 5
	mk := func(name string, im image.Image) *Img {
		var buf bytes.Buffer
		if err := png.Encode(&buf, im); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, name+".png")
		os.WriteFile(p, buf.Bytes(), 0o644)
		got, err := LoadImage(p)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	n8 := image.NewNRGBA(image.Rect(0, 0, w, h))
	n16 := image.NewNRGBA64(image.Rect(0, 0, w, h))
	g8 := image.NewGray(image.Rect(0, 0, w, h))
	g16 := image.NewGray16(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b := uint8(x*25), uint8(y*50), uint8(x*y*5)
			n8.SetNRGBA(x, y, color.NRGBA{r, g, b, 255})
			n16.SetNRGBA64(x, y, color.NRGBA64{uint16(r) * 257, uint16(g) * 257, uint16(b) * 257, 65535})
			g8.SetGray(x, y, color.Gray{r})
			g16.SetGray16(x, y, color.Gray16{uint16(r) * 257})
		}
	}
	a, b := mk("n8", n8), mk("n16", n16)
	if !bytes.Equal(a.Pix, b.Pix) {
		t.Fatal("8-bit and 16-bit colour PNG differ")
	}
	c, d := mk("g8", g8), mk("g16", g16)
	if !bytes.Equal(c.Pix, d.Pix) {
		t.Fatal("8-bit and 16-bit grey PNG differ")
	}
	for i := 0; i < w*h; i++ {
		if c.Pix[i*4] != a.Pix[i*4] || c.Pix[i*4+3] != 255 {
			t.Fatal("grey PNG pixel wrong")
		}
	}
	// save + reload each mode
	for _, ext := range []string{".png", ".tga"} {
		p := filepath.Join(dir, "rt"+ext)
		if err := SaveImage(p, a, SaveRGB); err != nil {
			t.Fatal(err)
		}
		back, err := LoadImage(p)
		if err != nil || !bytes.Equal(back.Pix, a.Pix) {
			t.Fatalf("round trip %s failed: %v", ext, err)
		}
		if w2, h2, err := ImageSize(p); err != nil || w2 != w || h2 != h {
			t.Fatalf("size of %s wrong", ext)
		}
	}
}

// Block codecs: decoding what we encode must stay close to the source, and flat blocks must be exact enough.
func TestBlockCodecs(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	var worst1, worst7 int
	for k := 0; k < 4000; k++ {
		src := make([]uint8, 64)
		base := [4]int{30 + rng.Intn(196), 30 + rng.Intn(196), 30 + rng.Intn(196), 255}
		span := rng.Intn(40)
		for i := 0; i < 16; i++ {
			tt := rng.Intn(span + 1)
			for c := 0; c < 3; c++ {
				v := base[c] + tt - span/2
				if v < 0 {
					v = 0
				}
				if v > 255 {
					v = 255
				}
				src[i*4+c] = uint8(v)
			}
			src[i*4+3] = 255
		}
		var b1 [8]byte
		var b7 [16]byte
		out := make([]uint8, 64)
		encodeBC1Block(src, 16, b1[:])
		decodeBC1Block(b1[:], out, 16)
		for i := 0; i < 64; i++ {
			if d := int(src[i]) - int(out[i]); d > worst1 || -d > worst1 {
				worst1 = max(d, -d)
			}
		}
		encodeBC7Block(src, 16, b7[:])
		decodeBC7Block(b7[:], out, 16)
		for i := 0; i < 64; i++ {
			if d := int(src[i]) - int(out[i]); d > worst7 || -d > worst7 {
				worst7 = max(d, -d)
			}
		}
	}
	t.Logf("worst channel error on grey-ramp blocks: BC1 %d, BC7 %d", worst1, worst7)
	if worst1 > 12 || worst7 > 4 {
		t.Fatalf("codec error too large: BC1 %d BC7 %d", worst1, worst7)
	}
}

func TestNamesAndKinds(t *testing.T) {
	if GuessKind("T_PP0021_su57_001_C") != KindColour || GuessKind("T_PP0021_su57_007q_C1") != KindColour || GuessKind("T_PP0021_su57_001_MRX") != KindFinish || GuessKind("T_PP0021_su57_001_NO") != KindNormal || GuessKind("T_PP0021_su57_DirtMask") != KindOther {
		t.Fatal("kind guess wrong")
	}
	for name, want := range map[string]int{"MI_PP0021_su57_001_Decal_Base_color_1003.tga": 1003, "su57_101_BaseColor.1005.png": 1005, "T_PP0021_su57_001_C.png": 0, "x_1001_final_1002.png": 1002, "PP1021_thing.png": 0, "a.2001.png": 0} {
		if _, got := splitTile(name); got != want {
			t.Fatalf("%s: tile %d, want %d", name, got, want)
		}
	}
	files := []string{}
	for _, ch := range []string{"Base_color", "Height", "Metallic", "Mixed_AO", "Normal", "Normal_DirectX", "Roughness"} {
		for u := 1001; u <= 1005; u++ {
			files = append(files, "D:/x/MI_PP0021_su57_001_Decal_"+ch+"_"+string(rune('0'+u/1000))+string(rune('0'+u/100%10))+string(rune('0'+u/10%10))+string(rune('0'+u%10))+".tga")
		}
	}
	files = append(files, "D:/x/T_PP0021_su57_DirtMask.1001.png", "D:/x/notes.txt")
	sets := FindPainted(files, []string{"T_PP0021_su57_001_C", "T_PP0021_su57_DirtMask"}, nil)
	for _, ch := range []string{ChBase, ChMetallic, ChRoughness, ChNormal, ChAO} {
		if sets[ch] == nil || len(sets[ch].Tiles) != 5 {
			t.Fatalf("channel %s not found completely: %+v", ch, sets[ch])
		}
	}
	if filepath.Base(sets[ChNormal].Tiles[1001]) != "MI_PP0021_su57_001_Decal_Normal_DirectX_1001.tga" {
		t.Fatalf("wrong normal chosen: %s", sets[ChNormal].Tiles[1001])
	}
	if sets["tex:T_PP0021_su57_DirtMask"] == nil || len(sets["tex:T_PP0021_su57_DirtMask"].Tiles) != 1 {
		t.Fatal("ready-made picture not matched by texture name")
	}
	if safeName("Su57 Navy!") != "Su57_Navy" || safeName("../..") != "" {
		t.Fatalf("safeName wrong: %q %q", safeName("Su57 Navy!"), safeName("../.."))
	}
}

func TestLoosePartNames(t *testing.T) {
	yes := []string{"SM_PP0021_su57_DPTK.glb", "SM_PP0021_su57_DPP.glb", "SM_PP0021_su57_DPP1.glb", "SM_PP0021_su57_DPP2.glb",
		"SM_PP0021_su57_DPP3.glb", "SM_PP0021_su57_DPTK2.glb", "sm_pp0005_f15c_dpp12.glb", "SM_PP0005_f15c_DPP1_L.glb", "SM_Pylon_03.glb"}
	no := []string{"SM_PP0021_su57.glb", "SM_PP0021_su57_GearOff.glb", "SM_PP0021_su57_RefuelOff.glb", "SM_PP0021_su57_Cockpit.glb"}
	for _, n := range yes {
		if !reLoosePart.MatchString(n) {
			t.Errorf("%s should count as a pylon", n)
		}
	}
	for _, n := range no {
		if reLoosePart.MatchString(n) {
			t.Errorf("%s should not count as a pylon", n)
		}
	}
}
