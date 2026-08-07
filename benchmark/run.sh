#!/usr/bin/env bash
# Benchmark WebP encoding across engines:
#   ours     - this pure-Go library
#   libwebp  - the C reference, via github.com/kolesa-team/go-webp (cgo)
#   wasm     - libwebp compiled to WASM, via github.com/gen2brain/webp (cgo-free)
#   nativewebp - github.com/HugoSmits86/nativewebp, a pure-Go VP8L encoder
#
# Modes: lossless, lossy-fast (high speed), lossy-slow (slowest).
# Reports output size and mean encode time; fast encoders loop until a time
# budget is met so timings are stable.
#
# Requirements: Go toolchain, cgo, libwebp + pkg-config (brew install webp pkg-config).
#
# Usage: benchmark/run.sh [budget_ms]   (default 2000)
set -euo pipefail

BUDGET_MS="${1:-2000}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
IMAGES_DIR="$REPO_ROOT/testdata/photos"
RESULTS="$(mktemp)"
MEM="$(mktemp)"
trap 'rm -f "$RESULTS" "$MEM"' EXIT

cd "$REPO_ROOT"

echo ">> Go engines (ours + libwebp + wasm)..." >&2
# webpbench lives in the nested benchmark module (it pulls in cgo/libwebp and
# other encoders, kept out of the root module). Run it from there.
( cd "$SCRIPT_DIR" && go run -tags testbenchmark,nodynamic ./webpbench \
  -dir "$IMAGES_DIR" -budget-ms "$BUDGET_MS" ) >>"$RESULTS"

echo ">> Go engines, peak RSS..." >&2
( cd "$SCRIPT_DIR" && go run -tags testbenchmark,nodynamic ./webpbench \
  -dir "$IMAGES_DIR" -mem ) >>"$MEM"

echo
echo "Results (budget ${BUDGET_MS}ms/measurement, quality 90 for lossy):"
echo
{
  echo "file,mode,engine,width,height,bytes,psnr_db,iters,ms_per_op"
  sort -t, -k3,3 -k2,2 -k1,1 "$RESULTS" \
    | awk -F, '{print $3","$2","$1","$4","$5","$6","$7","$8","$9}'
} | column -t -s,

echo
echo "Decoding is a separate pass: benchmark/run-decode.sh"
echo "The effort sweep is another: benchmark/run-sweep.sh"

echo
echo "Peak RSS (one encode per process, same settings):"
echo
{
  echo "file,mode,engine,width,height,megapixels,peak_rss_mib,mib_per_mp"
  sort -t, -k3,3 -k2,2 -k1,1 "$MEM" \
    | awk -F, '{print $3","$2","$1","$4","$5","$6","$7","$8}'
} | column -t -s,
