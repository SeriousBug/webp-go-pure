package webp

// elossyWeightY weighs the Hadamard coefficients of a residual by how much the
// eye and the quantizer care about them, low frequencies first. Taken from
// libwebp's kWeightY. The kernels below spell these values out as immediates;
// this table is what they are checked against.
var elossyWeightY = [16]uint32{
	38, 32, 20, 9,
	32, 28, 17, 7,
	20, 17, 10, 4,
	9, 7, 4, 2,
}

// elossyTDisto4x4Go scores a prediction against the source with a weighted
// Hadamard SATD of their difference. It stands in for the post-quantization
// distortion of a full trial at a fraction of the cost, so the mode search can
// rank candidates before transforming and quantizing any of them.
//
// The >>5 keeps the result in libwebp's Disto4x4 scale.
func elossyTDisto4x4Go(src []uint8, srcStride, srcX, srcY int, pred []uint8, predStride, predX, predY int) uint32 {
	var tmp [16]int32
	for row := 0; row < 4; row++ {
		srcOffset := (srcY+row)*srcStride + srcX
		predOffset := (predY+row)*predStride + predX
		srcRow := src[srcOffset : srcOffset+4 : srcOffset+4]
		predRow := pred[predOffset : predOffset+4 : predOffset+4]
		d0 := int32(srcRow[0]) - int32(predRow[0])
		d1 := int32(srcRow[1]) - int32(predRow[1])
		d2 := int32(srcRow[2]) - int32(predRow[2])
		d3 := int32(srcRow[3]) - int32(predRow[3])
		a0 := d0 + d2
		a1 := d1 + d3
		a2 := d1 - d3
		a3 := d0 - d2
		base := row * 4
		tmp[base] = a0 + a1
		tmp[base+1] = a3 + a2
		tmp[base+2] = a3 - a2
		tmp[base+3] = a0 - a1
	}
	return elossyTDistoVerticalPass(&tmp)
}

// elossyTDistoVerticalPass finishes a Hadamard SATD: the vertical butterflies
// and the weighted sum of absolute coefficients. The weights are spelled out so
// the multiplies stay immediate.
func elossyTDistoVerticalPass(tmp *[16]int32) uint32 {
	var sum int32
	for col := 0; col < 4; col++ {
		a0 := tmp[col] + tmp[8+col]
		a1 := tmp[4+col] + tmp[12+col]
		a2 := tmp[4+col] - tmp[12+col]
		a3 := tmp[col] - tmp[8+col]
		b0 := elossyAbsI32(a0 + a1)
		b1 := elossyAbsI32(a3 + a2)
		b2 := elossyAbsI32(a3 - a2)
		b3 := elossyAbsI32(a0 - a1)
		switch col {
		case 0:
			sum += 38*b0 + 32*b1 + 20*b2 + 9*b3
		case 1:
			sum += 32*b0 + 28*b1 + 17*b2 + 7*b3
		case 2:
			sum += 20*b0 + 17*b1 + 10*b2 + 4*b3
		default:
			sum += 9*b0 + 7*b1 + 4*b2 + 2*b3
		}
	}
	return uint32(sum) >> 5
}

// elossyTDisto4x4Contiguous is elossyTDisto4x4 for a prediction that already
// sits in a packed 4x4 block, which is how the 4x4 mode search holds them.
func elossyTDisto4x4Contiguous(src []uint8, srcStride, srcX, srcY int, pred *[16]uint8) uint32 {
	var tmp [16]int32
	offset := srcY*srcStride + srcX
	for row := 0; row < 4; row++ {
		srcRow := src[offset : offset+4 : offset+4]
		predRow := pred[row*4 : row*4+4 : row*4+4]
		d0 := int32(srcRow[0]) - int32(predRow[0])
		d1 := int32(srcRow[1]) - int32(predRow[1])
		d2 := int32(srcRow[2]) - int32(predRow[2])
		d3 := int32(srcRow[3]) - int32(predRow[3])
		a0 := d0 + d2
		a1 := d1 + d3
		a2 := d1 - d3
		a3 := d0 - d2
		base := row * 4
		tmp[base] = a0 + a1
		tmp[base+1] = a3 + a2
		tmp[base+2] = a3 - a2
		tmp[base+3] = a0 - a1
		offset += srcStride
	}
	return elossyTDistoVerticalPass(&tmp)
}

func elossyTDisto4x4(src []uint8, srcStride, srcX, srcY int, pred []uint8, predStride, predX, predY int) uint32 {
	return elossyTDisto4x4Go(src, srcStride, srcX, srcY, pred, predStride, predX, predY)
}

func elossyAbsI32(value int32) int32 {
	if value < 0 {
		return -value
	}
	return value
}

// elossyModeScreenTopK is how many of the proxy-ranked modes still get a full
// transform/quantize/reconstruct trial.
const elossyModeScreenTopK = 2

// elossyAllLuma4Candidates indexes every 4x4 mode, for the paths that skip the
// pre-screen and trial them all.
var elossyAllLuma4Candidates = [numBModes]uint8{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

// elossyModeScreen keeps the lowest-scoring modes seen so far in ascending
// score order.
type elossyModeScreen struct {
	limit  int
	count  int
	modes  [numBModes]uint8
	scores [numBModes]uint64
}

func (screen *elossyModeScreen) reset(limit int) {
	if limit > numBModes {
		limit = numBModes
	}
	screen.limit = limit
	screen.count = 0
}

func (screen *elossyModeScreen) add(mode uint8, score uint64) {
	if screen.count < screen.limit {
		screen.count++
	} else if score >= screen.scores[screen.limit-1] {
		return
	}
	index := screen.count - 1
	for index > 0 && screen.scores[index-1] > score {
		screen.scores[index] = screen.scores[index-1]
		screen.modes[index] = screen.modes[index-1]
		index--
	}
	screen.scores[index] = score
	screen.modes[index] = mode
}

func (screen *elossyModeScreen) selected() []uint8 {
	return screen.modes[:screen.count]
}

// elossyLuma16ProxyScore ranks a whole-macroblock luma mode by the summed
// Hadamard distortion of its sixteen sub-blocks plus the cost of signaling it.
func elossyLuma16ProxyScore(source *elossyPlanes, reconstructed *elossyPlanes, mbX, mbY int, quant *elossyQuantMatrices, rd *elossyRdMultipliers, mode uint8) uint64 {
	x := mbX * 16
	y := mbY * 16
	var prediction [16 * 16]uint8
	elossyFillPredictionBlock(reconstructed.y, reconstructed.yStride, reconstructed.yStride, x, y, mode, prediction[:], 16, 16)
	var distortion uint64
	for subY := 0; subY < 4; subY++ {
		for subX := 0; subX < 4; subX++ {
			distortion += uint64(elossyTDisto4x4(source.y, source.yStride, x+subX*4, y+subY*4, prediction[:], 16, subX*4, subY*4))
		}
	}
	return elossyRdScore(elossyScaleTDisto(distortion, quant.y1[1]), elossyI16ModeRate(mode), elossyModeRateLambda(rd))
}

// elossyChromaProxyScore is the chroma counterpart of elossyLuma16ProxyScore,
// summing the four sub-blocks of each of U and V.
func elossyChromaProxyScore(source *elossyPlanes, reconstructed *elossyPlanes, mbX, mbY int, quant *elossyQuantMatrices, rd *elossyRdMultipliers, mode uint8) uint64 {
	x := mbX * 8
	y := mbY * 8
	var predictionU [8 * 8]uint8
	var predictionV [8 * 8]uint8
	elossyFillPredictionBlock(reconstructed.u, reconstructed.uvStride, reconstructed.uvStride, x, y, mode, predictionU[:], 8, 8)
	elossyFillPredictionBlock(reconstructed.v, reconstructed.uvStride, reconstructed.uvStride, x, y, mode, predictionV[:], 8, 8)
	var distortion uint64
	for subY := 0; subY < 2; subY++ {
		for subX := 0; subX < 2; subX++ {
			distortion += uint64(elossyTDisto4x4(source.u, source.uvStride, x+subX*4, y+subY*4, predictionU[:], 8, subX*4, subY*4))
			distortion += uint64(elossyTDisto4x4(source.v, source.uvStride, x+subX*4, y+subY*4, predictionV[:], 8, subX*4, subY*4))
		}
	}
	return elossyRdScore(elossyScaleTDisto(distortion, quant.uv[1]), elossyUvModeRate(mode), elossyModeRateLambda(rd))
}

// elossyScaleTDisto brings a Hadamard SATD, which grows linearly with the
// residual, into the scale of the squared error the full trials score, so the
// screen can weigh it against the mode rate the same way they do. The residual
// a mode leaves behind scales with the quantizer.
func elossyScaleTDisto(distortion uint64, acQuant uint16) uint64 {
	return (distortion * uint64(acQuant)) >> 3
}

func elossyModeRateLambda(rd *elossyRdMultipliers) uint32 {
	if rd.mode < 1 {
		return 1
	}
	return rd.mode
}
