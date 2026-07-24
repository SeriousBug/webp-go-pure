//go:build arm64

package webp

import "unsafe"

//go:noescape
func elossyAddTransformAsm(planePtr unsafe.Pointer, stride int, coeffs *[16]int16)

//go:noescape
func elossyForwardTransformAsm(srcPtr unsafe.Pointer, srcStride int, predPtr unsafe.Pointer, predStride int, out *[16]int16)

func elossyForwardTransformAt(src []uint8, srcStride, srcX, srcY int, pred []uint8, predStride, predX, predY int) [16]int16 {
	// Touch the extreme source/pred indices so an out-of-range block panics here
	// in Go rather than reading past the buffers from assembly.
	_ = src[(srcY+3)*srcStride+srcX+3]
	_ = pred[(predY+3)*predStride+predX+3]
	var out [16]int16
	elossyForwardTransformAsm(
		unsafe.Pointer(&src[srcY*srcStride+srcX]), srcStride,
		unsafe.Pointer(&pred[predY*predStride+predX]), predStride,
		&out)
	return out
}

func elossyAddTransform(plane []uint8, stride, x, y int, coeffs *[16]int16) {
	// The 16 int16 coefficients are 32 bytes; OR them as four uint64 words to
	// detect an all-zero block branchlessly. Zero blocks add nothing, so skip them.
	words := (*[4]uint64)(unsafe.Pointer(coeffs))
	if words[0]|words[1]|words[2]|words[3] == 0 {
		return
	}
	// Touch the extreme index the asm will write so an out-of-range block panics
	// here in Go rather than corrupting memory from assembly.
	_ = plane[(y+3)*stride+x+3]
	elossyAddTransformAsm(unsafe.Pointer(&plane[y*stride+x]), stride, coeffs)
}
