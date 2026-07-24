//go:build !arm64

package webp

func elossyBlockSse(source []uint8, sourceStride, x, y int, reconstructed []uint8, reconstructedStride, width, height int) uint64 {
	return elossyBlockSseGo(source, sourceStride, x, y, reconstructed, reconstructedStride, width, height)
}
