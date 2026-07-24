package webp

import (
	"math/rand"
	"testing"
)

func TestElossyAddTransformMatchesGo(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for trial := 0; trial < 20000; trial++ {
		stride := 4 + rng.Intn(60)
		x := rng.Intn(16)
		y := rng.Intn(16)
		plane := make([]byte, (y+4)*stride+x+4+16)
		for i := range plane {
			plane[i] = byte(rng.Intn(256))
		}
		var coeffs [16]int16
		switch trial % 3 {
		case 0: // small coeffs
			for i := range coeffs {
				coeffs[i] = int16(rng.Intn(65) - 32)
			}
		case 1: // full int16 range (exercises overflow path)
			for i := range coeffs {
				coeffs[i] = int16(rng.Uint32())
			}
		default: // mostly zero with a few nonzero
			for i := 0; i < 3; i++ {
				coeffs[rng.Intn(16)] = int16(rng.Intn(2000) - 1000)
			}
		}

		want := append([]byte(nil), plane...)
		got := append([]byte(nil), plane...)
		elossyAddTransformGo(want, stride, x, y, &coeffs)
		elossyAddTransform(got, stride, x, y, &coeffs)
		for i := range want {
			if want[i] != got[i] {
				t.Fatalf("trial=%d stride=%d x=%d y=%d coeffs=%v: byte %d asm=%d go=%d",
					trial, stride, x, y, coeffs, i, got[i], want[i])
			}
		}
	}
}
