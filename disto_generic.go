//go:build !arm64

package webp

func elossyTDistoBlocks(src []uint8, srcStride, srcX, srcY int, pred []uint8, predStride, cols, rows int) uint64 {
	return elossyTDistoBlocksGo(src, srcStride, srcX, srcY, pred, predStride, cols, rows)
}

func elossyTDisto4x4(src []uint8, srcStride, srcX, srcY int, pred []uint8, predStride, predX, predY int) uint32 {
	return elossyTDisto4x4Go(src, srcStride, srcX, srcY, pred, predStride, predX, predY)
}

func elossyTDisto4x4Contiguous(src []uint8, srcStride, srcX, srcY int, pred *[16]uint8) uint32 {
	return elossyTDisto4x4ContiguousGo(src, srcStride, srcX, srcY, pred)
}
