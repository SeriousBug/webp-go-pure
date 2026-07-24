package webp

// VP8/WebP codec constants. Names mirror the upstream Rust implementation to
// keep the port easy to cross-reference against libwebp and webp-rust.

const (
	MB_FEATURE_TREE_PROBS = 3
	NUM_MB_SEGMENTS       = 4
	NUM_REF_LF_DELTAS     = 4
	NUM_MODE_LF_DELTAS    = 4
	MAX_NUM_PARTITIONS    = 8
	NUM_TYPES             = 4
	NUM_BANDS             = 8
	NUM_CTX               = 3
	NUM_PROBAS            = 11
	NUM_BMODES            = 10
)

const (
	B_DC_PRED uint8 = 0
	B_TM_PRED uint8 = 1
	B_VE_PRED uint8 = 2
	B_HE_PRED uint8 = 3
	B_RD_PRED uint8 = 4
	B_VR_PRED uint8 = 5
	B_LD_PRED uint8 = 6
	B_VL_PRED uint8 = 7
	B_HD_PRED uint8 = 8
	B_HU_PRED uint8 = 9

	DC_PRED uint8 = B_DC_PRED
	TM_PRED uint8 = B_TM_PRED
	V_PRED  uint8 = B_VE_PRED
	H_PRED  uint8 = B_HE_PRED
	B_PRED  uint8 = NUM_BMODES
)

const (
	TAG_SIZE              = 4
	CHUNK_HEADER_SIZE     = 8
	RIFF_HEADER_SIZE      = 12
	VP8_FRAME_HEADER_SIZE = 10
	VP8L_FRAME_HEADER_SIZE = 5
	VP8X_CHUNK_SIZE       = 10
	MAX_CHUNK_PAYLOAD     = int(^uint32(0)) - CHUNK_HEADER_SIZE - 1
	MAX_IMAGE_AREA        = uint64(1) << 32
)

const (
	ANIMATION_FLAG uint32 = 0x0000_0002
	ALPHA_FLAG     uint32 = 0x0000_0010
)

// WebpFormat identifies the primary still-image codec of a WebP container.
type WebpFormat int

const (
	FormatUndefined WebpFormat = 0
	FormatLossy     WebpFormat = 1
	FormatLossless  WebpFormat = 2
)
