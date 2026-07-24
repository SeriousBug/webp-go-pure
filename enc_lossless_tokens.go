package webp

// Backward references, cache selection, and tokenization for lossless encoding.
// Ported from src/encoder/lossless/tokens.rs.

import (
	"container/heap"
	"math/bits"
	"sort"
)

type elosslessMatch struct {
	distance int
	length   int
	set      bool
}

type elosslessOptKey struct {
	key int
	ok  bool
}

func elosslessFindMatchLength(argb []uint32, first, second, maxLen int) int {
	length := 0
	for length < maxLen && argb[first+length] == argb[second+length] {
		length++
	}
	return length
}

func elosslessTokenBuildOptionsFor(matchSearchLevel uint8, colorCacheBits int) elosslessTokenBuildOptions {
	var matchChainDepth, windowOffsetLimit int
	var useWindowOffsets, lazyMatching bool
	switch matchSearchLevel {
	case 0:
		matchChainDepth, useWindowOffsets, windowOffsetLimit, lazyMatching = 0, false, 0, false
	case 1:
		matchChainDepth, useWindowOffsets, windowOffsetLimit, lazyMatching = elosslessMatchChainDepthLevel1, false, 0, false
	case 2:
		matchChainDepth, useWindowOffsets, windowOffsetLimit, lazyMatching = elosslessMatchChainDepthLevel2, true, 16, false
	case 3:
		matchChainDepth, useWindowOffsets, windowOffsetLimit, lazyMatching = elosslessMatchChainDepthLevel3, true, 32, false
	case 4:
		matchChainDepth, useWindowOffsets, windowOffsetLimit, lazyMatching = elosslessMatchChainDepthLevel4, true, 64, true
	case 5:
		matchChainDepth, useWindowOffsets, windowOffsetLimit, lazyMatching = elosslessMatchChainDepthLevel5, true, 96, true
	case 6:
		matchChainDepth, useWindowOffsets, windowOffsetLimit, lazyMatching = elosslessMatchChainDepthLevel6, true, 128, true
	default:
		matchChainDepth, useWindowOffsets, windowOffsetLimit, lazyMatching = elosslessMatchChainDepthLevel7, true, 160, true
	}
	var useTraceback bool
	var tracebackMaxCandidates int
	switch {
	case matchSearchLevel <= 4:
		useTraceback, tracebackMaxCandidates = false, 0
	case matchSearchLevel == 5:
		useTraceback, tracebackMaxCandidates = true, 4
	case matchSearchLevel == 6:
		useTraceback, tracebackMaxCandidates = true, 6
	default:
		useTraceback, tracebackMaxCandidates = true, 8
	}
	return elosslessTokenBuildOptions{
		colorCacheBits:         colorCacheBits,
		matchChainDepth:        matchChainDepth,
		useWindowOffsets:       useWindowOffsets,
		windowOffsetLimit:      windowOffsetLimit,
		lazyMatching:           lazyMatching,
		useTraceback:           useTraceback,
		tracebackMaxCandidates: tracebackMaxCandidates,
	}
}

func elosslessMaxColorCacheBitsForProfile(profile *elosslessLosslessSearchProfile) int {
	if !profile.useColorCache {
		return 0
	}
	switch profile.entropySearchLevel {
	case 0:
		return 0
	case 1:
		return 7
	case 2:
		return 8
	case 3:
		return 9
	case 4:
		return 10
	default:
		return elosslessMaxCacheBits
	}
}

func elosslessShortlistColorCacheCandidatesForProfile(profile *elosslessLosslessSearchProfile) int {
	switch profile.entropySearchLevel {
	case 0, 1:
		return 1
	case 2, 3:
		return 2
	default:
		return 3
	}
}

func elosslessMetaHuffmanCandidates(entropySearchLevel uint8, width, height int) [][2]int {
	switch entropySearchLevel {
	case 0:
		return nil
	case 1:
		return [][2]int{{5, 4}}
	case 2:
		return [][2]int{{6, 2}, {5, 4}}
	case 3:
		return [][2]int{{6, 2}, {5, 4}, {4, 4}}
	case 4:
		if width*height >= 512*512 {
			return [][2]int{{6, 2}, {5, 4}, {5, 6}, {4, 4}}
		}
		return [][2]int{{6, 2}, {5, 4}, {5, 6}, {4, 4}, {4, 6}}
	case 5:
		if width*height >= 512*512 {
			return [][2]int{{6, 2}, {5, 4}, {5, 6}, {4, 4}, {4, 6}}
		}
		return [][2]int{{6, 2}, {5, 4}, {5, 6}, {4, 4}, {4, 6}, {4, 8}}
	default:
		if width*height >= 512*512 {
			return [][2]int{{6, 2}, {5, 4}, {5, 6}, {4, 4}, {4, 6}, {4, 8}}
		}
		return [][2]int{{6, 2}, {5, 4}, {5, 6}, {4, 4}, {4, 6}, {4, 8}, {3, 8}}
	}
}

func elosslessSuggestedMaxColorCacheBits(argb []uint32, maxCacheBits int) int {
	if maxCacheBits == 0 {
		return 0
	}

	uniqueLimit := 1 << maxCacheBits
	unique := make(map[uint32]struct{})
	for _, pixel := range argb {
		unique[pixel] = struct{}{}
		if len(unique) > uniqueLimit {
			return maxCacheBits
		}
	}

	if len(unique) <= 1 {
		return 0
	}
	bitsCount := 0
	capacity := 1
	for capacity < len(unique) && bitsCount < maxCacheBits {
		bitsCount++
		capacity <<= 1
	}
	if bitsCount > maxCacheBits {
		return maxCacheBits
	}
	return bitsCount
}

func elosslessBuildWindowOffsets(width, maxPlaneCodes int) []int {
	if maxPlaneCodes == 0 {
		return nil
	}
	radius := 6
	if maxPlaneCodes > 32 {
		radius = 12
	}
	byPlaneCode := make([]int, maxPlaneCodes)
	for y := 0; y <= radius; y++ {
		for x := -radius; x <= radius; x++ {
			offset := y*width + x
			if offset <= 0 {
				continue
			}
			planeCode := elosslessDistanceToPlaneCode(width, offset)
			if planeCode >= 1 {
				planeCode--
			} else {
				planeCode = 0
			}
			if planeCode < maxPlaneCodes && byPlaneCode[planeCode] == 0 {
				byPlaneCode[planeCode] = offset
			}
		}
	}
	var out []int
	for _, offset := range byPlaneCode {
		if offset != 0 {
			out = append(out, offset)
		}
	}
	return out
}

func elosslessMinMatchLengthForDistance(width, distance int) int {
	if distance == 1 || distance == width {
		return elosslessMinLength
	}
	planeCode := elosslessDistanceToPlaneCode(width, distance)
	if planeCode <= 32 {
		return elosslessMinLength
	} else if planeCode <= 80 {
		return elosslessMinLength + 1
	} else if planeCode <= 512 {
		return elosslessMinLength + 2
	}
	return elosslessMinLength + 3
}

func elosslessPrefixExtraBitCount(value int) int {
	if value <= 4 {
		return 0
	}
	value = value - 1
	highestBit := elosslessIlog2(value)
	return highestBit - 1
}

func elosslessCopyCostBits(width, distance, length int) int {
	planeCode := elosslessDistanceToPlaneCode(width, distance)
	return elosslessApproxCopyLengthSymbolBits +
		elosslessPrefixExtraBitCount(length) +
		elosslessApproxCopyDistanceSymbolBits +
		elosslessPrefixExtraBitCount(planeCode)
}

func elosslessMatchGainBits(width, distance, length int) int {
	return elosslessApproxLiteralCostBits*length - elosslessCopyCostBits(width, distance, length)
}

func elosslessConsiderMatch(width int, best *elosslessMatch, distance, length int) {
	if length < elosslessMinMatchLengthForDistance(width, distance) {
		return
	}

	candidateScore := elosslessMatchGainBits(width, distance, length)
	better := true
	if best.set {
		bestScore := elosslessMatchGainBits(width, best.distance, best.length)
		better = candidateScore > bestScore ||
			(candidateScore == bestScore &&
				(length > best.length ||
					(length == best.length && distance < best.distance)))
	}
	if better {
		*best = elosslessMatch{distance: distance, length: length, set: true}
	}
}

type elosslessPreview struct {
	hash    int
	oldPrev int
	oldHead int
	valid   bool
}

func elosslessPreviewUpdateMatchChain(argb []uint32, index int, heads, prev []int) elosslessPreview {
	if index+elosslessMinLength > len(argb) {
		return elosslessPreview{}
	}
	hash := elosslessHashMatchPixels(argb, index)
	oldPrev := prev[index]
	oldHead := heads[hash]
	elosslessUpdateMatchChain(argb, index, heads, prev)
	return elosslessPreview{hash: hash, oldPrev: oldPrev, oldHead: oldHead, valid: true}
}

func elosslessRestorePreviewedMatchChain(index int, preview elosslessPreview, heads, prev []int) {
	if preview.valid {
		prev[index] = preview.oldPrev
		heads[preview.hash] = preview.oldHead
	}
}

func elosslessHashMatchPixels(argb []uint32, index int) int {
	a := argb[index]
	b := bits.RotateLeft32(argb[index+1], 7)
	c := bits.RotateLeft32(argb[index+2], 13)
	d := bits.RotateLeft32(argb[index+3], 21)
	hash := a ^ b ^ c ^ (d * elosslessColorCacheHashMul)
	return int((hash * elosslessColorCacheHashMul) >> (32 - elosslessMatchHashBits))
}

func elosslessUpdateMatchChain(argb []uint32, index int, heads, prev []int) {
	if index+elosslessMinLength > len(argb) {
		return
	}
	hash := elosslessHashMatchPixels(argb, index)
	prev[index] = heads[hash]
	heads[hash] = index
}

func elosslessFindBestHashMatch(width int, argb []uint32, index, maxLen int, heads, prev []int, matchChainDepth int) elosslessMatch {
	var best elosslessMatch
	if matchChainDepth == 0 || maxLen < elosslessMinLength || index+elosslessMinLength > len(argb) {
		return best
	}

	hash := elosslessHashMatchPixels(argb, index)
	candidate := heads[hash]
	remaining := matchChainDepth

	for candidate != elosslessIntMax && remaining > 0 {
		remaining--
		if candidate >= index {
			break
		}
		distance := index - candidate
		if distance <= elosslessMaxFallbackDistance {
			length := elosslessFindMatchLength(argb, index, candidate, maxLen)
			if length >= elosslessMinLength {
				elosslessConsiderMatch(width, &best, distance, length)
			}
			if length == maxLen {
				break
			}
		}
		candidate = prev[candidate]
	}

	return best
}

func elosslessFindBestWindowOffsetMatch(width int, argb []uint32, index, maxLen int, windowOffsets []int) elosslessMatch {
	var best elosslessMatch
	for _, distance := range windowOffsets {
		if distance > index || distance > elosslessMaxFallbackDistance {
			continue
		}
		candidateIndex := index - distance
		length := elosslessFindMatchLength(argb, index, candidateIndex, maxLen)
		elosslessConsiderMatch(width, &best, distance, length)
	}
	return best
}

func elosslessSinglePixelCostBits(cacheHit bool) int {
	if cacheHit {
		return elosslessApproxCacheCostBits
	}
	return elosslessApproxLiteralCostBits
}

func elosslessFindBestMatch(width int, argb []uint32, index int, options elosslessTokenBuildOptions, heads, prev, windowOffsets []int) elosslessMatch {
	maxLen := len(argb) - index
	if elosslessMaxLength < maxLen {
		maxLen = elosslessMaxLength
	}
	var best elosslessMatch

	if index > 0 {
		rleLen := elosslessFindMatchLength(argb, index, index-1, maxLen)
		elosslessConsiderMatch(width, &best, 1, rleLen)
	}
	if index >= width {
		prevRowLen := elosslessFindMatchLength(argb, index, index-width, maxLen)
		elosslessConsiderMatch(width, &best, width, prevRowLen)
	}
	if options.useWindowOffsets {
		m := elosslessFindBestWindowOffsetMatch(width, argb, index, maxLen, windowOffsets)
		if m.set {
			elosslessConsiderMatch(width, &best, m.distance, m.length)
		}
	}
	m := elosslessFindBestHashMatch(width, argb, index, maxLen, heads, prev, options.matchChainDepth)
	if m.set {
		elosslessConsiderMatch(width, &best, m.distance, m.length)
	}

	return best
}

func elosslessFillInt(s []int, v int) {
	for i := range s {
		s[i] = v
	}
}

func elosslessBuildTokensGreedy(width int, argb []uint32, options elosslessTokenBuildOptions) ([]elosslessToken, error) {
	if len(argb) == 0 {
		return nil, nil
	}

	tokens := make([]elosslessToken, 0, len(argb))
	var cache *elosslessColorCache
	if options.colorCacheBits > 0 {
		c, err := elosslessColorCacheNew(options.colorCacheBits)
		if err != nil {
			return nil, err
		}
		cache = &c
	}
	heads := make([]int, elosslessMatchHashSize)
	elosslessFillInt(heads, elosslessIntMax)
	prev := make([]int, len(argb))
	elosslessFillInt(prev, elosslessIntMax)
	var windowOffsets []int
	if options.useWindowOffsets {
		windowOffsets = elosslessBuildWindowOffsets(width, options.windowOffsetLimit)
	}

	index := 0
	for index < len(argb) {
		cacheKey := 0
		cacheHit := false
		if cache != nil {
			cacheKey, cacheHit = cache.lookup(argb[index])
		}
		bestMatch := elosslessFindBestMatch(width, argb, index, options, heads, prev, windowOffsets)

		if options.lazyMatching {
			if bestMatch.set {
				distance := bestMatch.distance
				length := bestMatch.length
				if length < 64 && index+1 < len(argb) {
					preview := elosslessPreviewUpdateMatchChain(argb, index, heads, prev)
					nextMatch := elosslessFindBestMatch(width, argb, index+1, options, heads, prev, windowOffsets)
					elosslessRestorePreviewedMatchChain(index, preview, heads, prev)

					currentGain := elosslessMatchGainBits(width, distance, length)
					takeLiteral := false
					if nextMatch.set {
						nextLength := nextMatch.length
						nextGain := elosslessMatchGainBits(width, nextMatch.distance, nextMatch.length) +
							elosslessApproxLiteralCostBits - elosslessSinglePixelCostBits(cacheHit)
						if index+1+nextLength >= index+length && nextGain > currentGain {
							takeLiteral = true
						}
					}
					if takeLiteral {
						bestMatch.set = false
					} else {
						bestMatch = elosslessMatch{distance: distance, length: length, set: true}
					}
				}
			}
		}

		if bestMatch.set {
			distance := bestMatch.distance
			length := bestMatch.length
			tokens = append(tokens, elosslessToken{kind: elosslessTokCopy, distance: distance, length: length})
			if cache != nil {
				for _, pixel := range argb[index : index+length] {
					cache.insert(pixel)
				}
			}
			for position := index; position < index+length; position++ {
				elosslessUpdateMatchChain(argb, position, heads, prev)
			}
			index += length
		} else if cacheHit {
			tokens = append(tokens, elosslessToken{kind: elosslessTokCache, key: cacheKey})
			if cache != nil {
				cache.insert(argb[index])
			}
			elosslessUpdateMatchChain(argb, index, heads, prev)
			index++
		} else {
			tokens = append(tokens, elosslessToken{kind: elosslessTokLiteral, argb: argb[index]})
			if cache != nil {
				cache.insert(argb[index])
			}
			elosslessUpdateMatchChain(argb, index, heads, prev)
			index++
		}
	}

	return tokens, nil
}

func elosslessBuildTracebackCostModel(width int, tokens []elosslessToken, colorCacheBits int) (elosslessTracebackCostModel, error) {
	histograms, err := elosslessBuildHistograms(tokens, width, colorCacheBits)
	if err != nil {
		return elosslessTracebackCostModel{}, err
	}
	group, err := elosslessBuildGroupCodes(&histograms)
	if err != nil {
		return elosslessTracebackCostModel{}, err
	}
	var lengthCostIntervals [][3]int
	start := 1
	prefix, err := elosslessPrefixEncode(1)
	if err != nil {
		return elosslessTracebackCostModel{}, err
	}
	currentCost := int(group.green.getCodeLengths()[elosslessNumLiteralCodes+prefix.symbol]) + prefix.extraBits
	for length := 2; length <= elosslessMaxLength; length++ {
		prefix, err := elosslessPrefixEncode(length)
		if err != nil {
			return elosslessTracebackCostModel{}, err
		}
		cost := int(group.green.getCodeLengths()[elosslessNumLiteralCodes+prefix.symbol]) + prefix.extraBits
		if cost != currentCost {
			lengthCostIntervals = append(lengthCostIntervals, [3]int{start, length, currentCost})
			start = length
			currentCost = cost
		}
	}
	lengthCostIntervals = append(lengthCostIntervals, [3]int{start, elosslessMaxLength + 1, currentCost})

	toIntSlice := func(code *elosslessHuffmanCode) []int {
		lengths := code.getCodeLengths()
		out := make([]int, len(lengths))
		for i, b := range lengths {
			out[i] = int(b)
		}
		return out
	}

	return elosslessTracebackCostModel{
		literal:             toIntSlice(&group.green),
		red:                 toIntSlice(&group.red),
		blue:                toIntSlice(&group.blue),
		alpha:               toIntSlice(&group.alpha),
		distance:            toIntSlice(&group.dist),
		lengthCostIntervals: lengthCostIntervals,
	}, nil
}

func (m *elosslessTracebackCostModel) literalCost(argb uint32) int {
	return m.alpha[(argb>>24)&0xff] +
		m.red[(argb>>16)&0xff] +
		m.literal[(argb>>8)&0xff] +
		m.blue[argb&0xff]
}

func (m *elosslessTracebackCostModel) distanceCost(width, distance int) (int, error) {
	planeCode := elosslessDistanceToPlaneCode(width, distance)
	distPrefix, err := elosslessPrefixEncode(planeCode)
	if err != nil {
		return 0, err
	}
	return m.distance[distPrefix.symbol] + distPrefix.extraBits, nil
}

func (m *elosslessTracebackCostModel) cacheCost(key int) int {
	return m.literal[elosslessNumLiteralCodes+elosslessNumLengthCodes+key]
}

func elosslessPushMatchCandidate(width int, candidates *[][2]int, distance, length int) {
	if length < elosslessMinMatchLengthForDistance(width, distance) {
		return
	}
	for i := range *candidates {
		if (*candidates)[i][0] == distance {
			if length > (*candidates)[i][1] {
				(*candidates)[i][1] = length
			}
			return
		}
	}
	*candidates = append(*candidates, [2]int{distance, length})
}

func elosslessCollectMatchCandidates(width int, argb []uint32, index int, options elosslessTokenBuildOptions, heads, prev, windowOffsets []int) [][2]int {
	maxLen := len(argb) - index
	if elosslessMaxLength < maxLen {
		maxLen = elosslessMaxLength
	}
	var candidates [][2]int

	if index > 0 {
		rleLen := elosslessFindMatchLength(argb, index, index-1, maxLen)
		elosslessPushMatchCandidate(width, &candidates, 1, rleLen)
	}
	if index >= width {
		prevRowLen := elosslessFindMatchLength(argb, index, index-width, maxLen)
		elosslessPushMatchCandidate(width, &candidates, width, prevRowLen)
	}
	if options.useWindowOffsets {
		for _, distance := range windowOffsets {
			if distance > index || distance > elosslessMaxFallbackDistance {
				continue
			}
			length := elosslessFindMatchLength(argb, index, index-distance, maxLen)
			elosslessPushMatchCandidate(width, &candidates, distance, length)
		}
	}
	if options.matchChainDepth > 0 && maxLen >= elosslessMinLength && index+elosslessMinLength <= len(argb) {
		hash := elosslessHashMatchPixels(argb, index)
		candidate := heads[hash]
		remaining := options.matchChainDepth
		for candidate != elosslessIntMax && remaining > 0 {
			remaining--
			if candidate >= index {
				break
			}
			distance := index - candidate
			if distance <= elosslessMaxFallbackDistance {
				length := elosslessFindMatchLength(argb, index, candidate, maxLen)
				elosslessPushMatchCandidate(width, &candidates, distance, length)
				if length == maxLen {
					break
				}
			}
			candidate = prev[candidate]
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		lhsScore := elosslessMatchGainBits(width, candidates[i][0], candidates[i][1])
		rhsScore := elosslessMatchGainBits(width, candidates[j][0], candidates[j][1])
		if lhsScore != rhsScore {
			return rhsScore < lhsScore
		}
		if candidates[i][1] != candidates[j][1] {
			return candidates[j][1] < candidates[i][1]
		}
		return candidates[i][0] < candidates[j][0]
	})
	keep := options.tracebackMaxCandidates
	if keep < 1 {
		keep = 1
	}
	if len(candidates) > keep {
		candidates = candidates[:keep]
	}
	return candidates
}

func elosslessBuildCacheKeys(argb []uint32, colorCacheBits int) ([]elosslessOptKey, error) {
	keys := make([]elosslessOptKey, len(argb))
	if colorCacheBits == 0 {
		return keys, nil
	}

	cache, err := elosslessColorCacheNew(colorCacheBits)
	if err != nil {
		return nil, err
	}
	for i, pixel := range argb {
		key, ok := cache.lookup(pixel)
		keys[i] = elosslessOptKey{key: key, ok: ok}
		cache.insert(pixel)
	}
	return keys, nil
}

// Min-heaps mirroring Rust BinaryHeap<Reverse<...>> (lexicographic tuple order).

type elosslessPendingItem struct {
	start    int
	endEx    int
	cost     int
	source   int
	distance int
}

type elosslessPendingHeap []elosslessPendingItem

func (h elosslessPendingHeap) Len() int { return len(h) }
func (h elosslessPendingHeap) Less(i, j int) bool {
	a, b := h[i], h[j]
	if a.start != b.start {
		return a.start < b.start
	}
	if a.endEx != b.endEx {
		return a.endEx < b.endEx
	}
	if a.cost != b.cost {
		return a.cost < b.cost
	}
	if a.source != b.source {
		return a.source < b.source
	}
	return a.distance < b.distance
}
func (h elosslessPendingHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *elosslessPendingHeap) Push(x any)   { *h = append(*h, x.(elosslessPendingItem)) }
func (h *elosslessPendingHeap) Pop() any {
	old := *h
	n := len(old)
	it := old[n-1]
	*h = old[:n-1]
	return it
}

type elosslessActiveItem struct {
	cost     int
	endEx    int
	source   int
	distance int
}

type elosslessActiveHeap []elosslessActiveItem

func (h elosslessActiveHeap) Len() int { return len(h) }
func (h elosslessActiveHeap) Less(i, j int) bool {
	a, b := h[i], h[j]
	if a.cost != b.cost {
		return a.cost < b.cost
	}
	if a.endEx != b.endEx {
		return a.endEx < b.endEx
	}
	if a.source != b.source {
		return a.source < b.source
	}
	return a.distance < b.distance
}
func (h elosslessActiveHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *elosslessActiveHeap) Push(x any)   { *h = append(*h, x.(elosslessActiveItem)) }
func (h *elosslessActiveHeap) Pop() any {
	old := *h
	n := len(old)
	it := old[n-1]
	*h = old[:n-1]
	return it
}

func elosslessBuildTokensWithTraceback(width int, argb []uint32, options elosslessTokenBuildOptions, costModel *elosslessTracebackCostModel) ([]elosslessToken, error) {
	bestCosts := make([]int, len(argb)+1)
	previous := make([]int, len(argb)+1)
	steps := make([]elosslessTracebackStep, len(argb)+1)
	for i := range bestCosts {
		bestCosts[i] = elosslessIntMax
		previous[i] = elosslessIntMax
	}
	heads := make([]int, elosslessMatchHashSize)
	elosslessFillInt(heads, elosslessIntMax)
	prev := make([]int, len(argb))
	elosslessFillInt(prev, elosslessIntMax)
	cacheKeys, err := elosslessBuildCacheKeys(argb, options.colorCacheBits)
	if err != nil {
		return nil, err
	}
	var windowOffsets []int
	if options.useWindowOffsets {
		windowOffsets = elosslessBuildWindowOffsets(width, options.windowOffsetLimit)
	}
	pending := &elosslessPendingHeap{}
	active := &elosslessActiveHeap{}

	bestCosts[0] = 0
	for index := 0; index <= len(argb); index++ {
		for pending.Len() > 0 {
			top := (*pending)[0]
			if top.start > index {
				break
			}
			heap.Pop(pending)
			if top.endEx > index {
				heap.Push(active, elosslessActiveItem{cost: top.cost, endEx: top.endEx, source: top.source, distance: top.distance})
			}
		}
		for active.Len() > 0 {
			top := (*active)[0]
			if top.endEx > index {
				break
			}
			heap.Pop(active)
		}
		if active.Len() > 0 {
			top := (*active)[0]
			if top.cost < bestCosts[index] {
				bestCosts[index] = top.cost
				previous[index] = top.source
				steps[index] = elosslessTracebackStep{kind: elosslessStepCopy, distance: top.distance, length: index - top.source}
			}
		}
		if index == len(argb) {
			break
		}

		baseCost := bestCosts[index]
		if baseCost == elosslessIntMax {
			elosslessUpdateMatchChain(argb, index, heads, prev)
			continue
		}

		if cacheKeys[index].ok {
			key := cacheKeys[index].key
			cacheCost := elosslessSatAdd(baseCost, costModel.cacheCost(key))
			if cacheCost < bestCosts[index+1] {
				bestCosts[index+1] = cacheCost
				previous[index+1] = index
				steps[index+1] = elosslessTracebackStep{kind: elosslessStepCache, key: key}
			}
		}

		literalCost := elosslessSatAdd(baseCost, costModel.literalCost(argb[index]))
		if literalCost < bestCosts[index+1] {
			bestCosts[index+1] = literalCost
			previous[index+1] = index
			steps[index+1] = elosslessTracebackStep{kind: elosslessStepLiteral}
		}

		for _, dl := range elosslessCollectMatchCandidates(width, argb, index, options, heads, prev, windowOffsets) {
			distance := dl[0]
			length := dl[1]
			minLength := elosslessMinMatchLengthForDistance(width, distance)
			dc, err := costModel.distanceCost(width, distance)
			if err != nil {
				return nil, err
			}
			distanceCost := elosslessSatAdd(baseCost, dc)
			for _, iv := range costModel.lengthCostIntervals {
				startLength := iv[0]
				endLengthExclusive := iv[1]
				lengthCost := iv[2]
				if startLength > length {
					break
				}
				start := minLength
				if startLength > start {
					start = startLength
				}
				endExclusive := length + 1
				if endLengthExclusive < endExclusive {
					endExclusive = endLengthExclusive
				}
				if start < endExclusive {
					heap.Push(pending, elosslessPendingItem{
						start:    index + start,
						endEx:    index + endExclusive,
						cost:     elosslessSatAdd(distanceCost, lengthCost),
						source:   index,
						distance: distance,
					})
				}
			}
		}

		elosslessUpdateMatchChain(argb, index, heads, prev)
	}

	tokens := make([]elosslessToken, 0, len(argb))
	cursor := len(argb)
	for cursor > 0 {
		st := steps[cursor]
		if st.kind == elosslessStepNone {
			return nil, encBitstream("traceback path is incomplete")
		}
		switch st.kind {
		case elosslessStepLiteral:
			tokens = append(tokens, elosslessToken{kind: elosslessTokLiteral, argb: argb[cursor-1]})
			cursor = previous[cursor]
		case elosslessStepCache:
			tokens = append(tokens, elosslessToken{kind: elosslessTokCache, key: st.key})
			cursor = previous[cursor]
		case elosslessStepCopy:
			tokens = append(tokens, elosslessToken{kind: elosslessTokCopy, distance: st.distance, length: st.length})
			start := cursor - st.length
			if start < 0 {
				start = 0
			}
			if previous[cursor] != start {
				return nil, encBitstream("traceback predecessor is inconsistent")
			}
			cursor = start
		}
		if cursor != 0 && steps[cursor].kind == elosslessStepNone {
			return nil, encBitstream("traceback predecessor is missing")
		}
	}
	for i, j := 0, len(tokens)-1; i < j; i, j = i+1, j-1 {
		tokens[i], tokens[j] = tokens[j], tokens[i]
	}
	return tokens, nil
}

func elosslessBuildTokens(width int, argb []uint32, options elosslessTokenBuildOptions) ([]elosslessToken, error) {
	greedy, err := elosslessBuildTokensGreedy(width, argb, options)
	if err != nil {
		return nil, err
	}
	if !options.useTraceback {
		return greedy, nil
	}

	costModel, err := elosslessBuildTracebackCostModel(width, greedy, options.colorCacheBits)
	if err != nil {
		return nil, err
	}
	traceback, err := elosslessBuildTokensWithTraceback(width, argb, options, &costModel)
	if err != nil {
		return nil, err
	}
	height := len(argb) / width
	greedyCost, err := elosslessEstimateImageStreamSize(width, height, greedy, options.colorCacheBits, false, 0)
	if err != nil {
		return nil, err
	}
	tracebackCost, err := elosslessEstimateImageStreamSize(width, height, traceback, options.colorCacheBits, false, 0)
	if err != nil {
		return nil, err
	}
	if tracebackCost < greedyCost {
		return traceback, nil
	}
	return greedy, nil
}
