package webp

import "testing"

func benchPredictPlane(stride, height int) []uint8 {
	plane := make([]uint8, stride*height)
	state := uint32(12345)
	for i := range plane {
		state = state*1664525 + 1013904223
		plane[i] = uint8(state >> 24)
	}
	return plane
}

func BenchmarkFillPredictionBlock16(b *testing.B) {
	const stride = 320
	plane := benchPredictPlane(stride, 320)
	modes := [4]uint8{dcPred, vPred, hPred, tmPred}
	var out [256]uint8
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, mode := range modes {
			elossyFillPredictionBlock(plane, stride, stride, 64, 48, mode, out[:], 16, 16)
		}
	}
	b.SetBytes(4 * 256)
}

func BenchmarkFillPredictionBlock16Modes(b *testing.B) {
	const stride = 320
	plane := benchPredictPlane(stride, 320)
	var out [256]uint8
	for _, tc := range []struct {
		name string
		mode uint8
	}{{"DC", dcPred}, {"V", vPred}, {"H", hPred}, {"TM", tmPred}} {
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				elossyFillPredictionBlock(plane, stride, stride, 64, 48, tc.mode, out[:], 16, 16)
			}
		})
	}
}

func BenchmarkFillPredictionBlock8(b *testing.B) {
	const stride = 160
	plane := benchPredictPlane(stride, 160)
	modes := [4]uint8{dcPred, vPred, hPred, tmPred}
	var out [64]uint8
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, mode := range modes {
			elossyFillPredictionBlock(plane, stride, stride, 32, 24, mode, out[:], 8, 8)
		}
	}
	b.SetBytes(4 * 64)
}

func BenchmarkRgbaToYuv420(b *testing.B) {
	const width = 641
	const height = 483
	rgba := make([]byte, width*height*4)
	state := uint32(6789)
	for i := range rgba {
		state = state*1664525 + 1013904223
		rgba[i] = uint8(state >> 24)
	}
	mbWidth := (width + 15) / 16
	mbHeight := (height + 15) / 16
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		planes := elossyRgbaToYuv420(width, height, rgba, mbWidth, mbHeight)
		if len(planes.y) == 0 {
			b.Fatal("empty")
		}
	}
	b.SetBytes(int64(width * height * 4))
}
