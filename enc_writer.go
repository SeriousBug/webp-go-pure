package webp

// byteWriter is a growable little-endian byte sink for container assembly.
type byteWriter struct {
	bytes []byte
}

func newByteWriter(capacity int) *byteWriter {
	return &byteWriter{bytes: make([]byte, 0, capacity)}
}

func (w *byteWriter) writeByte(value byte) {
	w.bytes = append(w.bytes, value)
}

func (w *byteWriter) writeBytes(values []byte) {
	w.bytes = append(w.bytes, values...)
}

func (w *byteWriter) writeU16LE(value uint16) {
	w.bytes = append(w.bytes, byte(value), byte(value>>8))
}

func (w *byteWriter) writeU24LE(value uint32) {
	w.bytes = append(w.bytes, byte(value), byte(value>>8), byte(value>>16))
}

func (w *byteWriter) writeU32LE(value uint32) {
	w.bytes = append(w.bytes, byte(value), byte(value>>8), byte(value>>16), byte(value>>24))
}

func (w *byteWriter) intoBytes() []byte {
	return w.bytes
}

// bitWriter is an LSB-first bit writer used by the lossless VP8L encoder.
type bitWriter struct {
	bytes  []byte
	bitPos int
}

func newBitWriter() *bitWriter {
	return &bitWriter{}
}

// putBits appends numBits least-significant bits of value in LSB-first order.
func (w *bitWriter) putBits(value uint32, numBits int) error {
	if numBits > 32 {
		return encBitstream("bit write is too wide")
	}
	if numBits == 0 {
		return nil
	}
	if numBits < 32 {
		value &= (1 << uint(numBits)) - 1
	}

	byteIndex := w.bitPos >> 3
	bitOffset := uint(w.bitPos & 7)
	need := (w.bitPos + numBits + 7) >> 3
	for len(w.bytes) < need {
		w.bytes = append(w.bytes, 0)
	}

	w.bytes[byteIndex] |= byte(value << bitOffset)
	written := 8 - int(bitOffset)
	if written < numBits {
		v := value >> uint(written)
		byteIndex++
		remaining := numBits - written
		for remaining >= 8 {
			w.bytes[byteIndex] = byte(v)
			v >>= 8
			byteIndex++
			remaining -= 8
		}
		if remaining > 0 {
			w.bytes[byteIndex] |= byte(v)
		}
	}

	w.bitPos += numBits
	return nil
}

func (w *bitWriter) intoBytes() []byte {
	return w.bytes
}
