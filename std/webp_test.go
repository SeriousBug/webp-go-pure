package webp

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"testing"

	codec "github.com/SeriousBug/webp-go-pure"
)

func loadSample(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "testdata", name))
	if err != nil {
		t.Fatalf("read sample %s: %v", name, err)
	}
	return data
}

func gradient(width, height int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 4), G: uint8(y * 5), B: uint8((x ^ y) * 3), A: 0xff})
		}
	}
	return img
}

// decodeJPEG produces the *image.YCbCr that image/jpeg hands back, which is the
// input the planar encode path exists for.
func decodeJPEG(t *testing.T, src image.Image) *image.YCbCr {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: 92}); err != nil {
		t.Fatal(err)
	}
	decoded, err := jpeg.Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	ycbcr, ok := decoded.(*image.YCbCr)
	if !ok {
		t.Fatalf("image/jpeg returned %T, expected *image.YCbCr", decoded)
	}
	if ycbcr.SubsampleRatio != image.YCbCrSubsampleRatio420 {
		t.Fatalf("image/jpeg returned %v, expected 4:2:0", ycbcr.SubsampleRatio)
	}
	return ycbcr
}

func TestImportingThePackageRegistersTheFormat(t *testing.T) {
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

func TestSentinelsAreReExported(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	src.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 128})

	// Matching on the re-exported sentinel must work without importing the root
	// package, which is the reason it is re-exported.
	if _, err := EncodeBytes(src, nil); !errors.Is(err, ErrLossyAlpha) {
		t.Fatalf("expected ErrLossyAlpha, got %v", err)
	}
	if ErrLossyAlpha != codec.ErrLossyAlpha {
		t.Fatal("the re-exported sentinel is not the codec's own")
	}
}

func TestDecodeReturnsTheNativeRepresentation(t *testing.T) {
	cases := []struct {
		sample string
		want   string
	}{
		{"sample_lossy.webp", "*image.YCbCr"},
		{"sample_lossless.webp", "*image.NRGBA"},
	}
	for _, tc := range cases {
		t.Run(tc.sample, func(t *testing.T) {
			img, err := Decode(bytes.NewReader(loadSample(t, tc.sample)))
			if err != nil {
				t.Fatal(err)
			}
			if got := typeName(img); got != tc.want {
				t.Fatalf("decoded %s, want %s", got, tc.want)
			}
			if img.Bounds() != image.Rect(0, 0, 1920, 1080) {
				t.Fatalf("bounds %v", img.Bounds())
			}
		})
	}
}

func typeName(v any) string {
	switch v.(type) {
	case *image.YCbCr:
		return "*image.YCbCr"
	case *image.NYCbCrA:
		return "*image.NYCbCrA"
	case *image.NRGBA:
		return "*image.NRGBA"
	}
	return "unknown"
}

// image.Decode's contract is that DecodeConfig agrees with Decode. Disagreeing
// is a resource-exhaustion problem, not a cosmetic one.
func TestDecodeConfigAgreesWithDecode(t *testing.T) {
	for _, name := range []string{"sample_lossy.webp", "sample_lossless.webp", "sample_animation.webp"} {
		t.Run(name, func(t *testing.T) {
			data := loadSample(t, name)
			config, err := DecodeConfig(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			img, err := Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if config.Width != img.Bounds().Dx() || config.Height != img.Bounds().Dy() {
				t.Fatalf("config %dx%d, decoded %v", config.Width, config.Height, img.Bounds())
			}
			if config.ColorModel != img.ColorModel() {
				t.Fatalf("config model %T, decoded model %T", config.ColorModel, img.ColorModel())
			}
		})
	}
}

func TestDecodeConfigReadsOnlyAHeader(t *testing.T) {
	data := loadSample(t, "sample_lossy.webp")
	counting := &countingReader{data: data}
	config, err := DecodeConfig(counting)
	if err != nil {
		t.Fatal(err)
	}
	if config.Width != 1920 || config.Height != 1080 {
		t.Fatalf("config %dx%d", config.Width, config.Height)
	}
	if counting.read > 4096 {
		t.Fatalf("read %d bytes of a %d byte file to parse a header", counting.read, len(data))
	}
}

type countingReader struct {
	data []byte
	pos  int
	read int
}

func (r *countingReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	r.read += n
	return n, nil
}

func TestDecodeConfigRejectsTruncatedInput(t *testing.T) {
	data := loadSample(t, "sample_lossy.webp")
	_, err := DecodeConfig(bytes.NewReader(data[:8]))
	if err == nil {
		t.Fatal("expected an error for a truncated header")
	}
}

func TestDecodeConfigRejectsGarbage(t *testing.T) {
	_, err := DecodeConfig(bytes.NewReader([]byte("this is definitely not a webp file")))
	if err == nil {
		t.Fatal("expected an error for non-WebP input")
	}
}

// The point of the planar path: encoding an *image.YCbCr must reach the planar
// encoder, not the RGBA one.
func TestEncodeTakesThePlanarPathForYCbCr(t *testing.T) {
	src := decodeJPEG(t, gradient(64, 48))

	viaEncode, err := EncodeBytes(src, &Options{Quality: 80, Effort: 2})
	if err != nil {
		t.Fatal(err)
	}

	planes := &codec.YUVImage{
		Width: src.Rect.Dx(), Height: src.Rect.Dy(),
		Y: src.Y, U: src.Cb, V: src.Cr,
		YStride: src.YStride, UVStride: src.CStride,
	}
	viaPlanar, err := codec.EncodeLossyYUV(planes, &codec.LossyOptions{Quality: 80, Effort: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(viaEncode, viaPlanar) {
		t.Fatal("Encode did not produce what the planar encoder produces")
	}

	// And it must differ from the RGBA route, or the comparison above proves
	// nothing about which path ran.
	nrgba := image.NewNRGBA(src.Bounds())
	for y := 0; y < src.Rect.Dy(); y++ {
		for x := 0; x < src.Rect.Dx(); x++ {
			nrgba.Set(x, y, src.At(x, y))
		}
	}
	viaRGBA, err := EncodeBytes(nrgba, &Options{Quality: 80, Effort: 2})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(viaEncode, viaRGBA) {
		t.Fatal("the planar and RGBA paths produced identical bytes, so the fast path is not distinguishable")
	}
}

func TestEncodeJPEGTranscodeRoundTrips(t *testing.T) {
	original := gradient(64, 48)
	src := decodeJPEG(t, original)

	data, err := EncodeBytes(src, &Options{Quality: 95, Effort: 2})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds() != src.Bounds() {
		t.Fatalf("bounds %v, want %v", decoded.Bounds(), src.Bounds())
	}

	// The transcode should stay close to the JPEG it came from.
	var total, count float64
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			wantR, wantG, wantB, _ := src.At(x, y).RGBA()
			gotR, gotG, gotB, _ := decoded.At(x, y).RGBA()
			total += absDiff(wantR, gotR) + absDiff(wantG, gotG) + absDiff(wantB, gotB)
			count += 3
		}
	}
	if mean := total / count / 257; mean > 4 {
		t.Fatalf("average channel difference %.2f is too high for a transcode", mean)
	}
}

func absDiff(a, b uint32) float64 {
	if a > b {
		return float64(a - b)
	}
	return float64(b - a)
}

// image.RGBA is alpha-premultiplied. Handing its Pix to a straight-alpha
// encoder darkens everything that is not opaque.
func TestEncodeUnpremultipliesRGBA(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			src.Set(x, y, color.NRGBA{R: 255, G: 0, B: 0, A: 128})
		}
	}

	data, err := EncodeBytes(src, &Options{Lossless: true})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeBytes(data)
	if err != nil {
		t.Fatal(err)
	}

	got := color.NRGBAModel.Convert(decoded.At(1, 1)).(color.NRGBA)
	want := color.NRGBA{R: 255, G: 0, B: 0, A: 128}
	if got != want {
		t.Fatalf("got %v, want %v: premultiplied pixels leaked into the encoder", got, want)
	}
}

func TestEncodeLosslessRoundTripsExactly(t *testing.T) {
	src := gradient(32, 24)
	src.SetNRGBA(3, 4, color.NRGBA{R: 10, G: 20, B: 30, A: 90})

	data, err := EncodeBytes(src, &Options{Lossless: true})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeBytes(data)
	if err != nil {
		t.Fatal(err)
	}

	nrgba, ok := decoded.(*image.NRGBA)
	if !ok {
		t.Fatalf("decoded %T, want *image.NRGBA", decoded)
	}
	if !bytes.Equal(nrgba.Pix, src.Pix) {
		t.Fatal("lossless encoding did not round trip exactly")
	}
}

func TestEncodeRejectsAlphaForLossy(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	src.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 128})

	_, err := EncodeBytes(src, nil)
	if !errors.Is(err, codec.ErrLossyAlpha) {
		t.Fatalf("expected ErrLossyAlpha, got %v", err)
	}
}

// An opaque NYCbCrA carries no alpha information, so it should still reach the
// planar path rather than being rejected or converted.
func TestEncodeTakesThePlanarPathForOpaqueNYCbCrA(t *testing.T) {
	ycbcr := decodeJPEG(t, gradient(32, 24))
	opaque := &image.NYCbCrA{
		YCbCr:   *ycbcr,
		A:       bytes.Repeat([]byte{0xff}, 32*24),
		AStride: 32,
	}
	if !opaque.Opaque() {
		t.Fatal("test setup: expected an opaque image")
	}

	viaNYCbCrA, err := EncodeBytes(opaque, &Options{Quality: 80, Effort: 1})
	if err != nil {
		t.Fatal(err)
	}
	viaYCbCr, err := EncodeBytes(ycbcr, &Options{Quality: 80, Effort: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(viaNYCbCrA, viaYCbCr) {
		t.Fatal("an opaque NYCbCrA did not encode the same as its YCbCr")
	}
}

func TestEncodeRejectsTransparentNYCbCrA(t *testing.T) {
	ycbcr := decodeJPEG(t, gradient(32, 24))
	alpha := bytes.Repeat([]byte{0xff}, 32*24)
	alpha[0] = 0x40
	transparent := &image.NYCbCrA{YCbCr: *ycbcr, A: alpha, AStride: 32}

	_, err := EncodeBytes(transparent, nil)
	if !errors.Is(err, codec.ErrLossyAlpha) {
		t.Fatalf("expected ErrLossyAlpha, got %v", err)
	}
}

// A sub-image's planes carry the parent's strides, so this is the case that
// breaks if the encoder keys off the width instead.
func TestEncodeHandlesYCbCrSubImage(t *testing.T) {
	full := decodeJPEG(t, gradient(64, 48))
	sub := full.SubImage(image.Rect(16, 8, 48, 40)).(*image.YCbCr)

	data, err := EncodeBytes(sub, &Options{Quality: 95, Effort: 1})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := decoded.Bounds(); got.Dx() != 32 || got.Dy() != 32 {
		t.Fatalf("bounds %v, want 32x32", got)
	}

	// The crop's top-left pixel must match the same pixel in the full image.
	wantR, wantG, wantB, _ := sub.At(16, 8).RGBA()
	gotR, gotG, gotB, _ := decoded.At(0, 0).RGBA()
	for _, d := range []float64{absDiff(wantR, gotR), absDiff(wantG, gotG), absDiff(wantB, gotB)} {
		if d/257 > 12 {
			t.Fatalf("crop origin does not match the source pixel: want (%d,%d,%d) got (%d,%d,%d)",
				wantR>>8, wantG>>8, wantB>>8, gotR>>8, gotG>>8, gotB>>8)
		}
	}
}

// Non-4:2:0 chroma has no planar path, so it must fall back rather than fail or
// produce a misaligned image.
func TestEncodeFallsBackForNon420(t *testing.T) {
	src := image.NewYCbCr(image.Rect(0, 0, 16, 16), image.YCbCrSubsampleRatio444)
	for i := range src.Y {
		src.Y[i] = 128
	}
	for i := range src.Cb {
		src.Cb[i] = 100
		src.Cr[i] = 160
	}

	data, err := EncodeBytes(src, &Options{Quality: 90, Effort: 1})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds() != src.Bounds() {
		t.Fatalf("bounds %v, want %v", decoded.Bounds(), src.Bounds())
	}
}

func TestOptionsValidation(t *testing.T) {
	src := gradient(8, 8)
	cases := map[string]Options{
		"quality too high": {Quality: 101},
		"quality negative": {Quality: -5},
		"effort too high":  {Effort: 10},
		"effort negative":  {Effort: -2},
	}
	for name, opts := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := EncodeBytes(src, &opts); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestOptionsDefaults(t *testing.T) {
	src := gradient(16, 16)

	withNil, err := EncodeBytes(src, nil)
	if err != nil {
		t.Fatal(err)
	}
	withZero, err := EncodeBytes(src, &Options{})
	if err != nil {
		t.Fatal(err)
	}
	explicit, err := EncodeBytes(src, &Options{Quality: DefaultQuality, Effort: EffortFastest})
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(withNil, withZero) {
		t.Fatal("a nil Options and a zero Options disagree")
	}
	if !bytes.Equal(withNil, explicit) {
		t.Fatal("the documented defaults do not match what nil Options produces")
	}
}

func TestDecodeAllReadsEveryFrame(t *testing.T) {
	anim, err := DecodeAll(bytes.NewReader(loadSample(t, "sample_animation.webp")))
	if err != nil {
		t.Fatal(err)
	}
	if len(anim.Image) != 3 {
		t.Fatalf("got %d frames, want 3", len(anim.Image))
	}
	if len(anim.Delay) != len(anim.Image) {
		t.Fatalf("%d delays for %d frames", len(anim.Delay), len(anim.Image))
	}
	if anim.Config.Width != 1920 || anim.Config.Height != 1080 {
		t.Fatalf("config %dx%d", anim.Config.Width, anim.Config.Height)
	}
	for i, frame := range anim.Image {
		if frame.Bounds() != image.Rect(0, 0, 1920, 1080) {
			t.Fatalf("frame %d bounds %v", i, frame.Bounds())
		}
	}
}

func TestDecodeAllAcceptsAStillImage(t *testing.T) {
	anim, err := DecodeAll(bytes.NewReader(loadSample(t, "sample_lossy.webp")))
	if err != nil {
		t.Fatal(err)
	}
	if len(anim.Image) != 1 {
		t.Fatalf("got %d frames, want 1", len(anim.Image))
	}
}

func TestDecodeReturnsFirstAnimationFrame(t *testing.T) {
	img, err := Decode(bytes.NewReader(loadSample(t, "sample_animation.webp")))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != image.Rect(0, 0, 1920, 1080) {
		t.Fatalf("bounds %v", img.Bounds())
	}
}
