package main

// BC1 (DXT1) block compression, the format of the colour, finish and mask textures.

import "math"

func rgb565(c uint16) (r, g, b int) {
	r5, g6, b5 := int(c>>11)&31, int(c>>5)&63, int(c)&31
	return r5<<3 | r5>>2, g6<<2 | g6>>4, b5<<3 | b5>>2
}

// bc1Palette returns the four colours a block can use, exactly as the decoder computes them.
func bc1Palette(c0, c1 uint16) (pal [4][3]int) {
	r0, g0, b0 := rgb565(c0)
	r1, g1, b1 := rgb565(c1)
	pal[0] = [3]int{r0, g0, b0}
	pal[1] = [3]int{r1, g1, b1}
	if c0 > c1 {
		pal[2] = [3]int{(2*r0 + r1) / 3, (2*g0 + g1) / 3, (2*b0 + b1) / 3}
		pal[3] = [3]int{(r0 + 2*r1) / 3, (g0 + 2*g1) / 3, (b0 + 2*b1) / 3}
	} else {
		pal[2] = [3]int{(r0 + r1) / 2, (g0 + g1) / 2, (b0 + b1) / 2}
		pal[3] = [3]int{0, 0, 0}
	}
	return
}

// decodeBC1Block writes 16 RGBA pixels (4x4, row by row) into dst with the given row stride.
func decodeBC1Block(src []byte, dst []uint8, stride int) {
	c0 := uint16(src[0]) | uint16(src[1])<<8
	c1 := uint16(src[2]) | uint16(src[3])<<8
	pal := bc1Palette(c0, c1)
	bits := uint32(src[4]) | uint32(src[5])<<8 | uint32(src[6])<<16 | uint32(src[7])<<24
	for i := 0; i < 16; i++ {
		p := pal[(bits>>(2*uint(i)))&3]
		o := (i/4)*stride + (i%4)*4
		dst[o], dst[o+1], dst[o+2], dst[o+3] = uint8(p[0]), uint8(p[1]), uint8(p[2]), 255
	}
}

func to565(r, g, b float64) uint16 {
	q := func(v, m float64) uint16 {
		v = math.Round(v * m / 255)
		if v < 0 {
			v = 0
		}
		if v > m {
			v = m
		}
		return uint16(v)
	}
	return q(r, 31)<<11 | q(g, 63)<<5 | q(b, 31)
}

// bc1Assign picks the nearest palette entry for each pixel and returns the packed indices and total squared error.
func bc1Assign(px *[16][3]int, c0, c1 uint16) (uint32, int) {
	pal := bc1Palette(c0, c1)
	n := 4
	if c0 <= c1 {
		n = 3 // the fourth entry would be transparent black; never use it
	}
	var bits uint32
	total := 0
	for i := 0; i < 16; i++ {
		best, bi := math.MaxInt, 0
		for k := 0; k < n; k++ {
			dr, dg, db := px[i][0]-pal[k][0], px[i][1]-pal[k][1], px[i][2]-pal[k][2]
			e := dr*dr + dg*dg + db*db
			if e < best {
				best, bi = e, k
			}
		}
		bits |= uint32(bi) << (2 * uint(i))
		total += best
	}
	return bits, total
}

// encodeBC1Block compresses 16 RGBA pixels (read from src with the given row stride) into 8 bytes.
func encodeBC1Block(src []uint8, stride int, dst []byte) {
	var px [16][3]int
	var mean [3]float64
	minv := [3]int{255, 255, 255}
	maxv := [3]int{0, 0, 0}
	for i := 0; i < 16; i++ {
		o := (i/4)*stride + (i%4)*4
		for c := 0; c < 3; c++ {
			v := int(src[o+c])
			px[i][c] = v
			mean[c] += float64(v)
			if v < minv[c] {
				minv[c] = v
			}
			if v > maxv[c] {
				maxv[c] = v
			}
		}
	}
	for c := 0; c < 3; c++ {
		mean[c] /= 16
	}

	bestErr := math.MaxInt
	var bestC0, bestC1 uint16
	var bestBits uint32
	try := func(a, b uint16) {
		if a < b {
			a, b = b, a
		}
		bits, e := bc1Assign(&px, a, b)
		if e < bestErr {
			bestErr, bestC0, bestC1, bestBits = e, a, b, bits
		}
	}

	// Candidate 1: the two 5:6:5 colours either side of the mean (covers flat and near-flat blocks).
	floorQ := func(v, m float64) uint16 { return uint16(math.Floor(v * m / 255)) }
	ceilQ := func(v, m float64) uint16 { return uint16(math.Ceil(v * m / 255)) }
	try(ceilQ(mean[0], 31)<<11|ceilQ(mean[1], 63)<<5|ceilQ(mean[2], 31),
		floorQ(mean[0], 31)<<11|floorQ(mean[1], 63)<<5|floorQ(mean[2], 31))

	if maxv != minv {
		// Candidate 2: end points along the block's main colour direction.
		var cov [3][3]float64
		for i := 0; i < 16; i++ {
			d := [3]float64{float64(px[i][0]) - mean[0], float64(px[i][1]) - mean[1], float64(px[i][2]) - mean[2]}
			for a := 0; a < 3; a++ {
				for b := 0; b < 3; b++ {
					cov[a][b] += d[a] * d[b]
				}
			}
		}
		v := [3]float64{float64(maxv[0]-minv[0]) + 1e-3, float64(maxv[1]-minv[1]) + 1e-3, float64(maxv[2]-minv[2]) + 1e-3}
		for it := 0; it < 6; it++ {
			var n [3]float64
			for a := 0; a < 3; a++ {
				n[a] = cov[a][0]*v[0] + cov[a][1]*v[1] + cov[a][2]*v[2]
			}
			l := math.Sqrt(n[0]*n[0] + n[1]*n[1] + n[2]*n[2])
			if l < 1e-9 {
				break
			}
			v = [3]float64{n[0] / l, n[1] / l, n[2] / l}
		}
		l := math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2])
		v = [3]float64{v[0] / l, v[1] / l, v[2] / l}
		tmin, tmax := math.Inf(1), math.Inf(-1)
		for i := 0; i < 16; i++ {
			t := (float64(px[i][0])-mean[0])*v[0] + (float64(px[i][1])-mean[1])*v[1] + (float64(px[i][2])-mean[2])*v[2]
			tmin, tmax = math.Min(tmin, t), math.Max(tmax, t)
		}
		try(to565(mean[0]+v[0]*tmax, mean[1]+v[1]*tmax, mean[2]+v[2]*tmax),
			to565(mean[0]+v[0]*tmin, mean[1]+v[1]*tmin, mean[2]+v[2]*tmin))

		// Refinement: with the index choice fixed, solve for the best end points, then re-pick indices.
		for pass := 0; pass < 2 && bestC0 > bestC1; pass++ {
			var a2, b2, ab float64
			var at, bt [3]float64
			for i := 0; i < 16; i++ {
				var wa float64
				switch (bestBits >> (2 * uint(i))) & 3 {
				case 0:
					wa = 1
				case 1:
					wa = 0
				case 2:
					wa = 2.0 / 3
				default:
					wa = 1.0 / 3
				}
				wb := 1 - wa
				a2 += wa * wa
				b2 += wb * wb
				ab += wa * wb
				for c := 0; c < 3; c++ {
					at[c] += wa * float64(px[i][c])
					bt[c] += wb * float64(px[i][c])
				}
			}
			det := a2*b2 - ab*ab
			if math.Abs(det) < 1e-6 {
				break
			}
			var e0, e1 [3]float64
			for c := 0; c < 3; c++ {
				e0[c] = (at[c]*b2 - bt[c]*ab) / det
				e1[c] = (bt[c]*a2 - at[c]*ab) / det
			}
			before := bestErr
			try(to565(e0[0], e0[1], e0[2]), to565(e1[0], e1[1], e1[2]))
			if bestErr >= before {
				break
			}
		}
	}
	dst[0], dst[1] = byte(bestC0), byte(bestC0>>8)
	dst[2], dst[3] = byte(bestC1), byte(bestC1>>8)
	dst[4], dst[5], dst[6], dst[7] = byte(bestBits), byte(bestBits>>8), byte(bestBits>>16), byte(bestBits>>24)
}
