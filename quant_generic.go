//go:build !arm64 && !amd64

package webp

func elossyQuantizeBlockInto(coeffs *[16]int16, dcQuant, acQuant uint16, first int, levels *[16]int16) {
	*levels = elossyQuantizeBlockGo(coeffs, dcQuant, acQuant, first)
}
