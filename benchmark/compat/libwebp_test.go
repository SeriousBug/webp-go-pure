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

// Lossy alpha: our ALPH chunk against libwebp, in both directions.

func makeAlphaRGBA(width, height int) []byte {
	rgba := makeGradientRGBA(width, height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			o := (y*width + x) * 4
			switch {
			case x < width/4:
				rgba[o+3] = 0
			case x < width/2:
				rgba[o+3] = byte((y * 255) / max(1, height-1))
			default:
				rgba[o+3] = 255
			}
		}
	}
	return rgba
}

// libwebpDecodeToNRGBA keeps straight alpha, which image.RGBA would premultiply
// away.
func libwebpDecodeToNRGBA(t *testing.T, data []byte) *image.NRGBA {
	t.Helper()
	img, err := kwebp.Decode(bytes.NewReader(data), &decoder.Options{})
	if err != nil {
		t.Fatalf("libwebp decode: %v", err)
	}
	b := img.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
	return dst
}

func TestLibwebpDecodesOurLossyAlphaOutput(t *testing.T) {
	const w, h = 64, 48
	src := makeAlphaRGBA(w, h)
	encoded, err := webp.EncodeLossy(&webp.Image{Width: w, Height: h, RGBA: src}, nil)
	if err != nil {
		t.Fatal(err)
	}
	dst := libwebpDecodeToNRGBA(t, encoded)
	if dst.Rect.Dx() != w || dst.Rect.Dy() != h {
		t.Fatalf("dims %dx%d", dst.Rect.Dx(), dst.Rect.Dy())
	}
	// Alpha is stored losslessly, so libwebp must read back every byte of it.
	for i := 3; i < w*h*4; i += 4 {
		if dst.Pix[i] != src[i] {
			t.Fatalf("alpha byte %d: libwebp=%d ours=%d", i, dst.Pix[i], src[i])
		}
	}
	assertMeanAbsDiffSmall(t, maskInvisibleColor(dst.Pix, src), src, w, h, 16, 80)
}

// The alpha plane takes a different path per effort the way the color plane
// does: elossyAlphaFilterCandidates trial-encodes every filter at the top
// efforts and shortlists by entropy below them, and elossyAlphaLosslessEffort
// hands the survivor to a different lossless effort. Each of those has to
// produce an ALPH chunk the reference decoder reads back exactly.
func TestLibwebpDecodesOurLossyAlphaOutputAtEveryEffort(t *testing.T) {
	const w, h = 160, 144
	src := makeAlphaRGBA(w, h)
	// Structure in the alpha plane itself, so the filters rank differently from
	// each other and a flat plane cannot hide a filter bug.
	for y := 0; y < h; y++ {
		for x := w / 2; x < w; x++ {
			o := (y*w + x) * 4
			v := byte((x*31 ^ y*17) & 0xff)
			src[o], src[o+1], src[o+2] = v, v, v
			src[o+3] = byte((x*13 + y*7) & 0xff)
		}
	}

	for effort := uint8(0); effort <= 9; effort++ {
		encoded, err := webp.EncodeLossy(&webp.Image{Width: w, Height: h, RGBA: src},
			&webp.LossyOptions{Quality: 90, Effort: effort})
		if err != nil {
			t.Fatalf("effort %d: %v", effort, err)
		}
		reference := libwebpDecodeToNRGBA(t, encoded)
		if reference.Rect.Dx() != w || reference.Rect.Dy() != h {
			t.Fatalf("effort %d: dims %dx%d", effort, reference.Rect.Dx(), reference.Rect.Dy())
		}
		ours, err := webp.Decode(encoded)
		if err != nil {
			t.Fatalf("effort %d: %v", effort, err)
		}
		if !bytes.Equal(reference.Pix, ours.RGBA) {
			t.Fatalf("effort %d: our decode differs from libwebp's on our own output", effort)
		}
		for i := 3; i < w*h*4; i += 4 {
			if reference.Pix[i] != src[i] {
				t.Fatalf("effort %d: alpha byte %d: libwebp=%d ours=%d",
					effort, i, reference.Pix[i], src[i])
			}
		}
		assertMeanAbsDiffSmall(t, maskInvisibleColor(reference.Pix, src), src, w, h, 16, 80)
	}
}

func TestOurDecoderReadsLibwebpLossyAlphaOutput(t *testing.T) {
	const w, h = 64, 48
	src := makeAlphaRGBA(w, h)
	opts, err := encoder.NewLossyEncoderOptions(encoder.PresetDefault, 90)
	if err != nil {
		t.Fatal(err)
	}
	nrgba := image.NewNRGBA(image.Rect(0, 0, w, h))
	copy(nrgba.Pix, src)

	var buf bytes.Buffer
	if err := kwebp.Encode(&buf, nrgba, opts); err != nil {
		t.Fatalf("libwebp encode: %v", err)
	}
	decoded, err := webp.Decode(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Width != w || decoded.Height != h {
		t.Fatalf("dims %dx%d", decoded.Width, decoded.Height)
	}
	for i := 3; i < w*h*4; i += 4 {
		if decoded.RGBA[i] != src[i] {
			t.Fatalf("alpha byte %d: ours=%d src=%d", i, decoded.RGBA[i], src[i])
		}
	}
	assertMeanAbsDiffSmall(t, maskInvisibleColor(decoded.RGBA, src), src, w, h, 16, 80)

	// Our color plane has to be the one libwebp reads out of the same
	// bitstream, not merely close to the input: the checks above pass on a
	// decoder that gets alpha right and the RGB under it wrong by a few steps.
	reference := libwebpDecodeToNRGBA(t, buf.Bytes())
	if !bytes.Equal(reference.Pix, decoded.RGBA) {
		t.Fatal("our decode of libwebp's lossy alpha output differs from libwebp's own")
	}
}

// maskInvisibleColor returns a copy of decoded with the color under fully
// transparent pixels replaced by the source's. Neither codec preserves it, so
// comparing it measures nothing.
func maskInvisibleColor(decoded, src []byte) []byte {
	masked := bytes.Clone(decoded)
	for i := 3; i < len(masked); i += 4 {
		if src[i] == 0 {
			copy(masked[i-3:i], src[i-3:i])
		}
	}
	return masked
}
