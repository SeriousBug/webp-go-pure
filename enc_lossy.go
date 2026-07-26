package webp

import (
	"encoding/binary"
	"math"
	"sort"
)

const (
	elossyMaxWebpDimension    = 1 << 14
	elossyMaxPartition0Length = (1 << 19) - 1
	elossyYuvFix              = 16
	elossyYuvHalf             = 1 << (elossyYuvFix - 1)
	elossyVp8TransformAc3C1   = 20091
	elossyVp8TransformAc3C2   = 35468

	elossyDefaultLossyOptimizationLevel = 0
	elossyMaxLossyOptimizationLevel     = 9
)

var (
	elossyCat3   = [4]uint8{173, 148, 140, 0}
	elossyCat4   = [5]uint8{176, 155, 140, 135, 0}
	elossyCat5   = [6]uint8{180, 157, 141, 134, 130, 0}
	elossyCat6   = [12]uint8{254, 254, 243, 230, 196, 177, 153, 140, 133, 130, 129, 0}
	elossyZigzag = [16]int{0, 1, 4, 8, 5, 2, 3, 6, 9, 12, 13, 10, 7, 11, 14, 15}
	elossyBands  = [17]int{0, 1, 2, 3, 6, 4, 5, 6, 6, 6, 6, 6, 6, 6, 6, 7, 0}
)

type elossyCoeffProbTables [numTypes][numBands][numCtx][numProbas]uint8
type elossyCoeffStats [numTypes][numBands][numCtx][numProbas]uint32

type elossyNonZeroContext struct {
	nz   uint8
	nzDc uint8
}

type elossyMacroblockMode struct {
	luma    uint8
	subLuma [16]uint8
	chroma  uint8
	segment uint8
	skip    bool
}

type elossyQuantMatrices struct {
	y1 [2]uint16
	y2 [2]uint16
	uv [2]uint16
}

type elossyRdMultipliers struct {
	i16  uint32
	i4   uint32
	uv   uint32
	mode uint32
	// i4Penalty is libwebp's fixed offset (1000*q^2) charged to an i4x4
	// macroblock before it is compared against the whole-block modes. It stands
	// in for what the sub-block search's own lambda leaves unpriced: the
	// sub-blocks are picked with i4, and only the total is re-priced with i16.
	i4Penalty uint64
	// The trellis weighs rate against a transform-domain error rather than the
	// pixel-domain SSE the mode search uses, so it needs its own multipliers.
	trellisI16 uint32
	trellisI4  uint32
	trellisUv  uint32
}

type elossyPlanes struct {
	yStride  int
	uvStride int
	y        []uint8
	u        []uint8
	v        []uint8
}

type elossySegmentConfig struct {
	useSegment     bool
	updateMap      bool
	quantizer      [numMbSegments]uint8
	filterStrength [numMbSegments]int8
	probs          [mbFeatureTreeProbs]uint8
	segments       []uint8
}

type elossyFilterConfig struct {
	simple    bool
	level     uint8
	sharpness uint8
}

type elossyEncodedLossyCandidate struct {
	baseQuant      uint8
	segment        elossySegmentConfig
	probabilities  elossyCoeffProbTables
	modes          []elossyMacroblockMode
	tokenPartition []byte
	// distortion is the SSE of the encoder's own reconstruction against the
	// source, before loop filtering. Candidates are compared on rate *and*
	// distortion, so a segmentation that only wins on size cannot be chosen.
	distortion uint64
	// reconstructed is the unfiltered reconstruction, kept so the filter
	// search can score levels without decoding the frame back.
	reconstructed elossyPlanes
	// buffers is the pooled storage backing modes, tokenPartition and
	// reconstructed. Releasing a candidate returns it for the next pass.
	buffers *elossyPassBuffers
}

// The refine* flags select trellis quantization for the mode the search
// settles on. Every trial is quantized plainly and only the winner, or the
// handful of candidates refineI4TopK names, goes through the trellis, which is
// what libwebp does below its top method.
type elossyLossySearchProfile struct {
	fastModeSearch bool
	allowI4x4      bool
	refineI16      bool
	refineI4       bool
	// refineI4TopK is how many of the plain-quantized 4x4 candidates are
	// re-scored through the trellis. Zero refines only the plain winner.
	refineI4TopK        int
	refineChroma        bool
	updateProbabilities bool
	// modeScreenTopK is how many modes survive the Hadamard-domain pre-screen
	// and get a full transform/quantize/reconstruct trial. Zero disables the
	// screen and trials every mode.
	modeScreenTopK int
	// i4GateStrength scales the intra4 entry gate: a macroblock skips the i4x4
	// search when its i16 distortion is at or below strength/8 * q^2. Zero
	// disables the gate, including its all-levels-zero fast path.
	i4GateStrength uint32
}

func elossyDefaultOptions() LossyOptions {
	return LossyOptions{
		Quality: 90,
		Effort:  elossyDefaultLossyOptimizationLevel,
	}
}

func elossyValidateRgba(width, height int, rgba []byte) error {
	if width == 0 || height == 0 {
		return encInvalidParam("image dimensions must be non-zero")
	}
	if width > elossyMaxWebpDimension || height > elossyMaxWebpDimension {
		return encInvalidParam("image dimensions exceed VP8 limits")
	}
	expectedLen := width * height * 4
	if len(rgba) != expectedLen {
		return encInvalidParam("RGBA buffer length does not match dimensions")
	}
	for i := 0; i+4 <= len(rgba); i += 4 {
		if rgba[i+3] != 0xff {
			return encInvalidParam("lossy encoder does not support alpha yet")
		}
	}
	return nil
}

func elossyValidateOptions(options *LossyOptions) error {
	if options.Quality > 100 {
		return encInvalidParam("lossy quality must be in 0..=100")
	}
	if options.Effort > elossyMaxLossyOptimizationLevel {
		return encInvalidParam("lossy optimization level must be in 0..=9")
	}
	return nil
}

// elossyBaseQuantizerFromQuality maps a 0..=100 quality to a VP8 base
// quantizer index. It mirrors libwebp's nonlinear quality->compression curve
// (QualityToCompression in quant_enc.c): file size scales roughly with the
// cube of the quantizer, so the mapping cube-roots a linearized quality before
// inverting to a 0..=127 index. A prior linear map over-quantized the top of
// the range (q90 landed at index 13 vs libwebp's ~9).
func elossyBaseQuantizerFromQuality(quality uint8) int32 {
	c := float64(quality) / 100.0
	var linearC float64
	if c < 0.75 {
		linearC = c * (2.0 / 3.0)
	} else {
		linearC = 2.0*c - 1.0
	}
	compression := math.Cbrt(linearC)
	q := int32(127.0 * (1.0 - compression))
	return elossyClampI32(q, 0, 127)
}

func elossyClampI32(value, lo, hi int32) int32 {
	if value < lo {
		return lo
	}
	if value > hi {
		return hi
	}
	return value
}

func elossyBuildQuantMatrices(baseQ int32) elossyQuantMatrices {
	q := int(elossyClampI32(baseQ, 0, 127))
	y2ac := (uint32(acTable[q]) * 101581) >> 16
	if y2ac < 8 {
		y2ac = 8
	}
	uvIndex := q
	if uvIndex > 117 {
		uvIndex = 117
	}
	return elossyQuantMatrices{
		y1: [2]uint16{uint16(dcTable[q]), acTable[q]},
		y2: [2]uint16{uint16(dcTable[q]) * 2, uint16(y2ac)},
		uv: [2]uint16{uint16(dcTable[uvIndex]), acTable[q]},
	}
}

func elossyBuildRdMultipliers(quant *elossyQuantMatrices) elossyRdMultipliers {
	qI4 := uint32(max(quant.y1[1], 8))
	qI16 := uint32(max(quant.y2[1], 8))
	qUv := uint32(max(quant.uv[1], 8))
	return elossyRdMultipliers{
		i16:        max(3*qI16*qI16, 128),
		i4:         max(3*qI4*qI4, 128) >> 7,
		uv:         max(3*qUv*qUv, 128) >> 6,
		mode:       max(qI4*qI4, 128) >> 7,
		trellisI16: max((qI16*qI16)>>2, 1),
		trellisI4:  max((7*qI4*qI4)>>3, 1),
		trellisUv:  max((qUv*qUv)<<1, 1),
		i4Penalty:  1000 * uint64(qI4) * uint64(qI4),
	}
}

// elossyFrameRdCost scores a whole candidate frame as distortion plus rate
// weighted by lambda, so a segmentation cannot win on size alone.
//
// Lambda uses the same 3*q^2/128 form as the block-level intra4 multiplier,
// keeping the frame trade-off consistent with the decisions the block search
// already made. It is evaluated unshifted here because the block multiplier's
// >>7 truncates to uselessly coarse values at high quality.
func elossyFrameRdCost(distortion uint64, size int, baseQuant int32) uint64 {
	quant := elossyBuildQuantMatrices(int32(elossyClippedQuantizer(baseQuant)))
	q := uint64(max(quant.y1[1], 8))
	return distortion*128 + uint64(size)*8*max(3*q*q, 128)
}

func elossyClippedQuantizer(value int32) uint8 {
	return uint8(elossyClampI32(value, 0, 127))
}

func elossyMinU8(a, b uint8) uint8 {
	if a < b {
		return a
	}
	return b
}

func elossyFilterCandidates(baseQuant int32) []elossyFilterConfig {
	levels := []uint8{
		0,
		elossyMinU8(elossyClippedQuantizer((baseQuant+1)/2), 63),
		elossyMinU8(elossyClippedQuantizer(baseQuant), 63),
		elossyMinU8(elossyClippedQuantizer((baseQuant*3+1)/2), 63),
		elossyMinU8(elossyClippedQuantizer(baseQuant*2), 63),
	}
	sort.Slice(levels, func(i, j int) bool { return levels[i] < levels[j] })
	deduped := levels[:0]
	for i, level := range levels {
		if i == 0 || level != deduped[len(deduped)-1] {
			deduped = append(deduped, level)
		}
	}
	result := make([]elossyFilterConfig, 0, len(deduped))
	for _, level := range deduped {
		result = append(result, elossyFilterConfig{simple: false, level: level, sharpness: 0})
	}
	return result
}

func elossyHeuristicFilter(baseQuant int32) elossyFilterConfig {
	var level uint8
	if baseQuant <= 10 {
		level = 0
	} else {
		level = elossyMinU8(elossyClippedQuantizer((baseQuant*3+2)/4), 63)
	}
	return elossyFilterConfig{simple: false, level: level, sharpness: 0}
}

func elossySearchProfile(optimizationLevel uint8) elossyLossySearchProfile {
	switch optimizationLevel {
	case 0:
		// The converged coefficient table is worth 20%-38% of the file here,
		// far more than anything the mode search itself can reach at this
		// effort, and the sampled prior pass costs a quarter of one encode.
		return elossyLossySearchProfile{
			fastModeSearch:      true,
			updateProbabilities: true,
		}
	case 1, 2:
		return elossyLossySearchProfile{
			updateProbabilities: true,
			modeScreenTopK:      elossyModeScreenTopK,
		}
	case 3, 4:
		return elossyLossySearchProfile{
			allowI4x4:           true,
			updateProbabilities: true,
			modeScreenTopK:      elossyModeScreenTopK,
			i4GateStrength:      elossyIntra4GateStrength,
		}
	case 5:
		return elossyLossySearchProfile{
			allowI4x4:           true,
			refineChroma:        true,
			updateProbabilities: true,
			modeScreenTopK:      elossyModeScreenTopK,
			i4GateStrength:      elossyIntra4GateStrength,
		}
	case 6, 7:
		return elossyLossySearchProfile{
			allowI4x4:           true,
			refineI16:           true,
			refineI4:            true,
			refineChroma:        true,
			updateProbabilities: true,
			modeScreenTopK:      elossyModeScreenTopK,
			i4GateStrength:      elossyIntra4GateStrength,
		}
	default:
		return elossyLossySearchProfile{
			allowI4x4:           true,
			refineI16:           true,
			refineI4:            true,
			refineI4TopK:        elossyRefineI4TopK,
			refineChroma:        true,
			updateProbabilities: true,
			modeScreenTopK:      elossyTopEffortModeScreenTopK,
			i4GateStrength:      elossyIntra4GateStrengthTopEffort,
		}
	}
}

// elossyIntra4GateStrength and elossyIntra4GateStrengthTopEffort are the
// intra4 entry gate's thresholds in eighths of q^2 of macroblock distortion.
// A macroblock's i16 distortion tops out near 8*q^2 (the quantizer's own noise
// floor over 256 pixels), so 8/8 of q^2 gates the flattest eighth of that
// range. Measured on photo and portrait sources this cuts the i4x4 search on
// 25%-90% of macroblocks depending on content while leaving PSNR within
// 0.05 dB at a 2%-16% smaller file. Higher strengths (16 and up) start losing
// PSNR faster than they save bits. The top efforts use half the threshold to
// stay closer to an exhaustive search.
const (
	elossyIntra4GateStrength          = 8
	elossyIntra4GateStrengthTopEffort = 4
)

// elossyTopEffortModeScreenTopK is how wide the top effort's Hadamard
// pre-screen is. The effort used to trial all ten 4x4 modes. Trials are cheaper
// now that they quantize plainly, but they are no longer the final word either,
// so spending them on five modes and refining the best of those lands the same
// file size for two thirds of the search.
const elossyTopEffortModeScreenTopK = 5

// elossyRefineI4TopK is how many of the plain-quantized 4x4 candidates the top
// effort re-scores through the trellis. Plain quantization prices the modes
// closely but not identically, and re-scoring the best three recovers the file
// size that scoring only the plain winner gives up, at a fraction of what
// running the trellis on every candidate costs.
const elossyRefineI4TopK = 3

// elossyMaxFullSearchCandidates caps how many segmentation candidates get the
// full rate-distortion search. The rest are eliminated by the ranking pass.
const elossyMaxFullSearchCandidates = 1

// elossyStatsProfile is the reduced search used only to converge the
// coefficient probability table. It keeps the mode search that shapes the
// token distribution and drops the level refinement, which is what makes the
// full search expensive and barely moves the resulting probabilities.
func elossyStatsProfile(profile *elossyLossySearchProfile) elossyLossySearchProfile {
	return elossyLossySearchProfile{
		fastModeSearch: profile.fastModeSearch,
		allowI4x4:      profile.allowI4x4,
		modeScreenTopK: profile.modeScreenTopK,
		i4GateStrength: profile.i4GateStrength,
	}
}

func elossyUseExhaustiveSegmentSearch(optimizationLevel uint8) bool {
	return optimizationLevel >= 9
}

func elossyUseExhaustiveFilterSearch(optimizationLevel uint8, mbCount int) bool {
	if optimizationLevel >= 9 {
		return true
	}
	if optimizationLevel >= 6 {
		return mbCount < 2048
	}
	return mbCount < 1024
}

func elossySegmentWithUniformFilter(segment *elossySegmentConfig, level uint8) elossySegmentConfig {
	filtered := *segment
	if filtered.useSegment {
		for i := range filtered.filterStrength {
			filtered.filterStrength[i] = int8(level)
		}
	}
	return filtered
}

func elossyGetProba(a, b int) uint8 {
	total := a + b
	if total == 0 {
		return 255
	}
	return uint8((255*a + total/2) / total)
}

func elossyBuildSegmentQuantizers(segment *elossySegmentConfig) [numMbSegments]elossyQuantMatrices {
	var out [numMbSegments]elossyQuantMatrices
	for index := 0; index < numMbSegments; index++ {
		out[index] = elossyBuildQuantMatrices(int32(segment.quantizer[index]))
	}
	return out
}

func elossyDisabledSegmentConfig(mbCount int, baseQuant uint8) elossySegmentConfig {
	var quantizer [numMbSegments]uint8
	for i := range quantizer {
		quantizer[i] = baseQuant
	}
	var probs [mbFeatureTreeProbs]uint8
	for i := range probs {
		probs[i] = 255
	}
	return elossySegmentConfig{
		useSegment: false,
		updateMap:  false,
		quantizer:  quantizer,
		probs:      probs,
		segments:   make([]uint8, mbCount),
	}
}

func elossyRgbToY(r, g, b uint8) uint8 {
	luma := 16839*int32(r) + 33059*int32(g) + 6420*int32(b)
	return uint8((luma + elossyYuvHalf + (16 << elossyYuvFix)) >> elossyYuvFix)
}

func elossyClipUv(value, rounding int32) uint8 {
	uv := (value + rounding + (128 << (elossyYuvFix + 2))) >> (elossyYuvFix + 2)
	return uint8(elossyClampI32(uv, 0, 255))
}

func elossyRgbToU(r, g, b int32) uint8 {
	return elossyClipUv(-9719*r-19081*g+28800*b, elossyYuvHalf<<2)
}

func elossyRgbToV(r, g, b int32) uint8 {
	return elossyClipUv(28800*r-24116*g-4684*b, elossyYuvHalf<<2)
}

func elossyRgbaToYuv420(width, height int, rgba []byte, mbWidth, mbHeight int) elossyPlanes {
	yStride := mbWidth * 16
	uvStride := mbWidth * 8
	yHeight := mbHeight * 16
	uvHeight := mbHeight * 8
	y := make([]uint8, yStride*yHeight)
	u := make([]uint8, uvStride*uvHeight)
	v := make([]uint8, uvStride*uvHeight)

	// The planes are padded out to whole macroblocks. Only the padding needs the
	// edge clamps, so the interior runs without them and the padding replicates
	// the last real column and row instead of re-deriving them per pixel.
	rowBytes := width * 4
	for py := 0; py < yHeight; py++ {
		dst := y[py*yStride : py*yStride+yStride : py*yStride+yStride]
		if py >= height {
			copy(dst, y[(height-1)*yStride:height*yStride])
			continue
		}
		src := rgba[py*rowBytes : py*rowBytes+rowBytes : py*rowBytes+rowBytes]
		offset := 0
		for px := 0; px < width; px++ {
			// One 32-bit load beats three byte loads: the alpha byte is discarded
			// but the bounds check and the address arithmetic are paid once.
			pixel := binary.LittleEndian.Uint32(src[offset : offset+4 : offset+4])
			dst[px] = elossyRgbToY(uint8(pixel), uint8(pixel>>8), uint8(pixel>>16))
			offset += 4
		}
		if width < yStride {
			last := dst[width-1]
			for px := width; px < yStride; px++ {
				dst[px] = last
			}
		}
	}

	// A chroma row past the image bottom clamps both of its source rows to the
	// last one, so every such row is identical and only the first is computed.
	uvRows := (height + 1) / 2
	for py := 0; py < uvHeight; py++ {
		dstU := u[py*uvStride : py*uvStride+uvStride : py*uvStride+uvStride]
		dstV := v[py*uvStride : py*uvStride+uvStride : py*uvStride+uvStride]
		if py > uvRows {
			copy(dstU, u[uvRows*uvStride:(uvRows+1)*uvStride])
			copy(dstV, v[uvRows*uvStride:(uvRows+1)*uvStride])
			continue
		}
		y0 := py * 2
		if y0 > height-1 {
			y0 = height - 1
		}
		y1 := py*2 + 1
		if y1 > height-1 {
			y1 = height - 1
		}
		row0 := rgba[y0*rowBytes : y0*rowBytes+rowBytes : y0*rowBytes+rowBytes]
		row1 := rgba[y1*rowBytes : y1*rowBytes+rowBytes : y1*rowBytes+rowBytes]

		// A 2x2 quad is two 64-bit loads. Masking to alternate bytes keeps each
		// channel in its own 16-bit lane, which holds the sum of four samples
		// without carrying into its neighbour, so the twelve byte loads and the
		// per-sample widening collapse into a handful of word operations.
		const evenBytes = uint64(0x00FF00FF00FF00FF)
		quads := width / 2
		offset := 0
		for px := 0; px < quads; px++ {
			word0 := binary.LittleEndian.Uint64(row0[offset : offset+8 : offset+8])
			word1 := binary.LittleEndian.Uint64(row1[offset : offset+8 : offset+8])
			rb := (word0 & evenBytes) + (word1 & evenBytes)
			ga := ((word0 >> 8) & evenBytes) + ((word1 >> 8) & evenBytes)
			sumR := int32((rb & 0xffff) + ((rb >> 32) & 0xffff))
			sumB := int32(((rb >> 16) & 0xffff) + (rb >> 48))
			sumG := int32((ga & 0xffff) + ((ga >> 32) & 0xffff))
			dstU[px] = elossyRgbToU(sumR, sumG, sumB)
			dstV[px] = elossyRgbToV(sumR, sumG, sumB)
			offset += 8
		}
		if quads < uvStride {
			// Both an odd image's last chroma column and the padding columns clamp
			// their whole 2x2 quad onto the last pixel column, so they all take the
			// same value.
			last := (width - 1) * 4
			sumR := 2 * (int32(row0[last]) + int32(row1[last]))
			sumG := 2 * (int32(row0[last+1]) + int32(row1[last+1]))
			sumB := 2 * (int32(row0[last+2]) + int32(row1[last+2]))
			padU := elossyRgbToU(sumR, sumG, sumB)
			padV := elossyRgbToV(sumR, sumG, sumB)
			for px := quads; px < uvStride; px++ {
				dstU[px] = padU
				dstV[px] = padV
			}
		}
	}

	return elossyPlanes{
		yStride:  yStride,
		uvStride: uvStride,
		y:        y,
		u:        u,
		v:        v,
	}
}

func elossyEmptyReconstructedPlanes(mbWidth, mbHeight int) elossyPlanes {
	yStride := mbWidth * 16
	uvStride := mbWidth * 8
	yHeight := mbHeight * 16
	uvHeight := mbHeight * 8
	return elossyPlanes{
		yStride:  yStride,
		uvStride: uvStride,
		y:        make([]uint8, yStride*yHeight),
		u:        make([]uint8, uvStride*uvHeight),
		v:        make([]uint8, uvStride*uvHeight),
	}
}

func elossyAbsDiffU8(a, b uint8) uint32 {
	if a > b {
		return uint32(a - b)
	}
	return uint32(b - a)
}

func elossyMacroblockActivity(source *elossyPlanes, mbX, mbY int) uint32 {
	x0 := mbX * 16
	y0 := mbY * 16
	var activity uint32

	for row := 0; row < 16; row++ {
		rowOffset := (y0+row)*source.yStride + x0
		pixels := source.y[rowOffset : rowOffset+16]
		for col := 1; col < 16; col++ {
			activity += elossyAbsDiffU8(pixels[col], pixels[col-1])
		}
		if row > 0 {
			prevOffset := (y0+row-1)*source.yStride + x0
			prev := source.y[prevOffset : prevOffset+16]
			for col := 0; col < 16; col++ {
				activity += elossyAbsDiffU8(pixels[col], prev[col])
			}
		}
	}

	return activity
}

func elossyBuildSegmentProbs(counts *[numMbSegments]int) [mbFeatureTreeProbs]uint8 {
	return [mbFeatureTreeProbs]uint8{
		elossyGetProba(counts[0]+counts[1], counts[2]+counts[3]),
		elossyGetProba(counts[0], counts[1]),
		elossyGetProba(counts[2], counts[3]),
	}
}

func elossyBuildSegmentConfig(activities, sortedActivities []uint32, flatPercent int, flatDelta, detailDelta, baseQuant int32) (elossySegmentConfig, bool) {
	if len(activities) < 8 {
		return elossySegmentConfig{}, false
	}
	flatCount := len(activities) * flatPercent / 100
	if flatCount < 1 {
		flatCount = 1
	}
	if flatCount > len(activities)-1 {
		flatCount = len(activities) - 1
	}
	threshold := sortedActivities[flatCount-1]

	segments := make([]uint8, len(activities))
	var counts [numMbSegments]int
	for index, activity := range activities {
		segment := uint8(0)
		if activity > threshold {
			segment = 1
		}
		segments[index] = segment
		counts[segment]++
	}
	if counts[0] == 0 || counts[1] == 0 {
		return elossySegmentConfig{}, false
	}

	quant0 := elossyClippedQuantizer(baseQuant + flatDelta)
	quant1 := elossyClippedQuantizer(baseQuant + detailDelta)
	if quant0 == quant1 {
		return elossySegmentConfig{}, false
	}

	probs := elossyBuildSegmentProbs(&counts)
	updateMap := false
	for _, prob := range probs {
		if prob != 255 {
			updateMap = true
			break
		}
	}
	if !updateMap {
		return elossySegmentConfig{}, false
	}

	var quantizer [numMbSegments]uint8
	for i := range quantizer {
		quantizer[i] = quant0
	}
	quantizer[1] = quant1
	return elossySegmentConfig{
		useSegment: true,
		updateMap:  updateMap,
		quantizer:  quantizer,
		probs:      probs,
		segments:   segments,
	}, true
}

func elossyBuildMultiSegmentConfig(activities, sortedActivities []uint32, percentiles []int, deltas []int32, baseQuant int32) (elossySegmentConfig, bool) {
	segmentCount := len(deltas)
	if segmentCount < 2 || segmentCount > numMbSegments || len(percentiles)+1 != segmentCount {
		return elossySegmentConfig{}, false
	}

	thresholds := make([]uint32, 0, len(percentiles))
	for _, percentile := range percentiles {
		split := len(activities) * percentile / 100
		if split < 1 {
			split = 1
		}
		if split > len(activities)-1 {
			split = len(activities) - 1
		}
		thresholds = append(thresholds, sortedActivities[split-1])
	}
	sort.Slice(thresholds, func(i, j int) bool { return thresholds[i] < thresholds[j] })

	segments := make([]uint8, len(activities))
	var counts [numMbSegments]int
	for index, activity := range activities {
		segment := 0
		for _, threshold := range thresholds {
			if activity > threshold {
				segment++
			} else {
				break
			}
		}
		segments[index] = uint8(segment)
		counts[segment]++
	}

	for i := 0; i < segmentCount; i++ {
		if counts[i] == 0 {
			return elossySegmentConfig{}, false
		}
	}

	var quantizer [numMbSegments]uint8
	for i := range quantizer {
		quantizer[i] = elossyClippedQuantizer(baseQuant)
	}
	distinct := false
	for index, delta := range deltas {
		quantizer[index] = elossyClippedQuantizer(baseQuant + delta)
		if index > 0 && quantizer[index] != quantizer[index-1] {
			distinct = true
		}
	}
	if !distinct {
		return elossySegmentConfig{}, false
	}

	probs := elossyBuildSegmentProbs(&counts)
	updateMap := false
	for _, prob := range probs {
		if prob != 255 {
			updateMap = true
			break
		}
	}
	if !updateMap {
		return elossySegmentConfig{}, false
	}

	return elossySegmentConfig{
		useSegment: true,
		updateMap:  updateMap,
		quantizer:  quantizer,
		probs:      probs,
		segments:   segments,
	}, true
}

func elossyBuildSegmentCandidates(source *elossyPlanes, mbWidth, mbHeight int, baseQuant int32, optimizationLevel uint8) []elossySegmentConfig {
	mbCount := mbWidth * mbHeight
	candidates := []elossySegmentConfig{elossyDisabledSegmentConfig(mbCount, elossyClippedQuantizer(baseQuant))}
	if mbCount < 8 || optimizationLevel == 0 {
		return candidates
	}

	activities := make([]uint32, 0, mbCount)
	for mbY := 0; mbY < mbHeight; mbY++ {
		for mbX := 0; mbX < mbWidth; mbX++ {
			activities = append(activities, elossyMacroblockActivity(source, mbX, mbY))
		}
	}
	sorted := make([]uint32, len(activities))
	copy(sorted, activities)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	if !elossyUseExhaustiveSegmentSearch(optimizationLevel) && mbCount >= 1024 {
		if config, ok := elossyBuildSegmentConfig(activities, sorted, 65, 12, -2, baseQuant); ok {
			return []elossySegmentConfig{config}
		}
		return candidates
	}

	type twoSegmentPreset struct {
		flatPercent int
		flatDelta   int32
		detailDelta int32
	}
	var twoSegmentPresets []twoSegmentPreset
	if optimizationLevel <= 2 {
		twoSegmentPresets = []twoSegmentPreset{{65, 12, -2}}
	} else if mbCount >= 2048 && !elossyUseExhaustiveSegmentSearch(optimizationLevel) {
		twoSegmentPresets = []twoSegmentPreset{{65, 12, -2}, {55, 10, 0}}
	} else {
		twoSegmentPresets = []twoSegmentPreset{{55, 10, 0}, {65, 12, -2}, {45, 8, 0}}
	}
	for _, preset := range twoSegmentPresets {
		if config, ok := elossyBuildSegmentConfig(activities, sorted, preset.flatPercent, preset.flatDelta, preset.detailDelta, baseQuant); ok {
			candidates = append(candidates, config)
		}
	}

	if optimizationLevel >= 4 && (elossyUseExhaustiveSegmentSearch(optimizationLevel) || mbCount < 2048) {
		type multiPreset struct {
			percentiles []int
			deltas      []int32
		}
		multiPresets := []multiPreset{
			{[]int{35, 72}, []int32{12, 4, -4}},
			{[]int{25, 50, 78}, []int32{16, 8, 1, -7}},
			{[]int{30, 58, 84}, []int32{18, 10, 2, -8}},
		}
		for _, preset := range multiPresets {
			if config, ok := elossyBuildMultiSegmentConfig(activities, sorted, preset.percentiles, preset.deltas, baseQuant); ok {
				candidates = append(candidates, config)
			}
		}
	}

	return candidates
}

// elossyPassCoverage selects which macroblock rows an encode pass covers: a
// prefix of the frame, or a band of rows out of every stride of them.
type elossyPassCoverage struct {
	// mbLimit caps how many macroblocks the pass encodes, rounded up to whole
	// rows. Zero covers the frame.
	mbLimit int
	// rowBand and rowStride encode rowBand rows out of every rowStride. Equal
	// values cover every row.
	rowBand   int
	rowStride int
}

func (c elossyPassCoverage) sampled() bool {
	return c.rowBand < c.rowStride
}

func elossyFullCoverage() elossyPassCoverage {
	return elossyPassCoverage{rowBand: 1, rowStride: 1}
}

func elossyPrefixCoverage(mbLimit int) elossyPassCoverage {
	return elossyPassCoverage{mbLimit: mbLimit, rowBand: 1, rowStride: 1}
}

// elossyStatsCoverage is how the probability-convergence pass samples the
// frame: every fourth macroblock row rather than a prefix of half of them. The
// table it produces is an aggregate over token counts, so what it needs is a
// sample of the whole frame, not a contiguous piece of it, and a quarter of the
// rows spread down the frame prices a frame better than half of them taken from
// the top.
//
// The pass fills the rows it skips with the source, which the sampled rows
// then predict from. Sampling more sparsely than this is what costs: at one row
// in eight the lower efforts, where nothing downstream re-prices what the table
// mis-costs, give up over 1% of size. Sampling in deeper bands at the same row
// count is worse still, so what the table wants is spread, not locality.
//
// The sample is not thinned below ~256 macroblocks, under which the counts are
// too sparse to estimate the tail of the level distribution.
func elossyStatsCoverage(mbWidth, mbHeight int) elossyPassCoverage {
	const minimumMacroblocks = 256
	const maximumStride = 4
	rows := (minimumMacroblocks + mbWidth - 1) / mbWidth
	if rows < 1 {
		rows = 1
	}
	stride := mbHeight / rows
	if stride > maximumStride {
		stride = maximumStride
	}
	if stride <= 1 {
		return elossyFullCoverage()
	}
	return elossyPassCoverage{rowBand: 1, rowStride: stride}
}

// elossyLimitedRows is how many macroblock rows a pass covers under mbLimit.
// A pass is truncated to whole rows so every macroblock it does encode still
// sees the neighbours its prediction depends on.
func elossyLimitedRows(mbWidth, mbHeight, mbLimit int) int {
	if mbLimit <= 0 || mbLimit >= mbWidth*mbHeight {
		return mbHeight
	}
	return (mbLimit + mbWidth - 1) / mbWidth
}

// elossyRankingMacroblockLimit is how many macroblocks the candidate ranking
// pass encodes. Ranking only has to order the candidates, not measure them, so
// it runs the real search over a prefix of the frame rather than a reduced
// search over all of it: the refinement the reduced search dropped is what
// separates the candidates in the first place.
func elossyRankingMacroblockLimit(mbCount int) int {
	const minimum = 512
	if mbCount <= minimum {
		return mbCount
	}
	if limit := mbCount / 8; limit > minimum {
		return limit
	}
	return minimum
}
