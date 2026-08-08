# The std package

`github.com/SeriousBug/webp-go-pure/std` implements the standard library's codec
interfaces. Its package name is `webp`, so a bare import reads the way
`image/png` and `image/jpeg` do at the call site, and importing it registers the
format with `image.Decode`.

The root package is also named `webp`, so on the rare occasion you want both,
give one of them a name:

```go
import (
	"github.com/SeriousBug/webp-go-pure/std"
	codec "github.com/SeriousBug/webp-go-pure"
)
```

<!-- glitterate append=1 file="docs_std_test.go" text="
package webp_test

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"os"

	"github.com/SeriousBug/webp-go-pure/std"
)
" -->

```go
import "github.com/SeriousBug/webp-go-pure/std"
```

    Decode(r io.Reader) (image.Image, error)
    DecodeConfig(r io.Reader) (image.Config, error)
    DecodeAll(r io.Reader) (*Animation, error)
    Encode(w io.Writer, m image.Image, o *Options) error

`DecodeBytes` and `EncodeBytes` are the same two operations for callers who
already hold, or want, a `[]byte`. They skip the copy that `io.ReadAll` and
`Write` would make.

## What Decode returns

`Decode` returns the type that matches how the file stores its pixels, so no
conversion happens on the way out:

| input | type |
| --- | --- |
| lossy `VP8` | `*image.YCbCr`, 4:2:0 |
| lossy `VP8` with an `ALPH` chunk | `*image.NYCbCrA`, 4:2:0 |
| lossless `VP8L` | `*image.NRGBA` |
| animated | first frame, as `*image.NRGBA` |

<!-- glitterate append=2 file="docs_std_test.go" -->
```go
func decodedType(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	img, err := webp.Decode(f)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%T", img), nil
}
```

All four satisfy `image.Image`, so `At`, `Bounds` and `draw.Draw` work the same
way whichever one you get.

If you would rather have one predictable layout, `DecodeNRGBA` and
`DecodeNRGBABytes` always return an `*image.NRGBA`, with straight (not
premultiplied) alpha. That is cheaper than converting the result of `Decode`
yourself, because the codec converts its own planes directly.

<!-- glitterate append=3 file="docs_std_test.go" -->
```go
func decodeAsNRGBA(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	img, err := webp.DecodeNRGBA(f)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%T %v", img, img.Bounds()), nil
}
```

## What Encode recognizes

`Encode` looks for the types it can feed to the encoder without converting:

- `*image.YCbCr` at 4:2:0 goes straight through as planes.
- `*image.NYCbCrA` at 4:2:0 goes straight through when it is opaque. The lossy
  encoder has no alpha channel, so a transparent one is rejected instead.
- `*image.NRGBA` is already the layout the byte-oriented API takes.

`Encode` takes any `image.Image`. Anything not on that list, including
`*image.RGBA`, sub-images, non-4:2:0 chroma and odd crop origins, is drawn into
an `*image.NRGBA` first, which costs a pass over the pixels.

`*image.RGBA` is one of those. Its pixels are alpha-premultiplied and WebP
stores straight alpha, so the conversion keeps semi-transparent pixels from
coming out dark.

<!-- glitterate append=4 file="docs_std_test.go" -->
```go
func encodeHalfTransparentRed() (color.NRGBA, error) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	src.Set(0, 0, color.NRGBA{R: 255, A: 128})

	data, err := webp.EncodeBytes(src, &webp.Options{Lossless: true})
	if err != nil {
		return color.NRGBA{}, err
	}
	decoded, err := webp.DecodeBytes(data)
	if err != nil {
		return color.NRGBA{}, err
	}
	return color.NRGBAModel.Convert(decoded.At(0, 0)).(color.NRGBA), nil
}
```

That returns `{255 0 0 128}`. A library that passed `*image.RGBA` pixels through
unchanged would return `{128 0 0 128}`.

## Options

<!-- glitterate append=5 file="docs_std_test.go" -->
```go
func encodeLossless(img image.Image) ([]byte, error) {
	return webp.EncodeBytes(img, &webp.Options{Lossless: true, Effort: 6})
}
```

| field | meaning |
| --- | --- |
| `Quality` | 1..100 for lossy. Zero means 90. Ignored when `Lossless` is set. |
| `Effort` | 0..9 for lossy, 0..6 for lossless (7..9 accepted, same as 6). Higher is slower and smaller. Zero means the default for the mode, 0 lossy and 6 lossless. Pass `webp.EffortFastest` to ask for 0 explicitly. |
| `Lossless` | Encode with VP8L, reproducing the input exactly. |
| `EXIF` | Raw EXIF bytes to embed as a metadata chunk. |

A `nil` `*Options` means all of the above defaults, so `Encode(w, img, nil)` is
lossy at quality 90.

## Alpha

The lossy encoder cannot store transparency. Encoding an image that is not fully
opaque fails with an error matching `webp.ErrLossyAlpha`, rather than silently
flattening it:

<!-- glitterate append=6 file="docs_std_test.go" -->
```go
func encodeTransparent() string {
	src := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	src.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 128})

	_, err := webp.EncodeBytes(src, nil)
	if errors.Is(err, webp.ErrLossyAlpha) {
		return "needs lossless"
	}
	return fmt.Sprint(err)
}
```

Set `Lossless` to keep the alpha channel. Decoding transparency works either
way: lossy files carrying an `ALPH` chunk decode to `*image.NYCbCrA`.

## Animations

`DecodeAll` returns every frame, already composited onto the canvas, in the
shape of `gif.GIF`. Unlike `gif.GIF`, the delays are in milliseconds, because
that is what the WebP container stores.

<!-- glitterate append=7 file="docs_std_test.go" -->
```go
func summarizeAnimation(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	anim, err := webp.DecodeAll(f)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d frames, %dms first", len(anim.Image), anim.Delay[0]), nil
}
```

A still image decodes as a single-frame animation, so a caller that handles both
does not need to branch. In the other direction, `Decode` on an animated file
returns the first frame rather than failing, so `image.Decode` works on
animations.

Encoding animations is not implemented.

## DecodeConfig

`DecodeConfig` reports the dimensions and the color model `Decode` would
produce, without decoding the pixels. It reads a few hundred bytes of a typical
file, more only when metadata chunks sit in front of the image data. Use it to
check an image's size before committing to decoding it.

<!-- glitterate append=8 file="docs_std_test.go" text="
func Example_decodedType() {
	for _, path := range []string{"testdata/sample_lossy.webp", "testdata/sample_lossless.webp"} {
		name, err := decodedType(path)
		if err != nil {
			panic(err)
		}
		fmt.Println(name)
	}
	// Output:
	// *image.YCbCr
	// *image.NRGBA
}

func Example_decodeAsNRGBA() {
	// Lossy input, which Decode would hand back as an *image.YCbCr.
	line, err := decodeAsNRGBA("testdata/sample_lossy.webp")
	if err != nil {
		panic(err)
	}
	fmt.Println(line)
	// Output: *image.NRGBA (0,0)-(1920,1080)
}

func Example_encodeHalfTransparentRed() {
	pixel, err := encodeHalfTransparentRed()
	if err != nil {
		panic(err)
	}
	fmt.Println(pixel.R, pixel.G, pixel.B, pixel.A)
	// Output: 255 0 0 128
}

func Example_encodeLossless() {
	src := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	src.SetNRGBA(1, 1, color.NRGBA{R: 3, G: 5, B: 7, A: 9})

	data, err := encodeLossless(src)
	if err != nil {
		panic(err)
	}
	decoded, err := webp.DecodeBytes(data)
	if err != nil {
		panic(err)
	}
	fmt.Println(color.NRGBAModel.Convert(decoded.At(1, 1)).(color.NRGBA))
	// Output: {3 5 7 9}
}

func Example_encodeTransparent() {
	fmt.Println(encodeTransparent())
	// Output: needs lossless
}

func Example_summarizeAnimation() {
	line, err := summarizeAnimation("testdata/sample_animation.webp")
	if err != nil {
		panic(err)
	}
	fmt.Println(line)
	// Output: 3 frames, 500ms first
}

func Example_decodeConfig() {
	f, err := os.Open("testdata/sample_lossy.webp")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	config, err := webp.DecodeConfig(f)
	if err != nil {
		panic(err)
	}
	fmt.Println(config.Width, config.Height, config.ColorModel == color.YCbCrModel)
	// Output: 1920 1080 true
}
" -->
