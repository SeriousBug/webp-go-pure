//go:build testbenchmark

// webpbench encodes images across three modes with three engines, reporting
// output size and encode time:
//
//	ours    - this pure-Go library
//	libwebp - the C reference, via github.com/kolesa-team/go-webp (cgo)
//	wasm    - libwebp compiled to WASM, via github.com/gen2brain/webp (cgo-free)
//
// It is test-only tooling: it needs cgo + libwebp + pkg-config (for the libwebp
// engine) and is excluded from normal builds. Build/run with:
//
//	-tags testbenchmark,nodynamic
//
// The nodynamic tag forces gen2brain/webp onto its WASM path instead of dynamically
// loading a system libwebp, so the "wasm" engine is a true cgo-free comparison.
//
// It emits one CSV line per (engine, mode, image):
//
//	engine,mode,file,width,height,bytes,iters,ms_per_op
//
// so results can be merged with the Rust engine's output (see benchmark/run.sh).
package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"sort"
	"time"

	webp "github.com/SeriousBug/webp-go-pure"

	gwebp "github.com/gen2brain/webp"
	"github.com/kolesa-team/go-webp/encoder"
	kwebp "github.com/kolesa-team/go-webp/webp"
)

const (
	lossyQuality = 90
	oursFastOpt  = 0
	oursSlowOpt  = 9
	oursLLOpt    = 6
	libwebpFast  = 0 // libwebp method 0 = fastest
	libwebpSlow  = 6 // libwebp method 6 = slowest
	libwebpLL    = 6 // lossless preset level
)

func main() {
	dir := flag.String("dir", "testdata/photos", "directory of source images (jpg/png)")
	budgetMs := flag.Int("budget-ms", 2000, "per-measurement time budget in ms")
	minIters := flag.Int("min-iters", 1, "minimum iterations per measurement")
	maxIters := flag.Int("max-iters", 500, "maximum iterations per measurement")
	header := flag.Bool("header", false, "print CSV header line")
	flag.Parse()

	if *header {
		fmt.Println("engine,mode,file,width,height,bytes,iters,ms_per_op")
	}

	paths, err := listImages(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	budget := time.Duration(*budgetMs) * time.Millisecond

	for _, p := range paths {
		buf, err := loadImageBuffer(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip %s: %v\n", p, err)
			continue
		}
		name := filepath.Base(p)
		nrgba := &image.NRGBA{Pix: buf.RGBA, Stride: buf.Width * 4, Rect: image.Rect(0, 0, buf.Width, buf.Height)}

		encoders := []struct {
			engine, mode string
			fn           func() ([]byte, error)
		}{
			{"ours", "lossless", func() ([]byte, error) { return webp.EncodeLossless(&buf, &webp.LosslessOptions{Effort: oursLLOpt}) }},
			{"ours", "lossy-fast", func() ([]byte, error) {
				return webp.EncodeLossy(&buf, &webp.LossyOptions{Quality: lossyQuality, Effort: oursFastOpt})
			}},
			{"ours", "lossy-slow", func() ([]byte, error) {
				return webp.EncodeLossy(&buf, &webp.LossyOptions{Quality: lossyQuality, Effort: oursSlowOpt})
			}},
			{"libwebp", "lossless", libwebpEncoder(nrgba, true, 0, libwebpLL)},
			{"libwebp", "lossy-fast", libwebpEncoder(nrgba, false, lossyQuality, libwebpFast)},
			{"libwebp", "lossy-slow", libwebpEncoder(nrgba, false, lossyQuality, libwebpSlow)},
			{"wasm", "lossless", wasmEncoder(nrgba, gwebp.Options{Lossless: true, Method: libwebpSlow})},
			{"wasm", "lossy-fast", wasmEncoder(nrgba, gwebp.Options{Quality: lossyQuality, Method: libwebpFast})},
			{"wasm", "lossy-slow", wasmEncoder(nrgba, gwebp.Options{Quality: lossyQuality, Method: libwebpSlow})},
		}

		for _, e := range encoders {
			size, iters, perOp, err := measure(e.fn, budget, *minIters, *maxIters)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s/%s %s: %v\n", e.engine, e.mode, name, err)
				continue
			}
			fmt.Printf("%s,%s,%s,%d,%d,%d,%d,%.3f\n",
				e.engine, e.mode, name, buf.Width, buf.Height, size, iters, float64(perOp.Microseconds())/1000.0)
		}
	}
}

func libwebpEncoder(img *image.NRGBA, lossless bool, quality float32, method int) func() ([]byte, error) {
	return func() ([]byte, error) {
		var opts *encoder.Options
		var err error
		if lossless {
			opts, err = encoder.NewLosslessEncoderOptions(encoder.PresetDefault, method)
		} else {
			opts, err = encoder.NewLossyEncoderOptions(encoder.PresetDefault, quality)
			if err == nil {
				opts.Method = method
			}
		}
		if err != nil {
			return nil, err
		}
		var b bytes.Buffer
		if err := kwebp.Encode(&b, img, opts); err != nil {
			return nil, err
		}
		return b.Bytes(), nil
	}
}

func wasmEncoder(img *image.NRGBA, opts gwebp.Options) func() ([]byte, error) {
	return func() ([]byte, error) {
		var b bytes.Buffer
		if err := gwebp.Encode(&b, img, opts); err != nil {
			return nil, err
		}
		return b.Bytes(), nil
	}
}

func measure(fn func() ([]byte, error), budget time.Duration, minIters, maxIters int) (size, iters int, perOp time.Duration, err error) {
	t0 := time.Now()
	out, err := fn() // warmup / first sample
	if err != nil {
		return 0, 0, 0, err
	}
	size = len(out)
	// If a single encode already exceeds the budget, report it as one iteration
	// instead of paying for the warmup plus a full timed loop.
	if warm := time.Since(t0); warm >= budget {
		return size, 1, warm, nil
	}

	start := time.Now()
	for {
		if _, err = fn(); err != nil {
			return 0, 0, 0, err
		}
		iters++
		elapsed := time.Since(start)
		if iters >= maxIters || (iters >= minIters && elapsed >= budget) {
			perOp = elapsed / time.Duration(iters)
			return size, iters, perOp, nil
		}
	}
}

func listImages(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch filepath.Ext(e.Name()) {
		case ".jpg", ".jpeg", ".png":
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func loadImageBuffer(path string) (webp.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return webp.Image{}, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return webp.Image{}, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	rgba := make([]byte, w*h*4)
	i := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			rgba[i+0] = byte(r >> 8)
			rgba[i+1] = byte(g >> 8)
			rgba[i+2] = byte(bl >> 8)
			rgba[i+3] = byte(a >> 8)
			i += 4
		}
	}
	return webp.Image{Width: w, Height: h, RGBA: rgba}, nil
}
