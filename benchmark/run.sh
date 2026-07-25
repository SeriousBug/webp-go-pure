#!/usr/bin/env bash
# Benchmark WebP encoding across engines:
#   ours     - this pure-Go library
#   libwebp  - the C reference, via github.com/kolesa-team/go-webp (cgo)
#   wasm     - libwebp compiled to WASM, via github.com/gen2brain/webp (cgo-free)
#   webp-rust- the Rust library this port is based on (../webp-rust), if present
#
# Modes: lossless, lossy-fast (high speed), lossy-slow (slowest).
# Reports output size and mean encode time; fast encoders loop until a time
# budget is met so timings are stable.
#
# Requirements:
#   - Go toolchain, cgo, libwebp + pkg-config (brew install webp pkg-config)
#   - For the webp-rust engine: a cargo toolchain and ../webp-rust checked out
#
# Usage: benchmark/run.sh [budget_ms]   (default 2000)
set -euo pipefail

BUDGET_MS="${1:-2000}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
IMAGES_DIR="$REPO_ROOT/testdata/photos"
RESULTS="$(mktemp)"
trap 'rm -f "$RESULTS"' EXIT

cd "$REPO_ROOT"

echo ">> Go engines (ours + libwebp + wasm)..." >&2
# webpbench lives in the nested benchmark module (it pulls in cgo/libwebp and
# other encoders, kept out of the root module). Run it from there.
( cd "$SCRIPT_DIR" && go run -tags testbenchmark,nodynamic ./webpbench \
  -dir "$IMAGES_DIR" -budget-ms "$BUDGET_MS" ) >>"$RESULTS"

RUST_DIR="$REPO_ROOT/../webp-rust"
if command -v cargo >/dev/null 2>&1 && [ -d "$RUST_DIR" ]; then
  echo ">> Rust engine (webp-rust)..." >&2
  ( cd "$SCRIPT_DIR/rustbench" && cargo build --release --quiet )
  "$SCRIPT_DIR/rustbench/target/release/rustbench" "$IMAGES_DIR" "$BUDGET_MS" >>"$RESULTS"
else
  echo ">> Skipping webp-rust engine (need cargo and $RUST_DIR)." >&2
fi

echo
echo "Results (budget ${BUDGET_MS}ms/measurement, quality 90 for lossy):"
echo
{
  echo "file,mode,engine,width,height,bytes,psnr_db,iters,ms_per_op"
  sort -t, -k3,3 -k2,2 -k1,1 "$RESULTS" \
    | awk -F, '{print $3","$2","$1","$4","$5","$6","$7","$8","$9}'
} | column -t -s,
