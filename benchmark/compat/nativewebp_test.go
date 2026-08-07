//go:build testbenchmark

package compat

import (
	"bytes"
	"image"
	"testing"

	nativewebp "github.com/HugoSmits86/nativewebp"

	webp "github.com/SeriousBug/webp-go-pure"
)

// nativewebp is a third pure-Go implementation, VP8L encode only (its Decode is
// a wrapper around x/image), so only the one direction is testable: it encodes,
// we decode, and lossless means the pixels have to come back exactly.

func nativeEncode(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := nativewebp.Encode(&b, img, &nativewebp.Options{CompressionLevel: nativewebp.BestCompression}); err != nil {
		t.Fatalf("nativewebp encode: %v", err)
	}
	return b.Bytes()
}

func TestWeDecodeNativewebpOutput(t *testing.T) {
	const w, h = 64, 48
	src := makeGradientRGBA(w, h)
	encoded := nativeEncode(t, &image.NRGBA{Pix: src, Stride: w * 4, Rect: image.Rect(0, 0, w, h)})

	dec, err := webp.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if dec.Width != w || dec.Height != h {
		t.Fatalf("dims %dx%d", dec.Width, dec.Height)
	}
	for i := range src {
		if dec.RGBA[i] != src[i] {
			t.Fatalf("byte %d: nativewebp=%d ours=%d", i, src[i], dec.RGBA[i])
		}
	}
}

func TestWeDecodeNativewebpAlphaOutput(t *testing.T) {
	const w, h = 40, 32
	src := make([]byte, w*h*4)
	for y := range h {
		for x := range w {
			o := (y*w + x) * 4
			src[o+0] = byte(x * 6)
			src[o+1] = byte(y * 8)
			src[o+2] = byte((x + y) * 3)
			src[o+3] = byte(255 - y*4)
		}
	}
	encoded := nativeEncode(t, &image.NRGBA{Pix: src, Stride: w * 4, Rect: image.Rect(0, 0, w, h)})

	dec, err := webp.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	for i := range src {
		if dec.RGBA[i] != src[i] {
			t.Fatalf("byte %d: nativewebp=%d ours=%d", i, src[i], dec.RGBA[i])
		}
	}
}
