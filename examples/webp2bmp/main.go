// Command webp2bmp converts a still WebP to a BMP file and an animated WebP to a
// BMP sequence. It mirrors the webp-rust example of the same name.
//
//	go run ./examples/webp2bmp -- <input.webp> [output.bmp | output_prefix]
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	webp "github.com/SeriousBug/webp-go-pure"
)

const (
	fileHeaderSize = 14
	infoHeaderSize = 40
	bmpHeaderSize  = fileHeaderSize + infoHeaderSize
	bitsPerPixel   = 24
	pixelsPerMeter = 3780
)

func rowStride(width int) int {
	return (width*3 + 3) &^ 3
}

func encodeBMP24FromRGBA(width, height int, rgba []byte) ([]byte, error) {
	if width == 0 || height == 0 {
		return nil, fmt.Errorf("BMP dimensions must be non-zero")
	}
	if len(rgba) != width*height*4 {
		return nil, fmt.Errorf("RGBA buffer length does not match dimensions")
	}

	stride := rowStride(width)
	pixelBytes := stride * height
	fileSize := bmpHeaderSize + pixelBytes

	bmp := make([]byte, fileSize)
	copy(bmp[0:2], "BM")
	binary.LittleEndian.PutUint32(bmp[2:6], uint32(fileSize))
	binary.LittleEndian.PutUint32(bmp[10:14], uint32(bmpHeaderSize))

	binary.LittleEndian.PutUint32(bmp[14:18], uint32(infoHeaderSize))
	binary.LittleEndian.PutUint32(bmp[18:22], uint32(int32(width)))
	binary.LittleEndian.PutUint32(bmp[22:26], uint32(int32(height)))
	binary.LittleEndian.PutUint16(bmp[26:28], 1)
	binary.LittleEndian.PutUint16(bmp[28:30], bitsPerPixel)
	binary.LittleEndian.PutUint32(bmp[34:38], uint32(pixelBytes))
	binary.LittleEndian.PutUint32(bmp[38:42], pixelsPerMeter)
	binary.LittleEndian.PutUint32(bmp[42:46], pixelsPerMeter)

	destOffset := bmpHeaderSize
	row := make([]byte, stride)
	for y := height - 1; y >= 0; y-- {
		for i := range row {
			row[i] = 0
		}
		srcRow := y * width * 4
		for x := 0; x < width; x++ {
			src := srcRow + x*4
			dst := x * 3
			row[dst] = rgba[src+2]
			row[dst+1] = rgba[src+1]
			row[dst+2] = rgba[src]
		}
		copy(bmp[destOffset:destOffset+stride], row)
		destOffset += stride
	}

	return bmp, nil
}

func animationFramePath(prefix string, index int) string {
	parent := filepath.Dir(prefix)
	stem := filepath.Base(prefix)
	if stem == "" || stem == "." || stem == string(filepath.Separator) {
		stem = "frame"
	}
	return filepath.Join(parent, fmt.Sprintf("%s_%04d.bmp", stem, index))
}

func run() error {
	args := os.Args[1:]
	input := "testdata/sample.webp"
	if len(args) > 0 {
		input = args[0]
	}
	var output string
	if len(args) > 1 {
		output = args[1]
	} else {
		switch input {
		case "testdata/sample.webp":
			output = "output/sample.bmp"
		case "testdata/sample_animation.webp":
			output = "output/sample_animation"
		default:
			output = strings.TrimSuffix(input, filepath.Ext(input)) + ".bmp"
		}
	}

	data, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	features, err := webp.Features(data)
	if err != nil {
		return err
	}

	if features.HasAnimation {
		prefix := output
		if info, err := os.Stat(output); err == nil && info.IsDir() {
			stem := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
			if stem == "" {
				stem = "frame"
			}
			prefix = filepath.Join(output, stem)
		}
		if ext := filepath.Ext(prefix); ext != "" {
			prefix = strings.TrimSuffix(prefix, ext)
		}

		animation, err := webp.DecodeAnimation(data)
		if err != nil {
			return err
		}
		var written []string
		for index, frame := range animation.Frames {
			path := animationFramePath(prefix, index)
			if parent := filepath.Dir(path); parent != "" && parent != "." {
				if err := os.MkdirAll(parent, 0o755); err != nil {
					return err
				}
			}
			bmp, err := encodeBMP24FromRGBA(animation.Width, animation.Height, frame.RGBA)
			if err != nil {
				return err
			}
			if err := os.WriteFile(path, bmp, 0o644); err != nil {
				return err
			}
			written = append(written, path)
		}
		for _, path := range written {
			fmt.Println(path)
		}
		return nil
	}

	image, err := webp.Decode(data)
	if err != nil {
		return err
	}
	bmp, err := encodeBMP24FromRGBA(image.Width, image.Height, image.RGBA)
	if err != nil {
		return err
	}

	if parent := filepath.Dir(output); parent != "" && parent != "." {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(output, bmp, 0o644); err != nil {
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
