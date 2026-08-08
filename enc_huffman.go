package webp

import (
	"slices"
	"sync"
)

// Canonical Huffman code builder for the lossless VP8L encoder.
// Ported from src/encoder/huffman.rs.

const elosslessMaxAllowedCodeLength = 15

type elosslessHuffmanTreeToken struct {
	code      uint8
	extraBits uint8
}

type elosslessHuffmanCode struct {
	codeLengths     []uint8
	codes           []uint16
	singleSymbol    int
	hasSingleSymbol bool
}

func elosslessHuffmanCodeFromCodeLengths(codeLengths []uint8) (elosslessHuffmanCode, error) {
	var counts [elosslessMaxAllowedCodeLength + 1]uint32

	var symbols []int
	for symbol, length := range codeLengths {
		if length != 0 {
			symbols = append(symbols, symbol)
		}
	}

	if len(symbols) == 0 {
		return elosslessHuffmanCode{}, encBitstream("empty Huffman tree")
	}

	for _, length := range codeLengths {
		bits := int(length)
		if bits > elosslessMaxAllowedCodeLength {
			return elosslessHuffmanCode{}, encBitstream("invalid Huffman code length")
		}
		if bits > 0 {
			counts[bits]++
		}
	}

	hasSingle := len(symbols) == 1
	singleSymbol := 0
	if hasSingle {
		singleSymbol = symbols[0]
	}
	if len(symbols) > 1 {
		left := int32(1)
		for bits := 1; bits <= elosslessMaxAllowedCodeLength; bits++ {
			left = (left << 1) - int32(counts[bits])
			if left < 0 {
				return elosslessHuffmanCode{}, encBitstream("oversubscribed Huffman tree")
			}
		}
		if left != 0 {
			return elosslessHuffmanCode{}, encBitstream("incomplete Huffman tree")
		}
	}

	var nextCode [elosslessMaxAllowedCodeLength + 1]uint32
	code := uint32(0)
	for bits := 1; bits <= elosslessMaxAllowedCodeLength; bits++ {
		code = (code + counts[bits-1]) << 1
		nextCode[bits] = code
	}

	codes := make([]uint16, len(codeLengths))
	for symbol, length := range codeLengths {
		bits := int(length)
		if bits == 0 {
			continue
		}
		canonical := nextCode[bits]
		nextCode[bits]++
		codes[symbol] = elosslessReverseBits(canonical, bits)
	}

	return elosslessHuffmanCode{
		codeLengths:     codeLengths,
		codes:           codes,
		singleSymbol:    singleSymbol,
		hasSingleSymbol: hasSingle,
	}, nil
}

func elosslessHuffmanCodeFromHistogram(histogram []uint32, treeDepthLimit int) (elosslessHuffmanCode, error) {
	codeLengths, err := elosslessGenerateCodeLengths(histogram, treeDepthLimit)
	if err != nil {
		return elosslessHuffmanCode{}, err
	}
	return elosslessHuffmanCodeFromCodeLengths(codeLengths)
}

func (c *elosslessHuffmanCode) getCodeLengths() []uint8 {
	return c.codeLengths
}

// symbolDepth returns the number of bits writeSymbol emits for symbol: zero for
// a single-symbol code (which emits nothing), otherwise the code length.
func (c *elosslessHuffmanCode) symbolDepth(symbol int) int {
	if c.hasSingleSymbol {
		return 0
	}
	return int(c.codeLengths[symbol])
}

func (c *elosslessHuffmanCode) usedSymbols() []int {
	var symbols []int
	for symbol, length := range c.codeLengths {
		if length != 0 {
			symbols = append(symbols, symbol)
		}
	}
	return symbols
}

func (c *elosslessHuffmanCode) writeSymbol(bw *bitWriter, symbol int) error {
	if c.hasSingleSymbol {
		if symbol != c.singleSymbol {
			return encBitstream("attempted to write unexpected single-symbol Huffman code")
		}
		return nil
	}

	if symbol < 0 || symbol >= len(c.codeLengths) {
		return encInvalidParam("Huffman symbol is out of range")
	}
	depth := int(c.codeLengths[symbol])
	if depth == 0 {
		return encBitstream("attempted to write unused Huffman symbol")
	}
	return bw.putBits(uint32(c.codes[symbol]), depth)
}

func elosslessCompressHuffmanTree(codeLengths []uint8) []elosslessHuffmanTreeToken {
	tokens := make([]elosslessHuffmanTreeToken, 0, len(codeLengths))
	prevValue := uint8(8)
	index := 0

	for index < len(codeLengths) {
		value := codeLengths[index]
		next := index + 1
		for next < len(codeLengths) && codeLengths[next] == value {
			next++
		}
		runs := next - index
		if value == 0 {
			tokens = elosslessCodeRepeatedZeros(runs, tokens)
		} else {
			tokens = elosslessCodeRepeatedValues(runs, value, prevValue, tokens)
			prevValue = value
		}
		index = next
	}

	return tokens
}

func elosslessCodeRepeatedValues(repetitions int, value uint8, prevValue uint8, tokens []elosslessHuffmanTreeToken) []elosslessHuffmanTreeToken {
	if value != prevValue {
		tokens = append(tokens, elosslessHuffmanTreeToken{code: value, extraBits: 0})
		repetitions--
	}

	for repetitions >= 1 {
		if repetitions < 3 {
			for i := 0; i < repetitions; i++ {
				tokens = append(tokens, elosslessHuffmanTreeToken{code: value, extraBits: 0})
			}
			break
		} else if repetitions < 7 {
			tokens = append(tokens, elosslessHuffmanTreeToken{code: 16, extraBits: uint8(repetitions - 3)})
			break
		} else {
			tokens = append(tokens, elosslessHuffmanTreeToken{code: 16, extraBits: 3})
			repetitions -= 6
		}
	}
	return tokens
}

func elosslessCodeRepeatedZeros(repetitions int, tokens []elosslessHuffmanTreeToken) []elosslessHuffmanTreeToken {
	for repetitions >= 1 {
		if repetitions < 3 {
			for i := 0; i < repetitions; i++ {
				tokens = append(tokens, elosslessHuffmanTreeToken{code: 0, extraBits: 0})
			}
			break
		} else if repetitions < 11 {
			tokens = append(tokens, elosslessHuffmanTreeToken{code: 17, extraBits: uint8(repetitions - 3)})
			break
		} else if repetitions < 139 {
			tokens = append(tokens, elosslessHuffmanTreeToken{code: 18, extraBits: uint8(repetitions - 11)})
			break
		} else {
			tokens = append(tokens, elosslessHuffmanTreeToken{code: 18, extraBits: 0x7f})
			repetitions -= 138
		}
	}
	return tokens
}

type elosslessHuffmanLeaf struct {
	count uint32
	value int32
}

// elosslessHuffmanScratch holds the working buffers of
// elosslessGenerateCodeLengths. The function runs over a thousand times per
// encode, so the buffers are pooled instead of reallocated per call.
type elosslessHuffmanScratch struct {
	leaves   []elosslessHuffmanLeaf
	nodes    []uint32
	children [][2]int32
	depths   []int32
}

var elosslessHuffmanScratchPool = sync.Pool{
	New: func() any { return new(elosslessHuffmanScratch) },
}

// childLeaf encodes leaf index i as a negative child reference, keeping
// internal-node references non-negative.
func elosslessChildLeaf(i int) int32 { return int32(^i) }

func elosslessGenerateCodeLengths(histogram []uint32, treeDepthLimit int) ([]uint8, error) {
	codeLengths := make([]uint8, len(histogram))
	treeSizeOrig := 0
	for _, count := range histogram {
		if count != 0 {
			treeSizeOrig++
		}
	}
	if treeSizeOrig == 0 {
		return nil, encBitstream("empty Huffman histogram")
	}
	if treeSizeOrig > (1 << (treeDepthLimit - 1)) {
		return nil, encBitstream("Huffman tree exceeds depth limit")
	}

	scratch := elosslessHuffmanScratchPool.Get().(*elosslessHuffmanScratch)
	defer elosslessHuffmanScratchPool.Put(scratch)

	countMin := uint32(1)
	for {
		for i := range codeLengths {
			codeLengths[i] = 0
		}

		leaves := scratch.leaves[:0]
		for value, count := range histogram {
			if count != 0 {
				if count < countMin {
					count = countMin
				}
				leaves = append(leaves, elosslessHuffmanLeaf{count: count, value: int32(value)})
			}
		}
		scratch.leaves = leaves

		// Ascending by count, ties by descending symbol value: this is the order
		// in which the reference implementation's descending-sorted array is
		// consumed from its tail.
		slices.SortFunc(leaves, func(a, b elosslessHuffmanLeaf) int {
			if a.count != b.count {
				if a.count < b.count {
					return -1
				}
				return 1
			}
			return int(b.value - a.value)
		})

		maxDepth := 0
		if len(leaves) == 1 {
			codeLengths[leaves[0].value] = 1
			maxDepth = 1
		} else {
			maxDepth = elosslessBuildCodeLengths(scratch, codeLengths)
		}

		if maxDepth <= treeDepthLimit {
			return codeLengths, nil
		}

		if countMin > 0x7fff_ffff {
			return nil, encBitstream("Huffman count limit overflow")
		}
		countMin *= 2
	}
}

// elosslessBuildCodeLengths runs the two-queue canonical Huffman construction
// over scratch.leaves (already sorted ascending by count) and writes the leaf
// depths into codeLengths, returning the deepest one. Merging two ordered
// queues, one of leaves and one of the internal nodes created so far, keeps the
// construction linear: internal node counts are produced in non-decreasing
// order, so neither queue ever needs re-sorting.
func elosslessBuildCodeLengths(scratch *elosslessHuffmanScratch, codeLengths []uint8) int {
	leaves := scratch.leaves
	n := len(leaves)

	nodes := scratch.nodes[:0]
	children := scratch.children[:0]

	leafNext, nodeNext := 0, 0
	for i := 0; i < n-1; i++ {
		var pair [2]int32
		total := uint32(0)
		for k := 0; k < 2; k++ {
			if leafNext < n && (nodeNext >= len(nodes) || leaves[leafNext].count <= nodes[nodeNext]) {
				total += leaves[leafNext].count
				pair[k] = elosslessChildLeaf(leafNext)
				leafNext++
			} else {
				total += nodes[nodeNext]
				pair[k] = int32(nodeNext)
				nodeNext++
			}
		}
		nodes = append(nodes, total)
		children = append(children, pair)
	}
	scratch.nodes = nodes
	scratch.children = children

	depths := scratch.depths[:0]
	if cap(depths) < len(nodes) {
		depths = make([]int32, len(nodes))
	}
	depths = depths[:len(nodes)]
	scratch.depths = depths

	// Internal nodes are created in dependency order, so a single reverse pass
	// from the root propagates depths without recursion.
	depths[len(nodes)-1] = 0
	maxDepth := 0
	for i := len(nodes) - 1; i >= 0; i-- {
		childDepth := depths[i] + 1
		for _, child := range children[i] {
			if child < 0 {
				value := leaves[^child].value
				codeLengths[value] = uint8(childDepth)
				if int(childDepth) > maxDepth {
					maxDepth = int(childDepth)
				}
			} else {
				depths[child] = childDepth
			}
		}
	}
	return maxDepth
}

func elosslessReverseBits(code uint32, bits int) uint16 {
	out := uint32(0)
	for i := 0; i < bits; i++ {
		out = (out << 1) | (code & 1)
		code >>= 1
	}
	return uint16(out)
}
