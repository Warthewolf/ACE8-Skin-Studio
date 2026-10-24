package main

// Image loading and saving: PNG (standard library) and TGA (own reader/writer).
// Everything is held in memory as 8-bit RGBA, top row first.

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Img is an 8-bit RGBA picture, 4 bytes per pixel, top row first.
type Img struct {
	W, H int
	Pix  []uint8
}

func NewImg(w, h int) *Img { return &Img{W: w, H: h, Pix: make([]uint8, w*h*4)} }

// Sub copies a rectangle out of the picture.
func (m *Img) Sub(x0, y0, w, h int) *Img {
	o := NewImg(w, h)
	for y := 0; y < h; y++ {
		copy(o.Pix[y*w*4:(y+1)*w*4], m.Pix[((y0+y)*m.W+x0)*4:((y0+y)*m.W+x0+w)*4])
	}
	return o
}

// Paste copies src into the picture at x0,y0.
func (m *Img) Paste(src *Img, x0, y0 int) {
	for y := 0; y < src.H; y++ {
		copy(m.Pix[((y0+y)*m.W+x0)*4:((y0+y)*m.W+x0+src.W)*4], src.Pix[y*src.W*4:(y+1)*src.W*4])
	}
}

// IsGrey reports whether every pixel has equal red, green and blue.
func (m *Img) IsGrey() bool {
	p := m.Pix
	for i := 0; i < len(p); i += 4 {
		if p[i] != p[i+1] || p[i] != p[i+2] {
			return false
		}
	}
	return true
}

// HasAlpha reports whether any pixel is meaningfully see-through
// (254 counts as opaque: BC7 compression produces it for solid pixels).
func (m *Img) HasAlpha() bool {
	p := m.Pix
	for i := 3; i < len(p); i += 4 {
		if p[i] < 254 {
			return true
		}
	}
	return false
}

func isImageName(name string) bool {
	e := strings.ToLower(filepath.Ext(name))
	return e == ".png" || e == ".tga"
}

// LoadImage reads a PNG or TGA file.
func LoadImage(path string) (*Img, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	br := bufio.NewReaderSize(f, 1<<20)
	head, _ := br.Peek(8)
	if bytes.HasPrefix(head, []byte("\x89PNG\r\n\x1a\n")) {
		im, err := png.Decode(br)
		if err != nil {
			return nil, fmt.Errorf("%s: not a readable PNG (%v)", filepath.Base(path), err)
		}
		return fromGoImage(im), nil
	}
	if strings.EqualFold(filepath.Ext(path), ".tga") {
		data, err := io.ReadAll(br)
		if err != nil {
			return nil, err
		}
		im, err := decodeTGA(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", filepath.Base(path), err)
		}
		return im, nil
	}
	return nil, fmt.Errorf("%s: only PNG and TGA images are supported", filepath.Base(path))
}

// ImageSize reads only the header of a PNG or TGA and returns its size.
func ImageSize(path string) (int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	head := make([]byte, 32)
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	if bytes.HasPrefix(head, []byte("\x89PNG\r\n\x1a\n")) && n >= 24 {
		w := int(head[16])<<24 | int(head[17])<<16 | int(head[18])<<8 | int(head[19])
		h := int(head[20])<<24 | int(head[21])<<16 | int(head[22])<<8 | int(head[23])
		return w, h, nil
	}
	if strings.EqualFold(filepath.Ext(path), ".tga") && n >= 18 {
		return int(head[12]) | int(head[13])<<8, int(head[14]) | int(head[15])<<8, nil
	}
	return 0, 0, errors.New("not a PNG or TGA image")
}

func fromGoImage(im image.Image) *Img {
	b := im.Bounds()
	w, h := b.Dx(), b.Dy()
	o := NewImg(w, h)
	switch s := im.(type) {
	case *image.NRGBA:
		for y := 0; y < h; y++ {
			copy(o.Pix[y*w*4:(y+1)*w*4], s.Pix[y*s.Stride:y*s.Stride+w*4])
		}
	case *image.RGBA:
		for y := 0; y < h; y++ {
			row := s.Pix[y*s.Stride : y*s.Stride+w*4]
			dst := o.Pix[y*w*4 : (y+1)*w*4]
			copy(dst, row)
			for x := 0; x < w*4; x += 4 { // undo premultiplied alpha where needed
				a := uint32(dst[x+3])
				if a != 255 && a != 0 {
					dst[x] = uint8(uint32(dst[x]) * 255 / a)
					dst[x+1] = uint8(uint32(dst[x+1]) * 255 / a)
					dst[x+2] = uint8(uint32(dst[x+2]) * 255 / a)
				}
			}
		}
	case *image.Gray:
		for y := 0; y < h; y++ {
			row := s.Pix[y*s.Stride : y*s.Stride+w]
			dst := o.Pix[y*w*4 : (y+1)*w*4]
			for x, v := range row {
				dst[x*4], dst[x*4+1], dst[x*4+2], dst[x*4+3] = v, v, v, 255
			}
		}
	case *image.Gray16:
		for y := 0; y < h; y++ {
			row := s.Pix[y*s.Stride : y*s.Stride+w*2]
			dst := o.Pix[y*w*4 : (y+1)*w*4]
			for x := 0; x < w; x++ {
				v := row[x*2]
				dst[x*4], dst[x*4+1], dst[x*4+2], dst[x*4+3] = v, v, v, 255
			}
		}
	case *image.NRGBA64:
		for y := 0; y < h; y++ {
			row := s.Pix[y*s.Stride : y*s.Stride+w*8]
			dst := o.Pix[y*w*4 : (y+1)*w*4]
			for x := 0; x < w; x++ {
				dst[x*4], dst[x*4+1], dst[x*4+2], dst[x*4+3] = row[x*8], row[x*8+2], row[x*8+4], row[x*8+6]
			}
		}
	case *image.RGBA64:
		for y := 0; y < h; y++ {
			row := s.Pix[y*s.Stride : y*s.Stride+w*8]
			dst := o.Pix[y*w*4 : (y+1)*w*4]
			for x := 0; x < w; x++ {
				r, g, bl, a := uint32(row[x*8]), uint32(row[x*8+2]), uint32(row[x*8+4]), uint32(row[x*8+6])
				if a != 255 && a != 0 {
					r, g, bl = min(r*255/a, 255), min(g*255/a, 255), min(bl*255/a, 255)
				}
				dst[x*4], dst[x*4+1], dst[x*4+2], dst[x*4+3] = uint8(r), uint8(g), uint8(bl), uint8(a)
			}
		}
	default:
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				r, g, bl, a := im.At(b.Min.X+x, b.Min.Y+y).RGBA()
				if a != 0 && a != 0xffff {
					r, g, bl = r*0xffff/a, g*0xffff/a, bl*0xffff/a
				}
				i := (y*w + x) * 4
				o.Pix[i], o.Pix[i+1], o.Pix[i+2], o.Pix[i+3] = uint8(r>>8), uint8(g>>8), uint8(bl>>8), uint8(a>>8)
			}
		}
	}
	return o
}

// SaveMode says which channels are written.
type SaveMode int

const (
	SaveRGB  SaveMode = iota // colour, no alpha
	SaveRGBA                 // colour with alpha
	SaveGrey                 // single channel (red is used)
)

// SaveImage writes a PNG or TGA chosen by the file extension.
func SaveImage(path string, m *Img, mode SaveMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	bw := bufio.NewWriterSize(f, 1<<20)
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		err = encodePNG(bw, m, mode)
	case ".tga":
		err = encodeTGA(bw, m, mode)
	default:
		err = fmt.Errorf("unsupported output format %q", filepath.Ext(path))
	}
	if err == nil {
		err = bw.Flush()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	os.Remove(path)
	return os.Rename(tmp, path)
}

func encodePNG(w io.Writer, m *Img, mode SaveMode) error {
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	switch mode {
	case SaveGrey:
		g := image.NewGray(image.Rect(0, 0, m.W, m.H))
		for i := 0; i < m.W*m.H; i++ {
			g.Pix[i] = m.Pix[i*4]
		}
		return enc.Encode(w, g)
	case SaveRGBA:
		return enc.Encode(w, &image.NRGBA{Pix: m.Pix, Stride: m.W * 4, Rect: image.Rect(0, 0, m.W, m.H)})
	default:
		// An opaque image.RGBA is written by the encoder as plain RGB without an alpha channel.
		c := &image.RGBA{Pix: make([]uint8, len(m.Pix)), Stride: m.W * 4, Rect: image.Rect(0, 0, m.W, m.H)}
		copy(c.Pix, m.Pix)
		for i := 3; i < len(c.Pix); i += 4 {
			c.Pix[i] = 255
		}
		return enc.Encode(w, c)
	}
}

// ---------------------------------------------------------------- TGA

func decodeTGA(d []byte) (*Img, error) {
	if len(d) < 18 {
		return nil, errors.New("not a TGA image (file too short)")
	}
	idLen, cmapType, typ := int(d[0]), int(d[1]), int(d[2])
	cmapLen := int(d[5]) | int(d[6])<<8
	cmapBits := int(d[7])
	w := int(d[12]) | int(d[13])<<8
	h := int(d[14]) | int(d[15])<<8
	bpp, desc := int(d[16]), d[17]
	if w == 0 || h == 0 {
		return nil, errors.New("TGA image has no pixels")
	}
	p := 18 + idLen
	var cmap []byte
	cmapBytes := (cmapBits + 7) / 8
	if cmapType == 1 {
		n := cmapLen * cmapBytes
		if p+n > len(d) {
			return nil, errors.New("TGA colour map is cut short")
		}
		cmap = d[p : p+n]
		p += n
	}
	rle := false
	switch typ {
	case 1, 2, 3:
	case 9, 10, 11:
		rle = true
		typ -= 8
	default:
		return nil, fmt.Errorf("TGA type %d is not supported", typ)
	}
	bytesPP := (bpp + 7) / 8
	switch {
	case typ == 2 && (bpp == 24 || bpp == 32 || bpp == 16 || bpp == 15):
	case typ == 3 && (bpp == 8 || bpp == 16):
	case typ == 1 && bpp == 8 && cmap != nil && (cmapBits == 24 || cmapBits == 32):
	default:
		return nil, fmt.Errorf("TGA with %d bits per pixel (type %d) is not supported", bpp, typ)
	}
	raw := make([]byte, w*h*bytesPP)
	if !rle {
		if p+len(raw) > len(d) {
			return nil, errors.New("TGA pixel data is cut short")
		}
		copy(raw, d[p:])
	} else {
		o := 0
		for o < len(raw) {
			if p >= len(d) {
				return nil, errors.New("TGA pixel data is cut short")
			}
			c := int(d[p])
			p++
			n := (c & 0x7f) + 1
			if c&0x80 != 0 {
				if p+bytesPP > len(d) {
					return nil, errors.New("TGA pixel data is cut short")
				}
				px := d[p : p+bytesPP]
				p += bytesPP
				for i := 0; i < n && o < len(raw); i++ {
					copy(raw[o:], px)
					o += bytesPP
				}
			} else {
				k := n * bytesPP
				if p+k > len(d) {
					return nil, errors.New("TGA pixel data is cut short")
				}
				if o+k > len(raw) {
					k = len(raw) - o
				}
				copy(raw[o:], d[p:p+k])
				p += n * bytesPP
				o += k
			}
		}
	}
	m := NewImg(w, h)
	topDown := desc&0x20 != 0
	rightLeft := desc&0x10 != 0
	hasAlpha := desc&0x0f != 0
	for y := 0; y < h; y++ {
		sy := y
		if !topDown {
			sy = h - 1 - y
		}
		row := raw[sy*w*bytesPP : (sy+1)*w*bytesPP]
		dst := m.Pix[y*w*4 : (y+1)*w*4]
		for x := 0; x < w; x++ {
			sx := x
			if rightLeft {
				sx = w - 1 - x
			}
			s := row[sx*bytesPP : sx*bytesPP+bytesPP]
			var r, g, b uint8
			a := uint8(255)
			switch {
			case typ == 3:
				r, g, b = s[0], s[0], s[0]
				if bpp == 16 {
					a = s[1]
				}
			case typ == 1:
				e := cmap[int(s[0])*cmapBytes:]
				b, g, r = e[0], e[1], e[2]
				if cmapBytes == 4 {
					a = e[3]
				}
			case bytesPP == 2:
				v := uint16(s[0]) | uint16(s[1])<<8
				r = uint8((v>>10)&31) << 3
				g = uint8((v>>5)&31) << 3
				b = uint8(v&31) << 3
			default:
				b, g, r = s[0], s[1], s[2]
				if bytesPP == 4 && hasAlpha {
					a = s[3]
				}
			}
			dst[x*4], dst[x*4+1], dst[x*4+2], dst[x*4+3] = r, g, b, a
		}
	}
	return m, nil
}

// encodeTGA writes a run-length compressed TGA, bottom row first (the layout most programs expect).
func encodeTGA(w io.Writer, m *Img, mode SaveMode) error {
	bytesPP, typ, desc := 3, byte(10), byte(0)
	switch mode {
	case SaveGrey:
		bytesPP, typ = 1, 11
	case SaveRGBA:
		bytesPP, desc = 4, 8
	}
	hdr := make([]byte, 18)
	hdr[2] = typ
	hdr[12], hdr[13] = byte(m.W), byte(m.W>>8)
	hdr[14], hdr[15] = byte(m.H), byte(m.H>>8)
	hdr[16], hdr[17] = byte(bytesPP*8), desc
	if _, err := w.Write(hdr); err != nil {
		return err
	}
	row := make([]byte, m.W*bytesPP)
	out := make([]byte, 0, m.W*bytesPP*2+16)
	for y := m.H - 1; y >= 0; y-- {
		src := m.Pix[y*m.W*4 : (y+1)*m.W*4]
		for x := 0; x < m.W; x++ {
			s := src[x*4:]
			switch bytesPP {
			case 1:
				row[x] = s[0]
			case 3:
				row[x*3], row[x*3+1], row[x*3+2] = s[2], s[1], s[0]
			default:
				row[x*4], row[x*4+1], row[x*4+2], row[x*4+3] = s[2], s[1], s[0], s[3]
			}
		}
		out = rleRow(out[:0], row, bytesPP)
		if _, err := w.Write(out); err != nil {
			return err
		}
	}
	// TGA 2.0 footer
	_, err := w.Write(append(make([]byte, 8), []byte("TRUEVISION-XFILE.\x00")...))
	return err
}

func rleRow(out, row []byte, bpp int) []byte {
	n := len(row) / bpp
	same := func(a, b int) bool {
		for k := 0; k < bpp; k++ {
			if row[a*bpp+k] != row[b*bpp+k] {
				return false
			}
		}
		return true
	}
	i := 0
	for i < n {
		run := 1
		for i+run < n && run < 128 && same(i, i+run) {
			run++
		}
		if run >= 2 {
			out = append(out, byte(0x80|(run-1)))
			out = append(out, row[i*bpp:(i+1)*bpp]...)
			i += run
			continue
		}
		start := i
		i++
		for i < n && i-start < 128 {
			if i+1 < n && same(i, i+1) {
				break
			}
			i++
		}
		out = append(out, byte(i-start-1))
		out = append(out, row[start*bpp:i*bpp]...)
	}
	return out
}
