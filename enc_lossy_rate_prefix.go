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

	typeLevels := model.level[coeffType][:]
	for scan := first; scan < 16; scan++ {
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
	p.rate[16] = rate
	p.ctx[16] = uint8(ctx)
	p.band[16] = uint8(band)
}

// walk prices the block from scan position start onwards, reusing the cached
// prefix for everything below it.
func (p *elossyRatePrefix) walk(changed, last int) uint32 {
	model := p.model
	coeffType := p.coeffType
	if last < p.first {
		band := elossyBands[p.first]
		return elossyBitCost(false, model.probs[coeffType][band][p.entryCtx][0])
	}

	start := changed
	if start > last {
		start = last + 1
	}
	rate := p.rate[start]
	ctx := int(p.ctx[start])
	band := int(p.band[start])

	typeLevels := model.level[coeffType][:]
	for scan := start; scan <= last; scan++ {
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

	if last < 15 {
		rate += elossyBitCost(false, model.probs[coeffType][band][ctx][0])
	}
	return rate
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
	saved := p.zigzagged[scan]
	p.zigzagged[scan] = value
	rate := p.walk(scan, p.lastWith(scan, value))
	p.zigzagged[scan] = saved
	return rate
}

// commit accepts a trial value at position scan.
func (p *elossyRatePrefix) commit(scan int, value int16) {
	p.last = p.lastWith(scan, value)
	p.zigzagged[scan] = value
}

func (p *elossyRatePrefix) rate0() uint32 {
	return p.walk(p.first, p.last)
}
