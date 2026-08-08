//go:build testbenchmark

// webpbench encodes images across three modes with four engines, reporting
// output size and encode time:
//
//	ours       - this pure-Go library
//	libwebp    - the C reference, via github.com/kolesa-team/go-webp (cgo)
//	wasm       - libwebp compiled to WASM, via github.com/gen2brain/webp (cgo-free)
//	nativewebp - github.com/HugoSmits86/nativewebp, a pure-Go VP8L encoder
//
// nativewebp writes VP8L only, so it appears in the lossless mode alone.
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
//	engine,mode,file,width,height,bytes,psnr_db,iters,ms_per_op
//
// psnr_db scores the encoder's own output against the pixels it was handed, and
// is "-" for lossless.
//
// With -sweep it walks every effort setting each engine exposes instead of the
// three fixed modes, emitting an extra effort column:
//
//	engine,mode,effort,file,width,height,bytes,psnr_db,iters,ms_per_op
//
// Its modes are "lossless" and "lossy" (quality 90). That pass is what shows the
// time-for-size tradeoff: an engine is a curve through (time, size), not a point.
//
// With -mem it runs the memory pass instead, emitting:
//
//	engine,mode,file,width,height,megapixels,peak_rss_mib,mib_per_mp
//
// Peak RSS is process-wide, which is the only figure comparable across a pure-Go
// encoder, a cgo one that allocates in C, and wazero's WASM linear memory. Each
// measurement therefore gets its own subprocess (this binary, re-executed with
// -mem-one) that decodes the source image, encodes it once, and reports its own
// ru_maxrss: what an application pays to encode that image with that engine.
//
// With -decode it times decoding instead, on the same CSV shape, adding a fourth
// engine:
//
//	x/image - golang.org/x/image/webp, the Go project's own decoder (no encoder)
//
// Every engine decodes the same libwebp-encoded file, in modes "lossless" and
// "lossy". Here psnr_db scores each decoder against libwebp's decode of that
// same file, and is "-" when the two agree pixel for pixel.
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
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	webp "github.com/SeriousBug/webp-go-pure"

	nativewebp "github.com/HugoSmits86/nativewebp"
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

	oursMaxEffort    = 9 // our lossy Effort is 0..9
	oursMaxEffortLL  = 6 // our lossless Effort is 0..6; 7..9 behave as 6
	libwebpMaxMethod = 6 // libwebp method (the lossy knob, and wasm's only knob)
	libwebpMaxLevel  = 9 // libwebp lossless preset level, cwebp's -z
)

// nativewebpLevels are the compression levels nativewebp distinguishes: its
// getMethodLevel maps BestSpeed to 0, DefaultCompression to 4, BestCompression
// to 6, and everything else to 4, so those three are the whole scale.
var nativewebpLevels = []int{0, 4, 6}

type encoderCase struct {
	engine, mode string
	fn           func() ([]byte, error)
}

func encoderCases(buf *webp.Image) []encoderCase {
	nrgba := &image.NRGBA{Pix: buf.RGBA, Stride: buf.Width * 4, Rect: image.Rect(0, 0, buf.Width, buf.Height)}
	return []encoderCase{
		{"ours", "lossless", func() ([]byte, error) { return webp.EncodeLossless(buf, &webp.LosslessOptions{Effort: oursLLOpt}) }},
		{"ours", "lossy-fast", func() ([]byte, error) {
			return webp.EncodeLossy(buf, &webp.LossyOptions{Quality: lossyQuality, Effort: oursFastOpt})
		}},
		{"ours", "lossy-slow", func() ([]byte, error) {
			return webp.EncodeLossy(buf, &webp.LossyOptions{Quality: lossyQuality, Effort: oursSlowOpt})
		}},
		{"libwebp", "lossless", libwebpEncoder(nrgba, true, 0, libwebpLL)},
		{"libwebp", "lossy-fast", libwebpEncoder(nrgba, false, lossyQuality, libwebpFast)},
		{"libwebp", "lossy-slow", libwebpEncoder(nrgba, false, lossyQuality, libwebpSlow)},
		{"wasm", "lossless", wasmEncoder(nrgba, gwebp.Options{Lossless: true, Method: libwebpSlow})},
		{"wasm", "lossy-fast", wasmEncoder(nrgba, gwebp.Options{Quality: lossyQuality, Method: libwebpFast})},
		{"wasm", "lossy-slow", wasmEncoder(nrgba, gwebp.Options{Quality: lossyQuality, Method: libwebpSlow})},
		{"nativewebp", "lossless", nativeEncoder(nrgba, nativewebp.BestCompression)},
	}
}

type sweepCase struct {
	engine, mode string
	effort       int
	fn           func() ([]byte, error)
}

// sweepCases walks each engine's own effort knob end to end, in both modes. The
// scales are not the same knob: ours is Effort 0..9 lossy and 0..6 lossless, libwebp's lossy knob is
// method 0..6 and its lossless one is the preset level 0..9 (cwebp's -z), and
// gen2brain/webp exposes only method, so the "wasm" engine sweeps method in both
// modes. The point of the pass is each engine's reachable time-for-size curve,
// so what matters is that every engine's endpoints are covered, not that the
// numbers line up across engines.
func sweepCases(buf *webp.Image) []sweepCase {
	nrgba := &image.NRGBA{Pix: buf.RGBA, Stride: buf.Width * 4, Rect: image.Rect(0, 0, buf.Width, buf.Height)}
	var cases []sweepCase
	for e := 0; e <= oursMaxEffort; e++ {
		effort := uint8(e)
		if e <= oursMaxEffortLL {
			cases = append(cases, sweepCase{"ours", "lossless", e, func() ([]byte, error) {
				return webp.EncodeLossless(buf, &webp.LosslessOptions{Effort: effort})
			}})
		}
		cases = append(cases, sweepCase{"ours", "lossy", e, func() ([]byte, error) {
			return webp.EncodeLossy(buf, &webp.LossyOptions{Quality: lossyQuality, Effort: effort})
		}})
	}
	for z := 0; z <= libwebpMaxLevel; z++ {
		cases = append(cases, sweepCase{"libwebp", "lossless", z, libwebpEncoder(nrgba, true, 0, z)})
	}
	for m := 0; m <= libwebpMaxMethod; m++ {
		cases = append(cases,
			sweepCase{"libwebp", "lossy", m, libwebpEncoder(nrgba, false, lossyQuality, m)},
			sweepCase{"wasm", "lossless", m, wasmEncoder(nrgba, gwebp.Options{Lossless: true, Method: m})},
			sweepCase{"wasm", "lossy", m, wasmEncoder(nrgba, gwebp.Options{Quality: lossyQuality, Method: m})},
		)
	}
	for _, lvl := range nativewebpLevels {
		cases = append(cases, sweepCase{"nativewebp", "lossless", lvl, nativeEncoder(nrgba, nativewebp.CompressionLevel(lvl))})
	}
	return cases
}

func main() {
	dir := flag.String("dir", "testdata/photos", "directory of source images (jpg/png)")
	budgetMs := flag.Int("budget-ms", 2000, "per-measurement time budget in ms")
	minIters := flag.Int("min-iters", 1, "minimum iterations per measurement")
	maxIters := flag.Int("max-iters", 500, "maximum iterations per measurement")
	header := flag.Bool("header", false, "print CSV header line")
	mem := flag.Bool("mem", false, "run the peak-RSS pass instead of the timing pass")
	decode := flag.Bool("decode", false, "run the decode pass instead of the encode pass")
	sweep := flag.Bool("sweep", false, "sweep every effort setting instead of the three fixed modes")
	engine := flag.String("engine", "", "restrict the sweep to one engine, for re-measuring it after a change")
	memOne := flag.String("mem-one", "", "internal: measure peak RSS of one `engine/mode` on -mem-file and exit")
	memFile := flag.String("mem-file", "", "internal: source image for -mem-one")
	flag.Parse()

	if *memOne != "" {
		if err := measureRSS(*memOne, *memFile); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}

	paths, err := listImages(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	if *mem {
		if *header {
			fmt.Println("engine,mode,file,width,height,megapixels,peak_rss_mib,mib_per_mp")
		}
		if err := memPass(paths); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}

	if *header {
		if *sweep {
			fmt.Println("engine,mode,effort,file,width,height,bytes,psnr_db,iters,ms_per_op")
		} else {
			fmt.Println("engine,mode,file,width,height,bytes,psnr_db,iters,ms_per_op")
		}
	}
	budget := time.Duration(*budgetMs) * time.Millisecond

	if *decode {
		decodePass(paths, budget, *minIters, *maxIters)
		return
	}

	if *sweep {
		sweepPass(paths, budget, *minIters, *maxIters, *engine)
		return
	}

	for _, p := range paths {
		buf, err := loadImageBuffer(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip %s: %v\n", p, err)
			continue
		}
		name := filepath.Base(p)

		for _, e := range encoderCases(&buf) {
			out, iters, perOp, err := measure(e.fn, budget, *minIters, *maxIters)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s/%s %s: %v\n", e.engine, e.mode, name, err)
				continue
			}
			quality := "-"
			if e.mode != "lossless" {
				dB, err := psnrOf(buf.RGBA, out)
				if err != nil {
					fmt.Fprintf(os.Stderr, "%s/%s %s: psnr: %v\n", e.engine, e.mode, name, err)
					continue
				}
				quality = fmt.Sprintf("%.2f", dB)
			}
			fmt.Printf("%s,%s,%s,%d,%d,%d,%s,%d,%.3f\n",
				e.engine, e.mode, name, buf.Width, buf.Height, len(out), quality, iters,
				float64(perOp.Microseconds())/1000.0)
		}
	}
}

// sweepPass measures every (engine, mode, effort) on every image, so each engine
// comes out as a curve rather than a point.
func sweepPass(paths []string, budget time.Duration, minIters, maxIters int, engine string) {
	for _, p := range paths {
		buf, err := loadImageBuffer(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip %s: %v\n", p, err)
			continue
		}
		name := filepath.Base(p)
		for _, e := range sweepCases(&buf) {
			if engine != "" && e.engine != engine {
				continue
			}
			out, iters, perOp, err := measure(e.fn, budget, minIters, maxIters)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s/%s/%d %s: %v\n", e.engine, e.mode, e.effort, name, err)
				continue
			}
			quality := "-"
			if e.mode != "lossless" {
				dB, err := psnrOf(buf.RGBA, out)
				if err != nil {
					fmt.Fprintf(os.Stderr, "%s/%s/%d %s: psnr: %v\n", e.engine, e.mode, e.effort, name, err)
					continue
				}
				quality = fmt.Sprintf("%.2f", dB)
			}
			fmt.Printf("%s,%s,%d,%s,%d,%d,%d,%s,%d,%.3f\n",
				e.engine, e.mode, e.effort, name, buf.Width, buf.Height, len(out), quality, iters,
				float64(perOp.Microseconds())/1000.0)
		}
	}
}

// memPass re-executes this binary once per (image, engine, mode) and forwards
// each child's CSV line. A fresh process per measurement keeps one engine's peak
// from being credited to another, and keeps wazero's runtime out of the figures
// for engines that never instantiate it.
func memPass(paths []string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	var probe webp.Image
	for _, p := range paths {
		for _, e := range encoderCases(&probe) {
			cmd := exec.Command(self, "-mem-one", e.engine+"/"+e.mode, "-mem-file", p)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "%s/%s %s: %v\n", e.engine, e.mode, filepath.Base(p), err)
			}
		}
	}
	return nil
}

// measureRSS runs a single encode and reports this process's peak RSS: the
// source bitmap and the runtime are included, since an application encoding an
// image pays for those too.
func measureRSS(engineMode, path string) error {
	engine, mode, ok := strings.Cut(engineMode, "/")
	if !ok {
		return fmt.Errorf("bad -mem-one %q, want engine/mode", engineMode)
	}
	buf, err := loadImageBuffer(path)
	if err != nil {
		return err
	}
	for _, e := range encoderCases(&buf) {
		if e.engine != engine || e.mode != mode {
			continue
		}
		if _, err := e.fn(); err != nil {
			return err
		}
		rss, err := maxRSSBytes()
		if err != nil {
			return err
		}
		mib := float64(rss) / (1 << 20)
		mp := float64(buf.Width*buf.Height) / 1e6
		fmt.Printf("%s,%s,%s,%d,%d,%.2f,%.1f,%.1f\n",
			engine, mode, filepath.Base(path), buf.Width, buf.Height, mp, mib, mib/mp)
		return nil
	}
	return fmt.Errorf("no such engine/mode %q", engineMode)
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

// nativeEncoder is lossless-only: nativewebp writes VP8L and nothing else, so it
// has no row in the lossy modes.
func nativeEncoder(img *image.NRGBA, level nativewebp.CompressionLevel) func() ([]byte, error) {
	return func() ([]byte, error) {
		var b bytes.Buffer
		opts := &nativewebp.Options{CompressionLevel: level}
		if err := nativewebp.Encode(&b, img, opts); err != nil {
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

func measure[T any](fn func() (T, error), budget time.Duration, minIters, maxIters int) (out T, iters int, perOp time.Duration, err error) {
	var zero T
	t0 := time.Now()
	out, err = fn() // warmup / first sample
	if err != nil {
		return zero, 0, 0, err
	}
	// If a single encode already exceeds the budget, report it as one iteration
	// instead of paying for the warmup plus a full timed loop.
	if warm := time.Since(t0); warm >= budget {
		return out, 1, warm, nil
	}

	start := time.Now()
	for {
		if _, err = fn(); err != nil {
			return zero, 0, 0, err
		}
		iters++
		elapsed := time.Since(start)
		if iters >= maxIters || (iters >= minIters && elapsed >= budget) {
			perOp = elapsed / time.Duration(iters)
			return out, iters, perOp, nil
		}
	}
}

// psnrOf decodes an encoded WebP and scores it against the source pixels the
// encoder was given, over RGB only.
func psnrOf(src []byte, encoded []byte) (float64, error) {
	dec, err := webp.Decode(encoded)
	if err != nil {
		return 0, err
	}
	return psnrBetween(src, dec.RGBA)
}

// psnrBetween scores two RGBA buffers against each other, over RGB only.
func psnrBetween(want, got []byte) (float64, error) {
	if len(got) != len(want) {
		return 0, fmt.Errorf("got %d bytes, want %d", len(got), len(want))
	}
	var sum float64
	n := 0
	for i := range want {
		if i%4 == 3 {
			continue
		}
		d := float64(want[i]) - float64(got[i])
		sum += d * d
		n++
	}
	if sum == 0 {
		return math.Inf(1), nil
	}
	return 10 * math.Log10(255*255/(sum/float64(n))), nil
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
