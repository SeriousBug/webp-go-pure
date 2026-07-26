//go:build amd64

package webp

import "unsafe"

//go:noescape
func elossyPlaneSseAsm(srcPtr, decPtr unsafe.Pointer, srcStride, decStride, width, height int) uint64

func elossyPlaneSseRegion(source []uint8, sourceStride int, decoded []uint8, decodedStride, width, height int) uint64 {
	if width <= 0 || height <= 0 {
		return 0
	}
	// Touch the extreme indices the asm will read so an out-of-range region
	// panics here in Go rather than reading past the buffers from assembly.
	_ = source[(height-1)*sourceStride+width-1]
	_ = decoded[(height-1)*decodedStride+width-1]
	return elossyPlaneSseAsm(
		unsafe.Pointer(&source[0]), unsafe.Pointer(&decoded[0]),
		sourceStride, decodedStride, width, height,
	)
}
