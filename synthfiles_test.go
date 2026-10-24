package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// Writes a made-up seven-tile skin to disk when AC8_SYNTH_OUT is set, for trying the window by hand.
func TestWriteSynth(t *testing.T) {
	out := os.Getenv("AC8_SYNTH_OUT")
	if out == "" {
		t.Skip("AC8_SYNTH_OUT not set")
	}
	dir := filepath.Join(out, "Live", "Content", "Vehicles", "Aircraft", "PP", "PP0099_seven", "Textures", "001")
	os.MkdirAll(dir, 0o755)
	for _, x := range []struct {
		name, format string
		bb           int
	}{{"T_PP0099_seven_001_C", "PF_DXT1", 8}, {"T_PP0099_seven_001_MRX", "PF_DXT1", 8}} {
		uexp, ubulk := synthVT(7, 1, 1024, 128, 4, x.format, x.bb)
		vt, err := ParseVTex(uexp)
		if err != nil {
			t.Fatal(err)
		}
		if err := vt.ResolveChunks(uexp, ubulk); err != nil {
			t.Fatal(err)
		}
		src := smoothPicture(vt.Width, vt.Height, 7, 1, 5)
		for i := 0; i < len(src.Pix); i += 4 { // give it some colour so the preview is recognisable
			src.Pix[i] = src.Pix[i] / 2
			src.Pix[i+2] = 255 - src.Pix[i+2]/2
		}
		ne, nb, err := vt.Rebuild(uexp, ubulk, src, nil)
		if err != nil {
			t.Fatal(err)
		}
		var ua bytes.Buffer
		ua.Write(make([]byte, 40))
		pkg := "/Game/Vehicles/Aircraft/PP/PP0099_seven/Textures/001/" + x.name
		binary.Write(&ua, binary.LittleEndian, uint32(len(pkg)+1))
		ua.WriteString(pkg)
		ua.WriteByte(0)
		os.WriteFile(filepath.Join(dir, x.name+".uasset"), ua.Bytes(), 0o644)
		os.WriteFile(filepath.Join(dir, x.name+".uexp"), ne, 0o644)
		os.WriteFile(filepath.Join(dir, x.name+".ubulk"), nb, 0o644)
	}
	// and a seven-wide stitched picture for the Split screen
	SaveImage(filepath.Join(out, "T_PP0099_seven_001_C.tga"), smoothPicture(7*512, 512, 7, 1, 9), SaveRGB)
}

// Writes a made-up F-14-style skin (eight 4096-pixel tiles, of which 1007 and 1008 are stored
// at 1024 pixels) to disk when AC8_SYNTH_F14 is set, for trying the real size by hand.
func TestWriteSynthF14(t *testing.T) {
	out := os.Getenv("AC8_SYNTH_F14")
	if out == "" {
		t.Skip("AC8_SYNTH_F14 not set")
	}
	dir := filepath.Join(out, "Live", "Content", "Vehicles", "Aircraft", "PP", "PP0098_mixed", "Textures", "101")
	os.MkdirAll(dir, 0o755)
	for _, x := range []struct {
		name, format string
		bb           int
	}{{"T_PP0098_mixed_101_C", "PF_DXT1", 8}, {"T_PP0098_mixed_101_MRX", "PF_DXT1", 8}} {
		uexp, ubulk := synthVTMixed(8, 1, 4096, 128, 4, x.format, x.bb, map[int]int{6: 2, 7: 2})
		vt, err := ParseVTex(uexp)
		if err != nil {
			t.Fatal(err)
		}
		if err := vt.ResolveChunks(uexp, ubulk); err != nil {
			t.Fatal(err)
		}
		src := resize(smoothPicture(vt.Width/8, vt.Height/8, 8, 1, 5), vt.Width, vt.Height)
		for i := 0; i < len(src.Pix); i += 4 {
			src.Pix[i] = src.Pix[i] / 2
			src.Pix[i+2] = 255 - src.Pix[i+2]/2
		}
		ne, nb, err := vt.Rebuild(uexp, ubulk, src, nil)
		if err != nil {
			t.Fatal(err)
		}
		var ua bytes.Buffer
		ua.Write(make([]byte, 40))
		pkg := "/Game/Vehicles/Aircraft/PP/PP0098_mixed/Textures/101/" + x.name
		binary.Write(&ua, binary.LittleEndian, uint32(len(pkg)+1))
		ua.WriteString(pkg)
		ua.WriteByte(0)
		os.WriteFile(filepath.Join(dir, x.name+".uasset"), ua.Bytes(), 0o644)
		os.WriteFile(filepath.Join(dir, x.name+".uexp"), ne, 0o644)
		os.WriteFile(filepath.Join(dir, x.name+".ubulk"), nb, 0o644)
	}
}
