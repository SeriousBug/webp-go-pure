package webp

// ColorRange describes how a [YUVImage]'s samples use the 0..255 byte range.
type ColorRange uint8

const (
	// RangeLimited is BT.601 studio swing: luma spans 16..235 and chroma spans
	// 16..240. This is what lossy WebP stores, so it is the zero value and what
	// [DecodeYUV] returns.
	RangeLimited ColorRange = iota
	// RangeFull spans 0..255 in every plane. This is what image/jpeg produces
	// and what the standard library's image.YCbCr is defined to hold, so planes
	// taken from either are RangeFull.
	RangeFull
)

// Scaling between the two ranges. Luma occupies 219 of the 255 codes and chroma
// 224, both offset so that limited-range black sits at 16.
const (
	rangeLumaOffset = 16
	rangeLumaSpan   = 219
	rangeChromaSpan = 224
	rangeFullSpan   = 255
)

var (
	lumaFullToLimited   = buildRangeTable(rangeFullSpan, rangeLumaSpan, 0, rangeLumaOffset)
	lumaLimitedToFull   = buildRangeTable(rangeLumaSpan, rangeFullSpan, rangeLumaOffset, 0)
	chromaFullToLimited = buildRangeTable(rangeFullSpan, rangeChromaSpan, 128, 128)
	chromaLimitedToFull = buildRangeTable(rangeChromaSpan, rangeFullSpan, 128, 128)
)

// buildRangeTable maps value v to dstZero + (v-srcZero)*dstSpan/srcSpan,
// rounded and clamped. Chroma passes 128 for both zero points because it is
// signed around mid-grey; luma's zero moves between 0 and 16.
func buildRangeTable(srcSpan, dstSpan, srcZero, dstZero int32) *[256]uint8 {
	var table [256]uint8
	for i := range table {
		numerator := (int32(i) - srcZero) * dstSpan
		// Round half away from zero, since the input straddles srcZero.
		if numerator < 0 {
			numerator -= srcSpan / 2
		} else {
			numerator += srcSpan / 2
		}
		table[i] = uint8(clampI32(dstZero+numerator/srcSpan, 0, 255))
	}
	return &table
}

func clampI32(v, low, high int32) int32 {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}

// ConvertRange rewrites the Y, U and V planes in place so that they are
// expressed in the given range, and updates [YUVImage.Range] to match. It is a
// no-op when the image already carries that range.
//
// The conversion is lossy in both directions: limited range has fewer codes
// than full range, so a round trip does not always land back on the original
// sample. The alpha plane is left alone, as alpha is full range either way.
func (img *YUVImage) ConvertRange(to ColorRange) {
	if img.Range == to {
		return
	}
	luma, chroma := lumaFullToLimited, chromaFullToLimited
	if to == RangeFull {
		luma, chroma = lumaLimitedToFull, chromaLimitedToFull
	}

	uvWidth := (img.Width + 1) / 2
	uvHeight := (img.Height + 1) / 2
	mapPlane(img.Y, img.YStride, img.Width, img.Height, luma)
	mapPlane(img.U, img.UVStride, uvWidth, uvHeight, chroma)
	mapPlane(img.V, img.UVStride, uvWidth, uvHeight, chroma)
	img.Range = to
}

func mapPlane(plane []byte, stride, width, height int, table *[256]uint8) {
	for row := 0; row < height; row++ {
		start := row * stride
		if start+width > len(plane) {
			return
		}
		out := plane[start : start+width : start+width]
		for i, v := range out {
			out[i] = table[v]
		}
	}
}
