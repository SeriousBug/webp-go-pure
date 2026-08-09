//go:build testbenchmark

// psnrcmp reports size and PSNR for our encoder vs libwebp at matched settings,
// so rate/distortion trade-offs can be compared rather than size alone.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/draw"
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
	// Straight (non-premultiplied) RGBA, which is what both encoders want, and
	// what keeps the alpha channel intact for sources that have one.
	nrgba := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(nrgba, nrgba.Bounds(), img, b.Min, draw.Src)
	rgba := make([]byte, w*h*4)
	copy(rgba, nrgba.Pix)
	return webp.Image{Width: w, Height: h, RGBA: rgba}, nrgba
}

// psnr scores RGB weighted by the reference alpha, matching webpbench. RGB under
// a transparent pixel is invisible and encoders are free to rewrite it, so
// scoring it would report a difference no viewer can see.
func psnr(a, b []byte) float64 {
	if len(a) != len(b) {
		return -1
	}
	var sum, n float64
	for i := 0; i+3 < len(a); i += 4 {
		w := float64(a[i+3]) / 255
		if w == 0 {
			continue
		}
		for c := 0; c < 3; c++ {
			d := float64(a[i+c]) - float64(b[i+c])
			sum += w * d * d
			n += w
		}
	}
	if n == 0 || sum == 0 {
		return 99
	}
	return 10 * math.Log10(255*255/(sum/n))
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
