package webp

// Trellis-quantization of a 4x4 coefficient block, after libwebp's
// TrellisQuantizeBlock.
//
// The search is a Viterbi pass over the zigzag scan: each position keeps the
// best partial score for each candidate level, and the level chosen at one
// position only reaches the next through the coding context it hands forward.
// Distortion is measured on the quantization error in the transform domain, so
// a trial costs a multiply rather than an inverse transform and a pixel-domain
// SSE against the source.

// elossyTrellisWeight weights each position's quantization error by how much
// the inverse transform spreads it into the block, so low frequencies are
// protected more than high ones. libwebp's kWeightTrellis.
var elossyTrellisWeight = [16]int64{
	30, 27, 19, 11,
	27, 24, 17, 10,
	19, 17, 12, 8,
	11, 10, 8, 6,
}

const elossyTrellisMaxLevel = 2047

// elossyTrellisMaxCost marks a dead node: no live path can reach it.
const elossyTrellisMaxCost = int64(1) << 55

func elossyTrellisScore(lambda uint32, rate uint32, distortion int64) int64 {
	return int64(rate)*int64(lambda) + 256*distortion
}

// elossyTrellisQuantize quantizes coeffs into levels, choosing each level to
// minimize the block's rate-distortion score rather than rounding it
// independently. It returns the dequantized coefficients, both arrays in
// natural (non-zigzag) order.
//
// A coefficient quantizes to some level under round-half rounding; each
// position offers that level and the one below it, which is what truncating the
// same coefficient would have produced. The truncated level is always
// reachable, so the node holding it is never dead and needs no liveness check;
// only the rounded-up node can fall out of the search.
func elossyTrellisQuantize(coeffs *[16]int16, model *elossyRateModel, coeffType, ctx0, first int, dcQuant, acQuant uint16, lambda uint32, levels *[16]int16) [16]int16 {
	var dequantized [16]int16

	// A coefficient at or below half a step rounds to zero, so the positions
	// past the highest one above it cannot hold a level, and a block with none
	// of them quantizes to nothing at all.
	last := first - 1
	zeroThreshold := int32(acQuant >> 1)
	for scan := 15; scan >= first; scan-- {
		coeff := int32(coeffs[elossyZigzag[scan]])
		if coeff > zeroThreshold || coeff < -zeroThreshold {
			last = scan
			break
		}
	}
	if last < first {
		elossyClearLevels(levels, first)
		return dequantized
	}
	// One position past the last that can hold a level is enough: a longer
	// block is priced but can never win.
	if last < 15 {
		last++
	}

	acReciprocal := elossyReciprocals[acQuant]
	dcReciprocal := elossyReciprocals[dcQuant]
	typeLevels := model.level[coeffType][:]
	typeProbs := &model.probs[coeffType]

	// The best predecessor of each node, two bits per scan position. Levels and
	// signs are recomputed on the way out rather than stored, since a node's
	// level is fixed by its position and index.
	var paths uint32

	firstBand := elossyBands[first]
	lastProba := typeProbs[firstBand][ctx0][0]

	// Coding nothing at all is the score every path has to beat.
	bestScore := elossyTrellisScore(lambda, elossyBitCost(false, lastProba), 0)
	bestPosition, bestNode, bestPrev := -1, 0, 0

	var entryRate uint32
	if ctx0 == 0 {
		entryRate = elossyBitCost(true, lastProba)
	}
	entryScore := elossyTrellisScore(lambda, entryRate, 0)
	entryBase := int32((firstBand*numCtx + ctx0) * elossyLevelTableStride)

	// State of the two nodes at the position just below the one being scored:
	// the running best score, and where the level costs it hands forward live.
	prevScore0, prevScore1 := entryScore, entryScore
	prevBase0, prevBase1 := entryBase, entryBase

	for scan := first; scan <= last; scan++ {
		index := elossyZigzag[scan]
		quant := int32(acQuant)
		reciprocal := acReciprocal
		if index == 0 {
			quant = int32(dcQuant)
			reciprocal = dcReciprocal
		}
		coeff := int32(coeffs[index])
		if coeff < 0 {
			coeff = -coeff
		}
		truncated := int32(uint32(coeff) * reciprocal.multiplier >> reciprocal.shift)
		rounded := int32(uint32(coeff+quant>>1) * reciprocal.multiplier >> reciprocal.shift)
		if truncated > elossyTrellisMaxLevel {
			truncated = elossyTrellisMaxLevel
		}
		if rounded > elossyTrellisMaxLevel {
			rounded = elossyTrellisMaxLevel
		}

		band := elossyBands[scan+1]
		weight := elossyTrellisWeight[index]
		bandBase := int32(band*numCtx) * elossyLevelTableStride
		prev1Live := prevScore1 < elossyTrellisMaxCost

		var score0, score1 int64
		var base0, base1 int32

		// The truncated level.
		{
			level := truncated
			score := prevScore0 + elossyTrellisScore(lambda, elossyTrellisLevelRate(typeLevels, model, coeffType, prevBase0, level), 0)
			from := 0
			if prev1Live {
				if alt := prevScore1 + elossyTrellisScore(lambda, elossyTrellisLevelRate(typeLevels, model, coeffType, prevBase1, level), 0); alt < score {
					score, from = alt, 1
				}
			}
			residual := coeff - level*quant
			score += elossyTrellisScore(lambda, 0, weight*int64(residual*residual-coeff*coeff))
			if from != 0 {
				paths |= 1 << uint(scan*2)
			}
			score0 = score

			ctx := level
			if ctx > 2 {
				ctx = 2
			}
			base0 = bandBase + ctx*elossyLevelTableStride

			if level != 0 {
				var endRate uint32
				if scan < 15 {
					endRate = elossyBitCost(false, typeProbs[band][ctx][0])
				}
				if total := score + elossyTrellisScore(lambda, endRate, 0); total < bestScore {
					bestScore, bestPosition, bestNode, bestPrev = total, scan, 0, from
				}
			}
		}

		// The rounded-up level, when rounding reaches one.
		if truncated < rounded {
			level := truncated + 1
			score := prevScore0 + elossyTrellisScore(lambda, elossyTrellisLevelRate(typeLevels, model, coeffType, prevBase0, level), 0)
			from := 0
			if prev1Live {
				if alt := prevScore1 + elossyTrellisScore(lambda, elossyTrellisLevelRate(typeLevels, model, coeffType, prevBase1, level), 0); alt < score {
					score, from = alt, 1
				}
			}
			residual := coeff - level*quant
			score += elossyTrellisScore(lambda, 0, weight*int64(residual*residual-coeff*coeff))
			if from != 0 {
				paths |= 1 << uint(scan*2+1)
			}
			score1 = score

			ctx := level
			if ctx > 2 {
				ctx = 2
			}
			base1 = bandBase + ctx*elossyLevelTableStride

			// The rounded-up level is never zero, so it can always end a block.
			var endRate uint32
			if scan < 15 {
				endRate = elossyBitCost(false, typeProbs[band][ctx][0])
			}
			if total := score + elossyTrellisScore(lambda, endRate, 0); total < bestScore {
				bestScore, bestPosition, bestNode, bestPrev = total, scan, 1, from
			}
		} else {
			score1 = elossyTrellisMaxCost
		}

		prevScore0, prevScore1 = score0, score1
		prevBase0, prevBase1 = base0, base1
	}

	elossyClearLevels(levels, first)
	if bestPosition < 0 {
		return dequantized
	}

	// The best predecessor of the terminal node is the one found while scoring
	// it as a terminal, which is not necessarily the one recorded for it as a
	// non-terminal, so patch it in before unwinding.
	shift := uint(bestPosition*2 + bestNode)
	paths = paths&^(1<<shift) | uint32(bestPrev)<<shift

	node := bestNode
	for scan := bestPosition; scan >= first; scan-- {
		index := elossyZigzag[scan]
		quant := int32(acQuant)
		reciprocal := acReciprocal
		if index == 0 {
			quant = int32(dcQuant)
			reciprocal = dcReciprocal
		}
		coeff := int32(coeffs[index])
		negative := coeff < 0
		if negative {
			coeff = -coeff
		}
		level := int32(uint32(coeff) * reciprocal.multiplier >> reciprocal.shift)
		if level > elossyTrellisMaxLevel {
			level = elossyTrellisMaxLevel
		}
		level += int32(node)
		if negative {
			level = -level
		}
		levels[index] = int16(level)
		dequantized[index] = int16(level * quant)
		node = int(paths>>uint(scan*2+node)) & 1
	}
	return dequantized
}

// elossyClearLevels zeroes every level the trellis is responsible for. Blocks
// coded with a separate DC keep theirs.
func elossyClearLevels(levels *[16]int16, first int) {
	dc := levels[0]
	*levels = [16]int16{}
	if first != 0 {
		levels[0] = dc
	}
}

// elossyTrellisLevelRate prices magnitude level under the level costs the
// predecessor node hands forward, including the non-zero flag and, for ctx > 0,
// the preceding "not end of block" bit.
func elossyTrellisLevelRate(typeLevels []uint16, model *elossyRateModel, coeffType int, base int32, level int32) uint32 {
	if level <= elossyMaxTabulatedLevel {
		return uint32(typeLevels[base+level])
	}
	// Rare enough to unpack the band and context the base was built from.
	entry := base / elossyLevelTableStride
	return elossyUntabulatedLevelRate(model, coeffType, int(entry)/numCtx, int(entry)%numCtx, uint32(level))
}
