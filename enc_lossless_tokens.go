package webp

// Backward references, cache selection, and tokenization for lossless encoding.
// Ported from src/encoder/lossless/tokens.rs.

import (
	"math/bits"
)

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
	if index+elosslessMinLength > len(argb) {
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
	if index+elosslessMinLength > len(argb) {
		return
	}
	hash := elosslessHashMatchPixels(argb, index, hashShift)
	prev[index] = heads[hash]
	heads[hash] = index
}

func elosslessFindBestHashMatch(width int, argb []uint32, index, maxLen int, heads, prev []int, matchChainDepth int, hashShift uint32) elosslessMatch {
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
					elosslessConsiderMatch(width, &best, distance, length)
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

func elosslessFindBestWindowOffsetMatch(width int, argb []uint32, index, maxLen int, windowOffsets []int) elosslessMatch {
	var best elosslessMatch
	for _, distance := range windowOffsets {
		if distance > index || distance > elosslessMaxFallbackDistance {
			continue
		}
		candidateIndex := index - distance
		length := elosslessFindMatchLength(argb, index, candidateIndex, maxLen)
		if length >= elosslessMinLength {
			elosslessConsiderMatch(width, &best, distance, length)
		}
	}
	return best
}

func elosslessSinglePixelCostBits(cacheHit bool) int {
	if cacheHit {
		return elosslessApproxCacheCostBits
	}
	return elosslessApproxLiteralCostBits
}

func elosslessFindBestMatch(width int, argb []uint32, index int, options elosslessTokenBuildOptions, heads, prev, windowOffsets []int, hashShift uint32) elosslessMatch {
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
	m := elosslessFindBestHashMatch(width, argb, index, maxLen, heads, prev, options.matchChainDepth, hashShift)
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

func elosslessBuildTokens(width int, argb []uint32, options elosslessTokenBuildOptions) ([]elosslessToken, error) {
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
	headSize, hashShift := elosslessMatchHashParams(len(argb))
	heads := make([]int, headSize)
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
		bestMatch := elosslessFindBestMatch(width, argb, index, options, heads, prev, windowOffsets, hashShift)

		if options.lazyMatching {
			if bestMatch.set {
				distance := bestMatch.distance
				length := bestMatch.length
				if length < 64 && index+1 < len(argb) {
					preview := elosslessPreviewUpdateMatchChain(argb, index, heads, prev, hashShift)
					nextMatch := elosslessFindBestMatch(width, argb, index+1, options, heads, prev, windowOffsets, hashShift)
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
			tokens = append(tokens, elosslessToken{kind: elosslessTokCopy, distance: int32(distance), length: uint16(length)})
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
			tokens = append(tokens, elosslessToken{kind: elosslessTokCache, key: uint16(cacheKey)})
			if cache != nil {
				cache.insert(argb[index])
			}
			elosslessUpdateMatchChain(argb, index, heads, prev, hashShift)
			index++
		} else {
			tokens = append(tokens, elosslessToken{kind: elosslessTokLiteral, argb: argb[index]})
			if cache != nil {
				cache.insert(argb[index])
			}
			elosslessUpdateMatchChain(argb, index, heads, prev, hashShift)
			index++
		}
	}

	return tokens, nil
}
