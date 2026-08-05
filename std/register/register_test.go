package register_test

import (
	"bytes"
	"image"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/SeriousBug/webp-go-pure/std/register"
)

func loadSample(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatalf("read sample %s: %v", name, err)
	}
	return data
}

func TestImageDecodeRecognizesWebP(t *testing.T) {
	for _, name := range []string{"sample_lossy.webp", "sample_lossless.webp", "sample_animation.webp"} {
		t.Run(name, func(t *testing.T) {
			data := loadSample(t, name)

			img, format, err := image.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if format != "webp" {
				t.Fatalf("format %q, want webp", format)
			}
			if img.Bounds() != image.Rect(0, 0, 1920, 1080) {
				t.Fatalf("bounds %v", img.Bounds())
			}

			config, format, err := image.DecodeConfig(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if format != "webp" {
				t.Fatalf("config format %q, want webp", format)
			}
			if config.Width != 1920 || config.Height != 1080 {
				t.Fatalf("config %dx%d", config.Width, config.Height)
			}
		})
	}
}
