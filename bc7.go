package main

// BC7 block compression, the format of the aircraft normal maps, the icons and the radio
// portraits.
// The decoder handles every mode and follows the reference decoder in Pillow
// (src/libImaging/BcnDecode.c, HPND licence). The encoder writes mode 6, mode 5 or 4 for blocks
// whose transparency varies when one of those is closer, and modes 0 to 3 for blocks with no
// transparency whose colours need more than one line (bc7part.go).

import "math"

type bc7ModeInfo struct{ ns, pb, rb, isb, cb, ab, epb, spb, ib, ib2 int }

var bc7Modes = [8]bc7ModeInfo{
	{3, 4, 0, 0, 4, 0, 1, 0, 3, 0},
	{2, 6, 0, 0, 6, 0, 0, 1, 3, 0},
	{3, 6, 0, 0, 5, 0, 0, 0, 2, 0},
	{2, 6, 0, 0, 7, 0, 1, 0, 2, 0},
	{1, 0, 2, 1, 5, 6, 0, 0, 2, 3},
	{1, 0, 2, 0, 7, 8, 0, 0, 2, 2},
	{1, 0, 0, 0, 7, 7, 1, 0, 4, 0},
	{2, 6, 0, 0, 5, 5, 1, 0, 2, 0},
}

var bc7si2 = [64]uint16{
	0xcccc, 0x8888, 0xeeee, 0xecc8, 0xc880, 0xfeec, 0xfec8, 0xec80, 0xc800, 0xffec,
	0xfe80, 0xe800, 0xffe8, 0xff00, 0xfff0, 0xf000, 0xf710, 0x008e, 0x7100, 0x08ce,
	0x008c, 0x7310, 0x3100, 0x8cce, 0x088c, 0x3110, 0x6666, 0x366c, 0x17e8, 0x0ff0,
	0x718e, 0x399c, 0xaaaa, 0xf0f0, 0x5a5a, 0x33cc, 0x3c3c, 0x55aa, 0x9696, 0xa55a,
	0x73ce, 0x13c8, 0x324c, 0x3bdc, 0x6996, 0xc33c, 0x9966, 0x0660, 0x0272, 0x04e4,
	0x4e40, 0x2720, 0xc936, 0x936c, 0x39c6, 0x639c, 0x9336, 0x9cc6, 0x817e, 0xe718,
	0xccf0, 0x0fcc, 0x7744, 0xee22,
}

var bc7si3 = [64]uint32{
	0xaa685050, 0x6a5a5040, 0x5a5a4200, 0x5450a0a8, 0xa5a50000, 0xa0a05050, 0x5555a0a0,
	0x5a5a5050, 0xaa550000, 0xaa555500, 0xaaaa5500, 0x90909090, 0x94949494, 0xa4a4a4a4,
	0xa9a59450, 0x2a0a4250, 0xa5945040, 0x0a425054, 0xa5a5a500, 0x55a0a0a0, 0xa8a85454,
	0x6a6a4040, 0xa4a45000, 0x1a1a0500, 0x0050a4a4, 0xaaa59090, 0x14696914, 0x69691400,
	0xa08585a0, 0xaa821414, 0x50a4a450, 0x6a5a0200, 0xa9a58000, 0x5090a0a8, 0xa8a09050,
	0x24242424, 0x00aa5500, 0x24924924, 0x24499224, 0x50a50a50, 0x500aa550, 0xaaaa4444,
	0x66660000, 0xa5a0a5a0, 0x50a050a0, 0x69286928, 0x44aaaa44, 0x66666600, 0xaa444444,
	0x54a854a8, 0x95809580, 0x96969600, 0xa85454a8, 0x80959580, 0xaa141414, 0x96960000,
	0xaaaa1414, 0xa05050a0, 0xa0a5a5a0, 0x96000000, 0x40804080, 0xa9a8a9a8, 0xaaaaaa44,
	0x2a4a5254,
}

var bc7ai0 = [64]int{15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15, 15,
	15, 15, 15, 15, 2, 8, 2, 2, 8, 8, 15, 2, 8,
	2, 2, 8, 8, 2, 2, 15, 15, 6, 8, 2, 8, 15,
	15, 2, 8, 2, 2, 2, 15, 15, 6, 6, 2, 6, 8,
	15, 15, 2, 2, 15, 15, 15, 15, 15, 2, 2, 15}

var bc7ai1 = [64]int{3, 3, 15, 15, 8, 3, 15, 15, 8, 8, 6, 6, 6,
	5, 3, 3, 3, 3, 8, 15, 3, 3, 6, 10, 5, 8,
	8, 6, 8, 5, 15, 15, 8, 15, 3, 5, 6, 10, 8,
	15, 15, 3, 15, 5, 15, 15, 15, 15, 3, 15, 5, 5,
	5, 8, 5, 10, 5, 10, 8, 13, 15, 12, 3, 3}

var bc7ai2 = [64]int{15, 8, 8, 3, 15, 15, 3, 8, 15, 15, 15, 15, 15,
	15, 15, 8, 15, 8, 15, 3, 15, 8, 15, 8, 3, 15,
	6, 10, 15, 15, 10, 8, 15, 3, 15, 10, 10, 8, 9,
	10, 6, 15, 8, 15, 3, 6, 6, 8, 15, 3, 15, 15,
	15, 15, 15, 15, 15, 15, 15, 15, 3, 15, 15, 8}

var bc7w2 = []int{0, 21, 43, 64}
var bc7w3 = []int{0, 9, 18, 27, 37, 46, 55, 64}
var bc7w4 = []int{0, 4, 9, 13, 17, 21, 26, 30, 34, 38, 43, 47, 51, 55, 60, 64}

func bc7Weights(n int) []int {
	switch n {
	case 2:
		return bc7w2
	case 3:
		return bc7w3
	}
	return bc7w4
}

func bc7Bits(src []byte, bit, count int) int {
	if count == 0 {
		return 0
	}
	by := bit >> 3
	bit &= 7
	x := int(src[by])
	if by+1 < len(src) {
		x |= int(src[by+1]) << 8
	}
	return (x >> uint(bit)) & ((1 << uint(count)) - 1)
}

func bc7Expand(v, bits int) int {
	v = (v << uint(8-bits)) & 0xff
	return v | v>>uint(bits)
}

// decodeBC7Block writes 16 RGBA pixels (4x4, row by row) into dst with the given row stride.
func decodeBC7Block(src []byte, dst []uint8, stride int) {
	put := func(i, r, g, b, a int) {
		o := (i/4)*stride + (i%4)*4
		dst[o], dst[o+1], dst[o+2], dst[o+3] = uint8(r), uint8(g), uint8(b), uint8(a)
	}
	if src[0] == 0 {
		for i := 0; i < 16; i++ {
			put(i, 0, 0, 0, 255)
		}
		return
	}
	mode := 0
	for src[0]&(1<<uint(mode)) == 0 {
		mode++
	}
	bit := mode + 1
	info := bc7Modes[mode]
	cb, ab := info.cb, info.ab
	cw := bc7Weights(info.ib)
	aw := cw
	if ab != 0 && info.ib2 != 0 {
		aw = bc7Weights(info.ib2)
	}
	load := func(n int) int {
		v := bc7Bits(src, bit, n)
		bit += n
		return v
	}
	partition := load(info.pb)
	rotation := load(info.rb)
	indexSel := load(info.isb)
	numep := info.ns * 2
	var ep [6][4]int
	for c := 0; c < 3; c++ {
		for i := 0; i < numep; i++ {
			ep[i][c] = load(cb)
		}
	}
	for i := 0; i < numep; i++ {
		if ab != 0 {
			ep[i][3] = load(ab)
		} else {
			ep[i][3] = 255
		}
	}
	if info.epb != 0 {
		cb++
		if ab != 0 {
			ab++
		}
		for i := 0; i < numep; i++ {
			p := load(1)
			for c := 0; c < 3; c++ {
				ep[i][c] = ep[i][c]<<1 | p
			}
			if ab != 0 {
				ep[i][3] = ep[i][3]<<1 | p
			}
		}
	}
	if info.spb != 0 {
		cb++
		if ab != 0 {
			ab++
		}
		for i := 0; i < numep; i += 2 {
			p := load(1)
			for j := 0; j < 2; j++ {
				for c := 0; c < 3; c++ {
					ep[i+j][c] = ep[i+j][c]<<1 | p
				}
				if ab != 0 {
					ep[i+j][3] = ep[i+j][3]<<1 | p
				}
			}
		}
	}
	for i := 0; i < numep; i++ {
		for c := 0; c < 3; c++ {
			ep[i][c] = bc7Expand(ep[i][c], cb)
		}
		if ab != 0 {
			ep[i][3] = bc7Expand(ep[i][3], ab)
		}
	}
	cibit := bit
	aibit := cibit + 16*info.ib - info.ns
	for i := 0; i < 16; i++ {
		s := 0
		switch info.ns {
		case 2:
			s = int(bc7si2[partition]>>uint(i)) & 1
		case 3:
			s = int(bc7si3[partition]>>uint(2*i)) & 3
		}
		s *= 2
		ib := info.ib
		if i == 0 {
			ib--
		} else if info.ns == 2 {
			if i == bc7ai0[partition] {
				ib--
			}
		} else if info.ns == 3 {
			if i == bc7ai1[partition] || i == bc7ai2[partition] {
				ib--
			}
		}
		i0 := bc7Bits(src, cibit, ib)
		cibit += ib
		s0, s1 := cw[i0], cw[i0]
		if ab != 0 && info.ib2 != 0 {
			ib2 := info.ib2
			if i == 0 {
				ib2--
			}
			i1 := bc7Bits(src, aibit, ib2)
			aibit += ib2
			if indexSel != 0 {
				s0, s1 = aw[i1], cw[i0]
			} else {
				s0, s1 = cw[i0], aw[i1]
			}
		}
		t0, t1 := 64-s0, 64-s1
		r := (t0*ep[s][0] + s0*ep[s+1][0] + 32) >> 6
		g := (t0*ep[s][1] + s0*ep[s+1][1] + 32) >> 6
		b := (t0*ep[s][2] + s0*ep[s+1][2] + 32) >> 6
		a := (t1*ep[s][3] + s1*ep[s+1][3] + 32) >> 6
		switch rotation {
		case 1:
			r, a = a, r
		case 2:
			g, a = a, g
		case 3:
			b, a = a, b
		}
		put(i, r, g, b, a)
	}
}

// encodeBC7Block compresses 16 RGBA pixels into 16 bytes using mode 6
// (one colour line per block, 7-bit end points plus a shared low bit, sixteen steps).
func encodeBC7Block(src []uint8, stride int, dst []byte) {
	encodeBC7Mode6(src, stride, dst)
	// Where a block is partly see-through (text, soft edges), modes 5 and 4 keep colour and
	// transparency apart, so a white letter fading into a transparent background does not need
	// one line through both. Whichever comes out closer is kept. Blocks that are not see-through,
	// such as the aircraft normal maps, always use mode 6.
	amin := 255
	for i := 0; i < 16; i++ {
		amin = min(amin, int(src[(i/4)*stride+(i%4)*4+3]))
	}
	if amin == 255 {
		// not see-through at all: an edge between colours may fit better in two or three parts
		encodeBC7Opaque(src, stride, dst)
		return
	}
	if amin >= 250 {
		return
	}
	best := bc7Error(src, stride, dst)
	var alt [16]byte
	for _, mode := range []int{5, 4} {
		encodeBC7Split(src, stride, alt[:], mode)
		if e := bc7Error(src, stride, alt[:]); e < best {
			best = e
			copy(dst[:16], alt[:])
		}
	}
}

// bc7Error is how far a block's decoded pixels are from the source: colour counts as much as it
// can be seen (by its transparency), transparency always counts in full.
func bc7Error(src []uint8, stride int, blk []byte) float64 {
	var out [64]uint8
	decodeBC7Block(blk, out[:], 16)
	e := 0.0
	for i := 0; i < 16; i++ {
		o := (i/4)*stride + (i%4)*4
		w := float64(src[o+3])/255 + 0.001
		for c := 0; c < 3; c++ {
			d := float64(src[o+c]) - float64(out[i*4+c])
			e += w * d * d
		}
		d := float64(src[o+3]) - float64(out[i*4+3])
		e += d * d
	}
	return e
}

// encodeBC7Split writes one block in mode 5 or 4, where colour and transparency each have a line
// of their own. Mode 5: 7-bit colour and 8-bit transparency end points, 2-bit indices for both.
// Mode 4: 5-bit colour and 6-bit transparency end points, 2-bit colour and 3-bit transparency
// indices, for blocks with more steps of transparency than of colour.
func encodeBC7Split(src []uint8, stride int, dst []byte, mode int) {
	cbits, abits, aw := 7, 8, bc7w2
	if mode == 4 {
		cbits, abits, aw = 5, 6, bc7w3
	}
	expand := func(q, n int) int { return q<<uint(8-n) | q>>uint(2*n-8) }
	quant := func(x float64, n int) int {
		top := 1<<uint(n) - 1
		q := int(math.Max(0, math.Min(float64(top), math.Round(x*float64(top)/255))))
		best := q
		for _, c := range []int{q - 1, q + 1} {
			if c >= 0 && c <= top && math.Abs(float64(expand(c, n))-x) < math.Abs(float64(expand(best, n))-x) {
				best = c
			}
		}
		return best
	}
	var px [16][4]float64
	var w [16]float64
	for i := 0; i < 16; i++ {
		o := (i/4)*stride + (i%4)*4
		for c := 0; c < 4; c++ {
			px[i][c] = float64(src[o+c])
		}
		w[i] = px[i][3]/255 + 0.02
	}
	// colour: the main direction of the visible pixels
	var mean [3]float64
	sw := 0.0
	for i := 0; i < 16; i++ {
		for c := 0; c < 3; c++ {
			mean[c] += w[i] * px[i][c]
		}
		sw += w[i]
	}
	for c := 0; c < 3; c++ {
		mean[c] /= sw
	}
	var cov [3][3]float64
	for i := 0; i < 16; i++ {
		for a := 0; a < 3; a++ {
			for b := 0; b < 3; b++ {
				cov[a][b] += w[i] * (px[i][a] - mean[a]) * (px[i][b] - mean[b])
			}
		}
	}
	v := [3]float64{1, 1, 1}
	for it := 0; it < 8; it++ {
		var n [3]float64
		l := 0.0
		for a := 0; a < 3; a++ {
			for b := 0; b < 3; b++ {
				n[a] += cov[a][b] * v[b]
			}
			l += n[a] * n[a]
		}
		l = math.Sqrt(l)
		if l < 1e-9 {
			break
		}
		for a := 0; a < 3; a++ {
			v[a] = n[a] / l
		}
	}
	vl := math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2])
	tmin, tmax := math.Inf(1), math.Inf(-1)
	for i := 0; i < 16; i++ {
		if px[i][3] == 0 && sw > 0.33 { // fully transparent pixels do not stretch the line
			continue
		}
		t := 0.0
		for c := 0; c < 3; c++ {
			t += (px[i][c] - mean[c]) * v[c] / vl
		}
		tmin, tmax = math.Min(tmin, t), math.Max(tmax, t)
	}
	var e0, e1 [3]float64
	for c := 0; c < 3; c++ {
		e0[c], e1[c] = mean[c]+v[c]/vl*tmin, mean[c]+v[c]/vl*tmax
	}
	type colRes struct {
		q0, q1 [3]int
		idx    [16]int
		err    float64
	}
	evalCol := func(e0, e1 [3]float64) colRes {
		var r colRes
		for c := 0; c < 3; c++ {
			r.q0[c], r.q1[c] = quant(e0[c], cbits), quant(e1[c], cbits)
		}
		for i := 0; i < 16; i++ {
			best := math.Inf(1)
			for k := 0; k < 4; k++ {
				e := 0.0
				for c := 0; c < 3; c++ {
					a, b := expand(r.q0[c], cbits), expand(r.q1[c], cbits)
					d := px[i][c] - float64(((64-bc7w2[k])*a+bc7w2[k]*b+32)>>6)
					e += d * d
				}
				if e < best {
					best, r.idx[i] = e, k
				}
			}
			r.err += w[i] * best
		}
		return r
	}
	col := evalCol(e0, e1)
	for pass := 0; pass < 2; pass++ {
		var a2, b2, ab float64
		var at, bt [3]float64
		for i := 0; i < 16; i++ {
			wb := float64(bc7w2[col.idx[i]]) / 64
			wa := 1 - wb
			a2 += w[i] * wa * wa
			b2 += w[i] * wb * wb
			ab += w[i] * wa * wb
			for c := 0; c < 3; c++ {
				at[c] += w[i] * wa * px[i][c]
				bt[c] += w[i] * wb * px[i][c]
			}
		}
		det := a2*b2 - ab*ab
		if math.Abs(det) < 1e-9 {
			break
		}
		var n0, n1 [3]float64
		for c := 0; c < 3; c++ {
			n0[c] = (at[c]*b2 - bt[c]*ab) / det
			n1[c] = (bt[c]*a2 - at[c]*ab) / det
		}
		r := evalCol(n0, n1)
		if r.err >= col.err {
			break
		}
		col = r
	}
	// transparency: its own line
	type alRes struct {
		a0, a1 int
		idx    [16]int
		err    float64
	}
	evalA := func(a0, a1 float64) alRes {
		r := alRes{a0: quant(a0, abits), a1: quant(a1, abits)}
		x0, x1 := expand(r.a0, abits), expand(r.a1, abits)
		for i := 0; i < 16; i++ {
			best := math.Inf(1)
			for k := range aw {
				d := px[i][3] - float64(((64-aw[k])*x0+aw[k]*x1+32)>>6)
				if d*d < best {
					best, r.idx[i] = d*d, k
				}
			}
			r.err += best
		}
		return r
	}
	amin, amax := 255.0, 0.0
	for i := 0; i < 16; i++ {
		amin, amax = math.Min(amin, px[i][3]), math.Max(amax, px[i][3])
	}
	al := evalA(amin, amax)
	for pass := 0; pass < 2; pass++ {
		var a2, b2, ab, at, bt float64
		for i := 0; i < 16; i++ {
			wb := float64(aw[al.idx[i]]) / 64
			wa := 1 - wb
			a2, b2, ab = a2+wa*wa, b2+wb*wb, ab+wa*wb
			at, bt = at+wa*px[i][3], bt+wb*px[i][3]
		}
		det := a2*b2 - ab*ab
		if math.Abs(det) < 1e-9 {
			break
		}
		r := evalA((at*b2-bt*ab)/det, (bt*a2-at*ab)/det)
		if r.err >= al.err {
			break
		}
		al = r
	}
	// the first index of each set is stored with one bit fewer
	if col.idx[0] >= 2 {
		col.q0, col.q1 = col.q1, col.q0
		for i := range col.idx {
			col.idx[i] = 3 - col.idx[i]
		}
	}
	if al.idx[0] >= len(aw)/2 {
		al.a0, al.a1 = al.a1, al.a0
		for i := range al.idx {
			al.idx[i] = len(aw) - 1 - al.idx[i]
		}
	}
	for i := range dst[:16] {
		dst[i] = 0
	}
	bit := 0
	put := func(v, n int) {
		for k := 0; k < n; k++ {
			if v>>uint(k)&1 != 0 {
				dst[bit>>3] |= 1 << uint(bit&7)
			}
			bit++
		}
	}
	put(1<<uint(mode), mode+1)
	put(0, 2) // no rotation
	if mode == 4 {
		put(0, 1) // the 2-bit indices are for colour
	}
	for c := 0; c < 3; c++ {
		put(col.q0[c], cbits)
		put(col.q1[c], cbits)
	}
	put(al.a0, abits)
	put(al.a1, abits)
	put(col.idx[0], 1)
	for i := 1; i < 16; i++ {
		put(col.idx[i], 2)
	}
	aib := 2
	if len(aw) == 8 {
		aib = 3
	}
	put(al.idx[0], aib-1)
	for i := 1; i < 16; i++ {
		put(al.idx[i], aib)
	}
}

// encodeBC7Mode6 writes one block in mode 6: one RGBA line with 4-bit indices.
func encodeBC7Mode6(src []uint8, stride int, dst []byte) {
	var px [16][4]float64
	var mean [4]float64
	for i := 0; i < 16; i++ {
		o := (i/4)*stride + (i%4)*4
		for c := 0; c < 4; c++ {
			px[i][c] = float64(src[o+c])
			mean[c] += px[i][c]
		}
	}
	for c := 0; c < 4; c++ {
		mean[c] /= 16
	}
	var cov [4][4]float64
	var lo, hi [4]float64
	for c := 0; c < 4; c++ {
		lo[c], hi[c] = 255, 0
	}
	for i := 0; i < 16; i++ {
		var d [4]float64
		for c := 0; c < 4; c++ {
			d[c] = px[i][c] - mean[c]
			lo[c], hi[c] = math.Min(lo[c], px[i][c]), math.Max(hi[c], px[i][c])
		}
		for a := 0; a < 4; a++ {
			for b := 0; b < 4; b++ {
				cov[a][b] += d[a] * d[b]
			}
		}
	}
	v := [4]float64{hi[0] - lo[0] + 1e-3, hi[1] - lo[1] + 1e-3, hi[2] - lo[2] + 1e-3, hi[3] - lo[3] + 1e-3}
	for it := 0; it < 8; it++ {
		var n [4]float64
		l := 0.0
		for a := 0; a < 4; a++ {
			for b := 0; b < 4; b++ {
				n[a] += cov[a][b] * v[b]
			}
			l += n[a] * n[a]
		}
		l = math.Sqrt(l)
		if l < 1e-9 {
			break
		}
		for a := 0; a < 4; a++ {
			v[a] = n[a] / l
		}
	}
	l := 0.0
	for a := 0; a < 4; a++ {
		l += v[a] * v[a]
	}
	l = math.Sqrt(l)
	tmin, tmax := math.Inf(1), math.Inf(-1)
	for i := 0; i < 16; i++ {
		t := 0.0
		for c := 0; c < 4; c++ {
			t += (px[i][c] - mean[c]) * v[c] / l
		}
		tmin, tmax = math.Min(tmin, t), math.Max(tmax, t)
	}
	var e0, e1 [4]float64
	for c := 0; c < 4; c++ {
		e0[c] = mean[c] + v[c]/l*tmin
		e1[c] = mean[c] + v[c]/l*tmax
	}

	// quantise an end point to 7 bits per channel plus one shared low bit
	quant := func(e [4]float64) (q [4]int, p int) {
		bestErr := math.Inf(1)
		for pb := 0; pb < 2; pb++ {
			var cand [4]int
			err := 0.0
			for c := 0; c < 4; c++ {
				x := math.Round((e[c] - float64(pb)) / 2)
				if x < 0 {
					x = 0
				}
				if x > 127 {
					x = 127
				}
				cand[c] = int(x)
				d := e[c] - float64(int(x)*2+pb)
				err += d * d
			}
			if err < bestErr {
				bestErr, q, p = err, cand, pb
			}
		}
		return
	}
	type result struct {
		q0, q1 [4]int
		p0, p1 int
		idx    [16]int
		err    float64
	}
	evaluate := func(e0, e1 [4]float64) result {
		var r result
		r.q0, r.p0 = quant(e0)
		r.q1, r.p1 = quant(e1)
		var pal [16][4]int
		for k := 0; k < 16; k++ {
			w := bc7w4[k]
			for c := 0; c < 4; c++ {
				a, b := r.q0[c]*2+r.p0, r.q1[c]*2+r.p1
				pal[k][c] = ((64-w)*a + w*b + 32) >> 6
			}
		}
		for i := 0; i < 16; i++ {
			best := math.Inf(1)
			for k := 0; k < 16; k++ {
				e := 0.0
				for c := 0; c < 4; c++ {
					d := px[i][c] - float64(pal[k][c])
					e += d * d
				}
				if e < best {
					best, r.idx[i] = e, k
				}
			}
			r.err += best
		}
		return r
	}
	best := evaluate(e0, e1)
	for pass := 0; pass < 2; pass++ { // least-squares refinement of the end points
		var a2, b2, ab float64
		var at, bt [4]float64
		for i := 0; i < 16; i++ {
			wb := float64(bc7w4[best.idx[i]]) / 64
			wa := 1 - wb
			a2 += wa * wa
			b2 += wb * wb
			ab += wa * wb
			for c := 0; c < 4; c++ {
				at[c] += wa * px[i][c]
				bt[c] += wb * px[i][c]
			}
		}
		det := a2*b2 - ab*ab
		if math.Abs(det) < 1e-6 {
			break
		}
		var n0, n1 [4]float64
		for c := 0; c < 4; c++ {
			n0[c] = (at[c]*b2 - bt[c]*ab) / det
			n1[c] = (bt[c]*a2 - at[c]*ab) / det
		}
		r := evaluate(n0, n1)
		if r.err >= best.err {
			break
		}
		best = r
	}
	if best.idx[0] >= 8 { // the first index is stored with one bit fewer, so it must be below 8
		best.q0, best.q1 = best.q1, best.q0
		best.p0, best.p1 = best.p1, best.p0
		for i := range best.idx {
			best.idx[i] = 15 - best.idx[i]
		}
	}
	for i := range dst[:16] {
		dst[i] = 0
	}
	bit := 0
	put := func(v, n int) {
		for k := 0; k < n; k++ {
			if v>>uint(k)&1 != 0 {
				dst[bit>>3] |= 1 << uint(bit&7)
			}
			bit++
		}
	}
	put(1<<6, 7)
	for c := 0; c < 4; c++ {
		put(best.q0[c], 7)
		put(best.q1[c], 7)
	}
	put(best.p0, 1)
	put(best.p1, 1)
	put(best.idx[0], 3)
	for i := 1; i < 16; i++ {
		put(best.idx[i], 4)
	}
}
