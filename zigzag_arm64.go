//go:build arm64

package webp

// elossyZigzagBytes is the zigzag permutation as TBL byte indices: output lane
// s takes the two bytes of input lane elossyZigzag[s].
var elossyZigzagBytes = [32]uint8{
	0, 1, 2, 3, 8, 9, 16, 17, 10, 11, 4, 5, 6, 7, 12, 13,
	18, 19, 24, 25, 26, 27, 20, 21, 14, 15, 22, 23, 28, 29, 30, 31,
}

//go:noescape
func elossyZigzagLastAsm(levels, zigzagged *[16]int16, firstMask uint64) int

func elossyZigzagLast(levels, zigzagged *[16]int16, first int) int {
	firstMask := ^uint64(0)
	if first != 0 {
		firstMask = ^uint64(0xFF)
	}
	last := elossyZigzagLastAsm(levels, zigzagged, firstMask)
	if last < first {
		return first - 1
	}
	return last
}
