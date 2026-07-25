package webp

import (
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
}

type elossyLossySearchProfile struct {
	fastModeSearch      bool
	allowI4x4           bool
	refineI16           bool
	refineI4Search      bool
	refineI4Final       bool
	refineChroma        bool
	updateProbabilities bool
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
		return elossyLossySearchProfile{
			fastModeSearch: true,
		}
	case 1, 2:
		return elossyLossySearchProfile{
			updateProbabilities: true,
		}
	case 3, 4:
		return elossyLossySearchProfile{
			allowI4x4:           true,
			updateProbabilities: true,
		}
	case 5:
		return elossyLossySearchProfile{
			allowI4x4:           true,
			refineChroma:        true,
			updateProbabilities: true,
		}
	case 6:
		return elossyLossySearchProfile{
			allowI4x4:           true,
			refineI16:           true,
			refineI4Final:       true,
			refineChroma:        true,
			updateProbabilities: true,
		}
	case 7:
		return elossyLossySearchProfile{
			allowI4x4:           true,
			refineI16:           true,
			refineI4Search:      true,
			refineI4Final:       true,
			refineChroma:        true,
			updateProbabilities: true,
		}
	default:
		return elossyLossySearchProfile{
			allowI4x4:           true,
			refineI16:           true,
			refineI4Search:      true,
			refineI4Final:       true,
			refineChroma:        true,
			updateProbabilities: true,
		}
	}
}

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

	for py := 0; py < yHeight; py++ {
		srcY := py
		if srcY > height-1 {
			srcY = height - 1
		}
		for px := 0; px < yStride; px++ {
			srcX := px
			if srcX > width-1 {
				srcX = width - 1
			}
			offset := (srcY*width + srcX) * 4
			y[py*yStride+px] = elossyRgbToY(rgba[offset], rgba[offset+1], rgba[offset+2])
		}
	}

	for py := 0; py < uvHeight; py++ {
		for px := 0; px < uvStride; px++ {
			var sumR, sumG, sumB int32
			for dy := 0; dy < 2; dy++ {
				srcY := py*2 + dy
				if srcY > height-1 {
					srcY = height - 1
				}
				for dx := 0; dx < 2; dx++ {
					srcX := px*2 + dx
					if srcX > width-1 {
						srcX = width - 1
					}
					offset := (srcY*width + srcX) * 4
					sumR += int32(rgba[offset])
					sumG += int32(rgba[offset+1])
					sumB += int32(rgba[offset+2])
				}
			}
			u[py*uvStride+px] = elossyRgbToU(sumR, sumG, sumB)
			v[py*uvStride+px] = elossyRgbToV(sumR, sumG, sumB)
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

// elossyStatsMacroblockLimit is how many macroblocks the probability-convergence
// pass looks at. Its output is an aggregate over token counts, so half the frame
// pins the table down to within a percent of encoded size while halving what is
// the more expensive of the two search passes. A quarter of the frame is
// noticeably faster still but costs up to 3% on images whose top differs from
// the rest.
func elossyStatsMacroblockLimit(mbCount int) int {
	const minimum = 512
	if mbCount <= minimum {
		return mbCount
	}
	limit := mbCount / 2
	if limit < minimum {
		limit = minimum
	}
	return limit
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
