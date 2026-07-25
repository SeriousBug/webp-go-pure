package webp

import (
	"math/rand"
	"testing"
)

// The refinement loop prices trials through elossyRatePrefix instead of
// elossyCoefficientsRate. The two must agree exactly, or the search makes
// different decisions and the encoder's output changes.
func TestRatePrefixMatchesDirectRate(t *testing.T) {
	model := elossyBuildRateModel(&elossyCoeffsProba0)
	rng := rand.New(rand.NewSource(1))

	for trial := 0; trial < 20000; trial++ {
		coeffType := rng.Intn(numTypes)
		entryCtx := rng.Intn(numCtx)
		first := 0
		if rng.Intn(2) == 0 {
			first = 1
		}

		var levels [16]int16
		density := rng.Intn(5)
		for i := range levels {
			if rng.Intn(4) < density {
				magnitude := int16(1 + rng.Intn(3))
				if rng.Intn(8) == 0 {
					magnitude = int16(1 + rng.Intn(80))
				}
				if rng.Intn(2) == 0 {
					magnitude = -magnitude
				}
				levels[i] = magnitude
			}
		}

		var prefix elossyRatePrefix
		prefix.reset(model, coeffType, entryCtx, first, &levels)

		if got, want := prefix.rate0(), elossyCoefficientsRate(model, coeffType, entryCtx, first, &levels); got != want {
			t.Fatalf("trial %d: base rate = %d, want %d (levels %v, first %d, ctx %d, type %d)",
				trial, got, want, levels, first, entryCtx, coeffType)
		}

		// Walk the zigzag downward exactly as the refinement does, pricing each
		// step both ways and committing the ones it would accept.
		for scan := 15; scan >= first; scan-- {
			index := elossyZigzag[scan]
			for levels[index] != 0 {
				current := levels[index]
				var next int16
				if current > 0 {
					next = current - 1
				} else {
					next = current + 1
				}

				probe := levels
				probe[index] = next
				got := prefix.rateWith(scan, next)
				want := elossyCoefficientsRate(model, coeffType, entryCtx, first, &probe)
				if got != want {
					t.Fatalf("trial %d scan %d: rate = %d, want %d (levels %v -> %d, first %d, ctx %d, type %d)",
						trial, scan, got, want, levels, next, first, entryCtx, coeffType)
				}

				if rng.Intn(2) == 0 {
					break
				}
				levels[index] = next
				prefix.commit(scan, next)
			}
		}
	}
}
