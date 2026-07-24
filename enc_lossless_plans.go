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

func elosslessEstimateTransformCoefficient(pairs [][2]int32) int8 {
	numerator := int64(0)
	denominator := int64(0)
	for _, p := range pairs {
		numerator += int64(p[0]) * int64(p[1])
		denominator += int64(p[1]) * int64(p[1])
	}
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
	capacity := (endX - startX) * (endY - startY)

	redPairs := make([][2]int32, 0, capacity)
	blueGreenPairs := make([][2]int32, 0, capacity)
	for y := startY; y < endY; y++ {
		for x := startX; x < endX; x++ {
			pixel := argb[y*width+x]
			red := int32(int8(byte((pixel >> 16) & 0xff)))
			green := int32(int8(byte((pixel >> 8) & 0xff)))
			blue := int32(int8(byte(pixel & 0xff)))
			redPairs = append(redPairs, [2]int32{red, green})
			blueGreenPairs = append(blueGreenPairs, [2]int32{blue, green})
		}
	}

	greenToRed := elosslessEstimateTransformCoefficient(redPairs)
	greenToBlue := elosslessEstimateTransformCoefficient(blueGreenPairs)

	blueRedPairs := make([][2]int32, 0, capacity)
	for y := startY; y < endY; y++ {
		for x := startX; x < endX; x++ {
			pixel := argb[y*width+x]
			red := uint8((pixel >> 16) & 0xff)
			green := uint8((pixel >> 8) & 0xff)
			blue := uint8(pixel & 0xff)
			transformedBlue := uint8((int32(blue) - elosslessColorTransformDelta(greenToBlue, green)) & 0xff)
			blueRedPairs = append(blueRedPairs, [2]int32{int32(int8(transformedBlue)), int32(int8(red))})
		}
	}
	redToBlue := elosslessEstimateTransformCoefficient(blueRedPairs)

	return elosslessCrossColorTransform{
		greenToRed:  greenToRed,
		greenToBlue: greenToBlue,
		redToBlue:   redToBlue,
	}
}

func elosslessEstimateCrossColorTransform(argb []uint32) elosslessCrossColorTransform {
	redPairs := make([][2]int32, 0, len(argb))
	blueGreenPairs := make([][2]int32, 0, len(argb))
	for _, pixel := range argb {
		red := int32(int8(byte((pixel >> 16) & 0xff)))
		green := int32(int8(byte((pixel >> 8) & 0xff)))
		blue := int32(int8(byte(pixel & 0xff)))
		redPairs = append(redPairs, [2]int32{red, green})
		blueGreenPairs = append(blueGreenPairs, [2]int32{blue, green})
	}

	greenToRed := elosslessEstimateTransformCoefficient(redPairs)
	greenToBlue := elosslessEstimateTransformCoefficient(blueGreenPairs)

	blueRedPairs := make([][2]int32, 0, len(argb))
	for _, pixel := range argb {
		red := uint8((pixel >> 16) & 0xff)
		green := uint8((pixel >> 8) & 0xff)
		blue := uint8(pixel & 0xff)
		transformedBlue := uint8((int32(blue) - elosslessColorTransformDelta(greenToBlue, green)) & 0xff)
		blueRedPairs = append(blueRedPairs, [2]int32{int32(int8(transformedBlue)), int32(int8(red))})
	}
	redToBlue := elosslessEstimateTransformCoefficient(blueRedPairs)

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

func elosslessChoosePredictorMode(width, height int, argb []uint32, tileX, tileY, bits int) uint8 {
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
	var costs [elosslessNumPredictorModes]uint64
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
			elosslessScorePredictorRow(argb, width, y, x, interiorEnd, &costs)
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

func elosslessMakePredictorTransformImage(width, height int, argb []uint32) (int, int, []uint8, []uint32) {
	xsize := elosslessSubsampleSize(width, elosslessPredictorTransformBits)
	ysize := elosslessSubsampleSize(height, elosslessPredictorTransformBits)
	modes := make([]uint8, 0, xsize*ysize)
	image := make([]uint32, 0, xsize*ysize)
	for tileY := 0; tileY < ysize; tileY++ {
		for tileX := 0; tileX < xsize; tileX++ {
			mode := elosslessChoosePredictorMode(width, height, argb, tileX, tileY, elosslessPredictorTransformBits)
			modes = append(modes, mode)
			image = append(image, uint32(mode)<<8)
		}
	}
	return xsize, ysize, modes, image
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

func elosslessBuildGlobalTransformPlan(width, height int, input []uint32, useSubtractGreen bool) elosslessTransformPlan {
	crossPlan := elosslessBuildGlobalCrossPlan(width, height, input, useSubtractGreen)
	crossColored := crossPlan.predicted
	predictorWidth, _, predictorModes, predictorImage := elosslessMakeUniformPredictorTransformImage(width, height, elosslessGlobalPredictorTransformBits, elosslessGlobalPredictorMode)
	predicted := elosslessApplyPredictorTransform(width, height, crossColored, elosslessGlobalPredictorTransformBits, predictorModes)

	return elosslessTransformPlan{
		useSubtractGreen: useSubtractGreen,
		crossBits:        crossPlan.crossBits,
		crossBitsSet:     crossPlan.crossBitsSet,
		crossWidth:       crossPlan.crossWidth,
		crossImage:       crossPlan.crossImage,
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

func elosslessBuildTiledPredictorPlan(width, height int, input []uint32, useSubtractGreen bool) elosslessTransformPlan {
	predictorWidth, _, predictorModes, predictorImage := elosslessMakePredictorTransformImage(width, height, input)
	predicted := elosslessApplyPredictorTransform(width, height, input, elosslessPredictorTransformBits, predictorModes)

	return elosslessTransformPlan{
		useSubtractGreen: useSubtractGreen,
		predictorBits:    elosslessPredictorTransformBits,
		predictorBitsSet: true,
		predictorWidth:   predictorWidth,
		predictorImage:   predictorImage,
		predicted:        predicted,
	}
}

func elosslessBuildTiledTransformPlan(width, height int, input []uint32, useSubtractGreen bool) elosslessTransformPlan {
	crossPlan := elosslessBuildTiledCrossPlan(width, height, input, useSubtractGreen)
	crossColored := crossPlan.predicted
	predictorWidth, _, predictorModes, predictorImage := elosslessMakePredictorTransformImage(width, height, crossColored)
	predicted := elosslessApplyPredictorTransform(width, height, crossColored, elosslessPredictorTransformBits, predictorModes)

	return elosslessTransformPlan{
		useSubtractGreen: useSubtractGreen,
		crossBits:        crossPlan.crossBits,
		crossBitsSet:     crossPlan.crossBitsSet,
		crossWidth:       crossPlan.crossWidth,
		crossImage:       crossPlan.crossImage,
		predictorBits:    elosslessPredictorTransformBits,
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
			planeCode := elosslessDistanceToPlaneCode(width, token.distance)
			extraBits += elosslessPrefixExtraBitCount(token.length) + elosslessPrefixExtraBitCount(planeCode)
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

func elosslessCollectTransformPlans(width, height int, argb, subtractGreen []uint32, profile *elosslessLosslessSearchProfile) []elosslessTransformPlan {
	subtractIsDistinct := !elosslessSlicesEqualU32(subtractGreen, argb)
	plans := []elosslessTransformPlan{elosslessBuildRawPlan(argb)}

	if subtractIsDistinct && profile.transformSearchLevel >= 1 {
		plans = append(plans, elosslessBuildSubtractGreenPlan(subtractGreen))
	}
	if profile.transformSearchLevel >= 2 {
		plans = append(plans, elosslessBuildGlobalCrossPlan(width, height, argb, false))
		plans = append(plans, elosslessBuildGlobalPredictorPlan(width, height, argb, false))
	}
	if subtractIsDistinct && profile.transformSearchLevel >= 3 {
		plans = append(plans, elosslessBuildGlobalCrossPlan(width, height, subtractGreen, true))
		plans = append(plans, elosslessBuildGlobalPredictorPlan(width, height, subtractGreen, true))
	}
	if profile.transformSearchLevel >= 4 {
		plans = append(plans, elosslessBuildGlobalTransformPlan(width, height, argb, false))
		if subtractIsDistinct {
			plans = append(plans, elosslessBuildGlobalTransformPlan(width, height, subtractGreen, true))
		}
	}
	if profile.transformSearchLevel >= 5 {
		plans = append(plans, elosslessBuildTiledCrossPlan(width, height, argb, false))
		plans = append(plans, elosslessBuildTiledPredictorPlan(width, height, argb, false))
	}
	if subtractIsDistinct && profile.transformSearchLevel >= 6 {
		plans = append(plans, elosslessBuildTiledCrossPlan(width, height, subtractGreen, true))
		plans = append(plans, elosslessBuildTiledPredictorPlan(width, height, subtractGreen, true))
	}
	if profile.transformSearchLevel >= 7 {
		plans = append(plans, elosslessBuildTiledTransformPlan(width, height, argb, false))
		if subtractIsDistinct {
			plans = append(plans, elosslessBuildTiledTransformPlan(width, height, subtractGreen, true))
		}
	}

	return plans
}

func elosslessShortlistTransformPlans(width int, plans []elosslessTransformPlan, profile *elosslessLosslessSearchProfile) ([]elosslessRankedPlan, error) {
	ranked := make([]elosslessRankedPlan, 0, len(plans))
	for i := range plans {
		score, err := elosslessEstimateTransformPlanScore(width, &plans[i], profile)
		if err != nil {
			return nil, err
		}
		ranked = append(ranked, elosslessRankedPlan{score: score, plan: plans[i]})
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].score < ranked[j].score })
	keep := profile.shortlistKeep
	if len(ranked) < keep {
		keep = len(ranked)
	}
	ranked = ranked[:keep]
	return ranked, nil
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

func elosslessShouldStopTransformSearch(bestLen, nextEstimate int, profile *elosslessLosslessSearchProfile) bool {
	return profile.earlyStopRatioPercent != elosslessIntMax &&
		elosslessSatMul(nextEstimate, 100) >= elosslessSatMul(bestLen, profile.earlyStopRatioPercent)
}

// elosslessEncodeTransformPlanToVp8l tokenizes the predicted image once (no color
// cache) and derives every color-cache variant from that single token stream via
// elosslessApplyColorCacheToTokens. The LZ77 match structure is identical with or
// without a cache (the cache only reclassifies literals as cache references), so
// this avoids re-running the expensive match search once per cache-size candidate.
func elosslessEncodeTransformPlanToVp8l(width, height int, rgba []byte, plan *elosslessTransformPlan, profile *elosslessLosslessSearchProfile) ([]byte, error) {
	noCacheOptions := elosslessTokenBuildOptionsFor(profile.matchSearchLevel, 0)
	baseTokens, err := elosslessBuildTokens(width, plan.predicted, noCacheOptions)
	if err != nil {
		return nil, err
	}

	best, err := elosslessEncodeTransformPlanToVp8lWithTokens(width, height, rgba, plan, baseTokens, 0, profile.entropySearchLevel)
	if err != nil {
		return nil, err
	}

	if profile.useColorCache && len(plan.predicted) >= 64 {
		bestCacheBits, err := elosslessSelectBestColorCacheBits(width, height, plan.predicted, baseTokens, profile)
		if err != nil {
			return nil, err
		}
		if bestCacheBits > 0 {
			cachedTokens, err := elosslessApplyColorCacheToTokens(plan.predicted, baseTokens, bestCacheBits)
			if err != nil {
				return nil, err
			}
			withCache, err := elosslessEncodeTransformPlanToVp8lWithTokens(width, height, rgba, plan, cachedTokens, bestCacheBits, profile.entropySearchLevel)
			if err != nil {
				return nil, err
			}
			if len(withCache) < len(best) {
				best = withCache
			}
		}
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
	tokenOptions := noCacheOptions
	if profile.useColorCache && len(candidate.packedIndices) >= 64 {
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
