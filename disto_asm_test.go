package webp

import (
	"math/rand"
	"testing"
)

func TestElossyTDisto4x4MatchesGo(t *testing.T) {
	rng := rand.New(rand.NewSource(21))
	for trial := 0; trial < 20000; trial++ {
		srcStride := 4 + rng.Intn(60)
		predStride := 4 + rng.Intn(20)
		srcX := rng.Intn(20)
		srcY := rng.Intn(20)
		predX := rng.Intn(predStride - 3)
		predY := rng.Intn(8)
		src := make([]byte, (srcY+4)*srcStride+srcX+4)
		pred := make([]byte, (predY+4)*predStride+predX+4)
		fill := func(buf []byte) {
			switch trial % 4 {
			case 0:
				for i := range buf {
					buf[i] = byte(rng.Intn(256))
				}
			case 1:
				// Extremes only, which maximizes the transform's dynamic range.
				for i := range buf {
					buf[i] = byte(255 * rng.Intn(2))
				}
			case 2:
				for i := range buf {
					buf[i] = 255
				}
			default:
				for i := range buf {
					buf[i] = 0
				}
			}
		}
		fill(src)
		fill(pred)
		if trial%4 == 2 {
			for i := range pred {
				pred[i] = 0
			}
		}
		want := elossyTDisto4x4Go(src, srcStride, srcX, srcY, pred, predStride, predX, predY)
		got := elossyTDisto4x4(src, srcStride, srcX, srcY, pred, predStride, predX, predY)
		if got != want {
			t.Fatalf("trial=%d: asm=%d go=%d", trial, got, want)
		}
	}
}

func TestElossyTDisto4x4ContiguousMatchesGo(t *testing.T) {
	rng := rand.New(rand.NewSource(22))
	for trial := 0; trial < 20000; trial++ {
		srcStride := 4 + rng.Intn(60)
		srcX := rng.Intn(20)
		srcY := rng.Intn(20)
		src := make([]byte, (srcY+4)*srcStride+srcX+4)
		var pred [16]uint8
		switch trial % 3 {
		case 0:
			for i := range src {
				src[i] = byte(rng.Intn(256))
			}
			for i := range pred {
				pred[i] = byte(rng.Intn(256))
			}
		case 1:
			for i := range src {
				src[i] = byte(255 * rng.Intn(2))
			}
			for i := range pred {
				pred[i] = byte(255 * rng.Intn(2))
			}
		default:
			for i := range src {
				src[i] = 255
			}
		}
		want := elossyTDisto4x4ContiguousGo(src, srcStride, srcX, srcY, &pred)
		got := elossyTDisto4x4Contiguous(src, srcStride, srcX, srcY, &pred)
		if got != want {
			t.Fatalf("trial=%d: asm=%d go=%d", trial, got, want)
		}
	}
}

func TestElossyTDistoBlocksMatchesGo(t *testing.T) {
	rng := rand.New(rand.NewSource(25))
	for trial := 0; trial < 10000; trial++ {
		cols := 1 + rng.Intn(4)
		rows := 1 + rng.Intn(4)
		predStride := cols * 4
		srcStride := predStride + rng.Intn(40)
		srcX := rng.Intn(20)
		srcY := rng.Intn(20)
		src := make([]byte, (srcY+rows*4)*srcStride+srcX+cols*4)
		pred := make([]byte, rows*4*predStride)
		extreme := trial%3 == 1
		for i := range src {
			if extreme {
				src[i] = byte(255 * rng.Intn(2))
			} else {
				src[i] = byte(rng.Intn(256))
			}
		}
		for i := range pred {
			if extreme {
				pred[i] = byte(255 * rng.Intn(2))
			} else {
				pred[i] = byte(rng.Intn(256))
			}
		}
		want := elossyTDistoBlocksGo(src, srcStride, srcX, srcY, pred, predStride, cols, rows)
		got := elossyTDistoBlocks(src, srcStride, srcX, srcY, pred, predStride, cols, rows)
		if got != want {
			t.Fatalf("trial=%d cols=%d rows=%d: asm=%d go=%d", trial, cols, rows, got, want)
		}
	}
}

func BenchmarkTDistoBlocks16(b *testing.B) {
	rng := rand.New(rand.NewSource(26))
	src := make([]byte, 64*64)
	pred := make([]byte, 16*16)
	for i := range src {
		src[i] = byte(rng.Intn(256))
	}
	for i := range pred {
		pred[i] = byte(rng.Intn(256))
	}
	b.Run("go", func(b *testing.B) {
		var sink uint64
		for i := 0; i < b.N; i++ {
			sink += elossyTDistoBlocksGo(src, 64, 8, 8, pred, 16, 4, 4)
		}
		elossyAsmBenchSink = uint32(sink)
	})
	b.Run("dispatch", func(b *testing.B) {
		var sink uint64
		for i := 0; i < b.N; i++ {
			sink += elossyTDistoBlocks(src, 64, 8, 8, pred, 16, 4, 4)
		}
		elossyAsmBenchSink = uint32(sink)
	})
}

func BenchmarkTDisto4x4(b *testing.B) {
	rng := rand.New(rand.NewSource(23))
	src := make([]byte, 64*64)
	pred := make([]byte, 16*16)
	for i := range src {
		src[i] = byte(rng.Intn(256))
	}
	for i := range pred {
		pred[i] = byte(rng.Intn(256))
	}
	b.Run("go", func(b *testing.B) {
		var sink uint32
		for i := 0; i < b.N; i++ {
			sink += elossyTDisto4x4Go(src, 64, 8, 8, pred, 16, 4, 4)
		}
		elossyAsmBenchSink = sink
	})
	b.Run("dispatch", func(b *testing.B) {
		var sink uint32
		for i := 0; i < b.N; i++ {
			sink += elossyTDisto4x4(src, 64, 8, 8, pred, 16, 4, 4)
		}
		elossyAsmBenchSink = sink
	})
}

func BenchmarkTDisto4x4Contiguous(b *testing.B) {
	rng := rand.New(rand.NewSource(24))
	src := make([]byte, 64*64)
	var pred [16]uint8
	for i := range src {
		src[i] = byte(rng.Intn(256))
	}
	for i := range pred {
		pred[i] = byte(rng.Intn(256))
	}
	b.Run("go", func(b *testing.B) {
		var sink uint32
		for i := 0; i < b.N; i++ {
			sink += elossyTDisto4x4ContiguousGo(src, 64, 8, 8, &pred)
		}
		elossyAsmBenchSink = sink
	})
	b.Run("dispatch", func(b *testing.B) {
		var sink uint32
		for i := 0; i < b.N; i++ {
			sink += elossyTDisto4x4Contiguous(src, 64, 8, 8, &pred)
		}
		elossyAsmBenchSink = sink
	})
}

var elossyAsmBenchSink uint32
