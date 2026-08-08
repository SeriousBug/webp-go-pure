# WebP benchmark

Compares this pure-Go library against other WebP codecs for output size, encode
speed and decode speed. Re-runnable:

```sh
benchmark/run.sh        [budget_ms]   # encoding, default budget 2000ms per measurement
benchmark/run-decode.sh [budget_ms]   # decoding
benchmark/run-sweep.sh  [budget_ms]   # encoding at every effort setting, default 1000ms
```

## Engines

| Engine | What it is | cgo? | Encode | Decode |
| --- | --- | --- | --- | --- |
| `ours` | this pure-Go library | no | yes | yes |
| `libwebp` | the C reference, via [go-webp](https://github.com/kolesa-team/go-webp) | yes | yes | yes |
| `wasm` | libwebp compiled to WASM, via [gen2brain/webp](https://github.com/gen2brain/webp) | no | yes | yes |
| `nativewebp` | [HugoSmits86/nativewebp](https://github.com/HugoSmits86/nativewebp), another pure-Go encoder | no | lossless only | - |
| `x/image` | [golang.org/x/image/webp](https://pkg.go.dev/golang.org/x/image/webp), the Go project's own decoder | no | - | yes |

`wasm` is the cgo-free reference point: same libwebp algorithm as `libwebp`, but
run through wazero instead of a C toolchain. `x/image` is decode-only, so it
appears in the decode pass alone.

`nativewebp` writes VP8L and nothing else, so it has rows in the `lossless` mode
alone and none in the two lossy ones. It appears in no decode table either: its
`Decode` is a wrapper around `golang.org/x/image/webp`, which is already its own
engine here, so a `nativewebp` decode row would be a second measurement of the
same decoder. `benchmark/compat` does test that our decoder reproduces its
output exactly, which is the one direction there is to check.

### ImageMagick

ImageMagick is **not** benchmarked separately: its WebP coder delegates to
libwebp (it lists `webp` as a required dependency and its `webp.so` coder links
libwebp). Benchmarking it would just measure the `libwebp` engine again plus
process-launch overhead, so it adds no new data point.

## Modes

Lossy quality is fixed at 90.

| Mode | ours | libwebp / wasm | nativewebp |
| --- | --- | --- | --- |
| `lossless` | `EncodeLossless` effort 6 | lossless preset level 6 | `BestCompression` (level 6) |
| `lossy-fast` | `EncodeLossy` effort 0 | method 0 (fastest) | - |
| `lossy-slow` | `EncodeLossy` effort 9 | method 6 (slowest) | - |

The effort scales differ between codecs (our effort is 0..9, libwebp method is
0..6), so `fast`/`slow` mark each codec's own endpoints rather than an identical
setting. The effort sweep below covers everything in between.

The decode pass has two modes, `lossless` and `lossy`, named for the file being
decoded rather than for any decoder setting. Every engine is handed the same
file: libwebp encodes each test image once, at lossless level 6 and at lossy
quality 90 method 6. Letting each engine decode its own encoder's output would
measure the encoder too, and libwebp is the fastest encoder here, so it is also
the cheapest way to produce the inputs.

## The effort sweep

Fixed modes make an encoder look like a point when it is a curve: an encoder that
is fast because it searches less can be made to look fast against one that was
asked to search hard. `benchmark/run-sweep.sh` walks every effort setting each
engine exposes, in `lossless` and `lossy` (quality 90), and reports time and
output size at each:

| Engine | knob | range |
| --- | --- | --- |
| `ours` | `Effort` | lossy 0..9, lossless 0..6 |
| `libwebp` | lossy `method` / lossless preset level (`cwebp -z`) | 0..6 / 0..9 |
| `wasm` | `Method` | 0..6, both modes: gen2brain/webp exposes no lossless level |
| `nativewebp` | `CompressionLevel` | 0, 4, 6: the three its `getMethodLevel` distinguishes |

The numbers are each engine's own scale and do not line up across engines, which
is why the figure plots time against size and puts the effort number on the point
rather than on an axis: pick the tradeoff you want, then read the setting that
gets it.

## Requirements

- Go toolchain.
- cgo + libwebp + pkg-config for the `libwebp` engine: `brew install webp pkg-config`.

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

### Decoding

The decode pass times the same loop over `run-decode.sh`'s inputs, and every
engine has to end at packed 8-bit RGBA. That conversion is inside the timed
call, because the engines do not return the same thing: `libwebp` hands back an
`*image.NRGBA`, but `x/image` returns an `*image.YCbCr` for lossy files and
`wasm` an `*image.NYCbCrA` for everything, and a decoder that stops at YCbCr
planes has not done the color conversion the others already paid for. So the
figure is what an application pays to get pixels it can index, not what the
decode call alone costs.

`psnr_db` here scores each engine against libwebp's decode of the same file, and
reads `-` when the two agree pixel for pixel. A number means two decoders
resolved one file to different pixels: the YCbCr-returning engines land there,
because converting their planes through the standard library treats limited-range
samples as full-range ones.

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
