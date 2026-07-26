#!/usr/bin/env bash
# Peak-RSS pass only: the memory half of run.sh, for re-capturing the memory
# table without paying for the timing table again.
#
# Usage: benchmark/run-mem.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
IMAGES_DIR="$REPO_ROOT/testdata/photos"
MEM="$(mktemp)"
trap 'rm -f "$MEM"' EXIT

cd "$REPO_ROOT"

echo ">> Go engines, peak RSS..." >&2
( cd "$SCRIPT_DIR" && go run -tags testbenchmark,nodynamic ./webpbench \
  -dir "$IMAGES_DIR" -mem ) >>"$MEM"

RUST_DIR="$REPO_ROOT/../webp-rust"
if command -v cargo >/dev/null 2>&1 && [ -d "$RUST_DIR" ]; then
  echo ">> Rust engine, peak RSS..." >&2
  ( cd "$SCRIPT_DIR/rustbench" && cargo build --release --quiet )
  "$SCRIPT_DIR/rustbench/target/release/rustbench" "$IMAGES_DIR" --mem >>"$MEM"
else
  echo ">> Skipping webp-rust engine (need cargo and $RUST_DIR)." >&2
fi

echo
echo "Peak RSS (one encode per process, quality 90 for lossy):"
echo
{
  echo "file,mode,engine,width,height,megapixels,peak_rss_mib,mib_per_mp"
  sort -t, -k3,3 -k2,2 -k1,1 "$MEM" \
    | awk -F, '{print $3","$2","$1","$4","$5","$6","$7","$8}'
} | column -t -s,
