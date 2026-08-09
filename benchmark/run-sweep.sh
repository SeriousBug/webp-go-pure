#!/usr/bin/env bash
# Sweep every effort setting each engine exposes, in both modes:
#   ours       - this pure-Go library, Effort 0..9 lossy, 0..6 lossless
#   libwebp    - the C reference (cgo): lossy method 0..6, lossless preset level 0..9
#   wasm       - libwebp via WASM, cgo-free: method 0..6 in both modes
#   nativewebp - the other pure-Go encoder: compression level 0, 4, 6; lossless only
#
# Reports output size and mean encode time at each setting, so an engine reads as
# a time-for-size curve instead of a single point. Lossy quality is 90.
#
# Requirements: Go toolchain, cgo, libwebp + pkg-config (brew install webp pkg-config).
#
# Corpus selection: pass a name as the second argument or set CORPUS, and it
# resolves under testdata/. Set IMAGES_DIR to point somewhere else entirely.
#   photos      - the default: six JPEG photographs plus one PNG, all opaque
#   transparent - five PNGs with an alpha channel, flat-graphics content
# Usage: benchmark/run-sweep.sh [budget_ms] [corpus]   (defaults 1000, photos)
set -euo pipefail

BUDGET_MS="${1:-1000}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
CORPUS="${2:-${CORPUS:-photos}}"
IMAGES_DIR="${IMAGES_DIR:-$REPO_ROOT/testdata/$CORPUS}"
[ -d "$IMAGES_DIR" ] || { echo "no such corpus: $IMAGES_DIR" >&2; exit 1; }
RESULTS="$(mktemp)"
trap 'rm -f "$RESULTS"' EXIT

cd "$REPO_ROOT"

echo ">> Effort sweep (ours + libwebp + wasm + nativewebp)..." >&2
( cd "$SCRIPT_DIR" && go run -tags testbenchmark,nodynamic ./webpbench \
  -dir "$IMAGES_DIR" -sweep -budget-ms "$BUDGET_MS" ) >>"$RESULTS"

echo
echo "Effort sweep ($CORPUS corpus, budget ${BUDGET_MS}ms/measurement, quality 90 for lossy):"
echo
{
  echo "file,mode,engine,effort,width,height,bytes,psnr_db,iters,ms_per_op"
  sort -t, -k4,4 -k2,2 -k1,1 -k3,3n "$RESULTS" \
    | awk -F, '{print $4","$2","$1","$3","$5","$6","$7","$8","$9","$10}'
} | column -t -s,
