//go:build amd64

package webp

//go:noescape
func elossyQuantizeAcAsm(coeffs, out *[16]int16, bias, multiplier, shift uint32)

func elossyQuantizeBlock(coeffs *[16]int16, dcQuant, acQuant uint16, first int) [16]int16 {
	if acQuant == 0 || acQuant >= elossyQuantMaxStep || first > 1 {
		return elossyQuantizeBlockGo(coeffs, dcQuant, acQuant, first)
	}
	var levels [16]int16
	ac := &elossyReciprocals[acQuant]
	elossyQuantizeAcAsm(coeffs, &levels, uint32(acQuant>>1), ac.multiplier, ac.shift)
	if first == 0 {
		levels[0], _ = elossyQuantizeCoefficient(coeffs[0], dcQuant)
	} else {
		levels[0] = 0
	}
	return levels
}
