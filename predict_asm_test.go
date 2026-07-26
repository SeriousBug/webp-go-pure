package webp

import (
	"math/rand"
	"testing"
)

// TestElossyFillPredictionBlockFastPath checks the size-specialized kernels
// against the generic loop. Passing outStride == n+1 keeps the same rows but
// misses the fast path, so the generic code produces the comparison.
func TestElossyFillPredictionBlockFastPath(t *testing.T) {
	rng := rand.New(rand.NewSource(23))
	modes := []uint8{dcPred, vPred, hPred, tmPred}
	for _, n := range []int{8, 16} {
		stride := n * 6
		plane := make([]uint8, stride*n*6)
		for i := range plane {
			plane[i] = byte(rng.Intn(256))
		}
		for _, mode := range modes {
			for y := 0; y*n < stride; y += n {
				for x := 0; x+n <= stride; x += n {
					fast := make([]uint8, n*n)
					slow := make([]uint8, n*(n+1))
					elossyFillPredictionBlock(plane, stride, stride, x, y, mode, fast, n, n)
					elossyFillPredictionBlock(plane, stride, stride, x, y, mode, slow, n+1, n)
					for row := 0; row < n; row++ {
						for col := 0; col < n; col++ {
							if fast[row*n+col] != slow[row*(n+1)+col] {
								t.Fatalf("n=%d mode=%d x=%d y=%d row=%d col=%d: fast=%d slow=%d",
									n, mode, x, y, row, col, fast[row*n+col], slow[row*(n+1)+col])
							}
						}
					}
				}
			}
		}
	}
}

func TestElossyTmPredictMatchesGo(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	edges := []uint8{0, 1, 127, 128, 129, 254, 255}
	pick := func() uint8 {
		if rng.Intn(3) == 0 {
			return edges[rng.Intn(len(edges))]
		}
		return uint8(rng.Intn(256))
	}

	for trial := 0; trial < 20000; trial++ {
		topLeft := pick()

		var top16, left16 [16]uint8
		for i := range top16 {
			top16[i] = pick()
			left16[i] = pick()
		}
		var want16, got16 [256]uint8
		elossyTmPredict16Go(&top16, &left16, topLeft, &want16)
		elossyTmPredict16(&top16, &left16, topLeft, &got16)
		if got16 != want16 {
			t.Fatalf("trial=%d n=16 topLeft=%d: asm=%v go=%v", trial, topLeft, got16, want16)
		}

		var top8, left8 [8]uint8
		for i := range top8 {
			top8[i] = pick()
			left8[i] = pick()
		}
		var want8, got8 [64]uint8
		elossyTmPredict8Go(&top8, &left8, topLeft, &want8)
		elossyTmPredict8(&top8, &left8, topLeft, &got8)
		if got8 != want8 {
			t.Fatalf("trial=%d n=8 topLeft=%d: asm=%v go=%v", trial, topLeft, got8, want8)
		}
	}
}
