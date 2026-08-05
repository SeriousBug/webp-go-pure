package webp

// YUVImage is a planar 4:2:0 YCbCr image, the representation the lossy VP8
// codec works in natively.
//
// Lossy WebP stores luma at full resolution and chroma at half resolution in
// each direction, so going through [Image] costs a YUV/RGBA conversion in
// whichever direction you are travelling. [DecodeYUV] and [EncodeLossyYUV]
// skip it. This is what makes a JPEG transcode cheap: a baseline JPEG is
// already planar 4:2:0 YCbCr, so its planes need only a range rescale rather
// than a full colorspace conversion. Set [YUVImage.Range] to say which range
// the samples are in.
//
// The Y plane holds Height rows of Width samples at YStride bytes per row. The
// U and V planes hold (Height+1)/2 rows of (Width+1)/2 samples at UVStride
// bytes per row. Strides may exceed the row width, so a plane carrying trailing
// padding, as a JPEG's macroblock-padded planes do, needs no repacking.
type YUVImage struct {
	// Width in pixels.
	Width int
	// Height in pixels.
	Height int
	// Y is the full-resolution luma plane.
	Y []byte
	// U and V are the half-resolution chroma planes.
	U []byte
	V []byte
	// YStride is the byte offset between successive rows of Y.
	YStride int
	// UVStride is the byte offset between successive rows of U and V.
	UVStride int
	// Range is how the samples use the 0..255 byte range. The zero value,
	// [RangeLimited], is what lossy WebP itself stores. Planes taken from
	// image/jpeg or from an image.YCbCr are [RangeFull] and must say so, or
	// they will encode with lifted blacks and clipped whites.
	Range ColorRange
	// A, when non-nil, is a full-resolution straight-alpha plane of Height rows
	// at AStride bytes per row. [DecodeYUV] sets it when the container carries
	// an ALPH chunk. The lossy encoder does not support alpha, so
	// [EncodeLossyYUV] rejects input that sets it.
	A []byte
	// AStride is the byte offset between successive rows of A.
	AStride int
}

// DecodeYUV decodes a still lossy WebP image into its native planar 4:2:0
// buffers, skipping the YUV to RGBA conversion [Decode] performs.
//
// Lossless (VP8L) input is rejected: it is natively RGBA, so there are no YUV
// planes to hand back. Check [Features] first, or use [Decode], which handles
// both. Animated input is rejected with an error matching [ErrAnimated].
func DecodeYUV(data []byte) (YUVImage, error) {
	features, err := Features(data)
	if err != nil {
		return YUVImage{}, err
	}
	if features.HasAnimation {
		return YUVImage{}, animatedErr("animated WebP requires DecodeAnimation")
	}
	if features.Format != FormatLossy {
		return YUVImage{}, unsupportedErr("only lossy WebP has YUV planes; use Decode for lossless")
	}

	parsed, err := parseStillWebp(data)
	if err != nil {
		return YUVImage{}, err
	}
	yuv, err := decodeLossyVp8ToYuv(parsed.ImageData)
	if err != nil {
		return YUVImage{}, err
	}

	out := YUVImage{
		Width:    yuv.Width,
		Height:   yuv.Height,
		Range:    RangeLimited,
		Y:        yuv.Y,
		U:        yuv.U,
		V:        yuv.V,
		YStride:  yuv.YStride,
		UVStride: yuv.UVStride,
	}
	if parsed.AlphaData != nil {
		alpha, err := decodeAlphaPlane(parsed.AlphaData, yuv.Width, yuv.Height)
		if err != nil {
			return YUVImage{}, err
		}
		out.A = alpha
		out.AStride = yuv.Width
	}
	return out, nil
}

// EncodeLossyYUV encodes planar 4:2:0 buffers as a still lossy (VP8) WebP
// container, skipping the RGBA to YUV conversion [EncodeLossy] performs. A nil
// opts uses the defaults (quality 90, effort 0).
//
// The lossy encoder does not support alpha, so a non-nil [YUVImage.A] is
// rejected with an error matching [ErrLossyAlpha].
func EncodeLossyYUV(img *YUVImage, opts *LossyOptions) ([]byte, error) {
	o := elossyDefaultOptions()
	if opts != nil {
		o = *opts
	}
	if err := elossyValidateYuv(img); err != nil {
		return nil, err
	}
	if err := elossyValidateOptions(&o); err != nil {
		return nil, err
	}

	mbWidth := (img.Width + 15) >> 4
	mbHeight := (img.Height + 15) >> 4
	source := elossyYuvToPlanes(img, mbWidth, mbHeight)
	vp8, err := encodeLossyPlanesToVp8(img.Width, img.Height, &source, mbWidth, mbHeight, &o)
	if err != nil {
		return nil, err
	}
	return wrapStillWebp(stillImageChunk{
		fourcc:   [4]byte{'V', 'P', '8', ' '},
		payload:  vp8,
		width:    img.Width,
		height:   img.Height,
		hasAlpha: false,
	}, o.EXIF)
}
