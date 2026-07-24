package webp

const (
	lldecARGBBlack            = 0xff000000
	lldecMaxAllowedCodeLength = 15
	lldecMaxCacheBits         = 11
	lldecNumLiteralCodes      = 256
	lldecNumLengthCodes       = 24
	lldecNumDistanceCodes     = 40
	lldecNumCodeLengthCodes   = 19
	lldecMinHuffmanBits       = 2
	lldecNumHuffmanBits       = 3
	lldecMinTransformBits     = 2
	lldecNumTransformBits     = 3
	lldecDefaultCodeLength    = 8
	lldecCodeLengthRepeatCode = 16
	lldecColorCacheHashMul    = 0x1e35a7bd
)

var lldecCodeLengthExtraBits = [3]int{2, 3, 7}
var lldecCodeLengthRepeatOffsets = [3]int{3, 3, 11}
var lldecCodeLengthCodeOrder = [lldecNumCodeLengthCodes]int{
	17, 18, 0, 1, 2, 3, 4, 5, 16, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15,
}
var lldecCodeToPlane = [120]uint8{
	0x18, 0x07, 0x17, 0x19, 0x28, 0x06, 0x27, 0x29, 0x16, 0x1a, 0x26, 0x2a, 0x38, 0x05, 0x37, 0x39,
	0x15, 0x1b, 0x36, 0x3a, 0x25, 0x2b, 0x48, 0x04, 0x47, 0x49, 0x14, 0x1c, 0x35, 0x3b, 0x46, 0x4a,
	0x24, 0x2c, 0x58, 0x45, 0x4b, 0x34, 0x3c, 0x03, 0x57, 0x59, 0x13, 0x1d, 0x56, 0x5a, 0x23, 0x2d,
	0x44, 0x4c, 0x55, 0x5b, 0x33, 0x3d, 0x68, 0x02, 0x67, 0x69, 0x12, 0x1e, 0x66, 0x6a, 0x22, 0x2e,
	0x54, 0x5c, 0x43, 0x4d, 0x65, 0x6b, 0x32, 0x3e, 0x78, 0x01, 0x77, 0x79, 0x53, 0x5d, 0x11, 0x1f,
	0x64, 0x6c, 0x42, 0x4e, 0x76, 0x7a, 0x21, 0x2f, 0x75, 0x7b, 0x31, 0x3f, 0x63, 0x6d, 0x52, 0x5e,
	0x00, 0x74, 0x7c, 0x41, 0x4f, 0x10, 0x20, 0x62, 0x6e, 0x30, 0x73, 0x7d, 0x51, 0x5f, 0x40, 0x72,
	0x7e, 0x61, 0x6f, 0x50, 0x71, 0x7f, 0x60, 0x70,
}

type lldecBitReader struct {
	data   []byte
	bitPos int
}

func lldecNewBitReader(data []byte) lldecBitReader {
	return lldecBitReader{data: data, bitPos: 0}
}

func (br *lldecBitReader) readBit() (uint32, error) {
	return br.readBits(1)
}

func (br *lldecBitReader) readBits(numBits int) (uint32, error) {
	if numBits > 24 {
		return 0, invalidParam("VP8L bit read is too wide")
	}
	end := br.bitPos + numBits
	if end < br.bitPos {
		return 0, bitstreamErr("VP8L bit position overflow")
	}
	if end > len(br.data)*8 {
		return 0, notEnoughData("VP8L bitstream")
	}

	var value uint32
	for bitIndex := 0; bitIndex < numBits; bitIndex++ {
		streamBit := br.bitPos + bitIndex
		b := br.data[streamBit>>3]
		bit := (b >> (streamBit & 7)) & 1
		value |= uint32(bit) << bitIndex
	}
	br.bitPos = end
	return value, nil
}

type lldecHuffmanTree struct {
	singleSymbol int32
	byLen        []map[uint16]uint16
	maxLen       int
}

func lldecHuffmanFromCodeLengths(codeLengths []uint8) (lldecHuffmanTree, error) {
	var counts [lldecMaxAllowedCodeLength + 1]int32
	singleSymbol := int32(-1)
	numSymbols := 0

	for symbol, l := range codeLengths {
		bits := int(l)
		if bits > lldecMaxAllowedCodeLength {
			return lldecHuffmanTree{}, bitstreamErr("invalid VP8L Huffman code length")
		}
		if bits > 0 {
			counts[bits]++
			singleSymbol = int32(uint16(symbol))
			numSymbols++
		}
	}

	if numSymbols == 0 {
		return lldecHuffmanTree{}, bitstreamErr("empty VP8L Huffman tree")
	}
	if numSymbols == 1 {
		return lldecHuffmanTree{singleSymbol: singleSymbol, byLen: nil, maxLen: 0}, nil
	}

	left := int32(1)
	for bits := 1; bits <= lldecMaxAllowedCodeLength; bits++ {
		left = (left << 1) - counts[bits]
		if left < 0 {
			return lldecHuffmanTree{}, bitstreamErr("oversubscribed VP8L Huffman tree")
		}
	}
	if left != 0 {
		return lldecHuffmanTree{}, bitstreamErr("incomplete VP8L Huffman tree")
	}

	var nextCode [lldecMaxAllowedCodeLength + 1]uint32
	code := uint32(0)
	for bits := 1; bits <= lldecMaxAllowedCodeLength; bits++ {
		code = (code + uint32(counts[bits-1])) << 1
		nextCode[bits] = code
	}

	byLen := make([]map[uint16]uint16, lldecMaxAllowedCodeLength+1)
	for i := range byLen {
		byLen[i] = make(map[uint16]uint16)
	}
	maxLen := 0

	for symbol, l := range codeLengths {
		bits := int(l)
		if bits == 0 {
			continue
		}
		canonical := nextCode[bits]
		nextCode[bits]++
		byLen[bits][lldecReverseBits(canonical, bits)] = uint16(symbol)
		if bits > maxLen {
			maxLen = bits
		}
	}

	return lldecHuffmanTree{singleSymbol: -1, byLen: byLen, maxLen: maxLen}, nil
}

func (t *lldecHuffmanTree) readSymbol(br *lldecBitReader) (uint16, error) {
	if t.singleSymbol >= 0 {
		return uint16(t.singleSymbol), nil
	}

	code := uint16(0)
	for bits := 1; bits <= t.maxLen; bits++ {
		bit, err := br.readBit()
		if err != nil {
			return 0, err
		}
		code |= uint16(bit) << (bits - 1)
		if symbol, ok := t.byLen[bits][code]; ok {
			return symbol, nil
		}
	}

	return 0, bitstreamErr("invalid VP8L Huffman symbol")
}

type lldecColorCache struct {
	colors    []uint32
	hashShift uint32
}

func lldecNewColorCache(hashBits int) (lldecColorCache, error) {
	if hashBits < 1 || hashBits > lldecMaxCacheBits {
		return lldecColorCache{}, bitstreamErr("invalid VP8L color cache size")
	}
	size := 1 << hashBits
	return lldecColorCache{
		colors:    make([]uint32, size),
		hashShift: uint32(32 - hashBits),
	}, nil
}

func (c *lldecColorCache) insert(argb uint32) {
	key := int((argb * lldecColorCacheHashMul) >> c.hashShift)
	c.colors[key] = argb
}

func (c *lldecColorCache) lookup(key int) (uint32, error) {
	if key < 0 || key >= len(c.colors) {
		return 0, bitstreamErr("invalid VP8L color cache lookup")
	}
	return c.colors[key], nil
}

type lldecTransformType int

const (
	lldecPredictor lldecTransformType = iota
	lldecCrossColor
	lldecSubtractGreen
	lldecColorIndexing
)

type lldecTransform struct {
	kind  lldecTransformType
	bits  int
	xsize int
	ysize int
	data  []uint32
}

type lldecHTreeGroup struct {
	green lldecHuffmanTree
	red   lldecHuffmanTree
	blue  lldecHuffmanTree
	alpha lldecHuffmanTree
	dist  lldecHuffmanTree
}

type lldecHuffmanMetadata struct {
	huffmanSubsampleBits int
	huffmanXsize         int
	huffmanImage         []int
	hasHuffmanImage      bool
	groups               []lldecHTreeGroup
}

func (m *lldecHuffmanMetadata) groupIndex(x, y int) int {
	if m.hasHuffmanImage {
		return m.huffmanImage[(y>>m.huffmanSubsampleBits)*m.huffmanXsize+(x>>m.huffmanSubsampleBits)]
	}
	return 0
}

type lldecDecoder struct {
	br lldecBitReader
}

func lldecNewDecoder(data []byte) lldecDecoder {
	return lldecDecoder{br: lldecNewBitReader(data)}
}

func (d *lldecDecoder) decodeImageStream(xsize, ysize int, topLevel bool) ([]uint32, error) {
	var transforms []lldecTransform
	transformXsize := xsize
	transformYsize := ysize

	if topLevel {
		transformsSeen := uint32(0)
		for {
			bit, err := d.br.readBit()
			if err != nil {
				return nil, err
			}
			if bit != 1 {
				break
			}
			transform, err := d.readTransform(transformXsize, transformYsize, &transformsSeen)
			if err != nil {
				return nil, err
			}
			if transform.kind == lldecColorIndexing {
				transformXsize = lldecSubsampleSize(transformXsize, transform.bits)
			}
			transforms = append(transforms, transform)
		}
	}

	colorCacheBits := 0
	useCacheBit, err := d.br.readBit()
	if err != nil {
		return nil, err
	}
	if useCacheBit == 1 {
		bits, err := d.br.readBits(4)
		if err != nil {
			return nil, err
		}
		if bits < 1 || bits > lldecMaxCacheBits {
			return nil, bitstreamErr("invalid VP8L color cache bits")
		}
		colorCacheBits = int(bits)
	}

	metadata, err := d.readHuffmanCodes(transformXsize, transformYsize, colorCacheBits, topLevel)
	if err != nil {
		return nil, err
	}
	data, err := d.decodeImageData(transformXsize, transformYsize, colorCacheBits, &metadata)
	if err != nil {
		return nil, err
	}

	if topLevel {
		for i := len(transforms) - 1; i >= 0; i-- {
			data, err = lldecApplyInverseTransform(&transforms[i], data)
			if err != nil {
				return nil, err
			}
		}
	}

	return data, nil
}

func (d *lldecDecoder) readTransform(xsize, ysize int, transformsSeen *uint32) (lldecTransform, error) {
	typeBits, err := d.br.readBits(2)
	if err != nil {
		return lldecTransform{}, err
	}
	var kind lldecTransformType
	switch typeBits {
	case 0:
		kind = lldecPredictor
	case 1:
		kind = lldecCrossColor
	case 2:
		kind = lldecSubtractGreen
	case 3:
		kind = lldecColorIndexing
	}

	if (*transformsSeen & (1 << typeBits)) != 0 {
		return lldecTransform{}, bitstreamErr("duplicate VP8L transform")
	}
	*transformsSeen |= 1 << typeBits

	switch kind {
	case lldecPredictor, lldecCrossColor:
		extra, err := d.br.readBits(lldecNumTransformBits)
		if err != nil {
			return lldecTransform{}, err
		}
		bits := lldecMinTransformBits + int(extra)
		data, err := d.decodeImageStream(
			lldecSubsampleSize(xsize, bits),
			lldecSubsampleSize(ysize, bits),
			false,
		)
		if err != nil {
			return lldecTransform{}, err
		}
		return lldecTransform{kind: kind, bits: bits, xsize: xsize, ysize: ysize, data: data}, nil
	case lldecSubtractGreen:
		return lldecTransform{kind: kind, bits: 0, xsize: xsize, ysize: ysize, data: nil}, nil
	default: // lldecColorIndexing
		nc, err := d.br.readBits(8)
		if err != nil {
			return lldecTransform{}, err
		}
		numColors := int(nc) + 1
		var bits int
		switch {
		case numColors > 16:
			bits = 0
		case numColors > 4:
			bits = 1
		case numColors > 2:
			bits = 2
		default:
			bits = 3
		}
		palette, err := d.decodeImageStream(numColors, 1, false)
		if err != nil {
			return lldecTransform{}, err
		}
		expanded := lldecExpandColorMap(palette, numColors, bits)
		return lldecTransform{kind: kind, bits: bits, xsize: xsize, ysize: ysize, data: expanded}, nil
	}
}

func (d *lldecDecoder) readHuffmanCodes(xsize, ysize, colorCacheBits int, allowMeta bool) (lldecHuffmanMetadata, error) {
	huffmanSubsampleBits := 0
	huffmanXsize := 0
	var huffmanImage []int
	hasHuffmanImage := false

	var mapping []int

	metaBit := uint32(0)
	if allowMeta {
		b, err := d.br.readBit()
		if err != nil {
			return lldecHuffmanMetadata{}, err
		}
		metaBit = b
	}

	if allowMeta && metaBit == 1 {
		extra, err := d.br.readBits(lldecNumHuffmanBits)
		if err != nil {
			return lldecHuffmanMetadata{}, err
		}
		huffmanSubsampleBits = lldecMinHuffmanBits + int(extra)
		huffmanXsize = lldecSubsampleSize(xsize, huffmanSubsampleBits)
		huffmanYsize := lldecSubsampleSize(ysize, huffmanSubsampleBits)
		image, err := d.decodeImageStream(huffmanXsize, huffmanYsize, false)
		if err != nil {
			return lldecHuffmanMetadata{}, err
		}

		maxGroup := 0
		rawGroups := make([]int, len(image))
		for i, pixel := range image {
			group := int((pixel >> 8) & 0xffff)
			rawGroups[i] = group
			if group > maxGroup {
				maxGroup = group
			}
		}
		mapping = make([]int, maxGroup+1)
		for i := range mapping {
			mapping[i] = -1
		}
		denseImage := make([]int, 0, len(rawGroups))
		nextGroup := 0
		for _, group := range rawGroups {
			dense := mapping[group]
			if dense < 0 {
				dense = nextGroup
				mapping[group] = dense
				nextGroup++
			}
			denseImage = append(denseImage, dense)
		}
		huffmanImage = denseImage
		hasHuffmanImage = true
	} else {
		mapping = []int{0}
	}

	numGroups := 0
	for _, v := range mapping {
		if v >= 0 {
			numGroups++
		}
	}
	groups := make([]lldecHTreeGroup, numGroups)
	filled := make([]bool, numGroups)
	for _, dense := range mapping {
		group, err := d.readHTreeGroup(colorCacheBits)
		if err != nil {
			return lldecHuffmanMetadata{}, err
		}
		if dense >= 0 {
			groups[dense] = group
			filled[dense] = true
		}
	}
	for _, ok := range filled {
		if !ok {
			return lldecHuffmanMetadata{}, bitstreamErr("missing VP8L Huffman group")
		}
	}

	return lldecHuffmanMetadata{
		huffmanSubsampleBits: huffmanSubsampleBits,
		huffmanXsize:         huffmanXsize,
		huffmanImage:         huffmanImage,
		hasHuffmanImage:      hasHuffmanImage,
		groups:               groups,
	}, nil
}

func (d *lldecDecoder) readHTreeGroup(colorCacheBits int) (lldecHTreeGroup, error) {
	greenAlphabetSize := lldecNumLiteralCodes + lldecNumLengthCodes
	if colorCacheBits > 0 {
		greenAlphabetSize += 1 << colorCacheBits
	}

	green, err := d.readHuffmanCode(greenAlphabetSize)
	if err != nil {
		return lldecHTreeGroup{}, err
	}
	red, err := d.readHuffmanCode(lldecNumLiteralCodes)
	if err != nil {
		return lldecHTreeGroup{}, err
	}
	blue, err := d.readHuffmanCode(lldecNumLiteralCodes)
	if err != nil {
		return lldecHTreeGroup{}, err
	}
	alpha, err := d.readHuffmanCode(lldecNumLiteralCodes)
	if err != nil {
		return lldecHTreeGroup{}, err
	}
	dist, err := d.readHuffmanCode(lldecNumDistanceCodes)
	if err != nil {
		return lldecHTreeGroup{}, err
	}
	return lldecHTreeGroup{green: green, red: red, blue: blue, alpha: alpha, dist: dist}, nil
}

func (d *lldecDecoder) readHuffmanCode(alphabetSize int) (lldecHuffmanTree, error) {
	codeLengths := make([]uint8, alphabetSize)
	simpleBit, err := d.br.readBit()
	if err != nil {
		return lldecHuffmanTree{}, err
	}

	if simpleBit == 1 {
		nsBit, err := d.br.readBit()
		if err != nil {
			return lldecHuffmanTree{}, err
		}
		numSymbols := int(nsBit) + 1
		firstSymbolLenCode, err := d.br.readBit()
		if err != nil {
			return lldecHuffmanTree{}, err
		}
		firstBits := 1
		if firstSymbolLenCode != 0 {
			firstBits = 8
		}
		fs, err := d.br.readBits(firstBits)
		if err != nil {
			return lldecHuffmanTree{}, err
		}
		firstSymbol := int(fs)
		if firstSymbol >= alphabetSize {
			return lldecHuffmanTree{}, bitstreamErr("invalid VP8L simple Huffman symbol")
		}
		codeLengths[firstSymbol] = 1
		if numSymbols == 2 {
			ss, err := d.br.readBits(8)
			if err != nil {
				return lldecHuffmanTree{}, err
			}
			secondSymbol := int(ss)
			if secondSymbol >= alphabetSize {
				return lldecHuffmanTree{}, bitstreamErr("invalid VP8L simple Huffman symbol")
			}
			codeLengths[secondSymbol] = 1
		}
	} else {
		var codeLengthCodeLengths [lldecNumCodeLengthCodes]uint8
		ncBits, err := d.br.readBits(4)
		if err != nil {
			return lldecHuffmanTree{}, err
		}
		numCodes := int(ncBits) + 4
		if numCodes > lldecNumCodeLengthCodes {
			return lldecHuffmanTree{}, bitstreamErr("too many VP8L code length codes")
		}
		for i := 0; i < numCodes; i++ {
			v, err := d.br.readBits(3)
			if err != nil {
				return lldecHuffmanTree{}, err
			}
			codeLengthCodeLengths[lldecCodeLengthCodeOrder[i]] = uint8(v)
		}
		codeLengthTree, err := lldecHuffmanFromCodeLengths(codeLengthCodeLengths[:])
		if err != nil {
			return lldecHuffmanTree{}, err
		}
		if err := d.readHuffmanCodeLengths(&codeLengthTree, codeLengths); err != nil {
			return lldecHuffmanTree{}, err
		}
	}

	return lldecHuffmanFromCodeLengths(codeLengths)
}

func (d *lldecDecoder) readHuffmanCodeLengths(codeLengthTree *lldecHuffmanTree, codeLengths []uint8) error {
	numSymbols := len(codeLengths)

	useMaxBit, err := d.br.readBit()
	if err != nil {
		return err
	}
	maxSymbol := numSymbols
	if useMaxBit == 1 {
		nb, err := d.br.readBits(3)
		if err != nil {
			return err
		}
		lengthNbits := 2 + 2*int(nb)
		v, err := d.br.readBits(lengthNbits)
		if err != nil {
			return err
		}
		value := 2 + int(v)
		if value > numSymbols {
			return bitstreamErr("invalid VP8L Huffman code length span")
		}
		maxSymbol = value
	}

	symbol := 0
	prevCodeLen := uint8(lldecDefaultCodeLength)
	for symbol < numSymbols {
		if maxSymbol == 0 {
			break
		}
		maxSymbol--

		cl, err := codeLengthTree.readSymbol(&d.br)
		if err != nil {
			return err
		}
		codeLen := int(cl)
		if codeLen < lldecCodeLengthRepeatCode {
			codeLengths[symbol] = uint8(codeLen)
			if codeLen != 0 {
				prevCodeLen = uint8(codeLen)
			}
			symbol++
			continue
		}

		slot := codeLen - lldecCodeLengthRepeatCode
		if slot < 0 || slot >= len(lldecCodeLengthExtraBits) {
			return bitstreamErr("invalid VP8L repeat code")
		}
		rb, err := d.br.readBits(lldecCodeLengthExtraBits[slot])
		if err != nil {
			return err
		}
		repeat := int(rb) + lldecCodeLengthRepeatOffsets[slot]
		if symbol+repeat > numSymbols {
			return bitstreamErr("VP8L repeat overruns code lengths")
		}
		var value uint8
		if codeLen == lldecCodeLengthRepeatCode {
			value = prevCodeLen
		} else {
			value = 0
		}
		for i := symbol; i < symbol+repeat; i++ {
			codeLengths[i] = value
		}
		symbol += repeat
	}

	return nil
}

func (d *lldecDecoder) decodeImageData(width, height, colorCacheBits int, metadata *lldecHuffmanMetadata) ([]uint32, error) {
	data := make([]uint32, width*height)
	var colorCache *lldecColorCache
	if colorCacheBits > 0 {
		cc, err := lldecNewColorCache(colorCacheBits)
		if err != nil {
			return nil, err
		}
		colorCache = &cc
	}
	pos := 0
	lenCodeLimit := lldecNumLiteralCodes + lldecNumLengthCodes
	colorCacheLimit := lenCodeLimit
	if colorCacheBits > 0 {
		colorCacheLimit += 1 << colorCacheBits
	}

	for pos < len(data) {
		x := pos % width
		y := pos / width
		group := &metadata.groups[metadata.groupIndex(x, y)]
		codeSym, err := group.green.readSymbol(&d.br)
		if err != nil {
			return nil, err
		}
		code := int(codeSym)

		switch {
		case code < lldecNumLiteralCodes:
			redSym, err := group.red.readSymbol(&d.br)
			if err != nil {
				return nil, err
			}
			blueSym, err := group.blue.readSymbol(&d.br)
			if err != nil {
				return nil, err
			}
			alphaSym, err := group.alpha.readSymbol(&d.br)
			if err != nil {
				return nil, err
			}
			pixel := (uint32(alphaSym) << 24) | (uint32(redSym) << 16) | (uint32(code) << 8) | uint32(blueSym)
			data[pos] = pixel
			if colorCache != nil {
				colorCache.insert(pixel)
			}
			pos++
		case code < lenCodeLimit:
			length, err := lldecGetCopyValue(code-lldecNumLiteralCodes, &d.br)
			if err != nil {
				return nil, err
			}
			distSym, err := group.dist.readSymbol(&d.br)
			if err != nil {
				return nil, err
			}
			distCode, err := lldecGetCopyValue(int(distSym), &d.br)
			if err != nil {
				return nil, err
			}
			dist := lldecPlaneCodeToDistance(width, distCode)
			if dist > pos || pos+length > len(data) {
				return nil, bitstreamErr("invalid VP8L backward reference")
			}
			for i := 0; i < length; i++ {
				pixel := data[pos+i-dist]
				data[pos+i] = pixel
				if colorCache != nil {
					colorCache.insert(pixel)
				}
			}
			pos += length
		case code < colorCacheLimit:
			key := code - lenCodeLimit
			if colorCache == nil {
				return nil, bitstreamErr("unexpected VP8L color cache code")
			}
			pixel, err := colorCache.lookup(key)
			if err != nil {
				return nil, err
			}
			data[pos] = pixel
			colorCache.insert(pixel)
			pos++
		default:
			return nil, bitstreamErr("invalid VP8L green Huffman symbol")
		}
	}

	return data, nil
}

func lldecReverseBits(code uint32, bits int) uint16 {
	out := uint32(0)
	for i := 0; i < bits; i++ {
		out = (out << 1) | (code & 1)
		code >>= 1
	}
	return uint16(out)
}

func lldecSubsampleSize(size, bits int) int {
	return (size + (1 << bits) - 1) >> bits
}

func lldecGetCopyValue(symbol int, br *lldecBitReader) (int, error) {
	if symbol < 4 {
		return symbol + 1, nil
	}
	extraBits := (symbol - 2) >> 1
	offset := (2 + (symbol & 1)) << extraBits
	v, err := br.readBits(extraBits)
	if err != nil {
		return 0, err
	}
	return offset + int(v) + 1, nil
}

func lldecPlaneCodeToDistance(width, planeCode int) int {
	if planeCode > len(lldecCodeToPlane) {
		return planeCode - len(lldecCodeToPlane)
	}
	distCode := lldecCodeToPlane[planeCode-1]
	yOffset := int(distCode >> 4)
	xOffset := 8 - int(distCode&0x0f)
	dist := yOffset*width + xOffset
	if dist < 1 {
		return 1
	}
	return dist
}

func lldecAddPixels(a, b uint32) uint32 {
	alpha := uint32(uint8(a>>24) + uint8(b>>24))
	red := uint32(uint8(a>>16) + uint8(b>>16))
	green := uint32(uint8(a>>8) + uint8(b>>8))
	blue := uint32(uint8(a) + uint8(b))
	return (alpha << 24) | (red << 16) | (green << 8) | blue
}

func lldecAverage2(a, b uint32) uint32 {
	return (((a ^ b) & 0xfefefefe) >> 1) + (a & b)
}

func lldecClip255(value int32) uint32 {
	if value < 0 {
		return 0
	}
	if value > 255 {
		return 255
	}
	return uint32(value)
}

func lldecClampedAddSubtractFull(left, top, topLeft uint32) uint32 {
	alpha := lldecClip255(int32(left>>24) + int32(top>>24) - int32(topLeft>>24))
	red := lldecClip255(int32((left>>16)&0xff) + int32((top>>16)&0xff) - int32((topLeft>>16)&0xff))
	green := lldecClip255(int32((left>>8)&0xff) + int32((top>>8)&0xff) - int32((topLeft>>8)&0xff))
	blue := lldecClip255(int32(left&0xff) + int32(top&0xff) - int32(topLeft&0xff))
	return (alpha << 24) | (red << 16) | (green << 8) | blue
}

func lldecClampedAddSubtractHalf(left, top, topLeft uint32) uint32 {
	avg := lldecAverage2(left, top)
	alpha := lldecClip255(int32(avg>>24) + (int32(avg>>24)-int32(topLeft>>24))/2)
	red := lldecClip255(int32((avg>>16)&0xff) + (int32((avg>>16)&0xff)-int32((topLeft>>16)&0xff))/2)
	green := lldecClip255(int32((avg>>8)&0xff) + (int32((avg>>8)&0xff)-int32((topLeft>>8)&0xff))/2)
	blue := lldecClip255(int32(avg&0xff) + (int32(avg&0xff)-int32(topLeft&0xff))/2)
	return (alpha << 24) | (red << 16) | (green << 8) | blue
}

func lldecAbs(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

func lldecSelectPredictor(left, top, topLeft uint32) uint32 {
	predAlpha := int32(left>>24) + int32(top>>24) - int32(topLeft>>24)
	predRed := int32((left>>16)&0xff) + int32((top>>16)&0xff) - int32((topLeft>>16)&0xff)
	predGreen := int32((left>>8)&0xff) + int32((top>>8)&0xff) - int32((topLeft>>8)&0xff)
	predBlue := int32(left&0xff) + int32(top&0xff) - int32(topLeft&0xff)

	leftDistance := lldecAbs(predAlpha-int32(left>>24)) +
		lldecAbs(predRed-int32((left>>16)&0xff)) +
		lldecAbs(predGreen-int32((left>>8)&0xff)) +
		lldecAbs(predBlue-int32(left&0xff))
	topDistance := lldecAbs(predAlpha-int32(top>>24)) +
		lldecAbs(predRed-int32((top>>16)&0xff)) +
		lldecAbs(predGreen-int32((top>>8)&0xff)) +
		lldecAbs(predBlue-int32(top&0xff))

	if leftDistance < topDistance {
		return left
	}
	return top
}

func lldecPredict(mode uint8, left, top, topLeft, topRight uint32) uint32 {
	switch mode {
	case 0, 14, 15:
		return lldecARGBBlack
	case 1:
		return left
	case 2:
		return top
	case 3:
		return topRight
	case 4:
		return topLeft
	case 5:
		return lldecAverage2(lldecAverage2(left, topRight), top)
	case 6:
		return lldecAverage2(left, topLeft)
	case 7:
		return lldecAverage2(left, top)
	case 8:
		return lldecAverage2(topLeft, top)
	case 9:
		return lldecAverage2(top, topRight)
	case 10:
		return lldecAverage2(lldecAverage2(left, topLeft), lldecAverage2(top, topRight))
	case 11:
		return lldecSelectPredictor(left, top, topLeft)
	case 12:
		return lldecClampedAddSubtractFull(left, top, topLeft)
	case 13:
		return lldecClampedAddSubtractHalf(left, top, topLeft)
	default:
		return lldecARGBBlack
	}
}

func lldecColorTransformDelta(transform, color uint8) int32 {
	return (int32(int8(transform)) * int32(int8(color))) >> 5
}

func lldecExpandColorMap(palette []uint32, numColors, bits int) []uint32 {
	finalNumColors := 1 << (8 >> bits)
	expanded := make([]uint32, finalNumColors)
	if numColors == 0 {
		return expanded
	}

	expanded[0] = palette[0]
	for i := 1; i < numColors; i++ {
		expanded[i] = lldecAddPixels(palette[i], expanded[i-1])
	}
	return expanded
}

func lldecApplyInverseTransform(transform *lldecTransform, input []uint32) ([]uint32, error) {
	switch transform.kind {
	case lldecSubtractGreen:
		output := make([]uint32, len(input))
		for i, argb := range input {
			green := (argb >> 8) & 0xff
			red := (((argb >> 16) & 0xff) + green) & 0xff
			blue := ((argb & 0xff) + green) & 0xff
			output[i] = (argb & 0xff00ff00) | (red << 16) | blue
		}
		return output, nil
	case lldecCrossColor:
		expectedLen := transform.xsize * transform.ysize
		if len(input) != expectedLen {
			return nil, bitstreamErr("VP8L cross-color size mismatch")
		}
		tilesPerRow := lldecSubsampleSize(transform.xsize, transform.bits)
		output := make([]uint32, len(input))
		for y := 0; y < transform.ysize; y++ {
			for x := 0; x < transform.xsize; x++ {
				argb := input[y*transform.xsize+x]
				code := transform.data[(y>>transform.bits)*tilesPerRow+(x>>transform.bits)]
				greenToRed := uint8(code)
				greenToBlue := uint8((code >> 8) & 0xff)
				redToBlue := uint8((code >> 16) & 0xff)
				green := uint8((argb >> 8) & 0xff)
				red := int32((argb >> 16) & 0xff)
				blue := int32(argb & 0xff)
				red = (red + lldecColorTransformDelta(greenToRed, green)) & 0xff
				blue = (blue + lldecColorTransformDelta(greenToBlue, green)) & 0xff
				blue = (blue + lldecColorTransformDelta(redToBlue, uint8(red))) & 0xff
				output[y*transform.xsize+x] = (argb & 0xff00ff00) | (uint32(red) << 16) | uint32(blue)
			}
		}
		return output, nil
	case lldecPredictor:
		expectedLen := transform.xsize * transform.ysize
		if len(input) != expectedLen {
			return nil, bitstreamErr("VP8L predictor size mismatch")
		}
		tilesPerRow := lldecSubsampleSize(transform.xsize, transform.bits)
		output := make([]uint32, len(input))
		for y := 0; y < transform.ysize; y++ {
			for x := 0; x < transform.xsize; x++ {
				residual := input[y*transform.xsize+x]
				var pred uint32
				if y == 0 {
					if x == 0 {
						pred = lldecARGBBlack
					} else {
						pred = output[y*transform.xsize+x-1]
					}
				} else if x == 0 {
					pred = output[(y-1)*transform.xsize]
				} else {
					left := output[y*transform.xsize+x-1]
					top := output[(y-1)*transform.xsize+x]
					topLeft := output[(y-1)*transform.xsize+x-1]
					var topRight uint32
					if x+1 < transform.xsize {
						topRight = output[(y-1)*transform.xsize+x+1]
					} else {
						topRight = output[y*transform.xsize]
					}
					mode := uint8((transform.data[(y>>transform.bits)*tilesPerRow+(x>>transform.bits)] >> 8) & 0x0f)
					pred = lldecPredict(mode, left, top, topLeft, topRight)
				}
				output[y*transform.xsize+x] = lldecAddPixels(residual, pred)
			}
		}
		return output, nil
	default: // lldecColorIndexing
		reducedWidth := lldecSubsampleSize(transform.xsize, transform.bits)
		expectedLen := reducedWidth * transform.ysize
		if len(input) != expectedLen {
			return nil, bitstreamErr("VP8L color indexing size mismatch")
		}

		bitsPerPixel := 8 >> transform.bits
		pixelsPerByte := 1 << transform.bits
		bitMask := uint32(1<<bitsPerPixel) - 1
		output := make([]uint32, transform.xsize*transform.ysize)

		if transform.bits == 0 {
			for i, src := range input {
				index := int((src >> 8) & 0xff)
				if index < len(transform.data) {
					output[i] = transform.data[index]
				} else {
					output[i] = 0
				}
			}
			return output, nil
		}

		for y := 0; y < transform.ysize; y++ {
			srcRow := input[y*reducedWidth : (y+1)*reducedWidth]
			dstRow := output[y*transform.xsize : (y+1)*transform.xsize]
			x := 0
			for _, packed := range srcRow {
				indices := (packed >> 8) & 0xff
				for j := 0; j < pixelsPerByte; j++ {
					if x >= transform.xsize {
						break
					}
					index := int(indices & bitMask)
					if index < len(transform.data) {
						dstRow[x] = transform.data[index]
					} else {
						dstRow[x] = 0
					}
					indices >>= bitsPerPixel
					x++
				}
			}
		}

		return output, nil
	}
}

func lldecArgbToRgba(argb []uint32) []byte {
	rgba := make([]byte, len(argb)*4)
	for index, pixel := range argb {
		base := index * 4
		rgba[base] = byte((pixel >> 16) & 0xff)
		rgba[base+1] = byte((pixel >> 8) & 0xff)
		rgba[base+2] = byte(pixel & 0xff)
		rgba[base+3] = byte(pixel >> 24)
	}
	return rgba
}

func lldecDecodeVp8lToArgb(data []byte) (int, int, []uint32, error) {
	info, err := getLosslessInfo(data)
	if err != nil {
		return 0, 0, nil, err
	}
	if len(data) < 5 {
		return 0, 0, nil, notEnoughData("VP8L frame payload")
	}
	bitstream := data[5:]
	decoder := lldecNewDecoder(bitstream)
	argb, err := decoder.decodeImageStream(info.Width, info.Height, true)
	if err != nil {
		return 0, 0, nil, err
	}
	if len(argb) != info.Width*info.Height {
		return 0, 0, nil, bitstreamErr("decoded VP8L image has wrong size")
	}
	return info.Width, info.Height, argb, nil
}

func decodeLosslessStreamToArgb(data []byte, width, height int) ([]uint32, error) {
	decoder := lldecNewDecoder(data)
	argb, err := decoder.decodeImageStream(width, height, true)
	if err != nil {
		return nil, err
	}
	if len(argb) != width*height {
		return nil, bitstreamErr("decoded VP8L image has wrong size")
	}
	return argb, nil
}

// DecodeLosslessVp8lToRGBA decodes a raw VP8L frame payload to RGBA.
func decodeLosslessVp8lToRGBA(data []byte) (decodedImage, error) {
	width, height, argb, err := lldecDecodeVp8lToArgb(data)
	if err != nil {
		return decodedImage{}, err
	}
	return decodedImage{
		Width:  width,
		Height: height,
		RGBA:   lldecArgbToRgba(argb),
	}, nil
}

// DecodeLosslessWebpToRGBA decodes a still lossless WebP container to RGBA.
func decodeLosslessWebpToRGBA(data []byte) (decodedImage, error) {
	parsed, err := parseStillWebp(data)
	if err != nil {
		return decodedImage{}, err
	}
	if parsed.Features.Format != FormatLossless {
		return decodedImage{}, unsupportedErr("expected a still lossless WebP image")
	}
	return decodeLosslessVp8lToRGBA(parsed.ImageData)
}
