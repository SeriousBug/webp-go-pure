package webp

func elossyPlaneFits(plane []byte, stride, width, height int, name string) error {
	if stride < width {
		return encInvalidParam(name + " stride is smaller than the plane width")
	}
	// Only the last row has to be complete; whatever follows it is padding the
	// encoder never reads.
	need := (height-1)*stride + width
	if len(plane) < need {
		return encInvalidParam(name + " plane is shorter than its stride and height require")
	}
	return nil
}

func elossyValidateYuv(img *YUVImage) error {
	if img == nil {
		return encInvalidParam("image must not be nil")
	}
	if img.Width == 0 || img.Height == 0 {
		return encInvalidParam("image dimensions must be non-zero")
	}
	if img.Width > elossyMaxWebpDimension || img.Height > elossyMaxWebpDimension {
		return encInvalidParam("image dimensions exceed VP8 limits")
	}
	if img.A != nil {
		return encAlphaUnsupported("lossy encoder does not support alpha yet")
	}

	uvWidth := (img.Width + 1) / 2
	uvHeight := (img.Height + 1) / 2
	if err := elossyPlaneFits(img.Y, img.YStride, img.Width, img.Height, "Y"); err != nil {
		return err
	}
	if err := elossyPlaneFits(img.U, img.UVStride, uvWidth, uvHeight, "U"); err != nil {
		return err
	}
	return elossyPlaneFits(img.V, img.UVStride, uvWidth, uvHeight, "V")
}

// elossyCopyPaddedPlane copies a source plane into a macroblock-padded
// destination, replicating the last real column and row into the padding the
// same way elossyRgbaToYuv420 does. The encoder reads whole macroblocks, so the
// padding has to hold edge-clamped samples rather than zeroes, which would
// otherwise show up as a hard edge the transform has to spend bits on.
func elossyCopyPaddedPlane(dst []uint8, dstStride, dstHeight int, src []byte, srcStride, width, height int) {
	for row := 0; row < dstHeight; row++ {
		start := row * dstStride
		out := dst[start : start+dstStride : start+dstStride]
		if row >= height {
			last := (height - 1) * dstStride
			copy(out, dst[last:last+dstStride])
			continue
		}
		in := src[row*srcStride:]
		copy(out[:width], in[:width])
		if width < dstStride {
			pad := out[width-1]
			for col := width; col < dstStride; col++ {
				out[col] = pad
			}
		}
	}
}

// elossyYuvToPlanes repacks caller-supplied 4:2:0 planes into the encoder's
// macroblock-padded layout. Unlike elossyRgbaToYuv420 this is a per-row copy
// rather than per-pixel colorspace math: the samples are already what the
// encoder wants, only the strides and the edge padding differ.
func elossyYuvToPlanes(img *YUVImage, mbWidth, mbHeight int) elossyPlanes {
	yStride := mbWidth * 16
	uvStride := mbWidth * 8
	yHeight := mbHeight * 16
	uvHeight := mbHeight * 8

	y := make([]uint8, yStride*yHeight)
	u := make([]uint8, uvStride*uvHeight)
	v := make([]uint8, uvStride*uvHeight)

	uvWidth := (img.Width + 1) / 2
	uvRows := (img.Height + 1) / 2
	elossyCopyPaddedPlane(y, yStride, yHeight, img.Y, img.YStride, img.Width, img.Height)
	elossyCopyPaddedPlane(u, uvStride, uvHeight, img.U, img.UVStride, uvWidth, uvRows)
	elossyCopyPaddedPlane(v, uvStride, uvHeight, img.V, img.UVStride, uvWidth, uvRows)

	return elossyPlanes{
		yStride:  yStride,
		uvStride: uvStride,
		y:        y,
		u:        u,
		v:        v,
	}
}

// encodeLossyPlanesToVp8 is the lossy encoder proper: everything from a
// macroblock-padded source in the encoder's own layout to a raw VP8 frame
// payload. Both the RGBA and the planar entry points funnel through here, so
// they differ only in how the source planes were produced.
func encodeLossyPlanesToVp8(width, height int, source *elossyPlanes, mbWidth, mbHeight int, options *LossyOptions) ([]byte, error) {
	baseQuant := elossyBaseQuantizerFromQuality(options.Quality)
	profile := elossySearchProfile(options.Effort)
	candidates := elossyBuildSegmentCandidates(source, mbWidth, mbHeight, baseQuant, options.Effort)

	// Candidates are ranked on rate *and* distortion; only the winner pays for
	// the filter search.
	heuristic := elossyHeuristicFilter(baseQuant)
	scratch := elossyNewEncodeScratch(mbWidth, mbHeight, len(source.y)/4)
	shortlist := elossyShortlistCandidates(scratch, source, width, height, mbWidth, mbHeight, &profile, candidates, baseQuant)

	var best elossyEncodedLossyCandidate
	var bestCost uint64
	found := false
	for _, index := range shortlist {
		candidate, err := elossyEncodeLossyCandidate(scratch, width, height, source, mbWidth, mbHeight, &profile, &candidates[index], nil)
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
			scratch.release(best.buffers)
			best = candidate
			found = true
		} else {
			scratch.release(candidate.buffers)
		}
	}

	if !found {
		return nil, encBitstream("lossy candidate search produced no output")
	}
	return elossyFinalizeLossyCandidate(scratch, width, height, source, mbWidth, mbHeight, baseQuant, options.Effort, &best)
}
