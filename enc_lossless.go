package webp

// Shared state, constants, and search profiles for the lossless VP8L encoder.
// Ported from src/encoder/lossless/mod.rs.

const (
	elosslessMaxWebpDimension                     = 1 << 14
	elosslessMaxCacheBits                         = 11
	elosslessMinLength                            = 4
	elosslessMaxLength                            = 4096
	elosslessMinTransformBits                     = 2
	elosslessGlobalCrossColorTransformBits        = 9
	elosslessGlobalPredictorTransformBits         = 9
	elosslessGlobalPredictorMode           uint8  = 11
	elosslessCrossColorTransformBits              = 5
	elosslessPredictorTransformBits               = 5
	elosslessMaxOptimizationLevel          uint8  = 9
	elosslessDefaultOptimizationLevel      uint8  = 6
	elosslessNumPredictorModes             uint8  = 14
	elosslessNumLiteralCodes                      = 256
	elosslessNumLengthCodes                       = 24
	elosslessNumDistanceCodes                     = 40
	elosslessNumCodeLengthCodes                   = 19
	elosslessNumHistogramPartitions               = 4
	elosslessMinHuffmanBits                       = 2
	elosslessNumHuffmanBits                       = 3
	elosslessColorCacheHashMul             uint32 = 0x1e35_a7bd
	elosslessMatchHashBits                        = 15
	elosslessMatchHashSize                        = 1 << elosslessMatchHashBits
	elosslessMatchChainDepthLevel1                = 4
	elosslessMatchChainDepthLevel2                = 8
	elosslessMatchChainDepthLevel3                = 16
	elosslessMatchChainDepthLevel4                = 32
	elosslessMatchChainDepthLevel5                = 64
	elosslessMatchChainDepthLevel6                = 128
	elosslessMatchChainDepthLevel7                = 192
	elosslessMaxFallbackDistance                  = (1 << 20) - 120
	elosslessApproxLiteralCostBits                = 32
	elosslessApproxCacheCostBits                  = 8
	elosslessApproxCopyLengthSymbolBits           = 8
	elosslessApproxCopyDistanceSymbolBits         = 8

	elosslessIntMax = int(^uint(0) >> 1)
)

var elosslessCodeLengthCodeOrder = [elosslessNumCodeLengthCodes]int{
	17, 18, 0, 1, 2, 3, 4, 5, 16, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15,
}

var elosslessPlaneToCodeLut = [128]uint8{
	96, 73, 55, 39, 23, 13, 5, 1, 255, 255, 255, 255, 255, 255, 255, 255, 101, 78, 58, 42, 26, 16,
	8, 2, 0, 3, 9, 17, 27, 43, 59, 79, 102, 86, 62, 46, 32, 20, 10, 6, 4, 7, 11, 21, 33, 47, 63,
	87, 105, 90, 70, 52, 37, 28, 18, 14, 12, 15, 19, 29, 38, 53, 71, 91, 110, 99, 82, 66, 48, 35,
	30, 24, 22, 25, 31, 36, 49, 67, 83, 100, 115, 108, 94, 76, 64, 50, 44, 40, 34, 41, 45, 51, 65,
	77, 95, 109, 118, 113, 103, 92, 80, 68, 60, 56, 54, 57, 61, 69, 81, 93, 104, 114, 119, 116,
	111, 106, 97, 88, 84, 74, 72, 75, 85, 89, 98, 107, 112, 117,
}

const (
	elosslessTokLiteral = iota
	elosslessTokCache
	elosslessTokCopy
)

type elosslessToken struct {
	kind     int
	argb     uint32
	key      int
	distance int
	length   int
}

type elosslessPrefixCode struct {
	symbol     int
	extraBits  int
	extraValue int
}

type elosslessCrossColorTransform struct {
	greenToRed  int8
	greenToBlue int8
	redToBlue   int8
}

type elosslessColorCache struct {
	colors    []uint32
	hashShift uint32
}

type elosslessTransformPlan struct {
	useSubtractGreen bool
	crossBits        int
	crossBitsSet     bool
	crossWidth       int
	crossImage       []uint32
	predictorBits    int
	predictorBitsSet bool
	predictorWidth   int
	predictorImage   []uint32
	predicted        []uint32
}

type elosslessPaletteCandidate struct {
	palette       []uint32
	packedWidth   int
	packedIndices []uint32
}

type elosslessTokenBuildOptions struct {
	colorCacheBits         int
	matchChainDepth        int
	useWindowOffsets       bool
	windowOffsetLimit      int
	lazyMatching           bool
	useTraceback           bool
	tracebackMaxCandidates int
}

const (
	elosslessStepNone = iota
	elosslessStepLiteral
	elosslessStepCache
	elosslessStepCopy
)

type elosslessTracebackStep struct {
	kind     int
	key      int
	distance int
	length   int
}

type elosslessTracebackCostModel struct {
	literal             []int
	red                 []int
	blue                []int
	alpha               []int
	distance            []int
	lengthCostIntervals [][3]int
}

// elosslessHistogramSet mirrors the Rust [Vec<u32>; 5].
type elosslessHistogramSet [5][]uint32

type elosslessHuffmanGroupCodes struct {
	green elosslessHuffmanCode
	red   elosslessHuffmanCode
	blue  elosslessHuffmanCode
	alpha elosslessHuffmanCode
	dist  elosslessHuffmanCode
}

type elosslessMetaHuffmanPlan struct {
	huffmanBits  int
	huffmanXsize int
	assignments  []int
	groups       []elosslessHuffmanGroupCodes
}

type elosslessHistogramCandidate struct {
	histograms elosslessHistogramSet
	weight     int
}

type elosslessLosslessSearchProfile struct {
	transformSearchLevel  uint8
	matchSearchLevel      uint8
	entropySearchLevel    uint8
	useColorCache         bool
	shortlistKeep         int
	earlyStopRatioPercent int
}

func elosslessDefaultOptions() LosslessOptions {
	return LosslessOptions{Effort: elosslessDefaultOptimizationLevel}
}

func elosslessColorCacheNew(hashBits int) (elosslessColorCache, error) {
	if hashBits < 1 || hashBits > elosslessMaxCacheBits {
		return elosslessColorCache{}, encInvalidParam("invalid VP8L color cache size")
	}
	size := 1 << hashBits
	return elosslessColorCache{
		colors:    make([]uint32, size),
		hashShift: uint32(32 - hashBits),
	}, nil
}

func (c *elosslessColorCache) key(argb uint32) int {
	return int((argb * elosslessColorCacheHashMul) >> c.hashShift)
}

func (c *elosslessColorCache) lookup(argb uint32) (int, bool) {
	key := c.key(argb)
	if c.colors[key] == argb {
		return key, true
	}
	return 0, false
}

func (c *elosslessColorCache) insert(argb uint32) {
	c.colors[c.key(argb)] = argb
}

func elosslessValidateRgba(width, height int, rgba []byte) error {
	if width == 0 || height == 0 {
		return encInvalidParam("image dimensions must be non-zero")
	}
	if width > elosslessMaxWebpDimension || height > elosslessMaxWebpDimension {
		return encInvalidParam("image dimensions exceed VP8L limits")
	}

	expectedLen := width * height * 4
	if len(rgba) != expectedLen {
		return encInvalidParam("RGBA buffer length does not match dimensions")
	}
	return nil
}

func elosslessValidateOptions(options *LosslessOptions) error {
	if options.Effort > elosslessMaxOptimizationLevel {
		return encInvalidParam("lossless optimization level must be in 0..=9")
	}
	return nil
}

func elosslessSearchProfile(optimizationLevel uint8) elosslessLosslessSearchProfile {
	switch optimizationLevel {
	case 0:
		return elosslessLosslessSearchProfile{0, 0, 0, false, 1, 100}
	case 1:
		return elosslessLosslessSearchProfile{1, 1, 0, false, 2, 104}
	case 2:
		return elosslessLosslessSearchProfile{2, 2, 1, true, 2, 106}
	case 3:
		return elosslessLosslessSearchProfile{3, 2, 1, true, 3, 108}
	case 4:
		return elosslessLosslessSearchProfile{4, 3, 2, true, 3, 110}
	case 5:
		return elosslessLosslessSearchProfile{5, 4, 2, true, 4, 112}
	case 6:
		return elosslessLosslessSearchProfile{6, 4, 3, true, 4, 115}
	case 7:
		return elosslessLosslessSearchProfile{7, 5, 4, true, 5, 118}
	case 8:
		return elosslessLosslessSearchProfile{7, 6, 5, true, 6, 122}
	default:
		return elosslessLosslessSearchProfile{7, 7, 6, true, 8, 128}
	}
}

func elosslessCandidateProfiles(optimizationLevel uint8) []elosslessLosslessSearchProfile {
	switch optimizationLevel {
	case 8:
		return []elosslessLosslessSearchProfile{elosslessSearchProfile(7)}
	case 9:
		return []elosslessLosslessSearchProfile{elosslessSearchProfile(7)}
	default:
		return []elosslessLosslessSearchProfile{elosslessSearchProfile(optimizationLevel)}
	}
}

func elosslessRgbaHasAlpha(rgba []byte) bool {
	for i := 0; i+4 <= len(rgba); i += 4 {
		if rgba[i+3] != 0xff {
			return true
		}
	}
	return false
}

func elosslessRgbaToArgb(rgba []byte) []uint32 {
	out := make([]uint32, 0, len(rgba)/4)
	for i := 0; i+4 <= len(rgba); i += 4 {
		out = append(out,
			(uint32(rgba[i+3])<<24)|
				(uint32(rgba[i])<<16)|
				(uint32(rgba[i+1])<<8)|
				uint32(rgba[i+2]))
	}
	return out
}

// Small shared numeric helpers.

func elosslessIlog2(value int) int {
	// Equivalent to usize::BITS - 1 - value.leading_zeros() for value > 0.
	r := -1
	for value > 0 {
		r++
		value >>= 1
	}
	return r
}

func elosslessDivCeil(a, b int) int {
	return (a + b - 1) / b
}

func elosslessSatAdd(a, b int) int {
	if a == elosslessIntMax || b == elosslessIntMax {
		return elosslessIntMax
	}
	s := a + b
	if s < a {
		return elosslessIntMax
	}
	return s
}

func elosslessAbsI32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

func elosslessSlicesEqualU32(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func elosslessCloneHistogramSet(src *elosslessHistogramSet) elosslessHistogramSet {
	var dst elosslessHistogramSet
	for i := 0; i < 5; i++ {
		dst[i] = make([]uint32, len(src[i]))
		copy(dst[i], src[i])
	}
	return dst
}

func elosslessTokenLen(token elosslessToken) int {
	if token.kind == elosslessTokCopy {
		return token.length
	}
	return 1
}
