//go:build amd64

package webp

// TODO(simd): AVX2 backend. Falls back to the Go reference until the kernel lands.
func elosslessScorePredictorRow(argb []uint32, width, y, x0, x1 int, costs *[elosslessNumPredictorModes]uint64) {
	elosslessScorePredictorRowGo(argb, width, y, x0, x1, costs)
}
