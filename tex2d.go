package main

// Ordinary (not streamed) textures: pilot helmets, subtitle portraits and the like.
//
// Such a texture keeps a chain of mips, each half the size of the one before. The package's
// data-resource table says where each mip's pixels are: the big ones in the .ubulk, the small ones
// inside the .uexp. A rebuilt texture keeps its size and pixel format, so every mip keeps its byte
// count and the new pixels are written where the old ones were; nothing else in the files moves.

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
)

// Tex2DMip is one size of an ordinary texture.
type Tex2DMip struct {
	W, H   int
	InBulk bool // pixels in the .ubulk (else inside the .uexp)
	Off    int  // offset of the pixels in that file
	Size   int
	Res    int // which data resource holds it
	recAt  int // where its record starts in the .uexp
}

// Tex2D describes an ordinary texture package.
type Tex2D struct {
	Name   string // package path
	Format string // PF_DXT1, PF_BC5, ...
	Mips   []Tex2DMip
	SRGB   bool // colour (averaged in linear light when making smaller mips)

	// where things are in the .uexp, for laying the texture out anew (vtformat.go)
	pfAt, pfAfter, listEnd int
}

const (
	bulkInSeparateFile = 0x100 // BULKDATA_PayloadInSeperateFile: the mip is in the .ubulk
	bulkOptional       = 0x800 // BULKDATA_OptionalPayload: in a .uptnl file this program does not handle
)

// texBlock gives the bytes per 4x4 block (block formats) or per pixel (bpp > 0).
func texBlock(format string) (block, bpp int, ok bool) {
	switch format {
	case "PF_DXT1", "PF_BC4":
		return 8, 0, true
	case "PF_DXT5", "PF_BC5", "PF_BC7":
		return 16, 0, true
	case "PF_B8G8R8A8":
		return 0, 4, true
	case "PF_G8":
		return 0, 1, true
	}
	return 0, 0, false
}

func texBytes(format string, w, h int) int {
	block, bpp, _ := texBlock(format)
	if bpp > 0 {
		return w * h * bpp
	}
	return ((w + 3) / 4) * ((h + 3) / 4) * block
}

// texIsColour guesses whether a texture holds colour (sRGB) from its name and format: the game's
// colour maps end in _C, _D, _BC or _BaseColor; data such as _MRO, _N or masks is stored linear.
func texIsColour(name, format string) bool {
	if format == "PF_BC5" || format == "PF_BC4" || format == "PF_G8" {
		return false
	}
	n := strings.ToUpper(name[strings.LastIndexByte(name, '/')+1:])
	for _, s := range []string{"_C", "_D", "_BC", "_BASECOLOR", "_COLOR", "_ALBEDO"} {
		if strings.HasSuffix(n, s) || strings.Contains(n, s+"_") {
			return true
		}
	}
	// UI pictures (portraits, icons) and emblems are colour
	return strings.HasPrefix(n, "T_HUD_") || strings.HasPrefix(n, "T_UI_") || strings.HasPrefix(n, "T_EMBLEM_")
}

// ParseTex2D reads the mip chain of an ordinary texture package.
func ParseTex2D(p *Package) (*Tex2D, error) {
	ua, ue := p.UAsset, p.UExp
	name, at, after := findPixelFormat(ue, 0)
	if name == "" {
		return nil, errors.New("no pixel format found; this does not look like a texture")
	}
	if _, _, ok := texBlock(name); !ok {
		return nil, fmt.Errorf("pixel format %s is not supported", name)
	}
	if p.dataResAt <= 0 {
		return nil, errors.New("this texture keeps no pixels of its own in the package")
	}
	dro := int(le32(ua, p.dataResAt))
	if dro <= 0 || dro+8 > len(ua) {
		return nil, errors.New("this texture keeps no pixels of its own in the package")
	}
	nres := int(le32(ua, dro+4))
	if nres < 1 || nres > 32 || dro+8+nres*44 > len(ua) {
		return nil, fmt.Errorf("the package lists %d pixel blocks", nres)
	}
	type res struct {
		off, size int
		flags     uint32
	}
	rs := make([]res, nres)
	for i := range rs {
		e := dro + 8 + i*44
		rs[i] = res{int(le64(ua, e+4)), int(le64(ua, e+20)), uint32(le32(ua, e+40))}
	}
	r := &rdr{d: ue, p: after}
	first := int(r.u32())
	n := int(r.u32())
	if r.err != nil {
		return nil, r.err
	}
	if n == 0 {
		return nil, errNotOrdinary
	}
	if first != 0 || n < 1 || n > 16 {
		return nil, fmt.Errorf("unexpected mip chain (%d mips from %d)", n, first)
	}
	t := &Tex2D{Name: p.Name, Format: name, pfAt: at, pfAfter: after}
	for i := 0; i < n; i++ {
		recAt := r.p
		idx := int(int32(r.u32()))
		if r.err != nil || idx < 0 || idx >= nres {
			return nil, fmt.Errorf("mip %d points at pixel block %d of %d", i, idx, nres)
		}
		rr := rs[idx]
		if rr.flags&bulkOptional != 0 {
			return nil, errors.New("this texture keeps some of its pixels in an optional file (.uptnl), which this program does not handle")
		}
		m := Tex2DMip{InBulk: rr.flags&bulkInSeparateFile != 0, Off: rr.off, Size: rr.size, Res: idx, recAt: recAt}
		if !m.InBulk {
			if rr.off != r.p {
				return nil, fmt.Errorf("mip %d's pixels are not where the package says", i)
			}
			r.p += rr.size // the pixels sit right here
		}
		m.W, m.H = int(r.u32()), int(r.u32())
		r.u32() // depth
		if r.err != nil {
			return nil, r.err
		}
		if m.W < 1 || m.H < 1 || texBytes(name, m.W, m.H) != m.Size {
			return nil, fmt.Errorf("mip %d (%d x %d) does not fit its %d bytes of %s", i, m.W, m.H, m.Size, name)
		}
		if i > 0 && (m.W != max(t.Mips[i-1].W/2, 1) || m.H != max(t.Mips[i-1].H/2, 1)) {
			return nil, fmt.Errorf("mip %d is %d x %d, not half of the one before", i, m.W, m.H)
		}
		t.Mips = append(t.Mips, m)
	}
	t.listEnd = r.p
	t.SRGB = texIsColour(p.Name, name)
	return t, nil
}

var errNotOrdinary = errors.New("this texture is streamed (UDIM); use the aircraft skin screens for it")

func (t *Tex2D) W() int { return t.Mips[0].W }
func (t *Tex2D) H() int { return t.Mips[0].H }

// NeedsBulk tells whether some pixels are in the .ubulk.
func (t *Tex2D) NeedsBulk() bool {
	for _, m := range t.Mips {
		if m.InBulk {
			return true
		}
	}
	return false
}

func (t *Tex2D) mipBytes(m Tex2DMip, uexp, ubulk []byte) ([]byte, error) {
	src := uexp
	if m.InBulk {
		src = ubulk
	}
	if m.Off < 0 || m.Off+m.Size > len(src) {
		where := ".uexp"
		if m.InBulk {
			where = ".ubulk"
		}
		return nil, fmt.Errorf("the %s is shorter than the texture needs (%d of %d bytes)", where, len(src), m.Off+m.Size)
	}
	return src[m.Off : m.Off+m.Size], nil
}

// DecodeMip gives one mip as RGBA. Two-channel normal maps (BC5) get their blue rebuilt; one-channel
// textures come out grey.
func (t *Tex2D) DecodeMip(uexp, ubulk []byte, mip int) (*Img, error) {
	m := t.Mips[mip]
	px, err := t.mipBytes(m, uexp, ubulk)
	if err != nil {
		return nil, err
	}
	block, bpp, _ := texBlock(t.Format)
	if bpp > 0 {
		img := NewImg(m.W, m.H)
		for i := 0; i < m.W*m.H; i++ {
			o := img.Pix[i*4:]
			if bpp == 4 { // B8G8R8A8
				o[0], o[1], o[2], o[3] = px[i*4+2], px[i*4+1], px[i*4], px[i*4+3]
			} else {
				o[0], o[1], o[2], o[3] = px[i], px[i], px[i], 255
			}
		}
		return img, nil
	}
	bw, bh := (m.W+3)/4, (m.H+3)/4
	full := NewImg(bw*4, bh*4)
	parallelFor(bh, func(by int) {
		for bx := 0; bx < bw; bx++ {
			b := px[(by*bw+bx)*block:]
			dst := full.Pix[(by*4*full.W+bx*4)*4:]
			decodeTexBlock(t.Format, b[:block], dst, full.W*4)
		}
	})
	if t.Format == "PF_BC5" {
		full = rebuildNormalBlue(full)
	}
	if full.W != m.W || full.H != m.H {
		full = full.Sub(0, 0, m.W, m.H)
	}
	return full, nil
}

func decodeTexBlock(format string, src []byte, dst []uint8, stride int) {
	switch format {
	case "PF_DXT1":
		decodeBC1Block(src, dst, stride)
	case "PF_DXT5":
		decodeBC1Block(src[8:], dst, stride)
		decodeBC4Block(src[:8], dst, stride, 3)
	case "PF_BC4":
		decodeBC4Block(src, dst, stride, 0)
		for i := 0; i < 16; i++ {
			o := (i/4)*stride + (i%4)*4
			dst[o+1], dst[o+2], dst[o+3] = dst[o], dst[o], 255
		}
	case "PF_BC5":
		decodeBC4Block(src[:8], dst, stride, 0)
		decodeBC4Block(src[8:], dst, stride, 1)
		for i := 0; i < 16; i++ {
			o := (i/4)*stride + (i%4)*4
			dst[o+2], dst[o+3] = 0, 255
		}
	case "PF_BC7":
		decodeBC7Block(src, dst, stride)
	}
}

func encodeTexBlock(format string, src []uint8, stride int, dst []byte) {
	switch format {
	case "PF_DXT1":
		encodeBC1Block(src, stride, dst)
	case "PF_DXT5":
		encodeBC4Block(src, stride, 3, dst[:8])
		encodeBC1Block(src, stride, dst[8:16])
	case "PF_BC4":
		encodeBC4Block(src, stride, 0, dst)
	case "PF_BC5":
		encodeBC4Block(src, stride, 0, dst[:8])
		encodeBC4Block(src, stride, 1, dst[8:16])
	case "PF_BC7":
		encodeBC7Block(src, stride, dst)
	}
}

// encodeMip writes img (exactly the mip's size) into dst in the texture's format.
func (t *Tex2D) encodeMip(img *Img, dst []byte) {
	block, bpp, _ := texBlock(t.Format)
	if bpp > 0 {
		for i := 0; i < img.W*img.H; i++ {
			s := img.Pix[i*4:]
			if bpp == 4 {
				dst[i*4], dst[i*4+1], dst[i*4+2], dst[i*4+3] = s[2], s[1], s[0], s[3]
			} else {
				dst[i] = s[0]
			}
		}
		return
	}
	bw, bh := (img.W+3)/4, (img.H+3)/4
	src := img
	if img.W%4 != 0 || img.H%4 != 0 { // pad by repeating the edge
		src = NewImg(bw*4, bh*4)
		for y := 0; y < src.H; y++ {
			for x := 0; x < src.W; x++ {
				copy(src.Pix[(y*src.W+x)*4:(y*src.W+x)*4+4], img.Pix[(min(y, img.H-1)*img.W+min(x, img.W-1))*4:])
			}
		}
	}
	parallelFor(bh, func(by int) {
		for bx := 0; bx < bw; bx++ {
			encodeTexBlock(t.Format, src.Pix[(by*4*src.W+bx*4)*4:], src.W*4, dst[(by*bw+bx)*block:(by*bw+bx+1)*block])
		}
	})
}

// Rebuild writes a new picture (any size; it is resized to the texture's) into copies of the
// .uexp and .ubulk, with all the smaller mips made from it.
func (t *Tex2D) Rebuild(uexp, ubulk []byte, img *Img, progress func(done, total int)) ([]byte, []byte, error) {
	newExp := append([]byte(nil), uexp...)
	var newBulk []byte
	if t.NeedsBulk() {
		newBulk = append([]byte(nil), ubulk...)
	}
	cur := img
	if cur.W != t.W() || cur.H != t.H() {
		cur = resize(cur, t.W(), t.H())
	}
	for i, m := range t.Mips {
		if i > 0 {
			cur = halve(cur, t.SRGB)
			if t.Format == "PF_BC5" {
				cur = renormalise(cur)
			}
		}
		if cur.W != m.W || cur.H != m.H {
			cur = resize(cur, m.W, m.H)
		}
		dst, err := t.mipBytes(m, newExp, newBulk)
		if err != nil {
			return nil, nil, err
		}
		t.encodeMip(cur, dst)
		if progress != nil {
			progress(i+1, len(t.Mips))
		}
	}
	return newExp, newBulk, nil
}

// renormalise keeps a smaller normal map's vectors one long (averaging shortens them).
func renormalise(m *Img) *Img {
	for i := 0; i < m.W*m.H; i++ {
		o := m.Pix[i*4:]
		x, y := float64(o[0])/127.5-1, float64(o[1])/127.5-1
		l := math.Hypot(x, y)
		if l > 1 {
			x, y = x/l, y/l
		}
		o[0] = uint8(math.Round((x + 1) * 127.5))
		o[1] = uint8(math.Round((y + 1) * 127.5))
	}
	return m
}

// LoadTex2D reads an ordinary texture package and its .ubulk (when it has one).
func LoadTex2D(base string) (*Tex2D, *Package, []byte, error) {
	p, err := LoadPackage(base)
	if err != nil {
		return nil, nil, nil, err
	}
	t, err := ParseTex2D(p)
	if err != nil {
		return nil, nil, nil, err
	}
	var ubulk []byte
	if t.NeedsBulk() {
		if ubulk, err = os.ReadFile(base + ".ubulk"); err != nil {
			return nil, nil, nil, fmt.Errorf("%s keeps its largest sizes in a .ubulk file, which is missing", packageBaseName(p.Name))
		}
	}
	return t, p, ubulk, nil
}

// ------------------------------------------------------------------ BC4 (one channel)

// decodeBC4Block writes one channel (0 to 3) of 16 pixels.
func decodeBC4Block(src []byte, dst []uint8, stride, ch int) {
	pal := bc4Palette(src[0], src[1])
	var bits uint64
	for i := 0; i < 6; i++ {
		bits |= uint64(src[2+i]) << (8 * uint(i))
	}
	for i := 0; i < 16; i++ {
		dst[(i/4)*stride+(i%4)*4+ch] = pal[(bits>>(3*uint(i)))&7]
	}
}

func bc4Palette(a0, a1 uint8) (pal [8]uint8) {
	pal[0], pal[1] = a0, a1
	if a0 > a1 {
		for i := 1; i < 7; i++ {
			pal[i+1] = uint8((int(a0)*(7-i) + int(a1)*i + 3) / 7)
		}
	} else {
		for i := 1; i < 5; i++ {
			pal[i+1] = uint8((int(a0)*(5-i) + int(a1)*i + 2) / 5)
		}
		pal[6], pal[7] = 0, 255
	}
	return
}

// encodeBC4Block compresses one channel of 16 pixels into 8 bytes, trying both block modes and
// a few end points around the block's range.
func encodeBC4Block(src []uint8, stride, ch int, dst []byte) {
	var v [16]int
	lo, hi := 255, 0
	for i := 0; i < 16; i++ {
		x := int(src[(i/4)*stride+(i%4)*4+ch])
		v[i] = x
		lo, hi = min(lo, x), max(hi, x)
	}
	best := math.MaxInt
	var bestBits uint64
	var b0, b1 uint8
	try := func(a0, a1 int) {
		if a0 < 0 || a1 < 0 || a0 > 255 || a1 > 255 {
			return
		}
		pal := bc4Palette(uint8(a0), uint8(a1))
		var bits uint64
		total := 0
		for i := 0; i < 16; i++ {
			e, k := math.MaxInt, 0
			for j := 0; j < 8; j++ {
				d := v[i] - int(pal[j])
				if d*d < e {
					e, k = d*d, j
				}
			}
			total += e
			bits |= uint64(k) << (3 * uint(i))
		}
		if total < best {
			best, bestBits, b0, b1 = total, bits, uint8(a0), uint8(a1)
		}
	}
	if lo == hi {
		try(lo, lo)
	} else {
		for d0 := -1; d0 <= 1; d0++ {
			for d1 := -1; d1 <= 1; d1++ {
				try(hi+d0, lo+d1) // eight-value mode
			}
		}
		// six-value mode with exact 0 and 255, for blocks that touch the ends
		lo6, hi6 := 255, 0
		for _, x := range v {
			if x != 0 && x != 255 {
				lo6, hi6 = min(lo6, x), max(hi6, x)
			}
		}
		if lo6 <= hi6 {
			try(lo6, hi6)
		}
	}
	dst[0], dst[1] = b0, b1
	for i := 0; i < 6; i++ {
		dst[2+i] = byte(bestBits >> (8 * uint(i)))
	}
}
