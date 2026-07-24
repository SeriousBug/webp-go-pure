package webp

// DecodedImage is a decoded RGBA image.
type decodedImage struct {
	Width  int
	Height int
	RGBA   []byte
}

// DecodedYuvImage is a decoded YUV420 image.
type decodedYuvImage struct {
	Width    int
	Height   int
	YStride  int
	UVStride int
	Y        []byte
	U        []byte
	V        []byte
}

// DecodedAnimationFrame is one fully composited animation frame.
type AnimationFrame struct {
	// Duration is the display duration in milliseconds.
	Duration int
	// RGBA is the packed RGBA8 canvas after compositing this frame.
	RGBA []byte
}

// DecodedAnimation is a decoded animated WebP sequence.
type Animation struct {
	Width           int
	Height          int
	BackgroundColor uint32
	LoopCount       uint16
	Frames          []AnimationFrame
}
