package webp

import (
	"math/rand"
	"sort"
	"testing"
)

// refGenerateCodeLengths is the insertion-sort tree builder this package used
// before the two-queue merge replaced it. It is kept here so the rewrite can be
// checked against it symbol for symbol: the encoder's output is only unchanged
// if the code lengths are.
type refNode struct {
	totalCount uint32
	value      int
	left       int
	right      int
}

func refSetBitDepths(node *refNode, pool []refNode, bitDepths []uint8, level uint8) {
	if node.left >= 0 {
		refSetBitDepths(&pool[node.left], pool, bitDepths, level+1)
		refSetBitDepths(&pool[node.right], pool, bitDepths, level+1)
	} else {
		bitDepths[node.value] = level
	}
}

func refGenerateCodeLengths(histogram []uint32, treeDepthLimit int) ([]uint8, error) {
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
		var tree []refNode
		for value, count := range histogram {
			if count != 0 {
				tc := count
				if countMin > tc {
					tc = countMin
				}
				tree = append(tree, refNode{tc, value, -1, -1})
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
			treePool := make([]refNode, 0, len(tree)*2)
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
				newNode := refNode{count, -1, len(treePool) - 1, len(treePool) - 2}
				tree = append(tree, refNode{})
				copy(tree[insertAt+1:], tree[insertAt:])
				tree[insertAt] = newNode
				treeSize++
			}
			refSetBitDepths(&tree[0], treePool, codeLengths, 0)
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

// TestGenerateCodeLengthsMatchesReference compares the two builders over
// random histograms, including Fibonacci-like counts that force the deepest
// trees and a depth limit low enough to trigger the count-scaling retry.
func TestGenerateCodeLengthsMatchesReference(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for iter := 0; iter < 20000; iter++ {
		n := 1 + rng.Intn(300)
		hist := make([]uint32, n)
		mode := iter % 5
		for i := range hist {
			switch mode {
			case 0:
				hist[i] = uint32(rng.Intn(4))
			case 1:
				hist[i] = uint32(rng.Intn(1 << 20))
			case 2:
				if rng.Intn(10) == 0 {
					hist[i] = uint32(rng.Intn(1 << 28))
				}
			case 3:
				hist[i] = 1
			case 4:
				// Fibonacci-like counts force the deepest possible trees.
				// Stop growing before the total would overflow uint32.
				if i < 2 {
					hist[i] = 1
				} else if hist[i-1] < (1<<30)/2 {
					hist[i] = hist[i-1] + hist[i-2]
				} else {
					hist[i] = hist[i-1]
				}
			}
		}
		total := uint64(0)
		for _, c := range hist {
			total += uint64(c)
		}
		if total >= 1<<32 {
			// Internal node counts are uint32; the reference sorts on them, so
			// an overflowing total is outside both implementations' domain.
			continue
		}
		limit := 15
		if iter%3 == 0 {
			limit = 7
		}
		want, wantErr := refGenerateCodeLengths(hist, limit)
		got, gotErr := elosslessGenerateCodeLengths(hist, limit)
		if (wantErr == nil) != (gotErr == nil) {
			t.Fatalf("iter %d: error mismatch: ref=%v got=%v", iter, wantErr, gotErr)
		}
		if wantErr != nil {
			continue
		}
		for i := range want {
			if want[i] != got[i] {
				t.Fatalf("iter %d mode %d: code length %d: ref=%d got=%d", iter, mode, i, want[i], got[i])
			}
		}
	}
}
