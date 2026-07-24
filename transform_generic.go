//go:build !arm64

package webp

func elossyAddTransform(plane []uint8, stride, x, y int, coeffs *[16]int16) {
	elossyAddTransformGo(plane, stride, x, y, coeffs)
}

func elossyForwardTransformAt(src []uint8, srcStride, srcX, srcY int, pred []uint8, predStride, predX, predY int) [16]int16 {
	return elossyForwardTransformAtGo(src, srcStride, srcX, srcY, pred, predStride, predX, predY)
}
