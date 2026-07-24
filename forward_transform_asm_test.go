package webp

import (
	"math/rand"
	"testing"
)

func TestElossyForwardTransformMatchesGo(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	for trial := 0; trial < 20000; trial++ {
		srcStride := 4 + rng.Intn(60)
		predStride := 4 + rng.Intn(60)
		sx := rng.Intn(16)
		sy := rng.Intn(16)
		px := rng.Intn(16)
		py := rng.Intn(16)
		src := make([]byte, (sy+4)*srcStride+sx+4+16)
		pred := make([]byte, (py+4)*predStride+px+4+16)
		for i := range src {
			src[i] = byte(rng.Intn(256))
		}
		for i := range pred {
			pred[i] = byte(rng.Intn(256))
		}
		want := elossyForwardTransformAtGo(src, srcStride, sx, sy, pred, predStride, px, py)
		got := elossyForwardTransformAt(src, srcStride, sx, sy, pred, predStride, px, py)
		if want != got {
			t.Fatalf("trial=%d: asm=%v go=%v", trial, got, want)
		}
	}
}
