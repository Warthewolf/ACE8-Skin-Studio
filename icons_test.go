package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIconRoundTrip(t *testing.T) {
	base := gameExports + "/UI/Texture/Icon/Skin/T_UI_Icon_Skin_su57_001"
	needFile(t, base+".uasset")
	p, err := LoadPackage(base)
	if err != nil {
		t.Fatal(err)
	}
	im, err := DecodeIcon(p)
	if err != nil {
		t.Fatal(err)
	}
	if im.W != 180 || im.H != 180 {
		t.Fatalf("icon %dx%d", im.W, im.H)
	}
	navy := RecolourIcon(im, [3]uint8{24, 34, 58})
	ue, err := EncodeIcon(p, navy)
	if err != nil {
		t.Fatal(err)
	}
	if len(ue) != len(p.UExp) {
		t.Fatal("size changed")
	}
	q, _ := ParsePackage(p.UAsset, ue)
	back, err := DecodeIcon(q)
	if err != nil {
		t.Fatal(err)
	}
	var worst int
	for i := range navy.Pix {
		d := int(navy.Pix[i]) - int(back.Pix[i])
		if d < 0 {
			d = -d
		}
		worst = max(worst, d)
	}
	if worst > 24 {
		t.Errorf("encoded icon differs by up to %d", worst)
	}
	// the corner keeps the background, the middle takes the new colour
	c := back.Pix[(90*180+90)*4:]
	if c[0] > 60 || c[2] < 30 {
		t.Errorf("middle pixel %v is not navy", c[:3])
	}
	out := os.Getenv("ICON_OUT")
	if out != "" {
		SaveImage(filepath.Join(out, "icon_orig.png"), im, SaveRGB)
		SaveImage(filepath.Join(out, "icon_navy.png"), back, SaveRGB)
	}
}

func TestMainColour(t *testing.T) {
	m := NewImg(100, 100)
	for i := 0; i < len(m.Pix); i += 4 {
		m.Pix[i], m.Pix[i+1], m.Pix[i+2], m.Pix[i+3] = 30, 40, 80, 255
	}
	for i := 0; i < 2000*4; i += 4 { // a fifth of it white
		m.Pix[i], m.Pix[i+1], m.Pix[i+2] = 250, 250, 250
	}
	c := MainColour(m)
	if c != [3]uint8{30, 40, 80} {
		t.Errorf("main colour %v", c)
	}
}
