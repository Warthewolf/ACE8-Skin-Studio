package main

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

// A JPEG photo (kept with a radio portrait by versions before 2.5.0) still gets a small picture.
func TestThumbOfJPEG(t *testing.T) {
	p := filepath.Join(t.TempDir(), "photo.jpg")
	im := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			im.Set(x, y, color.RGBA{uint8(x * 4), 80, 160, 255})
		}
	}
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(f, im, nil); err != nil {
		t.Fatal(err)
	}
	f.Close()
	b, err := makeThumb(p, 32, false)
	if err != nil || len(b) == 0 {
		t.Fatalf("thumbnail of a JPEG: %v", err)
	}
}
