//go:build amd64

package webp

import "unsafe"

// elossyTDisto4x4Asm scores a 4x4 residual with the weighted Hadamard SATD,
// reading four rows of srcStride-pitched source and four rows of
// predStride-pitched prediction.
//
//go:noescape
func elossyTDisto4x4Asm(srcPtr, predPtr unsafe.Pointer, srcStride, predStride int) uint32

// elossyTDistoBlocksAsm sums elossyTDisto4x4Asm over a cols x rows grid of 4x4
// blocks, rounding each block down on its own.
//
//go:noescape
func elossyTDistoBlocksAsm(srcPtr, predPtr unsafe.Pointer, srcStride, predStride, cols, rows int) uint32

func elossyTDistoBlocks(src []uint8, srcStride, srcX, srcY int, pred []uint8, predStride, cols, rows int) uint64 {
	_ = src[(srcY+rows*4-1)*srcStride+srcX+cols*4-1]
	_ = pred[(rows*4-1)*predStride+cols*4-1]
	return uint64(elossyTDistoBlocksAsm(
		unsafe.Pointer(&src[srcY*srcStride+srcX]),
		unsafe.Pointer(&pred[0]),
		srcStride, predStride, cols, rows,
	))
}

func elossyTDisto4x4(src []uint8, srcStride, srcX, srcY int, pred []uint8, predStride, predX, predY int) uint32 {
	// Touch the extreme indices the asm will read so an out-of-range block
	// panics here in Go rather than reading past the buffer from assembly.
	_ = src[(srcY+3)*srcStride+srcX+3]
	_ = pred[(predY+3)*predStride+predX+3]
	return elossyTDisto4x4Asm(
		unsafe.Pointer(&src[srcY*srcStride+srcX]),
		unsafe.Pointer(&pred[predY*predStride+predX]),
		srcStride, predStride,
	)
}

func elossyTDisto4x4Contiguous(src []uint8, srcStride, srcX, srcY int, pred *[16]uint8) uint32 {
	_ = src[(srcY+3)*srcStride+srcX+3]
	return elossyTDisto4x4Asm(unsafe.Pointer(&src[srcY*srcStride+srcX]), unsafe.Pointer(pred), srcStride, 4)
}
