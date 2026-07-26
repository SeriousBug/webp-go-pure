package webp

import (
	"math/rand"
	"testing"
)

// elossyRgbaToYuv420Reference is the straightforward per-pixel conversion the
// optimized elossyRgbaToYuv420 has to reproduce exactly, edge clamping and
// macroblock padding included.
func elossyRgbaToYuv420Reference(width, height int, rgba []byte, mbWidth, mbHeight int) elossyPlanes {
	yStride := mbWidth * 16
	uvStride := mbWidth * 8
	yHeight := mbHeight * 16
	uvHeight := mbHeight * 8
	y := make([]uint8, yStride*yHeight)
	u := make([]uint8, uvStride*uvHeight)
	v := make([]uint8, uvStride*uvHeight)

	for py := 0; py < yHeight; py++ {
		srcY := py
		if srcY > height-1 {
			srcY = height - 1
		}
		for px := 0; px < yStride; px++ {
			srcX := px
			if srcX > width-1 {
				srcX = width - 1
			}
			offset := (srcY*width + srcX) * 4
			y[py*yStride+px] = elossyRgbToY(rgba[offset], rgba[offset+1], rgba[offset+2])
		}
	}

	for py := 0; py < uvHeight; py++ {
		for px := 0; px < uvStride; px++ {
			var sumR, sumG, sumB int32
			for dy := 0; dy < 2; dy++ {
				srcY := py*2 + dy
				if srcY > height-1 {
					srcY = height - 1
				}
				for dx := 0; dx < 2; dx++ {
					srcX := px*2 + dx
					if srcX > width-1 {
						srcX = width - 1
					}
					offset := (srcY*width + srcX) * 4
					sumR += int32(rgba[offset])
					sumG += int32(rgba[offset+1])
					sumB += int32(rgba[offset+2])
				}
			}
			u[py*uvStride+px] = elossyRgbToU(sumR, sumG, sumB)
			v[py*uvStride+px] = elossyRgbToV(sumR, sumG, sumB)
		}
	}

	return elossyPlanes{yStride: yStride, uvStride: uvStride, y: y, u: u, v: v}
}

func TestElossyRgbaToYuv420MatchesReference(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	sizes := []struct{ width, height int }{
		{1, 1}, {1, 2}, {2, 1}, {3, 3}, {5, 7}, {15, 15}, {16, 16}, {17, 16},
		{16, 17}, {17, 17}, {31, 33}, {32, 32}, {33, 1}, {1, 33}, {64, 48}, {65, 49},
	}
	for _, size := range sizes {
		rgba := make([]byte, size.width*size.height*4)
		for i := range rgba {
			rgba[i] = byte(rng.Intn(256))
		}
		mbWidth := (size.width + 15) / 16
		mbHeight := (size.height + 15) / 16
		want := elossyRgbaToYuv420Reference(size.width, size.height, rgba, mbWidth, mbHeight)
		got := elossyRgbaToYuv420(size.width, size.height, rgba, mbWidth, mbHeight)
		for _, plane := range []struct {
			name       string
			got, want  []uint8
			stride     int
			planeWidth int
		}{
			{"y", got.y, want.y, got.yStride, mbWidth * 16},
			{"u", got.u, want.u, got.uvStride, mbWidth * 8},
			{"v", got.v, want.v, got.uvStride, mbWidth * 8},
		} {
			for i := range plane.want {
				if plane.got[i] != plane.want[i] {
					t.Fatalf("%dx%d plane %s index %d (row %d col %d): got %d want %d",
						size.width, size.height, plane.name, i, i/plane.stride, i%plane.stride,
						plane.got[i], plane.want[i])
				}
			}
		}
	}
}
