package webp

var dcTable = [128]uint8{
	4, 5, 6, 7, 8, 9, 10, 10, 11, 12, 13, 14, 15, 16, 17, 17, 18, 19, 20, 20, 21, 21, 22, 22, 23,
	23, 24, 25, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 37, 38, 39, 40, 41, 42, 43, 44,
	45, 46, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63, 64, 65, 66, 67,
	68, 69, 70, 71, 72, 73, 74, 75, 76, 76, 77, 78, 79, 80, 81, 82, 83, 84, 85, 86, 87, 88, 89, 91,
	93, 95, 96, 98, 100, 101, 102, 104, 106, 108, 110, 112, 114, 116, 118, 122, 124, 126, 128, 130,
	132, 134, 136, 138, 140, 143, 145, 148, 151, 154, 157,
}

var acTable = [128]uint16{
	4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28,
	29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52,
	53, 54, 55, 56, 57, 58, 60, 62, 64, 66, 68, 70, 72, 74, 76, 78, 80, 82, 84, 86, 88, 90, 92, 94,
	96, 98, 100, 102, 104, 106, 108, 110, 112, 114, 116, 119, 122, 125, 128, 131, 134, 137, 140,
	143, 146, 149, 152, 155, 158, 161, 164, 167, 170, 173, 177, 181, 185, 189, 193, 197, 201, 205,
	209, 213, 217, 221, 225, 229, 234, 239, 245, 249, 254, 259, 264, 269, 274, 279, 284,
}

type QuantIndices struct {
	BaseQ0    int32
	Y1DCDelta int32
	Y2DCDelta int32
	Y2ACDelta int32
	UVDCDelta int32
	UVACDelta int32
}

type QuantMatrix struct {
	Y1      [2]uint16
	Y2      [2]uint16
	UV      [2]uint16
	UVQuant int32
}

type Quantization struct {
	Indices  QuantIndices
	Matrices [NUM_MB_SEGMENTS]QuantMatrix
}

func clipQ(v, max int32) int {
	if v < 0 {
		v = 0
	}
	if v > max {
		v = max
	}
	return int(v)
}

func parseQuantization(br *Vp8BoolDecoder, segmentHeader *SegmentHeader) (Quantization, error) {
	indices := QuantIndices{BaseQ0: int32(br.GetValue(7))}
	if br.Get() == 1 {
		indices.Y1DCDelta = int32(br.GetSignedValue(4))
	}
	if br.Get() == 1 {
		indices.Y2DCDelta = int32(br.GetSignedValue(4))
	}
	if br.Get() == 1 {
		indices.Y2ACDelta = int32(br.GetSignedValue(4))
	}
	if br.Get() == 1 {
		indices.UVDCDelta = int32(br.GetSignedValue(4))
	}
	if br.Get() == 1 {
		indices.UVACDelta = int32(br.GetSignedValue(4))
	}

	var matrices [NUM_MB_SEGMENTS]QuantMatrix
	for segment := 0; segment < NUM_MB_SEGMENTS; segment++ {
		var q int32
		if segmentHeader.UseSegment {
			q = int32(segmentHeader.Quantizer[segment])
			if !segmentHeader.AbsoluteDelta {
				q += indices.BaseQ0
			}
		} else {
			q = indices.BaseQ0
		}

		var matrix QuantMatrix
		matrix.Y1[0] = uint16(dcTable[clipQ(q+indices.Y1DCDelta, 127)])
		matrix.Y1[1] = acTable[clipQ(q, 127)]

		matrix.Y2[0] = uint16(dcTable[clipQ(q+indices.Y2DCDelta, 127)]) * 2
		y2ac := (uint32(acTable[clipQ(q+indices.Y2ACDelta, 127)]) * 101_581) >> 16
		if y2ac < 8 {
			y2ac = 8
		}
		matrix.Y2[1] = uint16(y2ac)

		matrix.UV[0] = uint16(dcTable[clipQ(q+indices.UVDCDelta, 117)])
		matrix.UV[1] = acTable[clipQ(q+indices.UVACDelta, 127)]
		matrix.UVQuant = q + indices.UVACDelta
		matrices[segment] = matrix
	}

	if br.EOF() {
		return Quantization{}, bitstreamErr("cannot parse quantization")
	}

	return Quantization{Indices: indices, Matrices: matrices}, nil
}
