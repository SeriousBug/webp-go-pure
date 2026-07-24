// Package webp is a pure Go WebP decoder and partial encoder.
//
// It is a Go port of the webp-rust crate by MITH@mmk
// (https://github.com/mith-mmk/webp-rust). The top-level API is intentionally
// small:
//
//   - Decode decodes a still WebP image into an ImageBuffer.
//   - Encode encodes an ImageBuffer as lossy or lossless WebP.
//   - EncodeLossy / EncodeLossless target a single compression mode.
//
// Lower-level codec and container entry points remain available as exported
// functions in this package (GetFeatures, ParseStillWebp, DecodeLossyWebpToRGBA,
// and so on). Animated WebP is decoded with DecodeAnimation.
package webp

import "os"

// WebpEncoding selects the still-image WebP compression mode.
type WebpEncoding int

const (
	// Lossless encodes as VP8L.
	Lossless WebpEncoding = iota
	// Lossy encodes as VP8.
	Lossy
)

// Decode decodes a still WebP image from memory into an RGBA buffer.
//
// Animated WebP is rejected; use DecodeAnimation for animated input.
func Decode(data []byte) (ImageBuffer, error) {
	features, err := GetFeatures(data)
	if err != nil {
		return ImageBuffer{}, err
	}
	if features.HasAnimation {
		return ImageBuffer{}, unsupportedErr("animated WebP requires animation decoder API")
	}

	var image DecodedImage
	switch features.Format {
	case FormatLossy:
		image, err = DecodeLossyWebpToRGBA(data)
	case FormatLossless:
		image, err = DecodeLosslessWebpToRGBA(data)
	default:
		return ImageBuffer{}, unsupportedErr("unsupported WebP format")
	}
	if err != nil {
		return ImageBuffer{}, err
	}

	return ImageBuffer{
		Width:  image.Width,
		Height: image.Height,
		RGBA:   image.RGBA,
	}, nil
}

func toLosslessOptions(optimize int) (LosslessEncodingOptions, error) {
	if optimize < 0 || optimize > 255 {
		return LosslessEncodingOptions{}, encInvalidParam("lossless optimization level must be in 0..=9")
	}
	return LosslessEncodingOptions{OptimizationLevel: uint8(optimize)}, nil
}

func toLossyOptions(optimize, quality int) (LossyEncodingOptions, error) {
	if optimize < 0 || optimize > 255 {
		return LossyEncodingOptions{}, encInvalidParam("lossy optimization level must be in 0..=9")
	}
	if quality < 0 || quality > 255 {
		return LossyEncodingOptions{}, encInvalidParam("quality must be in 0..=100")
	}
	return LossyEncodingOptions{
		Quality:           uint8(quality),
		OptimizationLevel: uint8(optimize),
	}, nil
}

// Encode encodes an image as a still WebP container.
//
// optimize is interpreted as 0..=9. quality is used only for lossy encoding and
// must be in 0..=100. If exif is non-nil it is embedded as a raw EXIF chunk.
func Encode(image *ImageBuffer, optimize, quality int, compression WebpEncoding, exif []byte) ([]byte, error) {
	switch compression {
	case Lossless:
		return EncodeLossless(image, optimize, exif)
	case Lossy:
		return EncodeLossy(image, optimize, quality, exif)
	default:
		return nil, encInvalidParam("unknown compression mode")
	}
}

// EncodeLossy encodes an image as a still lossy WebP container.
func EncodeLossy(image *ImageBuffer, optimize, quality int, exif []byte) ([]byte, error) {
	options, err := toLossyOptions(optimize, quality)
	if err != nil {
		return nil, err
	}
	return EncodeLossyImageToWebpWithOptionsAndExif(image, &options, exif)
}

// EncodeLossless encodes an image as a still lossless WebP container.
func EncodeLossless(image *ImageBuffer, optimize int, exif []byte) ([]byte, error) {
	options, err := toLosslessOptions(optimize)
	if err != nil {
		return nil, err
	}
	return EncodeLosslessImageToWebpWithOptionsAndExif(image, &options, exif)
}

// ImageFromBytes is a compatibility alias for Decode.
func ImageFromBytes(data []byte) (ImageBuffer, error) {
	return Decode(data)
}

// DecodeFile reads a still WebP image from disk and decodes it to RGBA.
func DecodeFile(filename string) (ImageBuffer, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return ImageBuffer{}, err
	}
	return Decode(data)
}

// ImageFromFile is a compatibility alias for DecodeFile.
func ImageFromFile(filename string) (ImageBuffer, error) {
	return DecodeFile(filename)
}
