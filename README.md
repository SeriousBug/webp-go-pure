# webp-go-pure

Pure Go WebP decoder and encoder. No cgo, no external dependencies.

    go get github.com/SeriousBug/webp-go-pure

Decodes lossy `VP8` and lossless `VP8L` still images, decodes animated WebP into
a composited RGBA frame sequence, and encodes still images as lossy or lossless
from RGBA. Alpha comes through `ALPH` chunks on lossy still images and on lossy
animation frames. All pixel data in and out of the library is packed 8-bit RGBA.

The decoders are a Go port of
[webp-rust](https://github.com/mith-mmk/webp-rust) by MITH@mmk. The encoders
draw on [libwebp](https://chromium.googlesource.com/webm/libwebp/), the C
reference implementation, with arm64 NEON and amd64 SSE assembly for the hot
paths.

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
	"fmt"
	"os"

	webp "github.com/SeriousBug/webp-go-pure"
)
" -->

Top-level still-image decode:

<!-- glitterate append=2 file="docs_readme_test.go" -->
```go
func decodeStill(data []byte) error {
	img, err := webp.Decode(data)
	if err != nil {
		return err
	}
	fmt.Printf("%dx%d\n", img.Width, img.Height)
	return nil
}
```

`Decode` returns a `webp.Image` (packed 8-bit RGBA, plus `Width`/`Height`).
`DecodeFile` does the same from a path, and `Features` reports dimensions and
format without a full decode.

Still-image encode takes a `*webp.Image` and an options struct; pass `nil` for
the defaults (lossy quality 90 effort 0, lossless effort 6). `Effort` runs 0..9,
trading speed for size:

<!-- glitterate append=3 file="docs_readme_test.go" -->
```go
func encodeBoth(img webp.Image) (lossy, lossless []byte, err error) {
	lossy, err = webp.EncodeLossy(&img, &webp.LossyOptions{Quality: 90, Effort: 4})
	if err != nil {
		return nil, nil, err
	}
	lossless, err = webp.EncodeLossless(&img, nil)
	if err != nil {
		return nil, nil, err
	}
	return lossy, lossless, nil
}
```

The lossy encoder does not handle transparency yet and rejects input with any
pixel whose alpha is not `0xff`. Lossless encoding takes alpha as it comes.

`webp.Image` is a plain struct, so converting to and from the standard
library's `image.Image` is up to the caller.
[docs/image-interop.md](docs/image-interop.md) covers both directions, including
premultiplied vs straight alpha.

To embed raw EXIF metadata, set it on the options:

<!-- glitterate append=4 file="docs_readme_test.go" -->
```go
func encodeWithExif(img webp.Image, exif []byte) ([]byte, error) {
	return webp.EncodeLossless(&img, &webp.LosslessOptions{EXIF: exif})
}
```

Animated WebP goes through `DecodeAnimation`, which returns composited RGBA
frames. `Decode` does not accept it, so check `Features` first to pick the entry
point:

<!-- glitterate append=5 file="docs_readme_test.go" -->
```go
func decodeStillOrAnimation(data []byte) error {
	features, err := webp.Features(data)
	if err != nil {
		return err
	}
	if features.HasAnimation {
		anim, err := webp.DecodeAnimation(data)
		if err != nil {
			return err
		}
		fmt.Println("animation, frames:", len(anim.Frames))
		return nil
	}
	return decodeStill(data)
}
```

<!-- glitterate append=6 file="docs_readme_test.go" text="
func opaque(w, h int) []byte {
	pix := make([]byte, w*h*4)
	for i := range pix {
		pix[i] = 0xff
	}
	return pix
}

func Example_decodeStill() {
	data, err := os.ReadFile("testdata/sample.webp")
	if err != nil {
		panic(err)
	}
	if err := decodeStill(data); err != nil {
		panic(err)
	}
	// Output: 1920x1080
}

func Example_encodeBoth() {
	img := webp.Image{Width: 2, Height: 2, RGBA: opaque(2, 2)}
	lossy, lossless, err := encodeBoth(img)
	if err != nil {
		panic(err)
	}

	lossyFeatures, err := webp.Features(lossy)
	if err != nil {
		panic(err)
	}
	losslessFeatures, err := webp.Features(lossless)
	if err != nil {
		panic(err)
	}
	fmt.Println(lossyFeatures.Format == webp.FormatLossy, losslessFeatures.Format == webp.FormatLossless)
	// Output: true true
}

func Example_encodeWithExif() {
	img := webp.Image{Width: 2, Height: 2, RGBA: opaque(2, 2)}
	data, err := encodeWithExif(img, []byte("Exif\x00\x00II*\x00\x08\x00\x00\x00\x00\x00"))
	if err != nil {
		panic(err)
	}

	decoded, err := webp.Decode(data)
	if err != nil {
		panic(err)
	}
	fmt.Println(decoded.Width, decoded.Height)
	// Output: 2 2
}

func Example_decodeStillOrAnimation() {
	for _, name := range []string{"testdata/sample.webp", "testdata/sample_animation.webp"} {
		data, err := os.ReadFile(name)
		if err != nil {
			panic(err)
		}
		if err := decodeStillOrAnimation(data); err != nil {
			panic(err)
		}
	}
	// Output:
	// 1920x1080
	// animation, frames: 3
}
" -->
