package webp

import (
	"math"
	"math/rand"
	"testing"
)

func alphaTestPlane(width, height int, fn func(x, y int) byte) []byte {
	plane := make([]byte, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			plane[y*width+x] = fn(x, y)
		}
	}
	return plane
}

func rgbaWithAlpha(width, height int, alpha []byte) []byte {
	rgba := make([]byte, width*height*4)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			i := (y*width + x) * 4
			rgba[i] = byte(x * 3)
			rgba[i+1] = byte(y * 5)
			rgba[i+2] = byte(x ^ y)
			rgba[i+3] = alpha[y*width+x]
		}
	}
	return rgba
}

func TestAlphaFilterRoundTrip(t *testing.T) {
	const width, height = 37, 21
	rng := rand.New(rand.NewSource(7))
	alpha := alphaTestPlane(width, height, func(x, y int) byte { return byte(rng.Intn(256)) })

	for filter := uint8(lossyAlphaFilterNone); filter <= lossyAlphaFilterGradient; filter++ {
		filtered := elossyFilterAlphaPlane(alpha, filter, width, height)
		decoded, err := lossyUnfilterAlpha(filtered, filter, width, height)
		if err != nil {
			t.Fatalf("filter %d: unfilter: %v", filter, err)
		}
		for i := range alpha {
			if decoded[i] != alpha[i] {
				t.Fatalf("filter %d: pixel %d: got %d want %d", filter, i, decoded[i], alpha[i])
			}
		}
	}
}

func TestAlphaChunkRoundTrip(t *testing.T) {
	const width, height = 40, 24
	cases := []struct {
		name  string
		plane []byte
	}{
		{"opaque", alphaTestPlane(width, height, func(x, y int) byte { return 0xff })},
		{"transparent", alphaTestPlane(width, height, func(x, y int) byte { return 0 })},
		{"gradient", alphaTestPlane(width, height, func(x, y int) byte { return byte((x*255)/(width-1)*1 + 0) })},
		{"partial", alphaTestPlane(width, height, func(x, y int) byte {
			if (x/4+y/4)%2 == 0 {
				return 0
			}
			return byte(128 + x%64)
		})},
		{"noise", func() []byte {
			rng := rand.New(rand.NewSource(11))
			return alphaTestPlane(width, height, func(x, y int) byte { return byte(rng.Intn(256)) })
		}()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := elossyEncodeAlphaChunk(width, height, tc.plane, 4)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			decoded, err := decodeAlphaPlane(payload, width, height)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			for i := range tc.plane {
				if decoded[i] != tc.plane[i] {
					t.Fatalf("pixel %d: got %d want %d", i, decoded[i], tc.plane[i])
				}
			}
		})
	}
}

// TestAlphaFilterSelection checks that each of the four filters wins on a plane
// built to suit it.
func TestAlphaFilterSelection(t *testing.T) {
	const width, height = 32, 32
	rng := rand.New(rand.NewSource(3))
	cases := []struct {
		name  string
		want  uint8
		plane []byte
	}{
		// Isolated impulses on a flat background: every differencing filter turns
		// each impulse into two nonzero samples instead of one.
		{"none", lossyAlphaFilterNone, func() []byte {
			plane := make([]byte, width*height)
			for i := 0; i < 20; i++ {
				plane[rng.Intn(width*height/4)*4+2] = byte(1 + rng.Intn(255))
			}
			return plane
		}()},
		// Rows are independent random walks, so only the left neighbour predicts.
		{"horizontal", lossyAlphaFilterHorizontal, func() []byte {
			plane := make([]byte, width*height)
			for y := 0; y < height; y++ {
				value := byte(rng.Intn(256))
				for x := 0; x < width; x++ {
					value += byte(rng.Intn(5)) - 2
					plane[y*width+x] = value
				}
			}
			return plane
		}()},
		// Every row is the same random pattern, so the row above predicts exactly.
		{"vertical", lossyAlphaFilterVertical, func() []byte {
			row := make([]byte, width)
			for x := range row {
				row[x] = byte(rng.Intn(256))
			}
			plane := make([]byte, width*height)
			for y := 0; y < height; y++ {
				copy(plane[y*width:(y+1)*width], row)
			}
			return plane
		}()},
		// Separable random walks in x and y: neither neighbour predicts on its
		// own, but left+top-topleft is exact.
		{"gradient", lossyAlphaFilterGradient, func() []byte {
			columns := make([]byte, width)
			rows := make([]byte, height)
			var value byte
			for x := range columns {
				value += byte(rng.Intn(41)) - 20
				columns[x] = value
			}
			value = 0
			for y := range rows {
				value += byte(rng.Intn(41)) - 20
				rows[y] = value
			}
			plane := make([]byte, width*height)
			for y := 0; y < height; y++ {
				for x := 0; x < width; x++ {
					plane[y*width+x] = columns[x] + rows[y]
				}
			}
			return plane
		}()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := elossyPickAlphaFilter(tc.plane, width, height)
			if got != tc.want {
				t.Fatalf("selected filter %d, want %d", got, tc.want)
			}
			// Below the exhaustive effort the chunk trials only filtering none
			// and the ranked filter, so the winner is one of the two.
			payload, err := elossyEncodeAlphaChunk(width, height, tc.plane, 2)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			header, err := parseAlphaHeader(payload)
			if err != nil {
				t.Fatalf("parse header: %v", err)
			}
			if header.Filter != tc.want && header.Filter != lossyAlphaFilterNone {
				t.Fatalf("chunk filter %d, want %d or none", header.Filter, tc.want)
			}
			decoded, err := decodeAlphaPlane(payload, width, height)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			for i := range tc.plane {
				if decoded[i] != tc.plane[i] {
					t.Fatalf("pixel %d: got %d want %d", i, decoded[i], tc.plane[i])
				}
			}
		})
	}
}

// TestAlphaRawBeatsCompressed covers the method-0 path: incompressible alpha
// must fall back to the raw form rather than growing.
func TestAlphaRawBeatsCompressed(t *testing.T) {
	const width, height = 64, 64
	rng := rand.New(rand.NewSource(5))
	plane := alphaTestPlane(width, height, func(x, y int) byte { return byte(rng.Intn(256)) })

	payload, err := elossyEncodeAlphaChunk(width, height, plane, 4)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	header, err := parseAlphaHeader(payload)
	if err != nil {
		t.Fatalf("parse header: %v", err)
	}
	if header.Compression != lossyAlphaNoCompression {
		t.Fatalf("compression %d, want raw", header.Compression)
	}
	if len(payload) != lossyAlphaHeaderLen+width*height {
		t.Fatalf("raw payload length %d, want %d", len(payload), lossyAlphaHeaderLen+width*height)
	}
}

func TestEncodeLossyWithAlphaRoundTrip(t *testing.T) {
	const width, height = 61, 43
	cases := []struct {
		name  string
		plane []byte
	}{
		{"opaque", alphaTestPlane(width, height, func(x, y int) byte { return 0xff })},
		{"transparent", alphaTestPlane(width, height, func(x, y int) byte { return 0 })},
		{"partial", alphaTestPlane(width, height, func(x, y int) byte {
			if x < width/3 {
				return 0
			}
			return byte(min(255, x*4))
		})},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rgba := rgbaWithAlpha(width, height, tc.plane)
			src := &Image{Width: width, Height: height, RGBA: rgba}
			encoded, err := EncodeLossy(src, &LossyOptions{Quality: 90, Effort: 2})
			if err != nil {
				t.Fatalf("encode: %v", err)
			}

			features, err := Features(encoded)
			if err != nil {
				t.Fatalf("features: %v", err)
			}
			opaque := elossyAlphaIsOpaque(tc.plane)
			if features.HasAlpha == opaque {
				t.Fatalf("HasAlpha=%v for opaque=%v", features.HasAlpha, opaque)
			}

			decoded, err := Decode(encoded)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if decoded.Width != width || decoded.Height != height {
				t.Fatalf("decoded %dx%d, want %dx%d", decoded.Width, decoded.Height, width, height)
			}
			for i, want := range tc.plane {
				if got := decoded.RGBA[i*4+3]; got != want {
					t.Fatalf("alpha at %d: got %d want %d", i, got, want)
				}
			}

			// Color is lossy, so score it where it is visible.
			var sum float64
			var count int
			for i := range tc.plane {
				if tc.plane[i] == 0 {
					continue
				}
				for c := 0; c < 3; c++ {
					d := float64(decoded.RGBA[i*4+c]) - float64(rgba[i*4+c])
					sum += d * d
					count++
				}
			}
			if count > 0 {
				psnr := 10 * math.Log10(255*255/(sum/float64(count)))
				if psnr < 30 {
					t.Fatalf("color PSNR %.2f dB is too low", psnr)
				}
			}
		})
	}
}

// TestEncodeLossyOpaqueStaysSimple keeps the container unchanged for opaque
// input: no VP8X, no ALPH.
func TestEncodeLossyOpaqueStaysSimple(t *testing.T) {
	const width, height = 16, 16
	plane := alphaTestPlane(width, height, func(x, y int) byte { return 0xff })
	src := &Image{Width: width, Height: height, RGBA: rgbaWithAlpha(width, height, plane)}
	encoded, err := EncodeLossy(src, &LossyOptions{Quality: 80, Effort: 0})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if string(encoded[12:16]) != "VP8 " {
		t.Fatalf("first chunk %q, want \"VP8 \"", encoded[12:16])
	}
}

func TestEncodeLossyYUVWithAlpha(t *testing.T) {
	const width, height = 34, 18
	plane := alphaTestPlane(width, height, func(x, y int) byte { return byte((x * 255) / (width - 1)) })

	uvWidth := (width + 1) / 2
	uvHeight := (height + 1) / 2
	img := &YUVImage{
		Width:    width,
		Height:   height,
		Y:        make([]byte, width*height),
		U:        make([]byte, uvWidth*uvHeight),
		V:        make([]byte, uvWidth*uvHeight),
		YStride:  width,
		UVStride: uvWidth,
		A:        plane,
		AStride:  width,
	}
	for i := range img.Y {
		img.Y[i] = byte(i % 200)
	}
	for i := range img.U {
		img.U[i] = 128
		img.V[i] = 128
	}

	encoded, err := EncodeLossyYUV(img, &LossyOptions{Quality: 90, Effort: 1})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	back, err := DecodeYUV(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if back.A == nil {
		t.Fatal("decoded image has no alpha plane")
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if got := back.A[y*back.AStride+x]; got != plane[y*width+x] {
				t.Fatalf("alpha at (%d,%d): got %d want %d", x, y, got, plane[y*width+x])
			}
		}
	}
}
