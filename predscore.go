package webp

// elosslessScorePredictorRow is dispatched to a vectorized backend on arm64 and
// amd64 (see predscore_{arm64,amd64}.go) and to the Go reference elsewhere (see
// predscore_generic.go).

// elosslessScorePredictorRowGo accumulates, for every interior pixel in row y at
// columns [x0, x1), the sum of predictor errors for each of the 14 predictor
// modes into costs. Every pixel in the range must be interior: y >= 1, x >= 1,
// and x+1 < width, so left/top/topLeft/topRight are all in-bounds without the
// last-column wrap. Callers handle the border pixels separately.
//
// This is the pure-Go reference; the arm64/amd64 builds replace the dispatched
// elosslessScorePredictorRow with a vectorized version that must match this
// byte-for-byte.
func elosslessScorePredictorRowGo(argb []uint32, width, y, x0, x1 int, costs *[elosslessNumPredictorModes]uint64) {
	base := y * width
	for x := x0; x < x1; x++ {
		index := base + x
		actual := argb[index]
		left := argb[index-1]
		top := argb[index-width]
		topLeft := argb[index-width-1]
		topRight := argb[index-width+1]
		for mode := uint8(0); mode < elosslessNumPredictorModes; mode++ {
			pred := elosslessPredictor(mode, left, top, topLeft, topRight)
			costs[mode] += uint64(elosslessPredictorError(actual, pred))
		}
	}
}
