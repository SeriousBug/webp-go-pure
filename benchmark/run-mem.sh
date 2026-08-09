#!/usr/bin/env bash
# Peak-RSS pass only: the memory half of run.sh, for re-capturing the memory
# table without paying for the timing table again.
#
# Corpus selection: pass a name as the second argument or set CORPUS, and it
# resolves under testdata/. Set IMAGES_DIR to point somewhere else entirely.
#   photos      - the default: six JPEG photographs plus one PNG, all opaque
#   transparent - five PNGs with an alpha channel, flat-graphics content
# Usage: benchmark/run-mem.sh [corpus]   (default photos)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
CORPUS="${1:-${CORPUS:-photos}}"
IMAGES_DIR="${IMAGES_DIR:-$REPO_ROOT/testdata/$CORPUS}"
[ -d "$IMAGES_DIR" ] || { echo "no such corpus: $IMAGES_DIR" >&2; exit 1; }
MEM="$(mktemp)"
trap 'rm -f "$MEM"' EXIT

cd "$REPO_ROOT"

echo ">> Go engines, peak RSS..." >&2
( cd "$SCRIPT_DIR" && go run -tags testbenchmark,nodynamic ./webpbench \
  -dir "$IMAGES_DIR" -mem ) >>"$MEM"


echo
echo "Peak RSS ($CORPUS corpus, one encode per process, quality 90 for lossy):"
echo
{
  echo "file,mode,engine,width,height,megapixels,peak_rss_mib,mib_per_mp"
  sort -t, -k3,3 -k2,2 -k1,1 "$MEM" \
    | awk -F, '{print $3","$2","$1","$4","$5","$6","$7","$8}'
} | column -t -s,
