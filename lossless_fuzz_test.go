package webp

import (
	"math/rand"
	"os"
	"testing"
)

// TestLosslessCorruptInputs covers the VP8L decoder's bounds contract: the
// Huffman reader is allowed to read past the end of the stream into the bit
// reader's zero padding, and only checks the position once per pixel. Truncated
// and corrupted streams have to come back as errors, never a panic.
func TestLosslessCorruptInputs(t *testing.T) {
	data, err := os.ReadFile("testdata/sample_lossless.webp")
	if err != nil {
		t.Fatal(err)
	}

	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 2000; i++ {
		b := append([]byte(nil), data...)
		switch i % 3 {
		case 0:
			b = b[:rng.Intn(len(b))]
		case 1:
			for j := 0; j < 8; j++ {
				b[rng.Intn(len(b))] = byte(rng.Intn(256))
			}
		case 2:
			b = b[:20+rng.Intn(len(b)-20)]
			for j := 0; j < 4; j++ {
				b[12+rng.Intn(len(b)-12)] = byte(rng.Intn(256))
			}
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("case %d: panic: %v", i, r)
				}
			}()
			_, _ = Decode(b)
		}()
	}
}
