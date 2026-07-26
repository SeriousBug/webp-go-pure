//go:build amd64

package webp

//go:noescape
func elossyZigzagLastAsm(levels, zigzagged *[16]int16, firstMask uint64) int

func elossyZigzagLast(levels, zigzagged *[16]int16, first int) int {
	firstMask := ^uint64(0)
	if first != 0 {
		firstMask = ^uint64(3)
	}
	last := elossyZigzagLastAsm(levels, zigzagged, firstMask)
	if last < first {
		return first - 1
	}
	return last
}
