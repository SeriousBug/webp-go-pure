package webp

import "math/bits"

type Vp8FrameHeader struct {
	KeyFrame        bool
	Profile         uint8
	Show            bool
	PartitionLength int
}

type Vp8PictureHeader struct {
	Width      uint16
	Height     uint16
	XScale     uint8
	YScale     uint8
	Colorspace uint8
	ClampType  uint8
}

type SegmentHeader struct {
	UseSegment     bool
	UpdateMap      bool
	AbsoluteDelta  bool
	Quantizer      [NUM_MB_SEGMENTS]int8
	FilterStrength [NUM_MB_SEGMENTS]int8
	SegmentProbs   [MB_FEATURE_TREE_PROBS]uint8
}

func defaultSegmentHeader() SegmentHeader {
	return SegmentHeader{
		AbsoluteDelta: true,
		SegmentProbs:  [MB_FEATURE_TREE_PROBS]uint8{255, 255, 255},
	}
}

type FilterType int

const (
	FilterOff FilterType = iota
	FilterSimple
	FilterComplex
)

type FilterHeader struct {
	Simple      bool
	Level       uint8
	Sharpness   uint8
	UseLFDelta  bool
	RefLFDelta  [NUM_REF_LF_DELTAS]int8
	ModeLFDelta [NUM_MODE_LF_DELTAS]int8
	FilterType  FilterType
}

type LosslessInfo struct {
	Width    int
	Height   int
	HasAlpha bool
}

type LossyHeader struct {
	Frame               Vp8FrameHeader
	Picture             Vp8PictureHeader
	MacroblockWidth     int
	MacroblockHeight    int
	Segment             SegmentHeader
	Filter              FilterHeader
	TokenPartitionSizes []int
	Quantization        Quantization
	Probabilities       ProbabilityUpdateSummary
}

type MacroBlockHeaders struct {
	Frame       LossyHeader
	Macroblocks []MacroBlockHeader
}

type MacroBlockData struct {
	Header    MacroBlockHeader
	Coeffs    [384]int16
	NonZeroY  uint32
	NonZeroUV uint32
}

type MacroBlockDataFrame struct {
	Frame       LossyHeader
	Macroblocks []MacroBlockData
}

type nonZeroContext struct {
	nz   uint8
	nzDC uint8
}

// Vp8BoolDecoder is the VP8 boolean arithmetic decoder.
type Vp8BoolDecoder struct {
	data     []byte
	position int
	value    uint64
	rng      uint32
	bits     int32
	eof      bool
}

func NewVp8BoolDecoder(data []byte) *Vp8BoolDecoder {
	d := &Vp8BoolDecoder{
		data: data,
		rng:  255 - 1,
		bits: -8,
	}
	d.loadNewBytes()
	return d
}

func (d *Vp8BoolDecoder) EOF() bool { return d.eof }

func (d *Vp8BoolDecoder) loadNewBytes() {
	for d.bits < 0 {
		if d.position < len(d.data) {
			d.bits += 8
			d.value = (d.value << 8) | uint64(d.data[d.position])
			d.position++
		} else if !d.eof {
			d.value <<= 8
			d.bits += 8
			d.eof = true
		} else {
			d.bits = 0
		}
	}
}

func (d *Vp8BoolDecoder) Get() uint32 { return d.GetBit(0x80) }

func (d *Vp8BoolDecoder) GetValue(numBits int) uint32 {
	var value uint32
	for bitIndex := numBits - 1; bitIndex >= 0; bitIndex-- {
		value |= d.Get() << uint(bitIndex)
	}
	return value
}

func (d *Vp8BoolDecoder) GetSignedValue(numBits int) int32 {
	value := int32(d.GetValue(numBits))
	if d.Get() == 1 {
		return -value
	}
	return value
}

func (d *Vp8BoolDecoder) GetSigned(value int32) int32 {
	if d.Get() == 1 {
		return -value
	}
	return value
}

func (d *Vp8BoolDecoder) GetBit(prob uint8) uint32 {
	if d.bits < 0 {
		d.loadNewBytes()
	}

	pos := uint(d.bits)
	rng := d.rng
	split := (rng * uint32(prob)) >> 8
	value := uint32(d.value >> pos)
	var bit uint32
	if value > split {
		bit = 1
	}
	if bit == 1 {
		rng -= split
		d.value -= uint64(split+1) << pos
	} else {
		rng = split + 1
	}

	shift := int32(7) ^ int32(31-bits.LeadingZeros32(rng))
	rng <<= uint(shift)
	d.bits -= shift
	d.rng = rng - 1
	return bit
}

func checkLossySignature(data []byte) bool {
	return len(data) >= 3 && data[0] == 0x9d && data[1] == 0x01 && data[2] == 0x2a
}

func getInfo(data []byte, chunkSize int) (int, int, error) {
	if len(data) < VP8_FRAME_HEADER_SIZE {
		return 0, 0, notEnoughData("VP8 frame header")
	}
	if !checkLossySignature(data[3:]) {
		return 0, 0, bitstreamErr("bad VP8 signature")
	}

	b := uint32(data[0]) | (uint32(data[1]) << 8) | (uint32(data[2]) << 16)
	keyFrame := (b & 1) == 0
	profile := uint8((b >> 1) & 0x07)
	show := ((b >> 4) & 1) == 1
	partitionLength := int(b >> 5)
	width := int(((uint16(data[7]) << 8) | uint16(data[6])) & 0x3fff)
	height := int(((uint16(data[9]) << 8) | uint16(data[8])) & 0x3fff)

	if !keyFrame {
		return 0, 0, unsupportedErr("interframes are not supported")
	}
	if profile > 3 {
		return 0, 0, bitstreamErr("unknown VP8 profile")
	}
	if !show {
		return 0, 0, unsupportedErr("invisible VP8 frame")
	}
	if partitionLength >= chunkSize {
		return 0, 0, bitstreamErr("bad VP8 partition length")
	}
	if width == 0 || height == 0 {
		return 0, 0, bitstreamErr("invalid VP8 dimensions")
	}

	return width, height, nil
}

func checkLosslessSignature(data []byte) bool {
	return len(data) >= VP8L_FRAME_HEADER_SIZE && data[0] == 0x2f && (data[4]>>5) == 0
}

func getLosslessInfo(data []byte) (LosslessInfo, error) {
	if len(data) < VP8L_FRAME_HEADER_SIZE {
		return LosslessInfo{}, notEnoughData("VP8L frame header")
	}
	if !checkLosslessSignature(data) {
		return LosslessInfo{}, bitstreamErr("bad VP8L signature")
	}

	b := uint32(data[1]) | uint32(data[2])<<8 | uint32(data[3])<<16 | uint32(data[4])<<24
	width := int((b & 0x3fff) + 1)
	height := int(((b >> 14) & 0x3fff) + 1)
	hasAlpha := ((b >> 28) & 1) == 1
	version := (b >> 29) & 0x07

	if version != 0 {
		return LosslessInfo{}, bitstreamErr("unsupported VP8L version")
	}

	return LosslessInfo{Width: width, Height: height, HasAlpha: hasAlpha}, nil
}

func parseSegmentHeader(br *Vp8BoolDecoder) (SegmentHeader, error) {
	header := defaultSegmentHeader()
	header.UseSegment = br.Get() == 1
	if header.UseSegment {
		header.UpdateMap = br.Get() == 1
		if br.Get() == 1 {
			header.AbsoluteDelta = br.Get() == 1
			for i := range header.Quantizer {
				if br.Get() == 1 {
					header.Quantizer[i] = int8(br.GetSignedValue(7))
				} else {
					header.Quantizer[i] = 0
				}
			}
			for i := range header.FilterStrength {
				if br.Get() == 1 {
					header.FilterStrength[i] = int8(br.GetSignedValue(6))
				} else {
					header.FilterStrength[i] = 0
				}
			}
		}
		if header.UpdateMap {
			for i := range header.SegmentProbs {
				if br.Get() == 1 {
					header.SegmentProbs[i] = uint8(br.GetValue(8))
				} else {
					header.SegmentProbs[i] = 255
				}
			}
		}
	}

	if br.EOF() {
		return SegmentHeader{}, bitstreamErr("cannot parse segment header")
	}
	return header, nil
}

func parseFilterHeader(br *Vp8BoolDecoder) (FilterHeader, error) {
	simple := br.Get() == 1
	level := uint8(br.GetValue(6))
	sharpness := uint8(br.GetValue(3))
	useLFDelta := br.Get() == 1
	header := FilterHeader{
		Simple:     simple,
		Level:      level,
		Sharpness:  sharpness,
		UseLFDelta: useLFDelta,
		FilterType: FilterOff,
	}

	if useLFDelta && br.Get() == 1 {
		for i := range header.RefLFDelta {
			if br.Get() == 1 {
				header.RefLFDelta[i] = int8(br.GetSignedValue(6))
			}
		}
		for i := range header.ModeLFDelta {
			if br.Get() == 1 {
				header.ModeLFDelta[i] = int8(br.GetSignedValue(6))
			}
		}
	}

	if level == 0 {
		header.FilterType = FilterOff
	} else if simple {
		header.FilterType = FilterSimple
	} else {
		header.FilterType = FilterComplex
	}

	if br.EOF() {
		return FilterHeader{}, bitstreamErr("cannot parse filter header")
	}
	return header, nil
}

func parseTokenPartitions(br *Vp8BoolDecoder, data []byte) ([]int, error) {
	numPartsMinusOne := (1 << br.GetValue(2)) - 1
	if numPartsMinusOne >= MAX_NUM_PARTITIONS {
		return nil, bitstreamErr("too many VP8 token partitions")
	}

	sizeBytes := numPartsMinusOne * 3
	if len(data) < sizeBytes {
		return nil, notEnoughData("VP8 token partition sizes")
	}

	partitions := make([]int, 0, numPartsMinusOne+1)
	sizeLeft := len(data) - sizeBytes
	for i := 0; i < sizeBytes; i += 3 {
		chunk := data[i : i+3]
		stored := int(chunk[0]) | int(chunk[1])<<8 | int(chunk[2])<<16
		actual := stored
		if actual > sizeLeft {
			actual = sizeLeft
		}
		partitions = append(partitions, actual)
		sizeLeft -= actual
	}
	partitions = append(partitions, sizeLeft)

	if len(data) == sizeBytes {
		return nil, notEnoughData("VP8 token partitions")
	}
	return partitions, nil
}

var cat3 = [4]uint8{173, 148, 140, 0}
var cat4 = [5]uint8{176, 155, 140, 135, 0}
var cat5 = [6]uint8{180, 157, 141, 134, 130, 0}
var cat6 = [12]uint8{254, 254, 243, 230, 196, 177, 153, 140, 133, 130, 129, 0}
var zigzag = [16]int{0, 1, 4, 8, 5, 2, 3, 6, 9, 12, 13, 10, 7, 11, 14, 15}

func transformWHT(input *[16]int16) [16]int16 {
	var tmp [16]int32
	for i := 0; i < 4; i++ {
		a0 := int32(input[i]) + int32(input[12+i])
		a1 := int32(input[4+i]) + int32(input[8+i])
		a2 := int32(input[4+i]) - int32(input[8+i])
		a3 := int32(input[i]) - int32(input[12+i])
		tmp[i] = a0 + a1
		tmp[8+i] = a0 - a1
		tmp[4+i] = a3 + a2
		tmp[12+i] = a3 - a2
	}

	var out [16]int16
	for i := 0; i < 4; i++ {
		base := i * 4
		dc := tmp[base] + 3
		a0 := dc + tmp[base+3]
		a1 := tmp[base+1] + tmp[base+2]
		a2 := tmp[base+1] - tmp[base+2]
		a3 := dc - tmp[base+3]
		out[base] = int16((a0 + a1) >> 3)
		out[base+1] = int16((a3 + a2) >> 3)
		out[base+2] = int16((a0 - a1) >> 3)
		out[base+3] = int16((a3 - a2) >> 3)
	}
	return out
}

func getLargeValue(br *Vp8BoolDecoder, p *[11]uint8) int32 {
	if br.GetBit(p[3]) == 0 {
		if br.GetBit(p[4]) == 0 {
			return 2
		}
		return 3 + int32(br.GetBit(p[5]))
	} else if br.GetBit(p[6]) == 0 {
		if br.GetBit(p[7]) == 0 {
			return 5 + int32(br.GetBit(159))
		}
		return 7 + 2*int32(br.GetBit(165)) + int32(br.GetBit(145))
	}
	var cat int
	var table []uint8
	if br.GetBit(p[8]) == 0 {
		if br.GetBit(p[9]) == 0 {
			cat, table = 0, cat3[:]
		} else {
			cat, table = 1, cat4[:]
		}
	} else if br.GetBit(p[10]) == 0 {
		cat, table = 2, cat5[:]
	} else {
		cat, table = 3, cat6[:]
	}
	var value int32
	for _, prob := range table {
		if prob == 0 {
			break
		}
		value = value + value + int32(br.GetBit(prob))
	}
	return value + 3 + int32(8<<uint(cat))
}

func getCoeffs(
	br *Vp8BoolDecoder,
	probabilities *ProbabilityTables,
	coeffType int,
	ctx int,
	dq [2]uint16,
	start int,
	out []int16,
) int {
	n := start
	p := probabilities.CoeffProbs(coeffType, n, ctx)
	for n < 16 {
		if br.GetBit(p[0]) == 0 {
			return n
		}
		for br.GetBit(p[1]) == 0 {
			n++
			if n == 16 {
				return 16
			}
			p = probabilities.CoeffProbs(coeffType, n, 0)
		}

		var nextCtx int
		var value int32
		if br.GetBit(p[2]) == 0 {
			nextCtx = 1
			value = 1
		} else {
			nextCtx = 2
			value = getLargeValue(br, p)
		}
		var dequant int32
		if n > 0 {
			dequant = int32(dq[1])
		} else {
			dequant = int32(dq[0])
		}
		out[zigzag[n]] = int16(br.GetSigned(value) * dequant)
		n++
		p = probabilities.CoeffProbs(coeffType, n, nextCtx)
	}
	return 16
}

func nzCodeBits(nzCoeffs uint32, nz int, dcNZ bool) uint32 {
	var low uint32
	if nz > 3 {
		low = 3
	} else if nz > 1 {
		low = 2
	} else if dcNZ {
		low = 1
	} else {
		low = 0
	}
	return (nzCoeffs << 2) | low
}

func parseResiduals(
	header MacroBlockHeader,
	top *nonZeroContext,
	left *nonZeroContext,
	tokenBr *Vp8BoolDecoder,
	quantization *Quantization,
	probabilities *ProbabilityTables,
) MacroBlockData {
	var coeffs [384]int16
	if header.Skip {
		top.nz = 0
		left.nz = 0
		if !header.IsI4x4 {
			top.nzDC = 0
			left.nzDC = 0
		}
		return MacroBlockData{Header: header, Coeffs: coeffs}
	}

	q := &quantization.Matrices[header.Segment]
	offset := 0
	var first int
	var coeffType int
	if !header.IsI4x4 {
		var dc [16]int16
		ctx := int(top.nzDC + left.nzDC)
		nz := getCoeffs(tokenBr, probabilities, 1, ctx, q.Y2, 0, dc[:])
		hasDC := nz > 0
		if hasDC {
			top.nzDC = 1
			left.nzDC = 1
		} else {
			top.nzDC = 0
			left.nzDC = 0
		}
		if nz > 1 {
			transformed := transformWHT(&dc)
			for block, value := range transformed {
				coeffs[block*16] = value
			}
		} else {
			dc0 := int16((int32(dc[0]) + 3) >> 3)
			for block := 0; block < 16; block++ {
				coeffs[block*16] = dc0
			}
		}
		first = 1
		coeffType = 0
	} else {
		first = 0
		coeffType = 3
	}

	var nonZeroY uint32
	tnz := top.nz & 0x0f
	lnz := left.nz & 0x0f
	for y := 0; y < 4; y++ {
		l := lnz & 1
		var nzCoeffs uint32
		for x := 0; x < 4; x++ {
			ctx := int(l + (tnz & 1))
			nz := getCoeffs(tokenBr, probabilities, coeffType, ctx, q.Y1, first, coeffs[offset:offset+16])
			if nz > first {
				l = 1
			} else {
				l = 0
			}
			tnz = (tnz >> 1) | (l << 7)
			nzCoeffs = nzCodeBits(nzCoeffs, nz, coeffs[offset] != 0)
			offset += 16
		}
		tnz >>= 4
		lnz = (lnz >> 1) | (l << 7)
		nonZeroY = (nonZeroY << 8) | nzCoeffs
	}

	outTNZ := tnz
	outLNZ := lnz >> 4
	var nonZeroUV uint32
	for _, ch := range [2]uint{0, 2} {
		var nzCoeffs uint32
		tnz := top.nz >> (4 + ch)
		lnz := left.nz >> (4 + ch)
		for y := 0; y < 2; y++ {
			l := lnz & 1
			for x := 0; x < 2; x++ {
				ctx := int(l + (tnz & 1))
				nz := getCoeffs(tokenBr, probabilities, 2, ctx, q.UV, 0, coeffs[offset:offset+16])
				if nz > 0 {
					l = 1
				} else {
					l = 0
				}
				tnz = (tnz >> 1) | (l << 3)
				nzCoeffs = nzCodeBits(nzCoeffs, nz, coeffs[offset] != 0)
				offset += 16
			}
			tnz >>= 2
			lnz = (lnz >> 1) | (l << 5)
		}
		nonZeroUV |= nzCoeffs << (4 * ch)
		outTNZ |= (tnz << 4) << ch
		outLNZ |= (lnz & 0xf0) << ch
	}
	top.nz = outTNZ
	left.nz = outLNZ

	return MacroBlockData{
		Header:    header,
		Coeffs:    coeffs,
		NonZeroY:  nonZeroY,
		NonZeroUV: nonZeroUV,
	}
}

func parseLossyHeaders(data []byte) (LossyHeader, error) {
	if len(data) < VP8_FRAME_HEADER_SIZE {
		return LossyHeader{}, notEnoughData("VP8 frame header")
	}

	frameBits := uint32(data[0]) | (uint32(data[1]) << 8) | (uint32(data[2]) << 16)
	frame := Vp8FrameHeader{
		KeyFrame:        (frameBits & 1) == 0,
		Profile:         uint8((frameBits >> 1) & 0x07),
		Show:            ((frameBits >> 4) & 1) == 1,
		PartitionLength: int(frameBits >> 5),
	}
	if !frame.KeyFrame {
		return LossyHeader{}, unsupportedErr("interframes are not supported")
	}
	if frame.Profile > 3 {
		return LossyHeader{}, bitstreamErr("unknown VP8 profile")
	}
	if !frame.Show {
		return LossyHeader{}, unsupportedErr("invisible VP8 frame")
	}
	if !checkLossySignature(data[3:]) {
		return LossyHeader{}, bitstreamErr("bad VP8 signature")
	}

	picture := Vp8PictureHeader{
		Width:  ((uint16(data[7]) << 8) | uint16(data[6])) & 0x3fff,
		Height: ((uint16(data[9]) << 8) | uint16(data[8])) & 0x3fff,
		XScale: data[7] >> 6,
		YScale: data[9] >> 6,
	}
	if picture.Width == 0 || picture.Height == 0 {
		return LossyHeader{}, bitstreamErr("invalid VP8 dimensions")
	}

	partition0Offset := VP8_FRAME_HEADER_SIZE
	partition0End := partition0Offset + frame.PartitionLength
	if partition0End > len(data) {
		return LossyHeader{}, notEnoughData("VP8 partition 0")
	}

	br := NewVp8BoolDecoder(data[partition0Offset:partition0End])
	picture.Colorspace = uint8(br.Get())
	picture.ClampType = uint8(br.Get())

	segment, err := parseSegmentHeader(br)
	if err != nil {
		return LossyHeader{}, err
	}
	filter, err := parseFilterHeader(br)
	if err != nil {
		return LossyHeader{}, err
	}
	tokenPartitionSizes, err := parseTokenPartitions(br, data[partition0End:])
	if err != nil {
		return LossyHeader{}, err
	}
	quantization, err := parseQuantization(br, &segment)
	if err != nil {
		return LossyHeader{}, err
	}
	br.Get()
	probabilities, err := parseProbabilityUpdates(br)
	if err != nil {
		return LossyHeader{}, err
	}

	return LossyHeader{
		Frame:               frame,
		Picture:             picture,
		MacroblockWidth:     (int(picture.Width) + 15) >> 4,
		MacroblockHeight:    (int(picture.Height) + 15) >> 4,
		Segment:             segment,
		Filter:              filter,
		TokenPartitionSizes: tokenPartitionSizes,
		Quantization:        quantization,
		Probabilities:       probabilities,
	}, nil
}

func parseMacroblockHeaders(data []byte) (MacroBlockHeaders, error) {
	frame, err := parseLossyHeaders(data)
	if err != nil {
		return MacroBlockHeaders{}, err
	}

	partition0Offset := VP8_FRAME_HEADER_SIZE
	partition0End := partition0Offset + frame.Frame.PartitionLength
	br := NewVp8BoolDecoder(data[partition0Offset:partition0End])

	br.Get()
	br.Get()
	segment, err := parseSegmentHeader(br)
	if err != nil {
		return MacroBlockHeaders{}, err
	}
	if _, err := parseFilterHeader(br); err != nil {
		return MacroBlockHeaders{}, err
	}
	if _, err := parseTokenPartitions(br, data[partition0End:]); err != nil {
		return MacroBlockHeaders{}, err
	}
	if _, err := parseQuantization(br, &segment); err != nil {
		return MacroBlockHeaders{}, err
	}
	br.Get()
	probabilities, err := parseProbabilityUpdates(br)
	if err != nil {
		return MacroBlockHeaders{}, err
	}

	topModes := make([]uint8, frame.MacroblockWidth*4)
	for i := range topModes {
		topModes[i] = B_DC_PRED
	}
	macroblocks := make([]MacroBlockHeader, 0, frame.MacroblockWidth*frame.MacroblockHeight)
	skipProb := uint8(0)
	if probabilities.SkipProbability != nil {
		skipProb = *probabilities.SkipProbability
	}
	for mbY := 0; mbY < frame.MacroblockHeight; mbY++ {
		leftModes := [4]uint8{B_DC_PRED, B_DC_PRED, B_DC_PRED, B_DC_PRED}
		row, err := parseIntraModeRow(
			br,
			frame.MacroblockWidth,
			segment.UpdateMap,
			&segment.SegmentProbs,
			probabilities.UseSkipProbability,
			skipProb,
			topModes,
			&leftModes,
		)
		if err != nil {
			return MacroBlockHeaders{}, err
		}
		macroblocks = append(macroblocks, row...)
	}

	return MacroBlockHeaders{Frame: frame, Macroblocks: macroblocks}, nil
}

func parseMacroblockData(data []byte) (MacroBlockDataFrame, error) {
	frame, err := parseLossyHeaders(data)
	if err != nil {
		return MacroBlockDataFrame{}, err
	}
	partition0Offset := VP8_FRAME_HEADER_SIZE
	partition0End := partition0Offset + frame.Frame.PartitionLength
	br := NewVp8BoolDecoder(data[partition0Offset:partition0End])

	br.Get()
	br.Get()
	segment, err := parseSegmentHeader(br)
	if err != nil {
		return MacroBlockDataFrame{}, err
	}
	if _, err := parseFilterHeader(br); err != nil {
		return MacroBlockDataFrame{}, err
	}
	tokenPartitionSizes, err := parseTokenPartitions(br, data[partition0End:])
	if err != nil {
		return MacroBlockDataFrame{}, err
	}
	quantization, err := parseQuantization(br, &segment)
	if err != nil {
		return MacroBlockDataFrame{}, err
	}
	br.Get()
	probabilities, err := parseProbabilityTables(br)
	if err != nil {
		return MacroBlockDataFrame{}, err
	}

	partitionSizeBytes := (len(tokenPartitionSizes) - 1) * 3
	tokenOffset := partition0End + partitionSizeBytes
	tokenReaders := make([]*Vp8BoolDecoder, 0, len(tokenPartitionSizes))
	for _, size := range tokenPartitionSizes {
		end := tokenOffset + size
		tokenReaders = append(tokenReaders, NewVp8BoolDecoder(data[tokenOffset:end]))
		tokenOffset = end
	}

	topModes := make([]uint8, frame.MacroblockWidth*4)
	for i := range topModes {
		topModes[i] = B_DC_PRED
	}
	topContexts := make([]nonZeroContext, frame.MacroblockWidth)
	partMask := len(tokenReaders) - 1
	macroblocks := make([]MacroBlockData, 0, frame.MacroblockWidth*frame.MacroblockHeight)

	skipProb := uint8(0)
	if probabilities.Summary.SkipProbability != nil {
		skipProb = *probabilities.Summary.SkipProbability
	}
	for mbY := 0; mbY < frame.MacroblockHeight; mbY++ {
		leftModes := [4]uint8{B_DC_PRED, B_DC_PRED, B_DC_PRED, B_DC_PRED}
		row, err := parseIntraModeRow(
			br,
			frame.MacroblockWidth,
			segment.UpdateMap,
			&segment.SegmentProbs,
			probabilities.Summary.UseSkipProbability,
			skipProb,
			topModes,
			&leftModes,
		)
		if err != nil {
			return MacroBlockDataFrame{}, err
		}

		tokenBr := tokenReaders[mbY&partMask]
		var leftContext nonZeroContext
		for mbX, header := range row {
			mb := parseResiduals(header, &topContexts[mbX], &leftContext, tokenBr, &quantization, &probabilities)
			macroblocks = append(macroblocks, mb)
		}
	}

	return MacroBlockDataFrame{Frame: frame, Macroblocks: macroblocks}, nil
}
