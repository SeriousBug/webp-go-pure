package webp

import (
	"bytes"
	"errors"
	"math"
	"testing"
)

func sampleRGBA() (int, int, []byte) {
	width, height := 3, 2
	rgba := []byte{
		0xff, 0x00, 0x00, 0xff, 0x00, 0xff, 0x00, 0x80, 0x00, 0x00, 0xff, 0x40, 0xff, 0xff, 0xff,
		0xff, 0x22, 0x44, 0x66, 0x00, 0x80, 0x20, 0xc0, 0xfe,
	}
	return width, height, rgba
}

func lossySampleRGBA() (int, int, []byte) {
	width, height := 19, 17
	rgba := make([]byte, width*height*4)
	satMul := func(a byte, b byte) byte {
		v := int(a) * int(b)
		if v > 255 {
			v = 255
		}
		return byte(v)
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			offset := (y*width + x) * 4
			rgba[offset] = satMul(byte(x), 12)
			rgba[offset+1] = satMul(byte(y), 13)
			rgba[offset+2] = satMul(byte(x+y), 7)
			rgba[offset+3] = 0xff
		}
	}
	return width, height, rgba
}

func assertInvalidParam(t *testing.T, err error, msg string) {
	t.Helper()
	var e *EncoderError
	if !errors.As(err, &e) {
		t.Fatalf("expected *EncoderError, got %v", err)
	}
	if e.Kind != EncErrInvalidParam || e.Msg != msg {
		t.Fatalf("expected InvalidParam(%q), got kind=%d msg=%q", msg, e.Kind, e.Msg)
	}
}

func TestEncodeLosslessRgbaToVp8lRoundTripsPixels(t *testing.T) {
	width, height, rgba := sampleRGBA()
	vp8l, err := encodeLosslessRgbaToVp8l(width, height, rgba)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeLosslessVp8lToRGBA(vp8l)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Width != width || decoded.Height != height || !bytes.Equal(decoded.RGBA, rgba) {
		t.Fatal("round trip mismatch")
	}
}

func TestEncodeLosslessRgbaToWebpRoundTripsPixels(t *testing.T) {
	width, height, rgba := sampleRGBA()
	webp, err := encodeLosslessRgbaToWebp(width, height, rgba)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(webp)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Width != width || decoded.Height != height || !bytes.Equal(decoded.RGBA, rgba) {
		t.Fatal("round trip mismatch")
	}
}

func TestEncodeLosslessRgbaToWebpRoundTripsAtOptLevelZero(t *testing.T) {
	width, height, rgba := sampleRGBA()
	options := LosslessOptions{Effort: 0}
	webp, err := encodeLosslessRgbaToWebpWithOptions(width, height, rgba, &options)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(webp)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Width != width || decoded.Height != height || !bytes.Equal(decoded.RGBA, rgba) {
		t.Fatal("round trip mismatch")
	}
}

func TestEncodeLosslessImageToWebpSetsLosslessFeatures(t *testing.T) {
	width, height, rgba := sampleRGBA()
	image := &Image{Width: width, Height: height, RGBA: rgba}
	webp, err := encodeLosslessImageToWebp(image)
	if err != nil {
		t.Fatal(err)
	}
	features, err := Features(webp)
	if err != nil {
		t.Fatal(err)
	}
	if features.Width != width || features.Height != height || features.Format != FormatLossless || !features.HasAlpha {
		t.Fatal("features")
	}
}

func TestEncodeLosslessRgbaToWebpRejectsMismatchedBufferLength(t *testing.T) {
	_, err := encodeLosslessRgbaToWebp(2, 2, make([]byte, 15))
	assertInvalidParam(t, err, "RGBA buffer length does not match dimensions")
}

func TestEncodeLosslessRgbaToWebpRejectsInvalidOptimizationLevel(t *testing.T) {
	_, err := encodeLosslessRgbaToWebpWithOptions(1, 1, []byte{0, 0, 0, 0xff}, &LosslessOptions{Effort: 10})
	assertInvalidParam(t, err, "lossless optimization level must be in 0..=9")
}

func TestEncodeLosslessRgbaToWebpCompressesFlatRuns(t *testing.T) {
	width, height := 64, 64
	rgba := make([]byte, width*height*4)
	for i := 0; i+4 <= len(rgba); i += 4 {
		copy(rgba[i:i+4], []byte{0x12, 0x34, 0x56, 0xff})
	}
	webp, err := encodeLosslessRgbaToWebp(width, height, rgba)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(webp)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded.RGBA, rgba) {
		t.Fatal("round trip mismatch")
	}
	if len(webp) >= 200 {
		t.Fatalf("unexpected flat-image size: %d", len(webp))
	}
}

func TestEncodeLosslessHigherOptimizationHelpsRepeatedTiles(t *testing.T) {
	width, height := 64, 64
	satMul := func(a byte, b byte) byte {
		v := int(a) * int(b)
		if v > 255 {
			v = 255
		}
		return byte(v)
	}
	rgba := make([]byte, width*height*4)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			sx := byte(x % 8)
			sy := byte(y % 8)
			offset := (y*width + x) * 4
			rgba[offset] = satMul(sx, 29)
			rgba[offset+1] = satMul(sy, 31)
			rgba[offset+2] = satMul(sx^sy, 17)
			rgba[offset+3] = 0xff
		}
	}
	opt0, err := encodeLosslessRgbaToWebpWithOptions(width, height, rgba, &LosslessOptions{Effort: 0})
	if err != nil {
		t.Fatal(err)
	}
	opt6, err := encodeLosslessRgbaToWebpWithOptions(width, height, rgba, &LosslessOptions{Effort: 6})
	if err != nil {
		t.Fatal(err)
	}
	if d, _ := Decode(opt0); !bytes.Equal(d.RGBA, rgba) {
		t.Fatal("opt0 mismatch")
	}
	if d, _ := Decode(opt6); !bytes.Equal(d.RGBA, rgba) {
		t.Fatal("opt6 mismatch")
	}
	if len(opt6) >= len(opt0) {
		t.Fatalf("expected opt6 to beat opt0: opt0=%d opt6=%d", len(opt0), len(opt6))
	}
}

func TestEncodeLosslessRoundTripsAtOptLevelNine(t *testing.T) {
	width, height, rgba := sampleRGBA()
	webp, err := encodeLosslessRgbaToWebpWithOptions(width, height, rgba, &LosslessOptions{Effort: 9})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(webp)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded.RGBA, rgba) {
		t.Fatal("round trip mismatch")
	}
}

func TestEncodeLosslessPaletteImageRoundTripsAndCompresses(t *testing.T) {
	width, height := 32, 32
	colors := [][4]byte{
		{0x00, 0x00, 0x00, 0xff},
		{0xff, 0x00, 0x00, 0xff},
		{0x00, 0xff, 0x00, 0xff},
		{0x00, 0x00, 0xff, 0xff},
	}
	rgba := make([]byte, width*height*4)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			color := colors[((x/4)+(y/4))%len(colors)]
			offset := (y*width + x) * 4
			copy(rgba[offset:offset+4], color[:])
		}
	}
	vp8l, err := encodeLosslessRgbaToVp8l(width, height, rgba)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeLosslessVp8lToRGBA(vp8l)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded.RGBA, rgba) {
		t.Fatal("round trip mismatch")
	}
	if len(vp8l) >= len(rgba) {
		t.Fatal("expected compression")
	}
}

func TestEncodeLossyRgbaToVp8RoundTripsAsLossyFrame(t *testing.T) {
	width, height, rgba := lossySampleRGBA()
	vp8, err := encodeLossyRgbaToVp8(width, height, rgba)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeLossyVp8ToRGBA(vp8)
	if err != nil {
		t.Fatal(err)
	}
	diff := averageAbsDiff(decoded.RGBA, rgba)
	if decoded.Width != width || decoded.Height != height {
		t.Fatal("dims")
	}
	if diff >= 26.0 {
		t.Fatalf("avg diff: %f", diff)
	}
}

func TestEncodeLossyRgbaToWebpSetsLossyFeatures(t *testing.T) {
	width, height, rgba := lossySampleRGBA()
	options := elossyDefaultOptions()
	options.Quality = 90
	webp, err := encodeLossyRgbaToWebpWithOptions(width, height, rgba, &options)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(webp)
	if err != nil {
		t.Fatal(err)
	}
	features, err := Features(webp)
	if err != nil {
		t.Fatal(err)
	}
	diff := averageAbsDiff(decoded.RGBA, rgba)
	if decoded.Width != width || decoded.Height != height {
		t.Fatal("dims")
	}
	if diff >= 26.0 {
		t.Fatalf("avg diff: %f", diff)
	}
	if features.Format != FormatLossy || features.HasAlpha {
		t.Fatal("features")
	}
}

func TestEncodeLossyRgbaToVp8MarksFlatMacroblocksAsSkip(t *testing.T) {
	width, height := 64, 64
	rgba := make([]byte, width*height*4)
	for i := 0; i+4 <= len(rgba); i += 4 {
		copy(rgba[i:i+4], []byte{0x80, 0x80, 0x80, 0xff})
	}
	vp8, err := encodeLossyRgbaToVp8(width, height, rgba)
	if err != nil {
		t.Fatal(err)
	}
	headers, err := parseMacroblockHeaders(vp8)
	if err != nil {
		t.Fatal(err)
	}
	anySkip := false
	for _, h := range headers.Macroblocks {
		if h.Skip {
			anySkip = true
			break
		}
	}
	if !anySkip {
		t.Fatal("expected at least one skipped macroblock")
	}
}

// TestEncodeLossyRareSkipsRoundTrip guards against a token-partition desync:
// when only a few macroblocks are skippable, skip signaling must still be
// enabled, otherwise the encoder omits those macroblocks' coefficient tokens
// without the decoder expecting it. Uses a mostly-detailed image (rare skips)
// at high effort and checks the decoded result matches the encoder.
func TestEncodeLossyRareSkipsRoundTrip(t *testing.T) {
	width, height := 256, 256
	rgba := make([]byte, width*height*4)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			o := (y*width + x) * 4
			// One flat (skippable) macroblock at (0,0); high-frequency detail
			// everywhere else so almost every macroblock is non-skip.
			var v byte = 0x80
			if x >= 16 || y >= 16 {
				v = byte((x*7 ^ y*13) & 0xff)
			}
			rgba[o] = v
			rgba[o+1] = v
			rgba[o+2] = v
			rgba[o+3] = 0xff
		}
	}
	enc, err := EncodeLossy(&Image{Width: width, Height: height, RGBA: rgba}, &LossyOptions{Quality: 90, Effort: 9})
	if err != nil {
		t.Fatal(err)
	}
	dec, err := Decode(enc)
	if err != nil {
		t.Fatal(err)
	}
	var mse float64
	n := 0
	for i := 0; i < len(rgba); i++ {
		if i%4 == 3 {
			continue
		}
		d := float64(rgba[i]) - float64(dec.RGBA[i])
		mse += d * d
		n++
	}
	mse /= float64(n)
	psnr := 99.0
	if mse > 0 {
		psnr = 10 * math.Log10(255*255/mse)
	}
	if psnr < 30 {
		t.Fatalf("decoded PSNR too low (%.2f): token partition likely desynced", psnr)
	}
}

func TestEncodeLossyImageToWebpAcceptsOpaqueImageBuffer(t *testing.T) {
	width, height, rgba := lossySampleRGBA()
	image := &Image{Width: width, Height: height, RGBA: rgba}
	webp, err := encodeLossyImageToWebp(image)
	if err != nil {
		t.Fatal(err)
	}
	features, err := Features(webp)
	if err != nil {
		t.Fatal(err)
	}
	if features.Width != width || features.Height != height || features.Format != FormatLossy {
		t.Fatal("features")
	}
}

func TestEncodeLossyRgbaToWebpRejectsAlphaInput(t *testing.T) {
	options := elossyDefaultOptions()
	_, err := encodeLossyRgbaToWebpWithOptions(1, 1, []byte{0, 0, 0, 0x7f}, &options)
	assertInvalidParam(t, err, "lossy encoder does not support alpha yet")
}

func TestEncodeLossyRgbaToWebpRejectsInvalidQuality(t *testing.T) {
	options := elossyDefaultOptions()
	options.Quality = 101
	_, err := encodeLossyRgbaToWebpWithOptions(1, 1, []byte{0, 0, 0, 0xff}, &options)
	assertInvalidParam(t, err, "lossy quality must be in 0..=100")
}

func TestEncodeLossyRgbaToWebpRejectsInvalidOptimizationLevel(t *testing.T) {
	options := elossyDefaultOptions()
	options.Effort = 10
	_, err := encodeLossyRgbaToWebpWithOptions(1, 1, []byte{0, 0, 0, 0xff}, &options)
	assertInvalidParam(t, err, "lossy optimization level must be in 0..=9")
}

func TestTopLevelEncodeLosslessRoundTrips(t *testing.T) {
	width, height, rgba := sampleRGBA()
	image := &Image{Width: width, Height: height, RGBA: rgba}
	webp, err := EncodeLossless(image, &LosslessOptions{Effort: 2})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(webp)
	if err != nil {
		t.Fatal(err)
	}
	features, err := Features(webp)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded.RGBA, rgba) || features.Format != FormatLossless {
		t.Fatal("mismatch")
	}
}

func TestTopLevelEncodeLossyUsesRequestedCompression(t *testing.T) {
	width, height, rgba := lossySampleRGBA()
	image := &Image{Width: width, Height: height, RGBA: rgba}
	webp, err := EncodeLossy(image, &LossyOptions{Quality: 90, Effort: 0})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(webp)
	if err != nil {
		t.Fatal(err)
	}
	features, err := Features(webp)
	if err != nil {
		t.Fatal(err)
	}
	diff := averageAbsDiff(decoded.RGBA, rgba)
	if features.Format != FormatLossy {
		t.Fatal("format")
	}
	if diff >= 26.0 {
		t.Fatalf("avg diff: %f", diff)
	}
}

func TestTopLevelEncodeVariantsEmbedExifChunk(t *testing.T) {
	width, height, rgba := sampleRGBA()
	image := &Image{Width: width, Height: height, RGBA: rgba}
	exif := []byte("Exif\x00\x00unit-test")

	lossless, err := EncodeLossless(image, &LosslessOptions{Effort: 2, EXIF: exif})
	if err != nil {
		t.Fatal(err)
	}
	opaque := make([]byte, len(image.RGBA))
	copy(opaque, image.RGBA)
	for i := 3; i < len(opaque); i += 4 {
		opaque[i] = 0xff
	}
	lossy, err := EncodeLossy(&Image{Width: image.Width, Height: image.Height, RGBA: opaque}, &LossyOptions{Quality: 90, Effort: 0, EXIF: exif})
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Contains(lossless, []byte("EXIF")) || !bytes.Contains(lossless, exif) {
		t.Fatal("lossless missing exif")
	}
	if !bytes.Contains(lossy, []byte("EXIF")) || !bytes.Contains(lossy, exif) {
		t.Fatal("lossy missing exif")
	}
}
