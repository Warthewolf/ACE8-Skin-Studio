package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// tgaHeader makes an 18-byte TGA header.
func tgaHeader(typ, cmapType byte, cmapFirst, cmapLen int, cmapBits byte, w, h int, bpp, desc byte) []byte {
	d := make([]byte, 18)
	d[1], d[2] = cmapType, typ
	binary.LittleEndian.PutUint16(d[3:], uint16(cmapFirst))
	binary.LittleEndian.PutUint16(d[5:], uint16(cmapLen))
	d[7] = cmapBits
	binary.LittleEndian.PutUint16(d[12:], uint16(w))
	binary.LittleEndian.PutUint16(d[14:], uint16(h))
	d[16], d[17] = bpp, desc
	return d
}

// allocated runs fn and returns how many bytes it set aside.
func allocated(fn func()) uint64 {
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	fn()
	runtime.ReadMemStats(&b)
	return b.TotalAlloc - a.TotalAlloc
}

// Colour-mapped TGAs: the map may start at an entry other than 0, and a pixel outside the map is
// an error, not a crash.
func TestTGAColourMap(t *testing.T) {
	// first entry 16, two entries (blue-green-red), pixels 16 and 17, top row first
	d := append(tgaHeader(1, 1, 16, 2, 24, 2, 1, 8, 0x20), 10, 20, 30, 40, 50, 60, 16, 17)
	im, err := decodeTGA(d)
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte{30, 20, 10, 255, 60, 50, 40, 255}; !bytes.Equal(im.Pix, want) {
		t.Fatalf("got %v, want %v", im.Pix, want)
	}
	for name, d := range map[string][]byte{
		"index past the map":   append(tgaHeader(1, 1, 0, 2, 24, 1, 1, 8, 0), 1, 2, 3, 4, 5, 6, 5),
		"index before the map": append(tgaHeader(1, 1, 16, 2, 24, 1, 1, 8, 0), 1, 2, 3, 4, 5, 6, 3),
		"empty map":            append(tgaHeader(1, 1, 0, 0, 24, 1, 1, 8, 0), 0),
	} {
		if _, err := decodeTGA(d); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

// 16-bit TGAs: 31 is full brightness, and the top bit is alpha when the header says there is one.
func TestTGA16Bit(t *testing.T) {
	px := func(desc byte, vals ...uint16) []byte {
		d := tgaHeader(2, 0, 0, 0, 0, len(vals), 1, 16, desc|0x20)
		for _, v := range vals {
			d = binary.LittleEndian.AppendUint16(d, v)
		}
		im, err := decodeTGA(d)
		if err != nil {
			t.Fatal(err)
		}
		return im.Pix
	}
	if got := px(0, 0x7fff, 0x0421); !bytes.Equal(got, []byte{255, 255, 255, 255, 8, 8, 8, 255}) {
		t.Errorf("white and dark grey: %v", got)
	}
	if got := px(1, 0x7c00, 0xfc00); !bytes.Equal(got, []byte{255, 0, 0, 0, 255, 0, 0, 255}) {
		t.Errorf("with alpha: %v", got)
	}
	if got := px(1, 0x7c00, 0x7c00); got[3] != 255 || got[7] != 255 {
		t.Errorf("alpha bit clear everywhere should be opaque: %v", got)
	}
}

// pngFile makes a PNG of the given size whose picture data is only a few bytes long.
func pngFile(w, h uint32) []byte {
	chunk := func(out []byte, typ string, data []byte) []byte {
		out = binary.BigEndian.AppendUint32(out, uint32(len(data)))
		body := append([]byte(typ), data...)
		out = append(out, body...)
		return binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(body))
	}
	ihdr := binary.BigEndian.AppendUint32(nil, w)
	ihdr = binary.BigEndian.AppendUint32(ihdr, h)
	ihdr = append(ihdr, 8, 6, 0, 0, 0) // 8 bits, RGBA
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	zw.Write(make([]byte, 16))
	zw.Close()
	out := []byte("\x89PNG\r\n\x1a\n")
	out = chunk(out, "IHDR", ihdr)
	out = chunk(out, "IDAT", z.Bytes())
	return chunk(out, "IEND", nil)
}

// A tiny file whose header claims a huge picture is refused before memory is set aside for it.
func TestHugeHeaderSmallFile(t *testing.T) {
	const limit = 16 << 20
	cases := map[string][]byte{
		"tga":        append(tgaHeader(2, 0, 0, 0, 0, 8000, 8000, 32, 8), 1, 2, 3, 4),
		"tga rle":    append(tgaHeader(10, 0, 0, 0, 0, 8000, 8000, 32, 8), 0xff, 1, 2, 3, 4),
		"tga 65535":  append(tgaHeader(2, 0, 0, 0, 0, 65535, 65535, 32, 8), 1, 2, 3, 4),
		"tga rle 2k": append(tgaHeader(10, 0, 0, 0, 0, 2000, 2000, 24, 0), bytes.Repeat([]byte{0xff, 1, 2, 3}, 20)...),
	}
	for name, d := range cases {
		var err error
		if n := allocated(func() { _, err = decodeTGA(d) }); n > limit {
			t.Errorf("%s: %d MB set aside", name, n>>20)
		}
		if err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	dir := t.TempDir()
	for name, b := range map[string][]byte{"cut short": pngFile(8000, 8000), "too large": pngFile(60000, 60000)} {
		p := filepath.Join(dir, strings.ReplaceAll(name, " ", "_")+".png")
		os.WriteFile(p, b, 0o644)
		for _, load := range []func(string) (*Img, error){LoadImage, LoadPicture} {
			var err error
			if n := allocated(func() { _, err = load(p) }); n > limit {
				t.Errorf("png %s: %d MB set aside", name, n>>20)
			}
			if err == nil || !strings.Contains(err.Error(), "8000 x 8000") && !strings.Contains(err.Error(), "60000 x 60000") {
				t.Errorf("png %s: %v", name, err)
			}
		}
	}
	// a real picture of the same kind still loads, even one that compresses very well
	p := filepath.Join(dir, "flat.png")
	if err := SaveImage(p, NewImg(3000, 2000), SaveRGBA); err != nil {
		t.Fatal(err)
	}
	if im, err := LoadImage(p); err != nil || im.W != 3000 {
		t.Fatalf("a flat picture: %v", err)
	}
	if im, err := LoadPicture(p); err != nil || im.W != 3000 {
		t.Fatalf("a flat picture: %v", err)
	}
	tp := filepath.Join(dir, "flat.tga")
	if err := SaveImage(tp, NewImg(3000, 2000), SaveRGB); err != nil {
		t.Fatal(err)
	}
	if im, err := LoadImage(tp); err != nil || im.W != 3000 {
		t.Fatalf("a flat TGA: %v", err)
	}
}
