# The codec API

The root package is the codec itself. It works on plain byte buffers rather than
`image.Image`, and it is what [the std package](std.md) is built on. Reach for it
when you already hold pixels in a buffer, when you want the planar YUV entry
points, or when you want to avoid the `image` package entirely.

<!-- glitterate append=1 file="docs_codec_api_test.go" text="
package webp_test

import (
	\"errors\"
	\"fmt\"
	\"os\"

	webp \"github.com/SeriousBug/webp-go-pure\"
)
" -->

```go
import webp "github.com/SeriousBug/webp-go-pure"
```

## Still images

`webp.Image` is width, height, and packed 8-bit RGBA in row-major order. The
alpha is *straight*, not premultiplied, which is the layout of the standard
library's `image.NRGBA` rather than its `image.RGBA`.

<!-- glitterate append=2 file="docs_codec_api_test.go" -->
```go
func codecRoundTrip(data []byte) (int, int, error) {
	img, err := webp.Decode(data)
	if err != nil {
		return 0, 0, err
	}

	reencoded, err := webp.EncodeLossy(&img, &webp.LossyOptions{Quality: 80, Effort: 4})
	if err != nil {
		return 0, 0, err
	}

	features, err := webp.Features(reencoded)
	if err != nil {
		return 0, 0, err
	}
	return features.Width, features.Height, nil
}
```

`DecodeFile` decodes from a path. `Features` reports dimensions, format and
whether there is alpha or animation, reading only a header rather than the whole
file.

`EncodeLossy` and `EncodeLossless` take a `nil` options pointer for the defaults,
quality 90 effort 0 and effort 6 respectively. Note that a non-nil
`LossyOptions` with `Quality` left at zero asks for quality 0, not the default.
The `std` package's `Options` does not have that edge; this one keeps it for
compatibility.

The lossy encoder does not support transparency and rejects input with any pixel
whose alpha is not `0xff`. Lossless takes alpha as it comes.

## EXIF

Set the raw chunk bytes on the options:

<!-- glitterate append=3 file="docs_codec_api_test.go" -->
```go
func encodeWithExif(img webp.Image, exif []byte) ([]byte, error) {
	return webp.EncodeLossless(&img, &webp.LosslessOptions{EXIF: exif})
}
```

## Errors

Errors unwrap to sentinels, so `errors.Is` answers the questions callers
actually ask, without matching on strings or switching on an error kind:

    webp.ErrInvalidParam
    webp.ErrNotEnoughData
    webp.ErrBitstream
    webp.ErrUnsupported
    webp.ErrAnimated
    webp.ErrLossyAlpha

`Decode` handles still images only, so animated input is the one case every
caller has to branch on:

<!-- glitterate append=4 file="docs_codec_api_test.go" -->
```go
func decodeAnything(data []byte) (string, error) {
	img, err := webp.Decode(data)
	if errors.Is(err, webp.ErrAnimated) {
		anim, err := webp.DecodeAnimation(data)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%d frames", len(anim.Frames)), nil
	}
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%dx%d still", img.Width, img.Height), nil
}
```

For the finer-grained cases, `errors.As` on `*webp.DecoderError` or
`*webp.EncoderError` gives you the `Kind` and message.

## Animations

`DecodeAnimation` returns every frame already composited onto the canvas, so a
frame can be displayed without reference to the ones before it. Each frame
carries its display duration in milliseconds.

Every frame is held at `width * height * 4` bytes, so a long animation at a
large canvas size is expensive. There is no frame iterator yet, and no animation
encoder.

## Planar YUV

Lossy WebP is natively planar 4:2:0 YCbCr. `Decode` and `EncodeLossy` convert to
and from RGBA, which costs about a quarter of a lossy decode and an extra
`width * height * 4` buffer. `DecodeYUV` and `EncodeLossyYUV` skip it:

<!-- glitterate append=5 file="docs_codec_api_test.go" -->
```go
func recompress(data []byte, quality uint8) ([]byte, error) {
	planes, err := webp.DecodeYUV(data)
	if err != nil {
		return nil, err
	}
	return webp.EncodeLossyYUV(&planes, &webp.LossyOptions{Quality: quality})
}
```

`YUVImage` carries the three planes with their strides, plus an optional alpha
plane that `DecodeYUV` fills when the container has an `ALPH` chunk. Strides may
exceed the row width, so planes that already carry trailing padding need no
repacking.

`DecodeYUV` is lossy-only: lossless WebP is natively RGBA, so there are no
planes to hand back. Check `Features` first, or use `Decode`, which handles
both. `EncodeLossyYUV` rejects a non-nil alpha plane, since the lossy encoder
has nowhere to put it.

Most callers should not need this directly. `std` uses it to produce and consume
`*image.YCbCr`, which covers the same ground with the standard library's types.

<!-- glitterate append=6 file="docs_codec_api_test.go" text="
func Example_codecRoundTrip() {
	data, err := os.ReadFile(\"testdata/sample_lossy.webp\")
	if err != nil {
		panic(err)
	}

	width, height, err := codecRoundTrip(data)
	if err != nil {
		panic(err)
	}
	fmt.Println(width, height)
	// Output: 1920 1080
}

func Example_encodeWithExif() {
	pix := make([]byte, 4*4*4)
	for i := range pix {
		pix[i] = 0xff
	}
	img := webp.Image{Width: 4, Height: 4, RGBA: pix}

	data, err := encodeWithExif(img, []byte(\"Exif\\x00\\x00II*\\x00\\x08\\x00\\x00\\x00\\x00\\x00\"))
	if err != nil {
		panic(err)
	}
	decoded, err := webp.Decode(data)
	if err != nil {
		panic(err)
	}
	fmt.Println(decoded.Width, decoded.Height)
	// Output: 4 4
}

func Example_decodeAnything() {
	for _, name := range []string{\"testdata/sample_lossy.webp\", \"testdata/sample_animation.webp\"} {
		data, err := os.ReadFile(name)
		if err != nil {
			panic(err)
		}
		line, err := decodeAnything(data)
		if err != nil {
			panic(err)
		}
		fmt.Println(line)
	}
	// Output:
	// 1920x1080 still
	// 3 frames
}

func Example_recompress() {
	data, err := os.ReadFile(\"testdata/sample_lossy.webp\")
	if err != nil {
		panic(err)
	}

	smaller, err := recompress(data, 50)
	if err != nil {
		panic(err)
	}
	features, err := webp.Features(smaller)
	if err != nil {
		panic(err)
	}
	fmt.Println(features.Width, features.Height, features.Format == webp.FormatLossy)
	// Output: 1920 1080 true
}
" -->
