//go:build !arm64 && !amd64

package webp

func elossyZigzagLast(levels, zigzagged *[16]int16, first int) int {
	return elossyZigzagLastGo(levels, zigzagged, first)
}
