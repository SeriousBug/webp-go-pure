package webp

import (
	"math/rand"
	"testing"
)

func TestElossyBlockSse4x4MatchesGo(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for trial := 0; trial < 5000; trial++ {
		stride := 4 + rng.Intn(60)
		x := rng.Intn(20)
		y := rng.Intn(20)
		source := make([]byte, (y+4)*stride+x+4+16)
		for i := range source {
			source[i] = byte(rng.Intn(256))
		}
		var cand [16]uint8
		for i := range cand {
			cand[i] = byte(rng.Intn(256))
		}
		want := elossyBlockSse4x4Go(source, stride, x, y, &cand)
		got := elossyBlockSse4x4(source, stride, x, y, &cand)
		if got != want {
			t.Fatalf("trial=%d stride=%d x=%d y=%d: asm=%d go=%d", trial, stride, x, y, got, want)
		}
	}
}
