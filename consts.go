package webp

// VP8/WebP codec constants. Names mirror the upstream Rust implementation to
// keep the port easy to cross-reference against libwebp and webp-rust.

const (
	mbFeatureTreeProbs = 3
	numMbSegments      = 4
	numRefLfDeltas     = 4
	numModeLfDeltas    = 4
	maxNumPartitions   = 8
	numTypes           = 4
	numBands           = 8
	numCtx             = 3
	numProbas          = 11
	numBModes          = 10
)

const (
	bDCPred uint8 = 0
	bTMPred uint8 = 1
	bVEPred uint8 = 2
	bHEPred uint8 = 3
	bRDPred uint8 = 4
	bVRPred uint8 = 5
	bLDPred uint8 = 6
	bVLPred uint8 = 7
	bHDPred uint8 = 8
	bHUPred uint8 = 9

	dcPred uint8 = bDCPred
	tmPred uint8 = bTMPred
	vPred  uint8 = bVEPred
	hPred  uint8 = bHEPred
	bPred  uint8 = numBModes
)

const (
	tagSize             = 4
	chunkHeaderSize     = 8
	riffHeaderSize      = 12
	vp8FrameHeaderSize  = 10
	vp8lFrameHeaderSize = 5
	vp8xChunkSize       = 10
	maxChunkPayload     = int(^uint32(0)) - chunkHeaderSize - 1
	maxImageArea        = uint64(1) << 32
)

const (
	animationFlag uint32 = 0x0000_0002
	alphaFlag     uint32 = 0x0000_0010
)

// WebpFormat identifies the primary still-image codec of a WebP container.
type Format int

const (
	FormatUndefined Format = 0
	FormatLossy     Format = 1
	FormatLossless  Format = 2
)
