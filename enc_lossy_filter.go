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

// elossyFilterInto applies the loop filter to a copy of the reconstruction,
// macroblock by macroblock in the same order a decoder would.
func elossyFilterInto(planes *lossyPlanes, reconstructed *elossyPlanes, mbWidth, mbHeight int, filter *elossyFilterConfig, modes []elossyMacroblockMode) {
	copy(planes.y, reconstructed.y)
	copy(planes.u, reconstructed.u)
	copy(planes.v, reconstructed.v)

	kind := elossyFilterKind(filter)
	if kind == filterOff {
		return
	}

	baseLevel := elossyFilterBaseLevel(filter)
	for mbY := 0; mbY < mbHeight; mbY++ {
		for mbX := 0; mbX < mbWidth; mbX++ {
			mode := &modes[mbY*mbWidth+mbX]
			inner := mode.luma == bPred || !mode.skip
			info, ok := lossyFilterInfoForLevel(baseLevel, filter.sharpness, inner)
			if !ok {
				continue
			}
			lossyFilterMacroblockWith(kind, planes, mbX, mbY, &info)
		}
	}
}

// elossyFilteredDistortion scores a filter config against the source, using
// scratch as its working copy.
func elossyFilteredDistortion(source *elossyPlanes, reconstructed *elossyPlanes, scratch *lossyPlanes, width, height, mbWidth, mbHeight int, filter *elossyFilterConfig, modes []elossyMacroblockMode) uint64 {
	elossyFilterInto(scratch, reconstructed, mbWidth, mbHeight, filter, modes)
	planes := scratch
	uvWidth := (width + 1) / 2
	uvHeight := (height + 1) / 2
	return elossyPlaneSseRegion(source.y, source.yStride, planes.y, planes.yStride, width, height) +
		elossyPlaneSseRegion(source.u, source.uvStride, planes.u, planes.uvStride, uvWidth, uvHeight) +
		elossyPlaneSseRegion(source.v, source.uvStride, planes.v, planes.uvStride, uvWidth, uvHeight)
}
