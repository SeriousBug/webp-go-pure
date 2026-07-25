package webp

import "testing"

func TestQuantReciprocalsMatchDivision(t *testing.T) {
	for q := 1; q < elossyQuantMaxStep; q++ {
		reciprocal := &elossyReciprocals[q]
		for n := 0; n < elossyQuantMaxDividend; n++ {
			got := uint32(uint64(n)*uint64(reciprocal.multiplier)) >> reciprocal.shift
			if want := uint32(n / q); got != want {
				t.Fatalf("q=%d n=%d: reciprocal gave %d, division gives %d", q, n, got, want)
			}
		}
	}
}

func TestQuantizeCoefficientMatchesDivision(t *testing.T) {
	for _, quant := range []uint16{4, 5, 7, 8, 17, 63, 64, 65, 157, 284, 314, 440, 511} {
		for coeff := -32768; coeff <= 32767; coeff++ {
			gotLevel, gotDequant := elossyQuantizeCoefficient(int16(coeff), quant)

			sign, abs := int32(1), int32(coeff)
			if abs < 0 {
				sign, abs = -1, -abs
			}
			level := (abs + int32(quant)>>1) / int32(quant)
			if level > 2047 {
				level = 2047
			}
			level *= sign
			if int32(gotLevel) != level || gotDequant != int16(level*int32(quant)) {
				t.Fatalf("quant=%d coeff=%d: got (%d,%d), want (%d,%d)",
					quant, coeff, gotLevel, gotDequant, level, int16(level*int32(quant)))
			}
		}
	}
}
