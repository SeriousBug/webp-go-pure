#!/usr/bin/env bash
# Benchmark WebP decoding across engines:
#   ours     - this pure-Go library
#   libwebp  - the C reference, via github.com/kolesa-team/go-webp (cgo)
#   wasm     - libwebp compiled to WASM, via github.com/gen2brain/webp (cgo-free)
#   x/image  - golang.org/x/image/webp, the Go project's own decoder
#   webp-rust- the Rust library this port is based on (../webp-rust), if present
#
# Every engine decodes the same files, encoded once by libwebp: lossless at
# level 6, lossy at quality 90. Reports mean decode time and each engine's
# agreement with libwebp's own decode.
#
# Requirements: as run.sh (Go toolchain, cgo, libwebp + pkg-config; cargo and
# ../webp-rust for the Rust engine).
#
# Usage: benchmark/run-decode.sh [budget_ms]   (default 2000)
set -euo pipefail

BUDGET_MS="${1:-2000}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
IMAGES_DIR="$REPO_ROOT/testdata/photos"
RESULTS="$(mktemp)"
# The encoded inputs and libwebp's decode of them, so the Rust engine measures
# the same bytes and scores against the same reference as the Go engines.
INPUTS="$(mktemp -d)"
trap 'rm -rf "$RESULTS" "$INPUTS"' EXIT

cd "$REPO_ROOT"

echo ">> Go engines (ours + libwebp + wasm + x/image)..." >&2
( cd "$SCRIPT_DIR" && go run -tags testbenchmark,nodynamic ./webpbench \
  -dir "$IMAGES_DIR" -decode -decode-dir "$INPUTS" -budget-ms "$BUDGET_MS" ) >>"$RESULTS"

RUST_DIR="$REPO_ROOT/../webp-rust"
if command -v cargo >/dev/null 2>&1 && [ -d "$RUST_DIR" ]; then
  echo ">> Rust engine (webp-rust)..." >&2
  ( cd "$SCRIPT_DIR/rustbench" && cargo build --release --quiet )
  "$SCRIPT_DIR/rustbench/target/release/rustbench" "$INPUTS" --decode "$BUDGET_MS" >>"$RESULTS"
else
  echo ">> Skipping webp-rust engine (need cargo and $RUST_DIR)." >&2
fi

echo
echo "Decode results (budget ${BUDGET_MS}ms/measurement, inputs encoded by libwebp):"
echo
{
  echo "file,mode,engine,width,height,bytes,psnr_db,iters,ms_per_op"
  sort -t, -k3,3 -k2,2 -k1,1 "$RESULTS" \
    | awk -F, '{print $3","$2","$1","$4","$5","$6","$7","$8","$9}'
} | column -t -s,
