package webp

import "testing"

// The endpoints are the whole point of the conversion: limited range puts black
// at 16 and white at 235, full range at 0 and 255.
func TestRangeTableEndpoints(t *testing.T) {
	cases := []struct {
		name  string
		table *[256]uint8
		in    uint8
		want  uint8
	}{
		{"luma full black to limited", lumaFullToLimited, 0, 16},
		{"luma full white to limited", lumaFullToLimited, 255, 235},
		{"luma limited black to full", lumaLimitedToFull, 16, 0},
		{"luma limited white to full", lumaLimitedToFull, 235, 255},
		{"luma below limited black clamps", lumaLimitedToFull, 0, 0},
		{"luma above limited white clamps", lumaLimitedToFull, 255, 255},
		{"chroma neutral is fixed", chromaFullToLimited, 128, 128},
		{"chroma neutral is fixed back", chromaLimitedToFull, 128, 128},
		{"chroma full low to limited", chromaFullToLimited, 0, 16},
		{"chroma full high to limited", chromaFullToLimited, 255, 240},
	}
	for _, tc := range cases {
		if got := tc.table[tc.in]; got != tc.want {
			t.Errorf("%s: %d became %d, want %d", tc.name, tc.in, got, tc.want)
		}
	}
}

// Limited range has fewer codes than full range, so a full round trip cannot be
// exact. It should still land next door rather than drift.
func TestConvertRangeRoundTripStaysClose(t *testing.T) {
	const size = 16
	img := YUVImage{
		Width: size, Height: size,
		Y: make([]byte, size*size), U: make([]byte, size*size/4), V: make([]byte, size*size/4),
		YStride: size, UVStride: size / 2,
	}
	for i := range img.Y {
		img.Y[i] = uint8(i)
	}
	for i := range img.U {
		img.U[i] = uint8(i * 4)
		img.V[i] = uint8(255 - i*4)
	}
	original := YUVImage{Y: append([]byte(nil), img.Y...), U: append([]byte(nil), img.U...), V: append([]byte(nil), img.V...)}

	img.ConvertRange(RangeFull)
	if img.Range != RangeFull {
		t.Fatal("ConvertRange did not update Range")
	}
	img.ConvertRange(RangeLimited)

	for i, want := range original.Y {
		// Values outside the limited-range window have nowhere to come back
		// from, so only the representable ones are checked.
		if want < 16 || want > 235 {
			continue
		}
		if diff := int(img.Y[i]) - int(want); diff > 1 || diff < -1 {
			t.Fatalf("Y[%d] round tripped %d to %d", i, want, img.Y[i])
		}
	}
	for i, want := range original.U {
		if want < 16 || want > 240 {
			continue
		}
		if diff := int(img.U[i]) - int(want); diff > 1 || diff < -1 {
			t.Fatalf("U[%d] round tripped %d to %d", i, want, img.U[i])
		}
	}
}

func TestConvertRangeIsANoOpWhenRangeMatches(t *testing.T) {
	img := YUVImage{
		Width: 2, Height: 2,
		Y: []byte{0, 255, 0, 255}, U: []byte{0}, V: []byte{255},
		YStride: 2, UVStride: 1,
	}
	img.ConvertRange(RangeLimited)
	if img.Y[0] != 0 || img.Y[1] != 255 {
		t.Fatalf("planes changed: %v", img.Y)
	}
}

// A full-range source has to be rescaled on the way into the encoder, so the
// same bytes labelled differently must not encode identically.
func TestEncodeLossyYUVHonorsRange(t *testing.T) {
	const size = 32
	planes := func(r ColorRange) *YUVImage {
		img := &YUVImage{
			Width: size, Height: size,
			Y: make([]byte, size*size), U: make([]byte, size*size/4), V: make([]byte, size*size/4),
			YStride: size, UVStride: size / 2, Range: r,
		}
		for i := range img.Y {
			img.Y[i] = uint8(i % 256)
		}
		for i := range img.U {
			img.U[i] = 128
			img.V[i] = 128
		}
		return img
	}

	limited, err := EncodeLossyYUV(planes(RangeLimited), &LossyOptions{Quality: 90})
	if err != nil {
		t.Fatal(err)
	}
	full, err := EncodeLossyYUV(planes(RangeFull), &LossyOptions{Quality: 90})
	if err != nil {
		t.Fatal(err)
	}
	if string(limited) == string(full) {
		t.Fatal("Range was ignored: full and limited range planes encoded identically")
	}
}

// Round tripping full-range planes through the encoder and back has to return
// full-range planes, which is what keeps a JPEG transcode from shifting.
func TestEncodeLossyYUVFullRangeRoundTrips(t *testing.T) {
	const size = 32
	img := &YUVImage{
		Width: size, Height: size,
		Y: make([]byte, size*size), U: make([]byte, size*size/4), V: make([]byte, size*size/4),
		YStride: size, UVStride: size / 2, Range: RangeFull,
	}
	for i := range img.Y {
		img.Y[i] = 255
	}
	for i := range img.U {
		img.U[i] = 128
		img.V[i] = 128
	}

	data, err := EncodeLossyYUV(img, &LossyOptions{Quality: 100})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeYUV(data)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Range != RangeLimited {
		t.Fatalf("DecodeYUV reported range %v", decoded.Range)
	}
	decoded.ConvertRange(RangeFull)
	if got := decoded.Y[decoded.YStride*size/2+size/2]; got < 253 {
		t.Fatalf("full-range white round tripped to Y=%d", got)
	}
}
