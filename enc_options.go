package webp

// LossyEncodingOptions tunes the lossy VP8 encoder.
type LossyEncodingOptions struct {
	// Quality is the VP8 quality target in 0..=100.
	Quality uint8
	// OptimizationLevel selects the encode-effort preset in 0..=9.
	OptimizationLevel uint8
}

// DefaultLossyEncodingOptions returns the default lossy options: quality 90 and
// optimization level 0 (fast encode), matching the upstream Rust defaults.
func DefaultLossyEncodingOptions() LossyEncodingOptions {
	return LossyEncodingOptions{Quality: 90, OptimizationLevel: 0}
}

// LosslessEncodingOptions tunes the lossless VP8L encoder.
type LosslessEncodingOptions struct {
	// OptimizationLevel selects the encode-effort preset in 0..=9.
	OptimizationLevel uint8
}

// DefaultLosslessEncodingOptions returns the balanced default lossless options
// (optimization level 6), matching the upstream Rust defaults.
func DefaultLosslessEncodingOptions() LosslessEncodingOptions {
	return LosslessEncodingOptions{OptimizationLevel: 6}
}
