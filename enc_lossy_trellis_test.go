package webp

import (
	"math/rand"
	"testing"
)

// trellisActivePositions bounds how many scan positions carry a coefficient in
// the generated blocks. Brute force is exponential in it.
const trellisActivePositions = 10

// trellisBruteForce scores every assignment of {truncate, round} over the
// positions the trellis considers, plus the empty block, and returns the levels
// of the best one. The trellis is a dynamic program over exactly this space, so
// the two must agree.
func trellisBruteForce(coeffs *[16]int16, model *elossyRateModel, coeffType, ctx0, first int, dcQuant, acQuant uint16, lambda uint32) [16]int16 {
	quantOf := func(index int) int32 {
		if index == 0 {
			return int32(dcQuant)
		}
		return int32(acQuant)
	}

	// The empty block is the score to beat, priced as the trellis prices it.
	best := elossyTrellisScore(lambda, elossyBitCost(false, model.probs[coeffType][elossyBands[first]][ctx0][0]), 0)
	var bestLevels [16]int16
	found := false

	// A block ends at its last non-zero level and everything past it is
	// dropped, so the enumeration is over (end position, levels up to it)
	// rather than over levels alone: an assignment cannot express a block that
	// stops before a position whose coefficient rounds to non-zero.
	for end := first; end < first+trellisActivePositions && end < 16; end++ {
		for assignment := 0; assignment < 1<<uint(end-first+1); assignment++ {
			var levels [16]int16
			var distortion int64
			legal := true
			for scan := first; scan <= end; scan++ {
				index := elossyZigzag[scan]
				quant := quantOf(index)
				coeff := int32(coeffs[index])
				negative := coeff < 0
				if negative {
					coeff = -coeff
				}
				truncated := coeff / quant
				rounded := (coeff + quant>>1) / quant
				level := truncated + int32(assignment>>uint(scan-first)&1)
				if level > rounded {
					// Not a state the trellis can occupy.
					legal = false
					break
				}
				if scan == end && level == 0 {
					// The block's end has to hold a level.
					legal = false
					break
				}
				residual := coeff - level*quant
				distortion += elossyTrellisWeight[index] * int64(residual*residual-coeff*coeff)
				if negative {
					level = -level
				}
				levels[index] = int16(level)
			}
			if !legal {
				continue
			}

			// The trellis prices every coefficient's sign as if it were
			// positive, as libwebp's does; the shared rate estimator charges
			// negatives the difference, so take it back out.
			rate := elossyCoefficientsRate(model, coeffType, ctx0, first, &levels)
			for _, level := range levels {
				if level < 0 {
					rate -= elossySignRateDelta
				}
			}
			if score := elossyTrellisScore(lambda, rate, distortion); score < best {
				best = score
				bestLevels = levels
				found = true
			}
		}
	}
	if !found {
		return [16]int16{}
	}
	return bestLevels
}

func TestTrellisQuantizeMatchesBruteForce(t *testing.T) {
	model := elossyBuildRateModel(&elossyCoeffsProba0)
	rng := rand.New(rand.NewSource(7))

	for trial := 0; trial < 3000; trial++ {
		dcQuant := uint16(1 + rng.Intn(60))
		acQuant := uint16(1 + rng.Intn(60))
		first := rng.Intn(2)
		ctx0 := rng.Intn(3)
		coeffType := 3
		if first == 1 {
			coeffType = 0
		}
		lambda := uint32(1 + rng.Intn(3000))

		var coeffs [16]int16
		for scan := first; scan < first+trellisActivePositions && scan < 16; scan++ {
			if rng.Intn(3) == 0 {
				continue
			}
			coeffs[elossyZigzag[scan]] = int16(rng.Intn(6*int(acQuant)+8) - 3*int(acQuant) - 4)
		}

		var levels [16]int16
		elossyTrellisQuantize(&coeffs, model, coeffType, ctx0, first, dcQuant, acQuant, lambda, &levels)
		want := trellisBruteForce(&coeffs, model, coeffType, ctx0, first, dcQuant, acQuant, lambda)
		if levels != want {
			t.Fatalf("trial %d: dc=%d ac=%d first=%d ctx0=%d lambda=%d\ncoeffs %v\ngot    %v\nwant   %v",
				trial, dcQuant, acQuant, first, ctx0, lambda, coeffs, levels, want)
		}
	}
}
