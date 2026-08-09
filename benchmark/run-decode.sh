#!/usr/bin/env bash
# Benchmark WebP decoding across engines:
#   ours     - this pure-Go library
#   libwebp  - the C reference, via github.com/kolesa-team/go-webp (cgo)
#   wasm     - libwebp compiled to WASM, via github.com/gen2brain/webp (cgo-free)
#   x/image  - golang.org/x/image/webp, the Go project's own decoder
#
# Every engine decodes the same files, encoded once by libwebp: lossless at
# level 6, lossy at quality 90. Reports mean decode time and each engine's
# agreement with libwebp's own decode.
#
# Requirements: as run.sh (Go toolchain, cgo, libwebp + pkg-config).
#
# Corpus selection: pass a name as the second argument or set CORPUS, and it
# resolves under testdata/. Set IMAGES_DIR to point somewhere else entirely.
#   photos      - the default: six JPEG photographs plus one PNG, all opaque
#   transparent - five PNGs with an alpha channel, flat-graphics content
# Usage: benchmark/run-decode.sh [budget_ms] [corpus]   (defaults 2000, photos)
set -euo pipefail

BUDGET_MS="${1:-2000}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
CORPUS="${2:-${CORPUS:-photos}}"
IMAGES_DIR="${IMAGES_DIR:-$REPO_ROOT/testdata/$CORPUS}"
[ -d "$IMAGES_DIR" ] || { echo "no such corpus: $IMAGES_DIR" >&2; exit 1; }
RESULTS="$(mktemp)"
trap 'rm -f "$RESULTS"' EXIT

cd "$REPO_ROOT"

echo ">> Go engines (ours + libwebp + wasm + x/image)..." >&2
( cd "$SCRIPT_DIR" && go run -tags testbenchmark,nodynamic ./webpbench \
  -dir "$IMAGES_DIR" -decode -budget-ms "$BUDGET_MS" ) >>"$RESULTS"


echo
echo "Decode results ($CORPUS corpus, budget ${BUDGET_MS}ms/measurement, inputs encoded by libwebp):"
echo
{
  echo "file,mode,engine,width,height,bytes,psnr_db,iters,ms_per_op"
  sort -t, -k3,3 -k2,2 -k1,1 "$RESULTS" \
    | awk -F, '{print $3","$2","$1","$4","$5","$6","$7","$8","$9}'
} | column -t -s,
