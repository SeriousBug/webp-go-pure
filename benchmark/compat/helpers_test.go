package compat

import (
	"os"
	"path/filepath"
	"testing"
)

func loadSample(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatalf("read sample %s: %v", name, err)
	}
	return data
}

func makeGradientRGBA(width, height int) []byte {
	rgba := make([]byte, width*height*4)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			o := (y*width + x) * 4
			rgba[o+0] = byte((x * 255) / max(1, width-1))
			rgba[o+1] = byte((y * 255) / max(1, height-1))
			rgba[o+2] = byte(((x + y) * 255) / max(1, width+height-2))
			rgba[o+3] = 255
		}
	}
	return rgba
}
