package main

// BC7 modes 0 to 3: a block with no transparency split into two or three parts, each with a colour
// line of its own. Mode 6 gives a whole block one line, which suits smooth paint; where a block holds
// an edge between colours that do not lie on one line (a red stripe with a white border on grey, the
// edge of a letter), one of these modes keeps each colour clean. Which pixels go in which part is one
// of the 64 patterns BC7 defines (16 for mode 0).
//
// Trying every pattern in full would be slow, so each pattern is first scored by how well a straight
// line fits each of its parts (the spread across the line, worked out from sums), and only the best
// few are encoded and compared.

import "math"

type bc7PartMode struct {
	mode, ns, pb, cb int // mode number, parts, pattern bits, colour bits per end point (before p-bits)
	pbit             int // 0: none, 1: one low bit per end point, 2: one shared by both ends of a part
	ib               int // index bits
}

var (
	bc7M0 = bc7PartMode{0, 3, 4, 4, 1, 3}
	bc7M1 = bc7PartMode{1, 2, 6, 6, 2, 3}
	bc7M2 = bc7PartMode{2, 3, 6, 5, 0, 2}
	bc7M3 = bc7PartMode{3, 2, 6, 7, 1, 2}
)

const (
	// A block that mode 6 gets this close (summed squared error over its 48 values: about 1.6 steps
	// per value) is left as it is: the extra modes could not make a difference anyone would see.
	bc7SearchFloor = 120.0
	bc7TryTwo      = 3 // two-part patterns encoded in full
	bc7TryThree    = 2 // three-part patterns encoded in full
	// Splitting is tried only when the best pattern leaves at most this share of the spread off
	// its lines.
	bc7SplitGain = 0.5
)

// bc7Patterns lists, for each pattern, the pixels of each part (worked out once).
type bc7Pattern struct {
	members [3][16]uint8
	n       [3]int
}

var bc7Pat2, bc7Pat3 [64]bc7Pattern

func init() {
	for p := 0; p < 64; p++ {
		for i := 0; i < 16; i++ {
			s := int(bc7si2[p]>>uint(i)) & 1
			bc7Pat2[p].members[s][bc7Pat2[p].n[s]] = uint8(i)
			bc7Pat2[p].n[s]++
			s = int(bc7si3[p]>>uint(2*i)) & 3
			bc7Pat3[p].members[s][bc7Pat3[p].n[s]] = uint8(i)
			bc7Pat3[p].n[s]++
		}
	}
}

func bc7PatternOf(ns, p int) *bc7Pattern {
	if ns == 2 {
		return &bc7Pat2[p]
	}
	return &bc7Pat3[p]
}

// bc7Anchor is the pixel whose index is stored with one bit fewer in part s.
func bc7Anchor(ns, p, s int) int {
	switch {
	case s == 0:
		return 0
	case ns == 2:
		return bc7ai0[p]
	case s == 1:
		return bc7ai1[p]
	}
	return bc7ai2[p]
}

// encodeBC7Opaque tries the two- and three-part modes for a block with no transparency. dst holds the
// mode 6 block on entry and the closest of all on return.
func encodeBC7Opaque(src []uint8, stride int, dst []byte) {
	best := bc7Error(src, stride, dst)
	if best <= bc7SearchFloor {
		return
	}
	var px [16][3]float64
	for i := 0; i < 16; i++ {
		o := (i/4)*stride + (i%4)*4
		px[i] = [3]float64{float64(src[o]), float64(src[o+1]), float64(src[o+2])}
	}
	var cand [16]byte
	try := func(m bc7PartMode, p int) {
		if e := m.encode(&px, p, cand[:], best); e < best {
			best = e
			copy(dst[:16], cand[:])
		}
	}
	// Only where splitting the block takes most of the spread off the line is it worth encoding
	// (an edge between colours); in grain or noise every pattern fits about as badly as one line.
	var top [bc7TryTwo]int
	two, spread2, whole := bc7BestPatterns(&px, 2, 64, top[:])
	if spread2 > bc7SplitGain*whole {
		return
	}
	for _, p := range two {
		try(bc7M1, p)
		try(bc7M3, p)
	}
	if best <= bc7SearchFloor {
		return
	}
	var top3 [bc7TryThree]int
	three, spread3, _ := bc7BestPatterns(&px, 3, 64, top3[:])
	if spread3 > bc7SplitGain*spread2 {
		return
	}
	for _, p := range three {
		try(bc7M2, p)
		if p < 16 {
			try(bc7M0, p)
		}
	}
}

// bc7BestPatterns scores the first n patterns for ns parts by how far each part's pixels lie from
// the best straight line through them, and fills out with the ones that fit best. It also gives the
// best pattern's spread and that of the whole block on one line.
func bc7BestPatterns(px *[16][3]float64, ns, n int, out []int) ([]int, float64, float64) {
	// sums of each pixel's values and products, so a part's spread is a few additions per pixel
	var all [9]float64
	var val [16][9]float64
	for i := range px {
		r, g, b := px[i][0], px[i][1], px[i][2]
		val[i] = [9]float64{r, g, b, r * r, r * g, r * b, g * g, g * b, b * b}
		for c := 0; c < 9; c++ {
			all[c] += val[i][c]
		}
	}
	spread := func(v *[9]float64, cnt float64) float64 {
		if cnt < 2 {
			return 0
		}
		mr, mg, mb := v[0]/cnt, v[1]/cnt, v[2]/cnt
		cov := [3][3]float64{
			{v[3] - cnt*mr*mr, v[4] - cnt*mr*mg, v[5] - cnt*mr*mb},
			{0, v[6] - cnt*mg*mg, v[7] - cnt*mg*mb},
			{0, 0, v[8] - cnt*mb*mb},
		}
		cov[1][0], cov[2][0], cov[2][1] = cov[0][1], cov[0][2], cov[1][2]
		return cov[0][0] + cov[1][1] + cov[2][2] - bc7LargestSpread(&cov)
	}
	whole := spread(&all, 16)
	var score [4]float64
	k := 0
	for p := 0; p < n; p++ {
		pat := bc7PatternOf(ns, p)
		var sums [3][9]float64
		sums[0] = all
		for s := 1; s < ns; s++ {
			for _, i := range pat.members[s][:pat.n[s]] {
				for c := 0; c < 9; c++ {
					sums[s][c] += val[i][c]
				}
			}
			for c := 0; c < 9; c++ {
				sums[0][c] -= sums[s][c]
			}
		}
		e := 0.0
		for s := 0; s < ns; s++ {
			e += spread(&sums[s], float64(pat.n[s]))
		}
		// keep the len(out) lowest, in order
		at := k
		for at > 0 && score[at-1] > e {
			at--
		}
		if at < len(out) {
			if k < len(out) {
				k++
			}
			copy(score[at+1:k], score[at:k-1])
			copy(out[at+1:k], out[at:k-1])
			score[at], out[at] = e, p
		}
	}
	return out[:k], score[0], whole
}

// bc7LargestSpread is the largest eigenvalue of a 3x3 covariance (the spread along the best line).
func bc7LargestSpread(cov *[3][3]float64) float64 {
	// start from the column with the most spread, so the start is never at right angles to the answer
	k := 0
	for c := 1; c < 3; c++ {
		if cov[c][c] > cov[k][k] {
			k = c
		}
	}
	v := [3]float64{cov[0][k], cov[1][k], cov[2][k]}
	l := 0.0
	for it := 0; it < 3; it++ {
		var n [3]float64
		for a := 0; a < 3; a++ {
			n[a] = cov[a][0]*v[0] + cov[a][1]*v[1] + cov[a][2]*v[2]
		}
		l = math.Sqrt(n[0]*n[0] + n[1]*n[1] + n[2]*n[2])
		if l < 1e-9 {
			return 0
		}
		v = [3]float64{n[0] / l, n[1] / l, n[2] / l}
	}
	return l
}

// effective bits of an end point value (with its p-bit)
func (m bc7PartMode) eb() int {
	if m.pbit != 0 {
		return m.cb + 1
	}
	return m.cb
}

// quant finds the stored value (q, with p as its low bit when the mode has p-bits) nearest e.
func (m bc7PartMode) quant(e float64, p int) int {
	eb := m.eb()
	top := 1<<uint(eb) - 1
	x := e / 255 * float64(top)
	var q0 int
	if m.pbit != 0 {
		q0 = int(math.Round((x - float64(p)) / 2))
	} else {
		q0 = int(math.Round(x))
	}
	qmax := 1<<uint(m.cb) - 1
	best, bestD := 0, math.Inf(1)
	for q := q0 - 1; q <= q0+1; q++ {
		if q < 0 || q > qmax {
			continue
		}
		v := q
		if m.pbit != 0 {
			v = q<<1 | p
		}
		if d := math.Abs(float64(bc7Expand(v, eb)) - e); d < bestD {
			best, bestD = v, d
		}
	}
	return best
}

type bc7Fit struct {
	v   [2][3]int // stored end point values (with p-bits), eb bits each
	err float64
}

var bc7Combos = [3][][2]int{{{0, 0}}, {{0, 0}, {0, 1}, {1, 0}, {1, 1}}, {{0, 0}, {1, 1}}}

// fit finds end points for the pixels in members and gives their indices in idx.
func (m bc7PartMode) fit(px *[16][3]float64, members []uint8, idx *[16]int) bc7Fit {
	n := float64(len(members))
	var mean [3]float64
	lo, hi := [3]float64{255, 255, 255}, [3]float64{}
	for _, i := range members {
		for c := 0; c < 3; c++ {
			mean[c] += px[i][c]
			lo[c], hi[c] = math.Min(lo[c], px[i][c]), math.Max(hi[c], px[i][c])
		}
	}
	for c := 0; c < 3; c++ {
		mean[c] /= n
	}
	var cov [3][3]float64
	for _, i := range members {
		d := [3]float64{px[i][0] - mean[0], px[i][1] - mean[1], px[i][2] - mean[2]}
		for a := 0; a < 3; a++ {
			for b := a; b < 3; b++ {
				cov[a][b] += d[a] * d[b]
			}
		}
	}
	cov[1][0], cov[2][0], cov[2][1] = cov[0][1], cov[0][2], cov[1][2]
	v := [3]float64{hi[0] - lo[0] + 1e-3, hi[1] - lo[1] + 1e-3, hi[2] - lo[2] + 1e-3}
	for it := 0; it < 4; it++ {
		var nv [3]float64
		for a := 0; a < 3; a++ {
			nv[a] = cov[a][0]*v[0] + cov[a][1]*v[1] + cov[a][2]*v[2]
		}
		l := math.Sqrt(nv[0]*nv[0] + nv[1]*nv[1] + nv[2]*nv[2])
		if l < 1e-9 {
			break
		}
		v = [3]float64{nv[0] / l, nv[1] / l, nv[2] / l}
	}
	if l := math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2]); l > 0 {
		v = [3]float64{v[0] / l, v[1] / l, v[2] / l}
	}
	tmin, tmax := math.Inf(1), math.Inf(-1)
	for _, i := range members {
		t := (px[i][0]-mean[0])*v[0] + (px[i][1]-mean[1])*v[1] + (px[i][2]-mean[2])*v[2]
		tmin, tmax = math.Min(tmin, t), math.Max(tmax, t)
	}
	var e0, e1 [3]float64
	for c := 0; c < 3; c++ {
		e0[c] = math.Max(0, math.Min(255, mean[c]+v[c]*tmin))
		e1[c] = math.Max(0, math.Min(255, mean[c]+v[c]*tmax))
	}
	var bestIdx [16]int
	best := m.evaluate(px, members, e0, e1, &bestIdx)
	if best.err > 0 { // least squares on the end points for the indices found
		w := bc7Weights(m.ib)
		var a2, b2, ab float64
		var at, bt [3]float64
		for _, i := range members {
			wb := float64(w[bestIdx[i]]) / 64
			wa := 1 - wb
			a2 += wa * wa
			b2 += wb * wb
			ab += wa * wb
			for c := 0; c < 3; c++ {
				at[c] += wa * px[i][c]
				bt[c] += wb * px[i][c]
			}
		}
		if det := a2*b2 - ab*ab; math.Abs(det) > 1e-6 {
			for c := 0; c < 3; c++ {
				e0[c] = math.Max(0, math.Min(255, (at[c]*b2-bt[c]*ab)/det))
				e1[c] = math.Max(0, math.Min(255, (bt[c]*a2-at[c]*ab)/det))
			}
			var tryIdx [16]int
			if r := m.evaluate(px, members, e0, e1, &tryIdx); r.err < best.err {
				best, bestIdx = r, tryIdx
			}
		}
	}
	for _, i := range members {
		idx[i] = bestIdx[i]
	}
	return best
}

// evaluate quantises a pair of end points (trying each choice of p-bits) and finds the indices.
func (m bc7PartMode) evaluate(px *[16][3]float64, members []uint8, e0, e1 [3]float64, idx *[16]int) bc7Fit {
	w := bc7Weights(m.ib)
	eb := m.eb()
	best := bc7Fit{err: math.Inf(1)}
	var pal [16][3]float64
	var tryIdx [16]int
	for _, pp := range bc7Combos[m.pbit] {
		var f bc7Fit
		for c := 0; c < 3; c++ {
			f.v[0][c] = m.quant(e0[c], pp[0])
			f.v[1][c] = m.quant(e1[c], pp[1])
		}
		for c := 0; c < 3; c++ {
			a, b := bc7Expand(f.v[0][c], eb), bc7Expand(f.v[1][c], eb)
			for k := range w {
				pal[k][c] = float64(((64-w[k])*a + w[k]*b + 32) >> 6)
			}
		}
		for _, i := range members {
			bk, bd := 0, math.Inf(1)
			r, g, b := px[i][0], px[i][1], px[i][2]
			for k := range w {
				dr, dg, db := r-pal[k][0], g-pal[k][1], b-pal[k][2]
				if d := dr*dr + dg*dg + db*db; d < bd {
					bk, bd = k, d
				}
			}
			tryIdx[i] = bk
			f.err += bd
			if f.err >= best.err {
				break
			}
		}
		if f.err < best.err {
			best = f
			for _, i := range members {
				idx[i] = tryIdx[i]
			}
		}
	}
	return best
}

// encode writes the block with pattern p into dst and returns its summed squared error. It stops
// early (returning at least limit, with dst unfinished) once the error reaches limit.
func (m bc7PartMode) encode(px *[16][3]float64, p int, dst []byte, limit float64) float64 {
	pat := bc7PatternOf(m.ns, p)
	var idx [16]int
	var fits [3]bc7Fit
	total := 0.0
	top := 1<<uint(m.ib) - 1
	for s := 0; s < m.ns; s++ {
		members := pat.members[s][:pat.n[s]]
		fits[s] = m.fit(px, members, &idx)
		total += fits[s].err
		if total >= limit {
			return total
		}
		// the anchor pixel's index must be in the lower half: if not, swap the ends and turn the
		// indices round (with one p-bit per end point, each end keeps its own)
		if a := bc7Anchor(m.ns, p, s); idx[a] > top/2 {
			fits[s].v[0], fits[s].v[1] = fits[s].v[1], fits[s].v[0]
			for _, i := range members {
				idx[i] = top - idx[i]
			}
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
	put(1<<uint(m.mode), m.mode+1)
	put(p, m.pb)
	shift := 0
	if m.pbit != 0 {
		shift = 1
	}
	for c := 0; c < 3; c++ {
		for s := 0; s < m.ns; s++ {
			put(fits[s].v[0][c]>>uint(shift), m.cb)
			put(fits[s].v[1][c]>>uint(shift), m.cb)
		}
	}
	switch m.pbit {
	case 1:
		for s := 0; s < m.ns; s++ {
			put(fits[s].v[0][0]&1, 1)
			put(fits[s].v[1][0]&1, 1)
		}
	case 2:
		for s := 0; s < m.ns; s++ {
			put(fits[s].v[0][0]&1, 1)
		}
	}
	for i := 0; i < 16; i++ {
		n := m.ib
		for s := 0; s < m.ns; s++ {
			if bc7Anchor(m.ns, p, s) == i {
				n--
			}
		}
		put(idx[i], n)
	}
	return total
}
