package webp

// ImageBuffer is an RGBA pixel buffer for a decoded or to-be-encoded still image.
type ImageBuffer struct {
	// Width in pixels.
	Width int
	// Height in pixels.
	Height int
	// RGBA holds packed RGBA8 pixels in row-major order.
	RGBA []byte
}
