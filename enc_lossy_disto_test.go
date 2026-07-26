package webp

import (
	"math/rand"
	"testing"
)

// elossyTDisto4x4Reference is a direct transcription of the definition: the
// weighted sum of the absolute values of H*D*H^T, where D is the residual.
func elossyTDisto4x4Reference(src []uint8, srcStride, srcX, srcY int, pred []uint8, predStride, predX, predY int) uint32 {
	hadamard := [4][4]int32{
		{1, 1, 1, 1},
		{1, 1, -1, -1},
		{1, -1, -1, 1},
		{1, -1, 1, -1},
	}
	var residual [4][4]int32
	for row := 0; row < 4; row++ {
		for col := 0; col < 4; col++ {
			s := int32(src[(srcY+row)*srcStride+srcX+col])
			p := int32(pred[(predY+row)*predStride+predX+col])
			residual[row][col] = s - p
		}
	}

	var rowPass [4][4]int32
	for row := 0; row < 4; row++ {
		for out := 0; out < 4; out++ {
			var acc int32
			for col := 0; col < 4; col++ {
				acc += hadamard[out][col] * residual[row][col]
			}
			rowPass[row][out] = acc
		}
	}

	var sum int32
	for col := 0; col < 4; col++ {
		for out := 0; out < 4; out++ {
			var acc int32
			for row := 0; row < 4; row++ {
				acc += hadamard[out][row] * rowPass[row][col]
			}
			if acc < 0 {
				acc = -acc
			}
			sum += int32(elossyWeightY[out*4+col]) * acc
		}
	}
	return uint32(sum) >> 5
}

func TestElossyTDisto4x4MatchesReference(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	for trial := 0; trial < 5000; trial++ {
		srcStride := 4 + rng.Intn(60)
		predStride := 4 + rng.Intn(20)
		srcX := rng.Intn(20)
		srcY := rng.Intn(20)
		predX := rng.Intn(predStride - 3)
		predY := rng.Intn(8)
		src := make([]byte, (srcY+4)*srcStride+srcX+4)
		for i := range src {
			src[i] = byte(rng.Intn(256))
		}
		pred := make([]byte, (predY+4)*predStride+predX+4)
		for i := range pred {
			pred[i] = byte(rng.Intn(256))
		}
		want := elossyTDisto4x4Reference(src, srcStride, srcX, srcY, pred, predStride, predX, predY)
		got := elossyTDisto4x4(src, srcStride, srcX, srcY, pred, predStride, predX, predY)
		if got != want {
			t.Fatalf("trial=%d: got=%d want=%d", trial, got, want)
		}
	}
}

func TestElossyTDisto4x4ContiguousMatchesReference(t *testing.T) {
	rng := rand.New(rand.NewSource(12))
	for trial := 0; trial < 5000; trial++ {
		srcStride := 4 + rng.Intn(60)
		srcX := rng.Intn(20)
		srcY := rng.Intn(20)
		src := make([]byte, (srcY+4)*srcStride+srcX+4)
		for i := range src {
			src[i] = byte(rng.Intn(256))
		}
		var pred [16]uint8
		for i := range pred {
			pred[i] = byte(rng.Intn(256))
		}
		want := elossyTDisto4x4Reference(src, srcStride, srcX, srcY, pred[:], 4, 0, 0)
		got := elossyTDisto4x4Contiguous(src, srcStride, srcX, srcY, &pred)
		if got != want {
			t.Fatalf("trial=%d: got=%d want=%d", trial, got, want)
		}
	}
}

func TestElossyTDisto4x4ZeroForExactPrediction(t *testing.T) {
	src := make([]byte, 4*8)
	for i := range src {
		src[i] = byte(i * 7)
	}
	var pred [16]uint8
	for row := 0; row < 4; row++ {
		copy(pred[row*4:row*4+4], src[row*8:row*8+4])
	}
	if got := elossyTDisto4x4Contiguous(src, 8, 0, 0, &pred); got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
}

func TestElossyModeScreenKeepsLowestScores(t *testing.T) {
	rng := rand.New(rand.NewSource(13))
	for trial := 0; trial < 500; trial++ {
		limit := 1 + rng.Intn(numBModes)
		var screen elossyModeScreen
		screen.reset(limit)
		var scores [numBModes]uint64
		for mode := 0; mode < numBModes; mode++ {
			scores[mode] = uint64(rng.Intn(1000))
			screen.add(uint8(mode), scores[mode])
		}
		selected := screen.selected()
		if len(selected) != limit {
			t.Fatalf("trial=%d: got %d modes, want %d", trial, len(selected), limit)
		}
		for i := 1; i < len(selected); i++ {
			if screen.scores[i-1] > screen.scores[i] {
				t.Fatalf("trial=%d: scores out of order", trial)
			}
		}
		for _, mode := range selected {
			for other := 0; other < numBModes; other++ {
				if scores[other] < screen.scores[len(selected)-1] {
					found := false
					for _, kept := range selected {
						if int(kept) == other {
							found = true
						}
					}
					if !found {
						t.Fatalf("trial=%d: mode %d (score %d) missing from top %d", trial, other, scores[other], limit)
					}
				}
			}
			_ = mode
		}
	}
}
