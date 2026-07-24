package webp

import "sort"

// Canonical Huffman code builder for the lossless VP8L encoder.
// Ported from src/encoder/huffman.rs.

const elosslessMaxAllowedCodeLength = 15

type elosslessHuffmanTreeNode struct {
	totalCount uint32
	value      int
	left       int
	right      int
}

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

	countMin := uint32(1)
	for {
		for i := range codeLengths {
			codeLengths[i] = 0
		}

		var tree []elosslessHuffmanTreeNode
		for value, count := range histogram {
			if count != 0 {
				tc := count
				if countMin > tc {
					tc = countMin
				}
				tree = append(tree, elosslessHuffmanTreeNode{
					totalCount: tc,
					value:      value,
					left:       -1,
					right:      -1,
				})
			}
		}
		sort.SliceStable(tree, func(a, b int) bool {
			if tree[a].totalCount != tree[b].totalCount {
				return tree[a].totalCount > tree[b].totalCount
			}
			return tree[a].value < tree[b].value
		})

		if len(tree) == 1 {
			codeLengths[tree[0].value] = 1
		} else {
			treePool := make([]elosslessHuffmanTreeNode, 0, len(tree)*2)
			treeSize := len(tree)
			for treeSize > 1 {
				treePool = append(treePool, tree[treeSize-1])
				treePool = append(treePool, tree[treeSize-2])
				count := treePool[len(treePool)-1].totalCount + treePool[len(treePool)-2].totalCount
				treeSize -= 2

				insertAt := 0
				for insertAt < treeSize && tree[insertAt].totalCount > count {
					insertAt++
				}
				newNode := elosslessHuffmanTreeNode{
					totalCount: count,
					value:      -1,
					left:       len(treePool) - 1,
					right:      len(treePool) - 2,
				}
				tree = append(tree, elosslessHuffmanTreeNode{})
				copy(tree[insertAt+1:], tree[insertAt:])
				tree[insertAt] = newNode
				treeSize++
			}
			elosslessSetBitDepths(&tree[0], treePool, codeLengths, 0)
		}

		maxDepth := 0
		for _, length := range codeLengths {
			if int(length) > maxDepth {
				maxDepth = int(length)
			}
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

func elosslessSetBitDepths(node *elosslessHuffmanTreeNode, pool []elosslessHuffmanTreeNode, bitDepths []uint8, level uint8) {
	if node.left >= 0 {
		elosslessSetBitDepths(&pool[node.left], pool, bitDepths, level+1)
		elosslessSetBitDepths(&pool[node.right], pool, bitDepths, level+1)
	} else {
		bitDepths[node.value] = level
	}
}

func elosslessReverseBits(code uint32, bits int) uint16 {
	out := uint32(0)
	for i := 0; i < bits; i++ {
		out = (out << 1) | (code & 1)
		code >>= 1
	}
	return uint16(out)
}
