package webp

func lossyArgbToRgba(argb uint32) [4]byte {
	return [4]byte{
		byte((argb >> 16) & 0xff),
		byte((argb >> 8) & 0xff),
		byte(argb & 0xff),
		byte(argb >> 24),
	}
}

func lossyFillRect(canvas []byte, canvasWidth, xOffset, yOffset, width, height int, rgba [4]byte) {
	for y := 0; y < height; y++ {
		row := ((yOffset+y)*canvasWidth + xOffset) * 4
		for x := 0; x < width; x++ {
			dst := row + x*4
			copy(canvas[dst:dst+4], rgba[:])
		}
	}
}

func lossyBlendChannel(src byte, srcAlpha uint32, dst byte, dstFactorAlpha, scale uint32) byte {
	blended := (uint32(src)*srcAlpha + uint32(dst)*dstFactorAlpha) * scale
	return byte(blended >> 24)
}

func lossyBlendPixelNonPremult(src, dst [4]byte) [4]byte {
	srcAlpha := uint32(src[3])
	if srcAlpha == 0 {
		return dst
	}
	if srcAlpha == 255 {
		return src
	}

	dstAlpha := uint32(dst[3])
	dstFactorAlpha := (dstAlpha * (256 - srcAlpha)) >> 8
	blendAlpha := srcAlpha + dstFactorAlpha
	scale := (uint32(1) << 24) / blendAlpha

	return [4]byte{
		lossyBlendChannel(src[0], srcAlpha, dst[0], dstFactorAlpha, scale),
		lossyBlendChannel(src[1], srcAlpha, dst[1], dstFactorAlpha, scale),
		lossyBlendChannel(src[2], srcAlpha, dst[2], dstFactorAlpha, scale),
		byte(blendAlpha),
	}
}

func lossyCompositeFrame(canvas []byte, canvasWidth int, frameRGBA []byte, frame *ParsedAnimationFrame) {
	for y := 0; y < frame.Height; y++ {
		srcRow := y * frame.Width * 4
		dstRow := ((frame.YOffset+y)*canvasWidth + frame.XOffset) * 4
		for x := 0; x < frame.Width; x++ {
			src := srcRow + x*4
			dst := dstRow + x*4
			if frame.Blend {
				srcPixel := [4]byte{frameRGBA[src], frameRGBA[src+1], frameRGBA[src+2], frameRGBA[src+3]}
				dstPixel := [4]byte{canvas[dst], canvas[dst+1], canvas[dst+2], canvas[dst+3]}
				out := lossyBlendPixelNonPremult(srcPixel, dstPixel)
				copy(canvas[dst:dst+4], out[:])
			} else {
				copy(canvas[dst:dst+4], frameRGBA[src:src+4])
			}
		}
	}
}

func lossyDecodeFrameImage(frame *ParsedAnimationFrame) (DecodedImage, error) {
	var image DecodedImage
	var err error
	switch frame.ImageChunk.Fourcc {
	case [4]byte{'V', 'P', '8', 'L'}:
		if frame.AlphaChunk != nil {
			return DecodedImage{}, bitstreamErr("VP8L animation frame must not carry ALPH chunk")
		}
		image, err = DecodeLosslessVp8lToRGBA(frame.ImageData)
	case [4]byte{'V', 'P', '8', ' '}:
		image, err = decodeLossyVp8FrameToRGBA(frame.ImageData, frame.AlphaData)
	default:
		return DecodedImage{}, bitstreamErr("unsupported animation frame chunk")
	}
	if err != nil {
		return DecodedImage{}, err
	}

	if image.Width != frame.Width || image.Height != frame.Height {
		return DecodedImage{}, bitstreamErr("animation frame dimensions do not match bitstream")
	}
	return image, nil
}

type lossyDisposeRect struct {
	xOffset int
	yOffset int
	width   int
	height  int
}

// DecodeAnimation decodes an animated WebP container to a sequence of composited
// RGBA frames.
func DecodeAnimation(data []byte) (DecodedAnimation, error) {
	parsed, err := ParseAnimationWebp(data)
	if err != nil {
		return DecodedAnimation{}, err
	}
	background := lossyArgbToRgba(parsed.Animation.BackgroundColor)
	canvas := make([]byte, parsed.Features.Width*parsed.Features.Height*4)
	lossyFillRect(canvas, parsed.Features.Width, 0, 0, parsed.Features.Width, parsed.Features.Height, background)

	var previousRect *lossyDisposeRect
	frames := make([]DecodedAnimationFrame, 0, len(parsed.Frames))
	for i := range parsed.Frames {
		frame := &parsed.Frames[i]
		if previousRect != nil {
			lossyFillRect(canvas, parsed.Features.Width, previousRect.xOffset, previousRect.yOffset, previousRect.width, previousRect.height, background)
			previousRect = nil
		}

		decoded, err := lossyDecodeFrameImage(frame)
		if err != nil {
			return DecodedAnimation{}, err
		}
		lossyCompositeFrame(canvas, parsed.Features.Width, decoded.RGBA, frame)

		frameCopy := make([]byte, len(canvas))
		copy(frameCopy, canvas)
		frames = append(frames, DecodedAnimationFrame{
			Duration: frame.Duration,
			RGBA:     frameCopy,
		})

		if frame.DisposeToBackground {
			previousRect = &lossyDisposeRect{
				xOffset: frame.XOffset,
				yOffset: frame.YOffset,
				width:   frame.Width,
				height:  frame.Height,
			}
		}
	}

	return DecodedAnimation{
		Width:           parsed.Features.Width,
		Height:          parsed.Features.Height,
		BackgroundColor: parsed.Animation.BackgroundColor,
		LoopCount:       parsed.Animation.LoopCount,
		Frames:          frames,
	}, nil
}
