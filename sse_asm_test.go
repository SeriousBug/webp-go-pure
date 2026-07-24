package webp

import (
	"math/rand"
	"testing"
)

func TestElossyBlockSseMatchesGo(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for _, n := range []int{8, 16} {
		for trial := 0; trial < 2000; trial++ {
			srcStride := n + rng.Intn(48) // >= width, arbitrary pitch
			x := rng.Intn(20)
			y := rng.Intn(20)
			source := make([]byte, (y+n)*srcStride+x+n+16)
			for i := range source {
				source[i] = byte(rng.Intn(256))
			}
			recon := make([]byte, n*n)
			for i := range recon {
				recon[i] = byte(rng.Intn(256))
			}
			want := elossyBlockSseGo(source, srcStride, x, y, recon, n, n, n)
			got := elossyBlockSse(source, srcStride, x, y, recon, n, n, n)
			if got != want {
				t.Fatalf("n=%d stride=%d x=%d y=%d: asm=%d go=%d", n, srcStride, x, y, got, want)
			}
		}
	}
}
