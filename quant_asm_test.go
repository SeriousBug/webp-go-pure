package webp

import (
	"math/rand"
	"testing"
)

func TestQuantizeBlockMatchesGo(t *testing.T) {
	steps := []uint16{4, 5, 7, 8, 17, 63, 64, 65, 157, 284, 314, 440, 511}
	rng := rand.New(rand.NewSource(1))
	var coeffs [16]int16
	for _, dcQuant := range steps {
		for _, acQuant := range steps {
			for _, first := range []int{0, 1} {
				for trial := 0; trial < 400; trial++ {
					for index := range coeffs {
						switch trial % 4 {
						case 0:
							coeffs[index] = int16(rng.Intn(65536) - 32768)
						case 1:
							coeffs[index] = int16(rng.Intn(64) - 32)
						case 2:
							coeffs[index] = 0
						default:
							coeffs[index] = []int16{-32768, 0, 32767}[rng.Intn(3)]
						}
					}
					got := elossyQuantizeBlock(&coeffs, dcQuant, acQuant, first)
					want := elossyQuantizeBlockGo(&coeffs, dcQuant, acQuant, first)
					if got != want {
						t.Fatalf("dc=%d ac=%d first=%d coeffs=%v:\n got %v\nwant %v",
							dcQuant, acQuant, first, coeffs, got, want)
					}
				}
			}
		}
	}
}

func BenchmarkQuantizeBlock(b *testing.B) {
	var coeffs [16]int16
	rng := rand.New(rand.NewSource(1))
	for index := range coeffs {
		coeffs[index] = int16(rng.Intn(2048) - 1024)
	}
	b.Run("asm", func(b *testing.B) {
		for range b.N {
			sink = elossyQuantizeBlock(&coeffs, 26, 31, 0)
		}
	})
	b.Run("go", func(b *testing.B) {
		for range b.N {
			sink = elossyQuantizeBlockGo(&coeffs, 26, 31, 0)
		}
	})
}

var sink [16]int16

func TestZigzagLastMatchesGo(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	var levels [16]int16
	for _, first := range []int{0, 1} {
		for trial := 0; trial < 20000; trial++ {
			for index := range levels {
				switch {
				case trial%3 == 0:
					levels[index] = int16(rng.Intn(65536) - 32768)
				case rng.Intn(4) == 0:
					levels[index] = int16(rng.Intn(9) - 4)
				default:
					levels[index] = 0
				}
			}
			var gotBuf, wantBuf [16]int16
			got := elossyZigzagLast(&levels, &gotBuf, first)
			want := elossyZigzagLastGo(&levels, &wantBuf, first)
			if got != want || gotBuf != wantBuf {
				t.Fatalf("first=%d levels=%v:\n got %d %v\nwant %d %v",
					first, levels, got, gotBuf, want, wantBuf)
			}
		}
	}
}
