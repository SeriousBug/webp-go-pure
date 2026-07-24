package webp

import "math/bits"

// vp8BoolWriter is the boolean arithmetic writer used by the lossy VP8 encoder.
type vp8BoolWriter struct {
	rng    int32
	value  int32
	run    int
	nbBits int32
	bytes  []byte
}

func newVp8BoolWriter(expectedSize int) *vp8BoolWriter {
	return &vp8BoolWriter{
		rng:    255 - 1,
		nbBits: -8,
		bytes:  make([]byte, 0, expectedSize),
	}
}

func (w *vp8BoolWriter) flush() {
	shift := 8 + w.nbBits
	bitsV := w.value >> uint(shift)
	w.value -= bitsV << uint(shift)
	w.nbBits -= 8
	if (bitsV & 0xff) != 0xff {
		pos := len(w.bytes)
		if (bitsV&0x100) != 0 && pos > 0 {
			w.bytes[pos-1]++
		}
		if w.run > 0 {
			var value byte = 0xff
			if (bitsV & 0x100) != 0 {
				value = 0x00
			}
			for i := 0; i < w.run; i++ {
				w.bytes = append(w.bytes, value)
			}
			w.run = 0
		}
		w.bytes = append(w.bytes, byte(bitsV&0xff))
	} else {
		w.run++
	}
}

// putBit encodes one probability-weighted bit.
func (w *vp8BoolWriter) putBit(bit bool, prob uint8) bool {
	split := (w.rng * int32(prob)) >> 8
	if bit {
		w.value += split + 1
		w.rng -= split + 1
	} else {
		w.rng = split
	}
	if w.rng < 127 {
		shift := int32(7) - int32(bits.Len32(uint32(w.rng+1))-1)
		w.rng = ((w.rng + 1) << uint(shift)) - 1
		w.value <<= uint(shift)
		w.nbBits += shift
		if w.nbBits > 0 {
			w.flush()
		}
	}
	return bit
}

// putBitUniform encodes one unbiased bit.
func (w *vp8BoolWriter) putBitUniform(bit bool) bool {
	split := w.rng >> 1
	if bit {
		w.value += split + 1
		w.rng -= split + 1
	} else {
		w.rng = split
	}
	if w.rng < 127 {
		w.rng = ((w.rng + 1) << 1) - 1
		w.value <<= 1
		w.nbBits++
		if w.nbBits > 0 {
			w.flush()
		}
	}
	return bit
}

// putBits encodes a fixed-width unsigned value in MSB-first order.
func (w *vp8BoolWriter) putBits(value uint32, numBits int) {
	for shift := numBits - 1; shift >= 0; shift-- {
		w.putBitUniform(((value >> uint(shift)) & 1) != 0)
	}
}

// putSignedBits encodes a signed magnitude value using the VP8 boolean layout.
func (w *vp8BoolWriter) putSignedBits(value int32, numBits int) {
	if !w.putBitUniform(value != 0) {
		return
	}
	if value < 0 {
		w.putBits((uint32(-value)<<1)|1, numBits+1)
	} else {
		w.putBits(uint32(value)<<1, numBits+1)
	}
}

// finish flushes the remaining coder state and returns the final byte stream.
func (w *vp8BoolWriter) finish() []byte {
	w.putBits(0, int(9-w.nbBits))
	w.nbBits = 0
	w.flush()
	return w.bytes
}
