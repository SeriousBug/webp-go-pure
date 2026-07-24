// webpcli is a small pure-Go command-line tool for converting between WebP and
// PNG/JPEG using github.com/SeriousBug/webp-go-pure. It builds with CGO_ENABLED=0
// and runs in a distroless static container.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"strings"

	_ "image/jpeg"

	webp "github.com/SeriousBug/webp-go-pure"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "encode":
		if err := encode(os.Args[2:]); err != nil {
			fatal(err)
		}
	case "decode":
		if err := decode(os.Args[2:]); err != nil {
			fatal(err)
		}
	case "-h", "--help", "help":
		usage()
	default:
		usage()
		os.Exit(2)
	}
}

func encode(args []string) error {
	fs := flag.NewFlagSet("encode", flag.ExitOnError)
	in := fs.String("in", "", "input PNG or JPEG file")
	out := fs.String("out", "", "output WebP file")
	lossless := fs.Bool("lossless", false, "encode lossless (VP8L) instead of lossy")
	quality := fs.Int("quality", 90, "lossy quality 0..100")
	optimize := fs.Int("optimize", 4, "optimization level 0..9")
	fs.Parse(args)
	if *in == "" || *out == "" {
		return fmt.Errorf("encode requires -in and -out")
	}

	src, err := readImage(*in)
	if err != nil {
		return err
	}
	buf := toImageBuffer(src)

	var data []byte
	if *lossless {
		data, err = webp.EncodeLossless(&buf, &webp.LosslessOptions{Effort: uint8(*optimize)})
	} else {
		data, err = webp.EncodeLossy(&buf, &webp.LossyOptions{Quality: uint8(*quality), Effort: uint8(*optimize)})
	}
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		return err
	}
	fmt.Printf("encoded %s (%dx%d) -> %s (%d bytes, %s)\n", *in, buf.Width, buf.Height, *out, len(data), modeName(*lossless))
	return nil
}

func decode(args []string) error {
	fs := flag.NewFlagSet("decode", flag.ExitOnError)
	in := fs.String("in", "", "input WebP file")
	out := fs.String("out", "", "output PNG file")
	fs.Parse(args)
	if *in == "" || *out == "" {
		return fmt.Errorf("decode requires -in and -out")
	}

	data, err := os.ReadFile(*in)
	if err != nil {
		return err
	}
	buf, err := webp.Decode(data)
	if err != nil {
		return err
	}

	img := &image.NRGBA{
		Pix:    buf.RGBA,
		Stride: buf.Width * 4,
		Rect:   image.Rect(0, 0, buf.Width, buf.Height),
	}
	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return err
	}
	fmt.Printf("decoded %s -> %s (%dx%d)\n", *in, *out, buf.Width, buf.Height)
	return nil
}

func readImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

func toImageBuffer(src image.Image) webp.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	rgba := make([]byte, w*h*4)
	i := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := src.At(x, y).RGBA()
			rgba[i+0] = byte(r >> 8)
			rgba[i+1] = byte(g >> 8)
			rgba[i+2] = byte(bl >> 8)
			rgba[i+3] = byte(a >> 8)
			i += 4
		}
	}
	return webp.Image{Width: w, Height: h, RGBA: rgba}
}

func modeName(lossless bool) string {
	if lossless {
		return "lossless"
	}
	return "lossy"
}

func usage() {
	fmt.Fprint(os.Stderr, strings.TrimSpace(`
webpcli - pure-Go WebP encode/decode

Usage:
  webpcli encode -in input.png -out output.webp [-lossless] [-quality 90] [-optimize 4]
  webpcli decode -in input.webp -out output.png
`)+"\n")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
