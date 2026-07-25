//go:build arm64

package webp

//go:noescape
func elossyQuantizeAcAsm(coeffs, out *[16]int16, bias, multiplier, negShift uint32)

func elossyQuantizeBlockInto(coeffs *[16]int16, dcQuant, acQuant uint16, first int, levels *[16]int16) {
	if acQuant == 0 || acQuant >= elossyQuantMaxStep || first > 1 {
		*levels = elossyQuantizeBlockGo(coeffs, dcQuant, acQuant, first)
		return
	}
	ac := &elossyReciprocals[acQuant]
	elossyQuantizeAcAsm(coeffs, levels, uint32(acQuant>>1), ac.multiplier, uint32(-int32(ac.shift)))
	if first == 0 {
		levels[0], _ = elossyQuantizeCoefficient(coeffs[0], dcQuant)
	} else {
		levels[0] = 0
	}
}
