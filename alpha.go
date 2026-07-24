package webp

const (
	lossyAlphaHeaderLen           = 1
	lossyAlphaNoCompression       = 0
	lossyAlphaLosslessCompression = 1
	lossyAlphaPreprocessedLevels  = 2
	lossyAlphaFilterNone          = 0
	lossyAlphaFilterHorizontal    = 1
	lossyAlphaFilterVertical      = 2
	lossyAlphaFilterGradient      = 3
)

// AlphaHeader is a parsed one-byte ALPH header.
type AlphaHeader struct {
	Compression   uint8
	Filter        uint8
	Preprocessing uint8
}

// parseAlphaHeader parses the one-byte header that prefixes an ALPH payload.
func parseAlphaHeader(data []byte) (AlphaHeader, error) {
	if len(data) == 0 {
		return AlphaHeader{}, notEnoughData("ALPH header")
	}
	header := data[0]

	reserved := header >> 6
	if reserved != 0 {
		return AlphaHeader{}, bitstreamErr("ALPH reserved bits must be zero")
	}

	alpha := AlphaHeader{
		Compression:   header & 0x03,
		Filter:        (header >> 2) & 0x03,
		Preprocessing: (header >> 4) & 0x03,
	}

	if alpha.Compression > lossyAlphaLosslessCompression {
		return AlphaHeader{}, bitstreamErr("unsupported ALPH compression method")
	}
	if alpha.Preprocessing > lossyAlphaPreprocessedLevels {
		return AlphaHeader{}, bitstreamErr("unsupported ALPH preprocessing mode")
	}

	return alpha, nil
}

func lossyGradientPredictor(left, top, topLeft byte) byte {
	v := int32(left) + int32(top) - int32(topLeft)
	if v < 0 {
		v = 0
	} else if v > 255 {
		v = 255
	}
	return byte(v)
}

func lossyUnfilterRow(filter uint8, prev, deltas, out []byte) error {
	switch filter {
	case lossyAlphaFilterNone:
		copy(out, deltas)
	case lossyAlphaFilterHorizontal:
		var pred byte
		if prev != nil {
			pred = prev[0]
		}
		for i := range out {
			out[i] = pred + deltas[i]
			pred = out[i]
		}
	case lossyAlphaFilterVertical:
		if prev != nil {
			for i := range out {
				out[i] = prev[i] + deltas[i]
			}
		} else {
			return lossyUnfilterRow(lossyAlphaFilterHorizontal, nil, deltas, out)
		}
	case lossyAlphaFilterGradient:
		if prev != nil {
			topLeft := prev[0]
			left := prev[0]
			for x := range out {
				top := prev[x]
				left = deltas[x] + lossyGradientPredictor(left, top, topLeft)
				topLeft = top
				out[x] = left
			}
		} else {
			return lossyUnfilterRow(lossyAlphaFilterHorizontal, nil, deltas, out)
		}
	default:
		return bitstreamErr("invalid ALPH filter")
	}
	return nil
}

func lossyUnfilterAlpha(alpha []byte, filter uint8, width, height int) ([]byte, error) {
	expectedLen := width * height
	if width != 0 && expectedLen/width != height {
		return nil, bitstreamErr("alpha plane size overflow")
	}
	if len(alpha) < expectedLen {
		return nil, notEnoughData("alpha plane payload")
	}

	decoded := make([]byte, expectedLen)
	for y := 0; y < height; y++ {
		rowStart := y * width
		rowEnd := rowStart + width
		var prev []byte
		if y != 0 {
			prev = decoded[rowStart-width : rowStart]
		}
		if err := lossyUnfilterRow(filter, prev, alpha[rowStart:rowEnd], decoded[rowStart:rowEnd]); err != nil {
			return nil, err
		}
	}
	return decoded, nil
}

// decodeAlphaPlane decodes an ALPH payload to a single-channel alpha plane, one
// alpha byte per pixel in row-major order.
func decodeAlphaPlane(data []byte, width, height int) ([]byte, error) {
	header, err := parseAlphaHeader(data)
	if err != nil {
		return nil, err
	}
	if len(data) < lossyAlphaHeaderLen {
		return nil, notEnoughData("ALPH payload")
	}
	payload := data[lossyAlphaHeaderLen:]
	pixelCount := width * height
	if width != 0 && pixelCount/width != height {
		return nil, bitstreamErr("alpha plane size overflow")
	}

	switch header.Compression {
	case lossyAlphaNoCompression:
		if len(payload) < pixelCount {
			return nil, notEnoughData("ALPH raw payload")
		}
		return lossyUnfilterAlpha(payload[:pixelCount], header.Filter, width, height)
	case lossyAlphaLosslessCompression:
		argb, err := decodeLosslessStreamToArgb(payload, width, height)
		if err != nil {
			return nil, err
		}
		filtered := make([]byte, pixelCount)
		for i := 0; i < pixelCount && i < len(argb); i++ {
			filtered[i] = byte((argb[i] >> 8) & 0xff)
		}
		return lossyUnfilterAlpha(filtered, header.Filter, width, height)
	default:
		return nil, bitstreamErr("unsupported ALPH compression method")
	}
}

// applyAlphaPlane replaces the alpha channel of an RGBA image with a decoded
// alpha plane.
func applyAlphaPlane(rgba, alpha []byte) error {
	expectedLen := len(alpha) * 4
	if len(rgba) != expectedLen {
		return invalidParam("RGBA buffer length does not match alpha plane")
	}
	for i, value := range alpha {
		rgba[i*4+3] = value
	}
	return nil
}
