//go:build testbenchmark

package main

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"os"
	"path/filepath"
	"time"

	webp "github.com/SeriousBug/webp-go-pure"

	gwebp "github.com/gen2brain/webp"
	"github.com/kolesa-team/go-webp/decoder"
	kwebp "github.com/kolesa-team/go-webp/webp"
	ximage "golang.org/x/image/webp"
)

// Every decoder is handed the same libwebp-encoded file. Letting each engine
// decode its own encoder's output would measure two things at once, and libwebp
// is the fastest encoder here, so it is also the cheapest way to produce them.
var decodeModes = []struct {
	name   string
	encode func(*image.NRGBA) func() ([]byte, error)
}{
	{"lossless", func(img *image.NRGBA) func() ([]byte, error) {
		return libwebpEncoder(img, true, 0, libwebpLL)
	}},
	{"lossy", func(img *image.NRGBA) func() ([]byte, error) {
		return libwebpEncoder(img, false, lossyQuality, libwebpSlow)
	}},
}

type rgbaImage struct {
	width, height int
	pix           []byte
}

type decodeCase struct {
	engine string
	fn     func() (rgbaImage, error)
}

func decodeCases(data []byte) []decodeCase {
	return []decodeCase{
		{"ours", func() (rgbaImage, error) {
			img, err := webp.Decode(data)
			if err != nil {
				return rgbaImage{}, err
			}
			return rgbaImage{img.Width, img.Height, img.RGBA}, nil
		}},
		{"libwebp", func() (rgbaImage, error) {
			img, err := kwebp.Decode(bytes.NewReader(data), &decoder.Options{})
			if err != nil {
				return rgbaImage{}, err
			}
			return toRGBA(img), nil
		}},
		{"wasm", func() (rgbaImage, error) {
			img, err := gwebp.Decode(bytes.NewReader(data))
			if err != nil {
				return rgbaImage{}, err
			}
			return toRGBA(img), nil
		}},
		{"x/image", func() (rgbaImage, error) {
			img, err := ximage.Decode(bytes.NewReader(data))
			if err != nil {
				return rgbaImage{}, err
			}
			return toRGBA(img), nil
		}},
	}
}

// toRGBA runs inside the timed call. Lossy WebP is planar YCbCr, so a decoder
// that returns those planes has not done the color conversion the ones handing
// back packed pixels already paid for; timing only the decode call would make
// the two look comparable when they are not. It is free for a decoder that
// already returns packed RGBA.
//
// The test corpus is opaque, so the premultiplied and straight-alpha packings
// hold the same bytes and the reference comparison stays valid across engines.
func toRGBA(img image.Image) rgbaImage {
	b := img.Bounds()
	if b.Min == (image.Point{}) {
		switch p := img.(type) {
		case *image.NRGBA:
			if p.Stride == b.Dx()*4 {
				return rgbaImage{b.Dx(), b.Dy(), p.Pix}
			}
		case *image.RGBA:
			if p.Stride == b.Dx()*4 {
				return rgbaImage{b.Dx(), b.Dy(), p.Pix}
			}
		}
	}
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
	return rgbaImage{b.Dx(), b.Dy(), dst.Pix}
}

// decodePass encodes each source image with libwebp, then times every decoder on
// those bytes. With outDir set it also writes the encoded file and libwebp's
// decode of it, which is what the Rust engine reads for its own pass.
func decodePass(paths []string, outDir string, budget time.Duration, minIters, maxIters int) {
	for _, p := range paths {
		buf, err := loadImageBuffer(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip %s: %v\n", p, err)
			continue
		}
		name := filepath.Base(p)
		nrgba := &image.NRGBA{Pix: buf.RGBA, Stride: buf.Width * 4, Rect: image.Rect(0, 0, buf.Width, buf.Height)}

		for _, mode := range decodeModes {
			data, err := mode.encode(nrgba)()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s %s: encode input: %v\n", name, mode.name, err)
				continue
			}

			// libwebp decodes it once up front: it is the reference
			// implementation, so its pixels are what the others are scored
			// against.
			ref, err := decodeCases(data)[1].fn()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s %s: reference decode: %v\n", name, mode.name, err)
				continue
			}
			if outDir != "" {
				if err := writeDecodeInput(outDir, name, mode.name, data, ref); err != nil {
					fmt.Fprintf(os.Stderr, "%s %s: write input: %v\n", name, mode.name, err)
				}
			}

			for _, e := range decodeCases(data) {
				out, iters, perOp, err := measure(e.fn, budget, minIters, maxIters)
				if err != nil {
					fmt.Fprintf(os.Stderr, "%s/%s %s: %v\n", e.engine, mode.name, name, err)
					continue
				}
				quality, err := decodeQuality(ref, out)
				if err != nil {
					fmt.Fprintf(os.Stderr, "%s/%s %s: psnr: %v\n", e.engine, mode.name, name, err)
					continue
				}
				fmt.Printf("%s,%s,%s,%d,%d,%d,%s,%d,%.3f\n",
					e.engine, mode.name, name, buf.Width, buf.Height, len(data), quality, iters,
					float64(perOp.Microseconds())/1000.0)
			}
		}
	}
}

// decodeQuality scores a decode against libwebp's, and reports "-" when the two
// agree pixel for pixel, which is what a correct lossless decode and libwebp's
// own row both look like. A number means the decoder resolved the same file to
// different pixels.
func decodeQuality(ref, got rgbaImage) (string, error) {
	if got.width != ref.width || got.height != ref.height {
		return "", fmt.Errorf("decoded %dx%d, reference is %dx%d", got.width, got.height, ref.width, ref.height)
	}
	if bytes.Equal(got.pix, ref.pix) {
		return "-", nil
	}
	dB, err := psnrBetween(ref.pix, got.pix)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%.2f", dB), nil
}

func writeDecodeInput(dir, name, mode string, data []byte, ref rgbaImage) error {
	stem := filepath.Join(dir, name+"."+mode)
	if err := os.WriteFile(stem+".webp", data, 0o644); err != nil {
		return err
	}
	return os.WriteFile(stem+".rgba", ref.pix, 0o644)
}
