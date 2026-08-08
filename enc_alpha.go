package webp

import "math"

// The ALPH chunk carries a lossy image's alpha plane. This file is the exact
// inverse of the decode side in alpha.go: extract the plane, apply one of the
// four spatial filters to it, then store it either raw or as a headerless VP8L
// image stream whose green channel holds the filtered alpha bytes.

// elossyAlphaVp8lHeaderBytes is the size of the frame header our lossless
// encoder writes ahead of the image stream: a 0x2f signature byte followed by
// 14+14+1+3 bits of width, height, alpha flag and version. An ALPH payload
// carries the image stream alone, and the header is a whole number of bytes, so
// stripping this prefix from a VP8L frame yields exactly that stream.
const elossyAlphaVp8lHeaderBytes = 5

func elossyAlphaPlane(width, height int, rgba []byte) []byte {
	alpha := make([]byte, width*height)
	for i := range alpha {
		alpha[i] = rgba[i*4+3]
	}
	return alpha
}

func elossyAlphaPlaneFromStrided(width, height int, plane []byte, stride int) []byte {
	alpha := make([]byte, width*height)
	for y := 0; y < height; y++ {
		copy(alpha[y*width:(y+1)*width], plane[y*stride:y*stride+width])
	}
	return alpha
}

func elossyAlphaIsOpaque(alpha []byte) bool {
	for _, v := range alpha {
		if v != 0xff {
			return false
		}
	}
	return true
}

// elossyFilterAlphaRow is the forward direction of lossyUnfilterRow.
func elossyFilterAlphaRow(filter uint8, prev, row, out []byte) {
	switch filter {
	case lossyAlphaFilterNone:
		copy(out, row)
	case lossyAlphaFilterHorizontal:
		var pred byte
		if prev != nil {
			pred = prev[0]
		}
		for i := range out {
			out[i] = row[i] - pred
			pred = row[i]
		}
	case lossyAlphaFilterVertical:
		if prev == nil {
			elossyFilterAlphaRow(lossyAlphaFilterHorizontal, nil, row, out)
			return
		}
		for i := range out {
			out[i] = row[i] - prev[i]
		}
	case lossyAlphaFilterGradient:
		if prev == nil {
			elossyFilterAlphaRow(lossyAlphaFilterHorizontal, nil, row, out)
			return
		}
		topLeft := prev[0]
		left := prev[0]
		for x := range out {
			top := prev[x]
			out[x] = row[x] - lossyGradientPredictor(left, top, topLeft)
			topLeft = top
			left = row[x]
		}
	}
}

func elossyFilterAlphaPlane(alpha []byte, filter uint8, width, height int) []byte {
	filtered := make([]byte, len(alpha))
	for y := 0; y < height; y++ {
		start := y * width
		end := start + width
		var prev []byte
		if y != 0 {
			prev = alpha[start-width : start]
		}
		elossyFilterAlphaRow(filter, prev, alpha[start:end], filtered[start:end])
	}
	return filtered
}

// elossyAlphaFilterCost estimates what a filtered plane costs to store, as the
// zero-order entropy of its bytes in bits. Ranking the four filters this way
// costs one pass each instead of four trial compressions, which is how libwebp
// picks a filter too.
func elossyAlphaFilterCost(filtered []byte) float64 {
	var histogram [256]int
	for _, v := range filtered {
		histogram[v]++
	}
	total := float64(len(filtered))
	cost := 0.0
	for _, count := range histogram {
		if count == 0 {
			continue
		}
		p := float64(count) / total
		cost -= float64(count) * math.Log2(p)
	}
	return cost
}

// elossyPickAlphaFilter returns the lowest-entropy of the four filters along
// with the plane it produces.
func elossyPickAlphaFilter(alpha []byte, width, height int) (uint8, []byte) {
	bestFilter := uint8(lossyAlphaFilterNone)
	var bestPlane []byte
	bestCost := math.Inf(1)
	for filter := uint8(lossyAlphaFilterNone); filter <= lossyAlphaFilterGradient; filter++ {
		filtered := elossyFilterAlphaPlane(alpha, filter, width, height)
		cost := elossyAlphaFilterCost(filtered)
		if cost < bestCost {
			bestCost = cost
			bestFilter = filter
			bestPlane = filtered
		}
	}
	return bestFilter, bestPlane
}

func elossyAlphaHeaderByte(compression, filter uint8) byte {
	return compression | (filter << 2)
}

// elossyAlphaToVp8lStream compresses a filtered alpha plane as a headerless
// VP8L image stream carrying the alpha bytes in the green channel.
func elossyAlphaToVp8lStream(width, height int, filtered []byte, effort uint8) ([]byte, error) {
	rgba := make([]byte, len(filtered)*4)
	for i, v := range filtered {
		rgba[i*4+1] = v
		rgba[i*4+3] = 0xff
	}
	options := LosslessOptions{Effort: effort}
	frame, err := encodeLosslessRgbaToVp8lWithOptions(width, height, rgba, &options)
	if err != nil {
		return nil, err
	}
	if len(frame) < elossyAlphaVp8lHeaderBytes {
		return nil, encBitstream("VP8L alpha stream is too short")
	}
	return frame[elossyAlphaVp8lHeaderBytes:], nil
}

// elossyAlphaLosslessEffort maps a lossy effort level onto the lossless
// encoder's narrower 0..=6 range, so a cheap lossy encode does not pay for an
// expensive alpha search.
func elossyAlphaLosslessEffort(effort uint8) uint8 {
	if effort > elosslessMaxOptimizationLevel {
		return elosslessMaxOptimizationLevel
	}
	return effort
}

// elossyAlphaExhaustiveFilterEffort is the effort at which every filter gets a
// trial encode rather than only the two that are usually in contention.
const elossyAlphaExhaustiveFilterEffort = 4

// elossyAlphaFilterCandidates lists the filters worth a trial encode.
//
// Ranking filters by the entropy of the plane they produce, as the raw form
// would be stored, is a poor proxy for what they cost once VP8L has run: VP8L
// applies its own spatial prediction, so pre-filtering often just destroys the
// structure that prediction would have exploited. Filtering none and the
// lowest-entropy filter are the two that end up in contention, and trying both
// costs two encodes instead of four.
func elossyAlphaFilterCandidates(alpha []byte, width, height int, effort uint8) []uint8 {
	if effort >= elossyAlphaExhaustiveFilterEffort {
		return []uint8{lossyAlphaFilterNone, lossyAlphaFilterHorizontal, lossyAlphaFilterVertical, lossyAlphaFilterGradient}
	}
	filter, _ := elossyPickAlphaFilter(alpha, width, height)
	if filter == lossyAlphaFilterNone {
		return []uint8{lossyAlphaFilterNone}
	}
	return []uint8{lossyAlphaFilterNone, filter}
}

// elossyEncodeAlphaChunk builds an ALPH chunk payload for an alpha plane,
// choosing whichever of the raw and VP8L-compressed forms is smaller.
func elossyEncodeAlphaChunk(width, height int, alpha []byte, effort uint8) ([]byte, error) {
	if len(alpha) != width*height {
		return nil, encInvalidParam("alpha plane length does not match dimensions")
	}

	losslessEffort := elossyAlphaLosslessEffort(effort)
	var best []byte
	for _, filter := range elossyAlphaFilterCandidates(alpha, width, height, effort) {
		filtered := elossyFilterAlphaPlane(alpha, filter, width, height)
		stream, err := elossyAlphaToVp8lStream(width, height, filtered, losslessEffort)
		if err != nil {
			return nil, err
		}
		if best != nil && lossyAlphaHeaderLen+len(stream) >= len(best) {
			continue
		}
		payload := make([]byte, 0, lossyAlphaHeaderLen+len(stream))
		payload = append(payload, elossyAlphaHeaderByte(lossyAlphaLosslessCompression, filter))
		best = append(payload, stream...)
	}

	// The raw form stores the plane byte for byte, so its size does not depend
	// on the filter and any filter will do.
	if best == nil || len(best) >= lossyAlphaHeaderLen+len(alpha) {
		filter, filtered := elossyPickAlphaFilter(alpha, width, height)
		raw := make([]byte, 0, lossyAlphaHeaderLen+len(filtered))
		raw = append(raw, elossyAlphaHeaderByte(lossyAlphaNoCompression, filter))
		return append(raw, filtered...), nil
	}
	return best, nil
}

// elossyBuildAlphaChunk returns the ALPH payload for a plane, or nil when the
// plane is fully opaque and the chunk would carry no information.
func elossyBuildAlphaChunk(width, height int, alpha []byte, effort uint8) ([]byte, error) {
	if alpha == nil || elossyAlphaIsOpaque(alpha) {
		return nil, nil
	}
	return elossyEncodeAlphaChunk(width, height, alpha, effort)
}
