package main

// Storing a rebuilt streamed texture differently from the game's original: as BC7 instead of DXT1
// (each 4x4 block 16 bytes instead of 8: sharper colour, files twice as big), and/or with only the
// full-size picture (no smaller copies, "no mipmaps").
//
// The game's own files show what a BC7 texture looks like next to a DXT1 one of the same size: the
// Su-57's normal map (BC7) and colour map (DXT1) have the same tiles, sizes, chunks and tables; only
// the pixel format's name, the bytes per tile, where each size starts inside its chunk, the chunk
// sizes and checksums, and the data-resource entries that point at the chunks differ. Laying a texture
// out anew changes exactly those, plus the list of sizes when the smaller ones are left out.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// TexOptions says how a rebuilt colour texture is stored.
type TexOptions struct {
	BC7    bool `json:"bc7,omitempty"`    // as BC7 instead of DXT1
	NoMips bool `json:"noMips,omitempty"` // only the full-size picture
}

func (o TexOptions) changes() bool { return o.BC7 || o.NoMips }

// forKind gives the options that apply to a texture of this kind: they are for the colour texture.
func (o TexOptions) forKind(k Kind) TexOptions {
	if k != KindColour {
		return TexOptions{}
	}
	return o
}

// describe says in a few words how a texture was stored, for the build log.
func (o TexOptions) describe() string {
	var parts []string
	if o.BC7 {
		parts = append(parts, "BC7")
	}
	if o.NoMips {
		parts = append(parts, "no mipmaps")
	}
	return strings.Join(parts, ", ")
}

// way says how colour textures are stored, for the problem report.
func (o TexOptions) way() string {
	if d := o.describe(); d != "" {
		return d
	}
	return "the game's own format"
}

// texNote is " as BC7, no mipmaps" (or "") for the build log.
func texNote(o TexOptions) string {
	if d := o.describe(); d != "" {
		return ", stored as " + d
	}
	return ""
}

// tableSlots counts the tiles a size stores, checking that its runs fill slots 0, 1, 2... in order.
func tableSlots(tb mipTable) (int, error) {
	if len(tb.Addr) != len(tb.Offs) || len(tb.Addr) == 0 {
		return 0, errors.New("a tile table is inconsistent")
	}
	slots := 0
	for i, a := range tb.Addr {
		end := tb.MaxAddr
		if i+1 < len(tb.Addr) {
			end = tb.Addr[i+1]
		}
		if end < a {
			return 0, errors.New("a tile table is out of order")
		}
		if tb.Offs[i] == 0xFFFFFFFF {
			continue
		}
		if int(tb.Offs[i]) != slots {
			return 0, errors.New("a tile table does not store its tiles in order")
		}
		slots += int(end - a)
	}
	return slots, nil
}

// unmorton is the tile position of a Morton number.
func unmorton(a uint32) (x, y int) {
	for b := uint(0); b < 16; b++ {
		x |= int(a>>(2*b)&1) << b
		y |= int(a>>(2*b+1)&1) << b
	}
	return
}

// fullTable stores every tile of a size that is w x h tiles.
func fullTable(w, h int, maxAddr uint32) mipTable {
	tb := mipTable{W: w, H: h, MaxAddr: maxAddr}
	slot, prev := uint32(0), -1
	for a := uint32(0); a < maxAddr; a++ {
		x, y := unmorton(a)
		in := 0
		if x < w && y < h {
			in = 1
		}
		if in != prev {
			tb.Addr = append(tb.Addr, a)
			if in == 1 {
				tb.Offs = append(tb.Offs, slot)
			} else {
				tb.Offs = append(tb.Offs, 0xFFFFFFFF)
			}
			prev = in
		}
		if in == 1 {
			slot++
		}
	}
	return tb
}

// vtLayout is where every size goes: each chunk is its header followed by the tiles of its sizes,
// smallest size number first.
type vtLayout struct {
	chunkOfMip, mipBase []uint32
	chunks              []int // old chunk number of each new chunk
	sizes               []int // size in bytes of each new chunk
}

func layoutVT(tables []mipTable, chunkOfMip []uint32, headers map[uint32]int, tileBytes int) (vtLayout, error) {
	var l vtLayout
	newOf := map[uint32]int{}
	for m, c := range chunkOfMip {
		nc, ok := newOf[c]
		if !ok {
			nc = len(l.chunks)
			newOf[c] = nc
			l.chunks = append(l.chunks, int(c))
			l.sizes = append(l.sizes, headers[c])
		}
		if m > 0 && int(chunkOfMip[m-1]) != int(c) && newOf[chunkOfMip[m-1]] > nc {
			return l, errors.New("the texture's sizes are not kept in chunk order")
		}
		n, err := tableSlots(tables[m])
		if err != nil {
			return l, err
		}
		l.chunkOfMip = append(l.chunkOfMip, uint32(nc))
		l.mipBase = append(l.mipBase, uint32(l.sizes[nc]))
		l.sizes[nc] += n * tileBytes
	}
	return l, nil
}

// reshapeVT returns the package laid out for opts: new .uasset and .uexp, and a .ubulk of the right
// size with each chunk's header in place and its tiles still empty, plus the description of the new
// layout for rebuildInto to write the pixels.
func reshapeVT(t *VTex, ua, uexp, ubulk []byte, opts TexOptions) (nua, nexp, nbulk []byte, nt *VTex, err error) {
	if !t.chunksResolved {
		return nil, nil, nil, nil, errors.New("internal: chunks not resolved")
	}
	pkg, err := ParsePackage(ua, uexp)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if len(pkg.Exports) != 1 || pkg.Exports[0].SerialOffset != int64(pkg.totalHeader) {
		return nil, nil, nil, nil, errors.New("the texture package is not laid out as expected")
	}
	expEnd := int(pkg.Exports[0].SerialSize)
	if t.chunkEnd <= 0 || t.chunkEnd > expEnd || t.layers != 1 {
		return nil, nil, nil, nil, errors.New("the texture package is not laid out as expected")
	}
	le := binary.LittleEndian
	// 1. the pixel format
	format, blockBytes := t.Format, t.BlockBytes
	if opts.BC7 && t.Format == "PF_DXT1" {
		format, blockBytes = "PF_BC7", 16
	}
	side := (t.Tile + 2*t.Border) / 4
	tileBytes := side * side * blockBytes
	// the offset before the pixel data that says where it ends, counted from where it is stored
	skipAt := t.pfAt - 36
	if skipAt < 8 || int(int64(le.Uint64(uexp[skipAt:])))+skipAt != t.chunkEnd {
		return nil, nil, nil, nil, errors.New("the texture's description is not laid out as expected (no skip offset)")
	}
	names := append([]string(nil), pkg.Names...)
	if format != t.Format {
		fi := int(le.Uint32(uexp[skipAt-8:]))
		if fi < 0 || fi >= len(names) || names[fi] != t.Format || le.Uint32(uexp[skipAt-4:]) != 0 {
			return nil, nil, nil, nil, errors.New("the texture's description is not laid out as expected (format name)")
		}
		for i, n := range names {
			if n == format && i != fi {
				return nil, nil, nil, nil, fmt.Errorf("the package already has a name %s", format)
			}
		}
		names[fi] = format
	}
	// 2. the sizes kept
	mips := t.Mips
	if opts.NoMips {
		mips = 1
	}
	tables := append([]mipTable(nil), t.Tables[:mips]...)
	if opts.NoMips {
		for _, bm := range t.blockMips {
			if bm != 0 { // a square stored smaller than the rest gets full-size tiles too
				tables[0] = fullTable(tables[0].W, tables[0].H, tables[0].MaxAddr)
				break
			}
		}
	}
	// 3. chunks: check the original's layout is the one this program writes, then lay out the new
	headers := map[uint32]int{}
	for m, c := range t.ChunkOfMip {
		if h, ok := headers[c]; !ok || int(t.MipBase[m]) < h {
			headers[c] = int(t.MipBase[m])
		}
	}
	for c, h := range headers {
		if h != int(t.Chunks[c].PayloadSize) || h > t.Chunks[c].Size {
			return nil, nil, nil, nil, errors.New("the texture's chunks are not laid out as expected")
		}
	}
	old, err := layoutVT(t.Tables, t.ChunkOfMip, headers, t.TileBytes)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if len(old.chunks) != len(t.Chunks) {
		return nil, nil, nil, nil, errors.New("the texture has chunks no size uses")
	}
	for m := range t.MipBase {
		if old.mipBase[m] != t.MipBase[m] || old.chunks[old.chunkOfMip[m]] != int(t.ChunkOfMip[m]) {
			return nil, nil, nil, nil, errors.New("the texture's sizes are not laid out as expected")
		}
	}
	for i, c := range old.chunks {
		if old.sizes[i] != t.Chunks[c].Size {
			return nil, nil, nil, nil, errors.New("the texture's chunk sizes are not as expected")
		}
	}
	lay, err := layoutVT(tables, t.ChunkOfMip[:mips], headers, tileBytes)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	// 4. data resources: each chunk has one; the kept chunks' entries follow each other in the .ubulk
	rs, err := pkg.DataResources()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	used := map[int32]bool{}
	for _, c := range t.Chunks {
		if c.BulkIndex < 0 || int(c.BulkIndex) >= len(rs) || used[c.BulkIndex] {
			return nil, nil, nil, nil, errors.New("the texture's chunks and data resources do not match")
		}
		used[c.BulkIndex] = true
	}
	if len(used) != len(rs) {
		return nil, nil, nil, nil, errors.New("the texture package holds other data besides its tiles")
	}
	var newRs []DataResource
	total := 0
	for i, c := range lay.chunks {
		r := rs[t.Chunks[c].BulkIndex]
		r.SerialOffset, r.SerialSize, r.RawSize = int64(total), int64(lay.sizes[i]), int64(lay.sizes[i])
		newRs = append(newRs, r)
		total += lay.sizes[i]
	}
	// 5. the description
	var b bytes.Buffer
	u32 := func(v uint32) { binary.Write(&b, le, v) }
	arr := func(a []uint32) {
		u32(uint32(len(a)))
		for _, v := range a {
			u32(v)
		}
	}
	b.Write(uexp[:t.pfAt])
	b.Write(fstrBytes(format))
	b.Write(uexp[t.pfAfter:t.perLayerAt])
	arr([]uint32{uint32(tileBytes)})
	u32(uint32(mips))
	u32(uint32(t.Width))
	u32(uint32(t.Height))
	arr(lay.chunkOfMip)
	arr(lay.mipBase)
	u32(uint32(mips))
	for _, tb := range tables {
		u32(uint32(tb.W))
		u32(uint32(tb.H))
		u32(tb.MaxAddr)
		arr(tb.Addr)
		arr(tb.Offs)
	}
	b.Write(uexp[t.legacyAt:t.layerPFAt])
	b.Write(fstrBytes(format))
	b.Write(uexp[t.FallbackAt : t.FallbackAt+16])
	u32(uint32(len(lay.chunks)))
	for i, c := range lay.chunks {
		ch := t.Chunks[c]
		b.Write(make([]byte, 20)) // checksum, written with the pixels
		u32(uint32(lay.sizes[i]))
		u32(ch.PayloadSize)
		b.Write(ch.codecRaw)
		u32(uint32(i))
	}
	chunkEnd := b.Len()
	b.Write(uexp[t.chunkEnd:expEnd])
	export := b.Bytes()
	binary.LittleEndian.PutUint64(export[skipAt:], uint64(chunkEnd-skipAt))
	nua, nexp, err = pkg.Write(PackageEdit{Names: names, Exports: [][]byte{export}, UExpTail: uexp[expEnd:], DataResources: newRs})
	if err != nil {
		return nil, nil, nil, nil, err
	}
	// 6. the .ubulk: chunk headers in place, tiles to come
	nbulk = make([]byte, total)
	for i, c := range lay.chunks {
		ch := t.Chunks[c]
		copy(nbulk[newRs[i].SerialOffset:], ubulk[ch.Start:ch.Start+int(ch.PayloadSize)])
	}
	if nt, err = ParseVTex(nexp); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("the re-laid texture does not read back: %v", err)
	}
	if err := nt.readChunks(nexp, len(nbulk), nil); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("the re-laid texture does not read back: %v", err)
	}
	if nt.Format != format || nt.Mips != mips || nt.TileBytes != tileBytes {
		return nil, nil, nil, nil, errors.New("the re-laid texture does not read back as written")
	}
	return nua, nexp, nbulk, nt, nil
}

// rebuildTexture writes img into the texture and returns its three files; with opts it is laid out
// anew first (the .uasset changes then too).
func rebuildTexture(t *VTex, ua, uexp, ubulk []byte, img *Img, opts TexOptions, progress func(done, total int)) ([]byte, []byte, []byte, error) {
	if !opts.changes() || (!opts.NoMips && t.Format == "PF_BC7") {
		ne, nb, err := t.Rebuild(uexp, ubulk, img, progress)
		return ua, ne, nb, err
	}
	// DXT1 keeps no transparency, so the game always read the colour texture as solid; BC7 would
	// keep any transparency in the picture, so make it solid as before
	solid := func() {
		if t.Format == "PF_DXT1" && opts.BC7 {
			for i := 3; i < len(img.Pix); i += 4 {
				img.Pix[i] = 255
			}
		}
	}
	if t.plain != nil {
		nua, nexp, nbulk, nt, err := reshapePlain(t.plain, ua, uexp, opts)
		if err != nil {
			return nil, nil, nil, err
		}
		solid()
		ne, nb, err := nt.Rebuild(nexp, nbulk, img, progress)
		return nua, ne, nb, err
	}
	nua, nexp, nbulk, nt, err := reshapeVT(t, ua, uexp, ubulk, opts)
	if err != nil {
		return nil, nil, nil, err
	}
	solid()
	if err := nt.rebuildInto(nexp, nbulk, img, progress); err != nil {
		return nil, nil, nil, err
	}
	return nua, nexp, nbulk, nil
}

// reshapePlain lays an ordinary texture out for opts: as BC7 every size takes twice the bytes, each
// kept where it was (.ubulk or .uexp); with no mipmaps only the full size is kept, inside the .uexp,
// the way the engine keeps a texture that has a single size (so it is never streamed). It returns
// the new files, the pixels still to be written, and the texture they describe.
func reshapePlain(tx *Tex2D, ua, uexp []byte, opts TexOptions) (nua, nexp, nbulk []byte, nt *Tex2D, err error) {
	fail := func(what string) ([]byte, []byte, []byte, *Tex2D, error) {
		return nil, nil, nil, nil, errors.New("the texture's description is not laid out as expected (" + what + ")")
	}
	pkg, err := ParsePackage(ua, uexp)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if len(pkg.Exports) != 1 || pkg.Exports[0].SerialOffset != int64(pkg.totalHeader) {
		return fail("exports")
	}
	expEnd := int(pkg.Exports[0].SerialSize)
	le := binary.LittleEndian
	format := tx.Format
	if opts.BC7 && format == "PF_DXT1" {
		format = "PF_BC7"
	}
	skipAt := tx.pfAt - 36
	if skipAt < 8 || tx.listEnd > expEnd {
		return fail("skip offset")
	}
	skip := int(int64(le.Uint64(uexp[skipAt:])))
	if end := skipAt + skip; end < tx.listEnd || end > expEnd {
		return fail("skip offset")
	}
	names := append([]string(nil), pkg.Names...)
	if format != tx.Format {
		fi := int(le.Uint32(uexp[skipAt-8:]))
		if fi < 0 || fi >= len(names) || names[fi] != tx.Format || le.Uint32(uexp[skipAt-4:]) != 0 {
			return fail("format name")
		}
		for i, n := range names {
			if n == format && i != fi {
				return nil, nil, nil, nil, fmt.Errorf("the package already has a name %s", format)
			}
		}
		names[fi] = format
	}
	rs, err := pkg.DataResources()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	used := map[int]bool{}
	inline := DataResource{DuplicateSerialOffset: -1, LegacyFlags: 0x48} // inline payload, used once
	for _, m := range tx.Mips {
		if m.Res < 0 || m.Res >= len(rs) || used[m.Res] {
			return fail("data resources")
		}
		used[m.Res] = true
		if !m.InBulk {
			inline = rs[m.Res]
		}
	}
	if len(used) != len(rs) {
		return nil, nil, nil, nil, errors.New("the texture package holds other data besides its pixels")
	}
	mips := tx.Mips
	if opts.NoMips {
		mips = mips[:1]
	}
	var b bytes.Buffer
	u32 := func(v uint32) { binary.Write(&b, le, v) }
	b.Write(uexp[:tx.pfAt])
	b.Write(fstrBytes(format))
	b.Write(uexp[tx.pfAfter : tx.pfAfter+4]) // first size kept in the file
	u32(uint32(len(mips)))
	var newRs []DataResource
	bulk := 0
	for i, m := range mips {
		size := texBytes(format, m.W, m.H)
		r := rs[m.Res]
		u32(uint32(i))
		if m.InBulk && !opts.NoMips {
			r.SerialOffset = int64(bulk)
			bulk += size
		} else {
			if m.InBulk {
				r = inline
			}
			r.SerialOffset = int64(b.Len())
			b.Write(make([]byte, size))
		}
		r.SerialSize, r.RawSize = int64(size), int64(size)
		newRs = append(newRs, r)
		dims := m.recAt + 4 // width, height and depth follow the pixels (or the index, when they are in the .ubulk)
		if !m.InBulk {
			dims = m.Off + m.Size
		}
		b.Write(uexp[dims : dims+12])
	}
	listEnd := b.Len()
	b.Write(uexp[tx.listEnd:expEnd])
	export := b.Bytes()
	le.PutUint64(export[skipAt:], uint64(int64(skip+listEnd-tx.listEnd)))
	nua, nexp, err = pkg.Write(PackageEdit{Names: names, Exports: [][]byte{export}, UExpTail: uexp[expEnd:], DataResources: newRs})
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if bulk > 0 {
		nbulk = make([]byte, bulk)
	}
	np, err := ParsePackage(nua, nexp)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("the re-laid texture does not read back: %v", err)
	}
	if nt, err = ParseTex2D(np); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("the re-laid texture does not read back: %v", err)
	}
	if nt.Format != format || len(nt.Mips) != len(mips) || nt.W() != tx.W() {
		return nil, nil, nil, nil, errors.New("the re-laid texture does not read back as written")
	}
	nt.SRGB = tx.SRGB
	return nua, nexp, nbulk, nt, nil
}
