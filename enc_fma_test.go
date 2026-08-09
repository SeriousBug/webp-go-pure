package webp

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// The encoder ranks candidate encodings by floating-point cost estimates and
// keeps the cheapest. Candidates routinely score within an ulp of each other,
// so the ranking only reproduces across machines if every machine computes the
// costs with identical rounding.
//
// IEEE-754 double arithmetic reproduces exactly, with one exception the Go spec
// allows: an implementation may fuse a multiply into the add or subtract that
// consumes it and round once instead of twice. The arm64 backend does fuse and
// the amd64 backend does not, so `sum -= float64(count) * log2(count)` in a
// cost loop summed differently per architecture, flipped a `cost < bestCost`
// comparison, and changed the encoded bytes. That produced ALPH chunks that
// differed by up to 0.35% between arm64 and amd64 for the same input.
//
// The spec stops the fusion when the product is explicitly rounded by a
// conversion, so every cost accumulation writes its product as
// `float64(a * b)`. This test compiles the package and fails if any fused
// multiply-add instruction survives, which is the only property that has to
// hold; it needs one architecture to be worth running, because the compiler for
// that architecture is the thing under test.
func TestNoFusedMultiplyAdd(t *testing.T) {
	if testing.Short() {
		t.Skip("compiling the package with -S is too slow for -short")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go toolchain on PATH")
	}

	cmd := exec.Command("go", "build", "-gcflags=-S", ".")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build -gcflags=-S: %v\n%s", err, out)
	}
	listing := string(out)
	if !strings.Contains(listing, "enc_alpha.go") {
		t.Fatalf("assembly listing does not mention the encoder sources; got %d bytes", len(listing))
	}

	// Mnemonics for fused multiply-add across the architectures whose Go
	// backends fuse: arm64, riscv64, loong64, ppc64, s390x.
	fused := regexp.MustCompile(`\b(F|VF|WF|XSF|XVF)N?M(ADD|SUB)`)
	var bad []string
	for _, line := range strings.Split(listing, "\n") {
		if fused.MatchString(line) {
			bad = append(bad, strings.TrimSpace(line))
		}
	}
	if len(bad) > 0 {
		t.Errorf("compiler fused %d multiply-add(s); round the product with float64(a*b) at each site:\n%s",
			len(bad), strings.Join(bad, "\n"))
	}
}
