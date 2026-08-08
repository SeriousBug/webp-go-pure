package webp

// Transform planning and candidate selection for lossless encoding.
// Ported from src/encoder/lossless/plans.rs.

import "sort"

func elosslessApplySubtractGreenTransform(argb []uint32) []uint32 {
	out := make([]uint32, len(argb))
	for i, pixel := range argb {
		alpha := pixel & 0xff00_0000
		red := (pixel >> 16) & 0xff
		green := (pixel >> 8) & 0xff
		blue := pixel & 0xff
		red = (red - green) & 0xff
		blue = (blue - green) & 0xff
		out[i] = alpha | (red << 16) | (green << 8) | blue
	}
	return out
}

func elosslessColorTransformDelta(transform int8, color uint8) int32 {
	return (int32(transform) * int32(int8(color))) >> 5
}

func elosslessTransformCoefficientFromSums(numerator, denominator int64) int8 {
	if denominator == 0 {
		return 0
	}
	coefficient := (32 * numerator) / denominator
	if coefficient < -128 {
		coefficient = -128
	} else if coefficient > 127 {
		coefficient = 127
	}
	return int8(coefficient)
}

// elosslessGreenCrossSums accumulates the green-to-red and green-to-blue dot
// products for one pixel.
func elosslessGreenCrossSums(pixel uint32, redGreen, blueGreen, greenSq *int64) {
	red := int64(int8(byte((pixel >> 16) & 0xff)))
	green := int64(int8(byte((pixel >> 8) & 0xff)))
	blue := int64(int8(byte(pixel & 0xff)))
	*redGreen += red * green
	*blueGreen += blue * green
	*greenSq += green * green
}

// elosslessRedBlueCrossSums accumulates the red-to-blue dot products for one
// pixel, against the blue channel already adjusted by greenToBlue.
func elosslessRedBlueCrossSums(pixel uint32, greenToBlue int8, blueRed, redSq *int64) {
	red := uint8((pixel >> 16) & 0xff)
	green := uint8((pixel >> 8) & 0xff)
	blue := uint8(pixel & 0xff)
	transformedBlue := uint8((int32(blue) - elosslessColorTransformDelta(greenToBlue, green)) & 0xff)
	tb := int64(int8(transformedBlue))
	r := int64(int8(red))
	*blueRed += tb * r
	*redSq += r * r
}

func elosslessEstimateCrossColorTransformRegion(width, height int, argb []uint32, tileX, tileY, bits int) elosslessCrossColorTransform {
	startX := tileX << bits
	startY := tileY << bits
	endX := (tileX + 1) << bits
	if width < endX {
		endX = width
	}
	endY := (tileY + 1) << bits
	if height < endY {
		endY = height
	}
	var redGreen, blueGreen, greenSq int64
	for y := startY; y < endY; y++ {
		row := argb[y*width:]
		for x := startX; x < endX; x++ {
			elosslessGreenCrossSums(row[x], &redGreen, &blueGreen, &greenSq)
		}
	}

	greenToRed := elosslessTransformCoefficientFromSums(redGreen, greenSq)
	greenToBlue := elosslessTransformCoefficientFromSums(blueGreen, greenSq)

	var blueRed, redSq int64
	for y := startY; y < endY; y++ {
		row := argb[y*width:]
		for x := startX; x < endX; x++ {
			elosslessRedBlueCrossSums(row[x], greenToBlue, &blueRed, &redSq)
		}
	}
	redToBlue := elosslessTransformCoefficientFromSums(blueRed, redSq)

	return elosslessCrossColorTransform{
		greenToRed:  greenToRed,
		greenToBlue: greenToBlue,
		redToBlue:   redToBlue,
	}
}

func elosslessEstimateCrossColorTransform(argb []uint32) elosslessCrossColorTransform {
	var redGreen, blueGreen, greenSq int64
	for _, pixel := range argb {
		elosslessGreenCrossSums(pixel, &redGreen, &blueGreen, &greenSq)
	}

	greenToRed := elosslessTransformCoefficientFromSums(redGreen, greenSq)
	greenToBlue := elosslessTransformCoefficientFromSums(blueGreen, greenSq)

	var blueRed, redSq int64
	for _, pixel := range argb {
		elosslessRedBlueCrossSums(pixel, greenToBlue, &blueRed, &redSq)
	}
	redToBlue := elosslessTransformCoefficientFromSums(blueRed, redSq)

	return elosslessCrossColorTransform{
		greenToRed:  greenToRed,
		greenToBlue: greenToBlue,
		redToBlue:   redToBlue,
	}
}

func elosslessPackCrossColorTransform(transform elosslessCrossColorTransform) uint32 {
	return (uint32(uint8(transform.redToBlue)) << 16) |
		(uint32(uint8(transform.greenToBlue)) << 8) |
		uint32(uint8(transform.greenToRed))
}

func elosslessApplyCrossColorTransform(width, height int, argb []uint32, bits int, transforms []elosslessCrossColorTransform) []uint32 {
	tilesPerRow := elosslessSubsampleSize(width, bits)
	output := make([]uint32, 0, len(argb))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			transform := transforms[(y>>bits)*tilesPerRow+(x>>bits)]
			pixel := argb[y*width+x]
			alpha := pixel & 0xff00_0000
			red := uint8((pixel >> 16) & 0xff)
			green := uint8((pixel >> 8) & 0xff)
			blue := uint8(pixel & 0xff)

			transformedRed := uint32((int32(red) - elosslessColorTransformDelta(transform.greenToRed, green)) & 0xff)
			transformedBlue := (int32(blue) - elosslessColorTransformDelta(transform.greenToBlue, green)) & 0xff
			transformedBlue = (transformedBlue - elosslessColorTransformDelta(transform.redToBlue, red)) & 0xff

			output = append(output, alpha|(transformedRed<<16)|(uint32(green)<<8)|uint32(transformedBlue))
		}
	}
	return output
}

func elosslessAverage2(a, b uint32) uint32 {
	return (((a ^ b) & 0xfefe_fefe) >> 1) + (a & b)
}

func elosslessSelectPredictor(left, top, topLeft uint32) uint32 {
	predAlpha := int32(left>>24) + int32(top>>24) - int32(topLeft>>24)
	predRed := int32((left>>16)&0xff) + int32((top>>16)&0xff) - int32((topLeft>>16)&0xff)
	predGreen := int32((left>>8)&0xff) + int32((top>>8)&0xff) - int32((topLeft>>8)&0xff)
	predBlue := int32(left&0xff) + int32(top&0xff) - int32(topLeft&0xff)

	leftDistance := elosslessAbsI32(predAlpha-int32(left>>24)) +
		elosslessAbsI32(predRed-int32((left>>16)&0xff)) +
		elosslessAbsI32(predGreen-int32((left>>8)&0xff)) +
		elosslessAbsI32(predBlue-int32(left&0xff))
	topDistance := elosslessAbsI32(predAlpha-int32(top>>24)) +
		elosslessAbsI32(predRed-int32((top>>16)&0xff)) +
		elosslessAbsI32(predGreen-int32((top>>8)&0xff)) +
		elosslessAbsI32(predBlue-int32(top&0xff))

	if leftDistance < topDistance {
		return left
	}
	return top
}

func elosslessClip255(value int32) uint32 {
	if value < 0 {
		return 0
	}
	if value > 255 {
		return 255
	}
	return uint32(value)
}

func elosslessClampedAddSubtractFull(left, top, topLeft uint32) uint32 {
	alpha := elosslessClip255(int32(left>>24) + int32(top>>24) - int32(topLeft>>24))
	red := elosslessClip255(int32((left>>16)&0xff) + int32((top>>16)&0xff) - int32((topLeft>>16)&0xff))
	green := elosslessClip255(int32((left>>8)&0xff) + int32((top>>8)&0xff) - int32((topLeft>>8)&0xff))
	blue := elosslessClip255(int32(left&0xff) + int32(top&0xff) - int32(topLeft&0xff))
	return (alpha << 24) | (red << 16) | (green << 8) | blue
}

func elosslessClampedAddSubtractHalf(left, top, topLeft uint32) uint32 {
	avg := elosslessAverage2(left, top)
	alpha := elosslessClip255(int32(avg>>24) + (int32(avg>>24)-int32(topLeft>>24))/2)
	red := elosslessClip255(int32((avg>>16)&0xff) + (int32((avg>>16)&0xff)-int32((topLeft>>16)&0xff))/2)
	green := elosslessClip255(int32((avg>>8)&0xff) + (int32((avg>>8)&0xff)-int32((topLeft>>8)&0xff))/2)
	blue := elosslessClip255(int32(avg&0xff) + (int32(avg&0xff)-int32(topLeft&0xff))/2)
	return (alpha << 24) | (red << 16) | (green << 8) | blue
}

func elosslessPredictor(mode uint8, left, top, topLeft, topRight uint32) uint32 {
	switch mode {
	case 0:
		return 0xff00_0000
	case 1:
		return left
	case 2:
		return top
	case 3:
		return topRight
	case 4:
		return topLeft
	case 5:
		return elosslessAverage2(elosslessAverage2(left, topRight), top)
	case 6:
		return elosslessAverage2(left, topLeft)
	case 7:
		return elosslessAverage2(left, top)
	case 8:
		return elosslessAverage2(topLeft, top)
	case 9:
		return elosslessAverage2(top, topRight)
	case 10:
		return elosslessAverage2(elosslessAverage2(left, topLeft), elosslessAverage2(top, topRight))
	case 11:
		return elosslessSelectPredictor(left, top, topLeft)
	case 12:
		return elosslessClampedAddSubtractFull(left, top, topLeft)
	case 13:
		return elosslessClampedAddSubtractHalf(left, top, topLeft)
	default:
		return 0xff00_0000
	}
}

func elosslessPredictorForMode(argb []uint32, width, x, y int, mode uint8) uint32 {
	if y == 0 {
		if x == 0 {
			return 0xff00_0000
		}
		return argb[y*width+x-1]
	} else if x == 0 {
		return argb[(y-1)*width]
	}
	left := argb[y*width+x-1]
	top := argb[(y-1)*width+x]
	topLeft := argb[(y-1)*width+x-1]
	var topRight uint32
	if x+1 < width {
		topRight = argb[(y-1)*width+x+1]
	} else {
		topRight = argb[y*width]
	}
	return elosslessPredictor(mode, left, top, topLeft, topRight)
}

func elosslessSubPixels(a, b uint32) uint32 {
	alpha := uint32(byte(a>>24) - byte(b>>24))
	red := uint32(byte((a>>16)&0xff) - byte((b>>16)&0xff))
	green := uint32(byte((a>>8)&0xff) - byte((b>>8)&0xff))
	blue := uint32(byte(a&0xff) - byte(b&0xff))
	return (alpha << 24) | (red << 16) | (green << 8) | blue
}

func elosslessWrappedChannelError(actual, predicted uint32, shift uint32) uint32 {
	a := int32((actual >> shift) & 0xff)
	p := int32((predicted >> shift) & 0xff)
	delta := elosslessAbsI32(a - p)
	if 256-delta < delta {
		return uint32(256 - delta)
	}
	return uint32(delta)
}

func elosslessPredictorError(actual, predicted uint32) uint32 {
	return elosslessWrappedChannelError(actual, predicted, 24) +
		elosslessWrappedChannelError(actual, predicted, 16) +
		elosslessWrappedChannelError(actual, predicted, 8) +
		elosslessWrappedChannelError(actual, predicted, 0)
}

func elosslessScorePredictorTile(width, height int, argb []uint32, tileX, tileY, bits int) elosslessPredictorTileCosts {
	startX := tileX << bits
	startY := tileY << bits
	endX := (tileX + 1) << bits
	if width < endX {
		endX = width
	}
	endY := (tileY + 1) << bits
	if height < endY {
		endY = height
	}

	// Read each pixel's neighbours once and score all 14 predictor modes from
	// them, instead of re-reading neighbours per mode. Interior pixels (y>=1,
	// x>=1, x+1<width) are scored by elosslessScorePredictorRow, which the
	// arm64/amd64 builds vectorize; the borders are scored scalar here.
	var costs elosslessPredictorTileCosts
	for y := startY; y < endY; y++ {
		if y == 0 {
			for x := startX; x < endX; x++ {
				pred := elosslessPredictorForMode(argb, width, x, 0, 0)
				e := uint64(elosslessPredictorError(argb[x], pred))
				for mode := range costs {
					costs[mode] += e
				}
			}
			continue
		}
		x := startX
		if x == 0 {
			pred := elosslessPredictorForMode(argb, width, 0, y, 0)
			e := uint64(elosslessPredictorError(argb[y*width], pred))
			for mode := range costs {
				costs[mode] += e
			}
			x = 1
		}
		interiorEnd := endX
		if interiorEnd > width-1 {
			interiorEnd = width - 1
		}
		if x < interiorEnd {
			elosslessScorePredictorRow(argb, width, y, x, interiorEnd, (*[elosslessNumPredictorModes]uint64)(&costs))
			x = interiorEnd
		}
		for ; x < endX; x++ {
			actual := argb[y*width+x]
			left := argb[y*width+x-1]
			top := argb[(y-1)*width+x]
			topLeft := argb[(y-1)*width+x-1]
			var topRight uint32
			if x+1 < width {
				topRight = argb[(y-1)*width+x+1]
			} else {
				topRight = argb[y*width]
			}
			for mode := uint8(0); mode < elosslessNumPredictorModes; mode++ {
				pred := elosslessPredictor(mode, left, top, topLeft, topRight)
				costs[mode] += uint64(elosslessPredictorError(actual, pred))
			}
		}
	}

	return costs
}

// elosslessPredictorTileCosts holds one tile's total predictor error per mode.
// Costs are sums over pixels, so summing the costs of a 2x2 block of tiles gives
// the costs of the one tile that covers them at the next larger tile size. That
// is what lets a search over tile sizes score every pixel only once.
type elosslessPredictorTileCosts [elosslessNumPredictorModes]uint64

func (costs *elosslessPredictorTileCosts) bestMode() uint8 {
	bestMode := uint8(11)
	bestCost := ^uint64(0)
	for mode := uint8(0); mode < elosslessNumPredictorModes; mode++ {
		if costs[mode] < bestCost {
			bestCost = costs[mode]
			bestMode = mode
		}
	}
	return bestMode
}

// elosslessPredictorTileScorer serves predictor transform images at any tile
// size at or above the one it was built for, scoring the pixels only once.
type elosslessPredictorTileScorer struct {
	bits  int
	xsize int
	ysize int
	costs []elosslessPredictorTileCosts
}

func elosslessNewPredictorTileScorer(width, height int, argb []uint32, bits int) *elosslessPredictorTileScorer {
	xsize := elosslessSubsampleSize(width, bits)
	ysize := elosslessSubsampleSize(height, bits)
	costs := make([]elosslessPredictorTileCosts, 0, xsize*ysize)
	for tileY := 0; tileY < ysize; tileY++ {
		for tileX := 0; tileX < xsize; tileX++ {
			costs = append(costs, elosslessScorePredictorTile(width, height, argb, tileX, tileY, bits))
		}
	}
	return &elosslessPredictorTileScorer{bits: bits, xsize: xsize, ysize: ysize, costs: costs}
}

// coarsen halves the tile grid resolution in place, advancing the scorer one
// tile-size step. Requested sizes are served in increasing order, so each step
// is taken at most once.
func (s *elosslessPredictorTileScorer) coarsen() {
	xsize := elosslessDivCeil(s.xsize, 2)
	ysize := elosslessDivCeil(s.ysize, 2)
	costs := make([]elosslessPredictorTileCosts, xsize*ysize)
	for tileY := 0; tileY < s.ysize; tileY++ {
		for tileX := 0; tileX < s.xsize; tileX++ {
			dst := &costs[(tileY/2)*xsize+tileX/2]
			src := &s.costs[tileY*s.xsize+tileX]
			for mode := range dst {
				dst[mode] += src[mode]
			}
		}
	}
	s.bits++
	s.xsize, s.ysize, s.costs = xsize, ysize, costs
}

// transformImage returns the tile grid size, per-tile modes, and the packed
// transform image for the given tile size, which must be at least the size the
// scorer was built for and at least as large as any size requested before it.
func (s *elosslessPredictorTileScorer) transformImage(bits int) (int, int, []uint8, []uint32) {
	for s.bits < bits {
		s.coarsen()
	}
	modes := make([]uint8, len(s.costs))
	image := make([]uint32, len(s.costs))
	for i := range s.costs {
		mode := s.costs[i].bestMode()
		modes[i] = mode
		image[i] = uint32(mode) << 8
	}
	return s.xsize, s.ysize, modes, image
}

func elosslessApplyPredictorTransform(width, height int, argb []uint32, bits int, modes []uint8) []uint32 {
	tilesPerRow := elosslessSubsampleSize(width, bits)
	residuals := make([]uint32, len(argb))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			index := y*width + x
			mode := modes[(y>>bits)*tilesPerRow+(x>>bits)]
			pred := elosslessPredictorForMode(argb, width, x, y, mode)
			residuals[index] = elosslessSubPixels(argb[index], pred)
		}
	}
	return residuals
}

func elosslessSubsampleSize(size, bits int) int {
	return (size + (1 << bits) - 1) >> bits
}

func elosslessMakeUniformPredictorTransformImage(width, height, bits int, mode uint8) (int, int, []uint8, []uint32) {
	xsize := elosslessSubsampleSize(width, bits)
	ysize := elosslessSubsampleSize(height, bits)
	pixel := uint32(mode) << 8
	modes := make([]uint8, xsize*ysize)
	image := make([]uint32, xsize*ysize)
	for i := range modes {
		modes[i] = mode
		image[i] = pixel
	}
	return xsize, ysize, modes, image
}

func elosslessMakeCrossColorTransformImage(width, height int, argb []uint32) (int, int, []elosslessCrossColorTransform, []uint32) {
	xsize := elosslessSubsampleSize(width, elosslessCrossColorTransformBits)
	ysize := elosslessSubsampleSize(height, elosslessCrossColorTransformBits)
	transforms := make([]elosslessCrossColorTransform, 0, xsize*ysize)
	image := make([]uint32, 0, xsize*ysize)
	for tileY := 0; tileY < ysize; tileY++ {
		for tileX := 0; tileX < xsize; tileX++ {
			transform := elosslessEstimateCrossColorTransformRegion(width, height, argb, tileX, tileY, elosslessCrossColorTransformBits)
			transforms = append(transforms, transform)
			image = append(image, elosslessPackCrossColorTransform(transform))
		}
	}
	return xsize, ysize, transforms, image
}

func elosslessMakeUniformCrossColorTransformImage(width, height, bits int, transform elosslessCrossColorTransform) (int, int, []elosslessCrossColorTransform, []uint32) {
	xsize := elosslessSubsampleSize(width, bits)
	ysize := elosslessSubsampleSize(height, bits)
	pixel := elosslessPackCrossColorTransform(transform)
	transforms := make([]elosslessCrossColorTransform, xsize*ysize)
	image := make([]uint32, xsize*ysize)
	for i := range transforms {
		transforms[i] = transform
		image[i] = pixel
	}
	return xsize, ysize, transforms, image
}

func elosslessPaletteXbits(paletteSize int) int {
	if paletteSize <= 2 {
		return 3
	} else if paletteSize <= 4 {
		return 2
	} else if paletteSize <= 16 {
		return 1
	}
	return 0
}

func elosslessCollectPalette(argb []uint32) ([]uint32, bool) {
	unique := make(map[uint32]struct{}, 256)
	for _, pixel := range argb {
		unique[pixel] = struct{}{}
		if len(unique) > 256 {
			return nil, false
		}
	}
	palette := make([]uint32, 0, len(unique))
	for color := range unique {
		palette = append(palette, color)
	}
	sort.Slice(palette, func(i, j int) bool { return palette[i] < palette[j] })
	return palette, true
}

func elosslessBuildPaletteCandidate(width, height int, argb []uint32) (*elosslessPaletteCandidate, error) {
	palette, ok := elosslessCollectPalette(argb)
	if !ok || len(palette) == 0 {
		return nil, nil
	}
	xbits := elosslessPaletteXbits(len(palette))
	packedWidth := elosslessSubsampleSize(width, xbits)
	bitsPerPixel := 8 >> xbits
	pixelsPerByte := 1 << xbits
	indexByColor := make(map[uint32]uint8, len(palette))
	for index, color := range palette {
		indexByColor[color] = uint8(index)
	}
	packedIndices := make([]uint32, packedWidth*height)

	for y := 0; y < height; y++ {
		for packedX := 0; packedX < packedWidth; packedX++ {
			packed := uint32(0)
			for slot := 0; slot < pixelsPerByte; slot++ {
				x := packedX*pixelsPerByte + slot
				if x >= width {
					break
				}
				index, found := indexByColor[argb[y*width+x]]
				if !found {
					return nil, encBitstream("palette index lookup failed")
				}
				packed |= uint32(index) << (slot * bitsPerPixel)
			}
			packedIndices[y*packedWidth+packedX] = packed << 8
		}
	}

	return &elosslessPaletteCandidate{
		palette:       palette,
		packedWidth:   packedWidth,
		packedIndices: packedIndices,
	}, nil
}

func elosslessBuildGlobalCrossPlan(width, height int, input []uint32, useSubtractGreen bool) elosslessTransformPlan {
	crossTransform := elosslessEstimateCrossColorTransform(input)
	crossWidth, _, crossTransforms, crossImage := elosslessMakeUniformCrossColorTransformImage(width, height, elosslessGlobalCrossColorTransformBits, crossTransform)
	crossColored := elosslessApplyCrossColorTransform(width, height, input, elosslessGlobalCrossColorTransformBits, crossTransforms)

	return elosslessTransformPlan{
		useSubtractGreen: useSubtractGreen,
		crossBits:        elosslessGlobalCrossColorTransformBits,
		crossBitsSet:     true,
		crossWidth:       crossWidth,
		crossImage:       crossImage,
		predicted:        crossColored,
	}
}

func elosslessBuildRawPlan(argb []uint32) elosslessTransformPlan {
	predicted := make([]uint32, len(argb))
	copy(predicted, argb)
	return elosslessTransformPlan{predicted: predicted}
}

func elosslessBuildSubtractGreenPlan(subtractGreen []uint32) elosslessTransformPlan {
	predicted := make([]uint32, len(subtractGreen))
	copy(predicted, subtractGreen)
	return elosslessTransformPlan{useSubtractGreen: true, predicted: predicted}
}

func elosslessBuildGlobalPredictorPlan(width, height int, input []uint32, useSubtractGreen bool) elosslessTransformPlan {
	predictorWidth, _, predictorModes, predictorImage := elosslessMakeUniformPredictorTransformImage(width, height, elosslessGlobalPredictorTransformBits, elosslessGlobalPredictorMode)
	predicted := elosslessApplyPredictorTransform(width, height, input, elosslessGlobalPredictorTransformBits, predictorModes)

	return elosslessTransformPlan{
		useSubtractGreen: useSubtractGreen,
		predictorBits:    elosslessGlobalPredictorTransformBits,
		predictorBitsSet: true,
		predictorWidth:   predictorWidth,
		predictorImage:   predictorImage,
		predicted:        predicted,
	}
}

// elosslessBuildGlobalTransformPlan predicts first and fits the cross-color
// transform on the prediction residual. Fitting it on the source instead and
// predicting the recolored image, as this used to do, decorrelates channels that
// prediction has already decorrelated and inflates the residual by about 20%,
// which is why no combined plan ever won the ranking.
func elosslessBuildGlobalTransformPlan(width, height int, input []uint32, useSubtractGreen bool) elosslessTransformPlan {
	predictorWidth, _, predictorModes, predictorImage := elosslessMakeUniformPredictorTransformImage(width, height, elosslessGlobalPredictorTransformBits, elosslessGlobalPredictorMode)
	residual := elosslessApplyPredictorTransform(width, height, input, elosslessGlobalPredictorTransformBits, predictorModes)

	crossTransform := elosslessEstimateCrossColorTransform(residual)
	crossWidth, _, crossTransforms, crossImage := elosslessMakeUniformCrossColorTransformImage(width, height, elosslessGlobalCrossColorTransformBits, crossTransform)
	predicted := elosslessApplyCrossColorTransform(width, height, residual, elosslessGlobalCrossColorTransformBits, crossTransforms)

	return elosslessTransformPlan{
		useSubtractGreen: useSubtractGreen,
		crossBits:        elosslessGlobalCrossColorTransformBits,
		crossBitsSet:     true,
		crossWidth:       crossWidth,
		crossImage:       crossImage,
		predictorBits:    elosslessGlobalPredictorTransformBits,
		predictorBitsSet: true,
		predictorWidth:   predictorWidth,
		predictorImage:   predictorImage,
		predicted:        predicted,
	}
}

func elosslessBuildTiledCrossPlan(width, height int, input []uint32, useSubtractGreen bool) elosslessTransformPlan {
	crossWidth, _, crossTransforms, crossImage := elosslessMakeCrossColorTransformImage(width, height, input)
	crossColored := elosslessApplyCrossColorTransform(width, height, input, elosslessCrossColorTransformBits, crossTransforms)

	return elosslessTransformPlan{
		useSubtractGreen: useSubtractGreen,
		crossBits:        elosslessCrossColorTransformBits,
		crossBitsSet:     true,
		crossWidth:       crossWidth,
		crossImage:       crossImage,
		predicted:        crossColored,
	}
}

func elosslessBuildTiledPredictorPlan(width, height int, input []uint32, useSubtractGreen bool, bits int) elosslessTransformPlan {
	return elosslessBuildScoredTiledPredictorPlan(width, height, input, useSubtractGreen, elosslessNewPredictorTileScorer(width, height, input, bits), bits)
}

func elosslessBuildScoredTiledPredictorPlan(width, height int, input []uint32, useSubtractGreen bool, scorer *elosslessPredictorTileScorer, bits int) elosslessTransformPlan {
	predictorWidth, _, predictorModes, predictorImage := scorer.transformImage(bits)
	predicted := elosslessApplyPredictorTransform(width, height, input, bits, predictorModes)

	return elosslessTransformPlan{
		useSubtractGreen: useSubtractGreen,
		predictorBits:    bits,
		predictorBitsSet: true,
		predictorWidth:   predictorWidth,
		predictorImage:   predictorImage,
		predicted:        predicted,
	}
}

func elosslessEstimateTokenStreamCostBytes(width int, argb []uint32, options elosslessTokenBuildOptions) (int, error) {
	tokens, err := elosslessBuildTokens(width, argb, options)
	if err != nil {
		return 0, err
	}
	histograms, err := elosslessBuildHistograms(tokens, width, 0)
	if err != nil {
		return 0, err
	}
	group, err := elosslessBuildGroupCodes(&histograms)
	if err != nil {
		return 0, err
	}
	extraBits := 0
	for _, token := range tokens {
		if token.kind == elosslessTokCopy {
			planeCode := elosslessDistanceToPlaneCode(width, int(token.distance))
			extraBits += elosslessPrefixExtraBitCount(int(token.length)) + elosslessPrefixExtraBitCount(planeCode)
		}
	}
	totalBits := elosslessHistogramCost(&histograms, &group) + extraBits + len(tokens)
	return elosslessDivCeil(totalBits, 8), nil
}

// elosslessChannelEntropyBytes estimates the compressed size of an image from the
// Shannon entropy of its per-channel residual histograms. This is a cheap O(n)
// proxy (no LZ77) used only to rank candidate transform plans; the shortlisted
// winners are still fully encoded and compared by real output size, so a coarse
// ranking here cannot regress the final result.
func elosslessChannelEntropyBytes(argb []uint32) int {
	var hg, hr, hb, ha [256]uint32
	for _, p := range argb {
		ha[(p>>24)&0xff]++
		hr[(p>>16)&0xff]++
		hg[(p>>8)&0xff]++
		hb[p&0xff]++
	}
	bits := elosslessHistogramEntropyCost(hg[:]) +
		elosslessHistogramEntropyCost(hr[:]) +
		elosslessHistogramEntropyCost(hb[:]) +
		elosslessHistogramEntropyCost(ha[:])
	return int(bits / 8)
}

func elosslessEstimateTransformPlanScore(width int, plan *elosslessTransformPlan, profile *elosslessLosslessSearchProfile) (int, error) {
	transformOptions := elosslessTokenBuildOptions{}
	score := elosslessChannelEntropyBytes(plan.predicted)
	if plan.useSubtractGreen {
		score += 1
	}
	if len(plan.crossImage) != 0 {
		crossCost, err := elosslessEstimateTokenStreamCostBytes(plan.crossWidth, plan.crossImage, transformOptions)
		if err != nil {
			return 0, err
		}
		score += 2 + crossCost
	}
	if len(plan.predictorImage) != 0 {
		predictorCost, err := elosslessEstimateTokenStreamCostBytes(plan.predictorWidth, plan.predictorImage, transformOptions)
		if err != nil {
			return 0, err
		}
		score += 2 + predictorCost
	}
	return score, nil
}

// elosslessPlanBuilder builds one candidate transform plan. Builders sharing a
// non-zero family are alternative parameterizations of the same transform rather
// than genuinely different plans, so only the best-scoring one of a family
// reaches the shortlist; without that they would crowd out every other plan.
type elosslessPlanBuilder struct {
	family int
	build  func(width, height int) elosslessTransformPlan
}

const (
	elosslessPlanFamilyNone = iota
	elosslessPlanFamilyTiledPredictor
	elosslessPlanFamilyTiledPredictorSubtractGreen
	elosslessPlanFamilyGlobalPredictor
	elosslessPlanFamilyGlobalPredictorSubtractGreen
)

func elosslessTransformPlanBuilders(argb, subtractGreen []uint32, profile *elosslessLosslessSearchProfile) []elosslessPlanBuilder {
	subtractIsDistinct := !elosslessSlicesEqualU32(subtractGreen, argb)
	builders := []elosslessPlanBuilder{
		{build: func(_, _ int) elosslessTransformPlan { return elosslessBuildRawPlan(argb) }},
	}

	if subtractIsDistinct && profile.transformSearchLevel >= 1 {
		builders = append(builders, elosslessPlanBuilder{build: func(_, _ int) elosslessTransformPlan {
			return elosslessBuildSubtractGreenPlan(subtractGreen)
		}})
	}
	// Every profile gets a tiled predictor plan on the input it is most likely to
	// want. Spatial prediction is worth more than anything else the transform
	// search finds, so even the profiles that run no search get one. The levels
	// that cover both inputs below are excluded here to avoid building the same
	// plan twice.
	if profile.transformSearchLevel < 5 || (subtractIsDistinct && profile.transformSearchLevel < 6) {
		input, useSubtractGreen := argb, false
		family := elosslessPlanFamilyTiledPredictor
		if subtractIsDistinct {
			input, useSubtractGreen = subtractGreen, true
			family = elosslessPlanFamilyTiledPredictorSubtractGreen
		}
		builders = append(builders, elosslessTiledPredictorBuilders(input, useSubtractGreen, profile.predictorTileBits, family)...)
	}
	if profile.transformSearchLevel >= 2 {
		builders = append(builders,
			elosslessPlanBuilder{build: func(w, h int) elosslessTransformPlan { return elosslessBuildGlobalCrossPlan(w, h, argb, false) }},
			elosslessPlanBuilder{family: elosslessPlanFamilyGlobalPredictor, build: func(w, h int) elosslessTransformPlan { return elosslessBuildGlobalPredictorPlan(w, h, argb, false) }})
	}
	if subtractIsDistinct && profile.transformSearchLevel >= 3 {
		builders = append(builders,
			elosslessPlanBuilder{build: func(w, h int) elosslessTransformPlan { return elosslessBuildGlobalCrossPlan(w, h, subtractGreen, true) }},
			elosslessPlanBuilder{family: elosslessPlanFamilyGlobalPredictorSubtractGreen, build: func(w, h int) elosslessTransformPlan {
				return elosslessBuildGlobalPredictorPlan(w, h, subtractGreen, true)
			}})
	}
	if profile.transformSearchLevel >= 4 {
		builders = append(builders,
			elosslessPlanBuilder{family: elosslessPlanFamilyGlobalPredictor, build: func(w, h int) elosslessTransformPlan { return elosslessBuildGlobalTransformPlan(w, h, argb, false) }})
		if subtractIsDistinct {
			builders = append(builders, elosslessPlanBuilder{family: elosslessPlanFamilyGlobalPredictorSubtractGreen, build: func(w, h int) elosslessTransformPlan {
				return elosslessBuildGlobalTransformPlan(w, h, subtractGreen, true)
			}})
		}
	}
	if profile.transformSearchLevel >= 5 {
		builders = append(builders,
			elosslessPlanBuilder{build: func(w, h int) elosslessTransformPlan { return elosslessBuildTiledCrossPlan(w, h, argb, false) }})
		builders = append(builders, elosslessTiledPredictorBuilders(argb, false, profile.predictorTileBits, elosslessPlanFamilyTiledPredictor)...)
	}
	if subtractIsDistinct && profile.transformSearchLevel >= 6 {
		builders = append(builders,
			elosslessPlanBuilder{build: func(w, h int) elosslessTransformPlan { return elosslessBuildTiledCrossPlan(w, h, subtractGreen, true) }})
		builders = append(builders, elosslessTiledPredictorBuilders(subtractGreen, true, profile.predictorTileBits, elosslessPlanFamilyTiledPredictorSubtractGreen)...)
	}
	return builders
}

// elosslessTiledPredictorBuilders builds one tiled predictor plan per requested
// tile size, all off a single shared scorer so the per-pixel predictor mode
// scoring runs once rather than once per tile size. The sizes are handed out in
// increasing order because the scorer only coarsens.
func elosslessTiledPredictorBuilders(input []uint32, useSubtractGreen bool, tileBits []int, family int) []elosslessPlanBuilder {
	sorted := append([]int(nil), tileBits...)
	sort.Ints(sorted)
	if len(sorted) == 1 {
		family = elosslessPlanFamilyNone
	}
	var scorer *elosslessPredictorTileScorer
	builders := make([]elosslessPlanBuilder, 0, len(sorted))
	for _, bits := range sorted {
		bits := bits
		builders = append(builders, elosslessPlanBuilder{family: family, build: func(w, h int) elosslessTransformPlan {
			if scorer == nil {
				scorer = elosslessNewPredictorTileScorer(w, h, input, sorted[0])
			}
			return elosslessBuildScoredTiledPredictorPlan(w, h, input, useSubtractGreen, scorer, bits)
		}})
	}
	return builders
}

// elosslessShortlistTransformPlans scores every candidate transform and keeps
// the best few. Candidates are built one at a time and their predicted images
// are dropped after scoring: a plan's transform tile images fully determine its
// predicted image, so the shortlisted ones are rebuilt by
// elosslessRematerializePlan rather than kept alive at 4 bytes per pixel each.
func elosslessShortlistTransformPlans(width, height int, argb, subtractGreen []uint32, profile *elosslessLosslessSearchProfile) ([]elosslessRankedPlan, error) {
	builders := elosslessTransformPlanBuilders(argb, subtractGreen, profile)
	ranked := make([]elosslessRankedPlan, 0, len(builders))
	bestOfFamily := make(map[int]int, len(builders))
	for _, builder := range builders {
		plan := builder.build(width, height)
		score, err := elosslessEstimateTransformPlanScore(width, &plan, profile)
		if err != nil {
			return nil, err
		}
		plan.predicted = nil
		if builder.family != elosslessPlanFamilyNone {
			if at, seen := bestOfFamily[builder.family]; seen {
				if score < ranked[at].score {
					ranked[at] = elosslessRankedPlan{score: score, plan: plan}
				}
				continue
			}
			bestOfFamily[builder.family] = len(ranked)
		}
		ranked = append(ranked, elosslessRankedPlan{score: score, plan: plan})
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].score < ranked[j].score })
	keep := profile.shortlistKeep
	if len(ranked) < keep {
		keep = len(ranked)
	}
	ranked = ranked[:keep]
	return ranked, nil
}

// elosslessRematerializePlan recomputes plan.predicted from the plan's transform
// tile images, reapplying the already-chosen transforms without repeating the
// per-tile search that selected them.
func elosslessRematerializePlan(width, height int, argb, subtractGreen []uint32, plan *elosslessTransformPlan) {
	input := argb
	if plan.useSubtractGreen {
		input = subtractGreen
	}
	owned := false

	if plan.predictorBitsSet {
		modes := make([]uint8, len(plan.predictorImage))
		for i, packed := range plan.predictorImage {
			modes[i] = uint8((packed >> 8) & 0xff)
		}
		input = elosslessApplyPredictorTransform(width, height, input, plan.predictorBits, modes)
		owned = true
	}

	if plan.crossBitsSet {
		transforms := make([]elosslessCrossColorTransform, len(plan.crossImage))
		for i, packed := range plan.crossImage {
			transforms[i] = elosslessCrossColorTransform{
				greenToRed:  int8(packed & 0xff),
				greenToBlue: int8((packed >> 8) & 0xff),
				redToBlue:   int8((packed >> 16) & 0xff),
			}
		}
		input = elosslessApplyCrossColorTransform(width, height, input, plan.crossBits, transforms)
		owned = true
	}

	if !owned {
		predicted := make([]uint32, len(input))
		copy(predicted, input)
		input = predicted
	}
	plan.predicted = input
}

type elosslessRankedPlan struct {
	score int
	plan  elosslessTransformPlan
}

func elosslessSatMul(a, b int) int {
	if a == 0 || b == 0 {
		return 0
	}
	s := a * b
	if s/b != a {
		return elosslessIntMax
	}
	return s
}

// elosslessShouldStopTransformSearch decides whether to skip fully encoding the
// next shortlisted plan. It compares that plan's cheap entropy estimate against
// the best plan's estimate (both the same proxy scale, so the ratio is
// meaningful) and stops once the next plan is more than earlyStopRatioPercent
// worse. The best-ranked plan by this proxy reliably produces the smallest real
// output on photographic images, so encoding a clearly-worse plan is wasted
// work; a tight margin still re-encodes genuine near-ties where the proxy can't
// separate the candidates.
func elosslessShouldStopTransformSearch(bestEstimate, nextEstimate int, profile *elosslessLosslessSearchProfile) bool {
	return profile.earlyStopRatioPercent != elosslessIntMax &&
		elosslessSatMul(nextEstimate, 100) >= elosslessSatMul(bestEstimate, profile.earlyStopRatioPercent)
}

// elosslessParseCostCacheBits reports the color cache size the parse should
// score against. The cache is applied after tokenization, so the parse is told
// how large it will be rather than which pixels land in it; the exact size the
// later search settles on only shifts the hit rate slightly, and scoring against
// the largest size the profile can pick is far closer than assuming no cache.
func elosslessParseCostCacheBits(argb []uint32, profile *elosslessLosslessSearchProfile) int {
	if len(argb) < 64 {
		return 0
	}
	if profile.fixedColorCacheBits > 0 {
		return profile.fixedColorCacheBits
	}
	if !profile.useColorCache {
		return 0
	}
	return elosslessMaxColorCacheBitsForProfile(profile)
}

// elosslessEncodeTransformPlanToVp8l tokenizes the predicted image once (no color
// cache) and derives every color-cache variant from that single token stream via
// elosslessApplyColorCacheToTokens. The LZ77 match structure is identical with or
// without a cache (the cache only reclassifies literals as cache references), so
// this avoids re-running the expensive match search once per cache-size candidate.
// elosslessSelectBestColorCacheBits already compares an estimated stream size for
// no cache against every candidate size, so only its winner is written out; the
// no-cache stream is not encoded again to be measured and thrown away.
func elosslessEncodeTransformPlanToVp8l(width, height int, rgba []byte, plan *elosslessTransformPlan, profile *elosslessLosslessSearchProfile) ([]byte, error) {
	noCacheOptions := elosslessTokenBuildOptionsFor(profile.matchSearchLevel, 0)
	noCacheOptions.costCacheBits = elosslessParseCostCacheBits(plan.predicted, profile)
	baseTokens, err := elosslessBuildTokens(width, plan.predicted, noCacheOptions)
	if err != nil {
		return nil, err
	}

	cacheBits := 0
	switch {
	case len(plan.predicted) < 64:
	case profile.fixedColorCacheBits > 0:
		cacheBits = profile.fixedColorCacheBits
	case profile.useColorCache:
		cacheBits, err = elosslessSelectBestColorCacheBits(width, height, plan.predicted, baseTokens, profile)
		if err != nil {
			return nil, err
		}
	}

	if cacheBits > 0 {
		// baseTokens is dead past this point, so rewrite it in place rather than
		// allocating a second stream of one token per pixel.
		if err := elosslessApplyColorCacheToTokens(baseTokens, plan.predicted, baseTokens, cacheBits); err != nil {
			return nil, err
		}
	}
	tokens := baseTokens
	if profile.tokenCostPasses > 0 {
		refined, err := elosslessRetokenizeWithMeasuredCosts(width, plan.predicted, tokens, cacheBits, profile)
		if err != nil {
			return nil, err
		}
		if refined != nil {
			tokens = refined
		}
	}
	return elosslessEncodeTransformPlanToVp8lWithTokens(width, height, rgba, plan, tokens, cacheBits, profile.entropySearchLevel)
}

// elosslessRetokenizeWithMeasuredCosts re-parses the image, scoring matches
// against the code lengths the previous parse's own token stream implies instead
// of against flat cost constants. A first parse cannot know what a literal, a
// cache reference or a copy really costs on this image, and on flat graphics
// those differ by several bits, which is enough to change which matches are
// worth taking. Further passes re-measure because the first re-parse shifts the
// distance distribution enough to make its own costs stale. It returns nil when
// no pass beats the stream it was handed, so the extra work can only cost time.
func elosslessRetokenizeWithMeasuredCosts(width int, argb []uint32, tokens []elosslessToken, cacheBits int, profile *elosslessLosslessSearchProfile) ([]elosslessToken, error) {
	bestSize, err := elosslessEstimateSingleGroupSizeForTokens(width, tokens, cacheBits)
	if err != nil {
		return nil, err
	}
	var best []elosslessToken
	measured := tokens
	for pass := 0; pass < profile.tokenCostPasses; pass++ {
		histograms, err := elosslessBuildHistograms(measured, width, cacheBits)
		if err != nil {
			return nil, err
		}
		options := elosslessTokenBuildOptionsFor(profile.matchSearchLevel, cacheBits)
		options.symbolCosts = elosslessNewSymbolCosts(&histograms)
		refined, err := elosslessBuildTokens(width, argb, options)
		if err != nil {
			return nil, err
		}
		size, err := elosslessEstimateSingleGroupSizeForTokens(width, refined, cacheBits)
		if err != nil {
			return nil, err
		}
		if size < bestSize {
			bestSize, best = size, refined
		}
		measured = refined
	}
	return best, nil
}

// elosslessEncodeTransformPlanToVp8lWithTokens writes a full VP8L frame from an
// already-tokenized image stream, avoiding a redundant tokenization pass.
func elosslessEncodeTransformPlanToVp8lWithTokens(width, height int, rgba []byte, plan *elosslessTransformPlan, tokens []elosslessToken, colorCacheBits int, entropySearchLevel uint8) ([]byte, error) {
	transformOptions := elosslessTokenBuildOptions{}
	bw := newBitWriter()
	if err := bw.putBits(uint32(width-1), 14); err != nil {
		return nil, err
	}
	if err := bw.putBits(uint32(height-1), 14); err != nil {
		return nil, err
	}
	if err := bw.putBits(elosslessBoolBit(elosslessRgbaHasAlpha(rgba)), 1); err != nil {
		return nil, err
	}
	if err := bw.putBits(0, 3); err != nil {
		return nil, err
	}

	if plan.useSubtractGreen {
		if err := bw.putBits(1, 1); err != nil {
			return nil, err
		}
		if err := bw.putBits(2, 2); err != nil {
			return nil, err
		}
	}
	if plan.predictorBitsSet {
		if err := bw.putBits(1, 1); err != nil {
			return nil, err
		}
		if err := bw.putBits(0, 2); err != nil {
			return nil, err
		}
		if err := bw.putBits(uint32(plan.predictorBits-elosslessMinTransformBits), 3); err != nil {
			return nil, err
		}
		if err := elosslessWriteImageStream(bw, plan.predictorWidth, plan.predictorImage, false, 0, transformOptions); err != nil {
			return nil, err
		}
	}
	if plan.crossBitsSet {
		if err := bw.putBits(1, 1); err != nil {
			return nil, err
		}
		if err := bw.putBits(1, 2); err != nil {
			return nil, err
		}
		if err := bw.putBits(uint32(plan.crossBits-elosslessMinTransformBits), 3); err != nil {
			return nil, err
		}
		if err := elosslessWriteImageStream(bw, plan.crossWidth, plan.crossImage, false, 0, transformOptions); err != nil {
			return nil, err
		}
	}
	if err := bw.putBits(0, 1); err != nil {
		return nil, err
	}
	if err := elosslessWriteImageStreamFromTokens(bw, width, height, tokens, true, entropySearchLevel, colorCacheBits); err != nil {
		return nil, err
	}

	bitstream := bw.intoBytes()
	vp8l := newByteWriter(1 + len(bitstream))
	vp8l.writeByte(0x2f)
	vp8l.writeBytes(bitstream)
	return vp8l.intoBytes(), nil
}

func elosslessEncodePaletteCandidateToVp8l(width, height int, rgba []byte, candidate *elosslessPaletteCandidate, profile *elosslessLosslessSearchProfile) ([]byte, error) {
	transformOptions := elosslessTokenBuildOptions{}
	noCacheOptions := elosslessTokenBuildOptionsFor(profile.matchSearchLevel, 0)
	noCacheOptions.costCacheBits = elosslessParseCostCacheBits(candidate.packedIndices, profile)
	tokenOptions := noCacheOptions
	if profile.fixedColorCacheBits > 0 && len(candidate.packedIndices) >= 64 {
		tokenOptions = elosslessTokenBuildOptionsFor(profile.matchSearchLevel, profile.fixedColorCacheBits)
	} else if profile.useColorCache && len(candidate.packedIndices) >= 64 {
		baseTokens, err := elosslessBuildTokens(candidate.packedWidth, candidate.packedIndices, noCacheOptions)
		if err != nil {
			return nil, err
		}
		bestCacheBits, err := elosslessSelectBestColorCacheBits(candidate.packedWidth, height, candidate.packedIndices, baseTokens, profile)
		if err != nil {
			return nil, err
		}
		tokenOptions = elosslessTokenBuildOptionsFor(profile.matchSearchLevel, bestCacheBits)
	}

	paletteImage := make([]uint32, 0, len(candidate.palette))
	for index, color := range candidate.palette {
		if index == 0 {
			paletteImage = append(paletteImage, color)
		} else {
			paletteImage = append(paletteImage, elosslessSubPixels(color, candidate.palette[index-1]))
		}
	}

	bw := newBitWriter()
	if err := bw.putBits(uint32(width-1), 14); err != nil {
		return nil, err
	}
	if err := bw.putBits(uint32(height-1), 14); err != nil {
		return nil, err
	}
	if err := bw.putBits(elosslessBoolBit(elosslessRgbaHasAlpha(rgba)), 1); err != nil {
		return nil, err
	}
	if err := bw.putBits(0, 3); err != nil {
		return nil, err
	}

	if err := bw.putBits(1, 1); err != nil {
		return nil, err
	}
	if err := bw.putBits(3, 2); err != nil {
		return nil, err
	}
	if err := bw.putBits(uint32(len(candidate.palette)-1), 8); err != nil {
		return nil, err
	}
	if err := elosslessWriteImageStream(bw, len(candidate.palette), paletteImage, false, 0, transformOptions); err != nil {
		return nil, err
	}

	if err := bw.putBits(0, 1); err != nil {
		return nil, err
	}
	if err := elosslessWriteImageStream(bw, candidate.packedWidth, candidate.packedIndices, true, profile.entropySearchLevel, tokenOptions); err != nil {
		return nil, err
	}

	bitstream := bw.intoBytes()
	vp8l := newByteWriter(1 + len(bitstream))
	vp8l.writeByte(0x2f)
	vp8l.writeBytes(bitstream)
	return vp8l.intoBytes(), nil
}
