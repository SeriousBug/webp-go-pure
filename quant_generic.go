//go:build !arm64 && !amd64

package webp

func elossyQuantizeBlock(coeffs *[16]int16, dcQuant, acQuant uint16, first int) [16]int16 {
	return elossyQuantizeBlockGo(coeffs, dcQuant, acQuant, first)
}
