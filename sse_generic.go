//go:build !arm64

package webp

func elossyBlockSse(source []uint8, sourceStride, x, y int, reconstructed []uint8, reconstructedStride, width, height int) uint64 {
	return elossyBlockSseGo(source, sourceStride, x, y, reconstructed, reconstructedStride, width, height)
}

func elossyBlockSse4x4(source []uint8, stride, x, y int, candidate *[16]uint8) uint64 {
	return elossyBlockSse4x4Go(source, stride, x, y, candidate)
}
