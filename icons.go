package main

// Skin icons: the small picture next to each skin in the hangar. Each is a 180 x 180 BC7 texture
// whose only mip sits inside the .uexp (no .ubulk); its place is found through the package's
// data-resource table. A new icon is a copy of an existing icon package with new pixels.

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type iconLayoutInfo struct {
	W, H        int
	PixAt, Size int // in the .uexp
}

func iconLayoutOf(p *Package) (iconLayoutInfo, error) {
	var info iconLayoutInfo
	ua, ue := p.UAsset, p.UExp
	dro := int(le32(ua, p.dataResAt))
	if dro <= 0 || dro+8 > len(ua) {
		return info, errors.New("this texture keeps no pixels of its own in the package")
	}
	if cnt := int(le32(ua, dro+4)); cnt != 1 {
		return info, fmt.Errorf("%d pixel blocks, expected 1 (not a skin icon)", cnt)
	}
	e := dro + 8
	info.PixAt = int(le64(ua, e+4))
	info.Size = int(le64(ua, e+20))
	end := info.PixAt + info.Size
	if info.PixAt <= 0 || end+8 > len(ue) {
		return info, errors.New("the icon's pixels are not where expected")
	}
	info.W = int(le32(ue, end))
	info.H = int(le32(ue, end+4))
	if info.W <= 0 || info.H <= 0 || info.W > 1024 || info.H > 1024 || (info.W+3)/4*((info.H+3)/4)*16 != info.Size {
		return info, fmt.Errorf("the pixel block of %d bytes does not fit a %d x %d BC7 icon", info.Size, info.W, info.H)
	}
	if fm, _, _ := findPixelFormat(ue, 0); fm != "PF_BC7" {
		return info, fmt.Errorf("icon format %s is not handled", fm)
	}
	return info, nil
}

// DecodeIcon gives an icon package's picture.
func DecodeIcon(p *Package) (*Img, error) {
	info, err := iconLayoutOf(p)
	if err != nil {
		return nil, err
	}
	bw, bh := (info.W+3)/4, (info.H+3)/4
	full := NewImg(bw*4, bh*4)
	px := p.UExp[info.PixAt : info.PixAt+info.Size]
	for by := 0; by < bh; by++ {
		for bx := 0; bx < bw; bx++ {
			blk := px[(by*bw+bx)*16:]
			decodeBC7Block(blk[:16], full.Pix[(by*4*full.W+bx*4)*4:], full.W*4)
		}
	}
	if full.W == info.W && full.H == info.H {
		return full, nil
	}
	return full.Sub(0, 0, info.W, info.H), nil
}

// EncodeIcon returns the icon package's .uexp with pic (resized to the icon's size) as its pixels.
func EncodeIcon(p *Package, pic *Img) ([]byte, error) {
	info, err := iconLayoutOf(p)
	if err != nil {
		return nil, err
	}
	bw, bh := (info.W+3)/4, (info.H+3)/4
	src := NewImg(bw*4, bh*4)
	src.Paste(resize(pic, info.W, info.H), 0, 0)
	out := append([]byte(nil), p.UExp...)
	px := out[info.PixAt : info.PixAt+info.Size]
	parallelFor(bh, func(by int) {
		for bx := 0; bx < bw; bx++ {
			encodeBC7Block(src.Pix[(by*4*src.W+bx*4)*4:], src.W*4, px[(by*bw+bx)*16:(by*bw+bx)*16+16])
		}
	})
	return out, nil
}

// LoadPicture reads a PNG, JPEG or TGA picture.
func LoadPicture(path string) (*Img, error) {
	if strings.EqualFold(filepath.Ext(path), ".tga") {
		return LoadImage(path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	im, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("%s is not a PNG, JPEG or TGA picture", filepath.Base(path))
	}
	return fromGoImage(im), nil
}

// squareCrop takes the middle square of a picture.
func squareCrop(m *Img) *Img {
	if m.W == m.H {
		return m
	}
	s := min(m.W, m.H)
	return m.Sub((m.W-s)/2, (m.H-s)/2, s, s)
}

// IconFromPicture makes an icon picture from any picture: the middle square, scaled.
func IconFromPicture(m *Img, size int) *Img {
	return flattenAlpha(resize(squareCrop(m), size, size), [3]uint8{40, 44, 52})
}

// flattenAlpha puts a picture with see-through parts on a plain background (icons are opaque).
func flattenAlpha(m *Img, bg [3]uint8) *Img {
	out := NewImg(m.W, m.H)
	for i := 0; i < len(m.Pix); i += 4 {
		a := int(m.Pix[i+3])
		for c := 0; c < 3; c++ {
			out.Pix[i+c] = uint8((int(m.Pix[i+c])*a + int(bg[c])*(255-a) + 127) / 255)
		}
		out.Pix[i+3] = 255
	}
	return out
}

func colourDist(a, b [3]float64) float64 {
	return math.Sqrt((a[0]-b[0])*(a[0]-b[0]) + (a[1]-b[1])*(a[1]-b[1]) + (a[2]-b[2])*(a[2]-b[2]))
}

// RecolourIcon paints the aircraft shape of a game icon in a new colour and keeps the background:
// game icons are a plain background with the aircraft's outline filled in the skin's main colour.
func RecolourIcon(icon *Img, to [3]uint8) *Img {
	px := func(x, y int) [3]float64 {
		o := (y*icon.W + x) * 4
		return [3]float64{float64(icon.Pix[o]), float64(icon.Pix[o+1]), float64(icon.Pix[o+2])}
	}
	// background: the corners
	var bg [3]float64
	corners := [][2]int{{1, 1}, {icon.W - 2, 1}, {1, icon.H - 2}, {icon.W - 2, icon.H - 2}}
	for _, c := range corners {
		p := px(c[0], c[1])
		for k := 0; k < 3; k++ {
			bg[k] += p[k] / 4
		}
	}
	// the shape's colour: the most common colour far from the background
	counts := map[[3]int]int{}
	for y := 0; y < icon.H; y++ {
		for x := 0; x < icon.W; x++ {
			p := px(x, y)
			if colourDist(p, bg) > 40 {
				counts[[3]int{int(p[0]) / 8, int(p[1]) / 8, int(p[2]) / 8}]++
			}
		}
	}
	if len(counts) == 0 {
		return icon
	}
	var best [3]int
	bn := -1
	for k, n := range counts {
		if n > bn || n == bn && fmt.Sprint(k) < fmt.Sprint(best) {
			best, bn = k, n
		}
	}
	var fg [3]float64
	m := 0
	for y := 0; y < icon.H; y++ {
		for x := 0; x < icon.W; x++ {
			p := px(x, y)
			if [3]int{int(p[0]) / 8, int(p[1]) / 8, int(p[2]) / 8} == best {
				for k := 0; k < 3; k++ {
					fg[k] += p[k]
				}
				m++
			}
		}
	}
	for k := 0; k < 3; k++ {
		fg[k] /= float64(m)
	}
	span := colourDist(fg, bg)
	out := NewImg(icon.W, icon.H)
	copy(out.Pix, icon.Pix)
	for y := 0; y < icon.H; y++ {
		for x := 0; x < icon.W; x++ {
			p := px(x, y)
			// how far towards the shape colour this pixel is (edges are in between)
			a := math.Max(0, math.Min(1, colourDist(p, bg)/span))
			if colourDist(p, fg) > 60 && a > 0.5 {
				continue // a detail drawn on top (a marking): keep it
			}
			o := (y*icon.W + x) * 4
			for k := 0; k < 3; k++ {
				out.Pix[o+k] = uint8(math.Round(bg[k]*(1-a) + float64(to[k])*a))
			}
		}
	}
	return out
}

// MainColour finds the colour most of a paint is in: the biggest group of similar colours.
func MainColour(m *Img) [3]uint8 {
	step := max(1, int(math.Sqrt(float64(m.W*m.H)/250000)))
	counts := map[[3]int]int{}
	sums := map[[3]int][3]int{}
	for y := 0; y < m.H; y += step {
		for x := 0; x < m.W; x += step {
			o := (y*m.W + x) * 4
			if m.Pix[o+3] < 128 {
				continue
			}
			r, g, b := int(m.Pix[o]), int(m.Pix[o+1]), int(m.Pix[o+2])
			k := [3]int{r / 24, g / 24, b / 24}
			counts[k]++
			s := sums[k]
			sums[k] = [3]int{s[0] + r, s[1] + g, s[2] + b}
		}
	}
	type kv struct {
		k [3]int
		n int
	}
	var all []kv
	for k, n := range counts {
		all = append(all, kv{k, n})
	}
	if len(all) == 0 {
		return [3]uint8{128, 128, 128}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].n != all[j].n {
			return all[i].n > all[j].n
		}
		return fmt.Sprint(all[i].k) < fmt.Sprint(all[j].k)
	})
	s := sums[all[0].k]
	n := all[0].n
	return [3]uint8{uint8(s[0] / n), uint8(s[1] / n), uint8(s[2] / n)}
}
