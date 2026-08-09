package benchmark

import (
	"bytes"
	"image"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"sync"
	"testing"

	webp "github.com/SeriousBug/webp-go-pure"
	"github.com/kolesa-team/go-webp/encoder"
	kwebp "github.com/kolesa-team/go-webp/webp"
	ximage "golang.org/x/image/webp"
)

// The lossless decoders are compared on libwebp's own output, so neither engine
// is measured on a bitstream shaped by its own encoder. Point LLDIR at a
// directory of .webp files to run against those instead.
type llInput struct {
	name string
	data []byte
}

var (
	llOnce   sync.Once
	llCorpus []llInput
	llErr    error
)

func losslessCorpus(t testing.TB) []llInput {
	llOnce.Do(func() { llCorpus, llErr = buildLosslessCorpus() })
	if llErr != nil {
		t.Fatal(llErr)
	}
	return llCorpus
}

func buildLosslessCorpus() ([]llInput, error) {
	if dir := os.Getenv("LLDIR"); dir != "" {
		paths, err := filepath.Glob(filepath.Join(dir, "*.webp"))
		if err != nil {
			return nil, err
		}
		var out []llInput
		for _, p := range paths {
			data, err := os.ReadFile(p)
			if err != nil {
				return nil, err
			}
			out = append(out, llInput{filepath.Base(p), data})
		}
		return out, nil
	}

	paths, err := filepath.Glob("../testdata/photos/*")
	if err != nil {
		return nil, err
	}
	opts, err := encoder.NewLosslessEncoderOptions(encoder.PresetDefault, 6)
	if err != nil {
		return nil, err
	}
	var out []llInput
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		src, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			continue
		}
		b := src.Bounds()
		nrgba := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		draw.Draw(nrgba, nrgba.Bounds(), src, b.Min, draw.Src)
		var buf bytes.Buffer
		if err := kwebp.Encode(&buf, nrgba, opts); err != nil {
			return nil, err
		}
		out = append(out, llInput{filepath.Base(p), buf.Bytes()})
	}
	return out, nil
}

func BenchmarkLosslessOurs(b *testing.B) {
	for _, in := range losslessCorpus(b) {
		b.Run(in.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := webp.Decode(in.data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkLosslessXImage(b *testing.B) {
	for _, in := range losslessCorpus(b) {
		b.Run(in.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := ximage.Decode(bytes.NewReader(in.data)); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// TestLosslessMatchesXImage pins the decode output while the decoder is tuned:
// lossless is exact, so the two pure-Go decoders have to agree byte for byte.
func TestLosslessMatchesXImage(t *testing.T) {
	for _, in := range losslessCorpus(t) {
		got, err := webp.Decode(in.data)
		if err != nil {
			t.Errorf("%s: %v", in.name, err)
			continue
		}
		ref, err := ximage.Decode(bytes.NewReader(in.data))
		if err != nil {
			t.Errorf("%s: x/image: %v", in.name, err)
			continue
		}
		if !bytes.Equal(got.RGBA, toNRGBA(ref)) {
			t.Errorf("%s: pixels differ", in.name)
		}
	}
}

func toNRGBA(img image.Image) []byte {
	b := img.Bounds()
	if p, ok := img.(*image.NRGBA); ok && p.Stride == b.Dx()*4 && b.Min == (image.Point{}) {
		return p.Pix
	}
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
	return dst.Pix
}
