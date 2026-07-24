package webp

import "bytes"

// ChunkHeader holds common metadata for a RIFF chunk.
type ChunkHeader struct {
	Fourcc     [4]byte
	Offset     int
	Size       int
	PaddedSize int
	DataOffset int
}

// Vp8xHeader is a parsed VP8X extended header.
type Vp8xHeader struct {
	Flags        uint32
	CanvasWidth  int
	CanvasHeight int
}

// WebpFeatures are high-level image features derived from the container and bitstream.
type WebpFeatures struct {
	Width        int
	Height       int
	HasAlpha     bool
	HasAnimation bool
	Format       WebpFormat
	Vp8x         *Vp8xHeader
}

// ParsedWebp is a parsed still-image WebP container with raw chunk slices.
type ParsedWebp struct {
	Features    WebpFeatures
	RiffSize    *int
	ImageChunk  ChunkHeader
	ImageData   []byte
	AlphaChunk  *ChunkHeader
	AlphaData   []byte
	AlphaHeader *AlphaHeader
}

// AnimationHeader is a parsed ANIM chunk.
type AnimationHeader struct {
	BackgroundColor uint32
	LoopCount       uint16
}

// ParsedAnimationFrame is a parsed animation frame entry.
type ParsedAnimationFrame struct {
	FrameChunk         ChunkHeader
	XOffset            int
	YOffset            int
	Width              int
	Height             int
	Duration           int
	Blend              bool
	DisposeToBackground bool
	ImageChunk         ChunkHeader
	ImageData          []byte
	AlphaChunk         *ChunkHeader
	AlphaData          []byte
	AlphaHeader        *AlphaHeader
}

// ParsedAnimationWebp is a parsed animated WebP container.
type ParsedAnimationWebp struct {
	Features  WebpFeatures
	RiffSize  *int
	Animation AnimationHeader
	Frames    []ParsedAnimationFrame
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

func parseChunk(data []byte, offset int, riffLimit *int) (ChunkHeader, error) {
	if len(data) < offset+CHUNK_HEADER_SIZE {
		return ChunkHeader{}, notEnoughData("chunk header")
	}
	size := readLE32(data[offset+TAG_SIZE : offset+CHUNK_HEADER_SIZE])
	if size > MAX_CHUNK_PAYLOAD {
		return ChunkHeader{}, bitstreamErr("invalid chunk size")
	}

	paddedSize := paddedPayloadSize(size)
	totalSize := CHUNK_HEADER_SIZE + paddedSize
	end := offset + totalSize
	if riffLimit != nil && end > *riffLimit {
		return ChunkHeader{}, bitstreamErr("chunk exceeds RIFF payload")
	}
	if len(data) < end {
		return ChunkHeader{}, notEnoughData("chunk payload")
	}

	var fourcc [4]byte
	copy(fourcc[:], data[offset:offset+TAG_SIZE])
	return ChunkHeader{
		Fourcc:     fourcc,
		Offset:     offset,
		Size:       size,
		PaddedSize: paddedSize,
		DataOffset: offset + CHUNK_HEADER_SIZE,
	}, nil
}

func parseRiff(data []byte) (*int, int, error) {
	if len(data) < RIFF_HEADER_SIZE {
		return nil, 0, notEnoughData("RIFF header")
	}
	if !bytes.Equal(data[:4], []byte("RIFF")) {
		return nil, 0, nil
	}
	if !bytes.Equal(data[8:12], []byte("WEBP")) {
		return nil, 0, bitstreamErr("wrong RIFF WEBP signature")
	}

	riffSize := readLE32(data[4:8])
	if riffSize < TAG_SIZE+CHUNK_HEADER_SIZE {
		return nil, 0, bitstreamErr("RIFF payload is too small")
	}
	if riffSize > MAX_CHUNK_PAYLOAD {
		return nil, 0, bitstreamErr("RIFF payload is too large")
	}
	if riffSize > len(data)-CHUNK_HEADER_SIZE {
		return nil, 0, notEnoughData("truncated RIFF payload")
	}

	return &riffSize, RIFF_HEADER_SIZE, nil
}

func parseVp8x(data []byte, offset int) (*Vp8xHeader, int, error) {
	if len(data) < offset+CHUNK_HEADER_SIZE {
		return nil, offset, nil
	}
	if !bytes.Equal(data[offset:offset+TAG_SIZE], []byte("VP8X")) {
		return nil, offset, nil
	}

	chunk, err := parseChunk(data, offset, nil)
	if err != nil {
		return nil, offset, err
	}
	if chunk.Size != VP8X_CHUNK_SIZE {
		return nil, offset, bitstreamErr("wrong VP8X chunk size")
	}

	flags := uint32(readLE32(data[offset+8 : offset+12]))
	canvasWidth := readLE24(data[offset+12:offset+15]) + 1
	canvasHeight := readLE24(data[offset+15:offset+18]) + 1
	if uint64(canvasWidth)*uint64(canvasHeight) >= MAX_IMAGE_AREA {
		return nil, offset, bitstreamErr("canvas is too large")
	}

	return &Vp8xHeader{
		Flags:        flags,
		CanvasWidth:  canvasWidth,
		CanvasHeight: canvasHeight,
	}, offset + CHUNK_HEADER_SIZE + chunk.PaddedSize, nil
}

func riffLimitOf(riffSize *int) *int {
	if riffSize == nil {
		return nil
	}
	limit := *riffSize + CHUNK_HEADER_SIZE
	return &limit
}

// GetFeatures returns high-level WebP features without fully decoding the image.
func GetFeatures(data []byte) (WebpFeatures, error) {
	riffSize, offset, err := parseRiff(data)
	if err != nil {
		return WebpFeatures{}, err
	}
	riffLimit := riffLimitOf(riffSize)

	vp8x, nextOffset, err := parseVp8x(data, offset)
	if err != nil {
		return WebpFeatures{}, err
	}
	offset = nextOffset
	if riffSize == nil && vp8x != nil {
		return WebpFeatures{}, bitstreamErr("VP8X chunk requires RIFF")
	}

	hasAlpha := vp8x != nil && (vp8x.Flags&ALPHA_FLAG) != 0
	hasAnimation := vp8x != nil && (vp8x.Flags&ANIMATION_FLAG) != 0

	if vp8x != nil && hasAnimation {
		return WebpFeatures{
			Width:        vp8x.CanvasWidth,
			Height:       vp8x.CanvasHeight,
			HasAlpha:     hasAlpha,
			HasAnimation: hasAnimation,
			Format:       FormatUndefined,
			Vp8x:         vp8x,
		}, nil
	}

	if len(data) < offset+TAG_SIZE {
		return WebpFeatures{}, notEnoughData("chunk tag")
	}

	if (riffSize != nil && vp8x != nil) ||
		(riffSize == nil && vp8x == nil && bytes.Equal(data[offset:offset+TAG_SIZE], []byte("ALPH"))) {
		for {
			chunk, err := parseChunk(data, offset, riffLimit)
			if err != nil {
				return WebpFeatures{}, err
			}
			if bytes.Equal(chunk.Fourcc[:], []byte("VP8 ")) || bytes.Equal(chunk.Fourcc[:], []byte("VP8L")) {
				break
			}
			if bytes.Equal(chunk.Fourcc[:], []byte("ALPH")) {
				hasAlpha = true
			}
			offset += CHUNK_HEADER_SIZE + chunk.PaddedSize
		}
	}

	chunk, err := parseChunk(data, offset, riffLimit)
	if err != nil {
		return WebpFeatures{}, err
	}
	payload := data[chunk.DataOffset : chunk.DataOffset+chunk.Size]
	var format WebpFormat
	var width, height int
	if bytes.Equal(chunk.Fourcc[:], []byte("VP8 ")) {
		width, height, err = getInfo(payload, chunk.Size)
		if err != nil {
			return WebpFeatures{}, err
		}
		format = FormatLossy
	} else if bytes.Equal(chunk.Fourcc[:], []byte("VP8L")) {
		info, err := getLosslessInfo(payload)
		if err != nil {
			return WebpFeatures{}, err
		}
		hasAlpha = hasAlpha || info.HasAlpha
		format, width, height = FormatLossless, info.Width, info.Height
	} else {
		return WebpFeatures{}, bitstreamErr("missing VP8/VP8L image chunk")
	}

	if vp8x != nil {
		if vp8x.CanvasWidth != width || vp8x.CanvasHeight != height {
			return WebpFeatures{}, bitstreamErr("VP8X canvas does not match image size")
		}
	}

	return WebpFeatures{
		Width:        width,
		Height:       height,
		HasAlpha:     hasAlpha,
		HasAnimation: hasAnimation,
		Format:       format,
		Vp8x:         vp8x,
	}, nil
}

// ParseStillWebp parses a still-image WebP container and returns raw chunk slices.
func ParseStillWebp(data []byte) (ParsedWebp, error) {
	riffSize, offset, err := parseRiff(data)
	if err != nil {
		return ParsedWebp{}, err
	}
	riffLimit := riffLimitOf(riffSize)

	vp8x, nextOffset, err := parseVp8x(data, offset)
	if err != nil {
		return ParsedWebp{}, err
	}
	offset = nextOffset
	if riffSize == nil && vp8x != nil {
		return ParsedWebp{}, bitstreamErr("VP8X chunk requires RIFF")
	}
	if vp8x != nil && (vp8x.Flags&ANIMATION_FLAG) != 0 {
		return ParsedWebp{}, unsupportedErr("animated WebP is not implemented")
	}

	var alphaChunk *ChunkHeader
	if len(data) < offset+TAG_SIZE {
		return ParsedWebp{}, notEnoughData("chunk tag")
	}
	if (riffSize != nil && vp8x != nil) ||
		(riffSize == nil && vp8x == nil && bytes.Equal(data[offset:offset+TAG_SIZE], []byte("ALPH"))) {
		for {
			chunk, err := parseChunk(data, offset, riffLimit)
			if err != nil {
				return ParsedWebp{}, err
			}
			if bytes.Equal(chunk.Fourcc[:], []byte("VP8 ")) || bytes.Equal(chunk.Fourcc[:], []byte("VP8L")) {
				break
			}
			if bytes.Equal(chunk.Fourcc[:], []byte("ALPH")) {
				c := chunk
				alphaChunk = &c
			}
			offset += CHUNK_HEADER_SIZE + chunk.PaddedSize
		}
	}

	imageChunk, err := parseChunk(data, offset, riffLimit)
	if err != nil {
		return ParsedWebp{}, err
	}
	if !bytes.Equal(imageChunk.Fourcc[:], []byte("VP8 ")) && !bytes.Equal(imageChunk.Fourcc[:], []byte("VP8L")) {
		return ParsedWebp{}, bitstreamErr("missing VP8/VP8L image chunk")
	}
	imageData := data[imageChunk.DataOffset : imageChunk.DataOffset+imageChunk.Size]
	features, err := GetFeatures(data)
	if err != nil {
		return ParsedWebp{}, err
	}
	var alphaData []byte
	var alphaHeader *AlphaHeader
	if alphaChunk != nil {
		alphaData = data[alphaChunk.DataOffset : alphaChunk.DataOffset+alphaChunk.Size]
		h, err := parseAlphaHeader(alphaData)
		if err != nil {
			return ParsedWebp{}, err
		}
		alphaHeader = &h
		features.HasAlpha = true
	}

	return ParsedWebp{
		Features:    features,
		RiffSize:    riffSize,
		ImageChunk:  imageChunk,
		ImageData:   imageData,
		AlphaChunk:  alphaChunk,
		AlphaData:   alphaData,
		AlphaHeader: alphaHeader,
	}, nil
}

func parseAnimationFrame(data []byte, features WebpFeatures, chunk ChunkHeader, riffLimit *int) (ParsedAnimationFrame, error) {
	if chunk.Size < 16 {
		return ParsedAnimationFrame{}, bitstreamErr("ANMF chunk is too small")
	}

	header := data[chunk.DataOffset : chunk.DataOffset+16]
	xOffset := readLE24(header[0:3]) * 2
	yOffset := readLE24(header[3:6]) * 2
	width := readLE24(header[6:9]) + 1
	height := readLE24(header[9:12]) + 1
	duration := readLE24(header[12:15])
	flags := header[15]
	if flags>>2 != 0 {
		return ParsedAnimationFrame{}, bitstreamErr("ANMF reserved bits must be zero")
	}
	if xOffset+width > features.Width || yOffset+height > features.Height {
		return ParsedAnimationFrame{}, bitstreamErr("ANMF frame exceeds animation canvas")
	}

	offset := chunk.DataOffset + 16
	frameEnd := chunk.DataOffset + chunk.Size
	frameLimit := &frameEnd
	var alphaChunk *ChunkHeader
	var imageChunk ChunkHeader
	for {
		subchunk, err := parseChunk(data, offset, frameLimit)
		if err != nil {
			return ParsedAnimationFrame{}, err
		}
		if bytes.Equal(subchunk.Fourcc[:], []byte("VP8 ")) || bytes.Equal(subchunk.Fourcc[:], []byte("VP8L")) {
			imageChunk = subchunk
			break
		}
		if bytes.Equal(subchunk.Fourcc[:], []byte("ALPH")) {
			c := subchunk
			alphaChunk = &c
		}
		offset += CHUNK_HEADER_SIZE + subchunk.PaddedSize
		if riffLimit != nil && offset > *riffLimit {
			return ParsedAnimationFrame{}, bitstreamErr("ANMF frame data exceeds RIFF payload")
		}
	}

	imageData := data[imageChunk.DataOffset : imageChunk.DataOffset+imageChunk.Size]
	var alphaData []byte
	var alphaHeader *AlphaHeader
	if alphaChunk != nil {
		alphaData = data[alphaChunk.DataOffset : alphaChunk.DataOffset+alphaChunk.Size]
		h, err := parseAlphaHeader(alphaData)
		if err != nil {
			return ParsedAnimationFrame{}, err
		}
		alphaHeader = &h
	}

	return ParsedAnimationFrame{
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
		AlphaHeader:         alphaHeader,
	}, nil
}

// ParseAnimationWebp parses an animated WebP container and returns frame-level chunk slices.
func ParseAnimationWebp(data []byte) (ParsedAnimationWebp, error) {
	riffSize, offset, err := parseRiff(data)
	if err != nil {
		return ParsedAnimationWebp{}, err
	}
	riffLimit := riffLimitOf(riffSize)

	vp8x, nextOffset, err := parseVp8x(data, offset)
	if err != nil {
		return ParsedAnimationWebp{}, err
	}
	offset = nextOffset
	if vp8x == nil {
		return ParsedAnimationWebp{}, bitstreamErr("animated WebP requires VP8X")
	}
	if (vp8x.Flags & ANIMATION_FLAG) == 0 {
		return ParsedAnimationWebp{}, unsupportedErr("animated WebP flag is not set")
	}

	animChunk, err := parseChunk(data, offset, riffLimit)
	if err != nil {
		return ParsedAnimationWebp{}, err
	}
	if !bytes.Equal(animChunk.Fourcc[:], []byte("ANIM")) {
		return ParsedAnimationWebp{}, bitstreamErr("missing ANIM chunk")
	}
	if animChunk.Size != 6 {
		return ParsedAnimationWebp{}, bitstreamErr("wrong ANIM chunk size")
	}
	animation := AnimationHeader{
		BackgroundColor: uint32(readLE32(data[animChunk.DataOffset : animChunk.DataOffset+4])),
		LoopCount:       readLE16(data[animChunk.DataOffset+4 : animChunk.DataOffset+6]),
	}
	offset += CHUNK_HEADER_SIZE + animChunk.PaddedSize

	features := WebpFeatures{
		Width:        vp8x.CanvasWidth,
		Height:       vp8x.CanvasHeight,
		HasAlpha:     (vp8x.Flags & ALPHA_FLAG) != 0,
		HasAnimation: true,
		Format:       FormatUndefined,
		Vp8x:         vp8x,
	}

	limit := len(data)
	if riffLimit != nil {
		limit = *riffLimit
	}
	var frames []ParsedAnimationFrame
	for offset+CHUNK_HEADER_SIZE <= limit {
		chunk, err := parseChunk(data, offset, riffLimit)
		if err != nil {
			return ParsedAnimationWebp{}, err
		}
		if !bytes.Equal(chunk.Fourcc[:], []byte("ANMF")) {
			break
		}
		frame, err := parseAnimationFrame(data, features, chunk, riffLimit)
		if err != nil {
			return ParsedAnimationWebp{}, err
		}
		frames = append(frames, frame)
		offset += CHUNK_HEADER_SIZE + chunk.PaddedSize
	}

	if len(frames) == 0 {
		return ParsedAnimationWebp{}, bitstreamErr("animated WebP has no ANMF frames")
	}

	return ParsedAnimationWebp{
		Features:  features,
		RiffSize:  riffSize,
		Animation: animation,
		Frames:    frames,
	}, nil
}
