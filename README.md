# webp-go-pure

Pure Go WebP decoder and encoder. No cgo, no external dependencies.

    go get github.com/SeriousBug/webp-go-pure

Decodes lossy `VP8` and lossless `VP8L` still images, decodes animated WebP into
a composited frame sequence, and encodes still images as lossy or lossless.
Alpha comes through `ALPH` chunks on lossy still images and on lossy animation
frames.

The `std` subpackage implements the standard library's codec interfaces, so
`image.Image` goes in and comes out and `image.Decode` works. Underneath it, the
root package is the codec itself, working on plain byte buffers.

## Performance

If you can use cgo, use libwebp itself. It encodes faster than webp-go-pure at
slightly better quality per byte.

Without cgo, the alternative is libwebp compiled to WebAssembly and run through
wazero, such as [gen2brain/webp](https://github.com/gen2brain/webp). That costs
both time and memory, and webp-go-pure comes out ahead of it:

- **Lossy:** 1.1-3.4x faster, at 1.7-3.8x lower peak memory.
- **Lossless:** roughly even on time (0.9-2.3x), at 1.2-1.8x lower peak memory.

![Encode time per image for each engine, one panel per mode and machine](benchmark/charts/encode-time-light.svg#gh-light-mode-only)
![Encode time per image for each engine, one panel per mode and machine](benchmark/charts/encode-time-dark.svg#gh-dark-mode-only)
![Peak memory per megapixel for each engine, one panel per mode and machine, on the same bar layout as the encode time figure](benchmark/charts/peak-memory-light.svg#gh-light-mode-only)
![Peak memory per megapixel for each engine, one panel per mode and machine, on the same bar layout as the encode time figure](benchmark/charts/peak-memory-dark.svg#gh-dark-mode-only)

Full tables, PSNR and peak-memory figures, the test corpus and the method are in
[benchmark/results.md](benchmark/results.md).

## Library API

<!-- glitterate append=1 file="docs_readme_test.go" text="
package webp_test

import (
	\"bytes\"
	\"fmt\"
	\"image\"
	\"image/jpeg\"
	\"io\"
	\"os\"

	\"github.com/SeriousBug/webp-go-pure/std\"
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

That reports `1920x1080 *image.YCbCr`, not `*image.RGBA`, because lossy WebP is
natively planar YCbCr and converting it would throw work away. Lossless input
decodes to `*image.NRGBA`, and lossy input with alpha to `*image.NYCbCrA`.

Encoding takes an options struct, or `nil` for the defaults (lossy, quality 90):

<!-- glitterate append=3 file="docs_readme_test.go" -->
```go
func writeWebP(w io.Writer, img image.Image) error {
	return webp.Encode(w, img, &webp.Options{Quality: 80})
}
```

Set `Lossless` to encode with VP8L instead, which reproduces the input exactly
and is the only mode that keeps an alpha channel. `Effort` runs 0..9 and trades
encode time for file size.

### Transcoding

Converting a JPEG needs no conversion code, and does no colorspace math:

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

`image/jpeg` decodes to a 4:2:0 `*image.YCbCr`, and that is exactly what the
lossy WebP encoder wants, so the planes move across untouched. Compared with
routing the same transcode through RGBA, on a 1920x1080 image that is 44ms
against 53ms and 17MB against 25MB. Any other image type still works, it just
costs a conversion.

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

That makes this package a drop-in for `golang.org/x/image/webp`, which has the
same two function signatures. Swapping the import path is the whole migration,
and you gain the encoder and animation support it does not have.

One caveat worth knowing: `image.RegisterFormat` is a process-wide list that
neither rejects duplicates nor allows unregistering, and `x/image/webp` and
`gen2brain/webp` claim the same magic bytes. If more than one of them ends up in
your binary, package initialization order decides which decoder `image.Decode`
picks.

## More

- [docs/std.md](docs/std.md) covers the `std` package in full: which concrete
  types the fast paths recognize, alpha and premultiplication, animations,
  and reusing buffers.
- [docs/codec-api.md](docs/codec-api.md) covers the root package: the byte
  oriented API, EXIF, raw planar YUV, and animation decoding.

<!-- glitterate append=6 file="docs_readme_test.go" text="
func Example_describe() {
	f, err := os.Open(\"testdata/sample_lossy.webp\")
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
	var source bytes.Buffer
	if err := jpeg.Encode(&source, image.NewRGBA(image.Rect(0, 0, 48, 32)), nil); err != nil {
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
	fmt.Println(img.Bounds())
	// Output: (0,0)-(48,32)
}

func Example_sniffFormat() {
	f, err := os.Open(\"testdata/sample_lossless.webp\")
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
