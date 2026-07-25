package webp

import (
	"image"
	"testing"
)

// The filter search scores levels by filtering the encoder's own
// reconstruction instead of decoding the frame back. If the two ever disagree
// the search silently picks a different filter level, so hold them equal
// directly.
func TestFilteredDistortionMatchesDecodedFrame(t *testing.T) {
	for _, size := range []struct{ width, height int }{{64, 48}, {97, 33}, {160, 144}} {
		src := syntheticPhoto(size.width, size.height)
		rgba := imageToRgba(src)

		mbWidth := (size.width + 15) >> 4
		mbHeight := (size.height + 15) >> 4
		baseQuant := elossyBaseQuantizerFromQuality(90)
		profile := elossySearchProfile(9)
		source := elossyRgbaToYuv420(size.width, size.height, rgba, mbWidth, mbHeight)

		segment := elossyDisabledSegmentConfig(mbWidth*mbHeight, elossyClippedQuantizer(baseQuant))
		scratch := elossyNewEncodeScratch(mbWidth, mbHeight, len(source.y)/4)
		candidate, err := elossyEncodeLossyCandidate(scratch, size.width, size.height, &source, mbWidth, mbHeight, &profile, &segment, nil)
		if err != nil {
			t.Fatalf("%dx%d: encode candidate: %v", size.width, size.height, err)
		}

		for _, filter := range elossyFilterCandidates(baseQuant) {
			vp8, err := elossyBuildCandidateVp8Frame(size.width, size.height, mbWidth, mbHeight, &candidate, &filter)
			if err != nil {
				t.Fatalf("%dx%d level %d: build frame: %v", size.width, size.height, filter.level, err)
			}
			want, err := elossyYuvSse(&source, size.width, size.height, vp8)
			if err != nil {
				t.Fatalf("%dx%d level %d: decode: %v", size.width, size.height, filter.level, err)
			}
			scratch := elossyNewFilterScratch(&candidate.reconstructed, mbWidth, mbHeight)
			got := elossyFilteredDistortion(&source, &candidate.reconstructed, &scratch, size.width, size.height, mbWidth, mbHeight, &filter, candidate.modes)
			if got != want {
				t.Errorf("%dx%d level %d: filtered distortion = %d, decoded = %d",
					size.width, size.height, filter.level, got, want)
			}
		}
	}
}

func syntheticPhoto(width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			offset := img.PixOffset(x, y)
			img.Pix[offset+0] = uint8((x*7 + y*3) % 256)
			img.Pix[offset+1] = uint8((x*x + y*5) % 256)
			img.Pix[offset+2] = uint8((x*2 ^ y*11) % 256)
			img.Pix[offset+3] = 0xff
		}
	}
	return img
}

func imageToRgba(img *image.RGBA) []byte {
	bounds := img.Bounds()
	out := make([]byte, bounds.Dx()*bounds.Dy()*4)
	for y := 0; y < bounds.Dy(); y++ {
		copy(out[y*bounds.Dx()*4:], img.Pix[y*img.Stride:y*img.Stride+bounds.Dx()*4])
	}
	return out
}

// elossyYuvSse scores a built frame by decoding it back, which is what the
// filter search used to do for every level. It survives as this test's oracle.
func elossyYuvSse(source *elossyPlanes, width, height int, vp8 []byte) (uint64, error) {
	decoded, err := decodeLossyVp8ToYuv(vp8)
	if err != nil {
		return 0, encBitstream("internal filter evaluation decode failed")
	}
	uvWidth := (width + 1) / 2
	uvHeight := (height + 1) / 2
	return elossyPlaneSseRegion(source.y, source.yStride, decoded.Y, decoded.YStride, width, height) +
		elossyPlaneSseRegion(source.u, source.uvStride, decoded.U, decoded.UVStride, uvWidth, uvHeight) +
		elossyPlaneSseRegion(source.v, source.uvStride, decoded.V, decoded.UVStride, uvWidth, uvHeight), nil
}
