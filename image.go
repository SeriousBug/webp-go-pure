package webp

// Image is an RGBA pixel buffer for a decoded or to-be-encoded still image.
type Image struct {
	// Width in pixels.
	Width int
	// Height in pixels.
	Height int
	// RGBA holds packed RGBA8 pixels in row-major order, with straight
	// (non-premultiplied) alpha, the same layout as the standard library's
	// image.NRGBA. This is not image.RGBA's premultiplied convention.
	RGBA []byte
}
