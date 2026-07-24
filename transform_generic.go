//go:build !arm64

package webp

func elossyAddTransform(plane []uint8, stride, x, y int, coeffs *[16]int16) {
	elossyAddTransformGo(plane, stride, x, y, coeffs)
}
