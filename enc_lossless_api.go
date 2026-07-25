package webp

// Public entry points for lossless still-image WebP encoding.
// Ported from src/encoder/lossless/api.rs.

// EncodeLosslessRgbaToVp8lWithOptions encodes RGBA pixels to a raw lossless VP8L frame payload with explicit options.
func encodeLosslessRgbaToVp8lWithOptions(width, height int, rgba []byte, options *LosslessOptions) ([]byte, error) {
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

	for _, profile := range elosslessCandidateProfiles(options.Effort) {
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
		bestEstimate := elosslessIntMax
		for _, ranked := range rankedPlans {
			plan := ranked.plan
			if bestEstimate != elosslessIntMax && elosslessShouldStopTransformSearch(bestEstimate, ranked.score, &profile) {
				break
			}

			encoded, err := elosslessEncodeTransformPlanToVp8l(width, height, rgba, &plan, &profile)
			if err != nil {
				return nil, err
			}
			if ranked.score < bestEstimate {
				bestEstimate = ranked.score
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
func encodeLosslessRgbaToVp8l(width, height int, rgba []byte) ([]byte, error) {
	options := elosslessDefaultOptions()
	return encodeLosslessRgbaToVp8lWithOptions(width, height, rgba, &options)
}

// EncodeLosslessRgbaToWebpWithOptions encodes RGBA pixels to a still lossless WebP container with explicit options.
func encodeLosslessRgbaToWebpWithOptions(width, height int, rgba []byte, options *LosslessOptions) ([]byte, error) {
	return encodeLosslessRgbaToWebpWithOptionsAndExif(width, height, rgba, options, nil)
}

// EncodeLosslessRgbaToWebpWithOptionsAndExif encodes RGBA pixels to a still lossless WebP container with explicit options and EXIF.
func encodeLosslessRgbaToWebpWithOptionsAndExif(width, height int, rgba []byte, options *LosslessOptions, exif []byte) ([]byte, error) {
	vp8l, err := encodeLosslessRgbaToVp8lWithOptions(width, height, rgba, options)
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
func encodeLosslessRgbaToWebp(width, height int, rgba []byte) ([]byte, error) {
	options := elosslessDefaultOptions()
	return encodeLosslessRgbaToWebpWithOptions(width, height, rgba, &options)
}

// EncodeLosslessImageToWebpWithOptions encodes an ImageBuffer to a still lossless WebP container with explicit options.
func encodeLosslessImageToWebpWithOptions(image *Image, options *LosslessOptions) ([]byte, error) {
	return encodeLosslessImageToWebpWithOptionsAndExif(image, options, nil)
}

// EncodeLosslessImageToWebpWithOptionsAndExif encodes an ImageBuffer to a still lossless WebP container with explicit options and EXIF.
func encodeLosslessImageToWebpWithOptionsAndExif(image *Image, options *LosslessOptions, exif []byte) ([]byte, error) {
	return encodeLosslessRgbaToWebpWithOptionsAndExif(image.Width, image.Height, image.RGBA, options, exif)
}

// EncodeLosslessImageToWebp encodes an ImageBuffer to a still lossless WebP container.
func encodeLosslessImageToWebp(image *Image) ([]byte, error) {
	options := elosslessDefaultOptions()
	return encodeLosslessImageToWebpWithOptions(image, &options)
}
