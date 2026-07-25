package webp

// elossyRatePrefix caches the coefficient-rate walk's running state at every
// scan position so a level-refinement trial only re-walks from the position it
// changed. The greedy refinement descends the zigzag order and only ever lowers
// magnitudes, so positions below the one under trial are final: their share of
// the rate, and the context and band they hand forward, can be computed once
// per block instead of once per trial.
type elossyRatePrefix struct {
	model     *elossyRateModel
	coeffType int
	first     int
	entryCtx  int

	zigzagged [16]int16
	last      int

	// Index p holds the state entering scan position p, so index 16 is the
	// state after the whole block.
	rate [17]uint32
	ctx  [17]uint8
	band [17]uint8

	// tail[t] prices scan positions t through last, including the end-of-block
	// bit when the block stops early. Positions above the one under trial are
	// final, so the context entering t is fixed by the value below it and the
	// tail needs no context dimension: only the position adjacent to the trial
	// sees a context the trial can change.
	tail [18]uint32
	// tailFrom is the lowest index whose tail entry is current.
	tailFrom int
}

func elossyNextCtx(value uint32) int {
	if value > 2 {
		return 2
	}
	return int(value)
}

// levelRate prices one coefficient, including its sign bit.
func (p *elossyRatePrefix) levelRate(band, ctx int, value uint32, negative int32) uint32 {
	rate := uint32(negative&1) * elossySignRateDelta
	if value <= elossyMaxTabulatedLevel {
		rate += uint32(p.model.level[p.coeffType][(band*numCtx+ctx)*elossyLevelTableStride+int(value)])
	} else {
		rate += elossyUntabulatedLevelRate(p.model, p.coeffType, band, ctx, value)
	}
	return rate
}

// ctxInto reports the context entering scan position t, which the final value
// at t-1 determines.
func (p *elossyRatePrefix) ctxInto(t int) int {
	coeff := int32(p.zigzagged[t-1])
	negative := coeff >> 31
	return elossyNextCtx(uint32((coeff ^ negative) - negative))
}

// endOfBlockRate prices the end-of-block bit that follows the block's last
// coefficient, entering with context ctx.
func (p *elossyRatePrefix) endOfBlockRate(ctx int) uint32 {
	if p.last >= 15 {
		return 0
	}
	return elossyBitCost(false, p.model.probs[p.coeffType][elossyBands[p.last+1]][ctx][0])
}

// resetTail seeds the tail table with the end-of-block term just past the
// block's last coefficient, discarding any stale entries below it.
func (p *elossyRatePrefix) resetTail() {
	end := p.last + 1
	p.tailFrom = end
	if end <= p.first {
		return
	}
	p.tail[end] = p.endOfBlockRate(p.ctxInto(end))
}

// ensureTail extends the tail table down to index t.
func (p *elossyRatePrefix) ensureTail(t int) {
	for p.tailFrom > t {
		scan := p.tailFrom - 1
		coeff := int32(p.zigzagged[scan])
		negative := coeff >> 31
		value := uint32((coeff ^ negative) - negative)
		p.tail[scan] = p.levelRate(elossyBands[scan], p.ctxInto(scan), value, negative) + p.tail[scan+1]
		p.tailFrom = scan
	}
}

func (p *elossyRatePrefix) reset(model *elossyRateModel, coeffType, ctx, first int, levels *[16]int16) {
	p.model = model
	p.coeffType = coeffType
	p.first = first
	p.entryCtx = ctx
	p.last = elossyZigzagLast(levels, &p.zigzagged, first)

	band := elossyBands[first]
	var rate uint32
	if ctx == 0 {
		rate = elossyBitCost(true, model.probs[coeffType][band][ctx][0])
	}

	// Only positions up to the block's end are ever the subject of a trial, and
	// the refinement only shortens the block, so the prefix stops there.
	end := p.last
	if end < first {
		end = first - 1
	}

	typeLevels := model.level[coeffType][:]
	for scan := first; scan <= end; scan++ {
		p.rate[scan] = rate
		p.ctx[scan] = uint8(ctx)
		p.band[scan] = uint8(band)

		coeff := int32(p.zigzagged[scan])
		negative := coeff >> 31
		value := uint32((coeff ^ negative) - negative)
		rate += uint32(negative&1) * elossySignRateDelta
		if value <= elossyMaxTabulatedLevel {
			rate += uint32(typeLevels[(band*numCtx+ctx)*elossyLevelTableStride+int(value)])
		} else {
			rate += elossyUntabulatedLevelRate(model, coeffType, band, ctx, value)
		}
		ctx = int(value)
		if ctx > 2 {
			ctx = 2
		}
		band = elossyBands[scan+1]
	}
	p.rate[end+1] = rate
	p.ctx[end+1] = uint8(ctx)
	p.band[end+1] = uint8(band)

	p.resetTail()
}

// lastWith reports where the block ends once position scan holds value. The
// refinement only ever moves magnitudes toward zero, so the end can recede but
// never extend.
func (p *elossyRatePrefix) lastWith(scan int, value int16) int {
	if value != 0 || scan != p.last {
		return p.last
	}
	last := scan - 1
	for last >= p.first && p.zigzagged[last] == 0 {
		last--
	}
	return last
}

// rateWith prices the block as if position scan held value, leaving the cached
// state untouched.
func (p *elossyRatePrefix) rateWith(scan int, value int16) uint32 {
	last := p.lastWith(scan, value)
	model := p.model
	if last < p.first {
		band := elossyBands[p.first]
		return elossyBitCost(false, model.probs[p.coeffType][band][p.entryCtx][0])
	}

	// A zero at the block's end drops every trailing position, so the answer is
	// the prefix up to the new end plus the end-of-block bit.
	if scan > last {
		rate := p.rate[last+1]
		if last < 15 {
			rate += elossyBitCost(false, model.probs[p.coeffType][int(p.band[last+1])][int(p.ctx[last+1])][0])
		}
		return rate
	}

	coeff := int32(value)
	negative := coeff >> 31
	magnitude := uint32((coeff ^ negative) - negative)
	rate := p.rate[scan] + p.levelRate(int(p.band[scan]), int(p.ctx[scan]), magnitude, negative)

	// The trial only reaches the position just above it, through the context it
	// hands forward; everything past that is already priced in the tail.
	trialCtx := elossyNextCtx(magnitude)
	if scan == last {
		return rate + p.endOfBlockRate(trialCtx)
	}

	nextCoeff := int32(p.zigzagged[scan+1])
	nextNegative := nextCoeff >> 31
	nextMagnitude := uint32((nextCoeff ^ nextNegative) - nextNegative)
	p.ensureTail(scan + 2)
	return rate + p.levelRate(elossyBands[scan+1], trialCtx, nextMagnitude, nextNegative) + p.tail[scan+2]
}

// commit accepts a trial value at position scan.
func (p *elossyRatePrefix) commit(scan int, value int16) {
	last := p.lastWith(scan, value)
	p.zigzagged[scan] = value
	if last != p.last {
		p.last = last
		p.resetTail()
		return
	}
	// The commit changes the context position scan hands upward, so every tail
	// entry at or below scan+1 is stale. At the block's end that includes the
	// end-of-block term itself.
	if scan+2 > p.last+1 {
		p.resetTail()
		return
	}
	if p.tailFrom < scan+2 {
		p.tailFrom = scan + 2
	}
}

// rate0 prices the block as reset left it. The prefix pass already accumulated
// the running rate through the block's end, so this is a lookup.
func (p *elossyRatePrefix) rate0() uint32 {
	if p.last < p.first {
		band := elossyBands[p.first]
		return elossyBitCost(false, p.model.probs[p.coeffType][band][p.entryCtx][0])
	}
	rate := p.rate[p.last+1]
	if p.last < 15 {
		rate += elossyBitCost(false, p.model.probs[p.coeffType][int(p.band[p.last+1])][int(p.ctx[p.last+1])][0])
	}
	return rate
}
