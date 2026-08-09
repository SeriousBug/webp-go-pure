# webp-go-pure

Pure Go WebP decoder and encoder. No cgo, no external dependencies.

    go get github.com/SeriousBug/webp-go-pure

Decodes lossy `VP8` and lossless `VP8L` still images, decodes animated WebP into
a composited frame sequence, and encodes still images as lossy or lossless.
Alpha comes through `ALPH` chunks on lossy still images and on lossy animation
frames. Both encoders keep transparency: lossless reproduces the whole image
exactly, and lossy stores the alpha channel losslessly next to the quantized
color. Encoder output is byte-identical on amd64 and arm64.

The `std` subpackage implements the standard library's codec interfaces, so
`image.Image` goes in and comes out and `image.Decode` works. Underneath it, the
root package is the codec itself, working on plain byte buffers.

## Performance

If you can use cgo, use libwebp itself. It is the faster decoder in every mode,
and the faster encoder in most of them.

Without cgo, the alternative is libwebp compiled to WebAssembly and run through
wazero, such as [gen2brain/webp](https://github.com/gen2brain/webp). That costs
both time and memory, and webp-go-pure comes out ahead of it:

- **Lossy:** 1.2-3.4x faster, at 1.6-2.9x lower peak memory.
- **Lossless:** 1.9-3.5x faster, at 1.3-1.8x lower peak memory.

Against libwebp itself, lossless is a close match on size, half a percent apart
at our effort 6, and on time we range from level with it to 1.47x slower
depending on the machine. Our lossy effort 8 writes 1.7% more than its method 6
at the same PSNR, for 1.0-1.2x the time.

For lossless encoding only, there is another pure Go encoder,
[nativewebp](https://github.com/HugoSmits86/nativewebp). We are smaller and
faster than it at the same time: our effort 0 writes 9.8% smaller files than its
best setting in a sixth of the time, and everything up to our effort 5 still
beats that setting on both. It does use less memory than we do, 0.43-0.63x our
peak, and that gap is the reason to reach for it.

Peak memory is the number to check before using lossless on large images: we cost
1.4-2.1x libwebp's peak there, and around 400 MiB to encode a 5.5 megapixel
image. The lossy modes are cheaper for everyone, and there we are the lightest of
the three.

Effort is the knob to reach for either way. The figure below is every setting of
every encoder: pick the time and size you want, then read the setting off the
point.

![Time vs file size at each effort level: one line per encoder through its effort settings, with encode time on the x axis and output size or PSNR on the y axis](benchmark/charts/effort-sweep-photos-light.svg#gh-light-mode-only)
![Time vs file size at each effort level: one line per encoder through its effort settings, with encode time on the x axis and output size or PSNR on the y axis](benchmark/charts/effort-sweep-photos-dark.svg#gh-dark-mode-only)
![Peak memory per megapixel for each engine, one panel per mode and machine](benchmark/charts/peak-memory-photos-light.svg#gh-light-mode-only)
![Peak memory per megapixel for each engine, one panel per mode and machine](benchmark/charts/peak-memory-photos-dark.svg#gh-dark-mode-only)

Those figures are for encoding. For decoding, the comparison to make is
`golang.org/x/image/webp`, which decodes but does not encode. We are faster than
it in every mode on both machines, by 2% to 20% on the geometric mean; on arm64
lossless the per-image results go either way. Against libwebp we are 2.2-5.8x
slower on lossy and 1.5-3.1x on lossless.

The numbers above are for photographs. Flat graphics with alpha are a different
encoding problem and the rankings shift; full tables, PSNR and peak-memory
figures, both test corpora and the method are in
[benchmark/results.md](benchmark/results.md).

## Library API

<!-- glitterate append=1 file="docs_readme_test.go" text="
package webp_test

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"io"
	"os"

	"github.com/SeriousBug/webp-go-pure/std"
)
" -->

```go
import "github.com/SeriousBug/webp-go-pure/std"
```

`Decode`, `DecodeConfig` and `Encode` have the same signatures as the ones in
`image/png` and `image/jpeg`:

<!-- glitterate append=2 file="docs_readme_test.go" -->
```go
func describe(r io.Reader) (string, error) {
	img, err := webp.Decode(r)
	if err != nil {
		return "", err
	}
	bounds := img.Bounds()
	return fmt.Sprintf("%dx%d %T", bounds.Dx(), bounds.Dy(), img), nil
}
```

The concrete type depends on the file: `*image.YCbCr` for lossy,
`*image.NYCbCrA` for lossy with transparency, `*image.NRGBA` for lossless. All
of them are an `image.Image`, so `At`, `Bounds` and `draw.Draw` work as usual.
If you would rather always get the same type, `DecodeNRGBA` returns an
`*image.NRGBA` whatever the file holds.

Encoding takes an options struct, or `nil` for the defaults (lossy, quality 90):

<!-- glitterate append=3 file="docs_readme_test.go" -->
```go
func writeWebP(w io.Writer, img image.Image) error {
	return webp.Encode(w, img, &webp.Options{Quality: 80})
}
```

Set `Lossless` to encode with VP8L instead, which reproduces the input exactly.
`Effort` runs 0..9 for lossy and 0..6 for lossless, and trades encode time for
file size.

### Transcoding

JPEG to WebP is a decode and an encode, with nothing in between:

<!-- glitterate append=4 file="docs_readme_test.go" -->
```go
func jpegToWebP(dst io.Writer, src io.Reader) error {
	img, err := jpeg.Decode(src)
	if err != nil {
		return err
	}
	return webp.Encode(dst, img, &webp.Options{Quality: 80})
}
```

This path is faster and about a third lighter on memory than transcoding through
RGBA.

### image.Decode

Importing the package registers WebP with `image.Decode` and
`image.DecodeConfig`, the way `image/png` and `golang.org/x/image/webp` do:

<!-- glitterate append=5 file="docs_readme_test.go" -->
```go
func sniffFormat(r io.Reader) (string, error) {
	_, format, err := image.Decode(r)
	return format, err
}
```

To migrate from `golang.org/x/image/webp`, swapping the import path is enough:
we support the same API, and add encoding and animations. One difference is that
lossy colors come out with a wider range here. `x/image/webp` crushes blacks and
whites slightly, decoding a white pixel to 235 where we give you 255.

Keep only one WebP package in your binary. `image.RegisterFormat` has no way to
unregister, and every WebP package claims the same magic bytes, so with two of
them linked in it is import order that decides which one `image.Decode` uses.

## More

- [docs/std.md](docs/std.md) covers the `std` package in full: which concrete
  types the fast paths recognize, alpha and premultiplication, animations,
  and reusing buffers.
- [docs/codec-api.md](docs/codec-api.md) covers the root package: the byte
  oriented API, EXIF, raw planar YUV, and animation decoding.

<!-- glitterate append=6 file="docs_readme_test.go" text="
func Example_describe() {
	f, err := os.Open("testdata/sample_lossy.webp")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	line, err := describe(f)
	if err != nil {
		panic(err)
	}
	fmt.Println(line)
	// Output: 1920x1080 *image.YCbCr
}

func Example_writeWebP() {
	src := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for i := range src.Pix {
		src.Pix[i] = 0xff
	}

	var buf bytes.Buffer
	if err := writeWebP(&buf, src); err != nil {
		panic(err)
	}

	config, format, err := image.DecodeConfig(bytes.NewReader(buf.Bytes()))
	if err != nil {
		panic(err)
	}
	fmt.Println(format, config.Width, config.Height)
	// Output: webp 32 32
}

func Example_jpegToWebP() {
	want := color.RGBA{40, 90, 160, 255}
	src := image.NewRGBA(image.Rect(0, 0, 48, 32))
	draw.Draw(src, src.Bounds(), &image.Uniform{want}, image.Point{}, draw.Src)

	var source bytes.Buffer
	if err := jpeg.Encode(&source, src, nil); err != nil {
		panic(err)
	}

	var out bytes.Buffer
	if err := jpegToWebP(&out, &source); err != nil {
		panic(err)
	}

	img, err := webp.Decode(bytes.NewReader(out.Bytes()))
	if err != nil {
		panic(err)
	}

	got := color.NRGBAModel.Convert(img.At(24, 16)).(color.NRGBA)
	fmt.Println(img.Bounds(), nearNRGBA(got, color.NRGBA{want.R, want.G, want.B, want.A}, 8))
	// Output: (0,0)-(48,32) true
}

func nearNRGBA(a, b color.NRGBA, tolerance int) bool {
	diff := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return diff(a.R, b.R) <= tolerance && diff(a.G, b.G) <= tolerance &&
		diff(a.B, b.B) <= tolerance && a.A == b.A
}

func Example_sniffFormat() {
	f, err := os.Open("testdata/sample_lossless.webp")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	format, err := sniffFormat(f)
	if err != nil {
		panic(err)
	}
	fmt.Println(format)
	// Output: webp
}
" -->
