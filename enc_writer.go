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
	for bitIndex := 0; bitIndex < numBits; bitIndex++ {
		byteIndex := w.bitPos >> 3
		if byteIndex == len(w.bytes) {
			w.bytes = append(w.bytes, 0)
		}
		bit := byte((value >> uint(bitIndex)) & 1)
		w.bytes[byteIndex] |= bit << uint(w.bitPos&7)
		w.bitPos++
	}
	return nil
}

func (w *bitWriter) intoBytes() []byte {
	return w.bytes
}
