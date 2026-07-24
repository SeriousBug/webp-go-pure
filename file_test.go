package webp

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestImageFromFileDecodesStillWebp(t *testing.T) {
	sample := loadSample(t, "sample.webp")
	decoded, err := Decode(sample)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "sample.webp")
	if err := os.WriteFile(path, sample, 0o644); err != nil {
		t.Fatal(err)
	}
	image, err := DecodeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if image.Width != decoded.Width || image.Height != decoded.Height || !bytes.Equal(image.RGBA, decoded.RGBA) {
		t.Fatal("file decode differs")
	}
}
