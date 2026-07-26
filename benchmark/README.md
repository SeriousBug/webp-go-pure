# WebP encoding benchmark

Compares this pure-Go library against other WebP encoders for output size and
encode speed. Re-runnable:

```sh
benchmark/run.sh [budget_ms]   # default budget 2000ms per measurement
```

## Engines

| Engine | What it is | cgo? |
| --- | --- | --- |
| `ours` | this pure-Go library | no |
| `libwebp` | the C reference, via [go-webp](https://github.com/kolesa-team/go-webp) | yes |
| `wasm` | libwebp compiled to WASM, via [gen2brain/webp](https://github.com/gen2brain/webp) | no |
| `webp-rust` | the Rust library this port is based on ([../webp-rust](https://github.com/mith-mmk/webp-rust)) | n/a (separate binary) |

`wasm` is the cgo-free reference point: same libwebp algorithm as `libwebp`, but
run through wazero instead of a C toolchain.

### ImageMagick

ImageMagick is **not** benchmarked separately: its WebP coder delegates to
libwebp (it lists `webp` as a required dependency and its `webp.so` coder links
libwebp). Benchmarking it would just measure the `libwebp` engine again plus
process-launch overhead, so it adds no new data point.

## Modes

Lossy quality is fixed at 90.

| Mode | ours | libwebp / wasm | webp-rust |
| --- | --- | --- | --- |
| `lossless` | `EncodeLossless` optimize 6 | lossless preset level 6 | `encode_lossless` optimize 6 |
| `lossy-fast` | `EncodeLossy` optimize 0 | method 0 (fastest) | `encode_lossy` optimize 0 |
| `lossy-slow` | `EncodeLossy` optimize 9 | method 6 (slowest) | `encode_lossy` optimize 9 |

The effort scales differ between codecs (our optimize is 0..9, libwebp method is
0..6), so `fast`/`slow` mark each codec's own endpoints rather than an identical
setting.

## Requirements

- Go toolchain.
- cgo + libwebp + pkg-config for the `libwebp` engine: `brew install webp pkg-config`.
- For the `webp-rust` engine: a `cargo` toolchain and `../webp-rust` checked out
  next to this repo. `run.sh` skips it automatically if either is missing.

The benchmark tooling and the cross-encoder compatibility tests live in their
own Go module (`benchmark/`, with `webpbench/` and `compat/`), so the cgo and
third-party encoder dependencies they need stay out of the root module — the
library itself has none. The `webpbench` command and the libwebp compatibility
tests are further gated behind the `testbenchmark` build tag (plus `nodynamic`,
which forces gen2brain/webp onto its WASM path instead of loading a system
libwebp).

## Method

Each measurement encodes the same image repeatedly until a per-measurement time
budget elapses, reporting mean ms/op and the output size. Encodes that already
exceed the budget in a single call are reported as one iteration. Timings are
wall-clock and machine-dependent; treat them as relative, not absolute.

For the lossy modes each engine's own output is decoded back and scored against
the pixels that engine was handed, reported as `psnr_db` over RGB (lossless is
exact, so it shows `-`). Size alone would rank an encoder that quantizes harder
as the winner, so the two columns have to be read together.

### Memory

`run.sh` also emits a peak-RSS table, and `benchmark/run-mem.sh` runs that pass
on its own. Peak RSS is process-wide, which is the only figure comparable across
a pure-Go encoder, a cgo one that allocates in C, and wazero's WASM linear
memory: Go heap statistics would miss two of the four engines. So each
measurement gets its own process that decodes the source image, encodes it once,
and reports its own `ru_maxrss`. Source bitmap and language runtime are included,
because an application encoding an image pays for those too.

`mib_per_mp` divides that peak by the image's megapixels, which makes the figures
comparable across test images of different sizes. It is not a constant per
engine: a fixed runtime floor spread over a small image inflates it.

## Results

See `benchmark/results.md` for captured runs on arm64 and amd64 (machines and
date noted there). Regenerate with `benchmark/run.sh`.

The figures in `results.md` are generated from the tables in that same file, so
they cannot drift from the numbers:

```sh
benchmark/chart/chart.go        # rewrites benchmark/charts/*.svg
```

It writes a light and a dark variant of each figure; `results.md` picks between
them with GitHub's `#gh-light-mode-only` / `#gh-dark-mode-only` anchors. The
first line of `chart.go` is a comment to Go and a command to `sh`, so running the
file is enough; its `-md` and `-out` defaults are relative to `benchmark/chart`,
so from `benchmark/` it is `go run ./chart -md results.md -out charts`.
