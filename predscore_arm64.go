//go:build arm64

package webp

import "unsafe"

// elosslessScorePredictorRowAsm scores `groups` runs of 4 interior pixels each,
// starting at actPtr (row y), reading the previous row at actPtr-rowBytes. It
// accumulates the 14 per-mode predictor-error sums into costs. The caller
// guarantees every pixel read is in bounds.
//
//go:noescape
func elosslessScorePredictorRowAsm(actPtr unsafe.Pointer, rowBytes, groups int, costs *[elosslessNumPredictorModes]uint64)

func elosslessScorePredictorRow(argb []uint32, width, y, x0, x1 int, costs *[elosslessNumPredictorModes]uint64) {
	n := x1 - x0
	if n <= 0 {
		return
	}
	groups := n >> 2
	if groups > 0 {
		base := y*width + x0
		// Touch the extreme indices the asm reads so an out-of-range segment
		// panics here in Go rather than reading past the buffer from assembly.
		_ = argb[base-width-1]       // topLeft of the first pixel
		_ = argb[base+(groups<<2)-1] // actual of the last vectorized pixel
		elosslessScorePredictorRowAsm(unsafe.Pointer(&argb[base]), width*4, groups, costs)
	}
	if rem := x0 + (groups << 2); rem < x1 {
		elosslessScorePredictorRowGo(argb, width, y, rem, x1, costs)
	}
}
