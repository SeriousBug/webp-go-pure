package webp

// Quantization divides a coefficient magnitude by the quantizer step, which is
// only known at run time. Integer division is the single most expensive
// operation in the encoder's inner loop, so each step gets a magic multiplier:
// n/q == (n*multiplier)>>shift for every n the encoder can produce.

// elossyQuantMaxStep is one past the largest quantizer step VP8 can select.
// The largest is the Y2 AC step, acTable[127]*101581>>16 == 440.
const elossyQuantMaxStep = 512

// elossyQuantMaxDividend bounds the dividend: |coeff| for an int16 coefficient
// is at most 32768, plus the rounding term q/2.
const elossyQuantMaxDividend = 32768 + elossyQuantMaxStep/2 + 1

type elossyReciprocal struct {
	multiplier uint32
	shift      uint32
}

var elossyReciprocals = buildQuantReciprocals()

// buildQuantReciprocals picks, per step, the smallest shift for which the
// round-up multiplier is exact over the whole dividend range. With
// m = ceil(2^s/q) and e = m*q - 2^s, (n*m)>>s == n/q for all n < N exactly when
// e*N <= 2^s. The multiplier is additionally kept small enough that n*m fits in
// 32 bits, so SIMD implementations can work in 32-bit lanes.
func buildQuantReciprocals() [elossyQuantMaxStep]elossyReciprocal {
	var table [elossyQuantMaxStep]elossyReciprocal
	const n = uint64(elossyQuantMaxDividend)
	for q := uint64(1); q < elossyQuantMaxStep; q++ {
		found := false
		for shift := uint32(0); shift <= 40; shift++ {
			pow := uint64(1) << shift
			m := (pow + q - 1) / q
			if m*n >= 1<<32 {
				break
			}
			if (m*q-pow)*n <= pow {
				table[q] = elossyReciprocal{multiplier: uint32(m), shift: shift}
				found = true
				break
			}
		}
		if !found {
			panic("no exact reciprocal for quantizer step")
		}
	}
	return table
}
