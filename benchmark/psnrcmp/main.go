//go:build testbenchmark

// psnrcmp reports size and PSNR for our encoder vs libwebp at matched settings,
// so rate/distortion trade-offs can be compared rather than size alone.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	"time"

	webp "github.com/SeriousBug/webp-go-pure"
	kwebp "github.com/kolesa-team/go-webp/webp"

	"github.com/kolesa-team/go-webp/encoder"
)

func main() {
	dir := flag.String("dir", "testdata/photos", "image directory")
	quality := flag.Int("q", 90, "quality")
	effort := flag.Int("effort", 9, "our effort level")
	method := flag.Int("method", 6, "libwebp method")
	flag.Parse()

	entries, err := os.ReadDir(*dir)
	must(err)
	fmt.Printf("%-34s %10s %8s %9s %10s %8s %9s\n", "image", "ours_B", "ours_dB", "ours_ms", "libwebp_B", "lw_dB", "lw_ms")
	for _, e := range entries {
		ext := filepath.Ext(e.Name())
		if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
			continue
		}
		path := filepath.Join(*dir, e.Name())
		src, nrgba := load(path)

		t0 := time.Now()
		ours, err := webp.EncodeLossy(&src, &webp.LossyOptions{Quality: uint8(*quality), Effort: uint8(*effort)})
		must(err)
		oursMs := float64(time.Since(t0).Microseconds()) / 1000.0
		oursDec, err := webp.Decode(ours)
		must(err)

		opts, err := encoder.NewLossyEncoderOptions(encoder.PresetDefault, float32(*quality))
		must(err)
		opts.Method = *method
		var lb bytes.Buffer
		t1 := time.Now()
		must(kwebp.Encode(&lb, nrgba, opts))
		lwMs := float64(time.Since(t1).Microseconds()) / 1000.0
		lwDec, err := webp.Decode(lb.Bytes())
		must(err)

		fmt.Printf("%-34s %10d %8.2f %9.1f %10d %8.2f %9.1f\n",
			trunc(e.Name(), 34), len(ours), psnr(src.RGBA, oursDec.RGBA), oursMs,
			lb.Len(), psnr(src.RGBA, lwDec.RGBA), lwMs)
	}
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

func load(path string) (webp.Image, *image.NRGBA) {
	f, err := os.Open(path)
	must(err)
	defer f.Close()
	img, _, err := image.Decode(f)
	must(err)
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	nrgba := image.NewNRGBA(image.Rect(0, 0, w, h))
	rgba := make([]byte, w*h*4)
	i := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			rgba[i], rgba[i+1], rgba[i+2], rgba[i+3] = byte(r>>8), byte(g>>8), byte(bl>>8), 0xff
			o := nrgba.PixOffset(x-b.Min.X, y-b.Min.Y)
			nrgba.Pix[o], nrgba.Pix[o+1], nrgba.Pix[o+2], nrgba.Pix[o+3] = byte(r>>8), byte(g>>8), byte(bl>>8), 0xff
			i += 4
		}
	}
	return webp.Image{Width: w, Height: h, RGBA: rgba}, nrgba
}

func psnr(a, b []byte) float64 {
	if len(a) != len(b) {
		return -1
	}
	var sum float64
	n := 0
	for i := range a {
		if i%4 == 3 {
			continue
		}
		d := float64(a[i]) - float64(b[i])
		sum += d * d
		n++
	}
	if sum == 0 {
		return 99
	}
	return 10 * math.Log10(255*255/(sum/float64(n)))
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
