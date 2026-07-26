package webp

// The filter search scores several loop-filter levels against the source. It
// used to build a frame and decode it back for each one, which redid the
// entropy decode and reconstruction every time even though only the filtering
// differs between levels.
//
// A still image is a single keyframe, and VP8 intra prediction reads unfiltered
// neighbours, so what a decoder shows is exactly the loop filter applied to the
// reconstruction the encoder already holds. Filtering a copy of that gives the
// same answer without decoding anything.

// elossyFilterBaseLevel is the level every macroblock is filtered at under a
// given config. The encoder writes segment filter strengths as absolute deltas
// and sets them all to the same value (elossySegmentWithUniformFilter), and
// never writes per-reference or per-mode deltas, so the base level a decoder
// derives is the config's level whether or not segmentation is on.
func elossyFilterBaseLevel(filter *elossyFilterConfig) int32 {
	return int32(filter.level)
}

func elossyFilterKind(filter *elossyFilterConfig) filterType {
	if filter.level == 0 {
		return filterOff
	}
	if filter.simple {
		return filterSimple
	}
	return filterComplex
}

// elossyNewFilterScratch is the working copy the filter search filters in
// place. The search reuses one across levels rather than copying fresh planes
// per level, which on a large frame is several megabytes each time.
func elossyNewFilterScratch(reconstructed *elossyPlanes, mbWidth, mbHeight int) lossyPlanes {
	return lossyPlanes{
		width:    mbWidth * 16,
		height:   mbHeight * 16,
		yStride:  reconstructed.yStride,
		uvStride: reconstructed.uvStride,
		y:        make([]byte, len(reconstructed.y)),
		u:        make([]byte, len(reconstructed.u)),
		v:        make([]byte, len(reconstructed.v)),
	}
}

// elossyFilterScratch takes the working copy from a dropped pass's
// reconstruction when there is one: the planes match in size and the filter
// search overwrites them wholesale.
func (s *elossyEncodeScratch) filterScratch(reconstructed *elossyPlanes) lossyPlanes {
	if last := len(s.free) - 1; last >= 0 {
		planes := &s.free[last].reconstructed
		s.free = s.free[:last]
		return lossyPlanes{
			width:    s.mbWidth * 16,
			height:   s.mbHeight * 16,
			yStride:  planes.yStride,
			uvStride: planes.uvStride,
			y:        planes.y,
			u:        planes.u,
			v:        planes.v,
		}
	}
	return elossyNewFilterScratch(reconstructed, s.mbWidth, s.mbHeight)
}

// elossyFilterRows applies the loop filter to a copy of the reconstruction's
// macroblock rows [mbYStart, mbYEnd), macroblock by macroblock in the same
// order a decoder would, copying just those rows. The first row's top edge is
// left unfiltered: a band that does not start at the frame's top has no
// neighbour above it in the copy.
func elossyFilterRows(planes *lossyPlanes, reconstructed *elossyPlanes, mbWidth, mbYStart, mbYEnd int, filter *elossyFilterConfig, modes []elossyMacroblockMode) {
	yFrom, yTo := mbYStart*16*planes.yStride, mbYEnd*16*planes.yStride
	uvFrom, uvTo := mbYStart*8*planes.uvStride, mbYEnd*8*planes.uvStride
	copy(planes.y[yFrom:yTo], reconstructed.y[yFrom:yTo])
	copy(planes.u[uvFrom:uvTo], reconstructed.u[uvFrom:uvTo])
	copy(planes.v[uvFrom:uvTo], reconstructed.v[uvFrom:uvTo])

	kind := elossyFilterKind(filter)
	if kind == filterOff {
		return
	}

	baseLevel := elossyFilterBaseLevel(filter)
	for mbY := mbYStart; mbY < mbYEnd; mbY++ {
		for mbX := 0; mbX < mbWidth; mbX++ {
			mode := &modes[mbY*mbWidth+mbX]
			inner := mode.luma == bPred || !mode.skip
			info, ok := lossyFilterInfoForLevel(baseLevel, filter.sharpness, inner)
			if !ok {
				continue
			}
			lossyFilterMacroblockEdges(kind, planes, mbX, mbY, &info, mbY > mbYStart)
		}
	}
}

// elossyFilterScoreBands is how the filter search samples the frame: it scores
// that many macroblock rows out of every stride of them. Trialing a level over
// the whole frame spends most of its time filtering macroblocks whose
// contribution every level pays for alike; a band sample spread down the frame
// ordered the levels identically on the test images for a third of the work.
// Small frames, where the search costs little anyway, are scored whole so their
// choice stays exact.
func elossyFilterScoreBands(mbHeight int) (int, int) {
	const bandRows, bandStride = 4, 16
	if mbHeight < 2*bandStride {
		return mbHeight, mbHeight
	}
	return bandRows, bandStride
}

// elossyFilteredDistortion scores a filter config against the source, using
// scratch as its working copy.
func elossyFilteredDistortion(source *elossyPlanes, reconstructed *elossyPlanes, scratch *lossyPlanes, width, height, mbWidth, mbHeight int, filter *elossyFilterConfig, modes []elossyMacroblockMode) uint64 {
	band, stride := elossyFilterScoreBands(mbHeight)
	uvWidth := (width + 1) / 2
	var total uint64
	for start := 0; start < mbHeight; start += stride {
		end := min(start+band, mbHeight)
		// The band's first row is filtered only to give the scored rows the
		// neighbour their top edge filter reads, so it is not scored itself.
		context := start
		if start > 0 {
			context--
		}
		elossyFilterRows(scratch, reconstructed, mbWidth, context, end, filter, modes)

		yRows := min(end*16, height) - start*16
		if yRows <= 0 {
			continue
		}
		uvRows := min(end*8, (height+1)/2) - start*8
		yOffset := start * 16 * scratch.yStride
		uvOffset := start * 8 * scratch.uvStride
		total += elossyPlaneSseRegion(source.y[yOffset:], source.yStride, scratch.y[yOffset:], scratch.yStride, width, yRows)
		if uvRows > 0 {
			total += elossyPlaneSseRegion(source.u[uvOffset:], source.uvStride, scratch.u[uvOffset:], scratch.uvStride, uvWidth, uvRows) +
				elossyPlaneSseRegion(source.v[uvOffset:], source.uvStride, scratch.v[uvOffset:], scratch.uvStride, uvWidth, uvRows)
		}
	}
	return total
}
