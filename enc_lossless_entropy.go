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
	symbols := code.usedSymbols()
	if len(symbols) == 0 {
		return encBitstream("empty Huffman tree")
	}
	allSmall := true
	for _, symbol := range symbols {
		if symbol >= (1 << 8) {
			allSmall = false
			break
		}
	}
	if len(symbols) <= 2 && allSmall {
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

func elosslessNewHistograms(colorCacheBits int) elosslessHistogramSet {
	cacheEntries := 0
	if colorCacheBits > 0 {
		cacheEntries = 1 << colorCacheBits
	}
	return elosslessHistogramSet{
		make([]uint32, elosslessNumLiteralCodes+elosslessNumLengthCodes+cacheEntries),
		make([]uint32, elosslessNumLiteralCodes),
		make([]uint32, elosslessNumLiteralCodes),
		make([]uint32, elosslessNumLiteralCodes),
		make([]uint32, elosslessNumDistanceCodes),
	}
}

func elosslessAddTokenToHistograms(histograms *elosslessHistogramSet, width int, token elosslessToken) error {
	switch token.kind {
	case elosslessTokLiteral:
		argb := token.argb
		histograms[0][(argb>>8)&0xff]++
		histograms[1][(argb>>16)&0xff]++
		histograms[2][argb&0xff]++
		histograms[3][(argb>>24)&0xff]++
	case elosslessTokCache:
		histograms[0][elosslessNumLiteralCodes+elosslessNumLengthCodes+token.key]++
	case elosslessTokCopy:
		lengthPrefix, err := elosslessPrefixEncode(token.length)
		if err != nil {
			return err
		}
		histograms[0][elosslessNumLiteralCodes+lengthPrefix.symbol]++

		planeCode := elosslessDistanceToPlaneCode(width, token.distance)
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

func elosslessHistogramEntropyCost(histogram []uint32) float64 {
	total := 0.0
	for _, count := range histogram {
		total += float64(count)
	}
	if total == 0.0 {
		return 0.0
	}

	sum := 0.0
	for _, count := range histogram {
		if count != 0 {
			c := float64(count)
			sum += c * math.Log2(total/c)
		}
	}
	return sum
}

func elosslessHistogramSignatureCosts(histograms *elosslessHistogramSet) [3]float64 {
	return [3]float64{
		elosslessHistogramEntropyCost(histograms[0]),
		elosslessHistogramEntropyCost(histograms[1]),
		elosslessHistogramEntropyCost(histograms[2]),
	}
}

func elosslessHistogramSetEntropyCost(histograms *elosslessHistogramSet) float64 {
	sum := 0.0
	for i := 0; i < 5; i++ {
		sum += elosslessHistogramEntropyCost(histograms[i])
	}
	return sum
}

// elosslessCombinedChannelEntropy returns the Shannon entropy cost of the
// element-wise sum of two histograms without materializing the sum.
func elosslessCombinedChannelEntropy(a, b []uint32) float64 {
	total := 0.0
	for i := range a {
		total += float64(a[i] + b[i])
	}
	if total == 0.0 {
		return 0.0
	}
	sum := 0.0
	for i := range a {
		c := a[i] + b[i]
		if c != 0 {
			cf := float64(c)
			sum += cf * math.Log2(total/cf)
		}
	}
	return sum
}

func elosslessCombinedHistogramSetEntropy(a, b *elosslessHistogramSet) float64 {
	return elosslessCombinedChannelEntropy(a[0], b[0]) +
		elosslessCombinedChannelEntropy(a[1], b[1]) +
		elosslessCombinedChannelEntropy(a[2], b[2]) +
		elosslessCombinedChannelEntropy(a[3], b[3]) +
		elosslessCombinedChannelEntropy(a[4], b[4])
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
	selfCost := make([]float64, n)
	comb := make([][]float64, n)
	for i := 0; i < n; i++ {
		selfCost[i] = elosslessHistogramSetEntropyCost(&candidates[i].histograms)
		comb[i] = make([]float64, n)
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			comb[i][j] = elosslessCombinedHistogramSetEntropy(&candidates[i].histograms, &candidates[j].histograms)
		}
	}
	recomputeRow := func(i, active int) {
		for j := 0; j < active; j++ {
			if j == i {
				continue
			}
			v := elosslessCombinedHistogramSetEntropy(&candidates[i].histograms, &candidates[j].histograms)
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
			for rhs := lhs + 1; rhs < n; rhs++ {
				penalty := comb[lhs][rhs] - selfCost[lhs] - selfCost[rhs]
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
		candidates[bestLhs].weight += rhsCandidate.weight
		selfCost[bestLhs] = elosslessHistogramSetEntropyCost(&candidates[bestLhs].histograms)

		last := n - 1
		candidates[bestRhs] = candidates[last]
		selfCost[bestRhs] = selfCost[last]
		candidates = candidates[:last]
		n = last
		if bestRhs < n {
			recomputeRow(bestRhs, n)
		}
		recomputeRow(bestLhs, n)
	}

	return candidates
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

func elosslessAssignTilesToGroups(nonEmptyTiles [][2]int, tileHistograms []elosslessHistogramSet, groupCodes []elosslessHuffmanGroupCodes, assignments []int) {
	for _, t := range nonEmptyTiles {
		tile := t[0]
		bestGroup := 0
		bestCost := elosslessIntMax
		for groupIndex := range groupCodes {
			cost := elosslessHistogramCost(&tileHistograms[tile], &groupCodes[groupIndex])
			if cost < bestCost {
				bestCost = cost
				bestGroup = groupIndex
			}
		}
		assignments[tile] = bestGroup
	}
}

func elosslessRefineMetaHuffmanPlan(tileCount, colorCacheBits int, nonEmptyTiles [][2]int, tileHistograms []elosslessHistogramSet, seedHistograms []elosslessHistogramSet) (*elosslessMetaHuffmanPlan, error) {
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
		elosslessAssignTilesToGroups(nonEmptyTiles, tileHistograms, groupCodes, assignments)

		remap := make([]int, len(groupCodes))
		for i := range remap {
			remap[i] = elosslessIntMax
		}
		var mergedHistograms []elosslessHistogramSet
		for groupIndex := range groupCodes {
			histograms := elosslessNewHistograms(colorCacheBits)
			used := false
			for _, t := range nonEmptyTiles {
				tile := t[0]
				if assignments[tile] == groupIndex {
					elosslessMergeHistograms(&histograms, &tileHistograms[tile])
					used = true
				}
			}
			if used {
				elosslessNormalizeHistograms(&histograms)
				remap[groupIndex] = len(mergedHistograms)
				mergedHistograms = append(mergedHistograms, histograms)
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

func elosslessMetaHuffmanAssignmentCost(nonEmptyTiles [][2]int, tileHistograms []elosslessHistogramSet, plan *elosslessMetaHuffmanPlan) int {
	total := 0
	for _, t := range nonEmptyTiles {
		tile := t[0]
		total += elosslessHistogramCost(&tileHistograms[tile], &plan.groups[plan.assignments[tile]])
	}
	return total
}

func elosslessApplyColorCacheToTokens(argb []uint32, tokens []elosslessToken, colorCacheBits int) ([]elosslessToken, error) {
	if colorCacheBits == 0 {
		out := make([]elosslessToken, len(tokens))
		copy(out, tokens)
		return out, nil
	}

	cache, err := elosslessColorCacheNew(colorCacheBits)
	if err != nil {
		return nil, err
	}
	cachedTokens := make([]elosslessToken, 0, len(tokens))
	pixelIndex := 0

	for _, token := range tokens {
		switch token.kind {
		case elosslessTokLiteral:
			pixel := token.argb
			if key, ok := cache.lookup(pixel); ok {
				cachedTokens = append(cachedTokens, elosslessToken{kind: elosslessTokCache, key: key})
			} else {
				cachedTokens = append(cachedTokens, elosslessToken{kind: elosslessTokLiteral, argb: pixel})
				cache.insert(pixel)
			}
			pixelIndex++
		case elosslessTokCache:
			cachedTokens = append(cachedTokens, elosslessToken{kind: elosslessTokCache, key: token.key})
			pixelIndex++
		case elosslessTokCopy:
			cachedTokens = append(cachedTokens, elosslessToken{kind: elosslessTokCopy, distance: token.distance, length: token.length})
			for _, pixel := range argb[pixelIndex : pixelIndex+token.length] {
				cache.insert(pixel)
			}
			pixelIndex += token.length
		}
	}

	return cachedTokens, nil
}

func elosslessBuildMetaHuffmanPlan(width, height int, tokens []elosslessToken, colorCacheBits, huffmanBits, maxGroups int) (*elosslessMetaHuffmanPlan, error) {
	if huffmanBits < elosslessMinHuffmanBits || huffmanBits >= elosslessMinHuffmanBits+(1<<elosslessNumHuffmanBits) {
		return nil, nil
	}

	huffmanXsize := elosslessSubsampleSize(width, huffmanBits)
	huffmanYsize := elosslessSubsampleSize(height, huffmanBits)
	tileCount := huffmanXsize * huffmanYsize
	if tileCount <= 1 {
		return nil, nil
	}

	tileHistograms := make([]elosslessHistogramSet, tileCount)
	for i := range tileHistograms {
		tileHistograms[i] = elosslessNewHistograms(colorCacheBits)
	}
	tileWeights := make([]int, tileCount)
	pos := 0
	for _, token := range tokens {
		tile := elosslessTileIndexForPos(width, huffmanBits, huffmanXsize, pos)
		if err := elosslessAddTokenToHistograms(&tileHistograms[tile], width, token); err != nil {
			return nil, err
		}
		tileWeights[tile] += elosslessTokenLen(token)
		pos += elosslessTokenLen(token)
	}

	var nonEmptyTiles [][2]int
	for index, weight := range tileWeights {
		if weight != 0 {
			nonEmptyTiles = append(nonEmptyTiles, [2]int{index, weight})
		}
	}
	if len(nonEmptyTiles) <= 1 {
		return nil, nil
	}
	sort.SliceStable(nonEmptyTiles, func(i, j int) bool {
		return nonEmptyTiles[j][1] < nonEmptyTiles[i][1]
	})

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
		plan, err := elosslessRefineMetaHuffmanPlan(tileCount, colorCacheBits, nonEmptyTiles, tileHistograms, seedHistograms)
		if err != nil {
			return nil, err
		}
		if plan != nil {
			plan.huffmanBits = huffmanBits
			plan.huffmanXsize = huffmanXsize
			cost := elosslessMetaHuffmanAssignmentCost(nonEmptyTiles, tileHistograms, plan)
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
			argb := token.argb
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
			if err := group.green.writeSymbol(bw, elosslessNumLiteralCodes+elosslessNumLengthCodes+token.key); err != nil {
				return err
			}
		case elosslessTokCopy:
			lengthPrefix, err := elosslessPrefixEncode(token.length)
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

			planeCode := elosslessDistanceToPlaneCode(width, token.distance)
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
			argb := token.argb
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
			if err := greenCodes.writeSymbol(bw, elosslessNumLiteralCodes+elosslessNumLengthCodes+token.key); err != nil {
				return err
			}
		case elosslessTokCopy:
			lengthPrefix, err := elosslessPrefixEncode(token.length)
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

			planeCode := elosslessDistanceToPlaneCode(width, token.distance)
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

func elosslessWriteSingleGroupImageStream(bw *bitWriter, width int, tokens []elosslessToken, allowMetaHuffman bool, colorCacheBits int, group *elosslessHuffmanGroupCodes) error {
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

	if err := elosslessWriteHuffmanGroup(bw, group); err != nil {
		return err
	}

	return elosslessWriteTokens(bw, tokens, width, &group.green, &group.red, &group.blue, &group.alpha, &group.dist)
}

func elosslessWriteMetaHuffmanImageStream(bw *bitWriter, width int, tokens []elosslessToken, colorCacheBits int, plan *elosslessMetaHuffmanPlan) error {
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
	return elosslessWriteTokensWithMeta(bw, tokens, width, plan)
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
		for _, hc := range metaCandidates {
			huffmanBits := hc[0]
			groupCount := hc[1]
			plan, err := elosslessBuildMetaHuffmanPlan(width, height, tokens, colorCacheBits, huffmanBits, groupCount)
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

func elosslessEstimateSingleGroupImageStreamSize(width int, tokens []elosslessToken, colorCacheBits int, allowMetaHuffman bool, group *elosslessHuffmanGroupCodes) (int, error) {
	bw := newBitWriter()
	if err := elosslessWriteSingleGroupImageStream(bw, width, tokens, allowMetaHuffman, colorCacheBits, group); err != nil {
		return 0, err
	}
	return len(bw.intoBytes()), nil
}

func elosslessEstimateMetaHuffmanImageStreamSize(width int, tokens []elosslessToken, colorCacheBits int, plan *elosslessMetaHuffmanPlan) (int, error) {
	bw := newBitWriter()
	if err := elosslessWriteMetaHuffmanImageStream(bw, width, tokens, colorCacheBits, plan); err != nil {
		return 0, err
	}
	return len(bw.intoBytes()), nil
}

func elosslessEstimateImageStreamSize(width, height int, tokens []elosslessToken, colorCacheBits int, emitMetaHuffmanFlag bool, entropySearchLevel uint8) (int, error) {
	bw := newBitWriter()
	if err := elosslessWriteImageStreamFromTokens(bw, width, height, tokens, emitMetaHuffmanFlag, entropySearchLevel, colorCacheBits); err != nil {
		return 0, err
	}
	return len(bw.intoBytes()), nil
}

func elosslessEstimateCacheCandidateCost(width int, tokens []elosslessToken, colorCacheBits int) (int, error) {
	histograms, err := elosslessBuildHistograms(tokens, width, colorCacheBits)
	if err != nil {
		return 0, err
	}
	group, err := elosslessBuildGroupCodes(&histograms)
	if err != nil {
		return 0, err
	}
	return elosslessHistogramCost(&histograms, &group), nil
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
		tokens, err := elosslessApplyColorCacheToTokens(argb, baseTokens, cacheBits)
		if err != nil {
			return 0, err
		}
		cost, err := elosslessEstimateCacheCandidateCost(width, tokens, cacheBits)
		if err != nil {
			return 0, err
		}
		cheapCandidates = append(cheapCandidates, cacheCandidate{cost, cacheBits})
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
	for _, cacheBits := range shortlist {
		var cheapCost int
		if cacheBits == 0 {
			cheapCost, err = elosslessEstimateCacheCandidateCost(width, baseTokens, 0)
			if err != nil {
				return 0, err
			}
		} else {
			tokens, err := elosslessApplyColorCacheToTokens(argb, baseTokens, cacheBits)
			if err != nil {
				return 0, err
			}
			cheapCost, err = elosslessEstimateCacheCandidateCost(width, tokens, cacheBits)
			if err != nil {
				return 0, err
			}
		}
		if bestSize != elosslessIntMax && elosslessShouldStopTransformSearch(bestSize, elosslessDivCeil(cheapCost, 8), profile) {
			break
		}
		var size int
		if cacheBits == 0 {
			size, err = elosslessEstimateImageStreamSize(width, height, baseTokens, 0, false, 0)
			if err != nil {
				return 0, err
			}
		} else {
			tokens, err := elosslessApplyColorCacheToTokens(argb, baseTokens, cacheBits)
			if err != nil {
				return 0, err
			}
			size, err = elosslessEstimateImageStreamSize(width, height, tokens, cacheBits, false, 0)
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
