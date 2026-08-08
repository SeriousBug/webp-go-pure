# The codec API

The root package is the codec itself. It works on plain byte buffers rather than
`image.Image`, and it is what [the std package](std.md) is built on. Reach for it
when you already hold pixels in a buffer, when you want the planar YUV entry
points, or when you want to avoid the `image` package entirely.

<!-- glitterate append=1 file="docs_codec_api_test.go" text="
package webp_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	webp "github.com/SeriousBug/webp-go-pure"
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
quality 90 effort 0 and effort 6 respectively. Lossy effort runs 0..9; lossless
effort runs 0..6, and 7..9 are accepted but encode the same as 6. Watch out for one edge: a non-nil
`LossyOptions` with `Quality` left at zero asks for quality 0, not the default.
The `std` package's `Options` reads zero as the default instead.

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

Errors unwrap to sentinels, so `errors.Is` answers "is this animated?" or "did
lossy reject the alpha?" without matching on strings:

    webp.ErrInvalidParam
    webp.ErrNotEnoughData
    webp.ErrBitstream
    webp.ErrUnsupported
    webp.ErrAnimated
    webp.ErrLossyAlpha

The std package exports the same sentinels, so matching an error from
`webp.Decode` or `webp.Encode` there does not need this import.

`Decode` handles still images only, so animated input needs a branch:

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
`width * height * 4` buffer. `DecodeYUV` and `EncodeLossyYUV` skip that, which
is worth it when you are recompressing and never touch the pixels:

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

### Sample range

WebP stores YCbCr in the BT.601 studio range, where luma runs 16..235 and chroma
16..240 rather than using all 256 codes. JPEG and Go's `image.YCbCr` use the
full 0..255. `YUVImage.Range` says which one a set of planes is in, and getting
it wrong lifts blacks and dims whites by around 8 percent.

`RangeLimited` is the zero value, so planes from `DecodeYUV` and planes you hand
to `EncodeLossyYUV` are assumed to be WebP's own. Set `Range: webp.RangeFull` on
planes that came from somewhere else, such as `image/jpeg`, and the encoder
rescales them as it reads. `ConvertRange` rescales an existing `YUVImage` in
place:

    planes.ConvertRange(webp.RangeFull)

Most callers do not need any of this: `std` handles the range when it converts
to and from `*image.YCbCr`.

<!-- glitterate append=6 file="docs_codec_api_test.go" text="
func Example_codecRoundTrip() {
	data, err := os.ReadFile("testdata/sample_lossy.webp")
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

	exif := []byte("Exif\x00\x00II*\x00\x08\x00\x00\x00\x00\x00")
	data, err := encodeWithExif(img, exif)
	if err != nil {
		panic(err)
	}
	decoded, err := webp.Decode(data)
	if err != nil {
		panic(err)
	}
	fmt.Println(decoded.Width, decoded.Height, bytes.Contains(data, exif))
	// Output: 4 4 true
}

func Example_decodeAnything() {
	for _, name := range []string{"testdata/sample_lossy.webp", "testdata/sample_animation.webp"} {
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
	data, err := os.ReadFile("testdata/sample_lossy.webp")
	if err != nil {
		panic(err)
	}

	low, err := recompress(data, 30)
	if err != nil {
		panic(err)
	}
	high, err := recompress(data, 90)
	if err != nil {
		panic(err)
	}
	features, err := webp.Features(low)
	if err != nil {
		panic(err)
	}
	fmt.Println(features.Width, features.Height, features.Format == webp.FormatLossy, len(low) < len(high))
	// Output: 1920 1080 true true
}
" -->
