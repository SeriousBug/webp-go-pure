package webp

import "bytes"

// chunkHeader holds common metadata for a RIFF chunk.
type chunkHeader struct {
	Fourcc     [4]byte
	Offset     int
	Size       int
	PaddedSize int
	DataOffset int
}

// vp8xHeader is a parsed VP8X extended header.
type vp8xHeader struct {
	Flags        uint32
	CanvasWidth  int
	CanvasHeight int
}

// FeatureInfo holds high-level image features derived from the container and bitstream.
type FeatureInfo struct {
	Width        int
	Height       int
	HasAlpha     bool
	HasAnimation bool
	Format       Format
	vp8x         *vp8xHeader
}

// parsedWebp is a parsed still-image WebP container with raw chunk slices.
type parsedWebp struct {
	Features    FeatureInfo
	RiffSize    *int
	ImageChunk  chunkHeader
	ImageData   []byte
	AlphaChunk  *chunkHeader
	AlphaData   []byte
	alphaHeader *alphaHeader
}

// animationHeader is a parsed ANIM chunk.
type animationHeader struct {
	BackgroundColor uint32
	LoopCount       uint16
}

// parsedAnimationFrame is a parsed animation frame entry.
type parsedAnimationFrame struct {
	FrameChunk          chunkHeader
	XOffset             int
	YOffset             int
	Width               int
	Height              int
	Duration            int
	Blend               bool
	DisposeToBackground bool
	ImageChunk          chunkHeader
	ImageData           []byte
	AlphaChunk          *chunkHeader
	AlphaData           []byte
	alphaHeader         *alphaHeader
}

// parsedAnimationWebp is a parsed animated WebP container.
type parsedAnimationWebp struct {
	Features  FeatureInfo
	RiffSize  *int
	Animation animationHeader
	Frames    []parsedAnimationFrame
}

func readLE24(b []byte) int {
	return int(b[0]) | int(b[1])<<8 | int(b[2])<<16
}

func readLE32(b []byte) int {
	return int(uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24)
}

func readLE16(b []byte) uint16 {
	return uint16(b[0]) | uint16(b[1])<<8
}

func paddedPayloadSize(size int) int {
	return size + (size & 1)
}

// parseChunkHeader reads a chunk header without requiring the payload it
// describes to be present. Features needs only the first few bytes of the image
// chunk, so demanding the whole payload would make reading a header cost as
// much as reading the file.
func parseChunkHeader(data []byte, offset int, riffLimit *int) (chunkHeader, error) {
	if len(data) < offset+chunkHeaderSize {
		return chunkHeader{}, notEnoughData("chunk header")
	}
	size := readLE32(data[offset+tagSize : offset+chunkHeaderSize])
	if size > maxChunkPayload {
		return chunkHeader{}, bitstreamErr("invalid chunk size")
	}

	paddedSize := paddedPayloadSize(size)
	if riffLimit != nil && offset+chunkHeaderSize+paddedSize > *riffLimit {
		return chunkHeader{}, bitstreamErr("chunk exceeds RIFF payload")
	}

	var fourcc [4]byte
	copy(fourcc[:], data[offset:offset+tagSize])
	return chunkHeader{
		Fourcc:     fourcc,
		Offset:     offset,
		Size:       size,
		PaddedSize: paddedSize,
		DataOffset: offset + chunkHeaderSize,
	}, nil
}

func parseChunk(data []byte, offset int, riffLimit *int) (chunkHeader, error) {
	chunk, err := parseChunkHeader(data, offset, riffLimit)
	if err != nil {
		return chunkHeader{}, err
	}
	if len(data) < chunk.Offset+chunkHeaderSize+chunk.PaddedSize {
		return chunkHeader{}, notEnoughData("chunk payload")
	}
	return chunk, nil
}

// parseRiffHeader reads the RIFF header without checking that the payload it
// declares is actually present, so a caller that only wants the image header
// does not have to hold the whole file.
func parseRiffHeader(data []byte) (*int, int, error) {
	if len(data) < riffHeaderSize {
		return nil, 0, notEnoughData("RIFF header")
	}
	if !bytes.Equal(data[:4], []byte("RIFF")) {
		return nil, 0, nil
	}
	if !bytes.Equal(data[8:12], []byte("WEBP")) {
		return nil, 0, bitstreamErr("wrong RIFF WEBP signature")
	}

	riffSize := readLE32(data[4:8])
	if riffSize < tagSize+chunkHeaderSize {
		return nil, 0, bitstreamErr("RIFF payload is too small")
	}
	if riffSize > maxChunkPayload {
		return nil, 0, bitstreamErr("RIFF payload is too large")
	}

	return &riffSize, riffHeaderSize, nil
}

func parseRiff(data []byte) (*int, int, error) {
	riffSize, offset, err := parseRiffHeader(data)
	if err != nil {
		return nil, 0, err
	}
	if riffSize != nil && *riffSize > len(data)-chunkHeaderSize {
		return nil, 0, notEnoughData("truncated RIFF payload")
	}
	return riffSize, offset, nil
}

func parseVp8x(data []byte, offset int) (*vp8xHeader, int, error) {
	if len(data) < offset+chunkHeaderSize {
		return nil, offset, nil
	}
	if !bytes.Equal(data[offset:offset+tagSize], []byte("VP8X")) {
		return nil, offset, nil
	}

	chunk, err := parseChunk(data, offset, nil)
	if err != nil {
		return nil, offset, err
	}
	if chunk.Size != vp8xChunkSize {
		return nil, offset, bitstreamErr("wrong VP8X chunk size")
	}

	flags := uint32(readLE32(data[offset+8 : offset+12]))
	canvasWidth := readLE24(data[offset+12:offset+15]) + 1
	canvasHeight := readLE24(data[offset+15:offset+18]) + 1
	if uint64(canvasWidth)*uint64(canvasHeight) >= maxImageArea {
		return nil, offset, bitstreamErr("canvas is too large")
	}

	return &vp8xHeader{
		Flags:        flags,
		CanvasWidth:  canvasWidth,
		CanvasHeight: canvasHeight,
	}, offset + chunkHeaderSize + chunk.PaddedSize, nil
}

func riffLimitOf(riffSize *int) *int {
	if riffSize == nil {
		return nil
	}
	limit := *riffSize + chunkHeaderSize
	return &limit
}

// Features returns high-level WebP features without fully decoding the image.
func Features(data []byte) (FeatureInfo, error) {
	riffSize, offset, err := parseRiffHeader(data)
	if err != nil {
		return FeatureInfo{}, err
	}
	riffLimit := riffLimitOf(riffSize)

	vp8x, nextOffset, err := parseVp8x(data, offset)
	if err != nil {
		return FeatureInfo{}, err
	}
	offset = nextOffset
	if riffSize == nil && vp8x != nil {
		return FeatureInfo{}, bitstreamErr("VP8X chunk requires RIFF")
	}

	hasAlpha := vp8x != nil && (vp8x.Flags&alphaFlag) != 0
	hasAnimation := vp8x != nil && (vp8x.Flags&animationFlag) != 0

	if vp8x != nil && hasAnimation {
		return FeatureInfo{
			Width:        vp8x.CanvasWidth,
			Height:       vp8x.CanvasHeight,
			HasAlpha:     hasAlpha,
			HasAnimation: hasAnimation,
			Format:       FormatUndefined,
			vp8x:         vp8x,
		}, nil
	}

	if len(data) < offset+tagSize {
		return FeatureInfo{}, notEnoughData("chunk tag")
	}

	if (riffSize != nil && vp8x != nil) ||
		(riffSize == nil && vp8x == nil && bytes.Equal(data[offset:offset+tagSize], []byte("ALPH"))) {
		for {
			chunk, err := parseChunk(data, offset, riffLimit)
			if err != nil {
				return FeatureInfo{}, err
			}
			if bytes.Equal(chunk.Fourcc[:], []byte("VP8 ")) || bytes.Equal(chunk.Fourcc[:], []byte("VP8L")) {
				break
			}
			if bytes.Equal(chunk.Fourcc[:], []byte("ALPH")) {
				hasAlpha = true
			}
			offset += chunkHeaderSize + chunk.PaddedSize
		}
	}

	// The image chunk's header carries the dimensions, so the payload only has
	// to be present as far as the frame header. Reporting features on an
	// otherwise truncated file is deliberate: it is what lets a caller size a
	// decode before committing to reading all of it.
	chunk, err := parseChunkHeader(data, offset, riffLimit)
	if err != nil {
		return FeatureInfo{}, err
	}
	payloadEnd := min(chunk.DataOffset+chunk.Size, len(data))
	payload := data[chunk.DataOffset:payloadEnd]
	var format Format
	var width, height int
	if bytes.Equal(chunk.Fourcc[:], []byte("VP8 ")) {
		width, height, err = getInfo(payload, chunk.Size)
		if err != nil {
			return FeatureInfo{}, err
		}
		format = FormatLossy
	} else if bytes.Equal(chunk.Fourcc[:], []byte("VP8L")) {
		info, err := getLosslessInfo(payload)
		if err != nil {
			return FeatureInfo{}, err
		}
		hasAlpha = hasAlpha || info.HasAlpha
		format, width, height = FormatLossless, info.Width, info.Height
	} else {
		return FeatureInfo{}, bitstreamErr("missing VP8/VP8L image chunk")
	}

	if vp8x != nil {
		if vp8x.CanvasWidth != width || vp8x.CanvasHeight != height {
			return FeatureInfo{}, bitstreamErr("VP8X canvas does not match image size")
		}
	}

	return FeatureInfo{
		Width:        width,
		Height:       height,
		HasAlpha:     hasAlpha,
		HasAnimation: hasAnimation,
		Format:       format,
		vp8x:         vp8x,
	}, nil
}

// parseStillWebp parses a still-image WebP container and returns raw chunk slices.
func parseStillWebp(data []byte) (parsedWebp, error) {
	riffSize, offset, err := parseRiff(data)
	if err != nil {
		return parsedWebp{}, err
	}
	riffLimit := riffLimitOf(riffSize)

	vp8x, nextOffset, err := parseVp8x(data, offset)
	if err != nil {
		return parsedWebp{}, err
	}
	offset = nextOffset
	if riffSize == nil && vp8x != nil {
		return parsedWebp{}, bitstreamErr("VP8X chunk requires RIFF")
	}
	if vp8x != nil && (vp8x.Flags&animationFlag) != 0 {
		return parsedWebp{}, unsupportedErr("animated WebP is not implemented")
	}

	var alphaChunk *chunkHeader
	if len(data) < offset+tagSize {
		return parsedWebp{}, notEnoughData("chunk tag")
	}
	if (riffSize != nil && vp8x != nil) ||
		(riffSize == nil && vp8x == nil && bytes.Equal(data[offset:offset+tagSize], []byte("ALPH"))) {
		for {
			chunk, err := parseChunk(data, offset, riffLimit)
			if err != nil {
				return parsedWebp{}, err
			}
			if bytes.Equal(chunk.Fourcc[:], []byte("VP8 ")) || bytes.Equal(chunk.Fourcc[:], []byte("VP8L")) {
				break
			}
			if bytes.Equal(chunk.Fourcc[:], []byte("ALPH")) {
				c := chunk
				alphaChunk = &c
			}
			offset += chunkHeaderSize + chunk.PaddedSize
		}
	}

	imageChunk, err := parseChunk(data, offset, riffLimit)
	if err != nil {
		return parsedWebp{}, err
	}
	if !bytes.Equal(imageChunk.Fourcc[:], []byte("VP8 ")) && !bytes.Equal(imageChunk.Fourcc[:], []byte("VP8L")) {
		return parsedWebp{}, bitstreamErr("missing VP8/VP8L image chunk")
	}
	imageData := data[imageChunk.DataOffset : imageChunk.DataOffset+imageChunk.Size]
	features, err := Features(data)
	if err != nil {
		return parsedWebp{}, err
	}
	var alphaData []byte
	var alphaHeader *alphaHeader
	if alphaChunk != nil {
		alphaData = data[alphaChunk.DataOffset : alphaChunk.DataOffset+alphaChunk.Size]
		h, err := parseAlphaHeader(alphaData)
		if err != nil {
			return parsedWebp{}, err
		}
		alphaHeader = &h
		features.HasAlpha = true
	}

	return parsedWebp{
		Features:    features,
		RiffSize:    riffSize,
		ImageChunk:  imageChunk,
		ImageData:   imageData,
		AlphaChunk:  alphaChunk,
		AlphaData:   alphaData,
		alphaHeader: alphaHeader,
	}, nil
}

func parseAnimationFrame(data []byte, features FeatureInfo, chunk chunkHeader, riffLimit *int) (parsedAnimationFrame, error) {
	if chunk.Size < 16 {
		return parsedAnimationFrame{}, bitstreamErr("ANMF chunk is too small")
	}

	header := data[chunk.DataOffset : chunk.DataOffset+16]
	xOffset := readLE24(header[0:3]) * 2
	yOffset := readLE24(header[3:6]) * 2
	width := readLE24(header[6:9]) + 1
	height := readLE24(header[9:12]) + 1
	duration := readLE24(header[12:15])
	flags := header[15]
	if flags>>2 != 0 {
		return parsedAnimationFrame{}, bitstreamErr("ANMF reserved bits must be zero")
	}
	if xOffset+width > features.Width || yOffset+height > features.Height {
		return parsedAnimationFrame{}, bitstreamErr("ANMF frame exceeds animation canvas")
	}

	offset := chunk.DataOffset + 16
	frameEnd := chunk.DataOffset + chunk.Size
	frameLimit := &frameEnd
	var alphaChunk *chunkHeader
	var imageChunk chunkHeader
	for {
		subchunk, err := parseChunk(data, offset, frameLimit)
		if err != nil {
			return parsedAnimationFrame{}, err
		}
		if bytes.Equal(subchunk.Fourcc[:], []byte("VP8 ")) || bytes.Equal(subchunk.Fourcc[:], []byte("VP8L")) {
			imageChunk = subchunk
			break
		}
		if bytes.Equal(subchunk.Fourcc[:], []byte("ALPH")) {
			c := subchunk
			alphaChunk = &c
		}
		offset += chunkHeaderSize + subchunk.PaddedSize
		if riffLimit != nil && offset > *riffLimit {
			return parsedAnimationFrame{}, bitstreamErr("ANMF frame data exceeds RIFF payload")
		}
	}

	imageData := data[imageChunk.DataOffset : imageChunk.DataOffset+imageChunk.Size]
	var alphaData []byte
	var alphaHeader *alphaHeader
	if alphaChunk != nil {
		alphaData = data[alphaChunk.DataOffset : alphaChunk.DataOffset+alphaChunk.Size]
		h, err := parseAlphaHeader(alphaData)
		if err != nil {
			return parsedAnimationFrame{}, err
		}
		alphaHeader = &h
	}

	return parsedAnimationFrame{
		FrameChunk:          chunk,
		XOffset:             xOffset,
		YOffset:             yOffset,
		Width:               width,
		Height:              height,
		Duration:            duration,
		Blend:               (flags & 0x02) == 0,
		DisposeToBackground: (flags & 0x01) != 0,
		ImageChunk:          imageChunk,
		ImageData:           imageData,
		AlphaChunk:          alphaChunk,
		AlphaData:           alphaData,
		alphaHeader:         alphaHeader,
	}, nil
}

// parseAnimationWebp parses an animated WebP container and returns frame-level chunk slices.
func parseAnimationWebp(data []byte) (parsedAnimationWebp, error) {
	riffSize, offset, err := parseRiff(data)
	if err != nil {
		return parsedAnimationWebp{}, err
	}
	riffLimit := riffLimitOf(riffSize)

	vp8x, nextOffset, err := parseVp8x(data, offset)
	if err != nil {
		return parsedAnimationWebp{}, err
	}
	offset = nextOffset
	if vp8x == nil {
		return parsedAnimationWebp{}, bitstreamErr("animated WebP requires VP8X")
	}
	if (vp8x.Flags & animationFlag) == 0 {
		return parsedAnimationWebp{}, unsupportedErr("animated WebP flag is not set")
	}

	animChunk, err := parseChunk(data, offset, riffLimit)
	if err != nil {
		return parsedAnimationWebp{}, err
	}
	if !bytes.Equal(animChunk.Fourcc[:], []byte("ANIM")) {
		return parsedAnimationWebp{}, bitstreamErr("missing ANIM chunk")
	}
	if animChunk.Size != 6 {
		return parsedAnimationWebp{}, bitstreamErr("wrong ANIM chunk size")
	}
	animation := animationHeader{
		BackgroundColor: uint32(readLE32(data[animChunk.DataOffset : animChunk.DataOffset+4])),
		LoopCount:       readLE16(data[animChunk.DataOffset+4 : animChunk.DataOffset+6]),
	}
	offset += chunkHeaderSize + animChunk.PaddedSize

	features := FeatureInfo{
		Width:        vp8x.CanvasWidth,
		Height:       vp8x.CanvasHeight,
		HasAlpha:     (vp8x.Flags & alphaFlag) != 0,
		HasAnimation: true,
		Format:       FormatUndefined,
		vp8x:         vp8x,
	}

	limit := len(data)
	if riffLimit != nil {
		limit = *riffLimit
	}
	var frames []parsedAnimationFrame
	for offset+chunkHeaderSize <= limit {
		chunk, err := parseChunk(data, offset, riffLimit)
		if err != nil {
			return parsedAnimationWebp{}, err
		}
		if !bytes.Equal(chunk.Fourcc[:], []byte("ANMF")) {
			break
		}
		frame, err := parseAnimationFrame(data, features, chunk, riffLimit)
		if err != nil {
			return parsedAnimationWebp{}, err
		}
		frames = append(frames, frame)
		offset += chunkHeaderSize + chunk.PaddedSize
	}

	if len(frames) == 0 {
		return parsedAnimationWebp{}, bitstreamErr("animated WebP has no ANMF frames")
	}

	return parsedAnimationWebp{
		Features:  features,
		RiffSize:  riffSize,
		Animation: animation,
		Frames:    frames,
	}, nil
}
