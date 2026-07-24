// Command bmp2webp converts an uncompressed 24bpp or 32bpp BMP file to a still
// WebP. It mirrors the webp-rust example of the same name.
//
//	go run ./examples/bmp2webp -- [-z 0..9] [--lossy --quality 0..100 [--lossy-opt-level 0..9]] [--opt-level 0..9] <input.bmp> [output.webp]
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	webp "github.com/SeriousBug/webp-go-pure"
)

const (
	fileHeaderSize    = 14
	minInfoHeaderSize = 40
)

func rowStride(width, bytesPerPixel int) int {
	return (width*bytesPerPixel + 3) &^ 3
}

func decodeBMPToRGBA(data []byte) (*webp.ImageBuffer, error) {
	if len(data) < fileHeaderSize+minInfoHeaderSize {
		return nil, fmt.Errorf("BMP file is too small")
	}
	if string(data[0:2]) != "BM" {
		return nil, fmt.Errorf("expected a BMP file")
	}

	pixelOffset := int(binary.LittleEndian.Uint32(data[10:14]))
	dibHeaderSize := int(binary.LittleEndian.Uint32(data[14:18]))
	if dibHeaderSize < minInfoHeaderSize {
		return nil, fmt.Errorf("unsupported BMP DIB header")
	}
	if len(data) < fileHeaderSize+dibHeaderSize {
		return nil, fmt.Errorf("BMP DIB header is truncated")
	}

	widthI32 := int32(binary.LittleEndian.Uint32(data[18:22]))
	heightI32 := int32(binary.LittleEndian.Uint32(data[22:26]))
	planes := binary.LittleEndian.Uint16(data[26:28])
	bitsPerPixel := binary.LittleEndian.Uint16(data[28:30])
	compression := binary.LittleEndian.Uint32(data[30:34])

	if planes != 1 {
		return nil, fmt.Errorf("unsupported BMP plane count")
	}
	if compression != 0 {
		return nil, fmt.Errorf("only uncompressed BMP is supported")
	}
	if widthI32 <= 0 {
		return nil, fmt.Errorf("BMP width must be positive")
	}
	if heightI32 == 0 {
		return nil, fmt.Errorf("BMP height must be non-zero")
	}

	var bytesPerPixel int
	switch bitsPerPixel {
	case 24:
		bytesPerPixel = 3
	case 32:
		bytesPerPixel = 4
	default:
		return nil, fmt.Errorf("only 24bpp and 32bpp BMP are supported")
	}

	width := int(widthI32)
	topDown := heightI32 < 0
	height := int(heightI32)
	if height < 0 {
		height = -height
	}

	stride := rowStride(width, bytesPerPixel)
	pixelEnd := pixelOffset + stride*height
	if pixelOffset > len(data) || pixelEnd > len(data) {
		return nil, fmt.Errorf("BMP pixel data is truncated")
	}

	rgba := make([]byte, width*height*4)
	for y := 0; y < height; y++ {
		srcY := height - 1 - y
		if topDown {
			srcY = y
		}
		srcRow := pixelOffset + srcY*stride
		dstRow := y * width * 4
		for x := 0; x < width; x++ {
			src := srcRow + x*bytesPerPixel
			dst := dstRow + x*4
			rgba[dst] = data[src+2]
			rgba[dst+1] = data[src+1]
			rgba[dst+2] = data[src]
			if bytesPerPixel == 4 {
				rgba[dst+3] = data[src+3]
			} else {
				rgba[dst+3] = 0xff
			}
		}
	}

	return &webp.ImageBuffer{Width: width, Height: height, RGBA: rgba}, nil
}

const usage = "usage: go run ./examples/bmp2webp -- [-z 0..9] [--lossy --quality 0..100 [--lossy-opt-level 0..9]] [--opt-level 0..9] <input.bmp> [output.webp]"

func parseU8Level(value, what string) (uint8, error) {
	n, err := strconv.ParseUint(value, 10, 8)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %s", what, value)
	}
	return uint8(n), nil
}

func run() error {
	args := os.Args[1:]
	var input, output string
	options := webp.DefaultLosslessEncodingOptions()
	lossy := false
	lossyOptions := webp.DefaultLossyEncodingOptions()
	var sharedLevel *uint8
	losslessExplicit := false
	lossyExplicit := false

	next := func(i *int) (string, error) {
		*i++
		if *i >= len(args) {
			return "", fmt.Errorf("%s", usage)
		}
		return args[*i], nil
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--lossy":
			lossy = true
		case "--opt", "--opt-level", "-O":
			v, err := next(&i)
			if err != nil {
				return err
			}
			lvl, err := parseU8Level(v, "optimization level")
			if err != nil {
				return err
			}
			if lvl > 9 {
				return fmt.Errorf("optimization level must be in 0..=9")
			}
			options.OptimizationLevel = lvl
			losslessExplicit = true
		case "--quality", "-q":
			v, err := next(&i)
			if err != nil {
				return err
			}
			q, err := parseU8Level(v, "quality")
			if err != nil {
				return err
			}
			if q > 100 {
				return fmt.Errorf("quality must be in 0..=100")
			}
			lossyOptions.Quality = q
		case "-z":
			v, err := next(&i)
			if err != nil {
				return err
			}
			lvl, err := parseU8Level(v, "optimization level")
			if err != nil {
				return err
			}
			sharedLevel = &lvl
		case "--lossy-opt-level":
			v, err := next(&i)
			if err != nil {
				return err
			}
			lvl, err := parseU8Level(v, "lossy optimization level")
			if err != nil {
				return err
			}
			if lvl > 9 {
				return fmt.Errorf("lossy optimization level must be in 0..=9")
			}
			lossyOptions.OptimizationLevel = lvl
			lossyExplicit = true
		default:
			if input == "" {
				input = arg
			} else if output == "" {
				output = arg
			} else {
				return fmt.Errorf("%s", usage)
			}
		}
	}

	if sharedLevel != nil {
		if *sharedLevel > 9 {
			if lossy {
				return fmt.Errorf("lossy optimization level must be in 0..=9")
			}
			return fmt.Errorf("optimization level must be in 0..=9")
		}
		if lossy {
			if !lossyExplicit {
				lossyOptions.OptimizationLevel = *sharedLevel
			}
		} else if !losslessExplicit {
			options.OptimizationLevel = *sharedLevel
		}
	}

	if input == "" {
		return fmt.Errorf("%s", usage)
	}
	if output == "" {
		output = strings.TrimSuffix(input, filepath.Ext(input)) + ".webp"
	}

	data, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	image, err := decodeBMPToRGBA(data)
	if err != nil {
		return err
	}
	var out []byte
	if lossy {
		out, err = webp.EncodeLossyImageToWebpWithOptions(image, &lossyOptions)
	} else {
		out, err = webp.EncodeLosslessImageToWebpWithOptions(image, &options)
	}
	if err != nil {
		return err
	}

	if parent := filepath.Dir(output); parent != "" && parent != "." {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(output, out, 0o644); err != nil {
		return err
	}

	fmt.Println(output)
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
