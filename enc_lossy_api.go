package webp

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

// elossyEncodeLossyCandidate encodes one lossy candidate and captures its token
// partition, probabilities, and modes.
func elossyEncodeLossyCandidate(source *elossyPlanes, mbWidth, mbHeight int, profile *elossyLossySearchProfile, segment *elossySegmentConfig) (elossyEncodedLossyCandidate, error) {
	segmentQuants := elossyBuildSegmentQuantizers(segment)
	var probabilities elossyCoeffProbTables
	var modes []elossyMacroblockMode
	var tokenPartition []byte

	if profile.updateProbabilities {
		var stats elossyCoeffStats
		initialPartition, _, initialModes := elossyEncodeTokenPartition(source, mbWidth, mbHeight, profile, segment, &segmentQuants, &elossyCoeffsProba0, &stats)
		probabilities = elossyFinalizeTokenProbabilities(&stats)
		if probabilities == elossyCoeffsProba0 {
			tokenPartition = initialPartition
			modes = initialModes
		} else {
			partition, _, m := elossyEncodeTokenPartition(source, mbWidth, mbHeight, profile, segment, &segmentQuants, &probabilities, nil)
			tokenPartition = partition
			modes = m
		}
	} else {
		partition, _, m := elossyEncodeTokenPartition(source, mbWidth, mbHeight, profile, segment, &segmentQuants, &elossyCoeffsProba0, nil)
		tokenPartition = partition
		probabilities = coeffsProba0
		modes = m
	}

	return elossyEncodedLossyCandidate{
		baseQuant:      segment.quantizer[0],
		segment:        *segment,
		probabilities:  probabilities,
		modes:          modes,
		tokenPartition: tokenPartition,
	}, nil
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

// EncodeLossyRgbaToVp8WithOptions encodes RGBA pixels to a raw lossy VP8 frame
// payload with explicit options.
func EncodeLossyRgbaToVp8WithOptions(width, height int, rgba []byte, options *LossyEncodingOptions) ([]byte, error) {
	if err := elossyValidateRgba(width, height, rgba); err != nil {
		return nil, err
	}
	if err := elossyValidateOptions(options); err != nil {
		return nil, err
	}

	mbWidth := (width + 15) >> 4
	mbHeight := (height + 15) >> 4
	baseQuant := elossyBaseQuantizerFromQuality(options.Quality)
	profile := elossySearchProfile(options.OptimizationLevel)
	source := elossyRgbaToYuv420(width, height, rgba, mbWidth, mbHeight)
	candidates := elossyBuildSegmentCandidates(&source, mbWidth, mbHeight, baseQuant, options.OptimizationLevel)

	var bestLen int
	var bestVp8 []byte
	found := false
	for i := range candidates {
		candidate, err := elossyEncodeLossyCandidate(&source, mbWidth, mbHeight, &profile, &candidates[i])
		if err != nil {
			return nil, err
		}
		vp8, err := elossyFinalizeLossyCandidate(width, height, &source, mbWidth, mbHeight, baseQuant, options.OptimizationLevel, &candidate)
		if err != nil {
			return nil, err
		}
		replace := !found || len(vp8) < bestLen
		if replace {
			bestLen = len(vp8)
			bestVp8 = vp8
			found = true
		}
	}

	if !found {
		return nil, encBitstream("lossy candidate search produced no output")
	}
	return bestVp8, nil
}

// EncodeLossyRgbaToVp8 encodes RGBA pixels to a raw lossy VP8 frame payload.
func EncodeLossyRgbaToVp8(width, height int, rgba []byte) ([]byte, error) {
	options := elossyDefaultOptions()
	return EncodeLossyRgbaToVp8WithOptions(width, height, rgba, &options)
}

// EncodeLossyRgbaToWebpWithOptions encodes RGBA pixels to a still lossy WebP
// container with explicit options.
func EncodeLossyRgbaToWebpWithOptions(width, height int, rgba []byte, options *LossyEncodingOptions) ([]byte, error) {
	return EncodeLossyRgbaToWebpWithOptionsAndExif(width, height, rgba, options, nil)
}

// EncodeLossyRgbaToWebpWithOptionsAndExif encodes RGBA pixels to a still lossy
// WebP container with explicit options and EXIF.
func EncodeLossyRgbaToWebpWithOptionsAndExif(width, height int, rgba []byte, options *LossyEncodingOptions, exif []byte) ([]byte, error) {
	vp8, err := EncodeLossyRgbaToVp8WithOptions(width, height, rgba, options)
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
func EncodeLossyRgbaToWebp(width, height int, rgba []byte) ([]byte, error) {
	options := elossyDefaultOptions()
	return EncodeLossyRgbaToWebpWithOptions(width, height, rgba, &options)
}

// EncodeLossyImageToWebpWithOptions encodes an ImageBuffer to a still lossy WebP
// container with explicit options.
func EncodeLossyImageToWebpWithOptions(image *ImageBuffer, options *LossyEncodingOptions) ([]byte, error) {
	return EncodeLossyImageToWebpWithOptionsAndExif(image, options, nil)
}

// EncodeLossyImageToWebpWithOptionsAndExif encodes an ImageBuffer to a still lossy
// WebP container with explicit options and EXIF.
func EncodeLossyImageToWebpWithOptionsAndExif(image *ImageBuffer, options *LossyEncodingOptions, exif []byte) ([]byte, error) {
	return EncodeLossyRgbaToWebpWithOptionsAndExif(image.Width, image.Height, image.RGBA, options, exif)
}

// EncodeLossyImageToWebp encodes an ImageBuffer to a still lossy WebP container.
func EncodeLossyImageToWebp(image *ImageBuffer) ([]byte, error) {
	options := elossyDefaultOptions()
	return EncodeLossyImageToWebpWithOptions(image, &options)
}
