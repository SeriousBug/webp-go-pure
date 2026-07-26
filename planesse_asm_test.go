package webp

import (
	"math/rand"
	"testing"
)

func TestElossyPlaneSseRegionMatchesGo(t *testing.T) {
	rng := rand.New(rand.NewSource(31))
	for trial := 0; trial < 3000; trial++ {
		width := 1 + rng.Intn(70)
		height := 1 + rng.Intn(20)
		sourceStride := width + rng.Intn(17)
		decodedStride := width + rng.Intn(17)
		source := make([]byte, (height-1)*sourceStride+width)
		decoded := make([]byte, (height-1)*decodedStride+width)
		switch trial % 3 {
		case 0:
			for i := range source {
				source[i] = byte(rng.Intn(256))
			}
			for i := range decoded {
				decoded[i] = byte(rng.Intn(256))
			}
		case 1:
			// Maximum per-pixel error everywhere, the worst case for the
			// accumulator lanes.
			for i := range source {
				source[i] = 255
			}
		default:
			for i := range decoded {
				decoded[i] = 255
			}
		}
		want := elossyPlaneSseRegionGo(source, sourceStride, decoded, decodedStride, width, height)
		got := elossyPlaneSseRegion(source, sourceStride, decoded, decodedStride, width, height)
		if got != want {
			t.Fatalf("trial=%d w=%d h=%d: asm=%d go=%d", trial, width, height, got, want)
		}
	}
}

func BenchmarkPlaneSseRegion(b *testing.B) {
	rng := rand.New(rand.NewSource(32))
	const width, height = 640, 480
	source := make([]byte, width*height)
	decoded := make([]byte, width*height)
	for i := range source {
		source[i] = byte(rng.Intn(256))
		decoded[i] = byte(rng.Intn(256))
	}
	b.Run("go", func(b *testing.B) {
		var sink uint64
		for i := 0; i < b.N; i++ {
			sink += elossyPlaneSseRegionGo(source, width, decoded, width, width, height)
		}
		elossyAsmBenchSink = uint32(sink)
	})
	b.Run("dispatch", func(b *testing.B) {
		var sink uint64
		for i := 0; i < b.N; i++ {
			sink += elossyPlaneSseRegion(source, width, decoded, width, width, height)
		}
		elossyAsmBenchSink = uint32(sink)
	})
}
