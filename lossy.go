package webp

const (
	lossyVp8TransformAC3C1 int32 = 20091
	lossyVp8TransformAC3C2 int32 = 35468

	lossyRGBYCoeff int32 = 19077
	lossyRGBVToR   int32 = 26149
	lossyRGBUToG   int32 = 6419
	lossyRGBVToG   int32 = 13320
	lossyRGBUToB   int32 = 33050
	lossyRGBRBias  int32 = 14234
	lossyRGBGBias  int32 = 8708
	lossyRGBBBias  int32 = 17685
	lossyYUVFix2   int32 = 6
	lossyYUVMask2  int32 = (256 << 6) - 1
)

type lossyPlanes struct {
	width    int
	height   int
	yStride  int
	uvStride int
	y        []byte
	u        []byte
	v        []byte
}

func lossyNewPlanes(frame *macroBlockDataFrame) lossyPlanes {
	yStride := frame.Frame.MacroblockWidth * 16
	uvStride := frame.Frame.MacroblockWidth * 8
	height := frame.Frame.MacroblockHeight * 16
	uvHeight := frame.Frame.MacroblockHeight * 8
	return lossyPlanes{
		width:    int(frame.Frame.Picture.Width),
		height:   int(frame.Frame.Picture.Height),
		yStride:  yStride,
		uvStride: uvStride,
		y:        make([]byte, yStride*height),
		u:        make([]byte, uvStride*uvHeight),
		v:        make([]byte, uvStride*uvHeight),
	}
}

type lossyFilterInfo struct {
	fLimit    uint8
	fILevel   uint8
	fInner    bool
	hevThresh uint8
}

func lossyAbsDiff(a, b byte) int32 {
	d := int32(a) - int32(b)
	if d < 0 {
		return -d
	}
	return d
}

func lossyClipSigned(value int32) int32 {
	if value < -128 {
		return -128
	}
	if value > 127 {
		return 127
	}
	return value
}

func lossyClipFilterValue(value int32) int32 {
	if value < -16 {
		return -16
	}
	if value > 15 {
		return 15
	}
	return value
}

func lossyClipByte(value int32) byte {
	if value < 0 {
		return 0
	}
	if value > 255 {
		return 255
	}
	return byte(value)
}

func lossyDoFilter2(plane []byte, pos, step int) {
	p1 := int32(plane[pos-2*step])
	p0 := int32(plane[pos-step])
	q0 := int32(plane[pos])
	q1 := int32(plane[pos+step])
	a := 3*(q0-p0) + lossyClipSigned(p1-q1)
	a1 := lossyClipFilterValue((a + 4) >> 3)
	a2 := lossyClipFilterValue((a + 3) >> 3)
	plane[pos-step] = lossyClipByte(p0 + a2)
	plane[pos] = lossyClipByte(q0 - a1)
}

func lossyDoFilter4(plane []byte, pos, step int) {
	p1 := int32(plane[pos-2*step])
	p0 := int32(plane[pos-step])
	q0 := int32(plane[pos])
	q1 := int32(plane[pos+step])
	a := 3 * (q0 - p0)
	a1 := lossyClipFilterValue((a + 4) >> 3)
	a2 := lossyClipFilterValue((a + 3) >> 3)
	a3 := (a1 + 1) >> 1
	plane[pos-2*step] = lossyClipByte(p1 + a3)
	plane[pos-step] = lossyClipByte(p0 + a2)
	plane[pos] = lossyClipByte(q0 - a1)
	plane[pos+step] = lossyClipByte(q1 - a3)
}

func lossyDoFilter6(plane []byte, pos, step int) {
	p2 := int32(plane[pos-3*step])
	p1 := int32(plane[pos-2*step])
	p0 := int32(plane[pos-step])
	q0 := int32(plane[pos])
	q1 := int32(plane[pos+step])
	q2 := int32(plane[pos+2*step])
	a := lossyClipSigned(3*(q0-p0) + lossyClipSigned(p1-q1))
	a1 := (27*a + 63) >> 7
	a2 := (18*a + 63) >> 7
	a3 := (9*a + 63) >> 7
	plane[pos-3*step] = lossyClipByte(p2 + a3)
	plane[pos-2*step] = lossyClipByte(p1 + a2)
	plane[pos-step] = lossyClipByte(p0 + a1)
	plane[pos] = lossyClipByte(q0 - a1)
	plane[pos+step] = lossyClipByte(q1 - a2)
	plane[pos+2*step] = lossyClipByte(q2 - a3)
}

func lossyHev(plane []byte, pos, step int, thresh int32) bool {
	p1 := plane[pos-2*step]
	p0 := plane[pos-step]
	q0 := plane[pos]
	q1 := plane[pos+step]
	return lossyAbsDiff(p1, p0) > thresh || lossyAbsDiff(q1, q0) > thresh
}

func lossyNeedsFilter(plane []byte, pos, step int, thresh int32) bool {
	p1 := plane[pos-2*step]
	p0 := plane[pos-step]
	q0 := plane[pos]
	q1 := plane[pos+step]
	return 4*lossyAbsDiff(p0, q0)+lossyAbsDiff(p1, q1) <= thresh
}

func lossyNeedsFilter2(plane []byte, pos, step int, thresh, innerThresh int32) bool {
	p3 := plane[pos-4*step]
	p2 := plane[pos-3*step]
	p1 := plane[pos-2*step]
	p0 := plane[pos-step]
	q0 := plane[pos]
	q1 := plane[pos+step]
	q2 := plane[pos+2*step]
	q3 := plane[pos+3*step]
	if 4*lossyAbsDiff(p0, q0)+lossyAbsDiff(p1, q1) > thresh {
		return false
	}
	return lossyAbsDiff(p3, p2) <= innerThresh &&
		lossyAbsDiff(p2, p1) <= innerThresh &&
		lossyAbsDiff(p1, p0) <= innerThresh &&
		lossyAbsDiff(q3, q2) <= innerThresh &&
		lossyAbsDiff(q2, q1) <= innerThresh &&
		lossyAbsDiff(q1, q0) <= innerThresh
}

func lossySimpleVFilter16(plane []byte, pos, stride int, thresh int32) {
	thresh2 := 2*thresh + 1
	for i := 0; i < 16; i++ {
		edge := pos + i
		if lossyNeedsFilter(plane, edge, stride, thresh2) {
			lossyDoFilter2(plane, edge, stride)
		}
	}
}

func lossySimpleHFilter16(plane []byte, pos, stride int, thresh int32) {
	thresh2 := 2*thresh + 1
	for i := 0; i < 16; i++ {
		edge := pos + i*stride
		if lossyNeedsFilter(plane, edge, 1, thresh2) {
			lossyDoFilter2(plane, edge, 1)
		}
	}
}

func lossySimpleVFilter16i(plane []byte, pos, stride int, thresh int32) {
	for k := 0; k < 3; k++ {
		pos += 4 * stride
		lossySimpleVFilter16(plane, pos, stride, thresh)
	}
}

func lossySimpleHFilter16i(plane []byte, pos, stride int, thresh int32) {
	for k := 0; k < 3; k++ {
		pos += 4
		lossySimpleHFilter16(plane, pos, stride, thresh)
	}
}

func lossyFilterLoop26(plane []byte, pos, hstride, vstride, size int, thresh, innerThresh, hevThresh int32) {
	thresh2 := 2*thresh + 1
	for i := 0; i < size; i++ {
		if lossyNeedsFilter2(plane, pos, hstride, thresh2, innerThresh) {
			if lossyHev(plane, pos, hstride, hevThresh) {
				lossyDoFilter2(plane, pos, hstride)
			} else {
				lossyDoFilter6(plane, pos, hstride)
			}
		}
		pos += vstride
	}
}

func lossyFilterLoop24(plane []byte, pos, hstride, vstride, size int, thresh, innerThresh, hevThresh int32) {
	thresh2 := 2*thresh + 1
	for i := 0; i < size; i++ {
		if lossyNeedsFilter2(plane, pos, hstride, thresh2, innerThresh) {
			if lossyHev(plane, pos, hstride, hevThresh) {
				lossyDoFilter2(plane, pos, hstride)
			} else {
				lossyDoFilter4(plane, pos, hstride)
			}
		}
		pos += vstride
	}
}

func lossyVFilter16(plane []byte, pos, stride int, thresh, innerThresh, hevThresh int32) {
	lossyFilterLoop26(plane, pos, stride, 1, 16, thresh, innerThresh, hevThresh)
}

func lossyHFilter16(plane []byte, pos, stride int, thresh, innerThresh, hevThresh int32) {
	lossyFilterLoop26(plane, pos, 1, stride, 16, thresh, innerThresh, hevThresh)
}

func lossyVFilter16i(plane []byte, pos, stride int, thresh, innerThresh, hevThresh int32) {
	for k := 0; k < 3; k++ {
		pos += 4 * stride
		lossyFilterLoop24(plane, pos, stride, 1, 16, thresh, innerThresh, hevThresh)
	}
}

func lossyHFilter16i(plane []byte, pos, stride int, thresh, innerThresh, hevThresh int32) {
	for k := 0; k < 3; k++ {
		pos += 4
		lossyFilterLoop24(plane, pos, 1, stride, 16, thresh, innerThresh, hevThresh)
	}
}

func lossyVFilter8(planeU, planeV []byte, pos, stride int, thresh, innerThresh, hevThresh int32) {
	lossyFilterLoop26(planeU, pos, stride, 1, 8, thresh, innerThresh, hevThresh)
	lossyFilterLoop26(planeV, pos, stride, 1, 8, thresh, innerThresh, hevThresh)
}

func lossyHFilter8(planeU, planeV []byte, pos, stride int, thresh, innerThresh, hevThresh int32) {
	lossyFilterLoop26(planeU, pos, 1, stride, 8, thresh, innerThresh, hevThresh)
	lossyFilterLoop26(planeV, pos, 1, stride, 8, thresh, innerThresh, hevThresh)
}

func lossyVFilter8i(planeU, planeV []byte, pos, stride int, thresh, innerThresh, hevThresh int32) {
	lossyFilterLoop24(planeU, pos+4*stride, stride, 1, 8, thresh, innerThresh, hevThresh)
	lossyFilterLoop24(planeV, pos+4*stride, stride, 1, 8, thresh, innerThresh, hevThresh)
}

func lossyHFilter8i(planeU, planeV []byte, pos, stride int, thresh, innerThresh, hevThresh int32) {
	lossyFilterLoop24(planeU, pos+4, 1, stride, 8, thresh, innerThresh, hevThresh)
	lossyFilterLoop24(planeV, pos+4, 1, stride, 8, thresh, innerThresh, hevThresh)
}

func lossyMacroblockFilterInfo(frame *macroBlockDataFrame, macroblock *macroBlockData) (lossyFilterInfo, bool) {
	filter := &frame.Frame.Filter
	if filter.filterType == filterOff {
		return lossyFilterInfo{}, false
	}

	segment := &frame.Frame.Segment
	var baseLevel int32
	if segment.UseSegment {
		level := int32(segment.FilterStrength[macroblock.Header.Segment])
		if segment.AbsoluteDelta {
			baseLevel = level
		} else {
			baseLevel = level + int32(filter.Level)
		}
	} else {
		baseLevel = int32(filter.Level)
	}

	if filter.UseLFDelta {
		baseLevel += int32(filter.RefLFDelta[0])
		if macroblock.Header.IsI4x4 {
			baseLevel += int32(filter.ModeLFDelta[0])
		}
	}

	inner := macroblock.Header.IsI4x4 || (macroblock.NonZeroY|macroblock.NonZeroUV) != 0
	return lossyFilterInfoForLevel(baseLevel, filter.Sharpness, inner)
}

// lossyFilterInfoForLevel derives the strengths a macroblock is filtered with
// from its base level. The encoder scores filter levels through this too, so
// the two agree by construction rather than by a second implementation.
func lossyFilterInfoForLevel(baseLevel int32, sharpness uint8, inner bool) (lossyFilterInfo, bool) {
	level := baseLevel
	if level < 0 {
		level = 0
	} else if level > 63 {
		level = 63
	}
	if level == 0 {
		return lossyFilterInfo{}, false
	}

	ilevel := level
	if sharpness > 0 {
		if sharpness > 4 {
			ilevel >>= 2
		} else {
			ilevel >>= 1
		}
		m := 9 - int32(sharpness)
		if ilevel > m {
			ilevel = m
		}
	}
	if ilevel < 1 {
		ilevel = 1
	}

	var hevThresh uint8
	if level >= 40 {
		hevThresh = 2
	} else if level >= 15 {
		hevThresh = 1
	} else {
		hevThresh = 0
	}

	return lossyFilterInfo{
		fLimit:    uint8(2*level + ilevel),
		fILevel:   uint8(ilevel),
		fInner:    inner,
		hevThresh: hevThresh,
	}, true
}

func lossyFilterMacroblock(frame *macroBlockDataFrame, planes *lossyPlanes, mbX, mbY int, macroblock *macroBlockData) {
	info, ok := lossyMacroblockFilterInfo(frame, macroblock)
	if !ok {
		return
	}
	lossyFilterMacroblockWith(frame.Frame.Filter.filterType, planes, mbX, mbY, &info)
}

func lossyFilterMacroblockWith(kind filterType, planes *lossyPlanes, mbX, mbY int, info *lossyFilterInfo) {
	yPos := mbY*16*planes.yStride + mbX*16
	uvPos := mbY*8*planes.uvStride + mbX*8
	limit := int32(info.fLimit)
	inner := int32(info.fILevel)
	hev := int32(info.hevThresh)

	switch kind {
	case filterOff:
	case filterSimple:
		if mbX > 0 {
			lossySimpleHFilter16(planes.y, yPos, planes.yStride, limit+4)
		}
		if info.fInner {
			lossySimpleHFilter16i(planes.y, yPos, planes.yStride, limit)
		}
		if mbY > 0 {
			lossySimpleVFilter16(planes.y, yPos, planes.yStride, limit+4)
		}
		if info.fInner {
			lossySimpleVFilter16i(planes.y, yPos, planes.yStride, limit)
		}
	case filterComplex:
		if mbX > 0 {
			lossyHFilter16(planes.y, yPos, planes.yStride, limit+4, inner, hev)
			lossyHFilter8(planes.u, planes.v, uvPos, planes.uvStride, limit+4, inner, hev)
		}
		if info.fInner {
			lossyHFilter16i(planes.y, yPos, planes.yStride, limit, inner, hev)
			lossyHFilter8i(planes.u, planes.v, uvPos, planes.uvStride, limit, inner, hev)
		}
		if mbY > 0 {
			lossyVFilter16(planes.y, yPos, planes.yStride, limit+4, inner, hev)
			lossyVFilter8(planes.u, planes.v, uvPos, planes.uvStride, limit+4, inner, hev)
		}
		if info.fInner {
			lossyVFilter16i(planes.y, yPos, planes.yStride, limit, inner, hev)
			lossyVFilter8i(planes.u, planes.v, uvPos, planes.uvStride, limit, inner, hev)
		}
	}
}

func lossyApplyLoopFilter(frame *macroBlockDataFrame, planes *lossyPlanes) {
	if frame.Frame.Filter.filterType == filterOff {
		return
	}

	for mbY := 0; mbY < frame.Frame.MacroblockHeight; mbY++ {
		for mbX := 0; mbX < frame.Frame.MacroblockWidth; mbX++ {
			macroblock := &frame.Macroblocks[mbY*frame.Frame.MacroblockWidth+mbX]
			lossyFilterMacroblock(frame, planes, mbX, mbY, macroblock)
		}
	}
}

func lossyMul1(value int32) int32 {
	return ((value * lossyVp8TransformAC3C1) >> 16) + value
}

func lossyMul2(value int32) int32 {
	return (value * lossyVp8TransformAC3C2) >> 16
}

func lossyAvg2(a, b byte) byte {
	return byte((uint16(a) + uint16(b) + 1) >> 1)
}

func lossyAvg3(a, b, c byte) byte {
	return byte((uint16(a) + 2*uint16(b) + uint16(c) + 2) >> 2)
}

func lossyTopLeftSample(plane []byte, stride, x, y int) byte {
	if y == 0 {
		return 127
	} else if x == 0 {
		return 129
	}
	return plane[(y-1)*stride+(x-1)]
}

func lossyTopSamples(plane []byte, stride, planeWidth, x, y, n int) []byte {
	out := make([]byte, n)
	if y == 0 {
		for i := range out {
			out[i] = 127
		}
		return out
	}
	row := (y - 1) * stride
	for i := 0; i < n; i++ {
		srcX := x + i
		if srcX > planeWidth-1 {
			srcX = planeWidth - 1
		}
		out[i] = plane[row+srcX]
	}
	return out
}

func lossyTopSamplesLuma4(plane []byte, stride, planeWidth, x, y int) [8]byte {
	var out [8]byte
	if y == 0 {
		for i := range out {
			out[i] = 127
		}
		return out
	}

	row := (y - 1) * stride
	for i := 0; i < 4; i++ {
		srcX := x + i
		if srcX > planeWidth-1 {
			srcX = planeWidth - 1
		}
		out[i] = plane[row+srcX]
	}

	localX := x & 15
	localY := y & 15
	if localX == 12 && localY != 0 {
		macroblockY := y - localY
		if macroblockY == 0 {
			for i := 4; i < 8; i++ {
				out[i] = 127
			}
		} else {
			topRow := (macroblockY - 1) * stride
			for i := 4; i < 8; i++ {
				srcX := x + i
				if srcX > planeWidth-1 {
					srcX = planeWidth - 1
				}
				out[i] = plane[topRow+srcX]
			}
		}
	} else {
		for i := 4; i < 8; i++ {
			srcX := x + i
			if srcX > planeWidth-1 {
				srcX = planeWidth - 1
			}
			out[i] = plane[row+srcX]
		}
	}
	return out
}

func lossyLeftSamples(plane []byte, stride, x, y, n int) []byte {
	out := make([]byte, n)
	if x == 0 {
		for i := range out {
			out[i] = 129
		}
		return out
	}
	srcX := x - 1
	for i := 0; i < n; i++ {
		out[i] = plane[(y+i)*stride+srcX]
	}
	return out
}

func lossyFillBlock(plane []byte, stride, x, y, width, height int, value byte) {
	for row := 0; row < height; row++ {
		offset := (y+row)*stride + x
		for c := 0; c < width; c++ {
			plane[offset+c] = value
		}
	}
}

func lossyPredictTrueMotion(plane []byte, stride, planeWidth, x, y, size int) {
	top := lossyTopSamples(plane, stride, planeWidth, x, y, size)
	left := lossyLeftSamples(plane, stride, x, y, size)
	topLeft := int32(lossyTopLeftSample(plane, stride, x, y))
	for row := 0; row < size; row++ {
		leftValue := int32(left[row])
		offset := (y+row)*stride + x
		for col := 0; col < size; col++ {
			plane[offset+col] = lossyClipByte(leftValue + int32(top[col]) - topLeft)
		}
	}
}

func lossyPredictLuma16(plane []byte, stride, planeWidth, x, y int, mode uint8) error {
	switch mode {
	case dcPred:
		hasTop := y > 0
		hasLeft := x > 0
		var value byte
		switch {
		case hasTop && hasLeft:
			top := lossyTopSamples(plane, stride, planeWidth, x, y, 16)
			left := lossyLeftSamples(plane, stride, x, y, 16)
			var sumTop, sumLeft uint32
			for _, t := range top {
				sumTop += uint32(t)
			}
			for _, l := range left {
				sumLeft += uint32(l)
			}
			value = byte((sumTop + sumLeft + 16) >> 5)
		case hasTop && !hasLeft:
			top := lossyTopSamples(plane, stride, planeWidth, x, y, 16)
			var sumTop uint32
			for _, t := range top {
				sumTop += uint32(t)
			}
			value = byte((sumTop + 8) >> 4)
		case !hasTop && hasLeft:
			left := lossyLeftSamples(plane, stride, x, y, 16)
			var sumLeft uint32
			for _, l := range left {
				sumLeft += uint32(l)
			}
			value = byte((sumLeft + 8) >> 4)
		default:
			value = 128
		}
		lossyFillBlock(plane, stride, x, y, 16, 16, value)
	case tmPred:
		lossyPredictTrueMotion(plane, stride, planeWidth, x, y, 16)
	case vPred:
		top := lossyTopSamples(plane, stride, planeWidth, x, y, 16)
		for row := 0; row < 16; row++ {
			offset := (y+row)*stride + x
			copy(plane[offset:offset+16], top)
		}
	case hPred:
		left := lossyLeftSamples(plane, stride, x, y, 16)
		for row := 0; row < 16; row++ {
			offset := (y+row)*stride + x
			for c := 0; c < 16; c++ {
				plane[offset+c] = left[row]
			}
		}
	default:
		return bitstreamErr("invalid luma prediction mode")
	}
	return nil
}

func lossyPredictChroma8(plane []byte, stride, planeWidth, x, y int, mode uint8) error {
	switch mode {
	case dcPred:
		hasTop := y > 0
		hasLeft := x > 0
		var value byte
		switch {
		case hasTop && hasLeft:
			top := lossyTopSamples(plane, stride, planeWidth, x, y, 8)
			left := lossyLeftSamples(plane, stride, x, y, 8)
			var sumTop, sumLeft uint32
			for _, t := range top {
				sumTop += uint32(t)
			}
			for _, l := range left {
				sumLeft += uint32(l)
			}
			value = byte((sumTop + sumLeft + 8) >> 4)
		case hasTop && !hasLeft:
			top := lossyTopSamples(plane, stride, planeWidth, x, y, 8)
			var sumTop uint32
			for _, t := range top {
				sumTop += uint32(t)
			}
			value = byte((sumTop + 4) >> 3)
		case !hasTop && hasLeft:
			left := lossyLeftSamples(plane, stride, x, y, 8)
			var sumLeft uint32
			for _, l := range left {
				sumLeft += uint32(l)
			}
			value = byte((sumLeft + 4) >> 3)
		default:
			value = 128
		}
		lossyFillBlock(plane, stride, x, y, 8, 8, value)
	case tmPred:
		lossyPredictTrueMotion(plane, stride, planeWidth, x, y, 8)
	case vPred:
		top := lossyTopSamples(plane, stride, planeWidth, x, y, 8)
		for row := 0; row < 8; row++ {
			offset := (y+row)*stride + x
			copy(plane[offset:offset+8], top)
		}
	case hPred:
		left := lossyLeftSamples(plane, stride, x, y, 8)
		for row := 0; row < 8; row++ {
			offset := (y+row)*stride + x
			for c := 0; c < 8; c++ {
				plane[offset+c] = left[row]
			}
		}
	default:
		return bitstreamErr("invalid chroma prediction mode")
	}
	return nil
}

func lossyPredictLuma4(plane []byte, stride, planeWidth, x, y int, mode uint8) error {
	x0 := lossyTopLeftSample(plane, stride, x, y)
	top := lossyTopSamplesLuma4(plane, stride, planeWidth, x, y)
	left := lossyLeftSamples(plane, stride, x, y, 4)

	a := top[0]
	b := top[1]
	c := top[2]
	d := top[3]
	e := top[4]
	f := top[5]
	g := top[6]
	h := top[7]
	i := left[0]
	j := left[1]
	k := left[2]
	l := left[3]

	var block [16]byte
	switch mode {
	case bDCPred:
		sumTop := uint32(a) + uint32(b) + uint32(c) + uint32(d)
		sumLeft := uint32(i) + uint32(j) + uint32(k) + uint32(l)
		dc := byte((sumTop + sumLeft + 4) >> 3)
		for n := range block {
			block[n] = dc
		}
	case bTMPred:
		topLeft := int32(x0)
		for row := 0; row < 4; row++ {
			leftValue := int32(left[row])
			for col := 0; col < 4; col++ {
				block[row*4+col] = lossyClipByte(leftValue + int32(top[col]) - topLeft)
			}
		}
	case bVEPred:
		vals := [4]byte{lossyAvg3(x0, a, b), lossyAvg3(a, b, c), lossyAvg3(b, c, d), lossyAvg3(c, d, e)}
		for row := 0; row < 4; row++ {
			copy(block[row*4:row*4+4], vals[:])
		}
	case bHEPred:
		vals := [4]byte{lossyAvg3(x0, i, j), lossyAvg3(i, j, k), lossyAvg3(j, k, l), lossyAvg3(k, l, l)}
		for row := 0; row < 4; row++ {
			for c := 0; c < 4; c++ {
				block[row*4+c] = vals[row]
			}
		}
	case bRDPred:
		block[12] = lossyAvg3(j, k, l)
		block[13] = lossyAvg3(i, j, k)
		block[8] = block[13]
		block[14] = lossyAvg3(x0, i, j)
		block[9] = block[14]
		block[4] = block[14]
		block[15] = lossyAvg3(a, x0, i)
		block[10] = block[15]
		block[5] = block[15]
		block[0] = block[15]
		block[11] = lossyAvg3(b, a, x0)
		block[6] = block[11]
		block[1] = block[11]
		block[7] = lossyAvg3(c, b, a)
		block[2] = block[7]
		block[3] = lossyAvg3(d, c, b)
	case bLDPred:
		block[0] = lossyAvg3(a, b, c)
		block[1] = lossyAvg3(b, c, d)
		block[4] = block[1]
		block[2] = lossyAvg3(c, d, e)
		block[5] = block[2]
		block[8] = block[2]
		block[3] = lossyAvg3(d, e, f)
		block[6] = block[3]
		block[9] = block[3]
		block[12] = block[3]
		block[7] = lossyAvg3(e, f, g)
		block[10] = block[7]
		block[13] = block[7]
		block[11] = lossyAvg3(f, g, h)
		block[14] = block[11]
		block[15] = lossyAvg3(g, h, h)
	case bVRPred:
		block[0] = lossyAvg2(x0, a)
		block[9] = block[0]
		block[1] = lossyAvg2(a, b)
		block[10] = block[1]
		block[2] = lossyAvg2(b, c)
		block[11] = block[2]
		block[3] = lossyAvg2(c, d)
		block[12] = lossyAvg3(k, j, i)
		block[8] = lossyAvg3(j, i, x0)
		block[4] = lossyAvg3(i, x0, a)
		block[13] = block[4]
		block[5] = lossyAvg3(x0, a, b)
		block[14] = block[5]
		block[6] = lossyAvg3(a, b, c)
		block[15] = block[6]
		block[7] = lossyAvg3(b, c, d)
	case bVLPred:
		block[0] = lossyAvg2(a, b)
		block[1] = lossyAvg2(b, c)
		block[2] = lossyAvg2(c, d)
		block[3] = lossyAvg2(d, e)
		block[4] = lossyAvg3(a, b, c)
		block[5] = lossyAvg3(b, c, d)
		block[6] = lossyAvg3(c, d, e)
		block[7] = lossyAvg3(d, e, f)
		block[8] = block[1]
		block[9] = block[2]
		block[10] = block[3]
		block[11] = lossyAvg3(e, f, g)
		block[12] = block[5]
		block[13] = block[6]
		block[14] = block[7]
		block[15] = lossyAvg3(f, g, h)
	case bHDPred:
		block[0] = lossyAvg2(i, x0)
		block[1] = lossyAvg3(i, x0, a)
		block[2] = lossyAvg3(x0, a, b)
		block[3] = lossyAvg3(a, b, c)
		block[4] = lossyAvg2(j, i)
		block[5] = lossyAvg3(j, i, x0)
		block[6] = block[0]
		block[7] = block[1]
		block[8] = lossyAvg2(k, j)
		block[9] = lossyAvg3(k, j, i)
		block[10] = block[4]
		block[11] = block[5]
		block[12] = lossyAvg2(l, k)
		block[13] = lossyAvg3(l, k, j)
		block[14] = block[8]
		block[15] = block[9]
	case bHUPred:
		block[0] = lossyAvg2(i, j)
		block[2] = lossyAvg2(j, k)
		block[4] = block[2]
		block[6] = lossyAvg2(k, l)
		block[8] = block[6]
		block[1] = lossyAvg3(i, j, k)
		block[3] = lossyAvg3(j, k, l)
		block[5] = block[3]
		block[7] = lossyAvg3(k, l, l)
		block[9] = block[7]
		block[11] = l
		block[10] = l
		block[12] = l
		block[13] = l
		block[14] = l
		block[15] = l
	default:
		return bitstreamErr("invalid 4x4 prediction mode")
	}

	for row := 0; row < 4; row++ {
		offset := (y+row)*stride + x
		copy(plane[offset:offset+4], block[row*4:row*4+4])
	}
	return nil
}

func lossyAddTransform(plane []byte, stride, x, y int, coeffs []int16) {
	allZero := true
	for _, coeff := range coeffs {
		if coeff != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		return
	}

	var tmp [16]int32
	for i := 0; i < 4; i++ {
		a := int32(coeffs[i]) + int32(coeffs[8+i])
		b := int32(coeffs[i]) - int32(coeffs[8+i])
		c := lossyMul2(int32(coeffs[4+i])) - lossyMul1(int32(coeffs[12+i]))
		d := lossyMul1(int32(coeffs[4+i])) + lossyMul2(int32(coeffs[12+i]))
		base := i * 4
		tmp[base] = a + d
		tmp[base+1] = b + c
		tmp[base+2] = b - c
		tmp[base+3] = a - d
	}

	for row := 0; row < 4; row++ {
		dc := tmp[row] + 4
		a := dc + tmp[8+row]
		b := dc - tmp[8+row]
		c := lossyMul2(tmp[4+row]) - lossyMul1(tmp[12+row])
		d := lossyMul1(tmp[4+row]) + lossyMul2(tmp[12+row])
		offset := (y+row)*stride + x
		plane[offset] = lossyClipByte(int32(plane[offset]) + ((a + d) >> 3))
		plane[offset+1] = lossyClipByte(int32(plane[offset+1]) + ((b + c) >> 3))
		plane[offset+2] = lossyClipByte(int32(plane[offset+2]) + ((b - c) >> 3))
		plane[offset+3] = lossyClipByte(int32(plane[offset+3]) + ((a - d) >> 3))
	}
}

func lossyReconstructMacroblock(planes *lossyPlanes, mbX, mbY int, macroblock *macroBlockData) error {
	yX := mbX * 16
	yY := mbY * 16
	yWidth := planes.yStride
	uvWidth := planes.uvStride

	if macroblock.Header.IsI4x4 {
		for subY := 0; subY < 4; subY++ {
			for subX := 0; subX < 4; subX++ {
				blockIndex := subY*4 + subX
				dstX := yX + subX*4
				dstY := yY + subY*4
				if err := lossyPredictLuma4(planes.y, planes.yStride, yWidth, dstX, dstY, macroblock.Header.SubModes[blockIndex]); err != nil {
					return err
				}
				coeffOffset := blockIndex * 16
				lossyAddTransform(planes.y, planes.yStride, dstX, dstY, macroblock.Coeffs[coeffOffset:coeffOffset+16])
			}
		}
	} else {
		if err := lossyPredictLuma16(planes.y, planes.yStride, yWidth, yX, yY, macroblock.Header.LumaMode); err != nil {
			return err
		}
		for subY := 0; subY < 4; subY++ {
			for subX := 0; subX < 4; subX++ {
				blockIndex := subY*4 + subX
				coeffOffset := blockIndex * 16
				lossyAddTransform(planes.y, planes.yStride, yX+subX*4, yY+subY*4, macroblock.Coeffs[coeffOffset:coeffOffset+16])
			}
		}
	}

	uvX := mbX * 8
	uvY := mbY * 8
	if err := lossyPredictChroma8(planes.u, planes.uvStride, uvWidth, uvX, uvY, macroblock.Header.UVMode); err != nil {
		return err
	}
	if err := lossyPredictChroma8(planes.v, planes.uvStride, uvWidth, uvX, uvY, macroblock.Header.UVMode); err != nil {
		return err
	}
	for subY := 0; subY < 2; subY++ {
		for subX := 0; subX < 2; subX++ {
			blockIndex := subY*2 + subX
			dstX := uvX + subX*4
			dstY := uvY + subY*4
			uOffset := 16*16 + blockIndex*16
			vOffset := 20*16 + blockIndex*16
			lossyAddTransform(planes.u, planes.uvStride, dstX, dstY, macroblock.Coeffs[uOffset:uOffset+16])
			lossyAddTransform(planes.v, planes.uvStride, dstX, dstY, macroblock.Coeffs[vOffset:vOffset+16])
		}
	}

	return nil
}

func lossyReconstructPlanes(frame *macroBlockDataFrame) (lossyPlanes, error) {
	expected := frame.Frame.MacroblockWidth * frame.Frame.MacroblockHeight
	if len(frame.Macroblocks) != expected {
		return lossyPlanes{}, bitstreamErr("macroblock count mismatch")
	}

	planes := lossyNewPlanes(frame)
	for mbY := 0; mbY < frame.Frame.MacroblockHeight; mbY++ {
		for mbX := 0; mbX < frame.Frame.MacroblockWidth; mbX++ {
			macroblock := &frame.Macroblocks[mbY*frame.Frame.MacroblockWidth+mbX]
			if err := lossyReconstructMacroblock(&planes, mbX, mbY, macroblock); err != nil {
				return lossyPlanes{}, err
			}
		}
	}
	lossyApplyLoopFilter(frame, &planes)
	return planes, nil
}

func lossyMultHi(value, coeff int32) int32 {
	return (value * coeff) >> 8
}

func lossyClipRGB(value int32) byte {
	if (value &^ lossyYUVMask2) == 0 {
		return byte(value >> lossyYUVFix2)
	} else if value < 0 {
		return 0
	}
	return 255
}

func lossyWriteRGBA(yy byte, u, v int32, dst []byte, offset int) {
	y := int32(yy)
	dst[offset] = lossyClipRGB(lossyMultHi(y, lossyRGBYCoeff) + lossyMultHi(v, lossyRGBVToR) - lossyRGBRBias)
	dst[offset+1] = lossyClipRGB(lossyMultHi(y, lossyRGBYCoeff) - lossyMultHi(u, lossyRGBUToG) - lossyMultHi(v, lossyRGBVToG) + lossyRGBGBias)
	dst[offset+2] = lossyClipRGB(lossyMultHi(y, lossyRGBYCoeff) + lossyMultHi(u, lossyRGBUToB) - lossyRGBBBias)
	dst[offset+3] = 255
}

func lossyUpsampleRGBALinePair(topY []byte, bottomY []byte, hasBottom bool, topU, topV, curU, curV []byte, rgba []byte, topOffset int, bottomOffset int, len_ int) {
	lastPixelPair := (len_ - 1) >> 1
	tlU := int32(topU[0])
	tlV := int32(topV[0])
	lU := int32(curU[0])
	lV := int32(curV[0])

	uv0U := (3*tlU + lU + 2) >> 2
	uv0V := (3*tlV + lV + 2) >> 2
	lossyWriteRGBA(topY[0], uv0U, uv0V, rgba, topOffset)
	if hasBottom {
		buU := (3*lU + tlU + 2) >> 2
		buV := (3*lV + tlV + 2) >> 2
		lossyWriteRGBA(bottomY[0], buU, buV, rgba, bottomOffset)
	}

	for x := 1; x <= lastPixelPair; x++ {
		tU := int32(topU[x])
		tV := int32(topV[x])
		u := int32(curU[x])
		v := int32(curV[x])

		avgU := tlU + tU + lU + u + 8
		avgV := tlV + tV + lV + v + 8
		diag12U := (avgU + 2*(tU+lU)) >> 3
		diag12V := (avgV + 2*(tV+lV)) >> 3
		diag03U := (avgU + 2*(tlU+u)) >> 3
		diag03V := (avgV + 2*(tlV+v)) >> 3

		topLeft := (2*x - 1) * 4
		topRight := 2 * x * 4
		lossyWriteRGBA(topY[2*x-1], (diag12U+tlU)>>1, (diag12V+tlV)>>1, rgba, topOffset+topLeft)
		lossyWriteRGBA(topY[2*x], (diag03U+tU)>>1, (diag03V+tV)>>1, rgba, topOffset+topRight)

		if hasBottom {
			lossyWriteRGBA(bottomY[2*x-1], (diag03U+lU)>>1, (diag03V+lV)>>1, rgba, bottomOffset+topLeft)
			lossyWriteRGBA(bottomY[2*x], (diag12U+u)>>1, (diag12V+v)>>1, rgba, bottomOffset+topRight)
		}

		tlU = tU
		tlV = tV
		lU = u
		lV = v
	}

	if len_&1 == 0 {
		last := (len_ - 1) * 4
		uv0U := (3*tlU + lU + 2) >> 2
		uv0V := (3*tlV + lV + 2) >> 2
		lossyWriteRGBA(topY[len_-1], uv0U, uv0V, rgba, topOffset+last)
		if hasBottom {
			buU := (3*lU + tlU + 2) >> 2
			buV := (3*lV + tlV + 2) >> 2
			lossyWriteRGBA(bottomY[len_-1], buU, buV, rgba, bottomOffset+last)
		}
	}
}

func lossyYuvToRgbaFancy(planes *lossyPlanes) []byte {
	rgba := make([]byte, planes.width*planes.height*4)
	if planes.width == 0 || planes.height == 0 {
		return rgba
	}

	uvWidth := (planes.width + 1) / 2
	uvHeight := (planes.height + 1) / 2

	topY := planes.y[:planes.width]
	topU := planes.u[:uvWidth]
	topV := planes.v[:uvWidth]
	lossyUpsampleRGBALinePair(topY, nil, false, topU, topV, topU, topV, rgba, 0, 0, planes.width)

	for uvRow := 1; uvRow < uvHeight; uvRow++ {
		topRow := 2*uvRow - 1
		bottomRow := topRow + 1
		if bottomRow >= planes.height {
			break
		}
		topYRow := planes.y[topRow*planes.yStride : topRow*planes.yStride+planes.width]
		bottomYRow := planes.y[bottomRow*planes.yStride : bottomRow*planes.yStride+planes.width]
		prevU := planes.u[(uvRow-1)*planes.uvStride : (uvRow-1)*planes.uvStride+uvWidth]
		prevV := planes.v[(uvRow-1)*planes.uvStride : (uvRow-1)*planes.uvStride+uvWidth]
		curU := planes.u[uvRow*planes.uvStride : uvRow*planes.uvStride+uvWidth]
		curV := planes.v[uvRow*planes.uvStride : uvRow*planes.uvStride+uvWidth]
		lossyUpsampleRGBALinePair(
			topYRow, bottomYRow, true,
			prevU, prevV, curU, curV,
			rgba,
			topRow*planes.width*4,
			bottomRow*planes.width*4,
			planes.width,
		)
	}

	if planes.height > 1 && planes.height&1 == 0 {
		lastRow := planes.height - 1
		uvRow := uvHeight - 1
		yRow := planes.y[lastRow*planes.yStride : lastRow*planes.yStride+planes.width]
		uRow := planes.u[uvRow*planes.uvStride : uvRow*planes.uvStride+uvWidth]
		vRow := planes.v[uvRow*planes.uvStride : uvRow*planes.uvStride+uvWidth]
		lossyUpsampleRGBALinePair(yRow, nil, false, uRow, vRow, uRow, vRow, rgba, lastRow*planes.width*4, 0, planes.width)
	}

	return rgba
}

func lossyIntoDecodedYuv(planes lossyPlanes) decodedYuvImage {
	return decodedYuvImage{
		Width:    planes.width,
		Height:   planes.height,
		YStride:  planes.yStride,
		UVStride: planes.uvStride,
		Y:        planes.y,
		U:        planes.u,
		V:        planes.v,
	}
}

// DecodeLossyVp8ToYuv decodes a raw "VP8 " frame payload to planar YUV420.
func decodeLossyVp8ToYuv(data []byte) (decodedYuvImage, error) {
	frame, err := parseMacroblockData(data)
	if err != nil {
		return decodedYuvImage{}, err
	}
	planes, err := lossyReconstructPlanes(&frame)
	if err != nil {
		return decodedYuvImage{}, err
	}
	return lossyIntoDecodedYuv(planes), nil
}

// DecodeLossyVp8ToRGBA decodes a raw "VP8 " frame payload to RGBA.
func decodeLossyVp8ToRGBA(data []byte) (decodedImage, error) {
	yuv, err := decodeLossyVp8ToYuv(data)
	if err != nil {
		return decodedImage{}, err
	}
	planes := lossyPlanes{
		width:    yuv.Width,
		height:   yuv.Height,
		yStride:  yuv.YStride,
		uvStride: yuv.UVStride,
		y:        yuv.Y,
		u:        yuv.U,
		v:        yuv.V,
	}
	return decodedImage{
		Width:  yuv.Width,
		Height: yuv.Height,
		RGBA:   lossyYuvToRgbaFancy(&planes),
	}, nil
}

func lossyApplyLossyAlpha(image *decodedImage, alphaData []byte) error {
	alpha, err := decodeAlphaPlane(alphaData, image.Width, image.Height)
	if err != nil {
		return err
	}
	return applyAlphaPlane(image.RGBA, alpha)
}

func decodeLossyVp8FrameToRGBA(imageData []byte, alphaData []byte) (decodedImage, error) {
	image, err := decodeLossyVp8ToRGBA(imageData)
	if err != nil {
		return decodedImage{}, err
	}
	if alphaData != nil {
		if err := lossyApplyLossyAlpha(&image, alphaData); err != nil {
			return decodedImage{}, err
		}
	}
	return image, nil
}

// DecodeLossyWebpToRGBA decodes a still lossy WebP container to RGBA. If an ALPH
// chunk is present, it is decoded and applied to the returned RGBA buffer.
func decodeLossyWebpToRGBA(data []byte) (decodedImage, error) {
	parsed, err := parseStillWebp(data)
	if err != nil {
		return decodedImage{}, err
	}
	if parsed.Features.Format != FormatLossy {
		return decodedImage{}, unsupportedErr("only still lossy WebP is supported")
	}
	return decodeLossyVp8FrameToRGBA(parsed.ImageData, parsed.AlphaData)
}

// DecodeLossyWebpToYuv decodes a still lossy WebP container to planar YUV420. It
// rejects input with alpha because the return type has no alpha channel.
func decodeLossyWebpToYuv(data []byte) (decodedYuvImage, error) {
	parsed, err := parseStillWebp(data)
	if err != nil {
		return decodedYuvImage{}, err
	}
	if parsed.Features.Format != FormatLossy {
		return decodedYuvImage{}, unsupportedErr("only still lossy WebP is supported")
	}
	if parsed.AlphaData != nil {
		return decodedYuvImage{}, unsupportedErr("lossy alpha is not implemented")
	}
	return decodeLossyVp8ToYuv(parsed.ImageData)
}
