#!/usr/bin/env bash
# Sweep every effort setting each engine exposes, in both modes:
#   ours       - this pure-Go library, Effort 0..9
#   libwebp    - the C reference (cgo): lossy method 0..6, lossless preset level 0..9
#   wasm       - libwebp via WASM, cgo-free: method 0..6 in both modes
#   nativewebp - the other pure-Go encoder: compression level 0, 4, 6; lossless only
#
# Reports output size and mean encode time at each setting, so an engine reads as
# a time-for-size curve instead of a single point. Lossy quality is 90.
#
# Requirements: Go toolchain, cgo, libwebp + pkg-config (brew install webp pkg-config).
#
# Usage: benchmark/run-sweep.sh [budget_ms]   (default 1000)
set -euo pipefail

BUDGET_MS="${1:-1000}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
IMAGES_DIR="$REPO_ROOT/testdata/photos"
RESULTS="$(mktemp)"
trap 'rm -f "$RESULTS"' EXIT

cd "$REPO_ROOT"

echo ">> Effort sweep (ours + libwebp + wasm + nativewebp)..." >&2
( cd "$SCRIPT_DIR" && go run -tags testbenchmark,nodynamic ./webpbench \
  -dir "$IMAGES_DIR" -sweep -budget-ms "$BUDGET_MS" ) >>"$RESULTS"

echo
echo "Effort sweep (budget ${BUDGET_MS}ms/measurement, quality 90 for lossy):"
echo
{
  echo "file,mode,engine,effort,width,height,bytes,psnr_db,iters,ms_per_op"
  sort -t, -k4,4 -k2,2 -k1,1 -k3,3n "$RESULTS" \
    | awk -F, '{print $4","$2","$1","$3","$5","$6","$7","$8","$9","$10}'
} | column -t -s,
