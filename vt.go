package main

// Reading and rebuilding the game's streamed ("virtual") UDIM textures.
//
// A texture is three files: .uasset (names, unchanged), .uexp (description) and .ubulk (pixels).
// The .ubulk is a run of chunks; each chunk is a small header followed by fixed-size compressed tiles.
// Each tile covers TileSize x TileSize pixels plus a border on every side.

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

type mipTable struct {
	W, H       int // tiles across and down
	MaxAddr    uint32
	Addr, Offs []uint32 // runs of tile numbers (in Morton order): from Addr[i], stored from slot Offs[i] on (or not stored)
}

type vtChunk struct {
	Start, Size int // position in the .ubulk
	HashAt      int // position of the 20-byte SHA-1 in the .uexp
	Codec       byte
	PayloadSize uint32 // the chunk's header before its tiles
	codecRaw    []byte // codec and payload offset of each layer, as stored
	BulkIndex   int32  // which of the package's data resources holds the chunk
}

// VTex is the description of one streamed texture, read from its .uexp.
type VTex struct {
	Format           string // PF_DXT1 or PF_BC7
	BlockBytes       int    // bytes per 4x4 block
	BlocksW, BlocksH int    // UDIM squares across and down
	Tile, Border     int
	TileBytes        int
	Mips             int
	Width, Height    int
	ChunkOfMip       []uint32
	MipBase          []uint32
	Tables           []mipTable
	FallbackAt       int // position of the stand-in colour (4 floats) in the .uexp
	Fallback         [4]float32
	Chunks           []vtChunk
	SRGB             bool // colour data (true) or plain numbers such as roughness (false)
	chunksResolved   bool
	chunkTableAt     int
	chunkEnd         int // end of the chunk records (and of the texture's own data) in the .uexp
	layers           int
	blockMips        []int // per UDIM square: the largest copy the file stores of it (0 = full size)

	// where things are in the .uexp, for laying the texture out anew (vtformat.go)
	pfAt, pfAfter, perLayerAt, legacyAt, layerPFAt int

	// an ordinary texture carried as a skin texture of one square (vtplain.go)
	plain    *Tex2D
	plainExp []byte
}

var errNotStreamed = errors.New("this is not a streamed UDIM texture (it is stored as an ordinary texture), which this program cannot rebuild yet")

func u32at(d []byte, p int) (uint32, error) {
	if p < 0 || p+4 > len(d) {
		return 0, errors.New("texture description ends unexpectedly")
	}
	return binary.LittleEndian.Uint32(d[p:]), nil
}

type rdr struct {
	d   []byte
	p   int
	err error
}

func (r *rdr) u32() uint32 {
	v, err := u32at(r.d, r.p)
	if err != nil && r.err == nil {
		r.err = err
	}
	r.p += 4
	return v
}

func (r *rdr) arr() []uint32 {
	n := int(r.u32())
	if r.err != nil || n < 0 || n > 1<<20 || r.p+4*n > len(r.d) {
		if r.err == nil {
			r.err = errors.New("texture description has an impossible list length")
		}
		return nil
	}
	out := make([]uint32, n)
	for i := range out {
		out[i] = binary.LittleEndian.Uint32(r.d[r.p+4*i:])
	}
	r.p += 4 * n
	return out
}

// findPixelFormat locates the first length-prefixed "PF_..." name in the .uexp.
func findPixelFormat(d []byte, from int) (name string, at, after int) {
	for i := from; ; {
		k := bytes.Index(d[i:], []byte("PF_"))
		if k < 0 {
			return "", -1, -1
		}
		k += i
		if k >= 4 {
			n := int(binary.LittleEndian.Uint32(d[k-4:]))
			if n > 3 && n < 64 && k+n <= len(d) && d[k+n-1] == 0 {
				return string(d[k : k+n-1]), k - 4, k + n
			}
		}
		i = k + 3
	}
}

// ParseVTex reads the texture description from the bytes of a .uexp file.
func ParseVTex(uexp []byte) (*VTex, error) {
	name, at, after := findPixelFormat(uexp, 0)
	if name == "" {
		return nil, errors.New("no pixel format found; this does not look like a texture")
	}
	t := &VTex{Format: name, pfAt: at, pfAfter: after}
	switch name {
	case "PF_DXT1":
		t.BlockBytes = 8
	case "PF_BC7":
		t.BlockBytes = 16
	}
	r := &rdr{d: uexp, p: after}
	r.u32() // first mip
	r.u32() // mip count (zero for streamed textures)
	isVirtual := r.u32()
	if r.err != nil {
		return nil, r.err
	}
	if isVirtual != 1 {
		return nil, errNotStreamed
	}
	r.u32() // cooked flag
	t.layers = int(r.u32())
	t.BlocksW, t.BlocksH = int(r.u32()), int(r.u32())
	t.Tile, t.Border = int(r.u32()), int(r.u32())
	t.perLayerAt = r.p
	perLayer := r.arr()
	t.Mips, t.Width, t.Height = int(r.u32()), int(r.u32()), int(r.u32())
	t.ChunkOfMip = r.arr()
	t.MipBase = r.arr()
	n := int(r.u32())
	if r.err != nil {
		return nil, r.err
	}
	if n < 1 || n > 32 {
		return nil, errors.New("texture description has an impossible number of sizes")
	}
	for i := 0; i < n; i++ {
		w, h := int(r.u32()), int(r.u32())
		maxAddr := r.u32()
		t.Tables = append(t.Tables, mipTable{W: w, H: h, MaxAddr: maxAddr, Addr: r.arr(), Offs: r.arr()})
	}
	t.legacyAt = r.p
	r.arr() // three legacy lists, empty in this game
	r.arr()
	r.arr()
	if r.err != nil {
		return nil, r.err
	}
	if t.layers != 1 {
		return nil, fmt.Errorf("texture has %d layers; only single-layer textures are supported", t.layers)
	}
	if t.BlockBytes == 0 {
		return nil, fmt.Errorf("pixel format %s is not supported (only PF_DXT1 and PF_BC7)", name)
	}
	if len(perLayer) != 1 || t.Mips != n || len(t.ChunkOfMip) != n || len(t.MipBase) != n {
		return nil, errors.New("texture description is inconsistent")
	}
	t.TileBytes = int(perLayer[0])
	stored := t.Tile + 2*t.Border
	if t.Tile <= 0 || t.Tile%4 != 0 || stored%4 != 0 || t.TileBytes != (stored/4)*(stored/4)*t.BlockBytes {
		return nil, errors.New("tile size does not match the pixel format; the tiles may be compressed in a way this program cannot handle")
	}
	if t.BlocksW < 1 || t.BlocksH < 1 || t.Width%t.BlocksW != 0 || t.Height%t.BlocksH != 0 {
		return nil, errors.New("texture size does not divide into its UDIM squares")
	}
	// layer pixel format again, then the stand-in colour
	name2, at2, after2 := findPixelFormat(uexp, r.p)
	if name2 != name || at2 != r.p {
		return nil, errors.New("texture description layout is not the expected one")
	}
	t.layerPFAt = at2
	t.FallbackAt = after2
	if after2+16+4 > len(uexp) {
		return nil, errors.New("texture description ends unexpectedly")
	}
	for i := 0; i < 4; i++ {
		t.Fallback[i] = math.Float32frombits(binary.LittleEndian.Uint32(uexp[after2+4*i:]))
	}
	t.chunkTableAt = after2 + 16
	t.SRGB = guessSRGB(t.Fallback)
	t.blockMips = make([]int, t.BlocksW*t.BlocksH)
	for by := 0; by < t.BlocksH; by++ {
		for bx := 0; bx < t.BlocksW; bx++ {
			t.blockMips[by*t.BlocksW+bx] = t.findBlockMip(bx, by)
		}
	}
	return t, nil
}

// tileStored reports whether the file keeps a tile of one of its sizes.
func (t *VTex) tileStored(mip, tx, ty int) bool {
	tb := t.Tables[mip]
	a := morton(tx, ty)
	k := -1
	for i, v := range tb.Addr {
		if v <= a {
			k = i
		}
	}
	return k >= 0 && k < len(tb.Offs) && tb.Offs[k] != 0xFFFFFFFF
}

// blockRect is the place of one UDIM square inside the picture of a given size.
func (t *VTex) blockRect(mip, bx, by int) (x0, y0, w, h int) {
	w = max((t.Width/t.BlocksW)>>uint(mip), 1)
	h = max((t.Height/t.BlocksH)>>uint(mip), 1)
	return bx * w, by * h, w, h
}

// findBlockMip finds the largest copy of a UDIM square that the file stores in full.
// The game can keep some squares smaller than others (a 1024-pixel tile next to 4096-pixel
// ones); such a square simply has no tiles in the larger sizes. -1 means nothing is stored.
func (t *VTex) findBlockMip(bx, by int) int {
	for m := 0; m < t.Mips && m < len(t.Tables); m++ {
		x0, y0, w, h := t.blockRect(m, bx, by)
		whole := true
		for ty := y0 / t.Tile; ty <= (y0+h-1)/t.Tile && whole; ty++ {
			for tx := x0 / t.Tile; tx <= (x0+w-1)/t.Tile; tx++ {
				if !t.tileStored(m, tx, ty) {
					whole = false
					break
				}
			}
		}
		if whole {
			return m
		}
	}
	return -1
}

// BlockMip says how much smaller than the others a UDIM square is stored: 0 for full size,
// 1 for half, 2 for a quarter, and so on.
func (t *VTex) BlockMip(bx, by int) int {
	if bx < 0 || by < 0 || bx >= t.BlocksW || by >= t.BlocksH || len(t.blockMips) != t.BlocksW*t.BlocksH {
		return 0
	}
	return max(t.blockMips[by*t.BlocksW+bx], 0)
}

// SmallTilesNote describes the tiles the game stores smaller than the rest, or "" if there are none.
func (t *VTex) SmallTilesNote() string {
	bySize := map[int][]string{}
	var sizes []int
	for by := 0; by < t.BlocksH; by++ {
		for bx := 0; bx < t.BlocksW; bx++ {
			if m := t.BlockMip(bx, by); m > 0 {
				px := (t.Width / t.BlocksW) >> uint(m)
				if bySize[px] == nil {
					sizes = append(sizes, px)
				}
				bySize[px] = append(bySize[px], strconv.Itoa(udimNumber(bx, by)))
			}
		}
	}
	if len(sizes) == 0 {
		return ""
	}
	full := t.Width / t.BlocksW
	var parts []string
	for _, px := range sizes {
		names := bySize[px]
		list, are := names[0], "tile %s is"
		if len(names) > 1 {
			list, are = strings.Join(names[:len(names)-1], ", ")+" and "+names[len(names)-1], "tiles %s are"
		}
		parts = append(parts, fmt.Sprintf(are+" %d x %d", list, px, px))
	}
	text := strings.Join(parts, "; ")
	return fmt.Sprintf("In the game, %s instead of %d x %d.", text, full, full)
}

// decodeRect returns part of one of the stored sizes.
func (t *VTex) decodeRect(ubulk []byte, mip, x0, y0, w, h int) (*Img, error) {
	out := NewImg(w, h)
	P := t.Tile + 2*t.Border
	side := P / 4
	buf := make([]uint8, P*P*4)
	for ty := y0 / t.Tile; ty <= (y0+h-1)/t.Tile; ty++ {
		for tx := x0 / t.Tile; tx <= (x0+w-1)/t.Tile; tx++ {
			off := t.tileOffset(mip, tx, ty)
			if off < 0 {
				return nil, errors.New("a tile of this size is not stored in the file")
			}
			src := ubulk[off : off+t.TileBytes]
			for by := 0; by < side; by++ {
				for bx := 0; bx < side; bx++ {
					t.decodeBlock(src[(by*side+bx)*t.BlockBytes:], buf[(by*4*P+bx*4)*4:], P*4)
				}
			}
			xa, xb := max(x0, tx*t.Tile), min(x0+w, tx*t.Tile+t.Tile)
			for y := max(y0, ty*t.Tile); y < min(y0+h, ty*t.Tile+t.Tile); y++ {
				copy(out.Pix[((y-y0)*w+(xa-x0))*4:((y-y0)*w+(xb-x0))*4], buf[((y-ty*t.Tile+t.Border)*P+(xa-tx*t.Tile+t.Border))*4:])
			}
		}
	}
	return out, nil
}

// fillSmallBlocks paints the squares that are stored smaller than size "mip" into out, each
// stored pixel repeated to fill its place, and returns how many tiles of that size they cover.
func (t *VTex) fillSmallBlocks(ubulk []byte, out *Img, mip int) (int, error) {
	covered := 0
	for by := 0; by < t.BlocksH; by++ {
		for bx := 0; bx < t.BlocksW; bx++ {
			bm := t.blockMips[by*t.BlocksW+bx]
			if bm < 0 {
				return 0, fmt.Errorf("tile %d has no picture stored in this file", udimNumber(bx, by))
			}
			if bm <= mip {
				continue
			}
			sx, sy, sw, sh := t.blockRect(bm, bx, by)
			src, err := t.decodeRect(ubulk, bm, sx, sy, sw, sh)
			if err != nil {
				return 0, err
			}
			dx, dy, dw, dh := t.blockRect(mip, bx, by)
			scale := 1 << uint(bm-mip)
			for y := 0; y < dh && dy+y < out.H; y++ {
				srow := src.Pix[min(y/scale, sh-1)*sw*4:]
				drow := out.Pix[((dy+y)*out.W+dx)*4:]
				for x := 0; x < dw && dx+x < out.W; x++ {
					copy(drow[x*4:x*4+4], srow[min(x/scale, sw-1)*4:])
				}
			}
			for ty := dy / t.Tile; ty <= (dy+dh-1)/t.Tile; ty++ {
				for tx := dx / t.Tile; tx <= (dx+dw-1)/t.Tile; tx++ {
					if !t.tileStored(mip, tx, ty) {
						covered++
					}
				}
			}
		}
	}
	return covered, nil
}

func srgbToLinear(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

func linearToSRGB(v float64) float64 {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 1
	}
	if v <= 0.0031308 {
		return v * 12.92
	}
	return 1.055*math.Pow(v, 1/2.4) - 0.055
}

// guessSRGB looks at the stand-in colour: the game stores it as whole 8-bit steps for plain data,
// and as those steps converted to linear light for colour textures.
func guessSRGB(fb [4]float32) bool {
	offPlain, offColour := 0.0, 0.0
	for i := 0; i < 3; i++ {
		a := float64(fb[i]) * 255
		b := linearToSRGB(float64(fb[i])) * 255
		offPlain += math.Abs(a - math.Round(a))
		offColour += math.Abs(b - math.Round(b))
	}
	return offColour+0.02 < offPlain
}

// ResolveChunks matches each chunk record in the .uexp to its bytes in the .ubulk by checksum.
func (t *VTex) ResolveChunks(uexp, ubulk []byte) error {
	return t.readChunks(uexp, len(ubulk), ubulk)
}

// readChunks reads the chunk records; with ubulk it also checks each chunk's checksum.
func (t *VTex) readChunks(uexp []byte, bulkLen int, ubulk []byte) error {
	p := t.chunkTableAt
	cnt, err := u32at(uexp, p)
	if err != nil {
		return err
	}
	p += 4
	if cnt < 1 || cnt > 64 {
		return errors.New("texture description has an impossible number of chunks")
	}
	rec := 20 + 4 + 4 + 5*t.layers + 4
	start := 0
	t.Chunks = nil
	for c := 0; c < int(cnt); c++ {
		if p+rec > len(uexp) {
			return errors.New("texture description ends unexpectedly")
		}
		size := int(binary.LittleEndian.Uint32(uexp[p+20:]))
		if size <= 0 || start+size > bulkLen {
			return errors.New("the .ubulk file is smaller than the .uexp says; the two files are not a matching pair")
		}
		if ubulk != nil {
			sum := sha1.Sum(ubulk[start : start+size])
			if !bytes.Equal(sum[:], uexp[p:p+20]) {
				return errors.New("the .uexp and .ubulk do not match (checksum differs); use an untouched pair extracted from the game")
			}
		}
		codec := uexp[p+28]
		if codec != 4 {
			return fmt.Errorf("chunk %d uses tile codec %d; only uncompressed tiles (codec 4) are supported", c, codec)
		}
		t.Chunks = append(t.Chunks, vtChunk{Start: start, Size: size, HashAt: p, Codec: codec,
			PayloadSize: binary.LittleEndian.Uint32(uexp[p+24:]),
			codecRaw:    append([]byte(nil), uexp[p+28:p+28+5*t.layers]...),
			BulkIndex:   int32(binary.LittleEndian.Uint32(uexp[p+28+5*t.layers:]))})
		start += size
		p += rec
	}
	if start != bulkLen {
		return errors.New("the .ubulk file has extra data the .uexp does not describe")
	}
	for m := 0; m < t.Mips; m++ {
		if int(t.ChunkOfMip[m]) >= len(t.Chunks) {
			return errors.New("texture description points at a missing chunk")
		}
	}
	t.chunkEnd = p
	t.chunksResolved = true
	return nil
}

func morton(x, y int) uint32 {
	var r uint32
	for b := uint(0); b < 16; b++ {
		r |= uint32(x>>b&1)<<(2*b) | uint32(y>>b&1)<<(2*b+1)
	}
	return r
}

// tileOffset returns the position of a tile in the .ubulk, or -1 if the tile is not stored.
func (t *VTex) tileOffset(mip, tx, ty int) int {
	tb := t.Tables[mip]
	a := morton(tx, ty)
	k := -1
	for i, v := range tb.Addr {
		if v <= a {
			k = i
		}
	}
	if k < 0 || k >= len(tb.Offs) || tb.Offs[k] == 0xFFFFFFFF {
		return -1
	}
	slot := int(tb.Offs[k]) + int(a-tb.Addr[k])
	ch := t.Chunks[t.ChunkOfMip[mip]]
	off := ch.Start + int(t.MipBase[mip]) + slot*t.TileBytes
	if off < ch.Start || off+t.TileBytes > ch.Start+ch.Size {
		return -1
	}
	return off
}

func (t *VTex) decodeBlock(src []byte, dst []uint8, stride int) {
	if t.BlockBytes == 8 {
		decodeBC1Block(src, dst, stride)
	} else {
		decodeBC7Block(src, dst, stride)
	}
}

func (t *VTex) encodeBlock(src []uint8, stride int, dst []byte) {
	if t.BlockBytes == 8 {
		encodeBC1Block(src, stride, dst)
	} else {
		encodeBC7Block(src, stride, dst)
	}
}

func parallelFor(n int, fn func(i int)) {
	workers := runtime.NumCPU()
	if workers > n {
		workers = n
	}
	if workers < 1 {
		workers = 1
	}
	var next int64 = -1
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(atomic.AddInt64(&next, 1))
				if i >= n {
					return
				}
				fn(i)
			}
		}()
	}
	wg.Wait()
}

// DecodeFull returns the full-size picture stored in the texture. A square the game stores
// smaller than the others comes back enlarged, each of its pixels repeated, so that cutting
// the picture down again gives back exactly what is stored.
func (t *VTex) DecodeFull(ubulk []byte, progress func(done, total int)) (*Img, error) {
	if t.plain != nil {
		return t.plain.DecodeMip(t.plainExp, ubulk, 0)
	}
	if !t.chunksResolved {
		return nil, errors.New("internal: chunks not resolved")
	}
	out := NewImg(t.Width, t.Height)
	tb := t.Tables[0]
	P := t.Tile + 2*t.Border
	side := P / 4
	var missing int64
	var done int64
	parallelFor(tb.H, func(ty int) {
		buf := make([]uint8, P*P*4)
		for tx := 0; tx < tb.W; tx++ {
			off := t.tileOffset(0, tx, ty)
			if off < 0 {
				atomic.AddInt64(&missing, 1)
				continue
			}
			src := ubulk[off : off+t.TileBytes]
			for by := 0; by < side; by++ {
				for bx := 0; bx < side; bx++ {
					t.decodeBlock(src[(by*side+bx)*t.BlockBytes:], buf[(by*4*P+bx*4)*4:], P*4)
				}
			}
			for y := 0; y < t.Tile; y++ {
				py := ty*t.Tile + y
				if py >= t.Height {
					break
				}
				w := t.Tile
				if tx*t.Tile+w > t.Width {
					w = t.Width - tx*t.Tile
				}
				if w <= 0 {
					break
				}
				copy(out.Pix[(py*t.Width+tx*t.Tile)*4:(py*t.Width+tx*t.Tile+w)*4], buf[((y+t.Border)*P+t.Border)*4:])
			}
		}
		if progress != nil {
			progress(int(atomic.AddInt64(&done, 1)), tb.H)
		}
	})
	if missing > 0 {
		// Squares the game stores smaller than the rest are filled in from their largest copy.
		covered, err := t.fillSmallBlocks(ubulk, out, 0)
		if err != nil {
			return nil, err
		}
		if int(missing) != covered {
			return nil, fmt.Errorf("%d tiles of the full-size picture are not stored in this file", int(missing)-covered)
		}
	}
	return out, nil
}

// DecodeMip returns one of the smaller copies stored in the texture (mip 0 is full size).
// Only the tiles of that copy are read, so small copies are quick to get.
func (t *VTex) DecodeMip(ubulk []byte, mip int) (*Img, error) {
	if t.plain != nil {
		if mip < 0 || mip >= len(t.plain.Mips) {
			return nil, errors.New("no such size in this texture")
		}
		return t.plain.DecodeMip(t.plainExp, ubulk, mip)
	}
	if !t.chunksResolved {
		return nil, errors.New("internal: chunks not resolved")
	}
	if mip < 0 || mip >= t.Mips {
		return nil, errors.New("no such size in this texture")
	}
	w, h := max(t.Width>>uint(mip), 1), max(t.Height>>uint(mip), 1)
	out := NewImg(w, h)
	tb := t.Tables[mip]
	P := t.Tile + 2*t.Border
	side := P / 4
	buf := make([]uint8, P*P*4)
	missing := 0
	for ty := 0; ty < tb.H; ty++ {
		for tx := 0; tx < tb.W; tx++ {
			if tx*t.Tile >= w || ty*t.Tile >= h {
				continue
			}
			off := t.tileOffset(mip, tx, ty)
			if off < 0 {
				missing++
				continue
			}
			src := ubulk[off : off+t.TileBytes]
			for by := 0; by < side; by++ {
				for bx := 0; bx < side; bx++ {
					t.decodeBlock(src[(by*side+bx)*t.BlockBytes:], buf[(by*4*P+bx*4)*4:], P*4)
				}
			}
			for y := 0; y < t.Tile && ty*t.Tile+y < h; y++ {
				n := min(t.Tile, w-tx*t.Tile)
				copy(out.Pix[((ty*t.Tile+y)*w+tx*t.Tile)*4:((ty*t.Tile+y)*w+tx*t.Tile+n)*4], buf[((y+t.Border)*P+t.Border)*4:])
			}
		}
	}
	if missing > 0 {
		covered, err := t.fillSmallBlocks(ubulk, out, mip)
		if err != nil {
			return nil, err
		}
		if missing != covered {
			return nil, errors.New("a tile of this size is not stored in the file")
		}
	}
	return out, nil
}

// halve makes the next smaller copy of a picture by averaging 2x2 pixels.
// Colour textures are averaged in linear light, plain data directly.
func halve(src *Img, srgb bool) *Img {
	w, h := max(src.W/2, 1), max(src.H/2, 1)
	dst := NewImg(w, h)
	var toLin [256]float32
	var toS [4097]uint8
	if srgb {
		for i := range toLin {
			toLin[i] = float32(srgbToLinear(float64(i) / 255))
		}
		for i := range toS {
			toS[i] = uint8(math.Round(linearToSRGB(float64(i)/4096) * 255))
		}
	}
	parallelFor(h, func(y int) {
		y0 := min(2*y, src.H-1)
		y1 := min(2*y+1, src.H-1)
		for x := 0; x < w; x++ {
			x0 := min(2*x, src.W-1)
			x1 := min(2*x+1, src.W-1)
			a := src.Pix[(y0*src.W+x0)*4:]
			b := src.Pix[(y0*src.W+x1)*4:]
			c := src.Pix[(y1*src.W+x0)*4:]
			d := src.Pix[(y1*src.W+x1)*4:]
			o := dst.Pix[(y*w+x)*4:]
			for ch := 0; ch < 4; ch++ {
				if srgb && ch < 3 {
					v := (toLin[a[ch]] + toLin[b[ch]] + toLin[c[ch]] + toLin[d[ch]]) / 4
					o[ch] = toS[int(v*4096+0.5)]
				} else {
					o[ch] = uint8((int(a[ch]) + int(b[ch]) + int(c[ch]) + int(d[ch]) + 2) / 4)
				}
			}
		}
	})
	return dst
}

// Rebuild writes a new picture into copies of the .uexp and .ubulk and returns them.
// The picture must be exactly the texture's full size.
func (t *VTex) Rebuild(uexp, ubulk []byte, img *Img, progress func(done, total int)) ([]byte, []byte, error) {
	if t.plain != nil {
		return t.plain.Rebuild(uexp, ubulk, img, progress)
	}
	newBulk := make([]byte, len(ubulk))
	copy(newBulk, ubulk)
	newExp := make([]byte, len(uexp))
	copy(newExp, uexp)
	if err := t.rebuildInto(newExp, newBulk, img, progress); err != nil {
		return nil, nil, err
	}
	return newExp, newBulk, nil
}

// rebuildInto writes the picture's tiles, chunk checksums and stand-in colour into the files.
func (t *VTex) rebuildInto(newExp, newBulk []byte, img *Img, progress func(done, total int)) error {
	if !t.chunksResolved {
		return errors.New("internal: chunks not resolved")
	}
	if img.W != t.Width || img.H != t.Height {
		return fmt.Errorf("picture is %d x %d but the texture needs %d x %d", img.W, img.H, t.Width, t.Height)
	}

	total := 0
	for m := 0; m < t.Mips; m++ {
		total += t.Tables[m].W * t.Tables[m].H
	}
	var done int64
	P := t.Tile + 2*t.Border
	side := P / 4
	blockW0, blockH0 := t.Width/t.BlocksW, t.Height/t.BlocksH

	cur := img
	var last *Img
	for m := 0; m < t.Mips; m++ {
		if m > 0 {
			cur = halve(cur, t.SRGB)
		}
		last = cur
		tb := t.Tables[m]
		bw, bh := blockW0>>uint(m), blockH0>>uint(m)
		islands := bw >= t.Tile && bh >= t.Tile
		mip := cur
		var failed int64
		parallelFor(tb.W*tb.H, func(i int) {
			tx, ty := i%tb.W, i/tb.W
			off := t.tileOffset(m, tx, ty)
			if off < 0 {
				// Not stored, so nothing to write. That is expected for a square the game keeps
				// smaller than the others; anywhere else inside the picture it is a fault.
				if tx*t.Tile < mip.W && ty*t.Tile < mip.H {
					small := islands && t.BlockMip(tx*t.Tile/bw, ty*t.Tile/bh) > m
					if !small {
						atomic.AddInt64(&failed, 1)
					}
				}
				atomic.AddInt64(&done, 1)
				return
			}
			buf := make([]uint8, P*P*4)
			// column and row lookups with the game's edge rules
			xs := make([]int, P)
			ys := make([]int, P)
			for k := 0; k < P; k++ {
				x := tx*t.Tile - t.Border + k
				y := ty*t.Tile - t.Border + k
				if islands {
					bx, by := tx*t.Tile/bw, ty*t.Tile/bh
					xs[k] = min(max(x, bx*bw), bx*bw+bw-1)
					ys[k] = min(max(y, by*bh), by*bh+bh-1)
				} else {
					if x < 0 {
						x = 0
					}
					if x >= mip.W {
						x = -1 // black past the right edge
					}
					xs[k] = x
					ys[k] = min(max(y, 0), mip.H-1)
				}
			}
			for r := 0; r < P; r++ {
				row := mip.Pix[ys[r]*mip.W*4:]
				dst := buf[r*P*4:]
				for c := 0; c < P; c++ {
					if xs[c] < 0 {
						dst[c*4], dst[c*4+1], dst[c*4+2], dst[c*4+3] = 0, 0, 0, 255
						continue
					}
					s := row[xs[c]*4:]
					dst[c*4], dst[c*4+1], dst[c*4+2], dst[c*4+3] = s[0], s[1], s[2], s[3]
				}
			}
			out := newBulk[off : off+t.TileBytes]
			for by := 0; by < side; by++ {
				for bx := 0; bx < side; bx++ {
					t.encodeBlock(buf[(by*4*P+bx*4)*4:], P*4, out[(by*side+bx)*t.BlockBytes:])
				}
			}
			d := int(atomic.AddInt64(&done, 1))
			if progress != nil && d%32 == 0 {
				progress(d, total)
			}
		})
		if failed > 0 {
			return fmt.Errorf("size %d: %d tiles have no place in the file", m, failed)
		}
	}
	if progress != nil {
		progress(total, total)
	}
	for _, c := range t.Chunks {
		sum := sha1.Sum(newBulk[c.Start : c.Start+c.Size])
		copy(newExp[c.HashAt:c.HashAt+20], sum[:])
	}
	// stand-in colour: the average of the smallest copy
	var acc [3]float64
	n := float64(last.W * last.H)
	for i := 0; i < last.W*last.H; i++ {
		for c := 0; c < 3; c++ {
			acc[c] += float64(last.Pix[i*4+c])
		}
	}
	for c := 0; c < 3; c++ {
		v := math.Round(acc[c]/n) / 255
		if t.SRGB {
			v = srgbToLinear(v)
		}
		binary.LittleEndian.PutUint32(newExp[t.FallbackAt+4*c:], math.Float32bits(float32(v)))
	}
	return nil
}

// ------------------------------------------------------------------ texture files on disk

// TexFiles is one raw texture: three files with the same name.
type TexFiles struct {
	UAsset, UExp, UBulk string
}

func texFilesFor(anyPath string) TexFiles {
	base := strings.TrimSuffix(anyPath, extOf(anyPath))
	return TexFiles{UAsset: base + ".uasset", UExp: base + ".uexp", UBulk: base + ".ubulk"}
}

func extOf(p string) string {
	for i := len(p) - 1; i >= 0 && p[i] != '/' && p[i] != '\\'; i-- {
		if p[i] == '.' {
			return p[i:]
		}
	}
	return ""
}

// PackageName finds the game path (for example /Game/Vehicles/.../T_Name) stored inside a .uasset.
func PackageName(uasset []byte) string {
	for i := 0; ; {
		k := bytes.Index(uasset[i:], []byte("/Game/"))
		if k < 0 {
			return ""
		}
		k += i
		if k >= 4 {
			n := int(binary.LittleEndian.Uint32(uasset[k-4:]))
			if n > 6 && n < 1024 && k+n <= len(uasset) && uasset[k+n-1] == 0 {
				return string(uasset[k : k+n-1])
			}
		}
		i = k + 6
	}
}

// LoadVTex reads and checks a raw texture from disk.
func LoadVTex(f TexFiles) (*VTex, []byte, []byte, error) {
	uexp, err := os.ReadFile(f.UExp)
	if err != nil {
		return nil, nil, nil, err
	}
	ua, _ := os.ReadFile(f.UAsset)
	t, err := parseAnyTex(ua, uexp)
	if err != nil {
		return nil, nil, nil, err
	}
	ubulk, err := os.ReadFile(f.UBulk)
	if err != nil {
		if t.plain != nil && !t.plain.NeedsBulk() {
			return t, uexp, nil, nil // every size is inside the .uexp
		}
		return nil, nil, nil, err
	}
	if t.plain != nil {
		for _, m := range t.plain.Mips {
			if m.InBulk && m.Off+m.Size > len(ubulk) {
				return nil, nil, nil, errors.New("the .ubulk file is smaller than the .uexp says; the two files are not a matching pair")
			}
		}
		return t, uexp, ubulk, nil
	}
	if err := t.ResolveChunks(uexp, ubulk); err != nil {
		return nil, nil, nil, err
	}
	return t, uexp, ubulk, nil
}
