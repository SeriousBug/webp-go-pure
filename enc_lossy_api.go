package webp

import "sort"

// elossyCoeffsProba0 is the default coefficient probability table as the named
// module type, so it can be passed by pointer to the token encoders.
var elossyCoeffsProba0 elossyCoeffProbTables = coeffsProba0

// elossyBuildVp8Frame builds a raw VP8 frame from the already encoded partitions.
func elossyBuildVp8Frame(width, height int, partition0, tokenPartition []byte) ([]byte, error) {
	if len(partition0) > elossyMaxPartition0Length {
		return nil, encBitstream("VP8 partition 0 overflow")
	}

	payloadSize := 10 + len(partition0) + len(tokenPartition)
	data := newByteWriter(payloadSize)
	frameBits := (uint32(len(partition0)) << 5) | (1 << 4)
	data.writeU24LE(frameBits)
	data.writeBytes([]byte{0x9d, 0x01, 0x2a})
	data.writeU16LE(uint16(width))
	data.writeU16LE(uint16(height))
	data.writeBytes(partition0)
	data.writeBytes(tokenPartition)
	return data.intoBytes(), nil
}

// elossyBuildCandidateVp8Frame builds a VP8 frame for a candidate mode/filter combination.
func elossyBuildCandidateVp8Frame(width, height, mbWidth, mbHeight int, candidate *elossyEncodedLossyCandidate, filter *elossyFilterConfig) ([]byte, error) {
	segment := elossySegmentWithUniformFilter(&candidate.segment, filter.level)
	partition0 := elossyEncodePartition0(mbWidth, mbHeight, candidate.baseQuant, &segment, filter, &candidate.probabilities, candidate.modes)
	return elossyBuildVp8Frame(width, height, partition0, candidate.tokenPartition)
}

// elossyRunCandidatePass searches one candidate under the given probability
// table and returns the resulting candidate alongside the table the emitted
// tokens imply.
func elossyRunCandidatePass(width, height int, source *elossyPlanes, mbWidth, mbHeight int, profile *elossyLossySearchProfile, segment *elossySegmentConfig, segmentQuants *[numMbSegments]elossyQuantMatrices, table *elossyCoeffProbTables) (elossyEncodedLossyCandidate, []elossyMacroblockCoeffs, elossyCoeffProbTables) {
	var stats elossyCoeffStats
	partition, reconstructed, modes, coeffs := elossyEncodeTokenPartition(source, mbWidth, mbHeight, profile, segment, segmentQuants, table, &stats)
	return elossyEncodedLossyCandidate{
		baseQuant:      segment.quantizer[0],
		segment:        *segment,
		probabilities:  *table,
		modes:          modes,
		tokenPartition: partition,
		distortion:     elossyReconstructionSse(source, &reconstructed, width, height),
	}, coeffs, elossyFinalizeTokenProbabilities(&stats)
}

// elossyEncodeLossyCandidate encodes one lossy candidate and captures its token
// partition, probabilities, and modes. searchTable is the probability table the
// search scores rates against; passing a table converged by an earlier cheap
// pass avoids repeating that pass.
func elossyEncodeLossyCandidate(width, height int, source *elossyPlanes, mbWidth, mbHeight int, profile *elossyLossySearchProfile, segment *elossySegmentConfig, searchTable *elossyCoeffProbTables) (elossyEncodedLossyCandidate, error) {
	segmentQuants := elossyBuildSegmentQuantizers(segment)

	if !profile.updateProbabilities {
		candidate, _, _ := elossyRunCandidatePass(width, height, source, mbWidth, mbHeight, profile, segment, &segmentQuants, &elossyCoeffsProba0)
		candidate.probabilities = coeffsProba0
		return candidate, nil
	}

	// The search scores rates against a probability table, so running it under
	// the default table and then re-estimating leaves the decisions matched to
	// a table that is no longer in use. Converge the table with a cheap search
	// first, spend the real search against it once, then replay the chosen
	// levels under the table those levels imply.
	table := elossyCoeffsProba0
	if searchTable != nil {
		table = *searchTable
	} else {
		priorProfile := elossyStatsProfile(profile)
		_, _, table = elossyRunCandidatePass(width, height, source, mbWidth, mbHeight, &priorProfile, segment, &segmentQuants, &elossyCoeffsProba0)
	}

	candidate, coeffs, emitted := elossyRunCandidatePass(width, height, source, mbWidth, mbHeight, profile, segment, &segmentQuants, &table)
	if emitted != table {
		candidate.tokenPartition = elossyReemitTokenPartition(mbWidth, mbHeight, len(candidate.tokenPartition), coeffs, &emitted)
		candidate.probabilities = emitted
	}
	return candidate, nil
}

// elossyReconstructionSse measures the encoder's own reconstruction against the
// source over the visible region, ignoring macroblock padding.
func elossyReconstructionSse(source *elossyPlanes, reconstructed *elossyPlanes, width, height int) uint64 {
	uvWidth := (width + 1) / 2
	uvHeight := (height + 1) / 2
	return elossyPlaneSseRegion(source.y, source.yStride, reconstructed.y, reconstructed.yStride, width, height) +
		elossyPlaneSseRegion(source.u, source.uvStride, reconstructed.u, reconstructed.uvStride, uvWidth, uvHeight) +
		elossyPlaneSseRegion(source.v, source.uvStride, reconstructed.v, reconstructed.uvStride, uvWidth, uvHeight)
}

// elossyFinalizeLossyCandidate finalizes the lossy candidate by choosing the best
// filter configuration.
func elossyFinalizeLossyCandidate(width, height int, source *elossyPlanes, mbWidth, mbHeight int, baseQuant int32, optimizationLevel uint8, candidate *elossyEncodedLossyCandidate) ([]byte, error) {
	mbCount := mbWidth * mbHeight
	if !elossyUseExhaustiveFilterSearch(optimizationLevel, mbCount) {
		filter := elossyHeuristicFilter(baseQuant)
		return elossyBuildCandidateVp8Frame(width, height, mbWidth, mbHeight, candidate, &filter)
	}

	filters := elossyFilterCandidates(baseQuant)
	var bestDistortion uint64
	var bestLen int
	var bestVp8 []byte
	found := false
	for i := range filters {
		vp8, err := elossyBuildCandidateVp8Frame(width, height, mbWidth, mbHeight, candidate, &filters[i])
		if err != nil {
			return nil, err
		}
		distortion, err := elossyYuvSse(source, width, height, vp8)
		if err != nil {
			return nil, err
		}
		replace := !found ||
			distortion < bestDistortion ||
			(distortion == bestDistortion && len(vp8) < bestLen)
		if replace {
			bestDistortion = distortion
			bestLen = len(vp8)
			bestVp8 = vp8
			found = true
		}
	}

	if !found {
		return nil, encBitstream("lossy filter search produced no output")
	}
	return bestVp8, nil
}

// elossyShortlistCandidates ranks segmentation candidates with the cheap
// stats-profile search and returns the indices worth searching in full, along
// with the probability table each cheap pass converged on so the full search
// does not have to repeat it.
//
// The cheap pass is run for its own sake regardless: the full search needs a
// converged table to score rates against. Ranking on its output costs only the
// frame build, and spares the fully exhaustive candidate sets several complete
// searches whose result was never going to be selected.
func elossyShortlistCandidates(width, height int, source *elossyPlanes, mbWidth, mbHeight int, profile *elossyLossySearchProfile, candidates []elossySegmentConfig, baseQuant int32, filter *elossyFilterConfig) ([]int, []*elossyCoeffProbTables, error) {
	if len(candidates) <= elossyMaxFullSearchCandidates || !profile.updateProbabilities {
		indices := make([]int, len(candidates))
		tables := make([]*elossyCoeffProbTables, len(candidates))
		for i := range candidates {
			indices[i] = i
		}
		return indices, tables, nil
	}

	statsProfile := elossyStatsProfile(profile)
	type ranked struct {
		index int
		cost  uint64
		table elossyCoeffProbTables
	}
	scored := make([]ranked, 0, len(candidates))
	for i := range candidates {
		segmentQuants := elossyBuildSegmentQuantizers(&candidates[i])
		candidate, _, table := elossyRunCandidatePass(width, height, source, mbWidth, mbHeight, &statsProfile, &candidates[i], &segmentQuants, &elossyCoeffsProba0)
		frame, err := elossyBuildCandidateVp8Frame(width, height, mbWidth, mbHeight, &candidate, filter)
		if err != nil {
			return nil, nil, err
		}
		scored = append(scored, ranked{
			index: i,
			cost:  elossyFrameRdCost(candidate.distortion, len(frame), baseQuant),
			table: table,
		})
	}
	sort.SliceStable(scored, func(a, b int) bool { return scored[a].cost < scored[b].cost })

	scored = scored[:elossyMaxFullSearchCandidates]
	indices := make([]int, len(scored))
	tables := make([]*elossyCoeffProbTables, len(scored))
	for i := range scored {
		indices[i] = scored[i].index
		tables[i] = &scored[i].table
	}
	return indices, tables, nil
}

// EncodeLossyRgbaToVp8WithOptions encodes RGBA pixels to a raw lossy VP8 frame
// payload with explicit options.
func encodeLossyRgbaToVp8WithOptions(width, height int, rgba []byte, options *LossyOptions) ([]byte, error) {
	if err := elossyValidateRgba(width, height, rgba); err != nil {
		return nil, err
	}
	if err := elossyValidateOptions(options); err != nil {
		return nil, err
	}

	mbWidth := (width + 15) >> 4
	mbHeight := (height + 15) >> 4
	baseQuant := elossyBaseQuantizerFromQuality(options.Quality)
	profile := elossySearchProfile(options.Effort)
	source := elossyRgbaToYuv420(width, height, rgba, mbWidth, mbHeight)
	candidates := elossyBuildSegmentCandidates(&source, mbWidth, mbHeight, baseQuant, options.Effort)

	// Candidates are ranked on rate *and* distortion using a cheap frame built
	// with the heuristic filter; only the winner pays for the filter search.
	heuristic := elossyHeuristicFilter(baseQuant)
	shortlist, searchTables, err := elossyShortlistCandidates(width, height, &source, mbWidth, mbHeight, &profile, candidates, baseQuant, &heuristic)
	if err != nil {
		return nil, err
	}

	var best elossyEncodedLossyCandidate
	var bestCost uint64
	found := false
	for i, index := range shortlist {
		candidate, err := elossyEncodeLossyCandidate(width, height, &source, mbWidth, mbHeight, &profile, &candidates[index], searchTables[i])
		if err != nil {
			return nil, err
		}
		probe, err := elossyBuildCandidateVp8Frame(width, height, mbWidth, mbHeight, &candidate, &heuristic)
		if err != nil {
			return nil, err
		}
		cost := elossyFrameRdCost(candidate.distortion, len(probe), baseQuant)
		if !found || cost < bestCost {
			bestCost = cost
			best = candidate
			found = true
		}
	}

	if !found {
		return nil, encBitstream("lossy candidate search produced no output")
	}
	return elossyFinalizeLossyCandidate(width, height, &source, mbWidth, mbHeight, baseQuant, options.Effort, &best)
}

// EncodeLossyRgbaToVp8 encodes RGBA pixels to a raw lossy VP8 frame payload.
func encodeLossyRgbaToVp8(width, height int, rgba []byte) ([]byte, error) {
	options := elossyDefaultOptions()
	return encodeLossyRgbaToVp8WithOptions(width, height, rgba, &options)
}

// EncodeLossyRgbaToWebpWithOptions encodes RGBA pixels to a still lossy WebP
// container with explicit options.
func encodeLossyRgbaToWebpWithOptions(width, height int, rgba []byte, options *LossyOptions) ([]byte, error) {
	return encodeLossyRgbaToWebpWithOptionsAndExif(width, height, rgba, options, nil)
}

// EncodeLossyRgbaToWebpWithOptionsAndExif encodes RGBA pixels to a still lossy
// WebP container with explicit options and EXIF.
func encodeLossyRgbaToWebpWithOptionsAndExif(width, height int, rgba []byte, options *LossyOptions, exif []byte) ([]byte, error) {
	vp8, err := encodeLossyRgbaToVp8WithOptions(width, height, rgba, options)
	if err != nil {
		return nil, err
	}
	return wrapStillWebp(stillImageChunk{
		fourcc:   [4]byte{'V', 'P', '8', ' '},
		payload:  vp8,
		width:    width,
		height:   height,
		hasAlpha: false,
	}, exif)
}

// EncodeLossyRgbaToWebp encodes RGBA pixels to a still lossy WebP container.
func encodeLossyRgbaToWebp(width, height int, rgba []byte) ([]byte, error) {
	options := elossyDefaultOptions()
	return encodeLossyRgbaToWebpWithOptions(width, height, rgba, &options)
}

// EncodeLossyImageToWebpWithOptions encodes an ImageBuffer to a still lossy WebP
// container with explicit options.
func encodeLossyImageToWebpWithOptions(image *Image, options *LossyOptions) ([]byte, error) {
	return encodeLossyImageToWebpWithOptionsAndExif(image, options, nil)
}

// EncodeLossyImageToWebpWithOptionsAndExif encodes an ImageBuffer to a still lossy
// WebP container with explicit options and EXIF.
func encodeLossyImageToWebpWithOptionsAndExif(image *Image, options *LossyOptions, exif []byte) ([]byte, error) {
	return encodeLossyRgbaToWebpWithOptionsAndExif(image.Width, image.Height, image.RGBA, options, exif)
}

// EncodeLossyImageToWebp encodes an ImageBuffer to a still lossy WebP container.
func encodeLossyImageToWebp(image *Image) ([]byte, error) {
	options := elossyDefaultOptions()
	return encodeLossyImageToWebpWithOptions(image, &options)
}
