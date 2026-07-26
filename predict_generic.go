//go:build !arm64 && !amd64

package webp

func elossyTmPredict16(top, left *[16]uint8, topLeft uint8, out *[256]uint8) {
	elossyTmPredict16Go(top, left, topLeft, out)
}

func elossyTmPredict8(top, left *[8]uint8, topLeft uint8, out *[64]uint8) {
	elossyTmPredict8Go(top, left, topLeft, out)
}
