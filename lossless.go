package webp

import "encoding/binary"

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

// lldecBitSlack is the zero padding lldecNewBitReader appends to the stream. It
// lets peek load eight bytes unconditionally, including past the last bit of
// real data: bitPos is only checked against totalBits once per decoded pixel,
// and between two checks the decoder reads at most four Huffman symbols of
// fifteen bits each, so it can be at most sixty bits beyond the end.
const lldecBitSlack = 32

type lldecBitReader struct {
	data      []byte
	bitPos    int
	totalBits int
}

func lldecNewBitReader(data []byte) lldecBitReader {
	padded := make([]byte, len(data)+lldecBitSlack)
	copy(padded, data)
	return lldecBitReader{data: padded, bitPos: 0, totalBits: len(data) * 8}
}

// peek returns the next 32 bits of the stream without consuming them, and
// zero-fills past the end. Reading past the end is not an error here; it is
// caught when the bits are consumed, which is what lets the Huffman lookup run
// without a bounds check per symbol.
func (br *lldecBitReader) peek() uint32 {
	return uint32(binary.LittleEndian.Uint64(br.data[br.bitPos>>3:]) >> uint(br.bitPos&7))
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
	if end > br.totalBits {
		return 0, notEnoughData("VP8L bitstream")
	}
	value := br.peek() & (1<<uint(numBits) - 1)
	br.bitPos = end
	return value, nil
}

const (
	lldecHuffRootBits = 8
	lldecHuffRootSize = 1 << lldecHuffRootBits
	lldecHuffRootMask = lldecHuffRootSize - 1
)

// lldecHuffCode is one entry of a canonical-code lookup table. In a root-table
// entry with bits > lldecHuffRootBits, value is the offset of a second-level
// table relative to this entry and bits-lldecHuffRootBits is how many further
// bits index it.
type lldecHuffCode struct {
	value uint16
	bits  uint8
}

type lldecHuffmanTree struct {
	table        []lldecHuffCode
	singleSymbol int32
}

// lldecNextCodeKey advances a bit-reversed canonical code of the given length.
func lldecNextCodeKey(key uint32, length int) uint32 {
	step := uint32(1) << (length - 1)
	for key&step != 0 {
		step >>= 1
	}
	if step == 0 {
		return key
	}
	return (key & (step - 1)) + step
}

// lldecReplicate writes code into every slot of table congruent to its index
// modulo step, which is how one code fills all the table entries whose extra
// low-order bits it does not distinguish.
func lldecReplicate(table []lldecHuffCode, step, end int, code lldecHuffCode) {
	for end > 0 {
		end -= step
		table[end] = code
	}
}

// lldecSubTableBits sizes a second-level table so it exactly covers the codes
// that extend past the root table under one root prefix.
func lldecSubTableBits(count *[lldecMaxAllowedCodeLength + 1]int, length int) int {
	left := 1 << (length - lldecHuffRootBits)
	for length < lldecMaxAllowedCodeLength {
		left -= count[length]
		if left <= 0 {
			break
		}
		length++
		left <<= 1
	}
	return length - lldecHuffRootBits
}

func lldecHuffmanFromCodeLengths(codeLengths []uint8) (lldecHuffmanTree, error) {
	var count [lldecMaxAllowedCodeLength + 1]int
	for _, l := range codeLengths {
		if int(l) > lldecMaxAllowedCodeLength {
			return lldecHuffmanTree{}, bitstreamErr("invalid VP8L Huffman code length")
		}
		count[l]++
	}
	count[0] = 0

	var offset [lldecMaxAllowedCodeLength + 1]int
	for l := 1; l < lldecMaxAllowedCodeLength; l++ {
		offset[l+1] = offset[l] + count[l]
	}
	numSymbols := offset[lldecMaxAllowedCodeLength] + count[lldecMaxAllowedCodeLength]

	if numSymbols == 0 {
		return lldecHuffmanTree{}, bitstreamErr("empty VP8L Huffman tree")
	}

	sorted := make([]uint16, numSymbols)
	for symbol, l := range codeLengths {
		if l > 0 {
			sorted[offset[l]] = uint16(symbol)
			offset[l]++
		}
	}
	if numSymbols == 1 {
		return lldecHuffmanTree{singleSymbol: int32(sorted[0])}, nil
	}

	table := make([]lldecHuffCode, lldecHuffRootSize, lldecHuffRootSize+64)
	var (
		key       uint32
		symbol    int
		numNodes  = 1
		numOpen   = 1
		tableBase = 0
		tableSize = lldecHuffRootSize
		low       = uint32(0xffffffff)
	)

	for length, step := 1, 2; length <= lldecHuffRootBits; length, step = length+1, step<<1 {
		numOpen <<= 1
		numNodes += numOpen
		numOpen -= count[length]
		if numOpen < 0 {
			return lldecHuffmanTree{}, bitstreamErr("oversubscribed VP8L Huffman tree")
		}
		for ; count[length] > 0; count[length]-- {
			code := lldecHuffCode{value: sorted[symbol], bits: uint8(length)}
			symbol++
			lldecReplicate(table[key:], step, tableSize, code)
			key = lldecNextCodeKey(key, length)
		}
	}

	for length, step := lldecHuffRootBits+1, 2; length <= lldecMaxAllowedCodeLength; length, step = length+1, step<<1 {
		numOpen <<= 1
		numNodes += numOpen
		numOpen -= count[length]
		if numOpen < 0 {
			return lldecHuffmanTree{}, bitstreamErr("oversubscribed VP8L Huffman tree")
		}
		for ; count[length] > 0; count[length]-- {
			if key&lldecHuffRootMask != low {
				tableBase += tableSize
				tableBits := lldecSubTableBits(&count, length)
				tableSize = 1 << tableBits
				low = key & lldecHuffRootMask
				table = append(table, make([]lldecHuffCode, tableSize)...)
				table[low] = lldecHuffCode{
					value: uint16(tableBase - int(low)),
					bits:  uint8(tableBits + lldecHuffRootBits),
				}
			}
			code := lldecHuffCode{value: sorted[symbol], bits: uint8(length - lldecHuffRootBits)}
			symbol++
			sub := key >> lldecHuffRootBits
			lldecReplicate(table[tableBase+int(sub):], step, tableSize, code)
			key = lldecNextCodeKey(key, length)
		}
	}

	if numNodes != 2*numSymbols-1 {
		return lldecHuffmanTree{}, bitstreamErr("incomplete VP8L Huffman tree")
	}

	return lldecHuffmanTree{table: table, singleSymbol: -1}, nil
}

// readSymbol does not report running off the end of the stream: past the end
// peek yields zeros, so the symbol is garbage but the read is in bounds, and
// the caller catches it by checking bitPos against totalBits once per pixel
// rather than once per symbol.
func (t *lldecHuffmanTree) readSymbol(br *lldecBitReader) uint16 {
	if t.singleSymbol >= 0 {
		return uint16(t.singleSymbol)
	}
	index := int(br.peek() & lldecHuffRootMask)
	entry := t.table[index]
	if entry.bits > lldecHuffRootBits {
		br.bitPos += lldecHuffRootBits
		nbits := uint(entry.bits) - lldecHuffRootBits
		index += int(entry.value) + int(br.peek()&(1<<nbits-1))
		entry = t.table[index]
	}
	br.bitPos += int(entry.bits)
	return entry.value
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

		if d.br.bitPos > d.br.totalBits {
			return notEnoughData("VP8L bitstream")
		}
		codeLen := int(codeLengthTree.readSymbol(&d.br))
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

	br := &d.br
	// The group only changes when the pixel crosses into another Huffman tile,
	// so tracking x and y and the current tile keeps the per-pixel cost to two
	// shifts, against a division and a lookup for every pixel.
	groups := metadata.groups
	hasHuffmanImage := metadata.hasHuffmanImage
	huffmanImage := metadata.huffmanImage
	huffmanXsize := metadata.huffmanXsize
	subBits := metadata.huffmanSubsampleBits
	group := &groups[0]
	tileX, tileY := -1, -1
	x, y := 0, 0

	// The colour cache is held as its own two fields so an insert is a multiply,
	// a shift and a store, with nothing to load through a pointer first.
	var cacheColors []uint32
	var cacheShift uint32
	if colorCache != nil {
		cacheColors, cacheShift = colorCache.colors, colorCache.hashShift
	}

	for pos < len(data) {
		if br.bitPos > br.totalBits {
			return nil, notEnoughData("VP8L bitstream")
		}
		if hasHuffmanImage {
			if tx, ty := x>>subBits, y>>subBits; tx != tileX || ty != tileY {
				group = &groups[huffmanImage[ty*huffmanXsize+tx]]
				tileX, tileY = tx, ty
			}
		}
		code := int(group.green.readSymbol(br))

		switch {
		case code < lldecNumLiteralCodes:
			redSym := group.red.readSymbol(br)
			blueSym := group.blue.readSymbol(br)
			alphaSym := group.alpha.readSymbol(br)
			pixel := (uint32(alphaSym) << 24) | (uint32(redSym) << 16) | (uint32(code) << 8) | uint32(blueSym)
			data[pos] = pixel
			if cacheColors != nil {
				cacheColors[(pixel*lldecColorCacheHashMul)>>cacheShift] = pixel
			}
			pos++
			if x++; x == width {
				x = 0
				y++
			}
		case code < lenCodeLimit:
			length, err := lldecGetCopyValue(code-lldecNumLiteralCodes, br)
			if err != nil {
				return nil, err
			}
			distCode, err := lldecGetCopyValue(int(group.dist.readSymbol(br)), br)
			if err != nil {
				return nil, err
			}
			dist := lldecPlaneCodeToDistance(width, distCode)
			if dist > pos || pos+length > len(data) {
				return nil, bitstreamErr("invalid VP8L backward reference")
			}
			src := data[pos-dist : pos-dist+length]
			dst := data[pos : pos+length]
			if dist >= length {
				copy(dst, src)
			} else {
				for i := range dst {
					dst[i] = src[i]
				}
			}
			if cacheColors != nil {
				for _, pixel := range dst {
					cacheColors[(pixel*lldecColorCacheHashMul)>>cacheShift] = pixel
				}
			}
			pos += length
			x, y = pos%width, pos/width
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
			cacheColors[(pixel*lldecColorCacheHashMul)>>cacheShift] = pixel
			pos++
			if x++; x == width {
				x = 0
				y++
			}
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

// lldecAddPixels adds two pixels channel-wise, each channel wrapping at 256. It
// adds the two even channels and the two odd channels as pairs, so the whole
// pixel takes two adds instead of four.
func lldecAddPixels(a, b uint32) uint32 {
	lo := (a & 0x00ff00ff) + (b & 0x00ff00ff)
	hi := ((a >> 8) & 0x00ff00ff) + ((b >> 8) & 0x00ff00ff)
	return (lo & 0x00ff00ff) | ((hi & 0x00ff00ff) << 8)
}

func lldecAverage2(a, b uint32) uint32 {
	return (((a ^ b) & 0xfefefefe) >> 1) + (a & b)
}

func lldecClip255(value int32) uint32 {
	if uint32(value) <= 255 {
		return uint32(value)
	}
	if value < 0 {
		return 0
	}
	return 255
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
	mask := v >> 31
	return (v ^ mask) - mask
}

// lldecSelectPredictor picks whichever of left and top is closer to
// left+top-topLeft. The distance to left is |top-topLeft| channel-wise and the
// distance to top is |left-topLeft|, so neither the prediction nor the two
// distances have to be formed to compare them.
func lldecSelectPredictor(left, top, topLeft uint32) uint32 {
	diff := lldecAbs(int32(left>>24)-int32(topLeft>>24)) -
		lldecAbs(int32(top>>24)-int32(topLeft>>24)) +
		lldecAbs(int32((left>>16)&0xff)-int32((topLeft>>16)&0xff)) -
		lldecAbs(int32((top>>16)&0xff)-int32((topLeft>>16)&0xff)) +
		lldecAbs(int32((left>>8)&0xff)-int32((topLeft>>8)&0xff)) -
		lldecAbs(int32((top>>8)&0xff)-int32((topLeft>>8)&0xff)) +
		lldecAbs(int32(left&0xff)-int32(topLeft&0xff)) -
		lldecAbs(int32(top&0xff)-int32(topLeft&0xff))
	if diff <= 0 {
		return top
	}
	return left
}

// lldecTopRight is the above-right neighbour, which wraps to the start of the
// current row on the last column.
func lldecTopRight(row, above []uint32, x, width int) uint32 {
	if x+1 < width {
		return above[x+1]
	}
	return row[0]
}

// lldecPredictSpan reconstructs row[from:to] under one predictor mode. The mode
// is fixed across a transform tile, so choosing it once per span leaves each
// mode a loop with no per-pixel dispatch.
func lldecPredictSpan(mode uint8, row, above []uint32, from, to, width int) {
	switch mode {
	case 1:
		left := row[from-1]
		for x := from; x < to; x++ {
			left = lldecAddPixels(row[x], left)
			row[x] = left
		}
	case 2:
		for x := from; x < to; x++ {
			row[x] = lldecAddPixels(row[x], above[x])
		}
	case 3:
		for x := from; x < to; x++ {
			row[x] = lldecAddPixels(row[x], lldecTopRight(row, above, x, width))
		}
	case 4:
		topLeft := above[from-1]
		for x := from; x < to; x++ {
			row[x] = lldecAddPixels(row[x], topLeft)
			topLeft = above[x]
		}
	case 5:
		left := row[from-1]
		for x := from; x < to; x++ {
			pred := lldecAverage2(lldecAverage2(left, lldecTopRight(row, above, x, width)), above[x])
			left = lldecAddPixels(row[x], pred)
			row[x] = left
		}
	case 6:
		left, topLeft := row[from-1], above[from-1]
		for x := from; x < to; x++ {
			left = lldecAddPixels(row[x], lldecAverage2(left, topLeft))
			row[x], topLeft = left, above[x]
		}
	case 7:
		left := row[from-1]
		for x := from; x < to; x++ {
			left = lldecAddPixels(row[x], lldecAverage2(left, above[x]))
			row[x] = left
		}
	case 8:
		topLeft := above[from-1]
		for x := from; x < to; x++ {
			top := above[x]
			row[x] = lldecAddPixels(row[x], lldecAverage2(topLeft, top))
			topLeft = top
		}
	case 9:
		for x := from; x < to; x++ {
			row[x] = lldecAddPixels(row[x], lldecAverage2(above[x], lldecTopRight(row, above, x, width)))
		}
	case 10:
		left, topLeft := row[from-1], above[from-1]
		for x := from; x < to; x++ {
			top := above[x]
			pred := lldecAverage2(
				lldecAverage2(left, topLeft),
				lldecAverage2(top, lldecTopRight(row, above, x, width)))
			left = lldecAddPixels(row[x], pred)
			row[x], topLeft = left, top
		}
	// The three neighbour-comparing modes carry left and top-left across
	// iterations: each is the previous pixel's own value and top, so they cost
	// two loads a pixel that a register already holds.
	case 11:
		left, topLeft := row[from-1], above[from-1]
		for x := from; x < to; x++ {
			top := above[x]
			left = lldecAddPixels(row[x], lldecSelectPredictor(left, top, topLeft))
			row[x], topLeft = left, top
		}
	case 12:
		left, topLeft := row[from-1], above[from-1]
		for x := from; x < to; x++ {
			top := above[x]
			left = lldecAddPixels(row[x], lldecClampedAddSubtractFull(left, top, topLeft))
			row[x], topLeft = left, top
		}
	case 13:
		left, topLeft := row[from-1], above[from-1]
		for x := from; x < to; x++ {
			top := above[x]
			left = lldecAddPixels(row[x], lldecClampedAddSubtractHalf(left, top, topLeft))
			row[x], topLeft = left, top
		}
	default: // opaque black
		for x := from; x < to; x++ {
			row[x] = lldecAddPixels(row[x], lldecARGBBlack)
		}
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
		// The first three transforms read each pixel only after every pixel they
		// predict it from is already reconstructed, so they run over the decoded
		// buffer in place and the decode holds one image-sized buffer instead of
		// one per transform.
		for i, argb := range input {
			green := (argb >> 8) & 0xff
			red := (((argb >> 16) & 0xff) + green) & 0xff
			blue := ((argb & 0xff) + green) & 0xff
			input[i] = (argb & 0xff00ff00) | (red << 16) | blue
		}
		return input, nil
	case lldecCrossColor:
		expectedLen := transform.xsize * transform.ysize
		if len(input) != expectedLen {
			return nil, bitstreamErr("VP8L cross-color size mismatch")
		}
		tilesPerRow := lldecSubsampleSize(transform.xsize, transform.bits)
		tileMask := 1<<transform.bits - 1
		for y := 0; y < transform.ysize; y++ {
			row := input[y*transform.xsize : (y+1)*transform.xsize : (y+1)*transform.xsize]
			tileRow := transform.data[(y>>transform.bits)*tilesPerRow:]
			var greenToRed, greenToBlue, redToBlue uint8
			for x, argb := range row {
				if x&tileMask == 0 {
					code := tileRow[x>>transform.bits]
					greenToRed = uint8(code)
					greenToBlue = uint8(code >> 8)
					redToBlue = uint8(code >> 16)
				}
				green := uint8(argb >> 8)
				red := int32((argb >> 16) & 0xff)
				blue := int32(argb & 0xff)
				red = (red + lldecColorTransformDelta(greenToRed, green)) & 0xff
				blue = (blue + lldecColorTransformDelta(greenToBlue, green)) & 0xff
				blue = (blue + lldecColorTransformDelta(redToBlue, uint8(red))) & 0xff
				row[x] = (argb & 0xff00ff00) | (uint32(red) << 16) | uint32(blue)
			}
		}
		return input, nil
	case lldecPredictor:
		expectedLen := transform.xsize * transform.ysize
		if len(input) != expectedLen {
			return nil, bitstreamErr("VP8L predictor size mismatch")
		}
		width := transform.xsize
		tilesPerRow := lldecSubsampleSize(width, transform.bits)
		tileMask := 1<<transform.bits - 1

		input[0] = lldecAddPixels(input[0], lldecARGBBlack)
		for x := 1; x < width; x++ {
			input[x] = lldecAddPixels(input[x], input[x-1])
		}
		for y := 1; y < transform.ysize; y++ {
			row := input[y*width : (y+1)*width : (y+1)*width]
			above := input[(y-1)*width : y*width : y*width]
			tileRow := transform.data[(y>>transform.bits)*tilesPerRow:]
			row[0] = lldecAddPixels(row[0], above[0])
			for x := 1; x < width; {
				end := (x | tileMask) + 1
				if end > width {
					end = width
				}
				mode := uint8((tileRow[x>>transform.bits] >> 8) & 0x0f)
				lldecPredictSpan(mode, row, above, x, end, width)
				x = end
			}
		}
		return input, nil
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
	out := rgba
	for _, pixel := range argb {
		binary.BigEndian.PutUint32(out, pixel<<8|pixel>>24)
		out = out[4:]
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
