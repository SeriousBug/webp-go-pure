package webp

import (
	"bytes"
	"image"
	"image/draw"
	"testing"

	ximage "golang.org/x/image/webp"
)

// These tests cross-check our codec against the independent pure-Go decoder
// golang.org/x/image/webp: our encoder output must be decodable by x/image, and
// our decoder must agree with x/image on the real sample files.

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

func ximageToRGBA(t *testing.T, data []byte) *image.RGBA {
	t.Helper()
	img, err := ximage.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("x/image decode: %v", err)
	}
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
	return dst
}

func TestXImageDecodesOurLosslessOutput(t *testing.T) {
	const w, h = 64, 48
	src := makeGradientRGBA(w, h)
	encoded, err := EncodeLosslessRgbaToWebp(w, h, src)
	if err != nil {
		t.Fatal(err)
	}
	dst := ximageToRGBA(t, encoded)
	if dst.Rect.Dx() != w || dst.Rect.Dy() != h {
		t.Fatalf("dims %dx%d", dst.Rect.Dx(), dst.Rect.Dy())
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := (y*w + x) * 4
			p := dst.PixOffset(x, y)
			for c := 0; c < 4; c++ {
				if dst.Pix[p+c] != src[o+c] {
					t.Fatalf("lossless pixel %d,%d chan %d: x/image=%d ours=%d", x, y, c, dst.Pix[p+c], src[o+c])
				}
			}
		}
	}
}

func TestXImageDecodesOurLossyOutput(t *testing.T) {
	const w, h = 64, 48
	src := makeGradientRGBA(w, h)
	encoded, err := EncodeLossyRgbaToWebp(w, h, src)
	if err != nil {
		t.Fatal(err)
	}
	dst := ximageToRGBA(t, encoded)
	if dst.Rect.Dx() != w || dst.Rect.Dy() != h {
		t.Fatalf("dims %dx%d", dst.Rect.Dx(), dst.Rect.Dy())
	}
	// Lossy plus independent YUV->RGB conversion in x/image: allow tolerance.
	const tol = 24
	for y := 0; y < h; y += 4 {
		for x := 0; x < w; x += 4 {
			o := (y*w + x) * 4
			p := dst.PixOffset(x, y)
			for c := 0; c < 3; c++ {
				if diff := int(dst.Pix[p+c]) - int(src[o+c]); diff > tol || diff < -tol {
					t.Fatalf("lossy pixel %d,%d chan %d: x/image=%d ours=%d (diff %d)", x, y, c, dst.Pix[p+c], src[o+c], diff)
				}
			}
		}
	}
}

func TestXImageAgreesWithOurDecoderOnLosslessSample(t *testing.T) {
	data := loadSample(t, "sample_lossless.webp")
	ours, err := DecodeLosslessWebpToRGBA(data)
	if err != nil {
		t.Fatal(err)
	}
	dst := ximageToRGBA(t, data)
	if dst.Rect.Dx() != ours.Width || dst.Rect.Dy() != ours.Height {
		t.Fatalf("dims x/image=%dx%d ours=%dx%d", dst.Rect.Dx(), dst.Rect.Dy(), ours.Width, ours.Height)
	}
	for _, p := range [][2]int{{0, 0}, {576, 448}, {1151, 895}, {123, 456}, {ours.Width - 1, ours.Height - 1}} {
		x, y := p[0], p[1]
		off := dst.PixOffset(x, y)
		oo := (y*ours.Width + x) * 4
		for c := 0; c < 4; c++ {
			if dst.Pix[off+c] != ours.RGBA[oo+c] {
				t.Fatalf("lossless sample %d,%d chan %d: x/image=%d ours=%d", x, y, c, dst.Pix[off+c], ours.RGBA[oo+c])
			}
		}
	}
}

func TestXImageAgreesWithOurDecoderOnLossySample(t *testing.T) {
	data := loadSample(t, "sample.webp")
	ours, err := DecodeLossyWebpToRGBA(data)
	if err != nil {
		t.Fatal(err)
	}
	dst := ximageToRGBA(t, data)
	if dst.Rect.Dx() != ours.Width || dst.Rect.Dy() != ours.Height {
		t.Fatalf("dims x/image=%dx%d ours=%dx%d", dst.Rect.Dx(), dst.Rect.Dy(), ours.Width, ours.Height)
	}
	// Two independent VP8 decoders differ in loop-filter and YUV->RGB rounding,
	// so compare statistically over a grid rather than per-pixel. A broken decode
	// would diverge by 100+; a faithful one stays within a few units on average.
	var sum, count, maxDiff int
	for y := 0; y < ours.Height; y += 7 {
		for x := 0; x < ours.Width; x += 7 {
			off := dst.PixOffset(x, y)
			oo := (y*ours.Width + x) * 4
			for c := 0; c < 3; c++ {
				d := int(dst.Pix[off+c]) - int(ours.RGBA[oo+c])
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
	if avg > 12 {
		t.Fatalf("mean abs diff vs x/image too high: %.2f", avg)
	}
	if maxDiff > 64 {
		t.Fatalf("max abs diff vs x/image too high: %d", maxDiff)
	}
}
