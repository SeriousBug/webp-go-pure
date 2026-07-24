package webp

import "math"

func elossyBlockHasNonZero(levels *[16]int16, first int) bool {
	for i := first; i < 16; i++ {
		if levels[i] != 0 {
			return true
		}
	}
	return false
}

// compute_skip_probability returns (prob, ok).
func elossyComputeSkipProbability(modes []elossyMacroblockMode) (uint8, bool) {
	const skipProbaThreshold uint8 = 250
	total := len(modes)
	skipCount := 0
	for i := range modes {
		if modes[i].skip {
			skipCount++
		}
	}
	if total == 0 || skipCount == 0 {
		return 0, false
	}
	nonSkip := total - skipCount
	probZero := ((nonSkip * 255) + total/2) / total
	if probZero < 1 {
		probZero = 1
	} else if probZero > 254 {
		probZero = 254
	}
	probability := uint8(probZero)
	if probability < skipProbaThreshold {
		return probability, true
	}
	return 0, false
}

func elossyIntra4TreeContains(node int8, mode uint8) bool {
	if node <= 0 {
		return uint8(-node) == mode
	}
	n := int(node)
	return elossyIntra4TreeContains(yModesIntra4[2*n], mode) ||
		elossyIntra4TreeContains(yModesIntra4[2*n+1], mode)
}

func elossyWalkIntra4ModeBits(topMode, leftMode, mode uint8, emit func(bit bool, prob uint8)) {
	probs := &bmodesProba[topMode][leftMode]
	var walk func(node int)
	walk = func(node int) {
		left := yModesIntra4[2*node]
		right := yModesIntra4[2*node+1]
		if elossyIntra4TreeContains(left, mode) {
			emit(false, probs[node])
			if left > 0 {
				walk(int(left))
			}
		} else {
			emit(true, probs[node])
			if right > 0 {
				walk(int(right))
			}
		}
	}
	walk(0)
}

func elossyEncodeIntra4Mode(writer *vp8BoolWriter, topMode, leftMode, mode uint8) {
	elossyWalkIntra4ModeBits(topMode, leftMode, mode, func(bit bool, prob uint8) {
		writer.putBit(bit, prob)
	})
}

func elossyIntra4ModeRate(topMode, leftMode, mode uint8) uint32 {
	var rate uint32
	elossyWalkIntra4ModeBits(topMode, leftMode, mode, func(bit bool, prob uint8) {
		rate += elossyBitCost(bit, prob)
	})
	return rate
}

func elossyUpdateModeCache(mode *elossyMacroblockMode, top []uint8, left *[4]uint8) {
	if mode.luma == B_PRED {
		for subY := 0; subY < 4; subY++ {
			ymode := left[subY]
			for subX := 0; subX < 4; subX++ {
				ymode = mode.subLuma[subY*4+subX]
				top[subX] = ymode
			}
			left[subY] = ymode
		}
	} else {
		for i := range top {
			top[i] = mode.luma
		}
		for i := range left {
			left[i] = mode.luma
		}
	}
}

func elossyCoeffProbs(probabilities *elossyCoeffProbTables, coeffType, coeffIndex, ctx int) *[11]uint8 {
	return &probabilities[coeffType][elossyBands[coeffIndex]][ctx]
}

func elossyLastNonZero(levels *[16]int16, first int) int {
	for scan := 15; scan >= first; scan-- {
		if levels[elossyZigzag[scan]] != 0 {
			return scan
		}
	}
	return first - 1
}

func elossyWriteLargeValue(writer *vp8BoolWriter, value uint32, probs *[11]uint8) {
	if !writer.putBit(value > 4, probs[3]) {
		if writer.putBit(value != 2, probs[4]) {
			writer.putBit(value == 4, probs[5])
		}
		return
	}

	if !writer.putBit(value > 10, probs[6]) {
		if !writer.putBit(value > 6, probs[7]) {
			writer.putBit(value == 6, 159)
		} else {
			writer.putBit(value >= 9, 165)
			writer.putBit((value&1) == 0, 145)
		}
		return
	}

	var residue, mask uint32
	var table []uint8
	if value < 19 {
		writer.putBit(false, probs[8])
		writer.putBit(false, probs[9])
		residue, mask, table = value-11, 1<<2, elossyCat3[:]
	} else if value < 35 {
		writer.putBit(false, probs[8])
		writer.putBit(true, probs[9])
		residue, mask, table = value-19, 1<<3, elossyCat4[:]
	} else if value < 67 {
		writer.putBit(true, probs[8])
		writer.putBit(false, probs[10])
		residue, mask, table = value-35, 1<<4, elossyCat5[:]
	} else {
		writer.putBit(true, probs[8])
		writer.putBit(true, probs[10])
		residue, mask, table = value-67, 1<<10, elossyCat6[:]
	}

	for _, prob := range table {
		if prob == 0 {
			break
		}
		writer.putBit((residue&mask) != 0, prob)
		mask >>= 1
	}
}

func elossyLargeValueRate(value uint32, probs *[11]uint8) uint32 {
	var rate uint32
	if value <= 4 {
		rate += elossyBitCost(false, probs[3])
		notTwo := value != 2
		rate += elossyBitCost(notTwo, probs[4])
		if notTwo {
			rate += elossyBitCost(value == 4, probs[5])
		}
		return rate
	}

	rate += elossyBitCost(true, probs[3])
	if value <= 10 {
		rate += elossyBitCost(false, probs[6])
		gt6 := value > 6
		rate += elossyBitCost(gt6, probs[7])
		if !gt6 {
			rate += elossyBitCost(value == 6, 159)
		} else {
			rate += elossyBitCost(value >= 9, 165)
			rate += elossyBitCost((value&1) == 0, 145)
		}
		return rate
	}

	rate += elossyBitCost(true, probs[6])
	if value < 19 {
		rate += elossyBitCost(false, probs[8])
		rate += elossyBitCost(false, probs[9])
		residue := value - 11
		mask := uint32(1 << 2)
		for _, prob := range elossyCat3 {
			if prob == 0 {
				break
			}
			rate += elossyBitCost((residue&mask) != 0, prob)
			mask >>= 1
		}
	} else if value < 35 {
		rate += elossyBitCost(false, probs[8])
		rate += elossyBitCost(true, probs[9])
		residue := value - 19
		mask := uint32(1 << 3)
		for _, prob := range elossyCat4 {
			if prob == 0 {
				break
			}
			rate += elossyBitCost((residue&mask) != 0, prob)
			mask >>= 1
		}
	} else if value < 67 {
		rate += elossyBitCost(true, probs[8])
		rate += elossyBitCost(false, probs[10])
		residue := value - 35
		mask := uint32(1 << 4)
		for _, prob := range elossyCat5 {
			if prob == 0 {
				break
			}
			rate += elossyBitCost((residue&mask) != 0, prob)
			mask >>= 1
		}
	} else {
		rate += elossyBitCost(true, probs[8])
		rate += elossyBitCost(true, probs[10])
		residue := value - 67
		mask := uint32(1 << 10)
		for _, prob := range elossyCat6 {
			if prob == 0 {
				break
			}
			rate += elossyBitCost((residue&mask) != 0, prob)
			mask >>= 1
		}
	}
	return rate
}

func elossyCoefficientsRate(probabilities *elossyCoeffProbTables, coeffType, ctx, first int, levels *[16]int16) uint32 {
	last := elossyLastNonZero(levels, first)
	scan := first
	probs := elossyCoeffProbs(probabilities, coeffType, scan, ctx)
	rate := elossyBitCost(last >= scan, probs[0])
	if last < scan {
		return rate
	}

	for scan < 16 {
		coeff := levels[elossyZigzag[scan]]
		rate += elossyBitCost(coeff != 0, probs[1])
		scan++
		if coeff == 0 {
			if scan == 16 {
				return rate
			}
			probs = elossyCoeffProbs(probabilities, coeffType, scan, 0)
			continue
		}

		value := uint32(int32(coeff))
		if coeff < 0 {
			value = uint32(-int32(coeff))
		}
		gt1 := value > 1
		rate += elossyBitCost(gt1, probs[2])
		nextCtx := 1
		if gt1 {
			rate += elossyLargeValueRate(value, probs)
			nextCtx = 2
		}
		rate += elossyBitCost(coeff < 0, 128)

		if scan == 16 {
			return rate
		}
		probs = elossyCoeffProbs(probabilities, coeffType, scan, nextCtx)
		rate += elossyBitCost(last >= scan, probs[0])
		if last < scan {
			return rate
		}
	}
	return rate
}

func elossyEncodeCoefficients(writer *vp8BoolWriter, probabilities *elossyCoeffProbTables, coeffType, ctx, first int, levels *[16]int16) bool {
	last := elossyLastNonZero(levels, first)
	scan := first
	probs := elossyCoeffProbs(probabilities, coeffType, scan, ctx)
	if !writer.putBit(last >= scan, probs[0]) {
		return false
	}

	for scan < 16 {
		coeff := levels[elossyZigzag[scan]]
		writer.putBit(coeff != 0, probs[1])
		scan++
		if coeff == 0 {
			if scan == 16 {
				return false
			}
			probs = elossyCoeffProbs(probabilities, coeffType, scan, 0)
			continue
		}

		value := uint32(int32(coeff))
		if coeff < 0 {
			value = uint32(-int32(coeff))
		}
		nextCtx := 1
		if writer.putBit(value > 1, probs[2]) {
			elossyWriteLargeValue(writer, value, probs)
			nextCtx = 2
		}
		writer.putBit(coeff < 0, 128)

		if scan == 16 {
			return true
		}
		probs = elossyCoeffProbs(probabilities, coeffType, scan, nextCtx)
		if !writer.putBit(last >= scan, probs[0]) {
			return true
		}
	}
	return true
}

func elossyRecordStat(bit bool, stat *uint32) {
	if *stat >= 0xfffe0000 {
		*stat = ((*stat + 1) >> 1) & 0x7fff7fff
	}
	b := uint32(0)
	if bit {
		b = 1
	}
	*stat += 0x00010000 + b
}

func elossyRecordLargeValue(stats *[NUM_PROBAS]uint32, value uint32) {
	gt4 := value > 4
	elossyRecordStat(gt4, &stats[3])
	if !gt4 {
		ne2 := value != 2
		elossyRecordStat(ne2, &stats[4])
		if ne2 {
			elossyRecordStat(value == 4, &stats[5])
		}
		return
	}

	gt10 := value > 10
	elossyRecordStat(gt10, &stats[6])
	if !gt10 {
		elossyRecordStat(value > 6, &stats[7])
		return
	}

	if value < 19 {
		elossyRecordStat(false, &stats[8])
		elossyRecordStat(false, &stats[9])
	} else if value < 35 {
		elossyRecordStat(false, &stats[8])
		elossyRecordStat(true, &stats[9])
	} else if value < 67 {
		elossyRecordStat(true, &stats[8])
		elossyRecordStat(false, &stats[10])
	} else {
		elossyRecordStat(true, &stats[8])
		elossyRecordStat(true, &stats[10])
	}
}

func elossyRecordCoefficientsStats(stats *elossyCoeffStats, coeffType, ctx, first int, levels *[16]int16) bool {
	last := elossyLastNonZero(levels, first)
	scan := first
	currentCtx := ctx
	elossyRecordStat(last >= scan, &stats[coeffType][elossyBands[scan]][currentCtx][0])
	if last < scan {
		return false
	}

	for scan < 16 {
		coeff := levels[elossyZigzag[scan]]
		band := elossyBands[scan]
		elossyRecordStat(coeff != 0, &stats[coeffType][band][currentCtx][1])
		scan++
		if coeff == 0 {
			if scan == 16 {
				return false
			}
			currentCtx = 0
			continue
		}

		value := uint32(int32(coeff))
		if coeff < 0 {
			value = uint32(-int32(coeff))
		}
		gt1 := value > 1
		elossyRecordStat(gt1, &stats[coeffType][band][currentCtx][2])
		if gt1 {
			elossyRecordLargeValue(&stats[coeffType][band][currentCtx], value)
		}

		if scan == 16 {
			return true
		}
		if gt1 {
			currentCtx = 2
		} else {
			currentCtx = 1
		}
		elossyRecordStat(last >= scan, &stats[coeffType][elossyBands[scan]][currentCtx][0])
		if last < scan {
			return true
		}
	}
	return true
}

// elossyEntropyCost[p] is the cost in 1/256-bit units of a decision whose
// probability of the observed outcome is p/256, i.e. round(-log2(p/256)*256).
// It mirrors libwebp's VP8EntropyCost table. Index 0 is clamped to p=1 so the
// cost stays finite. Precomputing this removes math.Log2 from the RD-optimization
// inner loop, where elossyBitCost is called millions of times per encode.
var elossyEntropyCost [256]uint16

func init() {
	for p := 0; p < 256; p++ {
		pp := p
		if pp < 1 {
			pp = 1
		}
		elossyEntropyCost[p] = uint16((-math.Log2(float64(pp)/256.0))*256.0 + 0.5)
	}
}

// bit_cost returns the bit cost of a boolean decision at the given probability.
func elossyBitCost(bit bool, prob uint8) uint32 {
	if bit {
		return uint32(elossyEntropyCost[255-prob])
	}
	return uint32(elossyEntropyCost[prob])
}

func elossyCalcTokenProbability(nb, total uint32) uint8 {
	if nb == 0 {
		return 255
	}
	return uint8(255 - nb*255/total)
}

// branch_cost returns the modeled cost of one probability branch.
func elossyBranchCost(nb, total uint32, prob uint8) uint32 {
	return nb*elossyBitCost(true, prob) + (total-nb)*elossyBitCost(false, prob)
}

func elossyFinalizeTokenProbabilities(stats *elossyCoeffStats) elossyCoeffProbTables {
	probabilities := coeffsProba0
	for t := 0; t < NUM_TYPES; t++ {
		for b := 0; b < NUM_BANDS; b++ {
			for c := 0; c < NUM_CTX; c++ {
				for p := 0; p < NUM_PROBAS; p++ {
					stat := stats[t][b][c][p]
					nb := stat & 0xffff
					total := stat >> 16
					updateProb := coeffsUpdateProba[t][b][c][p]
					oldProb := coeffsProba0[t][b][c][p]
					newProb := elossyCalcTokenProbability(nb, total)
					oldCost := elossyBranchCost(nb, total, oldProb) + elossyBitCost(false, updateProb)
					newCost := elossyBranchCost(nb, total, newProb) + elossyBitCost(true, updateProb) + 8*256
					if oldCost > newCost {
						probabilities[t][b][c][p] = newProb
					} else {
						probabilities[t][b][c][p] = oldProb
					}
				}
			}
		}
	}
	return probabilities
}

func elossyEncodePartition0(mbWidth, mbHeight int, baseQuant uint8, segment *elossySegmentConfig, filter *elossyFilterConfig, probabilities *elossyCoeffProbTables, modes []elossyMacroblockMode) []byte {
	writer := newVp8BoolWriter(mbWidth * mbHeight)
	writer.putBitUniform(false)
	writer.putBitUniform(false)

	writer.putBitUniform(segment.useSegment)
	if segment.useSegment {
		writer.putBitUniform(segment.updateMap)
		writer.putBitUniform(true)
		writer.putBitUniform(true)
		for _, quant := range segment.quantizer {
			writer.putSignedBits(int32(quant), 7)
		}
		for _, strength := range segment.filterStrength {
			writer.putSignedBits(int32(strength), 6)
		}
		if segment.updateMap {
			for _, prob := range segment.probs {
				if writer.putBitUniform(prob != 255) {
					writer.putBits(uint32(prob), 8)
				}
			}
		}
	}

	writer.putBitUniform(filter.simple)
	writer.putBits(uint32(filter.level), 6)
	writer.putBits(uint32(filter.sharpness), 3)
	writer.putBitUniform(false)

	writer.putBits(0, 2)
	writer.putBits(uint32(baseQuant), 7)
	for i := 0; i < 5; i++ {
		writer.putSignedBits(0, 4)
	}
	writer.putBitUniform(false)

	for t := 0; t < NUM_TYPES; t++ {
		for b := 0; b < NUM_BANDS; b++ {
			for c := 0; c < NUM_CTX; c++ {
				for p := 0; p < NUM_PROBAS; p++ {
					update := probabilities[t][b][c][p] != coeffsProba0[t][b][c][p]
					writer.putBit(update, coeffsUpdateProba[t][b][c][p])
					if update {
						writer.putBits(uint32(probabilities[t][b][c][p]), 8)
					}
				}
			}
		}
	}
	skipProbability, hasSkip := elossyComputeSkipProbability(modes)
	if hasSkip {
		writer.putBitUniform(true)
		writer.putBits(uint32(skipProbability), 8)
	} else {
		writer.putBitUniform(false)
	}

	topModes := make([]uint8, mbWidth*4)
	var leftModes [4]uint8
	for index := range modes {
		mode := &modes[index]
		if index%mbWidth == 0 {
			leftModes = [4]uint8{}
		}
		if segment.updateMap {
			if writer.putBit(mode.segment >= 2, segment.probs[0]) {
				writer.putBit(mode.segment == 3, segment.probs[2])
			} else {
				writer.putBit(mode.segment == 1, segment.probs[1])
			}
		}
		if hasSkip {
			writer.putBit(mode.skip, skipProbability)
		}
		mbX := index % mbWidth
		top := topModes[mbX*4 : mbX*4+4]
		if mode.luma == B_PRED {
			writer.putBit(false, 145)
			for subY := 0; subY < 4; subY++ {
				ymode := leftModes[subY]
				for subX := 0; subX < 4; subX++ {
					subMode := mode.subLuma[subY*4+subX]
					elossyEncodeIntra4Mode(writer, top[subX], ymode, subMode)
					top[subX] = subMode
					ymode = subMode
				}
				leftModes[subY] = ymode
			}
		} else {
			writer.putBit(true, 145)
			switch mode.luma {
			case DC_PRED:
				writer.putBit(false, 156)
				writer.putBit(false, 163)
			case V_PRED:
				writer.putBit(false, 156)
				writer.putBit(true, 163)
			case H_PRED:
				writer.putBit(true, 156)
				writer.putBit(false, 128)
			case TM_PRED:
				writer.putBit(true, 156)
				writer.putBit(true, 128)
			default:
				panic("unsupported luma mode")
			}
			for i := range top {
				top[i] = mode.luma
			}
			for i := range leftModes {
				leftModes[i] = mode.luma
			}
		}
		switch mode.chroma {
		case DC_PRED:
			writer.putBit(false, 142)
		case V_PRED:
			writer.putBit(true, 142)
			writer.putBit(false, 114)
		case H_PRED:
			writer.putBit(true, 142)
			writer.putBit(true, 114)
			writer.putBit(false, 183)
		case TM_PRED:
			writer.putBit(true, 142)
			writer.putBit(true, 114)
			writer.putBit(true, 183)
		default:
			panic("unsupported chroma mode")
		}
	}

	return writer.finish()
}

func elossyEncodeMacroblock(writer *vp8BoolWriter, probabilities *elossyCoeffProbTables, source *elossyPlanes, reconstructed *elossyPlanes, mbX, mbY int, profile *elossyLossySearchProfile, mode elossyMacroblockMode, quant *elossyQuantMatrices, top *elossyNonZeroContext, left *elossyNonZeroContext, stats *elossyCoeffStats) bool {
	yX := mbX * 16
	yY := mbY * 16
	uvX := mbX * 8
	uvY := mbY * 8
	isI4x4 := mode.luma == B_PRED
	rd := elossyBuildRdMultipliers(quant)

	if !isI4x4 {
		elossyPredictBlock(reconstructed.y, reconstructed.yStride, reconstructed.yStride, yX, yY, mode.luma, 16)
	}
	elossyPredictBlock(reconstructed.u, reconstructed.uvStride, reconstructed.uvStride, uvX, uvY, mode.chroma, 8)
	elossyPredictBlock(reconstructed.v, reconstructed.uvStride, reconstructed.uvStride, uvX, uvY, mode.chroma, 8)

	var yLevels [16][16]int16
	var yCoeffs [16][16]int16
	var y2Levels [16]int16

	if isI4x4 {
		for subY := 0; subY < 4; subY++ {
			for subX := 0; subX < 4; subX++ {
				block := subY*4 + subX
				blockX := yX + subX*4
				blockY := yY + subY*4
				elossyPredictLuma4Block(reconstructed.y, reconstructed.yStride, reconstructed.yStride, blockX, blockY, mode.subLuma[block])
				predictionBlock := elossyCopyBlock4(reconstructed.y, reconstructed.yStride, blockX, blockY)
				coeffs := elossyForwardTransform(source.y, source.yStride, reconstructed.y, reconstructed.yStride, blockX, blockY)
				ctx := int((left.nz>>subY)&1) + int((top.nz>>subX)&1)
				levels, _ := elossyQuantizeBlock(&coeffs, quant.y1[0], quant.y1[1], 0)
				coeffsR := elossyMaybeRefineLevels(profile.refineI4Final, source.y, source.yStride, blockX, blockY, &predictionBlock, probabilities, 3, ctx, 0, quant.y1[0], quant.y1[1], rd.i4, &levels)
				yLevels[block] = levels
				yCoeffs[block] = coeffsR
				elossyAddTransform(reconstructed.y, reconstructed.yStride, blockX, blockY, &yCoeffs[block])
			}
		}
	} else {
		var yDc [16]int16
		refineTnz := top.nz & 0x0f
		refineLnz := left.nz & 0x0f
		for subY := 0; subY < 4; subY++ {
			l := refineLnz & 1
			for subX := 0; subX < 4; subX++ {
				block := subY*4 + subX
				coeffs := elossyForwardTransform(source.y, source.yStride, reconstructed.y, reconstructed.yStride, yX+subX*4, yY+subY*4)
				yDc[block] = coeffs[0]
				acOnly := coeffs
				acOnly[0] = 0
				predictionBlock := elossyCopyBlock4(reconstructed.y, reconstructed.yStride, yX+subX*4, yY+subY*4)
				levels, _ := elossyQuantizeBlock(&acOnly, quant.y1[0], quant.y1[1], 1)
				ctx := int(l + (refineTnz & 1))
				coeffsR := elossyMaybeRefineLevels(profile.refineI16, source.y, source.yStride, yX+subX*4, yY+subY*4, &predictionBlock, probabilities, 0, ctx, 1, quant.y1[0], quant.y1[1], rd.i16, &levels)
				yLevels[block] = levels
				yCoeffs[block] = coeffsR
				hasAc := uint8(0)
				if elossyBlockHasNonZero(&yLevels[block], 1) {
					hasAc = 1
				}
				l = hasAc
				refineTnz = (refineTnz >> 1) | (hasAc << 7)
			}
			refineTnz >>= 4
			refineLnz = (refineLnz >> 1) | (l << 7)
		}

		y2Input := elossyForwardWht(&yDc)
		predictionBlock := elossyCopyBlock16(reconstructed.y, reconstructed.yStride, yX, yY)
		levels, _ := elossyQuantizeBlock(&y2Input, quant.y2[0], quant.y2[1], 0)
		y2Coeffs := elossyMaybeRefineY2Levels(profile, source.y, source.yStride, yX, yY, &predictionBlock, &yCoeffs, probabilities, int(top.nzDc+left.nzDc), quant.y2[0], quant.y2[1], rd.i16, &levels)
		y2Levels = levels
		y2Dc := elossyInverseWht(&y2Coeffs)
		for block := 0; block < 16; block++ {
			yCoeffs[block][0] = y2Dc[block]
		}
	}

	var uLevels [4][16]int16
	var uCoeffs [4][16]int16
	for subY := 0; subY < 2; subY++ {
		for subX := 0; subX < 2; subX++ {
			block := subY*2 + subX
			coeffs := elossyForwardTransform(source.u, source.uvStride, reconstructed.u, reconstructed.uvStride, uvX+subX*4, uvY+subY*4)
			predictionBlock := elossyCopyBlock4(reconstructed.u, reconstructed.uvStride, uvX+subX*4, uvY+subY*4)
			levels, _ := elossyQuantizeBlock(&coeffs, quant.uv[0], quant.uv[1], 0)
			coeffsR := elossyMaybeRefineLevels(profile.refineChroma, source.u, source.uvStride, uvX+subX*4, uvY+subY*4, &predictionBlock, probabilities, 2, 0, 0, quant.uv[0], quant.uv[1], rd.uv, &levels)
			uLevels[block] = levels
			uCoeffs[block] = coeffsR
		}
	}

	var vLevels [4][16]int16
	var vCoeffs [4][16]int16
	for subY := 0; subY < 2; subY++ {
		for subX := 0; subX < 2; subX++ {
			block := subY*2 + subX
			coeffs := elossyForwardTransform(source.v, source.uvStride, reconstructed.v, reconstructed.uvStride, uvX+subX*4, uvY+subY*4)
			predictionBlock := elossyCopyBlock4(reconstructed.v, reconstructed.uvStride, uvX+subX*4, uvY+subY*4)
			levels, _ := elossyQuantizeBlock(&coeffs, quant.uv[0], quant.uv[1], 0)
			coeffsR := elossyMaybeRefineLevels(profile.refineChroma, source.v, source.uvStride, uvX+subX*4, uvY+subY*4, &predictionBlock, probabilities, 2, 0, 0, quant.uv[0], quant.uv[1], rd.uv, &levels)
			vLevels[block] = levels
			vCoeffs[block] = coeffsR
		}
	}

	yAllZero := true
	if isI4x4 {
		for i := range yLevels {
			if elossyBlockHasNonZero(&yLevels[i], 0) {
				yAllZero = false
				break
			}
		}
	} else {
		if elossyBlockHasNonZero(&y2Levels, 0) {
			yAllZero = false
		} else {
			for i := range yLevels {
				if elossyBlockHasNonZero(&yLevels[i], 1) {
					yAllZero = false
					break
				}
			}
		}
	}
	uAllZero := true
	for i := range uLevels {
		if elossyBlockHasNonZero(&uLevels[i], 0) {
			uAllZero = false
			break
		}
	}
	vAllZero := true
	for i := range vLevels {
		if elossyBlockHasNonZero(&vLevels[i], 0) {
			vAllZero = false
			break
		}
	}
	skip := yAllZero && uAllZero && vAllZero
	if skip {
		top.nz = 0
		left.nz = 0
		if !isI4x4 {
			top.nzDc = 0
			left.nzDc = 0
		}
		return true
	}

	var coeffType, first int
	if isI4x4 {
		coeffType, first = 3, 0
	} else {
		ctx := int(top.nzDc + left.nzDc)
		var hasY2 bool
		if stats != nil {
			elossyRecordCoefficientsStats(stats, 1, ctx, 0, &y2Levels)
			hasY2 = elossyEncodeCoefficients(writer, probabilities, 1, ctx, 0, &y2Levels)
		} else {
			hasY2 = elossyEncodeCoefficients(writer, probabilities, 1, ctx, 0, &y2Levels)
		}
		nzDc := uint8(0)
		if hasY2 {
			nzDc = 1
		}
		top.nzDc = nzDc
		left.nzDc = nzDc
		coeffType, first = 0, 1
	}

	tnz := top.nz & 0x0f
	lnz := left.nz & 0x0f
	for subY := 0; subY < 4; subY++ {
		l := lnz & 1
		for subX := 0; subX < 4; subX++ {
			block := subY*4 + subX
			ctx := int(l + (tnz & 1))
			var hasAc bool
			if stats != nil {
				elossyRecordCoefficientsStats(stats, coeffType, ctx, first, &yLevels[block])
				hasAc = elossyEncodeCoefficients(writer, probabilities, coeffType, ctx, first, &yLevels[block])
			} else {
				hasAc = elossyEncodeCoefficients(writer, probabilities, coeffType, ctx, first, &yLevels[block])
			}
			l = 0
			if hasAc {
				l = 1
			}
			tnz = (tnz >> 1) | (l << 7)
		}
		tnz >>= 4
		lnz = (lnz >> 1) | (l << 7)
	}
	outTNz := tnz
	outLNz := lnz >> 4

	tnzU := top.nz >> 4
	lnzU := left.nz >> 4
	for subY := 0; subY < 2; subY++ {
		l := lnzU & 1
		for subX := 0; subX < 2; subX++ {
			block := subY*2 + subX
			ctx := int(l + (tnzU & 1))
			var encoded bool
			if stats != nil {
				elossyRecordCoefficientsStats(stats, 2, ctx, 0, &uLevels[block])
				encoded = elossyEncodeCoefficients(writer, probabilities, 2, ctx, 0, &uLevels[block])
			} else {
				encoded = elossyEncodeCoefficients(writer, probabilities, 2, ctx, 0, &uLevels[block])
			}
			hasCoeffs := uint8(0)
			if encoded {
				hasCoeffs = 1
			}
			l = hasCoeffs
			tnzU = (tnzU >> 1) | (hasCoeffs << 3)
		}
		tnzU >>= 2
		lnzU = (lnzU >> 1) | (l << 5)
	}
	outTNz |= tnzU << 4
	outLNz |= lnzU & 0xf0

	tnzV := top.nz >> 6
	lnzV := left.nz >> 6
	for subY := 0; subY < 2; subY++ {
		l := lnzV & 1
		for subX := 0; subX < 2; subX++ {
			block := subY*2 + subX
			ctx := int(l + (tnzV & 1))
			var encoded bool
			if stats != nil {
				elossyRecordCoefficientsStats(stats, 2, ctx, 0, &vLevels[block])
				encoded = elossyEncodeCoefficients(writer, probabilities, 2, ctx, 0, &vLevels[block])
			} else {
				encoded = elossyEncodeCoefficients(writer, probabilities, 2, ctx, 0, &vLevels[block])
			}
			hasCoeffs := uint8(0)
			if encoded {
				hasCoeffs = 1
			}
			l = hasCoeffs
			tnzV = (tnzV >> 1) | (hasCoeffs << 3)
		}
		tnzV >>= 2
		lnzV = (lnzV >> 1) | (l << 5)
	}
	outTNz |= (tnzV << 4) << 2
	outLNz |= (lnzV & 0xf0) << 2

	top.nz = outTNz
	left.nz = outLNz

	if !isI4x4 {
		for subY := 0; subY < 4; subY++ {
			for subX := 0; subX < 4; subX++ {
				block := subY*4 + subX
				elossyAddTransform(reconstructed.y, reconstructed.yStride, yX+subX*4, yY+subY*4, &yCoeffs[block])
			}
		}
	}

	for subY := 0; subY < 2; subY++ {
		for subX := 0; subX < 2; subX++ {
			block := subY*2 + subX
			elossyAddTransform(reconstructed.u, reconstructed.uvStride, uvX+subX*4, uvY+subY*4, &uCoeffs[block])
			elossyAddTransform(reconstructed.v, reconstructed.uvStride, uvX+subX*4, uvY+subY*4, &vCoeffs[block])
		}
	}

	return false
}

func elossyEncodeTokenPartition(source *elossyPlanes, mbWidth, mbHeight int, profile *elossyLossySearchProfile, segment *elossySegmentConfig, segmentQuants *[NUM_MB_SEGMENTS]elossyQuantMatrices, probabilities *elossyCoeffProbTables, stats *elossyCoeffStats) ([]byte, elossyPlanes, []elossyMacroblockMode) {
	writer := newVp8BoolWriter(len(source.y) / 4)
	reconstructed := elossyEmptyReconstructedPlanes(mbWidth, mbHeight)
	topContexts := make([]elossyNonZeroContext, mbWidth)
	topModes := make([]uint8, mbWidth*4)
	modes := make([]elossyMacroblockMode, 0, mbWidth*mbHeight)
	var segmentRd [NUM_MB_SEGMENTS]elossyRdMultipliers
	for index := 0; index < NUM_MB_SEGMENTS; index++ {
		segmentRd[index] = elossyBuildRdMultipliers(&segmentQuants[index])
	}

	for mbY := 0; mbY < mbHeight; mbY++ {
		var leftContext elossyNonZeroContext
		var leftModes [4]uint8
		for mbX := 0; mbX < mbWidth; mbX++ {
			index := mbY*mbWidth + mbX
			segmentID := int(segment.segments[index])
			quant := &segmentQuants[segmentID]
			rd := &segmentRd[segmentID]
			top := topModes[mbX*4 : mbX*4+4]
			mode := elossyChooseMacroblockMode(source, &reconstructed, mbX, mbY, profile, quant, rd, probabilities, &topContexts[mbX], &leftContext, top, &leftModes)
			mode.segment = uint8(segmentID)
			elossyUpdateModeCache(&mode, top, &leftModes)
			mode.skip = elossyEncodeMacroblock(writer, probabilities, source, &reconstructed, mbX, mbY, profile, mode, quant, &topContexts[mbX], &leftContext, stats)
			modes = append(modes, mode)
		}
	}

	return writer.finish(), reconstructed, modes
}
