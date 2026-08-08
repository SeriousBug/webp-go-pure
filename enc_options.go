package webp

// LossyOptions configures the lossy VP8 encoder. Pass a nil *LossyOptions to
// EncodeLossy to use the defaults (quality 90, effort 0).
type LossyOptions struct {
	// Quality is the VP8 quality target in 0..=100.
	Quality uint8
	// Effort selects the encode-effort preset in 0..=9. Higher is slower and
	// produces smaller output.
	Effort uint8
	// EXIF, if non-nil, is embedded in the container as a raw EXIF metadata chunk.
	EXIF []byte
}

// LosslessOptions configures the lossless VP8L encoder. Pass a nil
// *LosslessOptions to EncodeLossless to use the default (effort 6).
type LosslessOptions struct {
	// Effort selects the encode-effort preset in 0..=6. Higher is slower and
	// produces smaller output. Values 7..=9 are accepted but behave as 6: the
	// deeper searches they used to run cost many times the encode time for
	// about 1% smaller output.
	Effort uint8
	// EXIF, if non-nil, is embedded in the container as a raw EXIF metadata chunk.
	EXIF []byte
}
