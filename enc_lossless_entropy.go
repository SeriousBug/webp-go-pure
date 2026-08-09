package webp

// Huffman coding and image-stream emission for lossless encoding.
// Ported from src/encoder/lossless/entropy.rs.

import (
	"math"
	"sort"
)

func elosslessPrefixEncode(value int) (elosslessPrefixCode, error) {
	if value == 0 {
		return elosslessPrefixCode{}, encInvalidParam("prefix value must be non-zero")
	}

	if value <= 4 {
		return elosslessPrefixCode{symbol: value - 1, extraBits: 0, extraValue: 0}, nil
	}

	value = value - 1
	highestBit := elosslessIlog2(value)
	secondHighestBit := (value >> (highestBit - 1)) & 1
	extraBits := highestBit - 1
	extraValue := value & ((1 << extraBits) - 1)

	return elosslessPrefixCode{
		symbol:     2*highestBit + secondHighestBit,
		extraBits:  extraBits,
		extraValue: extraValue,
	}, nil
}

func elosslessDistanceToPlaneCode(width, distance int) int {
	yoffset := distance / width
	xoffset := distance - yoffset*width

	widthMinus8 := width - 8
	if widthMinus8 < 0 {
		widthMinus8 = 0
	}
	if xoffset <= 8 && yoffset < 8 {
		return int(elosslessPlaneToCodeLut[yoffset*16+8-xoffset]) + 1
	} else if xoffset > widthMinus8 && yoffset < 7 {
		return int(elosslessPlaneToCodeLut[(yoffset+1)*16+8+(width-xoffset)]) + 1
	}
	return distance + 120
}

func elosslessWriteSimpleHuffmanTree(bw *bitWriter, symbols []int) error {
	if len(symbols) == 0 || len(symbols) > 2 {
		return encInvalidParam("simple Huffman tree expects one or two symbols")
	}

	for _, symbol := range symbols {
		if symbol >= (1 << 8) {
			return encInvalidParam("simple Huffman symbol is too large")
		}
	}

	if err := bw.putBits(1, 1); err != nil {
		return err
	}
	if err := bw.putBits(uint32(len(symbols)-1), 1); err != nil {
		return err
	}

	first := symbols[0]
	if first <= 1 {
		if err := bw.putBits(0, 1); err != nil {
			return err
		}
		if err := bw.putBits(uint32(first), 1); err != nil {
			return err
		}
	} else {
		if err := bw.putBits(1, 1); err != nil {
			return err
		}
		if err := bw.putBits(uint32(first), 8); err != nil {
			return err
		}
	}

	if len(symbols) > 1 {
		if err := bw.putBits(uint32(symbols[1]), 8); err != nil {
			return err
		}
	}

	return nil
}

func elosslessWriteTrimmedLength(bw *bitWriter, trimmedLength int) error {
	if trimmedLength < 2 {
		return encBitstream("trimmed Huffman span is too small")
	}
	if trimmedLength == 2 {
		return bw.putBits(0, 5)
	}

	nbits := elosslessIlog2(trimmedLength - 2)
	nbitpairs := nbits/2 + 1
	if nbitpairs > 8 {
		return encBitstream("trimmed Huffman span is too large")
	}
	if err := bw.putBits(uint32(nbitpairs-1), 3); err != nil {
		return err
	}
	return bw.putBits(uint32(trimmedLength-2), nbitpairs*2)
}

func elosslessWriteHuffmanTree(bw *bitWriter, code *elosslessHuffmanCode) error {
	var symbolBuf [2]int
	symbols, usedCount := code.usedSymbols(symbolBuf[:0], 2)
	if usedCount == 0 {
		return encBitstream("empty Huffman tree")
	}
	allSmall := true
	for _, symbol := range symbols {
		if symbol >= (1 << 8) {
			allSmall = false
			break
		}
	}
	if usedCount <= 2 && allSmall {
		return elosslessWriteSimpleHuffmanTree(bw, symbols)
	}

	if err := bw.putBits(0, 1); err != nil {
		return err
	}
	tokens := elosslessCompressHuffmanTree(code.getCodeLengths())

	tokenHistogram := make([]uint32, elosslessNumCodeLengthCodes)
	for _, token := range tokens {
		tokenHistogram[token.code]++
	}
	tokenCode, err := elosslessHuffmanCodeFromHistogram(tokenHistogram, 7)
	if err != nil {
		return err
	}

	codeLengthBitdepth := tokenCode.getCodeLengths()
	codesToStore := elosslessNumCodeLengthCodes
	for codesToStore > 4 && codeLengthBitdepth[elosslessCodeLengthCodeOrder[codesToStore-1]] == 0 {
		codesToStore--
	}
	if err := bw.putBits(uint32(codesToStore-4), 4); err != nil {
		return err
	}
	for i := 0; i < codesToStore; i++ {
		orderedSymbol := elosslessCodeLengthCodeOrder[i]
		if err := bw.putBits(uint32(codeLengthBitdepth[orderedSymbol]), 3); err != nil {
			return err
		}
	}

	trailingZeroBits := 0
	trimmedLength := len(tokens)
	index := len(tokens)
	for index > 0 {
		index--
		token := tokens[index]
		if token.code == 0 || token.code == 17 || token.code == 18 {
			trimmedLength--
			trailingZeroBits += int(codeLengthBitdepth[token.code])
			if token.code == 17 {
				trailingZeroBits += 3
			} else if token.code == 18 {
				trailingZeroBits += 7
			}
		} else {
			break
		}
	}

	writeTrimmed := trimmedLength > 1 && trailingZeroBits > 12
	writeTrimmedBit := uint32(0)
	if writeTrimmed {
		writeTrimmedBit = 1
	}
	if err := bw.putBits(writeTrimmedBit, 1); err != nil {
		return err
	}
	length := len(tokens)
	if writeTrimmed {
		if err := elosslessWriteTrimmedLength(bw, trimmedLength); err != nil {
			return err
		}
		length = trimmedLength
	}

	for i := 0; i < length; i++ {
		token := tokens[i]
		if err := tokenCode.writeSymbol(bw, int(token.code)); err != nil {
			return err
		}
		switch token.code {
		case 16:
			if err := bw.putBits(uint32(token.extraBits), 2); err != nil {
				return err
			}
		case 17:
			if err := bw.putBits(uint32(token.extraBits), 3); err != nil {
				return err
			}
		case 18:
			if err := bw.putBits(uint32(token.extraBits), 7); err != nil {
				return err
			}
		}
	}

	return nil
}

func elosslessBuildHistograms(tokens []elosslessToken, width, colorCacheBits int) (elosslessHistogramSet, error) {
	histograms := elosslessNewHistograms(colorCacheBits)
	for _, token := range tokens {
		if err := elosslessAddTokenToHistograms(&histograms, width, token); err != nil {
			return histograms, err
		}
	}
	elosslessNormalizeHistograms(&histograms)
	return histograms, nil
}

// elosslessHistogramChannelSizes returns the length of each of the five
// per-channel histograms.
func elosslessHistogramChannelSizes(colorCacheBits int) [5]int {
	cacheEntries := 0
	if colorCacheBits > 0 {
		cacheEntries = 1 << colorCacheBits
	}
	return [5]int{
		elosslessNumLiteralCodes + elosslessNumLengthCodes + cacheEntries,
		elosslessNumLiteralCodes,
		elosslessNumLiteralCodes,
		elosslessNumLiteralCodes,
		elosslessNumDistanceCodes,
	}
}

func elosslessNewHistograms(colorCacheBits int) elosslessHistogramSet {
	sizes := elosslessHistogramChannelSizes(colorCacheBits)
	total := 0
	for _, n := range sizes {
		total += n
	}
	return elosslessCarveHistogramSet(make([]uint32, total), sizes)
}

// elosslessCarveHistogramSet splits one backing array into the five channel
// histograms. The five slices are allocated together because a histogram set is
// created per tile and the separate allocations dominate the encoder's object
// count otherwise.
func elosslessCarveHistogramSet(buf []uint32, sizes [5]int) elosslessHistogramSet {
	var set elosslessHistogramSet
	offset := 0
	for c, n := range sizes {
		set[c] = buf[offset : offset+n : offset+n]
		offset += n
	}
	return set
}

// elosslessNewHistogramSets returns count histogram sets carved out of a single
// allocation.
func elosslessNewHistogramSets(count, colorCacheBits int) []elosslessHistogramSet {
	sets, _ := elosslessCarveHistogramSets(make([]uint32, count*elosslessHistogramSetLen(colorCacheBits)), count, colorCacheBits)
	return sets
}

func elosslessHistogramSetLen(colorCacheBits int) int {
	sizes := elosslessHistogramChannelSizes(colorCacheBits)
	total := 0
	for _, n := range sizes {
		total += n
	}
	return total
}

func elosslessCarveHistogramSets(buf []uint32, count, colorCacheBits int) ([]elosslessHistogramSet, int) {
	sizes := elosslessHistogramChannelSizes(colorCacheBits)
	perSet := elosslessHistogramSetLen(colorCacheBits)
	sets := make([]elosslessHistogramSet, count)
	for i := range sets {
		sets[i] = elosslessCarveHistogramSet(buf[i*perSet:(i+1)*perSet], sizes)
	}
	return sets, count * perSet
}

func elosslessAddTokenToHistograms(histograms *elosslessHistogramSet, width int, token elosslessToken) error {
	switch token.kind {
	case elosslessTokLiteral:
		argb := token.argb()
		histograms[0][(argb>>8)&0xff]++
		histograms[1][(argb>>16)&0xff]++
		histograms[2][argb&0xff]++
		histograms[3][(argb>>24)&0xff]++
	case elosslessTokCache:
		histograms[0][elosslessNumLiteralCodes+elosslessNumLengthCodes+int(token.key())]++
	case elosslessTokCopy:
		lengthPrefix, err := elosslessPrefixEncode(int(token.length()))
		if err != nil {
			return err
		}
		histograms[0][elosslessNumLiteralCodes+lengthPrefix.symbol]++

		planeCode := elosslessDistanceToPlaneCode(width, int(token.distance()))
		distPrefix, err := elosslessPrefixEncode(planeCode)
		if err != nil {
			return err
		}
		histograms[4][distPrefix.symbol]++
	}
	return nil
}

func elosslessNormalizeHistograms(histograms *elosslessHistogramSet) {
	for h := 0; h < 4; h++ {
		allZero := true
		for _, count := range histograms[h] {
			if count != 0 {
				allZero = false
				break
			}
		}
		if allZero {
			histograms[h][0] = 1
		}
	}
	allZero := true
	for _, count := range histograms[4] {
		if count != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		histograms[4][0] = 1
	}
}

func elosslessMergeHistograms(dst *elosslessHistogramSet, src *elosslessHistogramSet) {
	for i := 0; i < 5; i++ {
		for j := range dst[i] {
			dst[i][j] += src[i][j]
		}
	}
}

func elosslessBuildGroupCodes(histograms *elosslessHistogramSet) (elosslessHuffmanGroupCodes, error) {
	green, err := elosslessHuffmanCodeFromHistogram(histograms[0], 15)
	if err != nil {
		return elosslessHuffmanGroupCodes{}, err
	}
	red, err := elosslessHuffmanCodeFromHistogram(histograms[1], 15)
	if err != nil {
		return elosslessHuffmanGroupCodes{}, err
	}
	blue, err := elosslessHuffmanCodeFromHistogram(histograms[2], 15)
	if err != nil {
		return elosslessHuffmanGroupCodes{}, err
	}
	alpha, err := elosslessHuffmanCodeFromHistogram(histograms[3], 15)
	if err != nil {
		return elosslessHuffmanGroupCodes{}, err
	}
	dist, err := elosslessHuffmanCodeFromHistogram(histograms[4], 15)
	if err != nil {
		return elosslessHuffmanGroupCodes{}, err
	}
	return elosslessHuffmanGroupCodes{green: green, red: red, blue: blue, alpha: alpha, dist: dist}, nil
}

func elosslessTileIndexForPos(width, huffmanBits, huffmanXsize, pos int) int {
	x := pos % width
	y := pos / width
	return (y>>huffmanBits)*huffmanXsize + (x >> huffmanBits)
}

func elosslessHistogramCost(histograms *elosslessHistogramSet, codes *elosslessHuffmanGroupCodes) int {
	sum := func(hist []uint32, code *elosslessHuffmanCode) int {
		lengths := code.getCodeLengths()
		total := 0
		n := len(hist)
		if len(lengths) < n {
			n = len(lengths)
		}
		for i := 0; i < n; i++ {
			total += int(hist[i]) * int(lengths[i])
		}
		return total
	}
	return sum(histograms[0], &codes.green) +
		sum(histograms[1], &codes.red) +
		sum(histograms[2], &codes.blue) +
		sum(histograms[3], &codes.alpha) +
		sum(histograms[4], &codes.dist)
}

// elosslessSparseHist stores only the non-zero (symbol, count) entries of a
// histogram set. Per-tile histograms are sparse (a small image tile touches few
// distinct residual values), so evaluating the assignment cost against a group's
// Huffman code lengths over just the non-zeros is far cheaper than scanning the
// full dense arrays, while producing identical totals.
type elosslessSparseEntry struct {
	count  uint32
	symbol uint16
}

type elosslessSparseHist struct {
	entries []elosslessSparseEntry
	// ends[c] is where channel c's entries stop within entries; channel 0
	// starts at zero and each later channel starts where the previous ended.
	ends [5]int32
}

func (s *elosslessSparseHist) channel(c int) []elosslessSparseEntry {
	start := int32(0)
	if c > 0 {
		start = s.ends[c-1]
	}
	return s.entries[start:s.ends[c]]
}

func elosslessCountNonZeros(h *elosslessHistogramSet) int {
	n := 0
	for c := 0; c < 5; c++ {
		for _, v := range h[c] {
			if v != 0 {
				n++
			}
		}
	}
	return n
}

// elosslessMakeSparseHist appends the non-zero entries of h to arena and
// returns a sparse histogram viewing them. The caller sizes arena for every
// tile up front so that the per-tile views share one allocation.
func elosslessMakeSparseHist(h *elosslessHistogramSet, arena []elosslessSparseEntry) (elosslessSparseHist, []elosslessSparseEntry) {
	start := len(arena)
	var s elosslessSparseHist
	for c := 0; c < 5; c++ {
		for i, v := range h[c] {
			if v != 0 {
				arena = append(arena, elosslessSparseEntry{count: v, symbol: uint16(i)})
			}
		}
		s.ends[c] = int32(len(arena) - start)
	}
	s.entries = arena[start:len(arena):len(arena)]
	return s, arena
}

func elosslessMergeSparseInto(dst *elosslessHistogramSet, s *elosslessSparseHist) {
	for c := 0; c < 5; c++ {
		d := dst[c]
		for _, e := range s.channel(c) {
			d[e.symbol] += e.count
		}
	}
}

func elosslessSparseHistCost(s *elosslessSparseHist, codes *elosslessHuffmanGroupCodes) int {
	channelCodes := [5]*elosslessHuffmanCode{&codes.green, &codes.red, &codes.blue, &codes.alpha, &codes.dist}
	total := 0
	for c := 0; c < 5; c++ {
		lengths := channelCodes[c].getCodeLengths()
		nLen := len(lengths)
		for _, e := range s.channel(c) {
			if int(e.symbol) < nLen {
				total += int(e.count) * int(lengths[e.symbol])
			}
		}
	}
	return total
}

// elosslessHistogramEntropyCost returns the Shannon cost in bits of a dense
// histogram. Zero counts contribute nothing, so the sparse and dense forms
// share one summation.
func elosslessHistogramEntropyCost(histogram []uint32) float64 {
	return elosslessChannelEntropyOfCounts(histogram)
}

func elosslessHistogramSignatureCosts(histograms *elosslessHistogramSet) [3]float64 {
	return [3]float64{
		elosslessHistogramEntropyCost(histograms[0]),
		elosslessHistogramEntropyCost(histograms[1]),
		elosslessHistogramEntropyCost(histograms[2]),
	}
}

func elosslessHistogramPartitionIndex(value, minValue, maxValue float64, partitions int) int {
	if partitions <= 1 || maxValue <= minValue {
		return 0
	}

	normalized := (value - minValue) / (maxValue - minValue)
	if normalized < 0.0 {
		normalized = 0.0
	} else if normalized > 1.0 {
		normalized = 1.0
	}
	index := int(normalized * float64(partitions))
	if index > partitions-1 {
		index = partitions - 1
	}
	return index
}

func elosslessEntropyHistogramCandidates(nonEmptyTiles [][2]int, tileHistograms []elosslessHistogramSet, targetCount int) []elosslessHistogramCandidate {
	if targetCount == 0 || len(nonEmptyTiles) == 0 {
		return nil
	}

	signatures := make([][3]float64, len(nonEmptyTiles))
	for i, t := range nonEmptyTiles {
		signatures[i] = elosslessHistogramSignatureCosts(&tileHistograms[t[0]])
	}

	mins := [3]float64{math.Inf(1), math.Inf(1), math.Inf(1)}
	maxs := [3]float64{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for _, sig := range signatures {
		for k := 0; k < 3; k++ {
			mins[k] = math.Min(mins[k], sig[k])
			maxs[k] = math.Max(maxs[k], sig[k])
		}
	}

	binCount := elosslessNumHistogramPartitions * elosslessNumHistogramPartitions * elosslessNumHistogramPartitions
	bins := make([]*elosslessHistogramCandidate, binCount)

	for i, t := range nonEmptyTiles {
		tile := t[0]
		weight := t[1]
		signature := signatures[i]
		greenBin := elosslessHistogramPartitionIndex(signature[0], mins[0], maxs[0], elosslessNumHistogramPartitions)
		redBin := elosslessHistogramPartitionIndex(signature[1], mins[1], maxs[1], elosslessNumHistogramPartitions)
		blueBin := elosslessHistogramPartitionIndex(signature[2], mins[2], maxs[2], elosslessNumHistogramPartitions)
		binIndex := greenBin*elosslessNumHistogramPartitions*elosslessNumHistogramPartitions +
			redBin*elosslessNumHistogramPartitions + blueBin

		if bins[binIndex] != nil {
			elosslessMergeHistograms(&bins[binIndex].histograms, &tileHistograms[tile])
			bins[binIndex].weight += weight
		} else {
			histograms := elosslessCloneHistogramSet(&tileHistograms[tile])
			elosslessNormalizeHistograms(&histograms)
			bins[binIndex] = &elosslessHistogramCandidate{histograms: histograms, weight: weight}
		}
	}

	var candidates []elosslessHistogramCandidate
	for _, b := range bins {
		if b != nil {
			candidates = append(candidates, *b)
		}
	}
	if len(candidates) == 0 {
		return nil
	}

	// Greedy pairwise merge minimizing total entropy. The merge penalty for a
	// pair is combinedEntropy(i,j) - selfCost[i] - selfCost[j]; caching each
	// candidate's self-entropy and the pairwise combined entropies keeps the
	// entropy evaluations at O(n^2) overall instead of recomputing them for
	// every pair on every merge step.
	n := len(candidates)
	var work elosslessEntropyWork
	nonZeros := make([]elosslessHistogramNonZeros, n)
	selfCost := make([]float64, n)
	comb := make([][]float64, n)
	for i := 0; i < n; i++ {
		nonZeros[i].rebuild(&candidates[i].histograms)
		selfCost[i] = work.setEntropy(&candidates[i].histograms, &nonZeros[i])
		comb[i] = make([]float64, n)
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			comb[i][j] = work.combinedSetEntropy(&candidates[i].histograms, &nonZeros[i], &candidates[j].histograms, &nonZeros[j])
		}
	}
	recomputeRow := func(i, active int) {
		for j := 0; j < active; j++ {
			if j == i {
				continue
			}
			v := work.combinedSetEntropy(&candidates[i].histograms, &nonZeros[i], &candidates[j].histograms, &nonZeros[j])
			if i < j {
				comb[i][j] = v
			} else {
				comb[j][i] = v
			}
		}
	}

	for n > targetCount {
		bestLhs, bestRhs := -1, -1
		bestPenalty := math.Inf(1)
		for lhs := 0; lhs < n; lhs++ {
			row := comb[lhs]
			for rhs := lhs + 1; rhs < n; rhs++ {
				penalty := row[rhs] - selfCost[lhs] - selfCost[rhs]
				if penalty < bestPenalty {
					bestPenalty = penalty
					bestLhs, bestRhs = lhs, rhs
				}
			}
		}

		if bestLhs < 0 {
			break
		}
		rhsCandidate := candidates[bestRhs]
		elosslessMergeHistograms(&candidates[bestLhs].histograms, &rhsCandidate.histograms)
		elosslessNormalizeHistograms(&candidates[bestLhs].histograms)
		nonZeros[bestLhs].union(&nonZeros[bestRhs], &work)
		candidates[bestLhs].weight += rhsCandidate.weight
		selfCost[bestLhs] = work.setEntropy(&candidates[bestLhs].histograms, &nonZeros[bestLhs])

		last := n - 1
		candidates[bestRhs] = candidates[last]
		selfCost[bestRhs] = selfCost[last]
		nonZeros[bestRhs], nonZeros[last] = nonZeros[last], nonZeros[bestRhs]
		candidates = candidates[:last]
		n = last
		// The candidate moved into bestRhs keeps its combined entropies, so its
		// row is a copy of the vacated one rather than a recomputation.
		if bestRhs < n {
			for j := 0; j < n; j++ {
				if j == bestRhs {
					continue
				}
				v := elosslessCombGet(comb, last, j)
				elosslessCombSet(comb, bestRhs, j, v)
			}
		}
		recomputeRow(bestLhs, n)
	}

	return candidates
}

func elosslessCombGet(comb [][]float64, i, j int) float64 {
	if i < j {
		return comb[i][j]
	}
	return comb[j][i]
}

func elosslessCombSet(comb [][]float64, i, j int, v float64) {
	if i < j {
		comb[i][j] = v
	} else {
		comb[j][i] = v
	}
}

// elosslessHistogramNonZeros lists, per channel, the ascending indices whose
// count is non-zero. Candidate histograms are sparse, and every entropy sum
// skips zero counts anyway, so walking these lists visits the same terms in the
// same order as a dense scan while touching far fewer entries.
type elosslessHistogramNonZeros struct {
	idx [5][]uint16
}

func (nz *elosslessHistogramNonZeros) rebuild(h *elosslessHistogramSet) {
	for c := 0; c < 5; c++ {
		list := nz.idx[c][:0]
		for i, v := range h[c] {
			if v != 0 {
				list = append(list, uint16(i))
			}
		}
		nz.idx[c] = list
	}
}

// union folds other's non-zero indices into nz, matching a histogram merge.
func (nz *elosslessHistogramNonZeros) union(other *elosslessHistogramNonZeros, work *elosslessEntropyWork) {
	for c := 0; c < 5; c++ {
		merged := work.indices[:0]
		a, b := nz.idx[c], other.idx[c]
		i, j := 0, 0
		for i < len(a) && j < len(b) {
			switch {
			case a[i] < b[j]:
				merged = append(merged, a[i])
				i++
			case b[j] < a[i]:
				merged = append(merged, b[j])
				j++
			default:
				merged = append(merged, a[i])
				i++
				j++
			}
		}
		merged = append(merged, a[i:]...)
		merged = append(merged, b[j:]...)
		work.indices = merged
		nz.idx[c] = append(nz.idx[c][:0], merged...)
	}
}

// elosslessEntropyWork holds the scratch buffers of the candidate merge search.
type elosslessEntropyWork struct {
	counts  []uint32
	indices []uint16
}

// elosslessLog2Table caches log2 of the small integers the histogram counts
// almost always are. Each entry is what math.Log2 returns for that integer, so
// a lookup is not an approximation of it.
var elosslessLog2Table = func() [1 << 14]float64 {
	var table [1 << 14]float64
	for i := 1; i < len(table); i++ {
		table[i] = math.Log2(float64(i))
	}
	return table
}()

func elosslessLog2OfCount(count uint64) float64 {
	if count < uint64(len(elosslessLog2Table)) {
		return elosslessLog2Table[count]
	}
	return math.Log2(float64(count))
}

// elosslessChannelEntropyOfCounts returns the Shannon cost in bits of the
// non-zero counts it is given. It sums the counts' own log terms rather than
// taking a log of total/count per symbol, which turns the per-symbol division
// and logarithm into a table lookup.
func elosslessChannelEntropyOfCounts(counts []uint32) float64 {
	total := uint64(0)
	sum := 0.0
	for _, count := range counts {
		total += uint64(count)
		sum -= float64(count) * elosslessLog2OfCount(uint64(count))
	}
	if total == 0 {
		return 0.0
	}
	return sum + float64(total)*elosslessLog2OfCount(total)
}

func (w *elosslessEntropyWork) setEntropy(h *elosslessHistogramSet, nz *elosslessHistogramNonZeros) float64 {
	sum := 0.0
	for c := 0; c < 5; c++ {
		hist := h[c]
		counts := w.counts[:0]
		for _, i := range nz.idx[c] {
			counts = append(counts, hist[i])
		}
		w.counts = counts
		sum += elosslessChannelEntropyOfCounts(counts)
	}
	return sum
}

func (w *elosslessEntropyWork) combinedSetEntropy(a *elosslessHistogramSet, nza *elosslessHistogramNonZeros, b *elosslessHistogramSet, nzb *elosslessHistogramNonZeros) float64 {
	sum := 0.0
	for c := 0; c < 5; c++ {
		ha, hb := a[c], b[c]
		ia, ib := nza.idx[c], nzb.idx[c]
		counts := w.counts[:0]
		i, j := 0, 0
		for i < len(ia) && j < len(ib) {
			switch {
			case ia[i] < ib[j]:
				counts = append(counts, ha[ia[i]])
				i++
			case ib[j] < ia[i]:
				counts = append(counts, hb[ib[j]])
				j++
			default:
				counts = append(counts, ha[ia[i]]+hb[ib[j]])
				i++
				j++
			}
		}
		for ; i < len(ia); i++ {
			counts = append(counts, ha[ia[i]])
		}
		for ; j < len(ib); j++ {
			counts = append(counts, hb[ib[j]])
		}
		w.counts = counts
		sum += elosslessChannelEntropyOfCounts(counts)
	}
	return sum
}

func elosslessBuildEntropySeedHistograms(nonEmptyTiles [][2]int, tileHistograms []elosslessHistogramSet, groupCount int) []elosslessHistogramSet {
	candidates := elosslessEntropyHistogramCandidates(nonEmptyTiles, tileHistograms, groupCount)
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[j].weight < candidates[i].weight
	})
	if len(candidates) > groupCount {
		candidates = candidates[:groupCount]
	}
	out := make([]elosslessHistogramSet, len(candidates))
	for i := range candidates {
		out[i] = candidates[i].histograms
	}
	return out
}

func elosslessBuildWeightedSeedHistograms(nonEmptyTiles [][2]int, tileHistograms []elosslessHistogramSet, groupCount int) []elosslessHistogramSet {
	n := groupCount
	if len(nonEmptyTiles) < n {
		n = len(nonEmptyTiles)
	}
	out := make([]elosslessHistogramSet, 0, n)
	for i := 0; i < n; i++ {
		histograms := elosslessCloneHistogramSet(&tileHistograms[nonEmptyTiles[i][0]])
		elosslessNormalizeHistograms(&histograms)
		out = append(out, histograms)
	}
	return out
}

func elosslessAssignTilesToGroups(nonEmptyTiles [][2]int, tileSparse []elosslessSparseHist, groupCodes []elosslessHuffmanGroupCodes, assignments []int) {
	for _, t := range nonEmptyTiles {
		tile := t[0]
		bestGroup := 0
		bestCost := elosslessIntMax
		for groupIndex := range groupCodes {
			cost := elosslessSparseHistCost(&tileSparse[tile], &groupCodes[groupIndex])
			if cost < bestCost {
				bestCost = cost
				bestGroup = groupIndex
			}
		}
		assignments[tile] = bestGroup
	}
}

func elosslessRefineMetaHuffmanPlan(tileCount, colorCacheBits int, nonEmptyTiles [][2]int, tileSparse []elosslessSparseHist, seedHistograms []elosslessHistogramSet) (*elosslessMetaHuffmanPlan, error) {
	if len(seedHistograms) <= 1 {
		return nil, nil
	}

	groupCodes := make([]elosslessHuffmanGroupCodes, 0, len(seedHistograms))
	for i := range seedHistograms {
		code, err := elosslessBuildGroupCodes(&seedHistograms[i])
		if err != nil {
			return nil, err
		}
		groupCodes = append(groupCodes, code)
	}
	assignments := make([]int, tileCount)

	for iter := 0; iter < 4; iter++ {
		elosslessAssignTilesToGroups(nonEmptyTiles, tileSparse, groupCodes, assignments)

		accum := elosslessNewHistogramSets(len(groupCodes), colorCacheBits)
		used := make([]bool, len(groupCodes))
		for _, t := range nonEmptyTiles {
			tile := t[0]
			g := assignments[tile]
			elosslessMergeSparseInto(&accum[g], &tileSparse[tile])
			used[g] = true
		}
		remap := make([]int, len(groupCodes))
		for i := range remap {
			remap[i] = elosslessIntMax
		}
		var mergedHistograms []elosslessHistogramSet
		for groupIndex := range accum {
			if used[groupIndex] {
				elosslessNormalizeHistograms(&accum[groupIndex])
				remap[groupIndex] = len(mergedHistograms)
				mergedHistograms = append(mergedHistograms, accum[groupIndex])
			}
		}
		if len(mergedHistograms) <= 1 {
			return nil, nil
		}
		for _, t := range nonEmptyTiles {
			tile := t[0]
			assignments[tile] = remap[assignments[tile]]
		}
		groupCodes = groupCodes[:0]
		for i := range mergedHistograms {
			code, err := elosslessBuildGroupCodes(&mergedHistograms[i])
			if err != nil {
				return nil, err
			}
			groupCodes = append(groupCodes, code)
		}
	}

	return &elosslessMetaHuffmanPlan{
		huffmanBits:  0,
		huffmanXsize: 0,
		assignments:  assignments,
		groups:       groupCodes,
	}, nil
}

func elosslessMetaHuffmanAssignmentCost(nonEmptyTiles [][2]int, tileSparse []elosslessSparseHist, plan *elosslessMetaHuffmanPlan) int {
	total := 0
	for _, t := range nonEmptyTiles {
		tile := t[0]
		total += elosslessSparseHistCost(&tileSparse[tile], &plan.groups[plan.assignments[tile]])
	}
	return total
}

// elosslessApplyColorCacheToTokens rewrites a token stream so that literals
// already held in the color cache become cache references. The mapping is one
// output token per input token, so dst may alias tokens for an in-place rewrite;
// it must otherwise have the same length. Callers pass a reused scratch buffer
// because the stream is one token per pixel.
func elosslessApplyColorCacheToTokens(dst []elosslessToken, argb []uint32, tokens []elosslessToken, colorCacheBits int) error {
	if len(dst) != len(tokens) {
		return encInvalidParam("color cache token buffer length mismatch")
	}
	if colorCacheBits == 0 {
		copy(dst, tokens)
		return nil
	}

	cache, err := elosslessColorCacheNew(colorCacheBits)
	if err != nil {
		return err
	}
	pixelIndex := 0

	for i, token := range tokens {
		switch token.kind {
		case elosslessTokLiteral:
			pixel := token.argb()
			if key, ok := cache.lookup(pixel); ok {
				dst[i] = elosslessCacheToken(uint16(key))
			} else {
				dst[i] = elosslessLiteralToken(pixel)
				cache.insert(pixel)
			}
			pixelIndex++
		case elosslessTokCache:
			dst[i] = elosslessCacheToken(token.key())
			pixelIndex++
		case elosslessTokCopy:
			length := int(token.length())
			dst[i] = elosslessCopyToken(token.distance(), token.length())
			for _, pixel := range argb[pixelIndex : pixelIndex+length] {
				cache.insert(pixel)
			}
			pixelIndex += length
		}
	}

	return nil
}

// elosslessTileHistograms holds the per-tile token histograms of one
// huffmanBits level along with the data the plan search derives from them.
type elosslessTileHistograms struct {
	huffmanBits   int
	huffmanXsize  int
	huffmanYsize  int
	histograms    []elosslessHistogramSet
	weights       []int
	nonEmptyTiles [][2]int
	sparse        []elosslessSparseHist
	buf           *[]uint32
}

// release returns the level's histogram storage to the pool. The caller must
// have finished with the histograms; the plan search keeps only clones of them.
func (t *elosslessTileHistograms) release() {
	if t.buf != nil {
		elosslessReturnU32Buf(t.buf)
		t.buf = nil
	}
}

func (t *elosslessTileHistograms) allocHistograms(colorCacheBits int) {
	count := t.tileCount()
	t.buf = elosslessBorrowZeroedU32Buf(count * elosslessHistogramSetLen(colorCacheBits))
	t.histograms, _ = elosslessCarveHistogramSets(*t.buf, count, colorCacheBits)
	t.weights = make([]int, count)
}

func (t *elosslessTileHistograms) tileCount() int { return t.huffmanXsize * t.huffmanYsize }

// finish computes the non-empty tile list and the sparse histograms the
// assignment cost is evaluated against.
func (t *elosslessTileHistograms) finish() {
	for index, weight := range t.weights {
		if weight != 0 {
			t.nonEmptyTiles = append(t.nonEmptyTiles, [2]int{index, weight})
		}
	}
	sort.SliceStable(t.nonEmptyTiles, func(i, j int) bool {
		return t.nonEmptyTiles[j][1] < t.nonEmptyTiles[i][1]
	})
	t.sparse = make([]elosslessSparseHist, t.tileCount())
	nonZeros := 0
	for _, tile := range t.nonEmptyTiles {
		nonZeros += elosslessCountNonZeros(&t.histograms[tile[0]])
	}
	arena := make([]elosslessSparseEntry, 0, nonZeros)
	for _, tile := range t.nonEmptyTiles {
		t.sparse[tile[0]], arena = elosslessMakeSparseHist(&t.histograms[tile[0]], arena)
	}
}

func elosslessScanTileHistograms(width, height int, tokens []elosslessToken, colorCacheBits, huffmanBits int) (*elosslessTileHistograms, error) {
	level := &elosslessTileHistograms{
		huffmanBits:  huffmanBits,
		huffmanXsize: elosslessSubsampleSize(width, huffmanBits),
		huffmanYsize: elosslessSubsampleSize(height, huffmanBits),
	}
	level.allocHistograms(colorCacheBits)

	pos := 0
	for _, token := range tokens {
		tile := elosslessTileIndexForPos(width, huffmanBits, level.huffmanXsize, pos)
		if err := elosslessAddTokenToHistograms(&level.histograms[tile], width, token); err != nil {
			return nil, err
		}
		level.weights[tile] += elosslessTokenLen(token)
		pos += elosslessTokenLen(token)
	}
	level.finish()
	return level, nil
}

// elosslessCoarsenTileHistograms returns the level one huffmanBits above fine.
// Each coarse tile is exactly the union of the 2x2 block of fine tiles under
// it, so its histogram is their sum and the token stream need not be rescanned.
func elosslessCoarsenTileHistograms(fine *elosslessTileHistograms, width, height, colorCacheBits int) *elosslessTileHistograms {
	huffmanBits := fine.huffmanBits + 1
	coarse := &elosslessTileHistograms{
		huffmanBits:  huffmanBits,
		huffmanXsize: elosslessSubsampleSize(width, huffmanBits),
		huffmanYsize: elosslessSubsampleSize(height, huffmanBits),
	}
	coarse.allocHistograms(colorCacheBits)

	for y := 0; y < fine.huffmanYsize; y++ {
		for x := 0; x < fine.huffmanXsize; x++ {
			src := y*fine.huffmanXsize + x
			if fine.weights[src] == 0 {
				continue
			}
			dst := (y>>1)*coarse.huffmanXsize + (x >> 1)
			elosslessMergeHistograms(&coarse.histograms[dst], &fine.histograms[src])
			coarse.weights[dst] += fine.weights[src]
		}
	}
	coarse.finish()
	return coarse
}

// elosslessTileHistogramLevels builds the tile histograms for every huffmanBits
// in candidates. Only the finest level scans the token stream; the coarser ones
// are summed up from it.
func elosslessTileHistogramLevels(width, height int, tokens []elosslessToken, colorCacheBits int, candidates [][2]int) (map[int]*elosslessTileHistograms, error) {
	minBits, maxBits := 0, 0
	for _, hc := range candidates {
		bits := hc[0]
		if bits < elosslessMinHuffmanBits || bits >= elosslessMinHuffmanBits+(1<<elosslessNumHuffmanBits) {
			continue
		}
		if minBits == 0 || bits < minBits {
			minBits = bits
		}
		if bits > maxBits {
			maxBits = bits
		}
	}
	levels := make(map[int]*elosslessTileHistograms, maxBits-minBits+1)
	if minBits == 0 {
		return levels, nil
	}

	level, err := elosslessScanTileHistograms(width, height, tokens, colorCacheBits, minBits)
	if err != nil {
		return nil, err
	}
	levels[minBits] = level
	for bits := minBits + 1; bits <= maxBits; bits++ {
		level = elosslessCoarsenTileHistograms(level, width, height, colorCacheBits)
		levels[bits] = level
	}
	return levels, nil
}

func elosslessBuildMetaHuffmanPlan(colorCacheBits, maxGroups int, level *elosslessTileHistograms) (*elosslessMetaHuffmanPlan, error) {
	if level == nil {
		return nil, nil
	}
	huffmanBits := level.huffmanBits
	if huffmanBits < elosslessMinHuffmanBits || huffmanBits >= elosslessMinHuffmanBits+(1<<elosslessNumHuffmanBits) {
		return nil, nil
	}

	huffmanXsize := level.huffmanXsize
	tileCount := level.tileCount()
	if tileCount <= 1 {
		return nil, nil
	}

	tileHistograms := level.histograms
	nonEmptyTiles := level.nonEmptyTiles
	if len(nonEmptyTiles) <= 1 {
		return nil, nil
	}
	tileSparse := level.sparse

	groupCount := maxGroups
	if len(nonEmptyTiles) < groupCount {
		groupCount = len(nonEmptyTiles)
	}
	if groupCount <= 1 {
		return nil, nil
	}

	seedCandidates := [][]elosslessHistogramSet{
		elosslessBuildWeightedSeedHistograms(nonEmptyTiles, tileHistograms, groupCount),
		elosslessBuildEntropySeedHistograms(nonEmptyTiles, tileHistograms, groupCount),
	}

	var bestPlan *elosslessMetaHuffmanPlan
	bestCost := elosslessIntMax
	for _, seedHistograms := range seedCandidates {
		plan, err := elosslessRefineMetaHuffmanPlan(tileCount, colorCacheBits, nonEmptyTiles, tileSparse, seedHistograms)
		if err != nil {
			return nil, err
		}
		if plan != nil {
			plan.huffmanBits = huffmanBits
			plan.huffmanXsize = huffmanXsize
			cost := elosslessMetaHuffmanAssignmentCost(nonEmptyTiles, tileSparse, plan)
			if cost < bestCost {
				bestCost = cost
				bestPlan = plan
			}
		}
	}

	return bestPlan, nil
}

func elosslessWriteHuffmanGroup(bw *bitWriter, group *elosslessHuffmanGroupCodes) error {
	if err := elosslessWriteHuffmanTree(bw, &group.green); err != nil {
		return err
	}
	if err := elosslessWriteHuffmanTree(bw, &group.red); err != nil {
		return err
	}
	if err := elosslessWriteHuffmanTree(bw, &group.blue); err != nil {
		return err
	}
	if err := elosslessWriteHuffmanTree(bw, &group.alpha); err != nil {
		return err
	}
	return elosslessWriteHuffmanTree(bw, &group.dist)
}

func elosslessWriteTokensWithMeta(bw *bitWriter, tokens []elosslessToken, width int, plan *elosslessMetaHuffmanPlan) error {
	pos := 0
	for _, token := range tokens {
		tile := elosslessTileIndexForPos(width, plan.huffmanBits, plan.huffmanXsize, pos)
		group := &plan.groups[plan.assignments[tile]]
		switch token.kind {
		case elosslessTokLiteral:
			argb := token.argb()
			green := int((argb >> 8) & 0xff)
			red := int((argb >> 16) & 0xff)
			blue := int(argb & 0xff)
			alpha := int((argb >> 24) & 0xff)

			if err := group.green.writeSymbol(bw, green); err != nil {
				return err
			}
			if err := group.red.writeSymbol(bw, red); err != nil {
				return err
			}
			if err := group.blue.writeSymbol(bw, blue); err != nil {
				return err
			}
			if err := group.alpha.writeSymbol(bw, alpha); err != nil {
				return err
			}
		case elosslessTokCache:
			if err := group.green.writeSymbol(bw, elosslessNumLiteralCodes+elosslessNumLengthCodes+int(token.key())); err != nil {
				return err
			}
		case elosslessTokCopy:
			lengthPrefix, err := elosslessPrefixEncode(int(token.length()))
			if err != nil {
				return err
			}
			if err := group.green.writeSymbol(bw, elosslessNumLiteralCodes+lengthPrefix.symbol); err != nil {
				return err
			}
			if lengthPrefix.extraBits > 0 {
				if err := bw.putBits(uint32(lengthPrefix.extraValue), lengthPrefix.extraBits); err != nil {
					return err
				}
			}

			planeCode := elosslessDistanceToPlaneCode(width, int(token.distance()))
			distPrefix, err := elosslessPrefixEncode(planeCode)
			if err != nil {
				return err
			}
			if err := group.dist.writeSymbol(bw, distPrefix.symbol); err != nil {
				return err
			}
			if distPrefix.extraBits > 0 {
				if err := bw.putBits(uint32(distPrefix.extraValue), distPrefix.extraBits); err != nil {
					return err
				}
			}
		}
		pos += elosslessTokenLen(token)
	}
	return nil
}

func elosslessWriteTokens(bw *bitWriter, tokens []elosslessToken, width int, greenCodes, redCodes, blueCodes, alphaCodes, distCodes *elosslessHuffmanCode) error {
	for _, token := range tokens {
		switch token.kind {
		case elosslessTokLiteral:
			argb := token.argb()
			green := int((argb >> 8) & 0xff)
			red := int((argb >> 16) & 0xff)
			blue := int(argb & 0xff)
			alpha := int((argb >> 24) & 0xff)

			if err := greenCodes.writeSymbol(bw, green); err != nil {
				return err
			}
			if err := redCodes.writeSymbol(bw, red); err != nil {
				return err
			}
			if err := blueCodes.writeSymbol(bw, blue); err != nil {
				return err
			}
			if err := alphaCodes.writeSymbol(bw, alpha); err != nil {
				return err
			}
		case elosslessTokCache:
			if err := greenCodes.writeSymbol(bw, elosslessNumLiteralCodes+elosslessNumLengthCodes+int(token.key())); err != nil {
				return err
			}
		case elosslessTokCopy:
			lengthPrefix, err := elosslessPrefixEncode(int(token.length()))
			if err != nil {
				return err
			}
			if err := greenCodes.writeSymbol(bw, elosslessNumLiteralCodes+lengthPrefix.symbol); err != nil {
				return err
			}
			if lengthPrefix.extraBits > 0 {
				if err := bw.putBits(uint32(lengthPrefix.extraValue), lengthPrefix.extraBits); err != nil {
					return err
				}
			}

			planeCode := elosslessDistanceToPlaneCode(width, int(token.distance()))
			distPrefix, err := elosslessPrefixEncode(planeCode)
			if err != nil {
				return err
			}
			if err := distCodes.writeSymbol(bw, distPrefix.symbol); err != nil {
				return err
			}
			if distPrefix.extraBits > 0 {
				if err := bw.putBits(uint32(distPrefix.extraValue), distPrefix.extraBits); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func elosslessWriteSingleGroupPrefix(bw *bitWriter, allowMetaHuffman bool, colorCacheBits int, group *elosslessHuffmanGroupCodes) error {
	if err := bw.putBits(elosslessBoolBit(colorCacheBits > 0), 1); err != nil {
		return err
	}
	if colorCacheBits > 0 {
		if err := bw.putBits(uint32(colorCacheBits), 4); err != nil {
			return err
		}
	}
	if allowMetaHuffman {
		if err := bw.putBits(0, 1); err != nil {
			return err
		}
	}
	return elosslessWriteHuffmanGroup(bw, group)
}

func elosslessWriteSingleGroupImageStream(bw *bitWriter, width int, tokens []elosslessToken, allowMetaHuffman bool, colorCacheBits int, group *elosslessHuffmanGroupCodes) error {
	if err := elosslessWriteSingleGroupPrefix(bw, allowMetaHuffman, colorCacheBits, group); err != nil {
		return err
	}
	return elosslessWriteTokens(bw, tokens, width, &group.green, &group.red, &group.blue, &group.alpha, &group.dist)
}

// elosslessCountSingleGroupTokenBits returns the exact number of bits
// elosslessWriteTokens would emit for these tokens under one Huffman group,
// without emitting them, by summing the code depths and extra bits.
func elosslessCountSingleGroupTokenBits(tokens []elosslessToken, width int, group *elosslessHuffmanGroupCodes) (int, error) {
	bits := 0
	for _, token := range tokens {
		switch token.kind {
		case elosslessTokLiteral:
			argb := token.argb()
			bits += group.green.symbolDepth(int((argb >> 8) & 0xff))
			bits += group.red.symbolDepth(int((argb >> 16) & 0xff))
			bits += group.blue.symbolDepth(int(argb & 0xff))
			bits += group.alpha.symbolDepth(int((argb >> 24) & 0xff))
		case elosslessTokCache:
			bits += group.green.symbolDepth(elosslessNumLiteralCodes + elosslessNumLengthCodes + int(token.key()))
		case elosslessTokCopy:
			lengthPrefix, err := elosslessPrefixEncode(int(token.length()))
			if err != nil {
				return 0, err
			}
			bits += group.green.symbolDepth(elosslessNumLiteralCodes+lengthPrefix.symbol) + lengthPrefix.extraBits
			planeCode := elosslessDistanceToPlaneCode(width, int(token.distance()))
			distPrefix, err := elosslessPrefixEncode(planeCode)
			if err != nil {
				return 0, err
			}
			bits += group.dist.symbolDepth(distPrefix.symbol) + distPrefix.extraBits
		}
	}
	return bits, nil
}

func elosslessWriteMetaHuffmanPrefix(bw *bitWriter, colorCacheBits int, plan *elosslessMetaHuffmanPlan) error {
	if err := bw.putBits(elosslessBoolBit(colorCacheBits > 0), 1); err != nil {
		return err
	}
	if colorCacheBits > 0 {
		if err := bw.putBits(uint32(colorCacheBits), 4); err != nil {
			return err
		}
	}
	if err := bw.putBits(1, 1); err != nil {
		return err
	}
	if err := bw.putBits(uint32(plan.huffmanBits-elosslessMinHuffmanBits), elosslessNumHuffmanBits); err != nil {
		return err
	}

	huffmanImage := make([]uint32, len(plan.assignments))
	for i, group := range plan.assignments {
		huffmanImage[i] = (uint32(group>>8) << 16) | (uint32(group&0xff) << 8)
	}
	if err := elosslessWriteImageStream(bw, plan.huffmanXsize, huffmanImage, false, 0, elosslessTokenBuildOptions{}); err != nil {
		return err
	}

	for i := range plan.groups {
		if err := elosslessWriteHuffmanGroup(bw, &plan.groups[i]); err != nil {
			return err
		}
	}
	return nil
}

func elosslessWriteMetaHuffmanImageStream(bw *bitWriter, width int, tokens []elosslessToken, colorCacheBits int, plan *elosslessMetaHuffmanPlan) error {
	if err := elosslessWriteMetaHuffmanPrefix(bw, colorCacheBits, plan); err != nil {
		return err
	}
	return elosslessWriteTokensWithMeta(bw, tokens, width, plan)
}

// elosslessCountMetaTokenBits returns the exact number of bits
// elosslessWriteTokensWithMeta would emit for these tokens, without emitting them.
func elosslessCountMetaTokenBits(tokens []elosslessToken, width int, plan *elosslessMetaHuffmanPlan) (int, error) {
	bits := 0
	pos := 0
	for _, token := range tokens {
		tile := elosslessTileIndexForPos(width, plan.huffmanBits, plan.huffmanXsize, pos)
		group := &plan.groups[plan.assignments[tile]]
		switch token.kind {
		case elosslessTokLiteral:
			argb := token.argb()
			bits += group.green.symbolDepth(int((argb >> 8) & 0xff))
			bits += group.red.symbolDepth(int((argb >> 16) & 0xff))
			bits += group.blue.symbolDepth(int(argb & 0xff))
			bits += group.alpha.symbolDepth(int((argb >> 24) & 0xff))
		case elosslessTokCache:
			bits += group.green.symbolDepth(elosslessNumLiteralCodes + elosslessNumLengthCodes + int(token.key()))
		case elosslessTokCopy:
			lengthPrefix, err := elosslessPrefixEncode(int(token.length()))
			if err != nil {
				return 0, err
			}
			bits += group.green.symbolDepth(elosslessNumLiteralCodes+lengthPrefix.symbol) + lengthPrefix.extraBits
			planeCode := elosslessDistanceToPlaneCode(width, int(token.distance()))
			distPrefix, err := elosslessPrefixEncode(planeCode)
			if err != nil {
				return 0, err
			}
			bits += group.dist.symbolDepth(distPrefix.symbol) + distPrefix.extraBits
		}
		pos += elosslessTokenLen(token)
	}
	return bits, nil
}

func elosslessWriteImageStreamFromTokens(bw *bitWriter, width, height int, tokens []elosslessToken, emitMetaHuffmanFlag bool, entropySearchLevel uint8, colorCacheBits int) error {
	histograms, err := elosslessBuildHistograms(tokens, width, colorCacheBits)
	if err != nil {
		return err
	}
	group, err := elosslessBuildGroupCodes(&histograms)
	if err != nil {
		return err
	}

	var metaCandidates [][2]int
	if emitMetaHuffmanFlag {
		metaCandidates = elosslessMetaHuffmanCandidates(entropySearchLevel, width, height)
	}
	if len(metaCandidates) != 0 {
		singleSize, err := elosslessEstimateSingleGroupImageStreamSize(width, tokens, colorCacheBits, true, &group)
		if err != nil {
			return err
		}
		var bestMeta *elosslessMetaHuffmanPlan
		bestMetaSize := elosslessIntMax
		levels, err := elosslessTileHistogramLevels(width, height, tokens, colorCacheBits, metaCandidates)
		if err != nil {
			return err
		}
		defer func() {
			for _, level := range levels {
				level.release()
			}
		}()
		for _, hc := range metaCandidates {
			huffmanBits := hc[0]
			groupCount := hc[1]
			plan, err := elosslessBuildMetaHuffmanPlan(colorCacheBits, groupCount, levels[huffmanBits])
			if err != nil {
				return err
			}
			if plan != nil {
				size, err := elosslessEstimateMetaHuffmanImageStreamSize(width, tokens, colorCacheBits, plan)
				if err != nil {
					return err
				}
				if size < bestMetaSize {
					bestMetaSize = size
					bestMeta = plan
				}
			}
		}
		if bestMeta != nil {
			if bestMetaSize < singleSize {
				return elosslessWriteMetaHuffmanImageStream(bw, width, tokens, colorCacheBits, bestMeta)
			}
		}
	}

	return elosslessWriteSingleGroupImageStream(bw, width, tokens, emitMetaHuffmanFlag, colorCacheBits, &group)
}

func elosslessWriteImageStream(bw *bitWriter, width int, argb []uint32, emitMetaHuffmanFlag bool, entropySearchLevel uint8, options elosslessTokenBuildOptions) error {
	tokens, err := elosslessBuildTokens(width, argb, options)
	if err != nil {
		return err
	}
	return elosslessWriteImageStreamFromTokens(bw, width, len(argb)/width, tokens, emitMetaHuffmanFlag, entropySearchLevel, options.colorCacheBits)
}

// elosslessEstimateSingleGroupImageStreamSize returns the byte size of the
// single-group image stream. The prefix (color-cache flags and Huffman trees) is
// emitted so its exact bit length is known, and the token bits are counted by
// summing code depths rather than emitting the whole stream.
func elosslessEstimateSingleGroupImageStreamSize(width int, tokens []elosslessToken, colorCacheBits int, allowMetaHuffman bool, group *elosslessHuffmanGroupCodes) (int, error) {
	bw := newBitWriter()
	if err := elosslessWriteSingleGroupPrefix(bw, allowMetaHuffman, colorCacheBits, group); err != nil {
		return 0, err
	}
	tokenBits, err := elosslessCountSingleGroupTokenBits(tokens, width, group)
	if err != nil {
		return 0, err
	}
	return (bw.bitPos + tokenBits + 7) / 8, nil
}

func elosslessEstimateMetaHuffmanImageStreamSize(width int, tokens []elosslessToken, colorCacheBits int, plan *elosslessMetaHuffmanPlan) (int, error) {
	bw := newBitWriter()
	if err := elosslessWriteMetaHuffmanPrefix(bw, colorCacheBits, plan); err != nil {
		return 0, err
	}
	tokenBits, err := elosslessCountMetaTokenBits(tokens, width, plan)
	if err != nil {
		return 0, err
	}
	return (bw.bitPos + tokenBits + 7) / 8, nil
}

// elosslessEstimateSingleGroupSizeForTokens builds the one-group Huffman codes
// for tokens and returns the byte size of the image stream they would produce,
// counting the token bits instead of emitting them.
func elosslessEstimateSingleGroupSizeForTokens(width int, tokens []elosslessToken, colorCacheBits int) (int, error) {
	histograms, err := elosslessBuildHistograms(tokens, width, colorCacheBits)
	if err != nil {
		return 0, err
	}
	group, err := elosslessBuildGroupCodes(&histograms)
	if err != nil {
		return 0, err
	}
	return elosslessEstimateSingleGroupImageStreamSize(width, tokens, colorCacheBits, false, &group)
}

func elosslessEstimateCacheCandidateCost(width int, tokens []elosslessToken, colorCacheBits int) (int, error) {
	histograms, err := elosslessBuildHistograms(tokens, width, colorCacheBits)
	if err != nil {
		return 0, err
	}
	return elosslessHistogramSetCost(&histograms)
}

func elosslessHistogramSetCost(histograms *elosslessHistogramSet) (int, error) {
	group, err := elosslessBuildGroupCodes(histograms)
	if err != nil {
		return 0, err
	}
	return elosslessHistogramCost(histograms, &group), nil
}

// elosslessCachedTokenHistograms builds the histograms that applying a color
// cache of colorCacheBits to tokens would produce, without materializing the
// rewritten token stream. Ranking cache-bit candidates only needs the
// histograms, and the token slices are large enough that allocating one per
// candidate dominates the search.
func elosslessCachedTokenHistograms(argb []uint32, tokens []elosslessToken, width, colorCacheBits int) (elosslessHistogramSet, error) {
	histograms := elosslessNewHistograms(colorCacheBits)
	cache, err := elosslessColorCacheNew(colorCacheBits)
	if err != nil {
		return histograms, err
	}

	pixelIndex := 0
	for _, token := range tokens {
		switch token.kind {
		case elosslessTokLiteral:
			pixel := token.argb()
			if key, ok := cache.lookup(pixel); ok {
				token = elosslessCacheToken(uint16(key))
			} else {
				cache.insert(pixel)
			}
			pixelIndex++
		case elosslessTokCache:
			pixelIndex++
		case elosslessTokCopy:
			for _, pixel := range argb[pixelIndex : pixelIndex+int(token.length())] {
				cache.insert(pixel)
			}
			pixelIndex += int(token.length())
		}
		if err := elosslessAddTokenToHistograms(&histograms, width, token); err != nil {
			return histograms, err
		}
	}

	elosslessNormalizeHistograms(&histograms)
	return histograms, nil
}

func elosslessSelectBestColorCacheBits(width, height int, argb []uint32, baseTokens []elosslessToken, profile *elosslessLosslessSearchProfile) (int, error) {
	maxCacheBits := elosslessSuggestedMaxColorCacheBits(argb, elosslessMaxColorCacheBitsForProfile(profile))
	shortlistSize := elosslessShortlistColorCacheCandidatesForProfile(profile)

	type cacheCandidate struct {
		cost int
		bits int
	}
	cheapCandidates := make([]cacheCandidate, 0, maxCacheBits+1)
	cost0, err := elosslessEstimateCacheCandidateCost(width, baseTokens, 0)
	if err != nil {
		return 0, err
	}
	cheapCandidates = append(cheapCandidates, cacheCandidate{cost0, 0})
	for cacheBits := 1; cacheBits <= maxCacheBits; cacheBits++ {
		histograms, err := elosslessCachedTokenHistograms(argb, baseTokens, width, cacheBits)
		if err != nil {
			return 0, err
		}
		cost, err := elosslessHistogramSetCost(&histograms)
		if err != nil {
			return 0, err
		}
		cheapCandidates = append(cheapCandidates, cacheCandidate{cost, cacheBits})
	}
	cheapCosts := make(map[int]int, len(cheapCandidates))
	for _, c := range cheapCandidates {
		cheapCosts[c.bits] = c.cost
	}

	sort.SliceStable(cheapCandidates, func(i, j int) bool {
		if cheapCandidates[i].cost != cheapCandidates[j].cost {
			return cheapCandidates[i].cost < cheapCandidates[j].cost
		}
		return cheapCandidates[i].bits < cheapCandidates[j].bits
	})
	keep := shortlistSize
	if keep < 1 {
		keep = 1
	}
	if len(cheapCandidates) > keep {
		cheapCandidates = cheapCandidates[:keep]
	}
	shortlist := make([]int, 0, len(cheapCandidates)+1)
	containsZero := false
	for _, c := range cheapCandidates {
		shortlist = append(shortlist, c.bits)
		if c.bits == 0 {
			containsZero = true
		}
	}
	if !containsZero {
		shortlist = append(shortlist, 0)
	}

	bestCacheBits := 0
	bestSize := elosslessIntMax
	var scratch []elosslessToken
	for _, cacheBits := range shortlist {
		cheapCost, ok := cheapCosts[cacheBits]
		if !ok {
			cheapCost, err = elosslessEstimateCacheCandidateCost(width, baseTokens, 0)
			if err != nil {
				return 0, err
			}
		}
		if bestSize != elosslessIntMax && elosslessShouldStopTransformSearch(bestSize, elosslessDivCeil(cheapCost, 8), profile) {
			break
		}
		var size int
		if cacheBits == 0 {
			size, err = elosslessEstimateSingleGroupSizeForTokens(width, baseTokens, 0)
			if err != nil {
				return 0, err
			}
		} else {
			if scratch == nil {
				scratch = make([]elosslessToken, len(baseTokens))
			}
			if err := elosslessApplyColorCacheToTokens(scratch, argb, baseTokens, cacheBits); err != nil {
				return 0, err
			}
			size, err = elosslessEstimateSingleGroupSizeForTokens(width, scratch, cacheBits)
			if err != nil {
				return 0, err
			}
		}
		if size < bestSize {
			bestSize = size
			bestCacheBits = cacheBits
		}
	}

	return bestCacheBits, nil
}

func elosslessBoolBit(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}
