package webp

import (
	"bytes"
	"errors"
	"testing"
)

// syntheticRGBA builds an opaque gradient with enough chroma variation that a
// YUV round trip through the encoder is actually exercised.
func syntheticRGBA(width, height int) []byte {
	rgba := make([]byte, width*height*4)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			offset := (y*width + x) * 4
			rgba[offset] = byte(x * 4)
			rgba[offset+1] = byte(y * 5)
			rgba[offset+2] = byte((x ^ y) * 3)
			rgba[offset+3] = 0xff
		}
	}
	return rgba
}

func TestDecodeYUVMatchesDecodePixels(t *testing.T) {
	data := loadSample(t, "sample_lossy.webp")

	yuv, err := DecodeYUV(data)
	if err != nil {
		t.Fatal(err)
	}
	rgba, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}

	if yuv.Width != rgba.Width || yuv.Height != rgba.Height {
		t.Fatalf("dimensions differ: yuv %dx%d, rgba %dx%d", yuv.Width, yuv.Height, rgba.Width, rgba.Height)
	}
	if yuv.YStride < yuv.Width || yuv.UVStride < (yuv.Width+1)/2 {
		t.Fatalf("strides too small: y=%d uv=%d for width %d", yuv.YStride, yuv.UVStride, yuv.Width)
	}
	if yuv.A != nil {
		t.Fatal("sample has no alpha, but a plane was returned")
	}

	// Converting the planes the same way Decode does must reproduce Decode's
	// output exactly, which pins DecodeYUV to the same decode path.
	planes := lossyPlanes{
		width:    yuv.Width,
		height:   yuv.Height,
		yStride:  yuv.YStride,
		uvStride: yuv.UVStride,
		y:        yuv.Y,
		u:        yuv.U,
		v:        yuv.V,
	}
	if !bytes.Equal(lossyYuvToRgbaFancy(&planes), rgba.RGBA) {
		t.Fatal("converting the YUV planes does not reproduce Decode's RGBA")
	}
}

func TestDecodeYUVReturnsAlphaPlane(t *testing.T) {
	alpha := makeAlphaPlane(1920, 1080)
	data := makeLossyAlphaStillWebp(t, alpha)

	yuv, err := DecodeYUV(data)
	if err != nil {
		t.Fatal(err)
	}
	if yuv.A == nil {
		t.Fatal("expected an alpha plane")
	}
	if yuv.AStride != yuv.Width {
		t.Fatalf("alpha stride %d, want %d", yuv.AStride, yuv.Width)
	}
	if !bytes.Equal(yuv.A, alpha) {
		t.Fatal("alpha plane does not round trip")
	}
}

func TestDecodeYUVRejectsLossless(t *testing.T) {
	_, err := DecodeYUV(loadSample(t, "sample_lossless.webp"))
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("expected ErrUnsupported, got %v", err)
	}
}

func TestDecodeYUVRejectsAnimation(t *testing.T) {
	_, err := DecodeYUV(loadSample(t, "sample_animation.webp"))
	if !errors.Is(err, ErrAnimated) {
		t.Fatalf("expected ErrAnimated, got %v", err)
	}
}

func TestDecodeRejectsAnimationWithErrAnimated(t *testing.T) {
	_, err := Decode(loadSample(t, "sample_animation.webp"))
	if !errors.Is(err, ErrAnimated) {
		t.Fatalf("expected ErrAnimated, got %v", err)
	}
}

// A JPEG hands over planes whose strides are wider than the image, so the
// encoder has to key off the strides rather than the width. Repacking the same
// samples tightly must not change a single output byte.
func TestEncodeLossyYUVIgnoresStridePadding(t *testing.T) {
	width, height := 61, 43
	rgba := syntheticRGBA(width, height)
	lossy, err := EncodeLossy(&Image{Width: width, Height: height, RGBA: rgba}, nil)
	if err != nil {
		t.Fatal(err)
	}
	padded, err := DecodeYUV(lossy)
	if err != nil {
		t.Fatal(err)
	}
	if padded.YStride <= padded.Width {
		t.Fatalf("expected the decoder to hand back padded planes, got stride %d for width %d", padded.YStride, padded.Width)
	}

	uvWidth := (width + 1) / 2
	uvHeight := (height + 1) / 2
	tight := YUVImage{
		Width:    width,
		Height:   height,
		Y:        make([]byte, width*height),
		U:        make([]byte, uvWidth*uvHeight),
		V:        make([]byte, uvWidth*uvHeight),
		YStride:  width,
		UVStride: uvWidth,
	}
	for row := 0; row < height; row++ {
		copy(tight.Y[row*width:(row+1)*width], padded.Y[row*padded.YStride:])
	}
	for row := 0; row < uvHeight; row++ {
		copy(tight.U[row*uvWidth:(row+1)*uvWidth], padded.U[row*padded.UVStride:])
		copy(tight.V[row*uvWidth:(row+1)*uvWidth], padded.V[row*padded.UVStride:])
	}

	fromPadded, err := EncodeLossyYUV(&padded, nil)
	if err != nil {
		t.Fatal(err)
	}
	fromTight, err := EncodeLossyYUV(&tight, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fromPadded, fromTight) {
		t.Fatal("stride padding changed the encoded output")
	}
}

// Re-encoding decoded planes must not drift: the planar path feeds the encoder
// exactly what the decoder produced, with no colorspace conversion in between.
func TestEncodeLossyYUVRoundTripsWithoutDrift(t *testing.T) {
	width, height := 64, 48
	rgba := syntheticRGBA(width, height)

	first, err := EncodeLossy(&Image{Width: width, Height: height, RGBA: rgba}, &LossyOptions{Quality: 95, Effort: 2})
	if err != nil {
		t.Fatal(err)
	}
	before, err := Decode(first)
	if err != nil {
		t.Fatal(err)
	}

	planes, err := DecodeYUV(first)
	if err != nil {
		t.Fatal(err)
	}
	second, err := EncodeLossyYUV(&planes, &LossyOptions{Quality: 95, Effort: 2})
	if err != nil {
		t.Fatal(err)
	}
	after, err := Decode(second)
	if err != nil {
		t.Fatal(err)
	}

	if diff := averageAbsDiff(after.RGBA, before.RGBA); diff > 2.0 {
		t.Fatalf("average channel drift %.2f is too high for a planar re-encode", diff)
	}
}

func TestEncodeLossyYUVRejectsAlpha(t *testing.T) {
	img := YUVImage{
		Width: 2, Height: 2,
		Y: make([]byte, 4), U: make([]byte, 1), V: make([]byte, 1),
		YStride: 2, UVStride: 1,
		A: make([]byte, 4), AStride: 2,
	}
	_, err := EncodeLossyYUV(&img, nil)
	if !errors.Is(err, ErrLossyAlpha) {
		t.Fatalf("expected ErrLossyAlpha, got %v", err)
	}
}

func TestEncodeLossyYUVValidatesPlanes(t *testing.T) {
	valid := func() YUVImage {
		return YUVImage{
			Width: 4, Height: 4,
			Y: make([]byte, 16), U: make([]byte, 4), V: make([]byte, 4),
			YStride: 4, UVStride: 2,
		}
	}

	cases := map[string]func(*YUVImage){
		"zero width":      func(i *YUVImage) { i.Width = 0 },
		"zero height":     func(i *YUVImage) { i.Height = 0 },
		"short Y":         func(i *YUVImage) { i.Y = i.Y[:8] },
		"short U":         func(i *YUVImage) { i.U = i.U[:2] },
		"short V":         func(i *YUVImage) { i.V = i.V[:2] },
		"narrow YStride":  func(i *YUVImage) { i.YStride = 2 },
		"narrow UVStride": func(i *YUVImage) { i.UVStride = 1 },
	}
	for name, corrupt := range cases {
		t.Run(name, func(t *testing.T) {
			img := valid()
			corrupt(&img)
			if _, err := EncodeLossyYUV(&img, nil); !errors.Is(err, ErrInvalidParam) {
				t.Fatalf("expected ErrInvalidParam, got %v", err)
			}
		})
	}

	img := valid()
	if _, err := EncodeLossyYUV(&img, nil); err != nil {
		t.Fatalf("the valid case must encode, got %v", err)
	}
}
