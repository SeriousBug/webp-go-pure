//go:build testbenchmark

// These tests cross-check our pure-Go codec against libwebp (the C reference)
// through github.com/kolesa-team/go-webp. They require cgo and the libwebp C
// library plus pkg-config, so they are gated behind the test-only build tag
// `testbenchmark` and excluded from the default build. Run from this module with:
//
//	go test -tags testbenchmark ./...
//
// On macOS: brew install webp pkg-config.
package compat

import (
	"bytes"
	"image"
	"image/draw"
	"testing"

	"github.com/kolesa-team/go-webp/decoder"
	"github.com/kolesa-team/go-webp/encoder"
	kwebp "github.com/kolesa-team/go-webp/webp"

	webp "github.com/SeriousBug/webp-go-pure"
)

func rgbaImageFromBytes(width, height int, rgba []byte) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	copy(img.Pix, rgba)
	return img
}

func libwebpDecodeToRGBA(t *testing.T, data []byte) *image.RGBA {
	t.Helper()
	img, err := kwebp.Decode(bytes.NewReader(data), &decoder.Options{})
	if err != nil {
		t.Fatalf("libwebp decode: %v", err)
	}
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
	return dst
}

// Direction 1: our encoder -> libwebp decoder.

func TestLibwebpDecodesOurLosslessOutput(t *testing.T) {
	const w, h = 64, 48
	src := makeGradientRGBA(w, h)
	encoded, err := webp.EncodeLossless(&webp.Image{Width: w, Height: h, RGBA: src}, nil)
	if err != nil {
		t.Fatal(err)
	}
	dst := libwebpDecodeToRGBA(t, encoded)
	if dst.Rect.Dx() != w || dst.Rect.Dy() != h {
		t.Fatalf("dims %dx%d", dst.Rect.Dx(), dst.Rect.Dy())
	}
	for i := 0; i < w*h*4; i++ {
		if dst.Pix[i] != src[i] {
			t.Fatalf("lossless byte %d: libwebp=%d ours=%d", i, dst.Pix[i], src[i])
		}
	}
}

func TestLibwebpDecodesOurLossyOutput(t *testing.T) {
	const w, h = 64, 48
	src := makeGradientRGBA(w, h)
	encoded, err := webp.EncodeLossy(&webp.Image{Width: w, Height: h, RGBA: src}, nil)
	if err != nil {
		t.Fatal(err)
	}
	dst := libwebpDecodeToRGBA(t, encoded)
	if dst.Rect.Dx() != w || dst.Rect.Dy() != h {
		t.Fatalf("dims %dx%d", dst.Rect.Dx(), dst.Rect.Dy())
	}
	assertMeanAbsDiffSmall(t, dst.Pix, src, w, h, 16, 80)
}

// Every effort level takes a different path through the encoder: level 0 skips
// probability re-estimation entirely, levels 1 and up converge a coefficient
// probability table and replay the chosen levels under it, and level 9 adds an
// exhaustive segmentation search. A bitstream defect in any of those is
// invisible to our own decoder if the encoder and decoder share the mistaken
// assumption, so each is checked against the reference decoder.
func TestLibwebpDecodesOurLossyOutputAtEveryEffort(t *testing.T) {
	const w, h = 160, 144
	src := makeGradientRGBA(w, h)
	// Detail in part of the frame keeps macroblocks from all being skippable,
	// which is what makes the skip and token paths interesting.
	for y := 0; y < h; y++ {
		for x := w / 2; x < w; x++ {
			o := (y*w + x) * 4
			v := byte((x*31 ^ y*17) & 0xff)
			src[o], src[o+1], src[o+2] = v, v, v
		}
	}

	for effort := uint8(0); effort <= 9; effort++ {
		encoded, err := webp.EncodeLossy(&webp.Image{Width: w, Height: h, RGBA: src},
			&webp.LossyOptions{Quality: 90, Effort: effort})
		if err != nil {
			t.Fatalf("effort %d: %v", effort, err)
		}
		reference := libwebpDecodeToRGBA(t, encoded)
		ours, err := webp.Decode(encoded)
		if err != nil {
			t.Fatalf("effort %d: %v", effort, err)
		}
		if reference.Rect.Dx() != w || reference.Rect.Dy() != h {
			t.Fatalf("effort %d: dims %dx%d", effort, reference.Rect.Dx(), reference.Rect.Dy())
		}
		for i := 0; i < w*h*4; i++ {
			if reference.Pix[i] != ours.RGBA[i] {
				t.Fatalf("effort %d: byte %d decodes as %d in libwebp but %d here",
					effort, i, reference.Pix[i], ours.RGBA[i])
			}
		}
		assertMeanAbsDiffSmall(t, reference.Pix, src, w, h, 16, 80)
	}
}

// Direction 2: libwebp encoder -> our decoder.

func TestOurDecoderReadsLibwebpLosslessOutput(t *testing.T) {
	const w, h = 64, 48
	src := makeGradientRGBA(w, h)
	opts, err := encoder.NewLosslessEncoderOptions(encoder.PresetDefault, 9)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := kwebp.Encode(&buf, rgbaImageFromBytes(w, h, src), opts); err != nil {
		t.Fatalf("libwebp encode: %v", err)
	}
	decoded, err := webp.Decode(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Width != w || decoded.Height != h {
		t.Fatalf("dims %dx%d", decoded.Width, decoded.Height)
	}
	for i := 0; i < w*h*4; i++ {
		if decoded.RGBA[i] != src[i] {
			t.Fatalf("lossless byte %d: ours=%d src=%d", i, decoded.RGBA[i], src[i])
		}
	}
}

func TestOurDecoderReadsLibwebpLossyOutput(t *testing.T) {
	const w, h = 64, 48
	src := makeGradientRGBA(w, h)
	opts, err := encoder.NewLossyEncoderOptions(encoder.PresetDefault, 90)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := kwebp.Encode(&buf, rgbaImageFromBytes(w, h, src), opts); err != nil {
		t.Fatalf("libwebp encode: %v", err)
	}
	decoded, err := webp.Decode(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Width != w || decoded.Height != h {
		t.Fatalf("dims %dx%d", decoded.Width, decoded.Height)
	}
	assertMeanAbsDiffSmall(t, decoded.RGBA, src, w, h, 16, 80)
}

// Cross-decode the real sample files: libwebp vs our decoder.

func TestLibwebpAgreesWithOurDecoderOnLosslessSample(t *testing.T) {
	data := loadSample(t, "sample_lossless.webp")
	ours, err := webp.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	dst := libwebpDecodeToRGBA(t, data)
	if dst.Rect.Dx() != ours.Width || dst.Rect.Dy() != ours.Height {
		t.Fatalf("dims libwebp=%dx%d ours=%dx%d", dst.Rect.Dx(), dst.Rect.Dy(), ours.Width, ours.Height)
	}
	if !bytes.Equal(dst.Pix, ours.RGBA) {
		t.Fatal("lossless sample: our decode differs from libwebp")
	}
}

func TestLibwebpAgreesWithOurDecoderOnLossySample(t *testing.T) {
	data := loadSample(t, "sample.webp")
	ours, err := webp.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	dst := libwebpDecodeToRGBA(t, data)
	if dst.Rect.Dx() != ours.Width || dst.Rect.Dy() != ours.Height {
		t.Fatalf("dims libwebp=%dx%d ours=%dx%d", dst.Rect.Dx(), dst.Rect.Dy(), ours.Width, ours.Height)
	}
	assertMeanAbsDiffSmall(t, dst.Pix, ours.RGBA, ours.Width, ours.Height, 4, 32)
}

func assertMeanAbsDiffSmall(t *testing.T, a, b []byte, width, height int, maxAvg float64, maxSingle int) {
	t.Helper()
	var sum, count, maxDiff int
	step := 1
	if width*height > 100000 {
		step = 7
	}
	for y := 0; y < height; y += step {
		for x := 0; x < width; x += step {
			o := (y*width + x) * 4
			for c := 0; c < 3; c++ {
				d := int(a[o+c]) - int(b[o+c])
				if d < 0 {
					d = -d
				}
				sum += d
				count++
				if d > maxDiff {
					maxDiff = d
				}
			}
		}
	}
	avg := float64(sum) / float64(count)
	if avg > maxAvg {
		t.Fatalf("mean abs diff too high: %.2f (limit %.2f)", avg, maxAvg)
	}
	if maxDiff > maxSingle {
		t.Fatalf("max abs diff too high: %d (limit %d)", maxDiff, maxSingle)
	}
}
