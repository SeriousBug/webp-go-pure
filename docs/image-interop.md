# Working with image.Image

The encoder takes a `webp.Image`: width, height, and packed straight-alpha
RGBA8 bytes. Converting to and from the standard library's `image.Image` takes
a few lines in each direction.

<!-- glitterate append=1 file="docs_image_interop_test.go" -->
```go
package webp_test

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"

	webp "github.com/SeriousBug/webp-go-pure"
)
```

## From image.Image

Drawing into an `*image.NRGBA` handles every source type: `*image.RGBA`, the
`*image.YCbCr` a JPEG decodes to, paletted PNGs, whatever your HEIC decoder
hands back. Sources already in that layout, including images from `fromWebP`
below, skip the copy.

<!-- glitterate append=2 file="docs_image_interop_test.go" -->
```go
func toWebP(src image.Image) webp.Image {
	b := src.Bounds()
	nrgba, ok := src.(*image.NRGBA)
	if !ok || !b.Eq(nrgba.Rect) || nrgba.Stride != b.Dx()*4 {
		nrgba = image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		draw.Draw(nrgba, nrgba.Bounds(), src, b.Min, draw.Src)
	}
	return webp.Image{Width: b.Dx(), Height: b.Dy(), RGBA: nrgba.Pix}
}
```

Do not read pixels with `src.At(x, y).RGBA()` in a loop: those values are
*premultiplied*, while the encoder expects straight alpha, so a pixel at half
opacity would encode at half its brightness. `draw.Draw` into an `NRGBA`
un-premultiplies for you.

## Back to image.Image

The pixel layouts are identical, so this direction is a wrapper over the same
bytes rather than a copy.

<!-- glitterate append=3 file="docs_image_interop_test.go" -->
```go
func fromWebP(img webp.Image) *image.NRGBA {
	return &image.NRGBA{
		Pix:    img.RGBA,
		Stride: img.Width * 4,
		Rect:   image.Rect(0, 0, img.Width, img.Height),
	}
}
```

## Encoding

`EncodeLossy` returns a `[]byte`. Pass `nil` options for the defaults, quality
90 and effort 0. `Effort` runs 0..9 and trades encode time for file size.

<!-- glitterate append=4 file="docs_image_interop_test.go" -->
```go
func ExampleEncodeLossy() {
	src := image.NewRGBA(image.Rect(0, 0, 20, 20))
	draw.Draw(src, src.Bounds(), &image.Uniform{color.RGBA{200, 30, 40, 255}}, image.Point{}, draw.Src)

	img := toWebP(src)
	data, err := webp.EncodeLossy(&img, &webp.LossyOptions{Quality: 80, Effort: 4})
	if err != nil {
		panic(err)
	}

	features, err := webp.Features(data)
	if err != nil {
		panic(err)
	}
	fmt.Println(features.Width, features.Height, features.Format == webp.FormatLossy)
	// Output: 20 20 true
}
```

Lossless encoding round-trips pixels exactly, alpha included. A half
transparent red comes back as `255 0 0 128`, not `128 0 0 128`.

<!-- glitterate append=5 file="docs_image_interop_test.go" -->
```go
func ExampleEncodeLossless() {
	src := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	src.Set(0, 0, color.NRGBA{R: 255, A: 128})

	img := toWebP(src)
	data, err := webp.EncodeLossless(&img, nil)
	if err != nil {
		panic(err)
	}

	decoded, err := webp.Decode(data)
	if err != nil {
		panic(err)
	}
	fmt.Println(decoded.RGBA[:4])
	// Output: [255 0 0 128]
}
```

## Decoding

`Decode` gives you a `webp.Image`; wrap it and it is ready for `image/png` and
the rest of the standard library. `DecodeFile` does the same from a path.

<!-- glitterate append=6 file="docs_image_interop_test.go" -->
```go
func decodeToImage(data []byte) (image.Image, error) {
	decoded, err := webp.Decode(data)
	if err != nil {
		return nil, err
	}
	return fromWebP(decoded), nil
}
```

The bounds of the result always start at the origin, and its pixels are
straight alpha, so `At` reports the colors the file stores.

<!-- glitterate append=7 file="docs_image_interop_test.go" text="
func Example_decodeToImage() {
	src := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	src.Set(2, 3, color.NRGBA{R: 10, G: 20, B: 30, A: 255})
	buf := toWebP(src)
	data, err := webp.EncodeLossless(&buf, nil)
	if err != nil {
		panic(err)
	}

	out, err := decodeToImage(data)
	if err != nil {
		panic(err)
	}
	fmt.Println(out.Bounds(), out.At(2, 3))
	// Output: (0,0)-(8,8) {10 20 30 255}
}
" -->

## Writing to a file, alongside PNG and JPEG

The encoders return a `[]byte` rather than writing to an `io.Writer`, so the
WebP branch writes the buffer itself. `*os.File` satisfies `io.Writer`, so the
call site stays the same as for `png.Encode`. This example treats quality 100 as
a request for lossless.

<!-- glitterate append=8 file="docs_image_interop_test.go" -->
```go
func encodeImage(to string, dst io.Writer, img image.Image, q int) error {
	switch to {
	case ".png":
		return png.Encode(dst, img)
	case ".jpg", ".jpeg":
		return jpeg.Encode(dst, img, &jpeg.Options{Quality: q})
	case ".webp":
		buf := toWebP(img)
		var data []byte
		var err error
		if q >= 100 {
			data, err = webp.EncodeLossless(&buf, nil)
		} else {
			data, err = webp.EncodeLossy(&buf, &webp.LossyOptions{Quality: uint8(q), Effort: 4})
		}
		if err != nil {
			return err
		}
		_, err = dst.Write(data)
		return err
	default:
		return errors.New("Unsupported destination format for image conversion: " + to)
	}
}
```

The lossy encoder does not handle transparency yet and rejects any pixel whose
alpha is not `0xff`. Lossless takes alpha as it comes.

<!-- glitterate append=9 file="docs_image_interop_test.go" text="
func Example_encodeImage() {
	src := image.NewRGBA(image.Rect(0, 0, 20, 20))
	draw.Draw(src, src.Bounds(), &image.Uniform{color.RGBA{60, 120, 180, 255}}, image.Point{}, draw.Src)

	path := filepath.Join(os.TempDir(), "example_encode_image.webp")
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer os.Remove(path)
	if err := encodeImage(".webp", f, src, 100); err != nil {
		panic(err)
	}
	if err := f.Close(); err != nil {
		panic(err)
	}

	decoded, err := webp.DecodeFile(path)
	if err != nil {
		panic(err)
	}
	fmt.Println(decoded.Width, decoded.Height, fromWebP(decoded).At(5, 5))
	// Output: 20 20 {60 120 180 255}
}
" -->
