//go:build !arm64

package webp

func elossyPlaneSseRegion(source []uint8, sourceStride int, decoded []uint8, decodedStride, width, height int) uint64 {
	return elossyPlaneSseRegionGo(source, sourceStride, decoded, decodedStride, width, height)
}
