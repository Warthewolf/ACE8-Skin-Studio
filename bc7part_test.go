package main

import (
	"math"
	"math/rand"
	"testing"
)

// Each multi-part mode must decode (with the decoder that follows the reference) to exactly what
// the encoder worked out, for every pattern.
func TestBC7PartModesDecode(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for _, m := range []bc7PartMode{bc7M0, bc7M1, bc7M2, bc7M3} {
		n := 64
		if m.mode == 0 {
			n = 16
		}
		for p := 0; p < n; p++ {
			for trial := 0; trial < 6; trial++ {
				src := make([]uint8, 64)
				var px [16][3]float64
				for i := 0; i < 16; i++ {
					// each part a colour of its own with a little noise, sometimes pure random
					s := 0
					for k := 1; k < m.ns; k++ {
						for _, j := range bc7PatternOf(m.ns, p).members[k][:bc7PatternOf(m.ns, p).n[k]] {
							if int(j) == i {
								s = k
							}
						}
					}
					for c := 0; c < 3; c++ {
						v := (s*97 + c*53 + trial*31) % 256
						if trial%3 == 2 {
							v = rng.Intn(256)
						} else {
							v = min(255, max(0, v+rng.Intn(21)-10))
						}
						src[i*4+c] = uint8(v)
						px[i][c] = float64(v)
					}
					src[i*4+3] = 255
				}
				var blk [16]byte
				want := m.encode(&px, p, blk[:], math.Inf(1))
				var out [64]uint8
				decodeBC7Block(blk[:], out[:], 16)
				got := 0.0
				for i := 0; i < 16; i++ {
					if out[i*4+3] != 255 {
						t.Fatalf("mode %d pattern %d: pixel %d decodes see-through (%d)", m.mode, p, i, out[i*4+3])
					}
					for c := 0; c < 3; c++ {
						d := float64(src[i*4+c]) - float64(out[i*4+c])
						got += d * d
					}
				}
				if math.Abs(got-want) > 1e-6 {
					t.Fatalf("mode %d pattern %d trial %d: encoder expected error %.0f, decoder gives %.0f", m.mode, p, trial, want, got)
				}
			}
		}
	}
}

// A block split between two colours that are not on one line (a red stripe on blue) comes out
// much closer than mode 6 can make it, and smooth blocks stay mode 6.
func TestBC7EdgesBetterThanOneLine(t *testing.T) {
	src := make([]uint8, 64)
	for i := 0; i < 16; i++ {
		x, y := i%4, i/4
		c := [3]uint8{30, 60, 200} // blue
		if x+y < 3 {
			c = [3]uint8{220, 30, 30} // red
		}
		if x+y == 3 {
			c = [3]uint8{245, 245, 245} // a white line between them
		}
		src[i*4], src[i*4+1], src[i*4+2], src[i*4+3] = c[0], c[1], c[2], 255
	}
	var one, many [16]byte
	encodeBC7Mode6(src, 16, one[:])
	encodeBC7Block(src, 16, many[:])
	e1, e2 := bc7Error(src, 16, one[:]), bc7Error(src, 16, many[:])
	if e2 >= e1/4 {
		t.Fatalf("three colours: mode 6 error %.0f, best %.0f (mode bits %08b); expected far better", e1, e2, many[0])
	}
	// smooth: a soft gradient
	for i := 0; i < 16; i++ {
		v := uint8(100 + i*3)
		src[i*4], src[i*4+1], src[i*4+2] = v, v/2+40, 255-v
	}
	encodeBC7Block(src, 16, many[:])
	if many[0]&0x7f != 1<<6 {
		t.Fatalf("a smooth block should stay mode 6, got mode bits %08b", many[0])
	}
}

func BenchmarkBC7Edge(b *testing.B) {
	src := make([]uint8, 64)
	for i := 0; i < 16; i++ {
		c := [3]uint8{30, 60, 200}
		if i%4+i/4 < 3 {
			c = [3]uint8{220, 30, 30}
		}
		src[i*4], src[i*4+1], src[i*4+2], src[i*4+3] = c[0], c[1], c[2], 255
	}
	src[0] = 250
	dst := make([]byte, 16)
	for i := 0; i < b.N; i++ {
		encodeBC7Block(src, 16, dst)
	}
}
