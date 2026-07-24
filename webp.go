// Package webp is a pure Go WebP decoder and encoder.
//
// It decodes still and animated WebP and encodes still images as lossy VP8 or
// lossless VP8L, with no cgo and no external dependencies. It is a Go port of
// the webp-rust crate by MITH@mmk (https://github.com/mith-mmk/webp-rust).
//
// The API is small:
//
//   - [Decode] and [DecodeFile] decode a still image into an [Image].
//   - [DecodeAnimation] decodes an animated WebP into an [Animation].
//   - [Features] reports dimensions and format without a full decode.
//   - [EncodeLossy] and [EncodeLossless] encode an [Image], configured with
//     [LossyOptions] / [LosslessOptions] (pass nil for defaults).
//
// All pixel data is 8-bit RGBA in row-major order.
package webp

import "os"

// Decode decodes a still WebP image from memory into an RGBA buffer.
//
// Animated WebP is rejected; use [DecodeAnimation] for animated input.
func Decode(data []byte) (Image, error) {
	features, err := Features(data)
	if err != nil {
		return Image{}, err
	}
	if features.HasAnimation {
		return Image{}, unsupportedErr("animated WebP requires animation decoder API")
	}

	var image decodedImage
	switch features.Format {
	case FormatLossy:
		image, err = decodeLossyWebpToRGBA(data)
	case FormatLossless:
		image, err = decodeLosslessWebpToRGBA(data)
	default:
		return Image{}, unsupportedErr("unsupported WebP format")
	}
	if err != nil {
		return Image{}, err
	}

	return Image{
		Width:  image.Width,
		Height: image.Height,
		RGBA:   image.RGBA,
	}, nil
}

// DecodeFile reads a still WebP image from disk and decodes it to RGBA.
func DecodeFile(filename string) (Image, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return Image{}, err
	}
	return Decode(data)
}

// EncodeLossy encodes an image as a still lossy (VP8) WebP container. A nil opts
// uses the defaults (quality 90, effort 0).
func EncodeLossy(image *Image, opts *LossyOptions) ([]byte, error) {
	o := elossyDefaultOptions()
	if opts != nil {
		o = *opts
	}
	return encodeLossyImageToWebpWithOptionsAndExif(image, &o, o.EXIF)
}

// EncodeLossless encodes an image as a still lossless (VP8L) WebP container. A
// nil opts uses the default (effort 6).
func EncodeLossless(image *Image, opts *LosslessOptions) ([]byte, error) {
	o := elosslessDefaultOptions()
	if opts != nil {
		o = *opts
	}
	return encodeLosslessImageToWebpWithOptionsAndExif(image, &o, o.EXIF)
}
