// Package std decodes and encodes WebP through the standard library's image
// interfaces.
//
// It has the same shape as image/png and image/jpeg, so it drops into code that
// already speaks image.Image:
//
//	img, err := std.Decode(r)
//	err = std.Encode(w, img, &std.Options{Quality: 80})
//
// Import the register subpackage to make image.Decode recognize WebP.
//
// # Avoiding conversions
//
// Lossy WebP and JPEG are both planar 4:2:0 YCbCr in the same colorspace, so
// this package moves planes rather than pixels wherever it can. [Decode] hands
// back the decoder's own planes as an *image.YCbCr instead of converting them
// to RGBA, and [Encode] feeds an *image.YCbCr straight to the encoder instead
// of converting it twice. Transcoding a JPEG to lossy WebP therefore does no
// colorspace math at all: image/jpeg decodes to *image.YCbCr, and that is
// exactly what the WebP encoder wants.
//
// Anything else still works, it just costs a conversion through
// *image.NRGBA.
package std

import (
	"errors"
	"image"
	"image/color"
	"image/draw"
	"io"

	webp "github.com/SeriousBug/webp-go-pure"
)

// DefaultQuality is the lossy quality [Options] uses when Quality is zero.
const DefaultQuality = 90

// EffortFastest asks for the fastest encode. It exists because the zero value
// of [Options.Effort] means "the default for this mode" rather than zero, so
// this is how you request effort 0 explicitly.
const EffortFastest = -1

// Default effort per mode. Lossy defaults to the fastest setting because its
// quality knob already governs size; lossless has no such knob, so it defaults
// to the middle of the range.
const (
	defaultLossyEffort    = 0
	defaultLosslessEffort = 6
	maxEffort             = 9
)

// Options configures [Encode]. A nil *Options, or a zero field, means the
// default.
type Options struct {
	// Quality is the lossy quality target in 1..100. Higher is better looking
	// and larger. Zero means [DefaultQuality]. Ignored when Lossless is set.
	Quality int
	// Effort trades encode time for file size, in 0..9. Higher is slower and
	// smaller. Zero means the default for the mode, which is 0 for lossy and 6
	// for lossless; pass [EffortFastest] to ask for 0 explicitly.
	Effort int
	// Lossless selects the VP8L encoder, which reproduces the input exactly.
	Lossless bool
	// EXIF, if non-nil, is embedded as a raw EXIF metadata chunk.
	EXIF []byte
}

func (o *Options) quality() (uint8, error) {
	if o.Quality == 0 {
		return DefaultQuality, nil
	}
	if o.Quality < 0 || o.Quality > 100 {
		return 0, errors.New("webp: quality must be in 1..100")
	}
	return uint8(o.Quality), nil
}

func (o *Options) effort(fallback int) (uint8, error) {
	effort := o.Effort
	switch {
	case effort == 0:
		effort = fallback
	case effort == EffortFastest:
		effort = 0
	case effort < 0 || effort > maxEffort:
		return 0, errors.New("webp: effort must be in 0..9")
	}
	return uint8(effort), nil
}

// Decode reads a WebP image from r.
//
// The returned concrete type is whatever avoids a conversion: *image.YCbCr for
// lossy input, *image.NYCbCrA for lossy input with an alpha channel, and
// *image.NRGBA for lossless input. For animated input it is the first frame,
// as an *image.NRGBA; use [DecodeAll] to get every frame.
func Decode(r io.Reader) (image.Image, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return decode(data)
}

func decode(data []byte) (image.Image, error) {
	features, err := webp.Features(data)
	if err != nil {
		return nil, err
	}

	if features.HasAnimation {
		anim, err := webp.DecodeAnimation(data)
		if err != nil {
			return nil, err
		}
		if len(anim.Frames) == 0 {
			return nil, errors.New("webp: animation has no frames")
		}
		return nrgbaOf(anim.Width, anim.Height, anim.Frames[0].RGBA), nil
	}

	if features.Format == webp.FormatLossy {
		yuv, err := webp.DecodeYUV(data)
		if err != nil {
			return nil, err
		}
		return imageOfYUV(&yuv), nil
	}

	img, err := webp.Decode(data)
	if err != nil {
		return nil, err
	}
	return nrgbaOf(img.Width, img.Height, img.RGBA), nil
}

func nrgbaOf(width, height int, pix []byte) *image.NRGBA {
	return &image.NRGBA{
		Pix:    pix,
		Stride: width * 4,
		Rect:   image.Rect(0, 0, width, height),
	}
}

// imageOfYUV wraps decoded planes without copying them.
func imageOfYUV(yuv *webp.YUVImage) image.Image {
	ycbcr := image.YCbCr{
		Y:              yuv.Y,
		Cb:             yuv.U,
		Cr:             yuv.V,
		YStride:        yuv.YStride,
		CStride:        yuv.UVStride,
		SubsampleRatio: image.YCbCrSubsampleRatio420,
		Rect:           image.Rect(0, 0, yuv.Width, yuv.Height),
	}
	if yuv.A == nil {
		return &ycbcr
	}
	return &image.NYCbCrA{YCbCr: ycbcr, A: yuv.A, AStride: yuv.AStride}
}

// configProbeLimit caps how much of r DecodeConfig will read looking for a
// header. Metadata chunks can precede the image data, so a fixed small read is
// not enough, but neither should a malformed file pull in the whole stream.
const configProbeLimit = 1 << 20

// DecodeConfig returns the color model and dimensions of a WebP image without
// decoding it.
//
// The color model is the one [Decode] would produce: color.YCbCrModel for
// lossy, color.NYCbCrAModel for lossy with alpha, and color.NRGBAModel for
// lossless and for animations.
func DecodeConfig(r io.Reader) (image.Config, error) {
	buf := make([]byte, 0, 512)
	for {
		features, err := webp.Features(buf)
		if err == nil {
			return configOf(features), nil
		}
		// Anything other than a short buffer is a real parse failure, and
		// reading more will not fix it.
		if !errors.Is(err, webp.ErrNotEnoughData) {
			return image.Config{}, err
		}
		if len(buf) >= configProbeLimit {
			return image.Config{}, err
		}

		if len(buf) == cap(buf) {
			grown := make([]byte, len(buf), 2*cap(buf))
			copy(grown, buf)
			buf = grown
		}
		n, readErr := r.Read(buf[len(buf):cap(buf)])
		buf = buf[:len(buf)+n]
		if readErr != nil {
			if readErr == io.EOF {
				// Report the parse failure rather than the EOF: the input is a
				// truncated WebP, not an I/O problem.
				return image.Config{}, err
			}
			return image.Config{}, readErr
		}
	}
}

func configOf(features webp.FeatureInfo) image.Config {
	model := color.NRGBAModel
	if !features.HasAnimation && features.Format == webp.FormatLossy {
		model = color.YCbCrModel
		if features.HasAlpha {
			model = color.NYCbCrAModel
		}
	}
	return image.Config{
		ColorModel: model,
		Width:      features.Width,
		Height:     features.Height,
	}
}

// Encode writes m to w as a WebP image. A nil *Options uses the defaults:
// lossy, quality 90.
//
// The lossy encoder does not support transparency and rejects an image that is
// not fully opaque with an error matching webp.ErrLossyAlpha. Set
// [Options.Lossless] to keep an alpha channel.
func Encode(w io.Writer, m image.Image, o *Options) error {
	var opts Options
	if o != nil {
		opts = *o
	}

	data, err := encode(m, &opts)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func encode(m image.Image, o *Options) ([]byte, error) {
	if m.Bounds().Empty() {
		return nil, errors.New("webp: image has no pixels")
	}

	if o.Lossless {
		effort, err := o.effort(defaultLosslessEffort)
		if err != nil {
			return nil, err
		}
		img, err := rgbaOf(m)
		if err != nil {
			return nil, err
		}
		return webp.EncodeLossless(img, &webp.LosslessOptions{Effort: effort, EXIF: o.EXIF})
	}

	quality, err := o.quality()
	if err != nil {
		return nil, err
	}
	effort, err := o.effort(defaultLossyEffort)
	if err != nil {
		return nil, err
	}
	lossy := &webp.LossyOptions{Quality: quality, Effort: effort, EXIF: o.EXIF}

	if planes, ok := planesOf(m); ok {
		return webp.EncodeLossyYUV(planes, lossy)
	}
	img, err := rgbaOf(m)
	if err != nil {
		return nil, err
	}
	return webp.EncodeLossy(img, lossy)
}

// planesOf recognizes the images that are already in the encoder's own layout,
// which is what makes a JPEG transcode free of colorspace math. Anything it
// turns down falls back to the RGBA path, so turning a case down costs
// performance and never correctness.
func planesOf(m image.Image) (*webp.YUVImage, bool) {
	var ycbcr *image.YCbCr
	switch src := m.(type) {
	case *image.YCbCr:
		ycbcr = src
	case *image.NYCbCrA:
		// The lossy encoder has no alpha channel to put A in. When the image is
		// opaque the plane carries no information and can be dropped; when it is
		// not, the RGBA path produces the ErrLossyAlpha rejection.
		if !src.Opaque() {
			return nil, false
		}
		ycbcr = &src.YCbCr
	default:
		return nil, false
	}

	// VP8 is 4:2:0 only. Other ratios would need a chroma resample, which the
	// RGBA path already does correctly.
	if ycbcr.SubsampleRatio != image.YCbCrSubsampleRatio420 {
		return nil, false
	}
	// An odd origin puts the chroma planes half a sample out of step with the
	// luma plane, which a packed 4:2:0 buffer cannot express.
	b := ycbcr.Rect
	if b.Min.X&1 != 0 || b.Min.Y&1 != 0 {
		return nil, false
	}

	return &webp.YUVImage{
		Width:    b.Dx(),
		Height:   b.Dy(),
		Y:        ycbcr.Y,
		U:        ycbcr.Cb,
		V:        ycbcr.Cr,
		YStride:  ycbcr.YStride,
		UVStride: ycbcr.CStride,
	}, true
}

// rgbaOf produces the packed straight-alpha buffer the byte-oriented API takes.
//
// Note that *image.RGBA is deliberately not special-cased: its pixels are
// alpha-premultiplied, and handing them over as-is would darken everything that
// is not fully opaque. draw.Draw un-premultiplies on the way into an NRGBA.
func rgbaOf(m image.Image) (*webp.Image, error) {
	b := m.Bounds()
	width, height := b.Dx(), b.Dy()

	if src, ok := m.(*image.NRGBA); ok && src.Stride == width*4 && len(src.Pix) >= width*height*4 {
		return &webp.Image{Width: width, Height: height, RGBA: src.Pix[:width*height*4]}, nil
	}

	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.Draw(dst, dst.Bounds(), m, b.Min, draw.Src)
	return &webp.Image{Width: width, Height: height, RGBA: dst.Pix}, nil
}

// Animation is a decoded animated WebP, in the shape of gif.GIF.
type Animation struct {
	// Image holds the frames, each already composited onto the canvas, so a
	// frame can be displayed without reference to the ones before it.
	Image []image.Image
	// Delay holds each frame's display duration in milliseconds. Note that
	// gif.GIF measures delays in 100ths of a second; WebP does not.
	Delay []int
	// LoopCount is how many times the animation repeats. Zero means forever.
	LoopCount int
	// Config is the canvas color model and dimensions.
	Config image.Config
}

// DecodeAll reads every frame of an animated WebP from r. A still image decodes
// as a single-frame animation.
func DecodeAll(r io.Reader) (*Animation, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	features, err := webp.Features(data)
	if err != nil {
		return nil, err
	}
	if !features.HasAnimation {
		img, err := decode(data)
		if err != nil {
			return nil, err
		}
		return &Animation{
			Image:  []image.Image{img},
			Delay:  []int{0},
			Config: configOf(features),
		}, nil
	}

	anim, err := webp.DecodeAnimation(data)
	if err != nil {
		return nil, err
	}
	out := &Animation{
		Image:     make([]image.Image, len(anim.Frames)),
		Delay:     make([]int, len(anim.Frames)),
		LoopCount: int(anim.LoopCount),
		Config:    configOf(features),
	}
	for i, frame := range anim.Frames {
		out.Image[i] = nrgbaOf(anim.Width, anim.Height, frame.RGBA)
		out.Delay[i] = frame.Duration
	}
	return out, nil
}

// DecodeBytes is Decode without the io.Reader, for callers who already hold the
// encoded image. It saves the copy io.ReadAll makes.
func DecodeBytes(data []byte) (image.Image, error) {
	return decode(data)
}

// EncodeBytes is Encode without the io.Writer, for callers who want the encoded
// image as a buffer. It saves the copy Encode's Write makes.
func EncodeBytes(m image.Image, o *Options) ([]byte, error) {
	var opts Options
	if o != nil {
		opts = *o
	}
	return encode(m, &opts)
}
