package webp

const exifFlag uint32 = 0x0000_0008

// stillImageChunk holds metadata and payload for a still-image WebP chunk.
type stillImageChunk struct {
	fourcc   [4]byte
	payload  []byte
	width    int
	height   int
	hasAlpha bool
}

func paddedLen(size int) (int, error) {
	if size < 0 {
		return 0, encInvalidParam("encoded output is too large")
	}
	return size + (size & 1), nil
}

func appendChunk(data *byteWriter, fourcc []byte, payload []byte) error {
	if uint64(len(payload)) > uint64(^uint32(0)) {
		return encInvalidParam("encoded output is too large")
	}
	data.writeBytes(fourcc)
	data.writeU32LE(uint32(len(payload)))
	data.writeBytes(payload)
	if len(payload)&1 == 1 {
		data.writeByte(0)
	}
	return nil
}

func extendRiff(body *byteWriter) ([]byte, error) {
	b := body.intoBytes()
	if uint64(len(b)) > uint64(^uint32(0)) {
		return nil, encInvalidParam("encoded output is too large")
	}
	riffSize := uint32(len(b))
	data := newByteWriter(8 + len(b))
	data.writeBytes([]byte("RIFF"))
	data.writeU32LE(riffSize)
	data.writeBytes(b)
	return data.intoBytes(), nil
}

func encodeLE24(value int) ([3]byte, error) {
	if value < 1 {
		return [3]byte{}, encInvalidParam("image dimensions must be non-zero")
	}
	encoded := value - 1
	if encoded > 0x00ff_ffff {
		return [3]byte{}, encInvalidParam("image dimensions exceed VP8X limits")
	}
	return [3]byte{byte(encoded), byte(encoded >> 8), byte(encoded >> 16)}, nil
}

// wrapStillWebp wraps an encoded still-image payload in a RIFF/WebP container.
func wrapStillWebp(image stillImageChunk, exif []byte) ([]byte, error) {
	paddedImageSize, err := paddedLen(len(image.payload))
	if err != nil {
		return nil, err
	}
	if exif == nil {
		bodySize := 4 + 8 + paddedImageSize
		body := newByteWriter(bodySize)
		body.writeBytes([]byte("WEBP"))
		if err := appendChunk(body, image.fourcc[:], image.payload); err != nil {
			return nil, err
		}
		return extendRiff(body)
	}

	vp8xPayloadSize := 10
	paddedExifSize, err := paddedLen(len(exif))
	if err != nil {
		return nil, err
	}
	bodySize := 4 + (8 + vp8xPayloadSize) + (8 + paddedImageSize) + (8 + paddedExifSize)
	body := newByteWriter(bodySize)
	body.writeBytes([]byte("WEBP"))

	flags := exifFlag
	if image.hasAlpha {
		flags |= alphaFlag
	}
	width, err := encodeLE24(image.width)
	if err != nil {
		return nil, err
	}
	height, err := encodeLE24(image.height)
	if err != nil {
		return nil, err
	}
	vp8xPayload := newByteWriter(10)
	vp8xPayload.writeU32LE(flags)
	vp8xPayload.writeBytes(width[:])
	vp8xPayload.writeBytes(height[:])

	if err := appendChunk(body, []byte("VP8X"), vp8xPayload.intoBytes()); err != nil {
		return nil, err
	}
	if err := appendChunk(body, image.fourcc[:], image.payload); err != nil {
		return nil, err
	}
	if err := appendChunk(body, []byte("EXIF"), exif); err != nil {
		return nil, err
	}
	return extendRiff(body)
}
