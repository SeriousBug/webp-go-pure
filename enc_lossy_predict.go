package webp

import (
	"math"
	"math/bits"
)

// clip_byte
func elossyClipByte(value int32) uint8 {
	if value < 0 {
		return 0
	}
	if value > 255 {
		return 255
	}
	return uint8(value)
}

func elossyTopLeftSample(plane []uint8, stride, x, y int) uint8 {
	if y == 0 {
		return 127
	} else if x == 0 {
		return 129
	}
	return plane[(y-1)*stride+(x-1)]
}

func elossyTopSamples(plane []uint8, stride, planeWidth, x, y, n int) []uint8 {
	out := make([]uint8, n)
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

func elossyTopSamplesLuma4(plane []uint8, stride, planeWidth, x, y int) [8]uint8 {
	var out [8]uint8
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

func elossyLeftSamples(plane []uint8, stride, x, y, n int) []uint8 {
	out := make([]uint8, n)
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

// avg2 averages two samples with integer rounding.
func elossyAvg2(a, b uint8) uint8 {
	return uint8((uint16(a) + uint16(b) + 1) >> 1)
}

// avg3 averages three samples with integer rounding.
func elossyAvg3(a, b, c uint8) uint8 {
	return uint8((uint16(a) + 2*uint16(b) + uint16(c) + 2) >> 2)
}

func elossyFillPredictionBlock(plane []uint8, stride, planeWidth, x, y int, mode uint8, out []uint8, outStride, n int) {
	switch mode {
	case dcPred:
		value := elossyDcPredictValue(plane, stride, x, y, n)
		for row := 0; row < n; row++ {
			offset := row * outStride
			for col := 0; col < n; col++ {
				out[offset+col] = value
			}
		}
	case vPred:
		top := elossyTopSamples(plane, stride, planeWidth, x, y, n)
		for row := 0; row < n; row++ {
			offset := row * outStride
			copy(out[offset:offset+n], top)
		}
	case hPred:
		left := elossyLeftSamples(plane, stride, x, y, n)
		for row := 0; row < n; row++ {
			offset := row * outStride
			for col := 0; col < n; col++ {
				out[offset+col] = left[row]
			}
		}
	case tmPred:
		top := elossyTopSamples(plane, stride, planeWidth, x, y, n)
		left := elossyLeftSamples(plane, stride, x, y, n)
		topLeft := int32(elossyTopLeftSample(plane, stride, x, y))
		for row := 0; row < n; row++ {
			leftValue := int32(left[row])
			offset := row * outStride
			for col := 0; col < n; col++ {
				out[offset+col] = elossyClipByte(leftValue + int32(top[col]) - topLeft)
			}
		}
	default:
		panic("unsupported macroblock prediction mode")
	}
}

// elossyLuma4Neighbors are the samples a 4x4 sub-block predicts from. They are
// all outside the sub-block, so they survive every mode trial and the search
// gathers them once instead of ten times.
type elossyLuma4Neighbors struct {
	topLeft uint8
	top     [8]uint8
	left    [4]uint8
}

func elossyGatherLuma4Neighbors(plane []uint8, stride, planeWidth, x, y int) elossyLuma4Neighbors {
	neighbors := elossyLuma4Neighbors{
		topLeft: elossyTopLeftSample(plane, stride, x, y),
		top:     elossyTopSamplesLuma4(plane, stride, planeWidth, x, y),
	}
	copy(neighbors.left[:], elossyLeftSamples(plane, stride, x, y, 4))
	return neighbors
}

func elossyFillLuma4PredictionBlock(plane []uint8, stride, planeWidth, x, y int, mode uint8, out []uint8, outStride int) {
	neighbors := elossyGatherLuma4Neighbors(plane, stride, planeWidth, x, y)
	elossyFillLuma4PredictionFrom(&neighbors, mode, out, outStride)
}

func elossyFillLuma4PredictionFrom(neighbors *elossyLuma4Neighbors, mode uint8, out []uint8, outStride int) {
	x0 := neighbors.topLeft
	top := &neighbors.top
	left := &neighbors.left

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

	var block [16]uint8
	switch mode {
	case bDCPred:
		sumTop := uint32(a) + uint32(b) + uint32(c) + uint32(d)
		sumLeft := uint32(i) + uint32(j) + uint32(k) + uint32(l)
		dc := uint8((sumTop + sumLeft + 4) >> 3)
		for idx := range block {
			block[idx] = dc
		}
	case bTMPred:
		topLeft := int32(x0)
		for row := 0; row < 4; row++ {
			leftValue := int32(left[row])
			for col := 0; col < 4; col++ {
				block[row*4+col] = elossyClipByte(leftValue + int32(top[col]) - topLeft)
			}
		}
	case bVEPred:
		vals := [4]uint8{elossyAvg3(x0, a, b), elossyAvg3(a, b, c), elossyAvg3(b, c, d), elossyAvg3(c, d, e)}
		for row := 0; row < 4; row++ {
			copy(block[row*4:row*4+4], vals[:])
		}
	case bHEPred:
		vals := [4]uint8{elossyAvg3(x0, i, j), elossyAvg3(i, j, k), elossyAvg3(j, k, l), elossyAvg3(k, l, l)}
		for row := 0; row < 4; row++ {
			for col := 0; col < 4; col++ {
				block[row*4+col] = vals[row]
			}
		}
	case bRDPred:
		block[12] = elossyAvg3(j, k, l)
		block[13] = elossyAvg3(i, j, k)
		block[8] = block[13]
		block[14] = elossyAvg3(x0, i, j)
		block[9] = block[14]
		block[4] = block[14]
		block[15] = elossyAvg3(a, x0, i)
		block[10] = block[15]
		block[5] = block[15]
		block[0] = block[15]
		block[11] = elossyAvg3(b, a, x0)
		block[6] = block[11]
		block[1] = block[11]
		block[7] = elossyAvg3(c, b, a)
		block[2] = block[7]
		block[3] = elossyAvg3(d, c, b)
	case bLDPred:
		block[0] = elossyAvg3(a, b, c)
		block[1] = elossyAvg3(b, c, d)
		block[4] = block[1]
		block[2] = elossyAvg3(c, d, e)
		block[5] = block[2]
		block[8] = block[2]
		block[3] = elossyAvg3(d, e, f)
		block[6] = block[3]
		block[9] = block[3]
		block[12] = block[3]
		block[7] = elossyAvg3(e, f, g)
		block[10] = block[7]
		block[13] = block[7]
		block[11] = elossyAvg3(f, g, h)
		block[14] = block[11]
		block[15] = elossyAvg3(g, h, h)
	case bVRPred:
		block[0] = elossyAvg2(x0, a)
		block[9] = block[0]
		block[1] = elossyAvg2(a, b)
		block[10] = block[1]
		block[2] = elossyAvg2(b, c)
		block[11] = block[2]
		block[3] = elossyAvg2(c, d)
		block[12] = elossyAvg3(k, j, i)
		block[8] = elossyAvg3(j, i, x0)
		block[4] = elossyAvg3(i, x0, a)
		block[13] = block[4]
		block[5] = elossyAvg3(x0, a, b)
		block[14] = block[5]
		block[6] = elossyAvg3(a, b, c)
		block[15] = block[6]
		block[7] = elossyAvg3(b, c, d)
	case bVLPred:
		block[0] = elossyAvg2(a, b)
		block[1] = elossyAvg2(b, c)
		block[2] = elossyAvg2(c, d)
		block[3] = elossyAvg2(d, e)
		block[4] = elossyAvg3(a, b, c)
		block[5] = elossyAvg3(b, c, d)
		block[6] = elossyAvg3(c, d, e)
		block[7] = elossyAvg3(d, e, f)
		block[8] = block[1]
		block[9] = block[2]
		block[10] = block[3]
		block[11] = elossyAvg3(e, f, g)
		block[12] = block[5]
		block[13] = block[6]
		block[14] = block[7]
		block[15] = elossyAvg3(f, g, h)
	case bHDPred:
		block[0] = elossyAvg2(i, x0)
		block[1] = elossyAvg3(i, x0, a)
		block[2] = elossyAvg3(x0, a, b)
		block[3] = elossyAvg3(a, b, c)
		block[4] = elossyAvg2(j, i)
		block[5] = elossyAvg3(j, i, x0)
		block[6] = block[0]
		block[7] = block[1]
		block[8] = elossyAvg2(k, j)
		block[9] = elossyAvg3(k, j, i)
		block[10] = block[4]
		block[11] = block[5]
		block[12] = elossyAvg2(l, k)
		block[13] = elossyAvg3(l, k, j)
		block[14] = block[8]
		block[15] = block[9]
	case bHUPred:
		block[0] = elossyAvg2(i, j)
		block[2] = elossyAvg2(j, k)
		block[4] = block[2]
		block[6] = elossyAvg2(k, l)
		block[8] = block[6]
		block[1] = elossyAvg3(i, j, k)
		block[3] = elossyAvg3(j, k, l)
		block[5] = block[3]
		block[7] = elossyAvg3(k, l, l)
		block[9] = block[7]
		block[10] = l
		block[11] = l
		block[12] = l
		block[13] = l
		block[14] = l
		block[15] = l
	default:
		panic("unsupported 4x4 prediction mode")
	}

	for row := 0; row < 4; row++ {
		src := row * 4
		dst := row * outStride
		copy(out[dst:dst+4], block[src:src+4])
	}
}

func elossyPredictBlock(plane []uint8, stride, planeWidth, x, y int, mode uint8, n int) {
	var scratch [16 * 16]uint8
	block := scratch[:n*n]
	elossyFillPredictionBlock(plane, stride, planeWidth, x, y, mode, block, n, n)
	for row := 0; row < n; row++ {
		src := row * n
		dst := (y+row)*stride + x
		copy(plane[dst:dst+n], block[src:src+n])
	}
}

func elossyPredictLuma4Block(plane []uint8, stride, planeWidth, x, y int, mode uint8) {
	neighbors := elossyGatherLuma4Neighbors(plane, stride, planeWidth, x, y)
	elossyPredictLuma4BlockFrom(plane, stride, x, y, &neighbors, mode)
}

func elossyPredictLuma4BlockFrom(plane []uint8, stride, x, y int, neighbors *elossyLuma4Neighbors, mode uint8) {
	var block [16]uint8
	elossyFillLuma4PredictionFrom(neighbors, mode, block[:], 4)
	for row := 0; row < 4; row++ {
		src := row * 4
		dst := (y+row)*stride + x
		copy(plane[dst:dst+4], block[src:src+4])
	}
}

func elossyCopyBlock4(plane []uint8, stride, x, y int) [16]uint8 {
	var block [16]uint8
	for row := 0; row < 4; row++ {
		src := (y+row)*stride + x
		copy(block[row*4:row*4+4], plane[src:src+4])
	}
	return block
}

func elossyRestoreBlock4(plane []uint8, stride, x, y int, block *[16]uint8) {
	for row := 0; row < 4; row++ {
		dst := (y+row)*stride + x
		copy(plane[dst:dst+4], block[row*4:row*4+4])
	}
}

func elossyCopyBlock4FromBuffer(buffer []uint8, stride, x, y int) [16]uint8 {
	var block [16]uint8
	for row := 0; row < 4; row++ {
		src := (y+row)*stride + x
		copy(block[row*4:row*4+4], buffer[src:src+4])
	}
	return block
}

func elossyCopyBlock16(plane []uint8, stride, x, y int) [256]uint8 {
	var block [256]uint8
	for row := 0; row < 16; row++ {
		src := (y+row)*stride + x
		copy(block[row*16:row*16+16], plane[src:src+16])
	}
	return block
}

func elossyRestoreBlock16(plane []uint8, stride, x, y int, block *[256]uint8) {
	for row := 0; row < 16; row++ {
		dst := (y+row)*stride + x
		copy(plane[dst:dst+16], block[row*16:row*16+16])
	}
}

// mul1 applies the first scaled multiply used by the VP8 transform.
func elossyMul1(value int32) int32 {
	return ((value * elossyVp8TransformAc3C1) >> 16) + value
}

// mul2 applies the second scaled multiply used by the VP8 transform.
func elossyMul2(value int32) int32 {
	return (value * elossyVp8TransformAc3C2) >> 16
}

func elossyDcPredictValue(plane []uint8, stride, x, y, size int) uint8 {
	hasTop := y > 0
	hasLeft := x > 0
	tz := uint(bits.TrailingZeros(uint(size)))
	switch {
	case hasTop && hasLeft:
		topRow := (y - 1) * stride
		var sumTop uint32
		for i := 0; i < size; i++ {
			sumTop += uint32(plane[topRow+x+i])
		}
		var sumLeft uint32
		for i := 0; i < size; i++ {
			sumLeft += uint32(plane[(y+i)*stride+x-1])
		}
		return uint8((sumTop + sumLeft + uint32(size)) >> (tz + 1))
	case hasTop && !hasLeft:
		topRow := (y - 1) * stride
		var sumTop uint32
		for i := 0; i < size; i++ {
			sumTop += uint32(plane[topRow+x+i])
		}
		return uint8((sumTop + (uint32(size) >> 1)) >> tz)
	case !hasTop && hasLeft:
		var sumLeft uint32
		for i := 0; i < size; i++ {
			sumLeft += uint32(plane[(y+i)*stride+x-1])
		}
		return uint8((sumLeft + (uint32(size) >> 1)) >> tz)
	default:
		return 128
	}
}

func elossyAddTransformGo(plane []uint8, stride, x, y int, coeffs *[16]int16) {
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
		c := elossyMul2(int32(coeffs[4+i])) - elossyMul1(int32(coeffs[12+i]))
		d := elossyMul1(int32(coeffs[4+i])) + elossyMul2(int32(coeffs[12+i]))
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
		c := elossyMul2(tmp[4+row]) - elossyMul1(tmp[12+row])
		d := elossyMul1(tmp[4+row]) + elossyMul2(tmp[12+row])
		offset := (y+row)*stride + x
		out := plane[offset : offset+4 : offset+4]
		out[0] = elossyClipByte(int32(out[0]) + ((a + d) >> 3))
		out[1] = elossyClipByte(int32(out[1]) + ((b + c) >> 3))
		out[2] = elossyClipByte(int32(out[2]) + ((b - c) >> 3))
		out[3] = elossyClipByte(int32(out[3]) + ((a - d) >> 3))
	}
}

func elossyForwardTransformAtGo(src []uint8, srcStride, srcX, srcY int, pred []uint8, predStride, predX, predY int) [16]int16 {
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
		a0 := d0 + d3
		a1 := d1 + d2
		a2 := d1 - d2
		a3 := d0 - d3
		tmp[row*4] = (a0 + a1) * 8
		tmp[row*4+1] = (a2*2217 + a3*5352 + 1812) >> 9
		tmp[row*4+2] = (a0 - a1) * 8
		tmp[row*4+3] = (a3*2217 - a2*5352 + 937) >> 9
	}

	var out [16]int16
	for i := 0; i < 4; i++ {
		a0 := tmp[i] + tmp[12+i]
		a1 := tmp[4+i] + tmp[8+i]
		a2 := tmp[4+i] - tmp[8+i]
		a3 := tmp[i] - tmp[12+i]
		out[i] = int16((a0 + a1 + 7) >> 4)
		nonZero := int32(0)
		if a3 != 0 {
			nonZero = 1
		}
		out[4+i] = int16(((a2*2217 + a3*5352 + 12000) >> 16) + nonZero)
		out[8+i] = int16((a0 - a1 + 7) >> 4)
		out[12+i] = int16((a3*2217 - a2*5352 + 51000) >> 16)
	}
	return out
}

func elossyForwardTransform(src []uint8, srcStride int, pred []uint8, predStride, x, y int) [16]int16 {
	return elossyForwardTransformAt(src, srcStride, x, y, pred, predStride, x, y)
}

func elossyForwardWht(input *[16]int16) [16]int16 {
	var tmp [16]int32
	for row := 0; row < 4; row++ {
		base := row * 4
		a0 := int32(input[base]) + int32(input[base+2])
		a1 := int32(input[base+1]) + int32(input[base+3])
		a2 := int32(input[base+1]) - int32(input[base+3])
		a3 := int32(input[base]) - int32(input[base+2])
		tmp[base] = a0 + a1
		tmp[base+1] = a3 + a2
		tmp[base+2] = a3 - a2
		tmp[base+3] = a0 - a1
	}

	var out [16]int16
	for i := 0; i < 4; i++ {
		a0 := tmp[i] + tmp[8+i]
		a1 := tmp[4+i] + tmp[12+i]
		a2 := tmp[4+i] - tmp[12+i]
		a3 := tmp[i] - tmp[8+i]
		b0 := a0 + a1
		b1 := a3 + a2
		b2 := a3 - a2
		b3 := a0 - a1
		out[i] = int16(b0 >> 1)
		out[4+i] = int16(b1 >> 1)
		out[8+i] = int16(b2 >> 1)
		out[12+i] = int16(b3 >> 1)
	}
	return out
}

func elossyInverseWht(input *[16]int16) [16]int16 {
	var tmp [16]int32
	for i := 0; i < 4; i++ {
		a0 := int32(input[i]) + int32(input[12+i])
		a1 := int32(input[4+i]) + int32(input[8+i])
		a2 := int32(input[4+i]) - int32(input[8+i])
		a3 := int32(input[i]) - int32(input[12+i])
		tmp[i] = a0 + a1
		tmp[8+i] = a0 - a1
		tmp[4+i] = a3 + a2
		tmp[12+i] = a3 - a2
	}

	var out [16]int16
	for row := 0; row < 4; row++ {
		base := row * 4
		dc := tmp[base] + 3
		a0 := dc + tmp[base+3]
		a1 := tmp[base+1] + tmp[base+2]
		a2 := tmp[base+1] - tmp[base+2]
		a3 := dc - tmp[base+3]
		out[base] = int16((a0 + a1) >> 3)
		out[base+1] = int16((a3 + a2) >> 3)
		out[base+2] = int16((a0 - a1) >> 3)
		out[base+3] = int16((a3 - a2) >> 3)
	}
	return out
}

// quantize_coefficient returns (level, dequantized).
func elossyQuantizeCoefficient(coeff int16, quant uint16) (int16, int16) {
	if quant == 0 {
		return 0, 0
	}
	sign := int32(1)
	if coeff < 0 {
		sign = -1
	}
	abs := int32(coeff)
	if abs < 0 {
		abs = -abs
	}
	q := int32(quant)
	level := (abs + (q >> 1)) / q
	if level > 2047 {
		level = 2047
	}
	level = sign * level
	return int16(level), int16(level * q)
}

// elossyQuantizeBlockGo returns the quantized levels only. Callers dequantize
// separately, since the levels the trellis settles on are not the ones plain
// rounding produces.
func elossyQuantizeBlockGo(coeffs *[16]int16, dcQuant, acQuant uint16, first int) [16]int16 {
	var levels [16]int16
	for index := first; index < 16; index++ {
		quant := acQuant
		if index == 0 {
			quant = dcQuant
		}
		levels[index], _ = elossyQuantizeCoefficient(coeffs[index], quant)
	}
	return levels
}

func elossyDequantizeLevels(levels *[16]int16, dcQuant, acQuant uint16) [16]int16 {
	var dequantized [16]int16
	for index := 0; index < 16; index++ {
		quant := int32(acQuant)
		if index == 0 {
			quant = int32(dcQuant)
		}
		dequantized[index] = int16(int32(levels[index]) * quant)
	}
	return dequantized
}

func elossyBlockSse4x4Go(source []uint8, stride, x, y int, candidate *[16]uint8) uint64 {
	var sse uint64
	for row := 0; row < 4; row++ {
		srcOffset := (y+row)*stride + x
		srcRow := source[srcOffset : srcOffset+4 : srcOffset+4]
		candRow := candidate[row*4 : row*4+4]
		for col := 0; col < 4; col++ {
			diff := int32(srcRow[col]) - int32(candRow[col])
			sse += uint64(diff * diff)
		}
	}
	return sse
}

func elossyReconstructLuma16FromPrediction(prediction *[256]uint8, acCoeffs *[16][16]int16, y2Coeffs *[16]int16) ([256]uint8, [16]int16) {
	candidate := *prediction
	y2Dc := elossyInverseWht(y2Coeffs)
	for block := 0; block < 16; block++ {
		coeffs := acCoeffs[block]
		coeffs[0] = y2Dc[block]
		subX := (block & 3) * 4
		subY := (block >> 2) * 4
		elossyAddTransform(candidate[:], 16, subX, subY, &coeffs)
	}
	return candidate, y2Dc
}

// elossyQuantizeLevels turns raw transform coefficients into levels, through
// the trellis when the profile asks for it and by plain rounding otherwise, and
// returns the dequantized coefficients to reconstruct from.
func elossyQuantizeLevels(trellis bool, coeffs *[16]int16, model *elossyRateModel, coeffType, ctx, first int, dcQuant, acQuant uint16, lambda uint32, levels *[16]int16) [16]int16 {
	if trellis {
		dequantized, _ := elossyTrellisQuantize(coeffs, model, coeffType, ctx, first, dcQuant, acQuant, lambda, levels)
		return dequantized
	}
	*levels = elossyQuantizeBlock(coeffs, dcQuant, acQuant, first)
	return elossyDequantizeLevels(levels, dcQuant, acQuant)
}

// elossyQuantizeLevelsRate also reports what coding the levels costs, which the
// mode search needs and the trellis has already worked out along the way.
func elossyQuantizeLevelsRate(trellis bool, coeffs *[16]int16, model *elossyRateModel, coeffType, ctx, first int, dcQuant, acQuant uint16, lambda uint32, levels *[16]int16) ([16]int16, uint32) {
	if trellis {
		return elossyTrellisQuantize(coeffs, model, coeffType, ctx, first, dcQuant, acQuant, lambda, levels)
	}
	*levels = elossyQuantizeBlock(coeffs, dcQuant, acQuant, first)
	return elossyDequantizeLevels(levels, dcQuant, acQuant), elossyCoefficientsRate(model, coeffType, ctx, first, levels)
}

// rd_score computes a rate-distortion score for the current candidate.
func elossyRdScore(distortion uint64, rate, lambda uint32) uint64 {
	l := lambda
	if l < 1 {
		l = 1
	}
	return distortion*256 + uint64(rate)*uint64(l)
}

func elossyI16ModeRate(mode uint8) uint32 {
	rate := elossyBitCost(true, 145)
	switch mode {
	case dcPred:
		rate += elossyBitCost(false, 156)
		rate += elossyBitCost(false, 163)
	case vPred:
		rate += elossyBitCost(false, 156)
		rate += elossyBitCost(true, 163)
	case hPred:
		rate += elossyBitCost(true, 156)
		rate += elossyBitCost(false, 128)
	case tmPred:
		rate += elossyBitCost(true, 156)
		rate += elossyBitCost(true, 128)
	default:
		panic("unsupported luma mode")
	}
	return rate
}

func elossyUvModeRate(mode uint8) uint32 {
	switch mode {
	case dcPred:
		return elossyBitCost(false, 142)
	case vPred:
		return elossyBitCost(true, 142) + elossyBitCost(false, 114)
	case hPred:
		return elossyBitCost(true, 142) + elossyBitCost(true, 114) + elossyBitCost(false, 183)
	case tmPred:
		return elossyBitCost(true, 142) + elossyBitCost(true, 114) + elossyBitCost(true, 183)
	default:
		panic("unsupported chroma mode")
	}
}

func elossyBlockSseGo(source []uint8, sourceStride, x, y int, reconstructed []uint8, reconstructedStride, width, height int) uint64 {
	var sse uint64
	for row := 0; row < height; row++ {
		srcOffset := (y+row)*sourceStride + x
		reconOffset := row * reconstructedStride
		srcRow := source[srcOffset : srcOffset+width : srcOffset+width]
		reconRow := reconstructed[reconOffset : reconOffset+width : reconOffset+width]
		for col := 0; col < width; col++ {
			diff := int32(srcRow[col]) - int32(reconRow[col])
			sse += uint64(diff * diff)
		}
	}
	return sse
}

func elossyPlaneSseRegion(source []uint8, sourceStride int, decoded []uint8, decodedStride, width, height int) uint64 {
	var sse uint64
	for row := 0; row < height; row++ {
		srcOffset := row * sourceStride
		decOffset := row * decodedStride
		for col := 0; col < width; col++ {
			diff := int32(source[srcOffset+col]) - int32(decoded[decOffset+col])
			sse += uint64(diff * diff)
		}
	}
	return sse
}

// elossyLumaTrial holds one luma mode trial's quantized levels and the
// reconstruction they produce. The mode search computes both for every
// candidate; keeping the winner's lets token emission reuse them instead of
// repeating the transform, quantization and refinement for the chosen mode.
type elossyLumaTrial struct {
	levels   [16][16]int16
	y2Levels [16]int16
	recon    [256]uint8
	subModes [16]uint8
	isI4x4   bool
}

// elossyChromaTrial is the chroma counterpart of elossyLumaTrial.
type elossyChromaTrial struct {
	uLevels [4][16]int16
	vLevels [4][16]int16
	uRecon  [64]uint8
	vRecon  [64]uint8
}

func elossyEvaluateLumaMode(trial *elossyLumaTrial, source *elossyPlanes, reconstructed *elossyPlanes, mbX, mbY int, profile *elossyLossySearchProfile, quant *elossyQuantMatrices, rd *elossyRdMultipliers, model *elossyRateModel, top *elossyNonZeroContext, left *elossyNonZeroContext, mode uint8) uint64 {
	x := mbX * 16
	y := mbY * 16
	var prediction [16 * 16]uint8
	elossyFillPredictionBlock(reconstructed.y, reconstructed.yStride, reconstructed.yStride, x, y, mode, prediction[:], 16, 16)
	trial.isI4x4 = false
	trial.recon = prediction
	candidate := trial.recon[:]
	var yDc [16]int16
	var yCoeffs [16][16]int16
	yLevels := &trial.levels
	var rate uint32
	refineTnz := top.nz & 0x0f
	refineLnz := left.nz & 0x0f

	for subY := 0; subY < 4; subY++ {
		l := refineLnz & 1
		for subX := 0; subX < 4; subX++ {
			block := subY*4 + subX
			coeffs := elossyForwardTransformAt(source.y, source.yStride, x+subX*4, y+subY*4, prediction[:], 16, subX*4, subY*4)
			yDc[block] = coeffs[0]
			acOnly := coeffs
			acOnly[0] = 0
			ctx := int(l + (refineTnz & 1))
			var levels [16]int16
			coeffsR, blockRate := elossyQuantizeLevelsRate(profile.refineI16, &acOnly, model, 0, ctx, 1, quant.y1[0], quant.y1[1], rd.trellisI16, &levels)
			rate += blockRate
			yLevels[block] = levels
			yCoeffs[block] = coeffsR
			hasAc := uint8(0)
			if elossyBlockHasNonZero(&yLevels[block], 1) {
				hasAc = 1
			}
			l = hasAc
			refineTnz = (refineTnz >> 1) | (hasAc << 7)
		}
		refineTnz >>= 4
		refineLnz = (refineLnz >> 1) | (l << 7)
	}

	y2Input := elossyForwardWht(&yDc)
	trial.y2Levels = elossyQuantizeBlock(&y2Input, quant.y2[0], quant.y2[1], 0)
	y2Coeffs := elossyDequantizeLevels(&trial.y2Levels, quant.y2[0], quant.y2[1])
	rate += elossyCoefficientsRate(model, 1, int(top.nzDc+left.nzDc), 0, &trial.y2Levels)
	y2Dc := elossyInverseWht(&y2Coeffs)
	for block := 0; block < 16; block++ {
		yCoeffs[block][0] = y2Dc[block]
	}

	for subY := 0; subY < 4; subY++ {
		for subX := 0; subX < 4; subX++ {
			block := subY*4 + subX
			elossyAddTransform(candidate, 16, subX*4, subY*4, &yCoeffs[block])
		}
	}

	distortion := elossyBlockSse(source.y, source.yStride, x, y, candidate, 16, 16, 16)
	rdMode := rd.mode
	if rdMode < 1 {
		rdMode = 1
	}
	return elossyRdScore(distortion, rate, rd.i16) + uint64(elossyI16ModeRate(mode))*uint64(rdMode)
}

// limit is the score the whole-macroblock modes already achieved. The i4x4
// score only accumulates, so once the sub-blocks decided so far exceed it the
// remaining ones cannot change the outcome and the search stops.
func elossyEvaluateLuma4Mode(trial *elossyLumaTrial, source *elossyPlanes, reconstructed *elossyPlanes, mbX, mbY int, profile *elossyLossySearchProfile, quant *elossyQuantMatrices, rd *elossyRdMultipliers, model *elossyRateModel, topContext *elossyNonZeroContext, leftContext *elossyNonZeroContext, topModes []uint8, leftModes *[4]uint8, limit uint64) (uint64, [16]uint8) {
	modes := [numBModes]uint8{
		bDCPred, bTMPred, bVEPred, bHEPred, bRDPred, bVRPred, bLDPred, bVLPred,
		bHDPred, bHUPred,
	}

	x := mbX * 16
	y := mbY * 16
	backup := elossyCopyBlock16(reconstructed.y, reconstructed.yStride, x, y)
	var totalScore uint64
	var subModes [16]uint8
	var localTop [4]uint8
	copy(localTop[:], topModes)
	localLeft := *leftModes
	tnz := topContext.nz & 0x0f
	lnz := leftContext.nz & 0x0f

	for subY := 0; subY < 4; subY++ {
		leftMode := localLeft[subY]
		l := lnz & 1
		for subX := 0; subX < 4; subX++ {
			block := subY*4 + subX
			blockX := x + subX*4
			blockY := y + subY*4
			topMode := localTop[subX]
			neighbors := elossyGatherLuma4Neighbors(reconstructed.y, reconstructed.yStride, reconstructed.yStride, blockX, blockY)
			ctx := int(l + (tnz & 1))

			bestMode := uint8(bDCPred)
			var bestRecon [16]uint8
			var bestLevels [16]int16
			bestScore := uint64(0xffffffffffffffff)
			bestNonZero := uint8(0)
			// The trials work in a contiguous 4x4 buffer rather than in the
			// reconstruction plane: only the winner is written back, which
			// keeps nine of the ten trials off the strided plane entirely.
			for _, mode := range modes {
				var predictionBlock [16]uint8
				elossyFillLuma4PredictionFrom(&neighbors, mode, predictionBlock[:], 4)
				coeffs := elossyForwardTransformAt(source.y, source.yStride, blockX, blockY, predictionBlock[:], 4, 0, 0)
				var levels [16]int16
				dequantized, coeffRate := elossyQuantizeLevelsRate(profile.refineI4Search, &coeffs, model, 3, ctx, 0, quant.y1[0], quant.y1[1], rd.trellisI4, &levels)
				candidate := predictionBlock
				elossyAddTransform(candidate[:], 4, 0, 0, &dequantized)
				distortion := elossyBlockSse4x4(source.y, source.yStride, blockX, blockY, &candidate)
				rdMode := rd.mode
				if rdMode < 1 {
					rdMode = 1
				}
				score := elossyRdScore(distortion, coeffRate, rd.i4) + uint64(elossyIntra4ModeRate(topMode, leftMode, mode))*uint64(rdMode)
				if score < bestScore {
					bestMode = mode
					bestRecon = candidate
					bestLevels = levels
					bestScore = score
					bestNonZero = 0
					if elossyBlockHasNonZero(&levels, 0) {
						bestNonZero = 1
					}
				}
			}

			elossyRestoreBlock4(reconstructed.y, reconstructed.yStride, blockX, blockY, &bestRecon)

			subModes[block] = bestMode
			trial.levels[block] = bestLevels
			totalScore += bestScore
			localTop[subX] = bestMode
			leftMode = bestMode
			l = bestNonZero
			tnz = (tnz >> 1) | (bestNonZero << 7)

			if totalScore >= limit {
				elossyRestoreBlock16(reconstructed.y, reconstructed.yStride, x, y, &backup)
				return math.MaxUint64, subModes
			}
		}
		tnz >>= 4
		lnz = (lnz >> 1) | (l << 7)
		localLeft[subY] = leftMode
	}

	trial.isI4x4 = true
	trial.subModes = subModes
	trial.y2Levels = [16]int16{}
	trial.recon = elossyCopyBlock16(reconstructed.y, reconstructed.yStride, x, y)

	elossyRestoreBlock16(reconstructed.y, reconstructed.yStride, x, y, &backup)
	rdMode := rd.mode
	if rdMode < 1 {
		rdMode = 1
	}
	return totalScore + uint64(elossyBitCost(false, 145))*uint64(rdMode), subModes
}

func elossyEvaluateChromaMode(trial *elossyChromaTrial, source *elossyPlanes, reconstructed *elossyPlanes, mbX, mbY int, profile *elossyLossySearchProfile, quant *elossyQuantMatrices, rd *elossyRdMultipliers, model *elossyRateModel, top *elossyNonZeroContext, left *elossyNonZeroContext, mode uint8) uint64 {
	x := mbX * 8
	y := mbY * 8
	var predictionU [8 * 8]uint8
	var predictionV [8 * 8]uint8
	elossyFillPredictionBlock(reconstructed.u, reconstructed.uvStride, reconstructed.uvStride, x, y, mode, predictionU[:], 8, 8)
	elossyFillPredictionBlock(reconstructed.v, reconstructed.uvStride, reconstructed.uvStride, x, y, mode, predictionV[:], 8, 8)
	trial.uRecon = predictionU
	trial.vRecon = predictionV
	candidateU := trial.uRecon[:]
	candidateV := trial.vRecon[:]
	var rate uint32
	tnzU := top.nz >> 4
	lnzU := left.nz >> 4

	for subY := 0; subY < 2; subY++ {
		l := lnzU & 1
		for subX := 0; subX < 2; subX++ {
			block := subY*2 + subX
			coeffsU := elossyForwardTransformAt(source.u, source.uvStride, x+subX*4, y+subY*4, predictionU[:], 8, subX*4, subY*4)
			ctx := int(l + (tnzU & 1))
			var levelsU [16]int16
			coeffsUR, blockRate := elossyQuantizeLevelsRate(profile.refineChroma, &coeffsU, model, 2, ctx, 0, quant.uv[0], quant.uv[1], rd.trellisUv, &levelsU)
			trial.uLevels[block] = levelsU
			hasCoeffs := uint8(0)
			if elossyBlockHasNonZero(&levelsU, 0) {
				hasCoeffs = 1
			}
			rate += blockRate
			l = hasCoeffs
			tnzU = (tnzU >> 1) | (hasCoeffs << 3)
			elossyAddTransform(candidateU, 8, subX*4, subY*4, &coeffsUR)
		}
		tnzU >>= 2
		lnzU = (lnzU >> 1) | (l << 5)
	}

	tnzV := top.nz >> 6
	lnzV := left.nz >> 6
	for subY := 0; subY < 2; subY++ {
		l := lnzV & 1
		for subX := 0; subX < 2; subX++ {
			block := subY*2 + subX
			coeffsV := elossyForwardTransformAt(source.v, source.uvStride, x+subX*4, y+subY*4, predictionV[:], 8, subX*4, subY*4)
			ctx := int(l + (tnzV & 1))
			var levelsV [16]int16
			coeffsVR, blockRate := elossyQuantizeLevelsRate(profile.refineChroma, &coeffsV, model, 2, ctx, 0, quant.uv[0], quant.uv[1], rd.trellisUv, &levelsV)
			trial.vLevels[block] = levelsV
			hasCoeffs := uint8(0)
			if elossyBlockHasNonZero(&levelsV, 0) {
				hasCoeffs = 1
			}
			rate += blockRate
			l = hasCoeffs
			tnzV = (tnzV >> 1) | (hasCoeffs << 3)
			elossyAddTransform(candidateV, 8, subX*4, subY*4, &coeffsVR)
		}
		tnzV >>= 2
		lnzV = (lnzV >> 1) | (l << 5)
	}

	distortionU := elossyBlockSse(source.u, source.uvStride, x, y, candidateU, 8, 8, 8)
	distortionV := elossyBlockSse(source.v, source.uvStride, x, y, candidateV, 8, 8, 8)
	rdMode := rd.mode
	if rdMode < 1 {
		rdMode = 1
	}
	return elossyRdScore(distortionU+distortionV, rate, rd.uv) + uint64(elossyUvModeRate(mode))*uint64(rdMode)
}

func elossyFastLumaPredictorScore(source *elossyPlanes, reconstructed *elossyPlanes, mbX, mbY int, mode uint8) uint64 {
	x := mbX * 16
	y := mbY * 16
	var prediction [16 * 16]uint8
	elossyFillPredictionBlock(reconstructed.y, reconstructed.yStride, reconstructed.yStride, x, y, mode, prediction[:], 16, 16)
	return elossyBlockSse(source.y, source.yStride, x, y, prediction[:], 16, 16, 16)
}

func elossyFastChromaPredictorScore(source *elossyPlanes, reconstructed *elossyPlanes, mbX, mbY int, mode uint8) uint64 {
	x := mbX * 8
	y := mbY * 8
	var predictionU [8 * 8]uint8
	var predictionV [8 * 8]uint8
	elossyFillPredictionBlock(reconstructed.u, reconstructed.uvStride, reconstructed.uvStride, x, y, mode, predictionU[:], 8, 8)
	elossyFillPredictionBlock(reconstructed.v, reconstructed.uvStride, reconstructed.uvStride, x, y, mode, predictionV[:], 8, 8)
	return elossyBlockSse(source.u, source.uvStride, x, y, predictionU[:], 8, 8, 8) +
		elossyBlockSse(source.v, source.uvStride, x, y, predictionV[:], 8, 8, 8)
}

// elossyMbTrials is the mode search's scratch space. Each of luma and chroma
// gets two buffers so the winning trial can be kept by swapping pointers
// instead of copying, and bestLuma/bestChroma point at the survivors once the
// search finishes.
type elossyMbTrials struct {
	luma       [2]elossyLumaTrial
	chroma     [2]elossyChromaTrial
	bestLuma   *elossyLumaTrial
	bestChroma *elossyChromaTrial
	valid      bool
}

func elossyChooseMacroblockMode(trials *elossyMbTrials, source *elossyPlanes, reconstructed *elossyPlanes, mbX, mbY int, profile *elossyLossySearchProfile, quant *elossyQuantMatrices, rd *elossyRdMultipliers, model *elossyRateModel, topContext *elossyNonZeroContext, leftContext *elossyNonZeroContext, topModes []uint8, leftModes *[4]uint8) elossyMacroblockMode {
	modes := [4]uint8{dcPred, vPred, hPred, tmPred}
	trials.valid = false

	if profile.fastModeSearch {
		bestLuma := uint8(dcPred)
		bestLumaScore := uint64(0xffffffffffffffff)
		for _, mode := range modes {
			score := elossyFastLumaPredictorScore(source, reconstructed, mbX, mbY, mode)
			if score < bestLumaScore {
				bestLuma = mode
				bestLumaScore = score
			}
		}

		bestChroma := uint8(dcPred)
		bestChromaScore := uint64(0xffffffffffffffff)
		for _, mode := range modes {
			score := elossyFastChromaPredictorScore(source, reconstructed, mbX, mbY, mode)
			if score < bestChromaScore {
				bestChroma = mode
				bestChromaScore = score
			}
		}

		return elossyMacroblockMode{
			luma:    bestLuma,
			chroma:  bestChroma,
			segment: 0,
			skip:    false,
		}
	}

	keptLuma, spareLuma := &trials.luma[0], &trials.luma[1]
	bestLuma := uint8(dcPred)
	bestLumaScore := uint64(0xffffffffffffffff)
	for _, mode := range modes {
		score := elossyEvaluateLumaMode(spareLuma, source, reconstructed, mbX, mbY, profile, quant, rd, model, topContext, leftContext, mode)
		if score < bestLumaScore {
			bestLuma = mode
			bestLumaScore = score
			keptLuma, spareLuma = spareLuma, keptLuma
		}
	}

	var subLuma [16]uint8
	if profile.allowI4x4 {
		i4Score, i4SubLuma := elossyEvaluateLuma4Mode(spareLuma, source, reconstructed, mbX, mbY, profile, quant, rd, model, topContext, leftContext, topModes, leftModes, bestLumaScore)
		if i4Score < bestLumaScore {
			bestLuma = bPred
			subLuma = i4SubLuma
			keptLuma, spareLuma = spareLuma, keptLuma
		} else {
			for idx := range subLuma {
				subLuma[idx] = bDCPred
			}
		}
	} else {
		for idx := range subLuma {
			subLuma[idx] = bDCPred
		}
	}

	keptChroma, spareChroma := &trials.chroma[0], &trials.chroma[1]
	bestChroma := uint8(dcPred)
	bestChromaScore := uint64(0xffffffffffffffff)
	for _, mode := range modes {
		score := elossyEvaluateChromaMode(spareChroma, source, reconstructed, mbX, mbY, profile, quant, rd, model, topContext, leftContext, mode)
		if score < bestChromaScore {
			bestChroma = mode
			bestChromaScore = score
			keptChroma, spareChroma = spareChroma, keptChroma
		}
	}

	trials.bestLuma = keptLuma
	trials.bestChroma = keptChroma
	// The i4x4 search and the final emission may refine levels differently; only
	// then does the chosen mode have to be re-encoded from scratch.
	trials.valid = bestLuma != bPred || profile.refineI4Search == profile.refineI4Final

	return elossyMacroblockMode{
		luma:    bestLuma,
		subLuma: subLuma,
		chroma:  bestChroma,
		segment: 0,
		skip:    false,
	}
}
