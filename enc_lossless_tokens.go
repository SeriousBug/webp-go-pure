package webp

// Backward references, cache selection, and tokenization for lossless encoding.
// Ported from src/encoder/lossless/tokens.rs.

import (
	"math"
	"math/bits"
	"sync"
)

// The lossless search re-tokenizes and re-histograms the image many times per
// encode, so its scratch arrays are recycled rather than handed to the
// collector. Buffers are only pooled where they provably do not outlive the
// call that borrowed them.
var elosslessIntBufPool = sync.Pool{New: func() any { return new([]int) }}

var elosslessU32BufPool = sync.Pool{New: func() any { return new([]uint32) }}

func elosslessBorrowIntBuf(n int) *[]int {
	buf := elosslessIntBufPool.Get().(*[]int)
	if cap(*buf) < n {
		*buf = make([]int, n)
	}
	*buf = (*buf)[:n]
	return buf
}

func elosslessReturnIntBuf(buf *[]int) { elosslessIntBufPool.Put(buf) }

// elosslessBorrowZeroedU32Buf returns a zeroed buffer of length n.
func elosslessBorrowZeroedU32Buf(n int) *[]uint32 {
	buf := elosslessU32BufPool.Get().(*[]uint32)
	if cap(*buf) < n {
		*buf = make([]uint32, n)
	} else {
		*buf = (*buf)[:n]
		clear(*buf)
	}
	*buf = (*buf)[:n]
	return buf
}

func elosslessReturnU32Buf(buf *[]uint32) { elosslessU32BufPool.Put(buf) }

var elosslessInt32BufPool = sync.Pool{New: func() any { return new([]int32) }}

func elosslessBorrowInt32Buf(n int) *[]int32 {
	buf := elosslessInt32BufPool.Get().(*[]int32)
	if cap(*buf) < n {
		*buf = make([]int32, n)
	}
	*buf = (*buf)[:n]
	return buf
}

func elosslessReturnInt32Buf(buf *[]int32) { elosslessInt32BufPool.Put(buf) }

type elosslessMatch struct {
	distance int
	length   int
	set      bool
	score    int
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
	default:
		matchChainDepth, useWindowOffsets, windowOffsetLimit, lazyMatching = elosslessMatchChainDepthLevel4, true, 64, true
	}
	return elosslessTokenBuildOptions{
		colorCacheBits:    colorCacheBits,
		matchChainDepth:   matchChainDepth,
		useWindowOffsets:  useWindowOffsets,
		windowOffsetLimit: windowOffsetLimit,
		lazyMatching:      lazyMatching,
	}
}

func (o elosslessTokenBuildOptions) parseCostCacheBits() int {
	if o.colorCacheBits > 0 {
		return o.colorCacheBits
	}
	return o.costCacheBits
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
	default:
		return 9
	}
}

func elosslessShortlistColorCacheCandidatesForProfile(profile *elosslessLosslessSearchProfile) int {
	switch profile.entropySearchLevel {
	case 0, 1:
		return 1
	default:
		return 2
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
	default:
		return [][2]int{{6, 2}, {5, 4}, {4, 4}}
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

// elosslessSymbolCosts holds the modelled code length of every token symbol, in
// elosslessCostScale-ths of a bit, derived from the token histograms of a first
// tokenization pass. Scoring a match against these instead of against flat
// per-symbol constants is what lets the parse tell a cheap copy from an
// expensive one on content where literals, cache references and copies do not
// cost anything like the same.
type elosslessSymbolCosts struct {
	green []int32
	red   []int32
	blue  []int32
	alpha []int32
	dist  []int32
	// length is indexed by match length and already folded together the length
	// prefix symbol and its extra bits.
	length []int32
}

// elosslessMaxSymbolCostBits caps the cost charged to a symbol the first pass
// never emitted. Such a symbol is not free to add to a tree, but it is also not
// infinitely expensive, and an uncapped estimate would let one unseen pixel
// value veto an otherwise good parse decision.
const elosslessMaxSymbolCostBits = 24

func elosslessChannelSymbolCosts(counts []uint32) []int32 {
	total := 0.0
	for _, count := range counts {
		total += float64(count)
	}
	out := make([]int32, len(counts))
	if total == 0 {
		for i := range out {
			out[i] = elosslessMaxSymbolCostBits * elosslessCostScale
		}
		return out
	}
	for i, count := range counts {
		frequency := float64(count)
		if frequency == 0 {
			frequency = 0.25
		}
		bits := math.Log2(total / frequency)
		if bits > elosslessMaxSymbolCostBits {
			bits = elosslessMaxSymbolCostBits
		}
		// The conversion keeps the product out of a fused multiply-add. See
		// enc_fma_test.go.
		out[i] = int32(float64(bits*elosslessCostScale) + 0.5)
	}
	return out
}

func elosslessNewSymbolCosts(histograms *elosslessHistogramSet) *elosslessSymbolCosts {
	costs := &elosslessSymbolCosts{
		green: elosslessChannelSymbolCosts(histograms[0]),
		red:   elosslessChannelSymbolCosts(histograms[1]),
		blue:  elosslessChannelSymbolCosts(histograms[2]),
		alpha: elosslessChannelSymbolCosts(histograms[3]),
		dist:  elosslessChannelSymbolCosts(histograms[4]),
	}
	costs.length = make([]int32, elosslessMaxLength+1)
	for length := 1; length <= elosslessMaxLength; length++ {
		symbol, extraBits := elosslessPrefixSymbolExtra(length)
		costs.length[length] = costs.green[elosslessNumLiteralCodes+symbol] + int32(extraBits*elosslessCostScale)
	}
	return costs
}

func (s *elosslessSymbolCosts) literalCost(pixel uint32) int32 {
	return s.green[(pixel>>8)&0xff] +
		s.red[(pixel>>16)&0xff] +
		s.blue[pixel&0xff] +
		s.alpha[pixel>>24]
}

func (s *elosslessSymbolCosts) cacheCost(key int) int32 {
	index := elosslessNumLiteralCodes + elosslessNumLengthCodes + key
	if index >= len(s.green) {
		return elosslessMaxSymbolCostBits * elosslessCostScale
	}
	return s.green[index]
}

func (s *elosslessSymbolCosts) distanceCost(planeCode int) int32 {
	symbol, extraBits := elosslessPrefixSymbolExtra(planeCode)
	if symbol >= len(s.dist) {
		symbol = len(s.dist) - 1
	}
	return s.dist[symbol] + int32(extraBits*elosslessCostScale)
}

// elosslessPrefixSymbolExtra is elosslessPrefixEncode without the extra-bit
// payload or the error return, for the parse's inner loop.
func elosslessPrefixSymbolExtra(value int) (symbol, extraBits int) {
	if value <= 4 {
		return value - 1, 0
	}
	value--
	highestBit := bits.Len(uint(value)) - 1
	return 2*highestBit + ((value >> (highestBit - 1)) & 1), highestBit - 1
}

// elosslessLiteralCostPrefix returns prefix sums over the modelled cost of
// coding each pixel on its own: a color cache reference for a pixel already in
// the cache at that point, a full literal otherwise. Every pixel enters the
// cache in stream order no matter how the parse codes it, so the hit pattern is
// a property of the pixel sequence alone and can be computed before the parse.
// Sums are taken over spans of at most elosslessMaxLength pixels, so the int32
// accumulator wrapping on huge images does not affect any difference read out of
// it.
func elosslessLiteralCostPrefix(prefix []int32, argb []uint32, cacheBits int, symbols *elosslessSymbolCosts) error {
	var cache elosslessColorCache
	if cacheBits > 0 {
		c, err := elosslessColorCacheNew(cacheBits)
		if err != nil {
			return err
		}
		cache = c
	}
	prefix[0] = 0
	for i, pixel := range argb {
		var cost int32
		key, hit := 0, false
		if cacheBits > 0 {
			key, hit = cache.lookup(pixel)
		}
		switch {
		case symbols == nil && hit:
			cost = elosslessApproxCacheCostBits * elosslessCostScale
		case symbols == nil:
			cost = elosslessApproxLiteralCostBits * elosslessCostScale
		case hit:
			cost = symbols.cacheCost(key)
		default:
			cost = symbols.literalCost(pixel)
		}
		prefix[i+1] = prefix[i] + cost
		if cacheBits > 0 {
			cache.insert(pixel)
		}
	}
	return nil
}

// elosslessMatchCosts scores a candidate match against the real alternative of
// coding the same pixels one at a time. Without a color cache that alternative
// is a literal per pixel; with one, pixels the cache already holds are charged
// the much cheaper cache reference, so the parse stops taking long matches over
// runs that would have coded as cache hits anyway. Costs are in
// elosslessCostScale-ths of a bit.
type elosslessMatchCosts struct {
	width         int
	literalPrefix []int32
	symbols       *elosslessSymbolCosts
}

func (c *elosslessMatchCosts) pixelsCostBits(index, length int) int {
	if c.literalPrefix == nil {
		return elosslessApproxLiteralCostBits * elosslessCostScale * length
	}
	return int(c.literalPrefix[index+length] - c.literalPrefix[index])
}

func (c *elosslessMatchCosts) copyCostBits(distance, length int) int {
	planeCode := elosslessDistanceToPlaneCode(c.width, distance)
	if c.symbols == nil {
		return elosslessCostScale * (elosslessApproxCopyLengthSymbolBits +
			elosslessPrefixExtraBitCount(length) +
			elosslessApproxCopyDistanceSymbolBits +
			elosslessPrefixExtraBitCount(planeCode))
	}
	return int(c.symbols.length[length] + c.symbols.distanceCost(planeCode))
}

func (c *elosslessMatchCosts) matchGainBits(index, distance, length int) int {
	return c.pixelsCostBits(index, length) - c.copyCostBits(distance, length)
}

func elosslessConsiderMatch(costs *elosslessMatchCosts, best *elosslessMatch, index, distance, length int) {
	if length < elosslessMinMatchLengthForDistance(costs.width, distance) {
		return
	}

	candidateScore := costs.matchGainBits(index, distance, length)
	// A match that costs more than coding the same pixels one at a time is worse
	// than not matching at all. Under the flat cost constants this never happens,
	// but once the costs are measured from the image a copy over pixels the color
	// cache already holds routinely loses to the cache references it displaces.
	if candidateScore <= 0 {
		return
	}
	better := true
	if best.set {
		better = candidateScore > best.score ||
			(candidateScore == best.score &&
				(length > best.length ||
					(length == best.length && distance < best.distance)))
	}
	if better {
		*best = elosslessMatch{distance: distance, length: length, set: true, score: candidateScore}
	}
}

type elosslessPreview struct {
	hash    int
	oldPrev int
	oldHead int
	valid   bool
}

func elosslessPreviewUpdateMatchChain(argb []uint32, index int, heads, prev []int, hashShift uint32) elosslessPreview {
	if prev == nil || index+elosslessMinLength > len(argb) {
		return elosslessPreview{}
	}
	hash := elosslessHashMatchPixels(argb, index, hashShift)
	oldPrev := prev[index]
	oldHead := heads[hash]
	elosslessUpdateMatchChain(argb, index, heads, prev, hashShift)
	return elosslessPreview{hash: hash, oldPrev: oldPrev, oldHead: oldHead, valid: true}
}

func elosslessRestorePreviewedMatchChain(index int, preview elosslessPreview, heads, prev []int) {
	if preview.valid {
		prev[index] = preview.oldPrev
		heads[preview.hash] = preview.oldHead
	}
}

// elosslessMatchHashParams sizes the hash-chain head table to the image: about
// one bucket per pixel keeps chains short, clamped so small images don't pay for
// an oversized memset and huge images don't blow the table up. Returns the head
// count and the hash right-shift (32 - bits).
func elosslessMatchHashParams(n int) (size int, hashShift uint32) {
	b := bits.Len(uint(n - 1))
	if b < elosslessMinMatchHashBits {
		b = elosslessMinMatchHashBits
	} else if b > elosslessMaxMatchHashBits {
		b = elosslessMaxMatchHashBits
	}
	return 1 << b, uint32(32 - b)
}

func elosslessHashMatchPixels(argb []uint32, index int, hashShift uint32) int {
	a := argb[index]
	b := bits.RotateLeft32(argb[index+1], 7)
	c := bits.RotateLeft32(argb[index+2], 13)
	d := bits.RotateLeft32(argb[index+3], 21)
	hash := a ^ b ^ c ^ (d * elosslessColorCacheHashMul)
	return int((hash * elosslessColorCacheHashMul) >> hashShift)
}

func elosslessUpdateMatchChain(argb []uint32, index int, heads, prev []int, hashShift uint32) {
	if prev == nil || index+elosslessMinLength > len(argb) {
		return
	}
	hash := elosslessHashMatchPixels(argb, index, hashShift)
	prev[index] = heads[hash]
	heads[hash] = index
}

func elosslessFindBestHashMatch(costs *elosslessMatchCosts, argb []uint32, index, maxLen int, heads, prev []int, matchChainDepth int, hashShift uint32) elosslessMatch {
	var best elosslessMatch
	if matchChainDepth == 0 || maxLen < elosslessMinLength || index+elosslessMinLength > len(argb) {
		return best
	}

	hash := elosslessHashMatchPixels(argb, index, hashShift)
	candidate := heads[hash]
	remaining := matchChainDepth

	// The chain is walked nearest-first (increasing distance), so a candidate
	// that cannot exceed the longest match seen cannot beat it on gain either.
	// Reject such candidates with a single comparison at position bestLen
	// (libwebp's best_argb trick) instead of a full FindMatchLength.
	bestLen := 0
	var bestArgb uint32

	for candidate != elosslessIntMax && remaining > 0 {
		remaining--
		if candidate >= index {
			break
		}
		distance := index - candidate
		if distance <= elosslessMaxFallbackDistance {
			if bestLen == 0 || argb[candidate+bestLen] == bestArgb {
				length := elosslessFindMatchLength(argb, index, candidate, maxLen)
				if length >= elosslessMinLength {
					elosslessConsiderMatch(costs, &best, index, distance, length)
				}
				if length > bestLen {
					bestLen = length
					if bestLen >= maxLen {
						break
					}
					bestArgb = argb[index+bestLen]
				}
			}
		}
		candidate = prev[candidate]
	}

	return best
}

func elosslessFindBestWindowOffsetMatch(costs *elosslessMatchCosts, argb []uint32, index, maxLen int, windowOffsets []int) elosslessMatch {
	var best elosslessMatch
	for _, distance := range windowOffsets {
		if distance > index || distance > elosslessMaxFallbackDistance {
			continue
		}
		candidateIndex := index - distance
		length := elosslessFindMatchLength(argb, index, candidateIndex, maxLen)
		if length >= elosslessMinLength {
			elosslessConsiderMatch(costs, &best, index, distance, length)
		}
	}
	return best
}

func elosslessFindBestMatch(costs *elosslessMatchCosts, argb []uint32, index int, options elosslessTokenBuildOptions, heads, prev, windowOffsets []int, hashShift uint32) elosslessMatch {
	maxLen := len(argb) - index
	if elosslessMaxLength < maxLen {
		maxLen = elosslessMaxLength
	}
	var best elosslessMatch

	if index > 0 {
		rleLen := elosslessFindMatchLength(argb, index, index-1, maxLen)
		elosslessConsiderMatch(costs, &best, index, 1, rleLen)
	}
	if index >= costs.width {
		prevRowLen := elosslessFindMatchLength(argb, index, index-costs.width, maxLen)
		elosslessConsiderMatch(costs, &best, index, costs.width, prevRowLen)
	}
	if options.useWindowOffsets {
		m := elosslessFindBestWindowOffsetMatch(costs, argb, index, maxLen, windowOffsets)
		if m.set {
			elosslessConsiderMatch(costs, &best, index, m.distance, m.length)
		}
	}
	m := elosslessFindBestHashMatch(costs, argb, index, maxLen, heads, prev, options.matchChainDepth, hashShift)
	if m.set {
		elosslessConsiderMatch(costs, &best, index, m.distance, m.length)
	}

	return best
}

func elosslessFillInt(s []int, v int) {
	for i := range s {
		s[i] = v
	}
}

func elosslessBuildTokens(width int, argb []uint32, options elosslessTokenBuildOptions) ([]elosslessToken, error) {
	return elosslessBuildTokensInto(nil, width, argb, options)
}

// elosslessBuildTokensInto tokenizes into dst's storage when it is large enough.
// The token stream is one token per pixel at worst, so re-parsing an image
// allocates tens of megabytes per pass unless the caller hands back a stream it
// has finished with.
func elosslessBuildTokensInto(dst []elosslessToken, width int, argb []uint32, options elosslessTokenBuildOptions) ([]elosslessToken, error) {
	if len(argb) == 0 {
		return nil, nil
	}

	tokens := dst[:0]
	if cap(tokens) < len(argb) {
		tokens = make([]elosslessToken, 0, len(argb))
	}
	var cache *elosslessColorCache
	if options.colorCacheBits > 0 {
		c, err := elosslessColorCacheNew(options.colorCacheBits)
		if err != nil {
			return nil, err
		}
		cache = &c
	}
	// The hash chain is one int per pixel plus a head table; at match search
	// level 0 nothing reads it, so neither is allocated or maintained.
	var heads, prev []int
	var hashShift uint32
	if options.matchChainDepth > 0 {
		var headSize int
		headSize, hashShift = elosslessMatchHashParams(len(argb))
		headsBuf := elosslessBorrowIntBuf(headSize)
		defer elosslessReturnIntBuf(headsBuf)
		heads = *headsBuf
		elosslessFillInt(heads, elosslessIntMax)
		prevBuf := elosslessBorrowIntBuf(len(argb))
		defer elosslessReturnIntBuf(prevBuf)
		prev = *prevBuf
		elosslessFillInt(prev, elosslessIntMax)
	}
	var windowOffsets []int
	if options.useWindowOffsets {
		windowOffsets = elosslessBuildWindowOffsets(width, options.windowOffsetLimit)
	}
	costs := elosslessMatchCosts{width: width, symbols: options.symbolCosts}
	costCacheBits := options.parseCostCacheBits()
	if costCacheBits > 0 || options.symbolCosts != nil {
		prefixBuf := elosslessBorrowInt32Buf(len(argb) + 1)
		defer elosslessReturnInt32Buf(prefixBuf)
		if err := elosslessLiteralCostPrefix(*prefixBuf, argb, costCacheBits, options.symbolCosts); err != nil {
			return nil, err
		}
		costs.literalPrefix = *prefixBuf
	}

	index := 0
	for index < len(argb) {
		cacheKey := 0
		cacheHit := false
		if cache != nil {
			cacheKey, cacheHit = cache.lookup(argb[index])
		}
		bestMatch := elosslessFindBestMatch(&costs, argb, index, options, heads, prev, windowOffsets, hashShift)

		if options.lazyMatching {
			if bestMatch.set {
				distance := bestMatch.distance
				length := bestMatch.length
				if length < 64 && index+1 < len(argb) {
					preview := elosslessPreviewUpdateMatchChain(argb, index, heads, prev, hashShift)
					nextMatch := elosslessFindBestMatch(&costs, argb, index+1, options, heads, prev, windowOffsets, hashShift)
					elosslessRestorePreviewedMatchChain(index, preview, heads, prev)

					currentGain := costs.matchGainBits(index, distance, length)
					takeLiteral := false
					if nextMatch.set {
						nextLength := nextMatch.length
						nextGain := costs.matchGainBits(index+1, nextMatch.distance, nextMatch.length)
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
			tokens = append(tokens, elosslessCopyToken(int32(distance), uint16(length)))
			if cache != nil {
				for _, pixel := range argb[index : index+length] {
					cache.insert(pixel)
				}
			}
			for position := index; position < index+length; position++ {
				elosslessUpdateMatchChain(argb, position, heads, prev, hashShift)
			}
			index += length
		} else if cacheHit {
			tokens = append(tokens, elosslessCacheToken(uint16(cacheKey)))
			if cache != nil {
				cache.insert(argb[index])
			}
			elosslessUpdateMatchChain(argb, index, heads, prev, hashShift)
			index++
		} else {
			tokens = append(tokens, elosslessLiteralToken(argb[index]))
			if cache != nil {
				cache.insert(argb[index])
			}
			elosslessUpdateMatchChain(argb, index, heads, prev, hashShift)
			index++
		}
	}

	return tokens, nil
}
