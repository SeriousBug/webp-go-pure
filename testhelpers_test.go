package webp

import (
	"os"
	"path/filepath"
	"testing"
)

func loadSample(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read sample %s: %v", name, err)
	}
	return data
}

func rgbaAt(rgba []byte, width, x, y int) [4]byte {
	offset := (y*width + x) * 4
	var out [4]byte
	copy(out[:], rgba[offset:offset+4])
	return out
}

func absDiffU8(a, b byte) byte {
	if a > b {
		return a - b
	}
	return b - a
}

func assertRGBAClose(t *testing.T, actual, expected [4]byte, tolerance byte) {
	t.Helper()
	for i := 0; i < 3; i++ {
		if diff := absDiffU8(actual[i], expected[i]); diff > tolerance {
			t.Fatalf("channel %d differs too much: actual=%d, expected=%d, tolerance=%d", i, actual[i], expected[i], tolerance)
		}
	}
	if actual[3] != expected[3] {
		t.Fatalf("alpha differs: actual=%d expected=%d", actual[3], expected[3])
	}
}

func le24(value int) [3]byte {
	return [3]byte{byte(value), byte(value >> 8), byte(value >> 16)}
}

func makeChunk(fourcc string, payload []byte) []byte {
	chunk := make([]byte, 0, 8+len(payload)+(len(payload)&1))
	chunk = append(chunk, fourcc...)
	n := uint32(len(payload))
	chunk = append(chunk, byte(n), byte(n>>8), byte(n>>16), byte(n>>24))
	chunk = append(chunk, payload...)
	if len(payload)&1 == 1 {
		chunk = append(chunk, 0)
	}
	return chunk
}

func wrapRiff(chunks ...[]byte) []byte {
	riffSize := 4
	for _, c := range chunks {
		riffSize += len(c)
	}
	data := make([]byte, 0, 8+riffSize)
	data = append(data, "RIFF"...)
	n := uint32(riffSize)
	data = append(data, byte(n), byte(n>>8), byte(n>>16), byte(n>>24))
	data = append(data, "WEBP"...)
	for _, c := range chunks {
		data = append(data, c...)
	}
	return data
}

func makeVp8xPayload(flags uint32, width, height int) []byte {
	payload := make([]byte, 0, 10)
	payload = append(payload, byte(flags), byte(flags>>8), byte(flags>>16), byte(flags>>24))
	w := le24(width - 1)
	h := le24(height - 1)
	payload = append(payload, w[:]...)
	payload = append(payload, h[:]...)
	return payload
}

func averageAbsDiff(a, b []byte) float64 {
	var sum float64
	for i := range a {
		d := int(a[i]) - int(b[i])
		if d < 0 {
			d = -d
		}
		sum += float64(d)
	}
	return sum / float64(len(a))
}
