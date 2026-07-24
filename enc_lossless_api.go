package webp

// Public entry points for lossless still-image WebP encoding.
// Ported from src/encoder/lossless/api.rs.

// EncodeLosslessRgbaToVp8lWithOptions encodes RGBA pixels to a raw lossless VP8L frame payload with explicit options.
func EncodeLosslessRgbaToVp8lWithOptions(width, height int, rgba []byte, options *LosslessEncodingOptions) ([]byte, error) {
	if err := elosslessValidateRgba(width, height, rgba); err != nil {
		return nil, err
	}
	if err := elosslessValidateOptions(options); err != nil {
		return nil, err
	}

	argb := elosslessRgbaToArgb(rgba)
	subtractGreen := elosslessApplySubtractGreenTransform(argb)
	var best []byte
	haveBest := false

	for _, profile := range elosslessCandidateProfiles(options.OptimizationLevel) {
		profile := profile
		var profileBest []byte
		haveProfileBest := false

		candidate, err := elosslessBuildPaletteCandidate(width, height, argb)
		if err != nil {
			return nil, err
		}
		if candidate != nil {
			encoded, err := elosslessEncodePaletteCandidateToVp8l(width, height, rgba, candidate, &profile)
			if err != nil {
				return nil, err
			}
			profileBest = encoded
			haveProfileBest = true
		}

		plans := elosslessCollectTransformPlans(width, height, argb, subtractGreen, &profile)
		rankedPlans, err := elosslessShortlistTransformPlans(width, plans, &profile)
		if err != nil {
			return nil, err
		}
		for _, ranked := range rankedPlans {
			estimate := ranked.score
			plan := ranked.plan
			if haveProfileBest && elosslessShouldStopTransformSearch(len(profileBest), estimate, &profile) {
				break
			}

			encoded, err := elosslessEncodeTransformPlanToVp8l(width, height, rgba, &plan, &profile)
			if err != nil {
				return nil, err
			}
			if !haveProfileBest || len(encoded) < len(profileBest) {
				profileBest = encoded
				haveProfileBest = true
			}
		}

		if haveProfileBest {
			if !haveBest || len(profileBest) < len(best) {
				best = profileBest
				haveBest = true
			}
		}
	}

	if !haveBest {
		return nil, encBitstream("lossless encoder produced no candidate")
	}
	return best, nil
}

// EncodeLosslessRgbaToVp8l encodes RGBA pixels to a raw lossless VP8L frame payload.
func EncodeLosslessRgbaToVp8l(width, height int, rgba []byte) ([]byte, error) {
	options := elosslessDefaultOptions()
	return EncodeLosslessRgbaToVp8lWithOptions(width, height, rgba, &options)
}

// EncodeLosslessRgbaToWebpWithOptions encodes RGBA pixels to a still lossless WebP container with explicit options.
func EncodeLosslessRgbaToWebpWithOptions(width, height int, rgba []byte, options *LosslessEncodingOptions) ([]byte, error) {
	return EncodeLosslessRgbaToWebpWithOptionsAndExif(width, height, rgba, options, nil)
}

// EncodeLosslessRgbaToWebpWithOptionsAndExif encodes RGBA pixels to a still lossless WebP container with explicit options and EXIF.
func EncodeLosslessRgbaToWebpWithOptionsAndExif(width, height int, rgba []byte, options *LosslessEncodingOptions, exif []byte) ([]byte, error) {
	vp8l, err := EncodeLosslessRgbaToVp8lWithOptions(width, height, rgba, options)
	if err != nil {
		return nil, err
	}
	return wrapStillWebp(stillImageChunk{
		fourcc:   [4]byte{'V', 'P', '8', 'L'},
		payload:  vp8l,
		width:    width,
		height:   height,
		hasAlpha: elosslessRgbaHasAlpha(rgba),
	}, exif)
}

// EncodeLosslessRgbaToWebp encodes RGBA pixels to a still lossless WebP container.
func EncodeLosslessRgbaToWebp(width, height int, rgba []byte) ([]byte, error) {
	options := elosslessDefaultOptions()
	return EncodeLosslessRgbaToWebpWithOptions(width, height, rgba, &options)
}

// EncodeLosslessImageToWebpWithOptions encodes an ImageBuffer to a still lossless WebP container with explicit options.
func EncodeLosslessImageToWebpWithOptions(image *ImageBuffer, options *LosslessEncodingOptions) ([]byte, error) {
	return EncodeLosslessImageToWebpWithOptionsAndExif(image, options, nil)
}

// EncodeLosslessImageToWebpWithOptionsAndExif encodes an ImageBuffer to a still lossless WebP container with explicit options and EXIF.
func EncodeLosslessImageToWebpWithOptionsAndExif(image *ImageBuffer, options *LosslessEncodingOptions, exif []byte) ([]byte, error) {
	return EncodeLosslessRgbaToWebpWithOptionsAndExif(image.Width, image.Height, image.RGBA, options, exif)
}

// EncodeLosslessImageToWebp encodes an ImageBuffer to a still lossless WebP container.
func EncodeLosslessImageToWebp(image *ImageBuffer) ([]byte, error) {
	options := elosslessDefaultOptions()
	return EncodeLosslessImageToWebpWithOptions(image, &options)
}
