//go:build amd64

package webp

import "unsafe"

// elossyBlockSseAsm computes the sum of squared differences between an n-byte-wide,
// n-row source block (row pitch srcStride) and a contiguous n*n reconstructed block.
// It handles n == 16 and n == 8 only; the caller guarantees this.
//
//go:noescape
func elossyBlockSseAsm(srcPtr, recPtr unsafe.Pointer, srcStride, n int) uint64

//go:noescape
func elossyBlockSse4x4Asm(srcPtr, candPtr unsafe.Pointer, srcStride int) uint64

func elossyBlockSse4x4(source []uint8, stride, x, y int, candidate *[16]uint8) uint64 {
	// Touch the extreme source index so an out-of-range block panics here in Go
	// rather than reading past the buffer from assembly.
	_ = source[(y+3)*stride+x+3]
	return elossyBlockSse4x4Asm(unsafe.Pointer(&source[y*stride+x]), unsafe.Pointer(candidate), stride)
}

func elossyBlockSse(source []uint8, sourceStride, x, y int, reconstructed []uint8, reconstructedStride, width, height int) uint64 {
	if width == height && (width == 16 || width == 8) && reconstructedStride == width {
		// Touch the extreme indices the asm will read so an out-of-range block
		// panics here in Go rather than corrupting memory from assembly.
		_ = source[(y+height-1)*sourceStride+x+width-1]
		_ = reconstructed[width*height-1]
		srcPtr := unsafe.Pointer(&source[y*sourceStride+x])
		recPtr := unsafe.Pointer(&reconstructed[0])
		return elossyBlockSseAsm(srcPtr, recPtr, sourceStride, width)
	}
	return elossyBlockSseGo(source, sourceStride, x, y, reconstructed, reconstructedStride, width, height)
}
