package webp

import (
	"math/rand"
	"testing"
)

// TestScorePredictorRowMatchesGo checks the dispatched (possibly vectorized)
// elosslessScorePredictorRow against the pure-Go reference byte-for-byte over a
// range of widths and interior segment lengths, including non-multiples of 4.
func TestScorePredictorRowMatchesGo(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for _, width := range []int{5, 8, 13, 16, 33, 64} {
		height := 6
		argb := make([]uint32, width*height)
		for i := range argb {
			argb[i] = rng.Uint32()
		}
		for _, y := range []int{1, 2, height - 1} {
			for x0 := 1; x0 < width-1; x0++ {
				for _, x1 := range []int{x0, x0 + 1, x0 + 3, x0 + 4, x0 + 7, width - 1} {
					if x1 < x0 || x1 > width-1 {
						continue
					}
					var got, want [elosslessNumPredictorModes]uint64
					elosslessScorePredictorRow(argb, width, y, x0, x1, &got)
					elosslessScorePredictorRowGo(argb, width, y, x0, x1, &want)
					if got != want {
						t.Fatalf("width=%d y=%d x0=%d x1=%d\n got=%v\nwant=%v", width, y, x0, x1, got, want)
					}
				}
			}
		}
	}
}
