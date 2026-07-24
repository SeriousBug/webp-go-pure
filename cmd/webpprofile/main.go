// webpprofile runs a single "ours" encoder repeatedly under CPU/mem profiling.
// Pure Go, no cgo, so it profiles only this library's hot paths.
//
//	go run ./cmd/webpprofile -img testdata/photos/Lena_512.png -mode lossy-slow -n 3 -cpu cpu.prof
//	go tool pprof -http=:0 cpu.prof
package main

import (
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	webp "github.com/SeriousBug/webp-go-pure"
)

func main() {
	img := flag.String("img", "testdata/photos/Lena_512.png", "source image")
	mode := flag.String("mode", "lossy-slow", "lossless|lossy-fast|lossy-slow")
	n := flag.Int("n", 1, "iterations")
	cpu := flag.String("cpu", "", "CPU profile output path")
	mem := flag.String("mem", "", "heap profile output path")
	flag.Parse()

	buf, err := load(*img)
	must(err)

	var fn func() ([]byte, error)
	switch *mode {
	case "lossless":
		fn = func() ([]byte, error) { return webp.EncodeLossless(&buf, 6, nil) }
	case "lossy-fast":
		fn = func() ([]byte, error) { return webp.EncodeLossy(&buf, 0, 90, nil) }
	case "lossy-slow":
		fn = func() ([]byte, error) { return webp.EncodeLossy(&buf, 9, 90, nil) }
	default:
		fmt.Fprintln(os.Stderr, "bad mode")
		os.Exit(1)
	}

	if *cpu != "" {
		f, err := os.Create(*cpu)
		must(err)
		must(pprof.StartCPUProfile(f))
		defer pprof.StopCPUProfile()
	}

	fmt.Fprintf(os.Stderr, "%s %s %dx%d, n=%d\n", *mode, *img, buf.Width, buf.Height, *n)
	t0 := time.Now()
	var size int
	for i := 0; i < *n; i++ {
		out, err := fn()
		must(err)
		size = len(out)
	}
	d := time.Since(t0)
	fmt.Fprintf(os.Stderr, "bytes=%d total=%s per-op=%s\n", size, d, d/time.Duration(*n))

	if *mem != "" {
		f, err := os.Create(*mem)
		must(err)
		runtime.GC()
		must(pprof.WriteHeapProfile(f))
		f.Close()
	}
}

func load(path string) (webp.ImageBuffer, error) {
	f, err := os.Open(path)
	if err != nil {
		return webp.ImageBuffer{}, err
	}
	defer f.Close()
	im, _, err := image.Decode(f)
	if err != nil {
		return webp.ImageBuffer{}, err
	}
	b := im.Bounds()
	w, h := b.Dx(), b.Dy()
	rgba := make([]byte, w*h*4)
	i := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := im.At(x, y).RGBA()
			rgba[i+0] = byte(r >> 8)
			rgba[i+1] = byte(g >> 8)
			rgba[i+2] = byte(bl >> 8)
			rgba[i+3] = byte(a >> 8)
			i += 4
		}
	}
	return webp.ImageBuffer{Width: w, Height: h, RGBA: rgba}, nil
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
