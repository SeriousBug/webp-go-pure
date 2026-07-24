//go:build !arm64 && !amd64

package webp

func elosslessScorePredictorRow(argb []uint32, width, y, x0, x1 int, costs *[elosslessNumPredictorModes]uint64) {
	elosslessScorePredictorRowGo(argb, width, y, x0, x1, costs)
}
