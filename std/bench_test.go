package webpstd

import (
	"os"
	"path/filepath"
	"testing"

	webp "github.com/SeriousBug/webp-go-pure"
)

func benchSample(b *testing.B) []byte {
	b.Helper()
	data, err := os.ReadFile(filepath.Join("..", "testdata", "sample_lossy.webp"))
	if err != nil {
		b.Fatal(err)
	}
	return data
}

// The two halves of a transcode, measured against the byte-oriented API doing
// the same work through RGBA. This is what the planar fast paths buy.

func BenchmarkDecodePlanar(b *testing.B) {
	data := benchSample(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := DecodeBytes(data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeRGBA(b *testing.B) {
	data := benchSample(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := webp.Decode(data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTranscodePlanar(b *testing.B) {
	data := benchSample(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		img, err := DecodeBytes(data)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := EncodeBytes(img, &Options{Quality: 80}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTranscodeRGBA(b *testing.B) {
	data := benchSample(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		img, err := webp.Decode(data)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := webp.EncodeLossy(&img, &webp.LossyOptions{Quality: 80}); err != nil {
			b.Fatal(err)
		}
	}
}
