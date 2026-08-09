# Benchmark results

Captured runs of `benchmark/run.sh`, `benchmark/run-mem.sh`, `benchmark/run-sweep.sh`
and `benchmark/run-decode.sh` on two machines, over two corpora. Timings are
machine-dependent; regenerate locally for your own hardware.

| | arm64 | amd64 |
| --- | --- | --- |
| **CPU** | Apple M4 Pro (14 cores) | AMD Ryzen 7 5700G (16 threads) |
| **OS** | macOS 26.5.1 | Arch Linux, kernel 7.1.3 |
| **Go** | go1.26.5 | go1.26.5 |

- **Library commit:** 6f1dd6a · captured 2026-08-09
- **libwebp:** 1.6.0 on both, via pkg-config (also used by the `wasm` engine, via WASM)
- **Other engines:** nativewebp v1.3.0, gen2brain/webp v0.6.4, kolesa-team/go-webp v1.0.5, golang.org/x/image v0.44.0
- **Budgets:** 2000 ms/measurement for the encode and decode passes, 1000 ms for
  the effort sweep; lossy quality 90 throughout
- **Peak RSS:** one encode per process, no time budget

Every table below is one capture at that commit, so the encode, memory, sweep and
decode passes describe the same code. Engines: `ours` (pure Go), `libwebp` (C,
cgo), `wasm` (libwebp via WASM, cgo-free), `nativewebp`
(HugoSmits86/nativewebp, pure Go, lossless only), and in the decode tables
`x/image` (golang.org/x/image/webp, decode only). See `README.md` for the exact
mode settings.

## The two corpora

**`photos`** is `testdata/photos`: six JPEG photographs and one PNG, 0.81 to
6.29 megapixels, all fully opaque. It is what the earlier captures used, and it
is the corpus where every engine spends its time on natural-image texture.

**`transparent`** is `testdata/transparent`: five PNGs with an alpha channel,
0.05 to 0.48 megapixels, taken unmodified from Google's WebP
[Lossless and Alpha gallery](https://developers.google.com/speed/webp/gallery2)
(`gstatic.com/webp/gallery3/`). The content is flat graphics rather than
photographs, and 18% to 75% of each image is fully transparent, with one image
81% partially transparent. `testdata/transparent/ATTRIBUTION.md` has the titles,
authors, licenses and per-image alpha coverage.

The alpha corpus is here because it is the content our lossless encoder used to
lose on, and because our lossy encoder refused it outright until this branch:
lossy encoding now stores alpha in an `ALPH` chunk, so the `lossy-fast` and
`lossy-slow` rows on that corpus exist for the first time.

## Method

Every engine is handed the same buffer: straight, non-premultiplied RGBA,
un-premultiplied through `image/draw` when the source decodes to a premultiplied
type. That matters only on the alpha corpus, where a premultiplied buffer would
hand every engine black under the transparent regions, which is both wrong and
easier to compress than the real image.

`psnr_db` decodes each encoder's own output and scores it against the pixels that
encoder was handed, over RGB, weighting each pixel's squared error by its source
alpha. Higher is better, and quality 90 lands near 40 dB. Lossless is exact, so
it shows `-`.

The alpha weighting is what makes the column meaningful on the alpha corpus. RGB
under a fully transparent pixel is invisible, and libwebp rewrites it to shrink
the VP8 chunk; scoring it compares bytes no viewer can see, and an unweighted
metric reports libwebp at 12-26 dB where a viewer sees no difference at all. On a
fully opaque image every weight is 1, so the photos figures are bit-identical to
what an unweighted RGB PSNR gives.

Size without quality is not comparable: an encoder can always make a smaller file
by quantizing harder, so the two columns have to be read together.

Each machine and corpus also has a peak-RSS table. Those measurements run one
encode per process and report that process's own `ru_maxrss`, so the figure is
what an application pays to encode one image: source bitmap, language runtime and
encoder together. `mib_per_mp` divides it by the image's megapixels so images of
different sizes are comparable; a 1080p frame is 2.1 MP, a 4K frame roughly
10 MP, and a phone photo about 12 MP, so multiply that column by those to size a
workload. The alpha corpus images are small enough (0.05-0.48 MP) that the Go
runtime's own floor dominates their per-megapixel figures, so read that corpus's
memory table as MiB, not as MiB per megapixel.

## Output is byte-identical on both machines

Every `bytes` column below matches between the arm64 and amd64 sections, in all
732 encoder rows of this capture: 4 engines over 2 corpora in the fixed-mode
tables, and every effort setting of every engine in the sweep tables. That is
checked mechanically over the two machines' captures rather than asserted.

Our own output used to differ between the two: on the alpha corpus three of five
images encoded lossy came out different sizes, because arm64 fuses `a - b*c` into
one instruction and keeps precision amd64 rounds away, so near-equal encoder
candidates ranked differently. The cost products are now rounded explicitly and
the two architectures agree. Since `psnr_db` scores each encoder's own output, it
matches between the machines too, and only the timing and memory columns differ.

## Reading these numbers: the photos corpus

- **vs libwebp, lossless: level on size, and the time gap now depends on the
  machine.** Per image our files run 0.96-1.04x libwebp's, geometric mean
  1.005x; over the corpus 14.67 MiB against 14.66 MiB. The geometric mean of our
  per-image output is 1.660 MB against libwebp's 1.652 MB, 0.5% apart. On time we
  are level on arm64, 0.90-1.14x its time with a geometric mean of 0.99x, and
  behind on amd64 at 1.31-1.86x (1.47x).
- **vs libwebp, `lossy-slow`:** sizes land at 0.95-1.06x libwebp's (geometric
  mean 1.013x) within -0.25 to +0.22 dB, for 1.3-2.4x the encode time on arm64
  and 1.3-2.2x on amd64.
- **vs libwebp, `lossy-fast`:** quality lands within -0.43 to +0.26 dB, sizes run
  0.96-1.21x (geometric mean 1.053x), and we take 1.1-1.7x the time on arm64,
  1.4-1.8x on amd64. Effort 0 is still where our files are furthest behind on
  size.
- **vs `wasm`, lossless: smaller and roughly twice as fast.** 0.96-1.03x its
  size, geometric mean 0.998x, at 0.31-0.52x its time on arm64 and 0.29-0.50x on
  amd64. `wasm` exposes only libwebp's `Method` for lossless, not the preset
  level, which is why the same C encoder lands slightly larger there than in the
  `libwebp` rows.
- **vs `nativewebp`, lossless: 9-23% smaller files.** Geometric mean 0.835x its
  size, for 1.03-1.65x its time on arm64 and 0.95-1.35x on amd64 at the fixed
  settings. The effort sweep is the fairer comparison and it is not close; see
  below. `nativewebp` has no lossy mode, so the choice only exists for VP8L.
- **`libwebp` and `wasm` are the same encoder.** Their output is byte-identical
  at every setting, so their sizes and PSNR match exactly; `wasm` is the cgo-free
  option and runs 2.1-3.6x slower than native `libwebp` on arm64 and 2.8-4.8x on
  amd64 in the lossy modes, 1.8-3.7x and 2.8-5.9x on lossless.

On effort. The modes above are two points on each encoder's curve; `run-sweep.sh`
walks every setting. The figures below are totals over the seven photos, arm64
first and amd64 second, and the effort numbers are each engine's own scale.

- **Our lossless effort 0 beats `nativewebp` at every level it has, on both
  size and time.** 15.69 MiB in 1.3 s / 1.9 s against its best output of
  17.39 MiB in 8.1 s / 14.0 s: 9.8% smaller for a sixth of the time on arm64 and
  a seventh on amd64. Against its fastest level, 17.40 MiB in 7.3 s / 12.2 s, we
  are 9.8% smaller and 5.5x / 6.4x faster. Our efforts 0 through 5 clear its best
  setting on both axes on both machines.
- **`nativewebp`'s three levels are one point on this corpus.** Level 0 to level
  6 moves the total by 0.07% (17.40 to 17.39 MiB) for 11% more time on arm64 and
  15% on amd64. That is a property of the content, not of the encoder: on the
  alpha corpus the same three levels do move, by 2.9%. See that section.
- **Our lossless curve is useful from effort 0.** The whole ladder spans
  15.69 to 14.67 MiB, 6.5% end to end, for 1.3 s to 9.6 s on arm64 and 1.9 s to
  14.2 s on amd64. Effort 0 is a working setting rather than a placeholder,
  effort 2 is where the corpus drops under 15 MiB, and effort 6 is where it meets
  libwebp. Efforts 7 through 9 are accepted and behave as 6.
- **libwebp's lossless curve flattens at level 3**, 14.69 MiB in 5.9 s / 6.0 s.
  Levels 4 to 9 stay within 0.4% of it and are not monotonic: level 7 is its
  smallest at 14.63 MiB, level 8 comes back up to 14.67 MiB, and level 9 spends
  99 s / 102 s to land at 14.66 MiB. Our effort 6 is 0.03% above its level 6 and
  0.24% above its level 7.
- **Lossy: our curve sits inside libwebp's quality band at every setting.**
  We run 3.65 to 3.18 MiB at 42.80-43.11 dB across efforts 0 to 9, against
  libwebp's 3.43 to 3.12 MiB at 42.74-43.10 dB across methods 0 to 6. Our PSNR
  climbs with effort except for a dip at efforts 6 and 7, which trade 0.10 dB for
  a 5.8% smaller file than efforts 3 to 5.
- **We are close to libwebp on lossy, and slightly behind on the tradeoff.**
  Effort 8 is 3.18 MiB at 43.04 dB in 3.6 s / 4.4 s against libwebp method 6's
  3.12 MiB at 43.06 dB in 3.0 s / 4.2 s: 1.7% larger at the same quality, for
  1.22x the time on arm64 and 1.03x on amd64. Effort 9 writes the same total at
  43.11 dB for 5.7 s / 7.3 s. libwebp's method 3 is the setting to beat: 3.19 MiB
  at 43.10 dB in 1.5 s / 1.9 s, which our effort 9 undercuts by 0.5% on size at
  the same quality but for 3.7x the time.
- **Several of our lossy settings are aliases.** Efforts 1 and 2, 3 through 5,
  and 6 and 7 each write byte-identical output. Efforts 8 and 9 write the same
  number of bytes on every image but different files on five of the seven, at
  43.04 and 43.11 dB. Our lossless ladder has no aliases on this corpus.
- **`wasm` is libwebp's curve shifted right**, 2.4-3.9x on arm64 and 3.2-5.3x on
  amd64 across the lossy settings, at the same sizes. Its lossless knob is
  `Method` rather than the preset level, so its curve stops at 6 and never
  reaches libwebp's level 9, and its methods 5 and 6 are not monotonic in either
  time or size.

On memory:

- **Lossy, we are the lightest of the three Go options:** 0.73-0.95x libwebp's
  peak on arm64 and 0.82-0.93x on amd64, 15-18 MiB per megapixel on the
  geometric mean. On the 5.5+ MP images that is 69-96 MiB against libwebp's
  89-106 MiB. Encoding a 4K frame costs on the order of 150-180 MiB.
- **`wasm` costs 1.4-2.5x libwebp's peak in the lossy modes:** it carries a
  WebAssembly runtime and its own linear memory on top of the encode. That is the
  memory half of the cgo-free tradeoff, next to the 2.1-4.8x on time.
- **Lossless is the expensive mode, and the two cgo-free engines sit either side
  of libwebp.** We sit at 1.4-2.1x libwebp's peak, 74-77 MiB per megapixel on the
  geometric mean and up to 89: 389-446 MiB on the 5.5+ MP images against
  libwebp's 201-220 MiB. `wasm` needs 577-700 MiB on the same images, so we cost
  0.56-0.77x its peak. Lossless still costs about five times what our own lossy
  modes do, so it is the mode to check before encoding large images.
- **`nativewebp` is the lightest lossless encoder here**, at 0.43-0.63x our peak
  and slightly under libwebp's: 36 MiB per megapixel on the geometric mean,
  182-193 MiB on the 5.5+ MP images. Alongside its larger files and slower
  encodes, it is the memory-for-nothing-else corner of the pure-Go range.
- Per-megapixel figures run higher on small images, because a fixed runtime floor
  is spread over fewer pixels: in the lossy modes Lena at 0.81 MP reads about
  twice the per-megapixel cost of the 5.5 MP images.

On decoding. Every engine decodes the same libwebp-encoded file and has to end at
straight RGBA, so an engine that returns YCbCr planes pays for that conversion
inside the measurement, as an application would:

- **vs `x/image`, the other pure-Go decoder: we are ahead in every mode on both
  machines on the geometric mean.** Lossy 0.94x its time on arm64 and 0.91x on
  amd64; lossless 0.98x and 0.86x. Per image it goes both ways on arm64 lossless
  (0.78-1.16x), and is a consistent win everywhere else.
- **vs libwebp: 2.6-5.8x slower on lossy** (2.4-5.5x on amd64), and 1.5-2.2x
  (arm64) / 1.9-3.1x (amd64) on lossless.
- **vs `wasm`, the other cgo-free libwebp: we are faster on lossless**,
  0.39-0.83x its time on arm64 and 0.40-0.50x on amd64; lossy is a wash on arm64
  (0.93-1.16x) and ours on amd64 (0.77-0.86x).
- **The `psnr_db` column in the decode tables is not zero, and that is about the
  other engines' output format.** `gen2brain/webp` hands back `*image.NYCbCrA`
  even for a lossless VP8L file, so its RGB differs from libwebp's own decode of
  the same bytes, 25-32 dB. `x/image` is exact on lossless and shows the same
  YUV-to-RGB rounding difference on lossy. Our decode agrees with libwebp byte
  for byte in every row of every table here.

## Reading these numbers: the alpha corpus

This corpus is small, flat-shaded and mostly transparent, which is a different
encoding problem from the photos above. The images are 0.05-0.48 MP, so
per-megapixel memory and sub-millisecond decode times are dominated by fixed
costs; read the absolute columns here.

- **vs `nativewebp`, lossless: our effort 0 is smaller than anything it can
  produce, in a fraction of the time.** 525024 B in 0.035 s / 0.054 s against its
  best output of 527642 B in 0.183 s / 0.426 s, and against its fastest level's
  543504 B in 0.132 s / 0.248 s: 0.5% smaller than its best for a fifth to an
  eighth of the time, and 3.4% smaller than its fastest at 3.8x / 4.6x the speed.
  Our efforts 0 to 2 beat its fastest level on size and time at once on both
  machines, and efforts 0 to 4 beat its best level on both. At the fixed settings
  in the tables (our effort 6 against its BestCompression) we run between 12%
  smaller and 0.3% larger per image, 5% smaller over the corpus, for 1.11-1.60x
  the time on arm64 and 0.61-1.13x on amd64.
- **`nativewebp`'s compression level does move here**, unlike on photos: level 0
  writes 543504 B and levels 4 and 6 both write 527642 B, 2.9% smaller, for 38%
  more time on arm64 and 72% on amd64. Levels 4 and 6 are byte-identical, so it
  has two distinct points on this corpus rather than one.
- **vs libwebp, lossless: this is where we still lose.** Our effort 6 writes
  499796 B against libwebp's 472464 B at level 6, 5.8% larger, and 9.2% larger
  than its level 9's 457684 B. Per image we run 1.04-1.11x its size. We are
  faster than it here, 0.31-0.76x its time on arm64 and 0.52-1.16x on amd64,
  which is the opposite of the photos corpus.
- **Lossy with alpha works, and lands within half a decibel of libwebp.**
  `lossy-fast` writes 207870 B at 39.34 dB against libwebp's 207684 B at
  39.71 dB, a 0.1% size difference at 0.37 dB lower quality; per image we run
  0.91-1.09x its size within -0.74 to +0.30 dB. `lossy-slow` writes 171258 B at
  39.73 dB against 158018 B at 39.92 dB, 8.4% larger. These rows did not exist
  before this branch: lossy encoding returned `ErrLossyAlpha` on any input with
  an alpha channel.
- **At `lossy-fast` we are much the slower encoder**, 2.9-5.7x libwebp's time on
  arm64 and 3.9-10.4x on amd64, because at these image sizes our fixed per-encode
  work is a large share of a few milliseconds.
- **libwebp's lossy method 6 is extraordinarily slow on alpha content**, and it
  dominates the `lossy-slow` row. Over this corpus its methods 0 to 5 cost 0.03 s
  to 0.57 s and method 6 costs 10.19 s on arm64: 17.8x its own method 5 for 1.75%
  less output. On a single 400x301 image it is 12.5 ms at method 5 and 1645 ms at
  method 6, 131x. That is libwebp's exhaustive alpha-filter search, and it is why
  our `lossy-slow` reads as 0.08-0.29x its time on arm64 and 0.15-0.61x on amd64
  while our photos `lossy-slow` is 1.3-2.4x slower than it. Compared against
  libwebp's method 5 instead, we are the slower encoder here too: our effort 9
  costs 1.58 s / 2.36 s against its 0.57 s / 0.33 s.
- **Our lossy effort ladder has three alias pairs on this corpus:** 3 and 4, 6
  and 7, and 8 and 9 each write byte-identical output.
- **`wasm` carries the same method 6 cost and adds the WASM tax:** 16.8 s / 30.9 s
  for the corpus at method 6, against libwebp's 10.2 s / 7.3 s.

On memory, at these image sizes the Go runtime's floor is a large share of every
figure:

- Lossless, we peak at 18-37 MiB on arm64 and 14-42 MiB on amd64, against
  libwebp's 25-45 MiB and `nativewebp`'s 16-30 MiB. We are 0.49-1.12x libwebp's
  peak and 0.80-1.42x `nativewebp`'s, so on this corpus lossless costs us about
  what the C encoder costs.
- `lossy-fast` is the one mode on either corpus where we are heavier than libwebp,
  1.17-1.46x its peak: 14-40 MiB against its 12-27 MiB.
- `lossy-slow` we are lighter than libwebp on the geometric mean (0.47x arm64,
  0.77x amd64), though per image it ranges 0.33-1.48x.
- `wasm` is the heaviest in every mode, up to 228 MiB on a 0.48 MP image.

On decoding, every figure is 1-10 ms, so these compare ratios rather than costs:
we are 0.80-0.93x `x/image`'s time on the geometric mean, 0.44-0.99x `wasm`'s,
and 2.1-3.1x libwebp's.

## Charts

One set of figures per corpus, regenerated from the tables below with
`benchmark/chart/chart.go`.

### photos

[![Time vs file size at each effort level, photos corpus: one line per engine through its effort settings, with encode time on the x axis and output size or mean PSNR on the y axis, three panels per machine and settings that are off the size axis marked on the frame](charts/effort-sweep-photos-light.svg#gh-light-mode-only)](https://raw.githubusercontent.com/SeriousBug/webp-go-pure/main/benchmark/charts/effort-sweep-photos-light.svg)
[![Time vs file size at each effort level, photos corpus: one line per engine through its effort settings, with encode time on the x axis and output size or mean PSNR on the y axis, three panels per machine and settings that are off the size axis marked on the frame](charts/effort-sweep-photos-dark.svg#gh-dark-mode-only)](https://raw.githubusercontent.com/SeriousBug/webp-go-pure/main/benchmark/charts/effort-sweep-photos-dark.svg)

[![Size and quality against libwebp on the photos corpus: one point per test image, with output size relative to libwebp on the x axis and PSNR difference on the y axis, faceted by lossy mode](charts/rate-distortion-photos-light.svg#gh-light-mode-only)](https://raw.githubusercontent.com/SeriousBug/webp-go-pure/main/benchmark/charts/rate-distortion-photos-light.svg)
[![Size and quality against libwebp on the photos corpus: one point per test image, with output size relative to libwebp on the x axis and PSNR difference on the y axis, faceted by lossy mode](charts/rate-distortion-photos-dark.svg#gh-dark-mode-only)](https://raw.githubusercontent.com/SeriousBug/webp-go-pure/main/benchmark/charts/rate-distortion-photos-dark.svg)

[![Encode time per image for each engine on the photos corpus, one panel per mode and machine, with bars that run off the panel drawn fading out under an arrow](charts/encode-time-photos-light.svg#gh-light-mode-only)](https://raw.githubusercontent.com/SeriousBug/webp-go-pure/main/benchmark/charts/encode-time-photos-light.svg)
[![Encode time per image for each engine on the photos corpus, one panel per mode and machine, with bars that run off the panel drawn fading out under an arrow](charts/encode-time-photos-dark.svg#gh-dark-mode-only)](https://raw.githubusercontent.com/SeriousBug/webp-go-pure/main/benchmark/charts/encode-time-photos-dark.svg)

[![Decode time per image for each engine on the photos corpus, one panel per mode and machine, on the same bar layout as the encode time figure, with x/image added](charts/decode-time-photos-light.svg#gh-light-mode-only)](https://raw.githubusercontent.com/SeriousBug/webp-go-pure/main/benchmark/charts/decode-time-photos-light.svg)
[![Decode time per image for each engine on the photos corpus, one panel per mode and machine, on the same bar layout as the encode time figure, with x/image added](charts/decode-time-photos-dark.svg#gh-dark-mode-only)](https://raw.githubusercontent.com/SeriousBug/webp-go-pure/main/benchmark/charts/decode-time-photos-dark.svg)

[![Peak memory per megapixel for each engine on the photos corpus, one panel per mode and machine, on the same bar layout as the encode time figure](charts/peak-memory-photos-light.svg#gh-light-mode-only)](https://raw.githubusercontent.com/SeriousBug/webp-go-pure/main/benchmark/charts/peak-memory-photos-light.svg)
[![Peak memory per megapixel for each engine on the photos corpus, one panel per mode and machine, on the same bar layout as the encode time figure](charts/peak-memory-photos-dark.svg#gh-dark-mode-only)](https://raw.githubusercontent.com/SeriousBug/webp-go-pure/main/benchmark/charts/peak-memory-photos-dark.svg)

### transparent

[![Time vs file size at each effort level, alpha corpus: one line per engine through its effort settings, with encode time on the x axis and output size or mean PSNR on the y axis, three panels per machine](charts/effort-sweep-transparent-light.svg#gh-light-mode-only)](https://raw.githubusercontent.com/SeriousBug/webp-go-pure/main/benchmark/charts/effort-sweep-transparent-light.svg)
[![Time vs file size at each effort level, alpha corpus: one line per engine through its effort settings, with encode time on the x axis and output size or mean PSNR on the y axis, three panels per machine](charts/effort-sweep-transparent-dark.svg#gh-dark-mode-only)](https://raw.githubusercontent.com/SeriousBug/webp-go-pure/main/benchmark/charts/effort-sweep-transparent-dark.svg)

[![Size and quality against libwebp on the alpha corpus: one point per test image, with output size relative to libwebp on the x axis and PSNR difference on the y axis, faceted by lossy mode](charts/rate-distortion-transparent-light.svg#gh-light-mode-only)](https://raw.githubusercontent.com/SeriousBug/webp-go-pure/main/benchmark/charts/rate-distortion-transparent-light.svg)
[![Size and quality against libwebp on the alpha corpus: one point per test image, with output size relative to libwebp on the x axis and PSNR difference on the y axis, faceted by lossy mode](charts/rate-distortion-transparent-dark.svg#gh-dark-mode-only)](https://raw.githubusercontent.com/SeriousBug/webp-go-pure/main/benchmark/charts/rate-distortion-transparent-dark.svg)

[![Encode time per image for each engine on the alpha corpus, one panel per mode and machine](charts/encode-time-transparent-light.svg#gh-light-mode-only)](https://raw.githubusercontent.com/SeriousBug/webp-go-pure/main/benchmark/charts/encode-time-transparent-light.svg)
[![Encode time per image for each engine on the alpha corpus, one panel per mode and machine](charts/encode-time-transparent-dark.svg#gh-dark-mode-only)](https://raw.githubusercontent.com/SeriousBug/webp-go-pure/main/benchmark/charts/encode-time-transparent-dark.svg)

[![Decode time per image for each engine on the alpha corpus, one panel per mode and machine, with x/image added](charts/decode-time-transparent-light.svg#gh-light-mode-only)](https://raw.githubusercontent.com/SeriousBug/webp-go-pure/main/benchmark/charts/decode-time-transparent-light.svg)
[![Decode time per image for each engine on the alpha corpus, one panel per mode and machine, with x/image added](charts/decode-time-transparent-dark.svg#gh-dark-mode-only)](https://raw.githubusercontent.com/SeriousBug/webp-go-pure/main/benchmark/charts/decode-time-transparent-dark.svg)

[![Peak memory per megapixel for each engine on the alpha corpus, one panel per mode and machine](charts/peak-memory-transparent-light.svg#gh-light-mode-only)](https://raw.githubusercontent.com/SeriousBug/webp-go-pure/main/benchmark/charts/peak-memory-transparent-light.svg)
[![Peak memory per megapixel for each engine on the alpha corpus, one panel per mode and machine](charts/peak-memory-transparent-dark.svg#gh-dark-mode-only)](https://raw.githubusercontent.com/SeriousBug/webp-go-pure/main/benchmark/charts/peak-memory-transparent-dark.svg)

## arm64 (Apple M4 Pro) / photos

```
file                                            mode        engine      width  height  bytes    psnr_db  iters  ms_per_op
Lena_512.png                                    lossless    libwebp     900    900     627632   -        8      253.533
Lena_512.png                                    lossless    nativewebp  900    900     738176   -        12     175.339
Lena_512.png                                    lossless    ours        900    900     638220   -        7      289.563
Lena_512.png                                    lossless    wasm        900    900     622766   -        3      947.781
Lena_512.png                                    lossy-fast  libwebp     900    900     103980   41.04    127    15.823
Lena_512.png                                    lossy-fast  ours        900    900     113182   41.02    86     23.526
Lena_512.png                                    lossy-fast  wasm        900    900     103980   41.04    52     38.533
Lena_512.png                                    lossy-slow  libwebp     900    900     89968    40.94    26     77.407
Lena_512.png                                    lossy-slow  ours        900    900     95710    41.13    14     152.156
Lena_512.png                                    lossy-slow  wasm        900    900     89968    40.94    10     202.499
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    libwebp     2025   2700    3241976  -        1      2026.630
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    nativewebp  2025   2700    3926412  -        2      1858.233
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    ours        2025   2700    3269944  -        2      1915.724
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    wasm        2025   2700    3249798  -        1      3799.598
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  libwebp     2025   2700    598034   41.96    20     101.463
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  ours        2025   2700    723064   42.02    13     155.059
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  wasm        2025   2700    598034   41.96    8      250.180
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  libwebp     2025   2700    610518   42.58    3      683.570
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  ours        2025   2700    603030   42.33    2      1340.533
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  wasm        2025   2700    610518   42.58    2      1604.019
pexels-martin-alargent-1165956-5665465.jpg      lossless    libwebp     2025   2700    2943528  -        1      2064.820
pexels-martin-alargent-1165956-5665465.jpg      lossless    nativewebp  2025   2700    3563584  -        2      1551.386
pexels-martin-alargent-1165956-5665465.jpg      lossless    ours        2025   2700    2919664  -        2      1857.237
pexels-martin-alargent-1165956-5665465.jpg      lossless    wasm        2025   2700    2953798  -        1      3794.645
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  libwebp     2025   2700    726824   42.63    20     103.478
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  ours        2025   2700    756806   42.39    13     154.855
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  wasm        2025   2700    726824   42.63    9      247.782
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  libwebp     2025   2700    603264   42.75    4      573.667
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  ours        2025   2700    616898   42.97    2      1057.163
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  wasm        2025   2700    603264   42.75    2      1418.037
pexels-mavihnt-38213559.jpg                     lossless    libwebp     2560   1706    3482480  -        2      1543.373
pexels-mavihnt-38213559.jpg                     lossless    nativewebp  2560   1706    4009534  -        2      1359.342
pexels-mavihnt-38213559.jpg                     lossless    ours        2560   1706    3509912  -        2      1594.843
pexels-mavihnt-38213559.jpg                     lossless    wasm        2560   1706    3488584  -        1      3076.712
pexels-mavihnt-38213559.jpg                     lossy-fast  libwebp     2560   1706    983360   40.87    19     106.911
pexels-mavihnt-38213559.jpg                     lossy-fast  ours        2560   1706    1037210  40.99    12     174.881
pexels-mavihnt-38213559.jpg                     lossy-fast  wasm        2560   1706    983360   40.87    9      247.575
pexels-mavihnt-38213559.jpg                     lossy-slow  libwebp     2560   1706    925032   41.14    3      675.425
pexels-mavihnt-38213559.jpg                     lossy-slow  ours        2560   1706    948446   41.27    2      1320.511
pexels-mavihnt-38213559.jpg                     lossy-slow  wasm        2560   1706    925032   41.14    2      1594.611
pexels-steve-15267299.jpg                       lossless    libwebp     2095   3000    2104690  -        1      2181.929
pexels-steve-15267299.jpg                       lossless    nativewebp  2095   3000    2622528  -        2      1691.686
pexels-steve-15267299.jpg                       lossless    ours        2095   3000    2024034  -        1      2018.005
pexels-steve-15267299.jpg                       lossless    wasm        2095   3000    2119370  -        1      4021.217
pexels-steve-15267299.jpg                       lossy-fast  libwebp     2095   3000    284570   44.34    22     92.866
pexels-steve-15267299.jpg                       lossy-fast  ours        2095   3000    294606   43.91    18     112.331
pexels-steve-15267299.jpg                       lossy-fast  wasm        2095   3000    284570   44.34    9      239.480
pexels-steve-15267299.jpg                       lossy-slow  libwebp     2095   3000    247610   44.50    6      345.780
pexels-steve-15267299.jpg                       lossy-slow  ours        2095   3000    252782   44.41    3      815.733
pexels-steve-15267299.jpg                       lossy-slow  wasm        2095   3000    247610   44.50    2      1067.997
pexels-steve-29626041.jpg                       lossless    libwebp     2560   1440    283988   -        3      888.844
pexels-steve-29626041.jpg                       lossless    nativewebp  2560   1440    379050   -        3      673.404
pexels-steve-29626041.jpg                       lossless    ours        2560   1440    295168   -        3      837.171
pexels-steve-29626041.jpg                       lossless    wasm        2560   1440    296596   -        2      1837.396
pexels-steve-29626041.jpg                       lossy-fast  libwebp     2560   1440    47614    49.05    45     45.161
pexels-steve-29626041.jpg                       lossy-fast  ours        2560   1440    45686    49.31    42     48.726
pexels-steve-29626041.jpg                       lossy-fast  wasm        2560   1440    47614    49.05    16     125.997
pexels-steve-29626041.jpg                       lossy-slow  libwebp     2560   1440    38252    49.65    15     134.574
pexels-steve-29626041.jpg                       lossy-slow  ours        2560   1440    36254    49.62    9      233.561
pexels-steve-29626041.jpg                       lossy-slow  wasm        2560   1440    38252    49.65    5      485.381
pexels-toulouse-10807703.jpg                    lossless    libwebp     1400   2100    2688812  -        2      1074.822
pexels-toulouse-10807703.jpg                    lossless    nativewebp  1400   2100    2996214  -        3      698.914
pexels-toulouse-10807703.jpg                    lossless    ours        1400   2100    2721246  -        2      1103.893
pexels-toulouse-10807703.jpg                    lossless    wasm        1400   2100    2685806  -        1      2450.260
pexels-toulouse-10807703.jpg                    lossy-fast  libwebp     1400   2100    857060   39.84    26     79.965
pexels-toulouse-10807703.jpg                    lossy-fast  ours        1400   2100    854742   39.93    15     134.721
pexels-toulouse-10807703.jpg                    lossy-fast  wasm        1400   2100    857060   39.84    12     175.970
pexels-toulouse-10807703.jpg                    lossy-slow  libwebp     1400   2100    758466   39.84    4      520.292
pexels-toulouse-10807703.jpg                    lossy-slow  ours        1400   2100    776930   40.03    3      676.129
pexels-toulouse-10807703.jpg                    lossy-slow  wasm        1400   2100    758466   39.84    2      1094.640
```

Peak RSS, one encode per process:

```
file                                            mode        engine      width  height  megapixels  peak_rss_mib  mib_per_mp
Lena_512.png                                    lossless    libwebp     900    900     0.81        50.1          61.9
Lena_512.png                                    lossless    nativewebp  900    900     0.81        44.9          55.4
Lena_512.png                                    lossless    ours        900    900     0.81        71.5          88.3
Lena_512.png                                    lossless    wasm        900    900     0.81        116.8         144.2
Lena_512.png                                    lossy-fast  libwebp     900    900     0.81        25.2          31.1
Lena_512.png                                    lossy-fast  ours        900    900     0.81        24.0          29.6
Lena_512.png                                    lossy-fast  wasm        900    900     0.81        38.7          47.8
Lena_512.png                                    lossy-slow  libwebp     900    900     0.81        26.7          33.0
Lena_512.png                                    lossy-slow  ours        900    900     0.81        25.1          31.0
Lena_512.png                                    lossy-slow  wasm        900    900     0.81        49.1          60.6
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    libwebp     2025   2700    5.47        219.9         40.2
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    nativewebp  2025   2700    5.47        187.3         34.3
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    ours        2025   2700    5.47        409.7         74.9
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    wasm        2025   2700    5.47        700.0         128.0
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  libwebp     2025   2700    5.47        92.1          16.8
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  ours        2025   2700    5.47        68.8          12.6
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  wasm        2025   2700    5.47        197.8         36.2
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  libwebp     2025   2700    5.47        101.1         18.5
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  ours        2025   2700    5.47        70.2          12.8
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  wasm        2025   2700    5.47        198.1         36.2
pexels-martin-alargent-1165956-5665465.jpg      lossless    libwebp     2025   2700    5.47        210.4         38.5
pexels-martin-alargent-1165956-5665465.jpg      lossless    nativewebp  2025   2700    5.47        183.2         33.5
pexels-martin-alargent-1165956-5665465.jpg      lossless    ours        2025   2700    5.47        408.8         74.8
pexels-martin-alargent-1165956-5665465.jpg      lossless    wasm        2025   2700    5.47        700.1         128.0
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  libwebp     2025   2700    5.47        92.5          16.9
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  ours        2025   2700    5.47        68.7          12.6
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  wasm        2025   2700    5.47        198.2         36.2
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  libwebp     2025   2700    5.47        100.7         18.4
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  ours        2025   2700    5.47        74.7          13.7
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  wasm        2025   2700    5.47        198.2         36.2
pexels-mavihnt-38213559.jpg                     lossless    libwebp     2560   1706    4.37        185.6         42.5
pexels-mavihnt-38213559.jpg                     lossless    nativewebp  2560   1706    4.37        160.7         36.8
pexels-mavihnt-38213559.jpg                     lossless    ours        2560   1706    4.37        357.5         81.9
pexels-mavihnt-38213559.jpg                     lossless    wasm        2560   1706    4.37        563.2         129.0
pexels-mavihnt-38213559.jpg                     lossy-fast  libwebp     2560   1706    4.37        78.6          18.0
pexels-mavihnt-38213559.jpg                     lossy-fast  ours        2560   1706    4.37        57.1          13.1
pexels-mavihnt-38213559.jpg                     lossy-fast  wasm        2560   1706    4.37        160.9         36.8
pexels-mavihnt-38213559.jpg                     lossy-slow  libwebp     2560   1706    4.37        92.5          21.2
pexels-mavihnt-38213559.jpg                     lossy-slow  ours        2560   1706    4.37        57.5          13.2
pexels-mavihnt-38213559.jpg                     lossy-slow  wasm        2560   1706    4.37        225.1         51.5
pexels-steve-15267299.jpg                       lossless    libwebp     2095   3000    6.29        212.6         33.8
pexels-steve-15267299.jpg                       lossless    nativewebp  2095   3000    6.29        193.2         30.7
pexels-steve-15267299.jpg                       lossless    ours        2095   3000    6.29        446.3         71.0
pexels-steve-15267299.jpg                       lossless    wasm        2095   3000    6.29        577.4         91.9
pexels-steve-15267299.jpg                       lossy-fast  libwebp     2095   3000    6.29        102.4         16.3
pexels-steve-15267299.jpg                       lossy-fast  ours        2095   3000    6.29        77.4          12.3
pexels-steve-15267299.jpg                       lossy-fast  wasm        2095   3000    6.29        225.7         35.9
pexels-steve-15267299.jpg                       lossy-slow  libwebp     2095   3000    6.29        106.3         16.9
pexels-steve-15267299.jpg                       lossy-slow  ours        2095   3000    6.29        78.2          12.4
pexels-steve-15267299.jpg                       lossy-slow  wasm        2095   3000    6.29        225.9         35.9
pexels-steve-29626041.jpg                       lossless    libwebp     2560   1440    3.69        135.0         36.6
pexels-steve-29626041.jpg                       lossless    nativewebp  2560   1440    3.69        105.4         28.6
pexels-steve-29626041.jpg                       lossless    ours        2560   1440    3.69        246.8         66.9
pexels-steve-29626041.jpg                       lossless    wasm        2560   1440    3.69        344.5         93.5
pexels-steve-29626041.jpg                       lossy-fast  libwebp     2560   1440    3.69        64.3          17.5
pexels-steve-29626041.jpg                       lossy-fast  ours        2560   1440    3.69        49.8          13.5
pexels-steve-29626041.jpg                       lossy-fast  wasm        2560   1440    3.69        94.0          25.5
pexels-steve-29626041.jpg                       lossy-slow  libwebp     2560   1440    3.69        65.2          17.7
pexels-steve-29626041.jpg                       lossy-slow  ours        2560   1440    3.69        50.6          13.7
pexels-steve-29626041.jpg                       lossy-slow  wasm        2560   1440    3.69        137.5         37.3
pexels-toulouse-10807703.jpg                    lossless    libwebp     1400   2100    2.94        129.5         44.0
pexels-toulouse-10807703.jpg                    lossless    nativewebp  1400   2100    2.94        118.5         40.3
pexels-toulouse-10807703.jpg                    lossless    ours        1400   2100    2.94        235.6         80.1
pexels-toulouse-10807703.jpg                    lossless    wasm        1400   2100    2.94        384.5         130.8
pexels-toulouse-10807703.jpg                    lossy-fast  libwebp     1400   2100    2.94        56.3          19.1
pexels-toulouse-10807703.jpg                    lossy-fast  ours        1400   2100    2.94        42.1          14.3
pexels-toulouse-10807703.jpg                    lossy-fast  wasm        1400   2100    2.94        112.6         38.3
pexels-toulouse-10807703.jpg                    lossy-slow  libwebp     1400   2100    2.94        71.0          24.1
pexels-toulouse-10807703.jpg                    lossy-slow  ours        1400   2100    2.94        45.7          15.6
pexels-toulouse-10807703.jpg                    lossy-slow  wasm        1400   2100    2.94        156.2         53.1
```

Decode, one file per mode encoded by libwebp:

```
file                                            mode      engine   width  height  bytes    psnr_db  iters  ms_per_op
Lena_512.png                                    lossless  libwebp  900    900     627632   -        199    10.092
Lena_512.png                                    lossless  ours     900    900     627632   -        106    18.890
Lena_512.png                                    lossless  wasm     900    900     627632   28.40    82     24.780
Lena_512.png                                    lossless  x/image  900    900     627632   -        105    19.069
Lena_512.png                                    lossy     libwebp  900    900     89968    -        414    4.831
Lena_512.png                                    lossy     ours     900    900     89968    -        117    17.232
Lena_512.png                                    lossy     wasm     900    900     89968    28.34    127    15.799
Lena_512.png                                    lossy     x/image  900    900     89968    28.34    109    18.422
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp  2025   2700    3241976  -        36     56.858
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours     2025   2700    3241976  -        17     122.510
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm     2025   2700    3241976  29.06    14     148.507
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  x/image  2025   2700    3241976  -        20     105.260
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp  2025   2700    610518   -        59     34.014
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours     2025   2700    610518   -        17     123.812
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm     2025   2700    610518   29.09    18     115.493
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     x/image  2025   2700    610518   29.09    16     131.384
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp  2025   2700    2943528  -        35     57.600
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours     2025   2700    2943528  -        19     109.801
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm     2025   2700    2943528  30.36    14     150.735
pexels-martin-alargent-1165956-5665465.jpg      lossless  x/image  2025   2700    2943528  -        20     103.342
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp  2025   2700    603264   -        60     33.620
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours     2025   2700    603264   -        17     117.997
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm     2025   2700    603264   30.34    19     107.956
pexels-martin-alargent-1165956-5665465.jpg      lossy     x/image  2025   2700    603264   30.34    17     123.816
pexels-mavihnt-38213559.jpg                     lossless  libwebp  2560   1706    3482480  -        40     50.763
pexels-mavihnt-38213559.jpg                     lossless  ours     2560   1706    3482480  -        21     98.497
pexels-mavihnt-38213559.jpg                     lossless  wasm     2560   1706    3482480  27.55    16     127.622
pexels-mavihnt-38213559.jpg                     lossless  x/image  2560   1706    3482480  -        21     98.155
pexels-mavihnt-38213559.jpg                     lossy     libwebp  2560   1706    925032   -        48     42.444
pexels-mavihnt-38213559.jpg                     lossy     ours     2560   1706    925032   -        18     113.578
pexels-mavihnt-38213559.jpg                     lossy     wasm     2560   1706    925032   27.55    21     99.526
pexels-mavihnt-38213559.jpg                     lossy     x/image  2560   1706    925032   27.55    16     127.522
pexels-steve-15267299.jpg                       lossless  libwebp  2095   3000    2104690  -        35     57.619
pexels-steve-15267299.jpg                       lossless  ours     2095   3000    2104690  -        20     102.598
pexels-steve-15267299.jpg                       lossless  wasm     2095   3000    2104690  28.98    13     158.536
pexels-steve-15267299.jpg                       lossless  x/image  2095   3000    2104690  -        20     101.502
pexels-steve-15267299.jpg                       lossy     libwebp  2095   3000    247610   -        99     20.239
pexels-steve-15267299.jpg                       lossy     ours     2095   3000    247610   -        21     99.002
pexels-steve-15267299.jpg                       lossy     wasm     2095   3000    247610   28.98    22     93.874
pexels-steve-15267299.jpg                       lossy     x/image  2095   3000    247610   28.98    21     96.523
pexels-steve-29626041.jpg                       lossless  libwebp  2560   1440    283988   -        113    17.717
pexels-steve-29626041.jpg                       lossless  ours     2560   1440    283988   -        76     26.633
pexels-steve-29626041.jpg                       lossless  wasm     2560   1440    283988   32.17    30     68.123
pexels-steve-29626041.jpg                       lossless  x/image  2560   1440    283988   -        59     33.966
pexels-steve-29626041.jpg                       lossy     libwebp  2560   1440    38252    -        286    6.993
pexels-steve-29626041.jpg                       lossy     ours     2560   1440    38252    -        50     40.384
pexels-steve-29626041.jpg                       lossy     wasm     2560   1440    38252    32.10    47     43.268
pexels-steve-29626041.jpg                       lossy     x/image  2560   1440    38252    32.10    49     40.983
pexels-toulouse-10807703.jpg                    lossless  libwebp  1400   2100    2688812  -        56     36.242
pexels-toulouse-10807703.jpg                    lossless  ours     1400   2100    2688812  -        31     66.630
pexels-toulouse-10807703.jpg                    lossless  wasm     1400   2100    2688812  27.59    23     88.711
pexels-toulouse-10807703.jpg                    lossless  x/image  1400   2100    2688812  -        28     73.330
pexels-toulouse-10807703.jpg                    lossy     libwebp  1400   2100    758466   -        60     33.731
pexels-toulouse-10807703.jpg                    lossy     ours     1400   2100    758466   -        24     86.690
pexels-toulouse-10807703.jpg                    lossy     wasm     1400   2100    758466   27.54    27     74.622
pexels-toulouse-10807703.jpg                    lossy     x/image  1400   2100    758466   27.54    21     98.060
```

Effort sweep, every setting of every engine:

```
file                                            mode      engine      effort  width  height  bytes    psnr_db  iters  ms_per_op
Lena_512.png                                    lossless  libwebp     0       900    900     734910   -        48     21.153
Lena_512.png                                    lossless  libwebp     1       900    900     661194   -        10     100.832
Lena_512.png                                    lossless  libwebp     2       900    900     625874   -        7      151.246
Lena_512.png                                    lossless  libwebp     3       900    900     628862   -        6      176.130
Lena_512.png                                    lossless  libwebp     4       900    900     628612   -        6      188.536
Lena_512.png                                    lossless  libwebp     5       900    900     628612   -        6      188.606
Lena_512.png                                    lossless  libwebp     6       900    900     627632   -        4      258.423
Lena_512.png                                    lossless  libwebp     7       900    900     625628   -        3      356.897
Lena_512.png                                    lossless  libwebp     8       900    900     616842   -        2      749.370
Lena_512.png                                    lossless  libwebp     9       900    900     609124   -        1      3904.905
Lena_512.png                                    lossless  nativewebp  0       900    900     741562   -        7      150.897
Lena_512.png                                    lossless  nativewebp  4       900    900     739400   -        7      155.664
Lena_512.png                                    lossless  nativewebp  6       900    900     738176   -        6      169.032
Lena_512.png                                    lossless  ours        0       900    900     695082   -        24     41.737
Lena_512.png                                    lossless  ours        1       900    900     691746   -        20     50.741
Lena_512.png                                    lossless  ours        2       900    900     661658   -        8      125.521
Lena_512.png                                    lossless  ours        3       900    900     661658   -        7      147.346
Lena_512.png                                    lossless  ours        4       900    900     660788   -        6      174.619
Lena_512.png                                    lossless  ours        5       900    900     657842   -        5      236.833
Lena_512.png                                    lossless  ours        6       900    900     638220   -        4      291.468
Lena_512.png                                    lossless  wasm        0       900    900     731954   -        10     105.283
Lena_512.png                                    lossless  wasm        1       900    900     639584   -        3      407.193
Lena_512.png                                    lossless  wasm        2       900    900     627632   -        3      451.079
Lena_512.png                                    lossless  wasm        3       900    900     627632   -        3      451.138
Lena_512.png                                    lossless  wasm        4       900    900     627632   -        3      452.775
Lena_512.png                                    lossless  wasm        5       900    900     618010   -        2      989.044
Lena_512.png                                    lossless  wasm        6       900    900     622766   -        2      927.275
Lena_512.png                                    lossy     libwebp     0       900    900     103980   41.04    64     15.627
Lena_512.png                                    lossy     libwebp     1       900    900     102500   41.05    49     20.749
Lena_512.png                                    lossy     libwebp     2       900    900     94342    40.70    47     21.452
Lena_512.png                                    lossy     libwebp     3       900    900     91788    40.98    24     43.072
Lena_512.png                                    lossy     libwebp     4       900    900     92124    40.97    24     43.240
Lena_512.png                                    lossy     libwebp     5       900    900     91586    40.91    22     47.225
Lena_512.png                                    lossy     libwebp     6       900    900     89968    40.94    14     76.638
Lena_512.png                                    lossy     ours        0       900    900     113182   41.02    43     23.576
Lena_512.png                                    lossy     ours        1       900    900     111048   41.05    31     32.566
Lena_512.png                                    lossy     ours        2       900    900     111048   41.05    32     31.480
Lena_512.png                                    lossy     ours        3       900    900     101404   41.15    22     47.457
Lena_512.png                                    lossy     ours        4       900    900     101404   41.15    20     51.347
Lena_512.png                                    lossy     ours        5       900    900     101404   41.15    20     50.708
Lena_512.png                                    lossy     ours        6       900    900     96566    41.08    19     55.293
Lena_512.png                                    lossy     ours        7       900    900     96566    41.08    20     52.074
Lena_512.png                                    lossy     ours        8       900    900     95710    41.21    11     91.639
Lena_512.png                                    lossy     ours        9       900    900     95710    41.13    7      153.364
Lena_512.png                                    lossy     wasm        0       900    900     103980   41.04    27     38.420
Lena_512.png                                    lossy     wasm        1       900    900     102500   41.05    19     54.076
Lena_512.png                                    lossy     wasm        2       900    900     94342    40.70    18     57.221
Lena_512.png                                    lossy     wasm        3       900    900     91788    40.98    7      164.701
Lena_512.png                                    lossy     wasm        4       900    900     92124    40.97    7      165.165
Lena_512.png                                    lossy     wasm        5       900    900     91586    40.91    6      173.989
Lena_512.png                                    lossy     wasm        6       900    900     89968    40.94    6      198.033
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     0       2025   2700    3987548  -        6      173.237
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     1       2025   2700    3992712  -        2      672.803
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     2       2025   2700    3993532  -        2      865.839
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     3       2025   2700    3246280  -        1      1110.659
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     4       2025   2700    3246304  -        1      1259.283
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     5       2025   2700    3242976  -        1      1350.863
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     6       2025   2700    3241976  -        1      2039.541
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     7       2025   2700    3237862  -        1      3288.088
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     8       2025   2700    3246580  -        1      3628.143
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     9       2025   2700    3245966  -        1      21005.392
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  nativewebp  0       2025   2700    3944492  -        1      1746.525
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  nativewebp  4       2025   2700    3938686  -        1      1721.931
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  nativewebp  6       2025   2700    3926412  -        1      1857.674
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        0       2025   2700    3496516  -        4      261.337
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        1       2025   2700    3486442  -        4      315.996
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        2       2025   2700    3398814  -        2      760.689
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        3       2025   2700    3288396  -        2      897.183
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        4       2025   2700    3289656  -        1      1101.391
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        5       2025   2700    3289656  -        1      1462.664
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        6       2025   2700    3269944  -        1      1874.951
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        0       2025   2700    3961206  -        1      1387.089
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        1       2025   2700    3244586  -        1      3278.143
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        2       2025   2700    3244586  -        1      3234.380
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        3       2025   2700    3244586  -        1      3575.868
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        4       2025   2700    3241976  -        1      3406.912
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        5       2025   2700    3249798  -        1      4014.553
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        6       2025   2700    3249798  -        1      3833.453
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     0       2025   2700    598034   41.96    11     99.965
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     1       2025   2700    588954   41.97    8      131.214
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     2       2025   2700    614528   42.20    7      156.631
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     3       2025   2700    627994   42.39    4      326.648
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     4       2025   2700    632636   42.42    4      328.056
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     5       2025   2700    625540   42.27    3      370.722
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     6       2025   2700    610518   42.58    2      677.907
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        0       2025   2700    723064   42.02    7      152.242
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        1       2025   2700    694902   42.09    5      230.933
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        2       2025   2700    694902   42.09    5      231.310
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        3       2025   2700    686064   42.28    3      407.500
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        4       2025   2700    686064   42.28    3      408.731
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        5       2025   2700    686064   42.28    3      409.267
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        6       2025   2700    611704   42.07    3      446.474
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        7       2025   2700    611704   42.07    3      446.785
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        8       2025   2700    603030   42.33    2      794.681
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        9       2025   2700    603030   42.33    1      1320.488
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        0       2025   2700    598034   41.96    5      246.334
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        1       2025   2700    588954   41.97    3      356.775
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        2       2025   2700    614528   42.20    3      409.486
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        3       2025   2700    627994   42.39    1      1210.492
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        4       2025   2700    632636   42.42    1      1195.344
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        5       2025   2700    625540   42.27    1      1289.299
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        6       2025   2700    610518   42.58    1      1627.862
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     0       2025   2700    3673696  -        6      188.867
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     1       2025   2700    3494146  -        2      763.844
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     2       2025   2700    3493206  -        2      994.793
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     3       2025   2700    2949858  -        1      1178.923
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     4       2025   2700    2950040  -        1      1321.249
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     5       2025   2700    2943056  -        1      1436.201
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     6       2025   2700    2943528  -        1      2100.583
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     7       2025   2700    2929290  -        1      2953.853
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     8       2025   2700    2938176  -        1      3522.658
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     9       2025   2700    2940364  -        1      24920.758
pexels-martin-alargent-1165956-5665465.jpg      lossless  nativewebp  0       2025   2700    3554880  -        1      1389.277
pexels-martin-alargent-1165956-5665465.jpg      lossless  nativewebp  4       2025   2700    3559008  -        1      1417.637
pexels-martin-alargent-1165956-5665465.jpg      lossless  nativewebp  6       2025   2700    3563584  -        1      1662.180
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        0       2025   2700    3178224  -        4      258.738
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        1       2025   2700    3178224  -        4      315.580
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        2       2025   2700    2936002  -        2      773.257
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        3       2025   2700    2936002  -        2      886.235
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        4       2025   2700    2936002  -        1      1076.355
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        5       2025   2700    2936002  -        1      1445.468
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        6       2025   2700    2919664  -        1      1877.613
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        0       2025   2700    3625218  -        1      1329.521
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        1       2025   2700    2948150  -        1      3169.274
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        2       2025   2700    2948150  -        1      3174.849
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        3       2025   2700    2948150  -        1      3117.918
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        4       2025   2700    2943528  -        1      3445.828
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        5       2025   2700    2953798  -        1      4175.816
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        6       2025   2700    2953798  -        1      3756.521
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     0       2025   2700    726824   42.63    10     103.423
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     1       2025   2700    664274   42.64    8      136.967
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     2       2025   2700    626708   42.42    7      153.454
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     3       2025   2700    616320   42.96    4      308.364
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     4       2025   2700    618856   42.83    4      304.361
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     5       2025   2700    615274   42.71    3      341.053
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     6       2025   2700    603264   42.75    2      573.158
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        0       2025   2700    756806   42.39    7      153.972
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        1       2025   2700    740538   42.48    5      233.123
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        2       2025   2700    740538   42.48    5      245.292
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        3       2025   2700    662858   42.65    3      354.393
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        4       2025   2700    662858   42.65    3      391.080
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        5       2025   2700    662858   42.65    3      386.425
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        6       2025   2700    622244   42.52    3      386.038
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        7       2025   2700    622244   42.52    3      389.691
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        8       2025   2700    616898   42.71    2      697.013
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        9       2025   2700    616898   42.97    1      1116.593
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        0       2025   2700    726824   42.63    4      251.875
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        1       2025   2700    664274   42.64    3      362.902
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        2       2025   2700    626708   42.42    3      405.708
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        3       2025   2700    616320   42.96    1      1125.775
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        4       2025   2700    618856   42.83    1      1154.542
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        5       2025   2700    615274   42.71    1      1223.812
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        6       2025   2700    603264   42.75    1      1403.922
pexels-mavihnt-38213559.jpg                     lossless  libwebp     0       2560   1706    4156742  -        8      138.052
pexels-mavihnt-38213559.jpg                     lossless  libwebp     1       2560   1706    3973816  -        2      537.298
pexels-mavihnt-38213559.jpg                     lossless  libwebp     2       2560   1706    3988280  -        2      665.796
pexels-mavihnt-38213559.jpg                     lossless  libwebp     3       2560   1706    3487548  -        2      887.085
pexels-mavihnt-38213559.jpg                     lossless  libwebp     4       2560   1706    3488224  -        2      964.056
pexels-mavihnt-38213559.jpg                     lossless  libwebp     5       2560   1706    3482892  -        1      1081.110
pexels-mavihnt-38213559.jpg                     lossless  libwebp     6       2560   1706    3482480  -        1      1581.786
pexels-mavihnt-38213559.jpg                     lossless  libwebp     7       2560   1706    3481722  -        1      2547.725
pexels-mavihnt-38213559.jpg                     lossless  libwebp     8       2560   1706    3485234  -        1      2981.806
pexels-mavihnt-38213559.jpg                     lossless  libwebp     9       2560   1706    3485216  -        1      14548.323
pexels-mavihnt-38213559.jpg                     lossless  nativewebp  0       2560   1706    4004276  -        1      1206.721
pexels-mavihnt-38213559.jpg                     lossless  nativewebp  4       2560   1706    4007402  -        1      1237.559
pexels-mavihnt-38213559.jpg                     lossless  nativewebp  6       2560   1706    4009534  -        1      1316.342
pexels-mavihnt-38213559.jpg                     lossless  ours        0       2560   1706    3667900  -        5      228.220
pexels-mavihnt-38213559.jpg                     lossless  ours        1       2560   1706    3667900  -        4      267.364
pexels-mavihnt-38213559.jpg                     lossless  ours        2       2560   1706    3515976  -        2      667.844
pexels-mavihnt-38213559.jpg                     lossless  ours        3       2560   1706    3515976  -        2      770.784
pexels-mavihnt-38213559.jpg                     lossless  ours        4       2560   1706    3515976  -        2      938.440
pexels-mavihnt-38213559.jpg                     lossless  ours        5       2560   1706    3515976  -        1      1230.590
pexels-mavihnt-38213559.jpg                     lossless  ours        6       2560   1706    3509912  -        1      1609.592
pexels-mavihnt-38213559.jpg                     lossless  wasm        0       2560   1706    4116810  -        1      1050.167
pexels-mavihnt-38213559.jpg                     lossless  wasm        1       2560   1706    3484606  -        1      2532.153
pexels-mavihnt-38213559.jpg                     lossless  wasm        2       2560   1706    3484606  -        1      2551.823
pexels-mavihnt-38213559.jpg                     lossless  wasm        3       2560   1706    3484606  -        1      2524.139
pexels-mavihnt-38213559.jpg                     lossless  wasm        4       2560   1706    3482480  -        1      2722.337
pexels-mavihnt-38213559.jpg                     lossless  wasm        5       2560   1706    3488584  -        1      3343.312
pexels-mavihnt-38213559.jpg                     lossless  wasm        6       2560   1706    3488584  -        1      3112.482
pexels-mavihnt-38213559.jpg                     lossy     libwebp     0       2560   1706    983360   40.87    10     108.210
pexels-mavihnt-38213559.jpg                     lossy     libwebp     1       2560   1706    976610   40.88    8      137.801
pexels-mavihnt-38213559.jpg                     lossy     libwebp     2       2560   1706    935824   40.67    7      148.297
pexels-mavihnt-38213559.jpg                     lossy     libwebp     3       2560   1706    931074   41.15    4      289.448
pexels-mavihnt-38213559.jpg                     lossy     libwebp     4       2560   1706    934782   41.17    4      289.342
pexels-mavihnt-38213559.jpg                     lossy     libwebp     5       2560   1706    933892   41.05    4      331.945
pexels-mavihnt-38213559.jpg                     lossy     libwebp     6       2560   1706    925032   41.14    2      683.812
pexels-mavihnt-38213559.jpg                     lossy     ours        0       2560   1706    1037210  40.99    6      172.405
pexels-mavihnt-38213559.jpg                     lossy     ours        1       2560   1706    1019438  41.05    5      248.117
pexels-mavihnt-38213559.jpg                     lossy     ours        2       2560   1706    1019438  41.05    5      248.866
pexels-mavihnt-38213559.jpg                     lossy     ours        3       2560   1706    987826   41.15    3      378.129
pexels-mavihnt-38213559.jpg                     lossy     ours        4       2560   1706    987826   41.15    3      395.795
pexels-mavihnt-38213559.jpg                     lossy     ours        5       2560   1706    987826   41.15    3      400.803
pexels-mavihnt-38213559.jpg                     lossy     ours        6       2560   1706    952402   41.01    3      440.733
pexels-mavihnt-38213559.jpg                     lossy     ours        7       2560   1706    952402   41.01    3      437.831
pexels-mavihnt-38213559.jpg                     lossy     ours        8       2560   1706    948446   41.27    2      744.168
pexels-mavihnt-38213559.jpg                     lossy     ours        9       2560   1706    948446   41.27    1      1330.367
pexels-mavihnt-38213559.jpg                     lossy     wasm        0       2560   1706    983360   40.87    4      250.287
pexels-mavihnt-38213559.jpg                     lossy     wasm        1       2560   1706    976610   40.88    3      341.236
pexels-mavihnt-38213559.jpg                     lossy     wasm        2       2560   1706    935824   40.67    3      385.091
pexels-mavihnt-38213559.jpg                     lossy     wasm        3       2560   1706    931074   41.15    1      1104.404
pexels-mavihnt-38213559.jpg                     lossy     wasm        4       2560   1706    934782   41.17    1      1099.299
pexels-mavihnt-38213559.jpg                     lossy     wasm        5       2560   1706    933892   41.05    1      1196.042
pexels-mavihnt-38213559.jpg                     lossy     wasm        6       2560   1706    925032   41.14    1      1582.586
pexels-steve-15267299.jpg                       lossless  libwebp     0       2095   3000    2603112  -        6      179.653
pexels-steve-15267299.jpg                       lossless  libwebp     1       2095   3000    2557750  -        2      812.037
pexels-steve-15267299.jpg                       lossless  libwebp     2       2095   3000    2550344  -        1      1088.488
pexels-steve-15267299.jpg                       lossless  libwebp     3       2095   3000    2104000  -        1      1341.250
pexels-steve-15267299.jpg                       lossless  libwebp     4       2095   3000    2104010  -        1      1486.727
pexels-steve-15267299.jpg                       lossless  libwebp     5       2095   3000    2104010  -        1      1597.193
pexels-steve-15267299.jpg                       lossless  libwebp     6       2095   3000    2104690  -        1      2228.529
pexels-steve-15267299.jpg                       lossless  libwebp     7       2095   3000    2096804  -        1      2854.765
pexels-steve-15267299.jpg                       lossless  libwebp     8       2095   3000    2111270  -        1      4023.493
pexels-steve-15267299.jpg                       lossless  libwebp     9       2095   3000    2111200  -        1      21875.192
pexels-steve-15267299.jpg                       lossless  nativewebp  0       2095   3000    2635282  -        1      1505.832
pexels-steve-15267299.jpg                       lossless  nativewebp  4       2095   3000    2627962  -        1      1521.065
pexels-steve-15267299.jpg                       lossless  nativewebp  6       2095   3000    2622528  -        1      1687.169
pexels-steve-15267299.jpg                       lossless  ours        0       2095   3000    2233424  -        4      278.000
pexels-steve-15267299.jpg                       lossless  ours        1       2095   3000    2224752  -        3      352.981
pexels-steve-15267299.jpg                       lossless  ours        2       2095   3000    2045866  -        2      800.289
pexels-steve-15267299.jpg                       lossless  ours        3       2095   3000    2045866  -        2      934.202
pexels-steve-15267299.jpg                       lossless  ours        4       2095   3000    2036060  -        1      1142.425
pexels-steve-15267299.jpg                       lossless  ours        5       2095   3000    2030150  -        1      1638.654
pexels-steve-15267299.jpg                       lossless  ours        6       2095   3000    2024034  -        1      2009.534
pexels-steve-15267299.jpg                       lossless  wasm        0       2095   3000    2546112  -        1      1149.462
pexels-steve-15267299.jpg                       lossless  wasm        1       2095   3000    2103668  -        1      3232.945
pexels-steve-15267299.jpg                       lossless  wasm        2       2095   3000    2103668  -        1      3186.600
pexels-steve-15267299.jpg                       lossless  wasm        3       2095   3000    2103668  -        1      3211.666
pexels-steve-15267299.jpg                       lossless  wasm        4       2095   3000    2104690  -        1      3507.228
pexels-steve-15267299.jpg                       lossless  wasm        5       2095   3000    2119370  -        1      4189.874
pexels-steve-15267299.jpg                       lossless  wasm        6       2095   3000    2119370  -        1      3945.381
pexels-steve-15267299.jpg                       lossy     libwebp     0       2095   3000    284570   44.34    11     93.485
pexels-steve-15267299.jpg                       lossy     libwebp     1       2095   3000    275444   44.38    9      123.835
pexels-steve-15267299.jpg                       lossy     libwebp     2       2095   3000    261602   44.30    9      120.993
pexels-steve-15267299.jpg                       lossy     libwebp     3       2095   3000    255364   44.59    4      255.074
pexels-steve-15267299.jpg                       lossy     libwebp     4       2095   3000    256038   44.58    4      266.518
pexels-steve-15267299.jpg                       lossy     libwebp     5       2095   3000    253192   44.51    4      284.339
pexels-steve-15267299.jpg                       lossy     libwebp     6       2095   3000    247610   44.50    3      346.521
pexels-steve-15267299.jpg                       lossy     ours        0       2095   3000    294606   43.91    9      114.646
pexels-steve-15267299.jpg                       lossy     ours        1       2095   3000    287430   44.01    6      172.256
pexels-steve-15267299.jpg                       lossy     ours        2       2095   3000    287430   44.01    7      159.479
pexels-steve-15267299.jpg                       lossy     ours        3       2095   3000    264092   44.12    4      271.080
pexels-steve-15267299.jpg                       lossy     ours        4       2095   3000    264092   44.12    4      271.168
pexels-steve-15267299.jpg                       lossy     ours        5       2095   3000    264092   44.12    5      215.392
pexels-steve-15267299.jpg                       lossy     ours        6       2095   3000    247744   44.09    4      277.669
pexels-steve-15267299.jpg                       lossy     ours        7       2095   3000    247744   44.09    4      271.201
pexels-steve-15267299.jpg                       lossy     ours        8       2095   3000    252782   44.23    2      542.335
pexels-steve-15267299.jpg                       lossy     ours        9       2095   3000    252782   44.41    2      815.434
pexels-steve-15267299.jpg                       lossy     wasm        0       2095   3000    284570   44.34    5      247.055
pexels-steve-15267299.jpg                       lossy     wasm        1       2095   3000    275444   44.38    3      344.873
pexels-steve-15267299.jpg                       lossy     wasm        2       2095   3000    261602   44.30    3      342.410
pexels-steve-15267299.jpg                       lossy     wasm        3       2095   3000    255364   44.59    1      1064.000
pexels-steve-15267299.jpg                       lossy     wasm        4       2095   3000    256038   44.58    1      1057.680
pexels-steve-15267299.jpg                       lossy     wasm        5       2095   3000    253192   44.51    1      1119.925
pexels-steve-15267299.jpg                       lossy     wasm        6       2095   3000    247610   44.50    1      1070.009
pexels-steve-29626041.jpg                       lossless  libwebp     0       2560   1440    367064   -        21     49.234
pexels-steve-29626041.jpg                       lossless  libwebp     1       2560   1440    360562   -        3      387.825
pexels-steve-29626041.jpg                       lossless  libwebp     2       2560   1440    309376   -        2      598.882
pexels-steve-29626041.jpg                       lossless  libwebp     3       2560   1440    292034   -        2      626.460
pexels-steve-29626041.jpg                       lossless  libwebp     4       2560   1440    287166   -        2      647.123
pexels-steve-29626041.jpg                       lossless  libwebp     5       2560   1440    287662   -        2      709.276
pexels-steve-29626041.jpg                       lossless  libwebp     6       2560   1440    283988   -        2      886.447
pexels-steve-29626041.jpg                       lossless  libwebp     7       2560   1440    282342   -        2      917.343
pexels-steve-29626041.jpg                       lossless  libwebp     8       2560   1440    294462   -        1      1184.593
pexels-steve-29626041.jpg                       lossless  libwebp     9       2560   1440    292264   -        1      4118.168
pexels-steve-29626041.jpg                       lossless  nativewebp  0       2560   1440    378544   -        2      609.635
pexels-steve-29626041.jpg                       lossless  nativewebp  4       2560   1440    378536   -        2      598.461
pexels-steve-29626041.jpg                       lossless  nativewebp  6       2560   1440    379050   -        2      659.587
pexels-steve-29626041.jpg                       lossless  ours        0       2560   1440    312250   -        9      115.826
pexels-steve-29626041.jpg                       lossless  ours        1       2560   1440    312250   -        7      151.872
pexels-steve-29626041.jpg                       lossless  ours        2       2560   1440    300532   -        4      312.304
pexels-steve-29626041.jpg                       lossless  ours        3       2560   1440    299726   -        3      379.053
pexels-steve-29626041.jpg                       lossless  ours        4       2560   1440    299726   -        3      488.985
pexels-steve-29626041.jpg                       lossless  ours        5       2560   1440    297802   -        2      696.042
pexels-steve-29626041.jpg                       lossless  ours        6       2560   1440    295168   -        2      835.400
pexels-steve-29626041.jpg                       lossless  wasm        0       2560   1440    346824   -        5      233.652
pexels-steve-29626041.jpg                       lossless  wasm        1       2560   1440    283552   -        1      1240.071
pexels-steve-29626041.jpg                       lossless  wasm        2       2560   1440    283552   -        1      1236.405
pexels-steve-29626041.jpg                       lossless  wasm        3       2560   1440    283552   -        1      1240.298
pexels-steve-29626041.jpg                       lossless  wasm        4       2560   1440    283988   -        1      1338.290
pexels-steve-29626041.jpg                       lossless  wasm        5       2560   1440    296596   -        1      1778.976
pexels-steve-29626041.jpg                       lossless  wasm        6       2560   1440    296596   -        1      1781.105
pexels-steve-29626041.jpg                       lossy     libwebp     0       2560   1440    47614    49.05    23     43.906
pexels-steve-29626041.jpg                       lossy     libwebp     1       2560   1440    47302    49.05    17     60.450
pexels-steve-29626041.jpg                       lossy     libwebp     2       2560   1440    39488    49.30    20     51.662
pexels-steve-29626041.jpg                       lossy     libwebp     3       2560   1440    39096    49.71    9      113.122
pexels-steve-29626041.jpg                       lossy     libwebp     4       2560   1440    39310    49.71    9      115.081
pexels-steve-29626041.jpg                       lossy     libwebp     5       2560   1440    38754    49.65    9      122.219
pexels-steve-29626041.jpg                       lossy     libwebp     6       2560   1440    38252    49.65    8      131.873
pexels-steve-29626041.jpg                       lossy     ours        0       2560   1440    45686    49.31    21     49.497
pexels-steve-29626041.jpg                       lossy     ours        1       2560   1440    44820    49.35    12     90.452
pexels-steve-29626041.jpg                       lossy     ours        2       2560   1440    44820    49.35    15     67.124
pexels-steve-29626041.jpg                       lossy     ours        3       2560   1440    39426    49.40    15     72.141
pexels-steve-29626041.jpg                       lossy     ours        4       2560   1440    39426    49.40    11     99.199
pexels-steve-29626041.jpg                       lossy     ours        5       2560   1440    39426    49.40    10     100.969
pexels-steve-29626041.jpg                       lossy     ours        6       2560   1440    37562    49.37    10     106.081
pexels-steve-29626041.jpg                       lossy     ours        7       2560   1440    37562    49.37    10     106.328
pexels-steve-29626041.jpg                       lossy     ours        8       2560   1440    36254    49.45    6      180.388
pexels-steve-29626041.jpg                       lossy     ours        9       2560   1440    36254    49.62    5      236.437
pexels-steve-29626041.jpg                       lossy     wasm        0       2560   1440    47614    49.05    8      129.069
pexels-steve-29626041.jpg                       lossy     wasm        1       2560   1440    47302    49.05    6      182.292
pexels-steve-29626041.jpg                       lossy     wasm        2       2560   1440    39488    49.30    7      164.861
pexels-steve-29626041.jpg                       lossy     wasm        3       2560   1440    39096    49.71    3      493.151
pexels-steve-29626041.jpg                       lossy     wasm        4       2560   1440    39310    49.71    3      497.154
pexels-steve-29626041.jpg                       lossy     wasm        5       2560   1440    38754    49.65    2      527.804
pexels-steve-29626041.jpg                       lossy     wasm        6       2560   1440    38252    49.65    3      476.050
pexels-toulouse-10807703.jpg                    lossless  libwebp     0       1400   2100    3044142  -        12     84.706
pexels-toulouse-10807703.jpg                    lossless  libwebp     1       1400   2100    2807602  -        3      420.949
pexels-toulouse-10807703.jpg                    lossless  libwebp     2       1400   2100    2780320  -        2      539.060
pexels-toulouse-10807703.jpg                    lossless  libwebp     3       1400   2100    2693662  -        2      603.359
pexels-toulouse-10807703.jpg                    lossless  libwebp     4       1400   2100    2693662  -        2      646.163
pexels-toulouse-10807703.jpg                    lossless  libwebp     5       1400   2100    2691936  -        2      724.071
pexels-toulouse-10807703.jpg                    lossless  libwebp     6       1400   2100    2688812  -        1      1109.767
pexels-toulouse-10807703.jpg                    lossless  libwebp     7       1400   2100    2688410  -        1      1406.335
pexels-toulouse-10807703.jpg                    lossless  libwebp     8       1400   2100    2686410  -        1      2029.696
pexels-toulouse-10807703.jpg                    lossless  libwebp     9       1400   2100    2685794  -        1      8547.538
pexels-toulouse-10807703.jpg                    lossless  nativewebp  0       1400   2100    2989804  -        2      700.621
pexels-toulouse-10807703.jpg                    lossless  nativewebp  4       1400   2100    2992710  -        2      719.875
pexels-toulouse-10807703.jpg                    lossless  nativewebp  6       1400   2100    2996214  -        2      746.765
pexels-toulouse-10807703.jpg                    lossless  ours        0       1400   2100    2868724  -        7      152.118
pexels-toulouse-10807703.jpg                    lossless  ours        1       1400   2100    2868724  -        6      180.140
pexels-toulouse-10807703.jpg                    lossless  ours        2       1400   2100    2718058  -        3      450.048
pexels-toulouse-10807703.jpg                    lossless  ours        3       1400   2100    2718058  -        2      530.090
pexels-toulouse-10807703.jpg                    lossless  ours        4       1400   2100    2718058  -        2      630.183
pexels-toulouse-10807703.jpg                    lossless  ours        5       1400   2100    2718058  -        2      850.478
pexels-toulouse-10807703.jpg                    lossless  ours        6       1400   2100    2721246  -        1      1112.734
pexels-toulouse-10807703.jpg                    lossless  wasm        0       1400   2100    3029106  -        2      512.671
pexels-toulouse-10807703.jpg                    lossless  wasm        1       1400   2100    2690886  -        1      1796.869
pexels-toulouse-10807703.jpg                    lossless  wasm        2       1400   2100    2690886  -        1      1801.028
pexels-toulouse-10807703.jpg                    lossless  wasm        3       1400   2100    2690886  -        1      1807.005
pexels-toulouse-10807703.jpg                    lossless  wasm        4       1400   2100    2688812  -        1      1956.820
pexels-toulouse-10807703.jpg                    lossless  wasm        5       1400   2100    2685806  -        1      2904.765
pexels-toulouse-10807703.jpg                    lossless  wasm        6       1400   2100    2685806  -        1      2452.866
pexels-toulouse-10807703.jpg                    lossy     libwebp     0       1400   2100    857060   39.84    13     80.885
pexels-toulouse-10807703.jpg                    lossy     libwebp     1       1400   2100    812040   39.84    10     103.168
pexels-toulouse-10807703.jpg                    lossy     libwebp     2       1400   2100    796814   39.60    9      111.125
pexels-toulouse-10807703.jpg                    lossy     libwebp     3       1400   2100    783482   39.95    5      201.421
pexels-toulouse-10807703.jpg                    lossy     libwebp     4       1400   2100    784734   39.96    5      201.314
pexels-toulouse-10807703.jpg                    lossy     libwebp     5       1400   2100    768426   39.81    5      227.442
pexels-toulouse-10807703.jpg                    lossy     libwebp     6       1400   2100    758466   39.84    3      468.559
pexels-toulouse-10807703.jpg                    lossy     ours        0       1400   2100    854742   39.93    8      132.330
pexels-toulouse-10807703.jpg                    lossy     ours        1       1400   2100    844538   39.95    6      171.040
pexels-toulouse-10807703.jpg                    lossy     ours        2       1400   2100    844538   39.95    6      184.928
pexels-toulouse-10807703.jpg                    lossy     ours        3       1400   2100    812788   40.06    4      267.219
pexels-toulouse-10807703.jpg                    lossy     ours        4       1400   2100    812788   40.06    4      260.945
pexels-toulouse-10807703.jpg                    lossy     ours        5       1400   2100    812788   40.06    4      262.424
pexels-toulouse-10807703.jpg                    lossy     ours        6       1400   2100    778956   39.92    4      312.587
pexels-toulouse-10807703.jpg                    lossy     ours        7       1400   2100    778956   39.92    4      314.646
pexels-toulouse-10807703.jpg                    lossy     ours        8       1400   2100    776930   40.10    2      550.947
pexels-toulouse-10807703.jpg                    lossy     ours        9       1400   2100    776930   40.03    2      679.936
pexels-toulouse-10807703.jpg                    lossy     wasm        0       1400   2100    857060   39.84    6      179.400
pexels-toulouse-10807703.jpg                    lossy     wasm        1       1400   2100    812040   39.84    4      251.083
pexels-toulouse-10807703.jpg                    lossy     wasm        2       1400   2100    796814   39.60    4      275.397
pexels-toulouse-10807703.jpg                    lossy     wasm        3       1400   2100    783482   39.95    2      778.908
pexels-toulouse-10807703.jpg                    lossy     wasm        4       1400   2100    784734   39.96    2      778.453
pexels-toulouse-10807703.jpg                    lossy     wasm        5       1400   2100    768426   39.81    2      842.099
pexels-toulouse-10807703.jpg                    lossy     wasm        6       1400   2100    758466   39.84    1      1103.865
```

## amd64 (AMD Ryzen 7 5700G) / photos

```
file                                            mode        engine      width  height  bytes    psnr_db  iters  ms_per_op
Lena_512.png                                    lossless    libwebp     900    900     627632   -        8      251.360
Lena_512.png                                    lossless    nativewebp  900    900     738176   -        7      314.280
Lena_512.png                                    lossless    ours        900    900     638220   -        5      423.833
Lena_512.png                                    lossless    wasm        900    900     622766   -        2      1484.923
Lena_512.png                                    lossy-fast  libwebp     900    900     103980   41.04    116    17.305
Lena_512.png                                    lossy-fast  ours        900    900     113182   41.02    67     29.940
Lena_512.png                                    lossy-fast  wasm        900    900     103980   41.04    31     66.598
Lena_512.png                                    lossy-slow  libwebp     900    900     89968    40.94    19     108.754
Lena_512.png                                    lossy-slow  ours        900    900     95710    41.13    11     196.868
Lena_512.png                                    lossy-slow  wasm        900    900     89968    40.94    6      362.613
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    libwebp     2025   2700    3241976  -        1      2041.033
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    nativewebp  2025   2700    3926412  -        1      2889.340
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    ours        2025   2700    3269944  -        1      2734.709
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    wasm        2025   2700    3249798  -        1      5878.134
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  libwebp     2025   2700    598034   41.96    18     114.473
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  ours        2025   2700    723064   42.02    11     198.126
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  wasm        2025   2700    598034   41.96    5      452.604
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  libwebp     2025   2700    610518   42.58    3      967.028
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  ours        2025   2700    603030   42.33    2      1702.602
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  wasm        2025   2700    610518   42.58    1      2864.939
pexels-martin-alargent-1165956-5665465.jpg      lossless    libwebp     2025   2700    2943528  -        1      2097.458
pexels-martin-alargent-1165956-5665465.jpg      lossless    nativewebp  2025   2700    3563584  -        1      2510.540
pexels-martin-alargent-1165956-5665465.jpg      lossless    ours        2025   2700    2919664  -        1      2751.442
pexels-martin-alargent-1165956-5665465.jpg      lossless    wasm        2025   2700    2953798  -        1      5819.781
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  libwebp     2025   2700    726824   42.63    18     117.501
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  ours        2025   2700    756806   42.39    11     198.528
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  wasm        2025   2700    726824   42.63    5      465.310
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  libwebp     2025   2700    603264   42.75    3      808.652
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  ours        2025   2700    616898   42.97    2      1399.126
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  wasm        2025   2700    603264   42.75    1      2589.171
pexels-mavihnt-38213559.jpg                     lossless    libwebp     2560   1706    3482480  -        2      1631.828
pexels-mavihnt-38213559.jpg                     lossless    nativewebp  2560   1706    4009534  -        1      2102.759
pexels-mavihnt-38213559.jpg                     lossless    ours        2560   1706    3509912  -        1      2293.608
pexels-mavihnt-38213559.jpg                     lossless    wasm        2560   1706    3488584  -        1      4745.788
pexels-mavihnt-38213559.jpg                     lossy-fast  libwebp     2560   1706    983360   40.87    17     119.958
pexels-mavihnt-38213559.jpg                     lossy-fast  ours        2560   1706    1037210  40.99    10     212.370
pexels-mavihnt-38213559.jpg                     lossy-fast  wasm        2560   1706    983360   40.87    5      411.695
pexels-mavihnt-38213559.jpg                     lossy-slow  libwebp     2560   1706    925032   41.14    3      960.699
pexels-mavihnt-38213559.jpg                     lossy-slow  ours        2560   1706    948446   41.27    2      1608.840
pexels-mavihnt-38213559.jpg                     lossy-slow  wasm        2560   1706    925032   41.14    1      2686.217
pexels-steve-15267299.jpg                       lossless    libwebp     2095   3000    2104690  -        1      2169.076
pexels-steve-15267299.jpg                       lossless    nativewebp  2095   3000    2622528  -        1      2838.410
pexels-steve-15267299.jpg                       lossless    ours        2095   3000    2024034  -        1      3033.071
pexels-steve-15267299.jpg                       lossless    wasm        2095   3000    2119370  -        1      6048.222
pexels-steve-15267299.jpg                       lossy-fast  libwebp     2095   3000    284570   44.34    20     104.449
pexels-steve-15267299.jpg                       lossy-fast  ours        2095   3000    294606   43.91    13     158.060
pexels-steve-15267299.jpg                       lossy-fast  wasm        2095   3000    284570   44.34    5      459.701
pexels-steve-15267299.jpg                       lossy-slow  libwebp     2095   3000    247610   44.50    4      510.949
pexels-steve-15267299.jpg                       lossy-slow  ours        2095   3000    252782   44.41    2      1142.128
pexels-steve-15267299.jpg                       lossy-slow  wasm        2095   3000    247610   44.50    1      2112.128
pexels-steve-29626041.jpg                       lossless    libwebp     2560   1440    283988   -        3      726.903
pexels-steve-29626041.jpg                       lossless    nativewebp  2560   1440    379050   -        2      1362.290
pexels-steve-29626041.jpg                       lossless    ours        2560   1440    295168   -        2      1352.605
pexels-steve-29626041.jpg                       lossless    wasm        2560   1440    296596   -        1      2829.326
pexels-steve-29626041.jpg                       lossy-fast  libwebp     2560   1440    47614    49.05    40     50.222
pexels-steve-29626041.jpg                       lossy-fast  ours        2560   1440    45686    49.31    29     70.896
pexels-steve-29626041.jpg                       lossy-fast  wasm        2560   1440    47614    49.05    9      242.922
pexels-steve-29626041.jpg                       lossy-slow  libwebp     2560   1440    38252    49.65    10     204.269
pexels-steve-29626041.jpg                       lossy-slow  ours        2560   1440    36254    49.62    6      348.722
pexels-steve-29626041.jpg                       lossy-slow  wasm        2560   1440    38252    49.65    3      974.254
pexels-toulouse-10807703.jpg                    lossless    libwebp     1400   2100    2688812  -        2      1118.991
pexels-toulouse-10807703.jpg                    lossless    nativewebp  1400   2100    2996214  -        2      1234.350
pexels-toulouse-10807703.jpg                    lossless    ours        1400   2100    2721246  -        2      1546.427
pexels-toulouse-10807703.jpg                    lossless    wasm        1400   2100    2685806  -        1      3887.466
pexels-toulouse-10807703.jpg                    lossy-fast  libwebp     1400   2100    857060   39.84    23     89.954
pexels-toulouse-10807703.jpg                    lossy-fast  ours        1400   2100    854742   39.93    13     162.057
pexels-toulouse-10807703.jpg                    lossy-fast  wasm        1400   2100    857060   39.84    7      295.200
pexels-toulouse-10807703.jpg                    lossy-slow  libwebp     1400   2100    758466   39.84    4      658.069
pexels-toulouse-10807703.jpg                    lossy-slow  ours        1400   2100    776930   40.03    3      834.433
pexels-toulouse-10807703.jpg                    lossy-slow  wasm        1400   2100    758466   39.84    2      1844.012
```

Peak RSS, one encode per process:

```
file                                            mode        engine      width  height  megapixels  peak_rss_mib  mib_per_mp
Lena_512.png                                    lossless    libwebp     900    900     0.81        46.3          57.2
Lena_512.png                                    lossless    nativewebp  900    900     0.81        43.9          54.2
Lena_512.png                                    lossless    ours        900    900     0.81        72.3          89.2
Lena_512.png                                    lossless    wasm        900    900     0.81        117.6         145.2
Lena_512.png                                    lossy-fast  libwebp     900    900     0.81        24.8          30.7
Lena_512.png                                    lossy-fast  ours        900    900     0.81        22.3          27.5
Lena_512.png                                    lossy-fast  wasm        900    900     0.81        37.3          46.0
Lena_512.png                                    lossy-slow  libwebp     900    900     0.81        26.3          32.5
Lena_512.png                                    lossy-slow  ours        900    900     0.81        24.2          29.9
Lena_512.png                                    lossy-slow  wasm        900    900     0.81        49.3          60.8
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    libwebp     2025   2700    5.47        209.0         38.2
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    nativewebp  2025   2700    5.47        186.2         34.1
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    ours        2025   2700    5.47        428.8         78.4
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    wasm        2025   2700    5.47        699.9         128.0
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  libwebp     2025   2700    5.47        90.8          16.6
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  ours        2025   2700    5.47        78.1          14.3
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  wasm        2025   2700    5.47        197.2         36.1
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  libwebp     2025   2700    5.47        99.5          18.2
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  ours        2025   2700    5.47        91.9          16.8
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  wasm        2025   2700    5.47        197.1         36.0
pexels-martin-alargent-1165956-5665465.jpg      lossless    libwebp     2025   2700    5.47        200.9         36.7
pexels-martin-alargent-1165956-5665465.jpg      lossless    nativewebp  2025   2700    5.47        182.3         33.3
pexels-martin-alargent-1165956-5665465.jpg      lossless    ours        2025   2700    5.47        389.1         71.2
pexels-martin-alargent-1165956-5665465.jpg      lossless    wasm        2025   2700    5.47        700.1         128.0
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  libwebp     2025   2700    5.47        88.9          16.3
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  ours        2025   2700    5.47        76.1          13.9
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  wasm        2025   2700    5.47        199.3         36.5
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  libwebp     2025   2700    5.47        101.0         18.5
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  ours        2025   2700    5.47        92.2          16.9
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  wasm        2025   2700    5.47        199.2         36.4
pexels-mavihnt-38213559.jpg                     lossless    libwebp     2560   1706    4.37        175.5         40.2
pexels-mavihnt-38213559.jpg                     lossless    nativewebp  2560   1706    4.37        160.1         36.7
pexels-mavihnt-38213559.jpg                     lossless    ours        2560   1706    4.37        313.0         71.7
pexels-mavihnt-38213559.jpg                     lossless    wasm        2560   1706    4.37        563.3         129.0
pexels-mavihnt-38213559.jpg                     lossy-fast  libwebp     2560   1706    4.37        75.9          17.4
pexels-mavihnt-38213559.jpg                     lossy-fast  ours        2560   1706    4.37        64.3          14.7
pexels-mavihnt-38213559.jpg                     lossy-fast  wasm        2560   1706    4.37        161.3         36.9
pexels-mavihnt-38213559.jpg                     lossy-slow  libwebp     2560   1706    4.37        91.7          21.0
pexels-mavihnt-38213559.jpg                     lossy-slow  ours        2560   1706    4.37        76.4          17.5
pexels-mavihnt-38213559.jpg                     lossy-slow  wasm        2560   1706    4.37        225.3         51.6
pexels-steve-15267299.jpg                       lossless    libwebp     2095   3000    6.29        207.5         33.0
pexels-steve-15267299.jpg                       lossless    nativewebp  2095   3000    6.29        192.3         30.6
pexels-steve-15267299.jpg                       lossless    ours        2095   3000    6.29        431.5         68.7
pexels-steve-15267299.jpg                       lossless    wasm        2095   3000    6.29        578.1         92.0
pexels-steve-15267299.jpg                       lossy-fast  libwebp     2095   3000    6.29        102.4         16.3
pexels-steve-15267299.jpg                       lossy-fast  ours        2095   3000    6.29        84.4          13.4
pexels-steve-15267299.jpg                       lossy-fast  wasm        2095   3000    6.29        227.2         36.1
pexels-steve-15267299.jpg                       lossy-slow  libwebp     2095   3000    6.29        105.9         16.8
pexels-steve-15267299.jpg                       lossy-slow  ours        2095   3000    6.29        96.0          15.3
pexels-steve-15267299.jpg                       lossy-slow  wasm        2095   3000    6.29        227.5         36.2
pexels-steve-29626041.jpg                       lossless    libwebp     2560   1440    3.69        128.0         34.7
pexels-steve-29626041.jpg                       lossless    nativewebp  2560   1440    3.69        104.2         28.3
pexels-steve-29626041.jpg                       lossless    ours        2560   1440    3.69        242.7         65.8
pexels-steve-29626041.jpg                       lossless    wasm        2560   1440    3.69        345.5         93.7
pexels-steve-29626041.jpg                       lossy-fast  libwebp     2560   1440    3.69        65.9          17.9
pexels-steve-29626041.jpg                       lossy-fast  ours        2560   1440    3.69        54.0          14.6
pexels-steve-29626041.jpg                       lossy-fast  wasm        2560   1440    3.69        93.1          25.3
pexels-steve-29626041.jpg                       lossy-slow  libwebp     2560   1440    3.69        62.9          17.1
pexels-steve-29626041.jpg                       lossy-slow  ours        2560   1440    3.69        58.2          15.8
pexels-steve-29626041.jpg                       lossy-slow  wasm        2560   1440    3.69        139.2         37.7
pexels-toulouse-10807703.jpg                    lossless    libwebp     1400   2100    2.94        120.4         40.9
pexels-toulouse-10807703.jpg                    lossless    nativewebp  1400   2100    2.94        122.4         41.6
pexels-toulouse-10807703.jpg                    lossless    ours        1400   2100    2.94        214.7         73.0
pexels-toulouse-10807703.jpg                    lossless    wasm        1400   2100    2.94        383.9         130.6
pexels-toulouse-10807703.jpg                    lossy-fast  libwebp     1400   2100    2.94        56.0          19.1
pexels-toulouse-10807703.jpg                    lossy-fast  ours        1400   2100    2.94        50.2          17.1
pexels-toulouse-10807703.jpg                    lossy-fast  wasm        1400   2100    2.94        111.0         37.8
pexels-toulouse-10807703.jpg                    lossy-slow  libwebp     1400   2100    2.94        67.9          23.1
pexels-toulouse-10807703.jpg                    lossy-slow  ours        1400   2100    2.94        56.2          19.1
pexels-toulouse-10807703.jpg                    lossy-slow  wasm        1400   2100    2.94        155.2         52.8
```

Decode, one file per mode encoded by libwebp:

```
file                                            mode      engine   width  height  bytes    psnr_db  iters  ms_per_op
Lena_512.png                                    lossless  libwebp  900    900     627632   -        208    9.655
Lena_512.png                                    lossless  ours     900    900     627632   -        104    19.335
Lena_512.png                                    lossless  wasm     900    900     627632   28.40    52     38.534
Lena_512.png                                    lossless  x/image  900    900     627632   -        90     22.342
Lena_512.png                                    lossy     libwebp  900    900     89968    -        309    6.488
Lena_512.png                                    lossy     ours     900    900     89968    -        92     21.865
Lena_512.png                                    lossy     wasm     900    900     89968    28.34    78     25.748
Lena_512.png                                    lossy     x/image  900    900     89968    28.34    83     24.114
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp  2025   2700    3241976  -        40     50.747
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours     2025   2700    3241976  -        18     116.221
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm     2025   2700    3241976  29.06    9      245.587
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  x/image  2025   2700    3241976  -        15     133.842
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp  2025   2700    610518   -        44     46.006
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours     2025   2700    610518   -        14     151.901
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm     2025   2700    610518   29.09    12     180.275
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     x/image  2025   2700    610518   29.09    12     167.772
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp  2025   2700    2943528  -        40     50.555
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours     2025   2700    2943528  -        19     109.392
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm     2025   2700    2943528  30.36    9      244.500
pexels-martin-alargent-1165956-5665465.jpg      lossless  x/image  2025   2700    2943528  -        16     127.304
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp  2025   2700    603264   -        45     45.092
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours     2025   2700    603264   -        14     148.823
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm     2025   2700    603264   30.34    12     173.614
pexels-martin-alargent-1165956-5665465.jpg      lossy     x/image  2025   2700    603264   30.34    13     161.530
pexels-mavihnt-38213559.jpg                     lossless  libwebp  2560   1706    3482480  -        43     47.048
pexels-mavihnt-38213559.jpg                     lossless  ours     2560   1706    3482480  -        22     94.708
pexels-mavihnt-38213559.jpg                     lossless  wasm     2560   1706    3482480  27.55    10     206.506
pexels-mavihnt-38213559.jpg                     lossless  x/image  2560   1706    3482480  -        18     114.698
pexels-mavihnt-38213559.jpg                     lossy     libwebp  2560   1706    925032   -        39     51.990
pexels-mavihnt-38213559.jpg                     lossy     ours     2560   1706    925032   -        15     133.420
pexels-mavihnt-38213559.jpg                     lossy     wasm     2560   1706    925032   27.55    13     155.574
pexels-mavihnt-38213559.jpg                     lossy     x/image  2560   1706    925032   27.55    14     147.647
pexels-steve-15267299.jpg                       lossless  libwebp  2095   3000    2104690  -        40     50.975
pexels-steve-15267299.jpg                       lossless  ours     2095   3000    2104690  -        17     123.745
pexels-steve-15267299.jpg                       lossless  wasm     2095   3000    2104690  28.98    8      260.647
pexels-steve-15267299.jpg                       lossless  x/image  2095   3000    2104690  -        15     138.641
pexels-steve-15267299.jpg                       lossy     libwebp  2095   3000    247610   -        66     30.515
pexels-steve-15267299.jpg                       lossy     ours     2095   3000    247610   -        15     136.602
pexels-steve-15267299.jpg                       lossy     wasm     2095   3000    247610   28.98    13     164.930
pexels-steve-15267299.jpg                       lossy     x/image  2095   3000    247610   28.98    14     149.493
pexels-steve-29626041.jpg                       lossless  libwebp  2560   1440    283988   -        131    15.342
pexels-steve-29626041.jpg                       lossless  ours     2560   1440    283988   -        42     47.658
pexels-steve-29626041.jpg                       lossless  wasm     2560   1440    283988   32.17    17     118.890
pexels-steve-29626041.jpg                       lossless  x/image  2560   1440    283988   -        38     53.264
pexels-steve-29626041.jpg                       lossy     libwebp  2560   1440    38252    -        181    11.060
pexels-steve-29626041.jpg                       lossy     ours     2560   1440    38252    -        33     60.717
pexels-steve-29626041.jpg                       lossy     wasm     2560   1440    38252    32.10    26     78.968
pexels-steve-29626041.jpg                       lossy     x/image  2560   1440    38252    32.10    30     68.789
pexels-toulouse-10807703.jpg                    lossless  libwebp  1400   2100    2688812  -        58     34.909
pexels-toulouse-10807703.jpg                    lossless  ours     1400   2100    2688812  -        30     67.830
pexels-toulouse-10807703.jpg                    lossless  wasm     1400   2100    2688812  27.59    15     142.043
pexels-toulouse-10807703.jpg                    lossless  x/image  1400   2100    2688812  -        24     86.094
pexels-toulouse-10807703.jpg                    lossy     libwebp  1400   2100    758466   -        49     41.534
pexels-toulouse-10807703.jpg                    lossy     ours     1400   2100    758466   -        20     101.490
pexels-toulouse-10807703.jpg                    lossy     wasm     1400   2100    758466   27.54    18     117.444
pexels-toulouse-10807703.jpg                    lossy     x/image  1400   2100    758466   27.54    19     109.160
```

Effort sweep, every setting of every engine:

```
file                                            mode      engine      effort  width  height  bytes    psnr_db  iters  ms_per_op
Lena_512.png                                    lossless  libwebp     0       900    900     734910   -        45     22.302
Lena_512.png                                    lossless  libwebp     1       900    900     661194   -        9      118.236
Lena_512.png                                    lossless  libwebp     2       900    900     625874   -        7      154.972
Lena_512.png                                    lossless  libwebp     3       900    900     628862   -        6      184.763
Lena_512.png                                    lossless  libwebp     4       900    900     628612   -        6      193.921
Lena_512.png                                    lossless  libwebp     5       900    900     628612   -        6      189.940
Lena_512.png                                    lossless  libwebp     6       900    900     627632   -        4      254.734
Lena_512.png                                    lossless  libwebp     7       900    900     625628   -        3      364.413
Lena_512.png                                    lossless  libwebp     8       900    900     616842   -        2      648.229
Lena_512.png                                    lossless  libwebp     9       900    900     609124   -        1      3910.922
Lena_512.png                                    lossless  nativewebp  0       900    900     741562   -        4      255.086
Lena_512.png                                    lossless  nativewebp  4       900    900     739400   -        4      270.955
Lena_512.png                                    lossless  nativewebp  6       900    900     738176   -        4      321.278
Lena_512.png                                    lossless  ours        0       900    900     695082   -        17     59.089
Lena_512.png                                    lossless  ours        1       900    900     691746   -        14     72.318
Lena_512.png                                    lossless  ours        2       900    900     661658   -        6      177.292
Lena_512.png                                    lossless  ours        3       900    900     661658   -        5      208.758
Lena_512.png                                    lossless  ours        4       900    900     660788   -        4      255.159
Lena_512.png                                    lossless  ours        5       900    900     657842   -        3      349.512
Lena_512.png                                    lossless  ours        6       900    900     638220   -        3      419.747
Lena_512.png                                    lossless  wasm        0       900    900     731954   -        8      136.858
Lena_512.png                                    lossless  wasm        1       900    900     639584   -        2      637.909
Lena_512.png                                    lossless  wasm        2       900    900     627632   -        2      702.292
Lena_512.png                                    lossless  wasm        3       900    900     627632   -        2      703.084
Lena_512.png                                    lossless  wasm        4       900    900     627632   -        2      702.416
Lena_512.png                                    lossless  wasm        5       900    900     618010   -        1      1597.023
Lena_512.png                                    lossless  wasm        6       900    900     622766   -        1      1489.910
Lena_512.png                                    lossy     libwebp     0       900    900     103980   41.04    59     17.235
Lena_512.png                                    lossy     libwebp     1       900    900     102500   41.05    44     22.913
Lena_512.png                                    lossy     libwebp     2       900    900     94342    40.70    43     23.494
Lena_512.png                                    lossy     libwebp     3       900    900     91788    40.98    20     52.390
Lena_512.png                                    lossy     libwebp     4       900    900     92124    40.97    20     52.335
Lena_512.png                                    lossy     libwebp     5       900    900     91586    40.91    17     58.874
Lena_512.png                                    lossy     libwebp     6       900    900     89968    40.94    10     108.673
Lena_512.png                                    lossy     ours        0       900    900     113182   41.02    33     30.338
Lena_512.png                                    lossy     ours        1       900    900     111048   41.05    24     42.802
Lena_512.png                                    lossy     ours        2       900    900     111048   41.05    24     43.104
Lena_512.png                                    lossy     ours        3       900    900     101404   41.15    17     61.749
Lena_512.png                                    lossy     ours        4       900    900     101404   41.15    16     62.596
Lena_512.png                                    lossy     ours        5       900    900     101404   41.15    17     61.937
Lena_512.png                                    lossy     ours        6       900    900     96566    41.08    15     67.760
Lena_512.png                                    lossy     ours        7       900    900     96566    41.08    15     69.327
Lena_512.png                                    lossy     ours        8       900    900     95710    41.21    9      119.417
Lena_512.png                                    lossy     ours        9       900    900     95710    41.13    6      199.962
Lena_512.png                                    lossy     wasm        0       900    900     103980   41.04    15     66.832
Lena_512.png                                    lossy     wasm        1       900    900     102500   41.05    11     95.939
Lena_512.png                                    lossy     wasm        2       900    900     94342    40.70    11     97.458
Lena_512.png                                    lossy     wasm        3       900    900     91788    40.98    4      280.270
Lena_512.png                                    lossy     wasm        4       900    900     92124    40.97    4      282.419
Lena_512.png                                    lossy     wasm        5       900    900     91586    40.91    4      304.424
Lena_512.png                                    lossy     wasm        6       900    900     89968    40.94    3      363.925
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     0       2025   2700    3987548  -        5      210.947
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     1       2025   2700    3992712  -        2      765.830
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     2       2025   2700    3993532  -        2      876.954
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     3       2025   2700    3246280  -        1      1140.613
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     4       2025   2700    3246304  -        1      1283.978
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     5       2025   2700    3242976  -        1      1384.683
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     6       2025   2700    3241976  -        1      2021.983
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     7       2025   2700    3237862  -        1      3264.545
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     8       2025   2700    3246580  -        1      3576.638
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     9       2025   2700    3245966  -        1      22012.344
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  nativewebp  0       2025   2700    3944492  -        1      2860.176
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  nativewebp  4       2025   2700    3938686  -        1      2646.952
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  nativewebp  6       2025   2700    3926412  -        1      3079.215
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        0       2025   2700    3496516  -        3      368.069
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        1       2025   2700    3486442  -        3      453.737
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        2       2025   2700    3398814  -        1      1109.881
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        3       2025   2700    3288396  -        1      1338.781
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        4       2025   2700    3289656  -        1      1656.423
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        5       2025   2700    3289656  -        1      2230.021
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        6       2025   2700    3269944  -        1      2769.173
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        0       2025   2700    3961206  -        1      1903.646
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        1       2025   2700    3244586  -        1      5040.162
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        2       2025   2700    3244586  -        1      4948.186
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        3       2025   2700    3244586  -        1      4913.912
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        4       2025   2700    3241976  -        1      5044.705
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        5       2025   2700    3249798  -        1      6245.534
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        6       2025   2700    3249798  -        1      5852.114
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     0       2025   2700    598034   41.96    9      114.120
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     1       2025   2700    588954   41.97    7      151.187
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     2       2025   2700    614528   42.20    6      172.680
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     3       2025   2700    627994   42.39    3      390.175
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     4       2025   2700    632636   42.42    3      391.980
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     5       2025   2700    625540   42.27    3      450.396
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     6       2025   2700    610518   42.58    2      970.757
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        0       2025   2700    723064   42.02    6      195.361
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        1       2025   2700    694902   42.09    4      274.510
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        2       2025   2700    694902   42.09    4      273.477
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        3       2025   2700    686064   42.28    2      501.301
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        4       2025   2700    686064   42.28    2      503.121
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        5       2025   2700    686064   42.28    2      500.751
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        6       2025   2700    611704   42.07    2      568.939
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        7       2025   2700    611704   42.07    2      570.574
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        8       2025   2700    603030   42.33    2      955.972
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        9       2025   2700    603030   42.33    1      1682.357
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        0       2025   2700    598034   41.96    3      450.988
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        1       2025   2700    588954   41.97    2      641.416
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        2       2025   2700    614528   42.20    2      728.758
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        3       2025   2700    627994   42.39    1      2044.023
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        4       2025   2700    632636   42.42    1      2042.631
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        5       2025   2700    625540   42.27    1      2222.093
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        6       2025   2700    610518   42.58    1      2896.224
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     0       2025   2700    3673696  -        5      213.830
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     1       2025   2700    3494146  -        2      839.223
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     2       2025   2700    3493206  -        2      972.411
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     3       2025   2700    2949858  -        1      1194.856
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     4       2025   2700    2950040  -        1      1349.964
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     5       2025   2700    2943056  -        1      1452.624
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     6       2025   2700    2943528  -        1      2150.982
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     7       2025   2700    2929290  -        1      2887.232
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     8       2025   2700    2938176  -        1      3397.595
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     9       2025   2700    2940364  -        1      26162.390
pexels-martin-alargent-1165956-5665465.jpg      lossless  nativewebp  0       2025   2700    3554880  -        1      2314.381
pexels-martin-alargent-1165956-5665465.jpg      lossless  nativewebp  4       2025   2700    3559008  -        1      2411.733
pexels-martin-alargent-1165956-5665465.jpg      lossless  nativewebp  6       2025   2700    3563584  -        1      2684.085
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        0       2025   2700    3178224  -        3      362.323
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        1       2025   2700    3178224  -        3      437.128
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        2       2025   2700    2936002  -        1      1108.071
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        3       2025   2700    2936002  -        1      1274.594
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        4       2025   2700    2936002  -        1      1580.474
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        5       2025   2700    2936002  -        1      2147.351
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        6       2025   2700    2919664  -        1      2694.598
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        0       2025   2700    3625218  -        1      1790.678
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        1       2025   2700    2948150  -        1      4825.336
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        2       2025   2700    2948150  -        1      4739.296
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        3       2025   2700    2948150  -        1      4669.745
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        4       2025   2700    2943528  -        1      5156.165
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        5       2025   2700    2953798  -        1      6455.698
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        6       2025   2700    2953798  -        1      5812.387
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     0       2025   2700    726824   42.63    9      116.868
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     1       2025   2700    664274   42.64    7      156.123
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     2       2025   2700    626708   42.42    6      176.670
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     3       2025   2700    616320   42.96    3      372.049
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     4       2025   2700    618856   42.83    3      383.814
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     5       2025   2700    615274   42.71    3      424.494
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     6       2025   2700    603264   42.75    2      816.822
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        0       2025   2700    756806   42.39    5      203.165
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        1       2025   2700    740538   42.48    4      277.267
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        2       2025   2700    740538   42.48    4      277.439
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        3       2025   2700    662858   42.65    3      467.464
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        4       2025   2700    662858   42.65    3      470.113
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        5       2025   2700    662858   42.65    3      468.717
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        6       2025   2700    622244   42.52    2      515.173
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        7       2025   2700    622244   42.52    2      519.242
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        8       2025   2700    616898   42.71    2      890.199
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        9       2025   2700    616898   42.97    1      1407.726
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        0       2025   2700    726824   42.63    3      470.684
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        1       2025   2700    664274   42.64    2      646.736
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        2       2025   2700    626708   42.42    2      734.872
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        3       2025   2700    616320   42.96    1      1932.776
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        4       2025   2700    618856   42.83    1      1934.361
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        5       2025   2700    615274   42.71    1      2127.417
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        6       2025   2700    603264   42.75    1      2563.702
pexels-mavihnt-38213559.jpg                     lossless  libwebp     0       2560   1706    4156742  -        7      164.213
pexels-mavihnt-38213559.jpg                     lossless  libwebp     1       2560   1706    3973816  -        2      593.977
pexels-mavihnt-38213559.jpg                     lossless  libwebp     2       2560   1706    3988280  -        2      748.704
pexels-mavihnt-38213559.jpg                     lossless  libwebp     3       2560   1706    3487548  -        2      917.986
pexels-mavihnt-38213559.jpg                     lossless  libwebp     4       2560   1706    3488224  -        1      1001.068
pexels-mavihnt-38213559.jpg                     lossless  libwebp     5       2560   1706    3482892  -        1      1112.834
pexels-mavihnt-38213559.jpg                     lossless  libwebp     6       2560   1706    3482480  -        1      1607.294
pexels-mavihnt-38213559.jpg                     lossless  libwebp     7       2560   1706    3481722  -        1      2592.256
pexels-mavihnt-38213559.jpg                     lossless  libwebp     8       2560   1706    3485234  -        1      2975.239
pexels-mavihnt-38213559.jpg                     lossless  libwebp     9       2560   1706    3485216  -        1      14855.696
pexels-mavihnt-38213559.jpg                     lossless  nativewebp  0       2560   1706    4004276  -        1      1982.115
pexels-mavihnt-38213559.jpg                     lossless  nativewebp  4       2560   1706    4007402  -        1      2047.846
pexels-mavihnt-38213559.jpg                     lossless  nativewebp  6       2560   1706    4009534  -        1      2294.090
pexels-mavihnt-38213559.jpg                     lossless  ours        0       2560   1706    3667900  -        4      319.353
pexels-mavihnt-38213559.jpg                     lossless  ours        1       2560   1706    3667900  -        3      384.253
pexels-mavihnt-38213559.jpg                     lossless  ours        2       2560   1706    3515976  -        1      1002.607
pexels-mavihnt-38213559.jpg                     lossless  ours        3       2560   1706    3515976  -        1      1102.983
pexels-mavihnt-38213559.jpg                     lossless  ours        4       2560   1706    3515976  -        1      1353.984
pexels-mavihnt-38213559.jpg                     lossless  ours        5       2560   1706    3515976  -        1      1786.533
pexels-mavihnt-38213559.jpg                     lossless  ours        6       2560   1706    3509912  -        1      2317.438
pexels-mavihnt-38213559.jpg                     lossless  wasm        0       2560   1706    4116810  -        1      1458.936
pexels-mavihnt-38213559.jpg                     lossless  wasm        1       2560   1706    3484606  -        1      3964.235
pexels-mavihnt-38213559.jpg                     lossless  wasm        2       2560   1706    3484606  -        1      3847.363
pexels-mavihnt-38213559.jpg                     lossless  wasm        3       2560   1706    3484606  -        1      3846.818
pexels-mavihnt-38213559.jpg                     lossless  wasm        4       2560   1706    3482480  -        1      4194.880
pexels-mavihnt-38213559.jpg                     lossless  wasm        5       2560   1706    3488584  -        1      5166.540
pexels-mavihnt-38213559.jpg                     lossless  wasm        6       2560   1706    3488584  -        1      4838.671
pexels-mavihnt-38213559.jpg                     lossy     libwebp     0       2560   1706    983360   40.87    9      120.033
pexels-mavihnt-38213559.jpg                     lossy     libwebp     1       2560   1706    976610   40.88    7      155.245
pexels-mavihnt-38213559.jpg                     lossy     libwebp     2       2560   1706    935824   40.67    6      168.819
pexels-mavihnt-38213559.jpg                     lossy     libwebp     3       2560   1706    931074   41.15    3      344.656
pexels-mavihnt-38213559.jpg                     lossy     libwebp     4       2560   1706    934782   41.17    3      350.788
pexels-mavihnt-38213559.jpg                     lossy     libwebp     5       2560   1706    933892   41.05    3      406.182
pexels-mavihnt-38213559.jpg                     lossy     libwebp     6       2560   1706    925032   41.14    2      970.381
pexels-mavihnt-38213559.jpg                     lossy     ours        0       2560   1706    1037210  40.99    5      213.488
pexels-mavihnt-38213559.jpg                     lossy     ours        1       2560   1706    1019438  41.05    4      283.044
pexels-mavihnt-38213559.jpg                     lossy     ours        2       2560   1706    1019438  41.05    4      282.533
pexels-mavihnt-38213559.jpg                     lossy     ours        3       2560   1706    987826   41.15    3      473.623
pexels-mavihnt-38213559.jpg                     lossy     ours        4       2560   1706    987826   41.15    3      469.713
pexels-mavihnt-38213559.jpg                     lossy     ours        5       2560   1706    987826   41.15    3      474.193
pexels-mavihnt-38213559.jpg                     lossy     ours        6       2560   1706    952402   41.01    2      561.588
pexels-mavihnt-38213559.jpg                     lossy     ours        7       2560   1706    952402   41.01    2      582.292
pexels-mavihnt-38213559.jpg                     lossy     ours        8       2560   1706    948446   41.27    2      912.283
pexels-mavihnt-38213559.jpg                     lossy     ours        9       2560   1706    948446   41.27    1      1662.650
pexels-mavihnt-38213559.jpg                     lossy     wasm        0       2560   1706    983360   40.87    3      410.797
pexels-mavihnt-38213559.jpg                     lossy     wasm        1       2560   1706    976610   40.88    2      581.610
pexels-mavihnt-38213559.jpg                     lossy     wasm        2       2560   1706    935824   40.67    2      650.688
pexels-mavihnt-38213559.jpg                     lossy     wasm        3       2560   1706    931074   41.15    1      1790.869
pexels-mavihnt-38213559.jpg                     lossy     wasm        4       2560   1706    934782   41.17    1      1795.515
pexels-mavihnt-38213559.jpg                     lossy     wasm        5       2560   1706    933892   41.05    1      1992.251
pexels-mavihnt-38213559.jpg                     lossy     wasm        6       2560   1706    925032   41.14    1      2706.900
pexels-steve-15267299.jpg                       lossless  libwebp     0       2095   3000    2603112  -        5      209.803
pexels-steve-15267299.jpg                       lossless  libwebp     1       2095   3000    2557750  -        2      871.018
pexels-steve-15267299.jpg                       lossless  libwebp     2       2095   3000    2550344  -        1      1055.975
pexels-steve-15267299.jpg                       lossless  libwebp     3       2095   3000    2104000  -        1      1330.400
pexels-steve-15267299.jpg                       lossless  libwebp     4       2095   3000    2104010  -        1      1461.017
pexels-steve-15267299.jpg                       lossless  libwebp     5       2095   3000    2104010  -        1      1592.889
pexels-steve-15267299.jpg                       lossless  libwebp     6       2095   3000    2104690  -        1      2145.825
pexels-steve-15267299.jpg                       lossless  libwebp     7       2095   3000    2096804  -        1      2742.719
pexels-steve-15267299.jpg                       lossless  libwebp     8       2095   3000    2111270  -        1      3798.941
pexels-steve-15267299.jpg                       lossless  libwebp     9       2095   3000    2111200  -        1      22009.887
pexels-steve-15267299.jpg                       lossless  nativewebp  0       2095   3000    2635282  -        1      2481.067
pexels-steve-15267299.jpg                       lossless  nativewebp  4       2095   3000    2627962  -        1      2535.068
pexels-steve-15267299.jpg                       lossless  nativewebp  6       2095   3000    2622528  -        1      2961.496
pexels-steve-15267299.jpg                       lossless  ours        0       2095   3000    2233424  -        3      403.546
pexels-steve-15267299.jpg                       lossless  ours        1       2095   3000    2224752  -        2      508.716
pexels-steve-15267299.jpg                       lossless  ours        2       2095   3000    2045866  -        1      1214.747
pexels-steve-15267299.jpg                       lossless  ours        3       2095   3000    2045866  -        1      1390.327
pexels-steve-15267299.jpg                       lossless  ours        4       2095   3000    2036060  -        1      1716.744
pexels-steve-15267299.jpg                       lossless  ours        5       2095   3000    2030150  -        1      2381.431
pexels-steve-15267299.jpg                       lossless  ours        6       2095   3000    2024034  -        1      3007.312
pexels-steve-15267299.jpg                       lossless  wasm        0       2095   3000    2546112  -        1      1513.177
pexels-steve-15267299.jpg                       lossless  wasm        1       2095   3000    2103668  -        1      4906.704
pexels-steve-15267299.jpg                       lossless  wasm        2       2095   3000    2103668  -        1      4896.370
pexels-steve-15267299.jpg                       lossless  wasm        3       2095   3000    2103668  -        1      4911.080
pexels-steve-15267299.jpg                       lossless  wasm        4       2095   3000    2104690  -        1      5195.465
pexels-steve-15267299.jpg                       lossless  wasm        5       2095   3000    2119370  -        1      6448.464
pexels-steve-15267299.jpg                       lossless  wasm        6       2095   3000    2119370  -        1      6051.467
pexels-steve-15267299.jpg                       lossy     libwebp     0       2095   3000    284570   44.34    10     105.166
pexels-steve-15267299.jpg                       lossy     libwebp     1       2095   3000    275444   44.38    8      140.654
pexels-steve-15267299.jpg                       lossy     libwebp     2       2095   3000    261602   44.30    8      138.733
pexels-steve-15267299.jpg                       lossy     libwebp     3       2095   3000    255364   44.59    3      344.809
pexels-steve-15267299.jpg                       lossy     libwebp     4       2095   3000    256038   44.58    3      339.467
pexels-steve-15267299.jpg                       lossy     libwebp     5       2095   3000    253192   44.51    3      374.281
pexels-steve-15267299.jpg                       lossy     libwebp     6       2095   3000    247610   44.50    2      510.235
pexels-steve-15267299.jpg                       lossy     ours        0       2095   3000    294606   43.91    7      155.701
pexels-steve-15267299.jpg                       lossy     ours        1       2095   3000    287430   44.01    5      231.085
pexels-steve-15267299.jpg                       lossy     ours        2       2095   3000    287430   44.01    5      234.863
pexels-steve-15267299.jpg                       lossy     ours        3       2095   3000    264092   44.12    4      314.239
pexels-steve-15267299.jpg                       lossy     ours        4       2095   3000    264092   44.12    4      315.006
pexels-steve-15267299.jpg                       lossy     ours        5       2095   3000    264092   44.12    4      314.089
pexels-steve-15267299.jpg                       lossy     ours        6       2095   3000    247744   44.09    3      339.862
pexels-steve-15267299.jpg                       lossy     ours        7       2095   3000    247744   44.09    3      333.909
pexels-steve-15267299.jpg                       lossy     ours        8       2095   3000    252782   44.23    2      647.787
pexels-steve-15267299.jpg                       lossy     ours        9       2095   3000    252782   44.41    1      1142.451
pexels-steve-15267299.jpg                       lossy     wasm        0       2095   3000    284570   44.34    3      484.224
pexels-steve-15267299.jpg                       lossy     wasm        1       2095   3000    275444   44.38    2      661.137
pexels-steve-15267299.jpg                       lossy     wasm        2       2095   3000    261602   44.30    2      618.755
pexels-steve-15267299.jpg                       lossy     wasm        3       2095   3000    255364   44.59    1      1868.155
pexels-steve-15267299.jpg                       lossy     wasm        4       2095   3000    256038   44.58    1      1886.916
pexels-steve-15267299.jpg                       lossy     wasm        5       2095   3000    253192   44.51    1      2022.758
pexels-steve-15267299.jpg                       lossy     wasm        6       2095   3000    247610   44.50    1      2101.679
pexels-steve-29626041.jpg                       lossless  libwebp     0       2560   1440    367064   -        17     59.333
pexels-steve-29626041.jpg                       lossless  libwebp     1       2560   1440    360562   -        3      384.202
pexels-steve-29626041.jpg                       lossless  libwebp     2       2560   1440    309376   -        2      523.499
pexels-steve-29626041.jpg                       lossless  libwebp     3       2560   1440    292034   -        2      553.222
pexels-steve-29626041.jpg                       lossless  libwebp     4       2560   1440    287166   -        2      591.363
pexels-steve-29626041.jpg                       lossless  libwebp     5       2560   1440    287662   -        2      624.983
pexels-steve-29626041.jpg                       lossless  libwebp     6       2560   1440    283988   -        2      728.721
pexels-steve-29626041.jpg                       lossless  libwebp     7       2560   1440    282342   -        2      766.230
pexels-steve-29626041.jpg                       lossless  libwebp     8       2560   1440    294462   -        1      1010.306
pexels-steve-29626041.jpg                       lossless  libwebp     9       2560   1440    292264   -        1      3923.227
pexels-steve-29626041.jpg                       lossless  nativewebp  0       2560   1440    378544   -        1      1108.279
pexels-steve-29626041.jpg                       lossless  nativewebp  4       2560   1440    378536   -        1      1238.971
pexels-steve-29626041.jpg                       lossless  nativewebp  6       2560   1440    379050   -        1      1373.721
pexels-steve-29626041.jpg                       lossless  ours        0       2560   1440    312250   -        6      177.069
pexels-steve-29626041.jpg                       lossless  ours        1       2560   1440    312250   -        5      244.635
pexels-steve-29626041.jpg                       lossless  ours        2       2560   1440    300532   -        3      485.499
pexels-steve-29626041.jpg                       lossless  ours        3       2560   1440    299726   -        2      601.523
pexels-steve-29626041.jpg                       lossless  ours        4       2560   1440    299726   -        2      799.630
pexels-steve-29626041.jpg                       lossless  ours        5       2560   1440    297802   -        1      1168.064
pexels-steve-29626041.jpg                       lossless  ours        6       2560   1440    295168   -        1      1323.108
pexels-steve-29626041.jpg                       lossless  wasm        0       2560   1440    346824   -        3      368.384
pexels-steve-29626041.jpg                       lossless  wasm        1       2560   1440    283552   -        1      1929.640
pexels-steve-29626041.jpg                       lossless  wasm        2       2560   1440    283552   -        1      2011.849
pexels-steve-29626041.jpg                       lossless  wasm        3       2560   1440    283552   -        1      2006.127
pexels-steve-29626041.jpg                       lossless  wasm        4       2560   1440    283988   -        1      2189.237
pexels-steve-29626041.jpg                       lossless  wasm        5       2560   1440    296596   -        1      2943.746
pexels-steve-29626041.jpg                       lossless  wasm        6       2560   1440    296596   -        1      2881.384
pexels-steve-29626041.jpg                       lossy     libwebp     0       2560   1440    47614    49.05    18     56.520
pexels-steve-29626041.jpg                       lossy     libwebp     1       2560   1440    47302    49.05    14     71.487
pexels-steve-29626041.jpg                       lossy     libwebp     2       2560   1440    39488    49.30    17     60.688
pexels-steve-29626041.jpg                       lossy     libwebp     3       2560   1440    39096    49.71    7      157.927
pexels-steve-29626041.jpg                       lossy     libwebp     4       2560   1440    39310    49.71    7      161.463
pexels-steve-29626041.jpg                       lossy     libwebp     5       2560   1440    38754    49.65    6      193.734
pexels-steve-29626041.jpg                       lossy     libwebp     6       2560   1440    38252    49.65    5      205.803
pexels-steve-29626041.jpg                       lossy     ours        0       2560   1440    45686    49.31    15     70.325
pexels-steve-29626041.jpg                       lossy     ours        1       2560   1440    44820    49.35    10     106.715
pexels-steve-29626041.jpg                       lossy     ours        2       2560   1440    44820    49.35    10     106.377
pexels-steve-29626041.jpg                       lossy     ours        3       2560   1440    39426    49.40    10     110.417
pexels-steve-29626041.jpg                       lossy     ours        4       2560   1440    39426    49.40    10     109.874
pexels-steve-29626041.jpg                       lossy     ours        5       2560   1440    39426    49.40    10     110.090
pexels-steve-29626041.jpg                       lossy     ours        6       2560   1440    37562    49.37    9      122.527
pexels-steve-29626041.jpg                       lossy     ours        7       2560   1440    37562    49.37    9      120.686
pexels-steve-29626041.jpg                       lossy     ours        8       2560   1440    36254    49.45    6      200.178
pexels-steve-29626041.jpg                       lossy     ours        9       2560   1440    36254    49.62    3      372.782
pexels-steve-29626041.jpg                       lossy     wasm        0       2560   1440    47614    49.05    4      252.482
pexels-steve-29626041.jpg                       lossy     wasm        1       2560   1440    47302    49.05    3      363.461
pexels-steve-29626041.jpg                       lossy     wasm        2       2560   1440    39488    49.30    4      297.035
pexels-steve-29626041.jpg                       lossy     wasm        3       2560   1440    39096    49.71    2      913.508
pexels-steve-29626041.jpg                       lossy     wasm        4       2560   1440    39310    49.71    2      928.962
pexels-steve-29626041.jpg                       lossy     wasm        5       2560   1440    38754    49.65    1      1034.503
pexels-steve-29626041.jpg                       lossy     wasm        6       2560   1440    38252    49.65    2      986.074
pexels-toulouse-10807703.jpg                    lossless  libwebp     0       1400   2100    3044142  -        11     93.827
pexels-toulouse-10807703.jpg                    lossless  libwebp     1       1400   2100    2807602  -        3      486.424
pexels-toulouse-10807703.jpg                    lossless  libwebp     2       1400   2100    2780320  -        2      574.421
pexels-toulouse-10807703.jpg                    lossless  libwebp     3       1400   2100    2693662  -        2      633.471
pexels-toulouse-10807703.jpg                    lossless  libwebp     4       1400   2100    2693662  -        2      680.794
pexels-toulouse-10807703.jpg                    lossless  libwebp     5       1400   2100    2691936  -        2      759.891
pexels-toulouse-10807703.jpg                    lossless  libwebp     6       1400   2100    2688812  -        1      1164.320
pexels-toulouse-10807703.jpg                    lossless  libwebp     7       1400   2100    2688410  -        1      1474.674
pexels-toulouse-10807703.jpg                    lossless  libwebp     8       1400   2100    2686410  -        1      2138.067
pexels-toulouse-10807703.jpg                    lossless  libwebp     9       1400   2100    2685794  -        1      9341.791
pexels-toulouse-10807703.jpg                    lossless  nativewebp  0       1400   2100    2989804  -        1      1155.737
pexels-toulouse-10807703.jpg                    lossless  nativewebp  4       1400   2100    2992710  -        1      1081.213
pexels-toulouse-10807703.jpg                    lossless  nativewebp  6       1400   2100    2996214  -        1      1254.617
pexels-toulouse-10807703.jpg                    lossless  ours        0       1400   2100    2868724  -        5      212.868
pexels-toulouse-10807703.jpg                    lossless  ours        1       1400   2100    2868724  -        4      257.224
pexels-toulouse-10807703.jpg                    lossless  ours        2       1400   2100    2718058  -        2      650.799
pexels-toulouse-10807703.jpg                    lossless  ours        3       1400   2100    2718058  -        2      775.693
pexels-toulouse-10807703.jpg                    lossless  ours        4       1400   2100    2718058  -        2      939.242
pexels-toulouse-10807703.jpg                    lossless  ours        5       1400   2100    2718058  -        1      1234.879
pexels-toulouse-10807703.jpg                    lossless  ours        6       1400   2100    2721246  -        1      1616.140
pexels-toulouse-10807703.jpg                    lossless  wasm        0       1400   2100    3029106  -        2      708.787
pexels-toulouse-10807703.jpg                    lossless  wasm        1       1400   2100    2690886  -        1      2920.168
pexels-toulouse-10807703.jpg                    lossless  wasm        2       1400   2100    2690886  -        1      2888.061
pexels-toulouse-10807703.jpg                    lossless  wasm        3       1400   2100    2690886  -        1      2900.424
pexels-toulouse-10807703.jpg                    lossless  wasm        4       1400   2100    2688812  -        1      3114.203
pexels-toulouse-10807703.jpg                    lossless  wasm        5       1400   2100    2685806  -        1      5359.222
pexels-toulouse-10807703.jpg                    lossless  wasm        6       1400   2100    2685806  -        1      4021.615
pexels-toulouse-10807703.jpg                    lossy     libwebp     0       1400   2100    857060   39.84    12     90.473
pexels-toulouse-10807703.jpg                    lossy     libwebp     1       1400   2100    812040   39.84    9      117.740
pexels-toulouse-10807703.jpg                    lossy     libwebp     2       1400   2100    796814   39.60    8      125.936
pexels-toulouse-10807703.jpg                    lossy     libwebp     3       1400   2100    783482   39.95    5      244.110
pexels-toulouse-10807703.jpg                    lossy     libwebp     4       1400   2100    784734   39.96    5      242.486
pexels-toulouse-10807703.jpg                    lossy     libwebp     5       1400   2100    768426   39.81    4      284.398
pexels-toulouse-10807703.jpg                    lossy     libwebp     6       1400   2100    758466   39.84    2      661.863
pexels-toulouse-10807703.jpg                    lossy     ours        0       1400   2100    854742   39.93    6      173.929
pexels-toulouse-10807703.jpg                    lossy     ours        1       1400   2100    844538   39.95    5      223.909
pexels-toulouse-10807703.jpg                    lossy     ours        2       1400   2100    844538   39.95    5      215.142
pexels-toulouse-10807703.jpg                    lossy     ours        3       1400   2100    812788   40.06    4      327.492
pexels-toulouse-10807703.jpg                    lossy     ours        4       1400   2100    812788   40.06    4      329.583
pexels-toulouse-10807703.jpg                    lossy     ours        5       1400   2100    812788   40.06    4      326.758
pexels-toulouse-10807703.jpg                    lossy     ours        6       1400   2100    778956   39.92    3      381.433
pexels-toulouse-10807703.jpg                    lossy     ours        7       1400   2100    778956   39.92    3      376.175
pexels-toulouse-10807703.jpg                    lossy     ours        8       1400   2100    776930   40.10    2      641.057
pexels-toulouse-10807703.jpg                    lossy     ours        9       1400   2100    776930   40.03    2      858.961
pexels-toulouse-10807703.jpg                    lossy     wasm        0       1400   2100    857060   39.84    4      290.633
pexels-toulouse-10807703.jpg                    lossy     wasm        1       1400   2100    812040   39.84    3      407.145
pexels-toulouse-10807703.jpg                    lossy     wasm        2       1400   2100    796814   39.60    3      444.490
pexels-toulouse-10807703.jpg                    lossy     wasm        3       1400   2100    783482   39.95    1      1252.085
pexels-toulouse-10807703.jpg                    lossy     wasm        4       1400   2100    784734   39.96    1      1255.295
pexels-toulouse-10807703.jpg                    lossy     wasm        5       1400   2100    768426   39.81    1      1370.841
pexels-toulouse-10807703.jpg                    lossy     wasm        6       1400   2100    758466   39.84    1      1848.626
```

## arm64 (Apple M4 Pro) / transparent

```
file          mode        engine      width  height  bytes   psnr_db  iters  ms_per_op
1_webp_a.png  lossless    libwebp     400    301     75254   -        19     107.432
1_webp_a.png  lossless    nativewebp  400    301     81934   -        68     29.547
1_webp_a.png  lossless    ours        400    301     79910   -        51     39.967
1_webp_a.png  lossless    wasm        400    301     74350   -        6      386.715
1_webp_a.png  lossy-fast  libwebp     400    301     20134   41.94    500    3.889
1_webp_a.png  lossy-fast  ours        400    301     21748   41.48    105    19.200
1_webp_a.png  lossy-fast  wasm        400    301     20134   41.94    222    9.048
1_webp_a.png  lossy-slow  libwebp     400    301     17032   42.23    2      1656.225
1_webp_a.png  lossy-slow  ours        400    301     18610   42.09    8      256.775
1_webp_a.png  lossy-slow  wasm        400    301     17032   42.23    1      2644.720
2_webp_a.png  lossless    libwebp     386    395     41648   -        16     130.814
2_webp_a.png  lossless    nativewebp  386    395     49924   -        56     36.182
2_webp_a.png  lossless    ours        386    395     43988   -        50     40.200
2_webp_a.png  lossless    wasm        386    395     41428   -        5      447.266
2_webp_a.png  lossy-fast  libwebp     386    395     18844   43.02    500    3.723
2_webp_a.png  lossy-fast  ours        386    395     20230   42.42    95     21.148
2_webp_a.png  lossy-fast  wasm        386    395     18844   43.02    213    9.410
2_webp_a.png  lossy-slow  libwebp     386    395     13994   43.38    1      2007.115
2_webp_a.png  lossy-slow  ours        386    395     14732   42.83    10     217.971
2_webp_a.png  lossy-slow  wasm        386    395     13994   43.38    1      3044.692
3_webp_a.png  lossless    libwebp     800    600     181124  -        13     160.117
3_webp_a.png  lossless    nativewebp  800    600     202886  -        26     77.208
3_webp_a.png  lossless    ours        800    600     188144  -        17     122.048
3_webp_a.png  lossless    wasm        800    600     178918  -        5      498.728
3_webp_a.png  lossy-fast  libwebp     800    600     80676   41.56    114    17.575
3_webp_a.png  lossy-fast  ours        800    600     73228   40.82    39     51.649
3_webp_a.png  lossy-fast  wasm        800    600     80676   41.56    51     39.242
3_webp_a.png  lossy-slow  libwebp     800    600     52308   41.61    1      2684.195
3_webp_a.png  lossy-slow  ours        800    600     59116   41.18    4      636.160
3_webp_a.png  lossy-slow  wasm        800    600     52308   41.61    1      4589.574
4_webp_a.png  lossless    libwebp     421    163     34420   -        33     62.005
4_webp_a.png  lossless    nativewebp  421    163     37928   -        118    16.972
4_webp_a.png  lossless    ours        421    163     38046   -        92     21.972
4_webp_a.png  lossless    wasm        421    163     33558   -        10     200.438
4_webp_a.png  lossy-fast  libwebp     421    163     23740   39.28    500    2.389
4_webp_a.png  lossy-fast  ours        421    163     22468   38.92    165    12.131
4_webp_a.png  lossy-fast  wasm        421    163     23740   39.28    351    5.699
4_webp_a.png  lossy-slow  libwebp     421    163     18758   39.73    3      671.252
4_webp_a.png  lossy-slow  ours        421    163     19190   39.33    11     195.859
4_webp_a.png  lossy-slow  wasm        421    163     18758   39.73    2      1103.639
5_webp_a.png  lossless    libwebp     300    300     140018  -        18     111.448
5_webp_a.png  lossless    nativewebp  300    300     154970  -        84     23.843
5_webp_a.png  lossless    ours        300    300     149708  -        53     38.068
5_webp_a.png  lossless    wasm        300    300     137538  -        6      362.828
5_webp_a.png  lossy-fast  libwebp     300    300     64290   32.77    392    5.111
5_webp_a.png  lossy-fast  ours        300    300     70196   33.07    85     23.768
5_webp_a.png  lossy-fast  wasm        300    300     64290   32.77    184    10.923
5_webp_a.png  lossy-slow  libwebp     300    300     55926   32.67    1      3324.305
5_webp_a.png  lossy-slow  ours        300    300     59610   33.23    8      267.594
5_webp_a.png  lossy-slow  wasm        300    300     55926   32.67    1      5698.167
```

Peak RSS, one encode per process:

```
file          mode        engine      width  height  megapixels  peak_rss_mib  mib_per_mp
1_webp_a.png  lossless    libwebp     400    301     0.12        34.6          287.2
1_webp_a.png  lossless    nativewebp  400    301     0.12        18.9          156.9
1_webp_a.png  lossless    ours        400    301     0.12        20.0          165.9
1_webp_a.png  lossless    wasm        400    301     0.12        59.4          493.5
1_webp_a.png  lossy-fast  libwebp     400    301     0.12        14.5          120.7
1_webp_a.png  lossy-fast  ours        400    301     0.12        19.4          160.8
1_webp_a.png  lossy-fast  wasm        400    301     0.12        25.0          207.5
1_webp_a.png  lossy-slow  libwebp     400    301     0.12        65.6          544.8
1_webp_a.png  lossy-slow  ours        400    301     0.12        29.6          245.7
1_webp_a.png  lossy-slow  wasm        400    301     0.12        102.5         851.3
2_webp_a.png  lossless    libwebp     386    395     0.15        45.3          297.2
2_webp_a.png  lossless    nativewebp  386    395     0.15        20.0          131.4
2_webp_a.png  lossless    ours        386    395     0.15        22.3          146.5
2_webp_a.png  lossless    wasm        386    395     0.15        95.5          626.4
2_webp_a.png  lossy-fast  libwebp     386    395     0.15        15.7          103.0
2_webp_a.png  lossy-fast  ours        386    395     0.15        19.9          130.8
2_webp_a.png  lossy-fast  wasm        386    395     0.15        31.0          203.3
2_webp_a.png  lossy-slow  libwebp     386    395     0.15        103.8         680.5
2_webp_a.png  lossy-slow  ours        386    395     0.15        33.8          221.9
2_webp_a.png  lossy-slow  wasm        386    395     0.15        135.0         885.7
3_webp_a.png  lossless    libwebp     800    600     0.48        40.2          83.9
3_webp_a.png  lossless    nativewebp  800    600     0.48        28.0          58.3
3_webp_a.png  lossless    ours        800    600     0.48        36.6          76.3
3_webp_a.png  lossless    wasm        800    600     0.48        108.4         225.9
3_webp_a.png  lossy-fast  libwebp     800    600     0.48        27.2          56.7
3_webp_a.png  lossy-fast  ours        800    600     0.48        39.5          82.2
3_webp_a.png  lossy-fast  wasm        800    600     0.48        62.5          130.3
3_webp_a.png  lossy-slow  libwebp     800    600     0.48        109.0         227.0
3_webp_a.png  lossy-slow  ours        800    600     0.48        88.5          184.3
3_webp_a.png  lossy-slow  wasm        800    600     0.48        227.4         473.7
4_webp_a.png  lossless    libwebp     421    163     0.07        26.8          390.3
4_webp_a.png  lossless    nativewebp  421    163     0.07        16.7          243.2
4_webp_a.png  lossless    ours        421    163     0.07        17.8          258.9
4_webp_a.png  lossless    wasm        421    163     0.07        39.7          578.8
4_webp_a.png  lossy-fast  libwebp     421    163     0.07        13.0          189.2
4_webp_a.png  lossy-fast  ours        421    163     0.07        15.7          228.4
4_webp_a.png  lossy-fast  wasm        421    163     0.07        20.7          301.5
4_webp_a.png  lossy-slow  libwebp     421    163     0.07        43.1          628.2
4_webp_a.png  lossy-slow  ours        421    163     0.07        24.3          354.1
4_webp_a.png  lossy-slow  wasm        421    163     0.07        86.2          1255.7
5_webp_a.png  lossless    libwebp     300    300     0.09        27.5          305.6
5_webp_a.png  lossless    nativewebp  300    300     0.09        18.4          204.3
5_webp_a.png  lossless    ours        300    300     0.09        20.1          223.8
5_webp_a.png  lossless    wasm        300    300     0.09        49.3          548.3
5_webp_a.png  lossy-fast  libwebp     300    300     0.09        13.6          151.0
5_webp_a.png  lossy-fast  ours        300    300     0.09        17.2          191.7
5_webp_a.png  lossy-fast  wasm        300    300     0.09        25.9          287.3
5_webp_a.png  lossy-slow  libwebp     300    300     0.09        73.4          815.5
5_webp_a.png  lossy-slow  ours        300    300     0.09        26.2          290.8
5_webp_a.png  lossy-slow  wasm        300    300     0.09        108.8         1209.4
```

Decode, one file per mode encoded by libwebp:

```
file          mode      engine   width  height  bytes   psnr_db  iters  ms_per_op
1_webp_a.png  lossless  libwebp  400    301     75254   -        500    1.114
1_webp_a.png  lossless  ours     400    301     75254   -        500    2.129
1_webp_a.png  lossless  wasm     400    301     75254   29.40    500    3.296
1_webp_a.png  lossless  x/image  400    301     75254   -        500    2.365
1_webp_a.png  lossy     libwebp  400    301     17032   -        500    0.665
1_webp_a.png  lossy     ours     400    301     17032   -        500    2.249
1_webp_a.png  lossy     wasm     400    301     17032   29.63    500    2.250
1_webp_a.png  lossy     x/image  400    301     17032   29.63    500    2.870
2_webp_a.png  lossless  libwebp  386    395     41648   -        500    0.872
2_webp_a.png  lossless  ours     386    395     41648   -        500    1.754
2_webp_a.png  lossless  wasm     386    395     41648   25.35    500    3.473
2_webp_a.png  lossless  x/image  386    395     41648   -        500    1.930
2_webp_a.png  lossy     libwebp  386    395     13994   -        500    0.773
2_webp_a.png  lossy     ours     386    395     13994   -        500    2.567
2_webp_a.png  lossy     wasm     386    395     13994   25.46    500    2.790
2_webp_a.png  lossy     x/image  386    395     13994   25.46    500    3.137
3_webp_a.png  lossless  libwebp  800    600     181124  -        500    3.040
3_webp_a.png  lossless  ours     800    600     181124  -        310    6.459
3_webp_a.png  lossless  wasm     800    600     181124  27.66    191    10.493
3_webp_a.png  lossless  x/image  800    600     181124  -        296    6.768
3_webp_a.png  lossy     libwebp  800    600     52308   -        500    2.275
3_webp_a.png  lossy     ours     800    600     52308   -        277    7.223
3_webp_a.png  lossy     wasm     800    600     52308   27.81    245    8.186
3_webp_a.png  lossy     x/image  800    600     52308   27.81    204    9.814
4_webp_a.png  lossless  libwebp  421    163     34420   -        500    0.471
4_webp_a.png  lossless  ours     421    163     34420   -        500    1.016
4_webp_a.png  lossless  wasm     421    163     34420   30.27    500    1.696
4_webp_a.png  lossless  x/image  421    163     34420   -        500    1.043
4_webp_a.png  lossy     libwebp  421    163     18758   -        500    0.551
4_webp_a.png  lossy     ours     421    163     18758   -        500    1.696
4_webp_a.png  lossy     wasm     421    163     18758   31.62    500    1.635
4_webp_a.png  lossy     x/image  421    163     18758   31.62    500    2.026
5_webp_a.png  lossless  libwebp  300    300     140018  -        500    1.381
5_webp_a.png  lossless  ours     300    300     140018  -        500    2.962
5_webp_a.png  lossless  wasm     300    300     140018  24.63    500    3.503
5_webp_a.png  lossless  x/image  300    300     140018  -        500    3.284
5_webp_a.png  lossy     libwebp  300    300     55926   -        500    1.759
5_webp_a.png  lossy     ours     300    300     55926   -        469    4.272
5_webp_a.png  lossy     wasm     300    300     55926   25.61    500    3.698
5_webp_a.png  lossy     x/image  300    300     55926   25.61    391    5.120
```

Effort sweep, every setting of every engine:

```
file          mode      engine      effort  width  height  bytes   psnr_db  iters  ms_per_op
1_webp_a.png  lossless  libwebp     0       400    301     89358   -        460    2.173
1_webp_a.png  lossless  libwebp     1       400    301     80522   -        67     15.137
1_webp_a.png  lossless  libwebp     2       400    301     77132   -        41     24.714
1_webp_a.png  lossless  libwebp     3       400    301     77242   -        27     38.446
1_webp_a.png  lossless  libwebp     4       400    301     76798   -        26     38.738
1_webp_a.png  lossless  libwebp     5       400    301     75278   -        11     92.430
1_webp_a.png  lossless  libwebp     6       400    301     75254   -        10     108.020
1_webp_a.png  lossless  libwebp     7       400    301     75162   -        10     110.733
1_webp_a.png  lossless  libwebp     8       400    301     73858   -        4      260.031
1_webp_a.png  lossless  libwebp     9       400    301     71932   -        1      1881.546
1_webp_a.png  lossless  nativewebp  0       400    301     83958   -        56     17.892
1_webp_a.png  lossless  nativewebp  4       400    301     81934   -        34     29.668
1_webp_a.png  lossless  nativewebp  6       400    301     81934   -        35     29.233
1_webp_a.png  lossless  ours        0       400    301     83468   -        183    5.489
1_webp_a.png  lossless  ours        1       400    301     82496   -        154    6.494
1_webp_a.png  lossless  ours        2       400    301     82476   -        60     16.806
1_webp_a.png  lossless  ours        3       400    301     82476   -        50     20.279
1_webp_a.png  lossless  ours        4       400    301     81832   -        43     23.475
1_webp_a.png  lossless  ours        5       400    301     80522   -        32     31.860
1_webp_a.png  lossless  ours        6       400    301     79910   -        26     38.571
1_webp_a.png  lossless  wasm        0       400    301     88930   -        150    6.668
1_webp_a.png  lossless  wasm        1       400    301     78950   -        24     43.417
1_webp_a.png  lossless  wasm        2       400    301     77160   -        19     54.913
1_webp_a.png  lossless  wasm        3       400    301     76576   -        13     79.023
1_webp_a.png  lossless  wasm        4       400    301     75254   -        6      172.101
1_webp_a.png  lossless  wasm        5       400    301     73842   -        3      405.113
1_webp_a.png  lossless  wasm        6       400    301     74350   -        3      381.072
1_webp_a.png  lossy     libwebp     0       400    301     20134   41.94    273    3.671
1_webp_a.png  lossy     libwebp     1       400    301     18496   41.98    223    4.487
1_webp_a.png  lossy     libwebp     2       400    301     17446   41.40    217    4.616
1_webp_a.png  lossy     libwebp     3       400    301     17122   42.23    134    7.515
1_webp_a.png  lossy     libwebp     4       400    301     17142   42.25    109    9.227
1_webp_a.png  lossy     libwebp     5       400    301     17228   42.22    80     12.543
1_webp_a.png  lossy     libwebp     6       400    301     17032   42.23    1      1645.209
1_webp_a.png  lossy     ours        0       400    301     21748   41.48    54     18.733
1_webp_a.png  lossy     ours        1       400    301     21436   41.55    34     30.067
1_webp_a.png  lossy     ours        2       400    301     21448   41.55    24     43.179
1_webp_a.png  lossy     ours        3       400    301     19330   41.88    16     65.659
1_webp_a.png  lossy     ours        4       400    301     19330   41.88    8      126.626
1_webp_a.png  lossy     ours        5       400    301     19330   41.88    7      162.577
1_webp_a.png  lossy     ours        6       400    301     18766   41.80    5      200.939
1_webp_a.png  lossy     ours        7       400    301     18766   41.80    5      200.387
1_webp_a.png  lossy     ours        8       400    301     18610   42.09    4      254.078
1_webp_a.png  lossy     ours        9       400    301     18610   42.09    4      252.991
1_webp_a.png  lossy     wasm        0       400    301     20134   41.94    107    9.374
1_webp_a.png  lossy     wasm        1       400    301     18496   41.98    86     11.652
1_webp_a.png  lossy     wasm        2       400    301     17446   41.40    83     12.061
1_webp_a.png  lossy     wasm        3       400    301     17122   42.23    39     25.688
1_webp_a.png  lossy     wasm        4       400    301     17142   42.25    36     28.375
1_webp_a.png  lossy     wasm        5       400    301     17228   42.22    29     34.778
1_webp_a.png  lossy     wasm        6       400    301     17032   42.23    1      2615.996
2_webp_a.png  lossless  libwebp     0       386    395     57368   -        370    2.705
2_webp_a.png  lossless  libwebp     1       386    395     49780   -        52     19.585
2_webp_a.png  lossless  libwebp     2       386    395     44900   -        30     34.266
2_webp_a.png  lossless  libwebp     3       386    395     42970   -        20     50.934
2_webp_a.png  lossless  libwebp     4       386    395     43076   -        20     52.029
2_webp_a.png  lossless  libwebp     5       386    395     41610   -        9      116.016
2_webp_a.png  lossless  libwebp     6       386    395     41648   -        8      130.856
2_webp_a.png  lossless  libwebp     7       386    395     41610   -        8      135.370
2_webp_a.png  lossless  libwebp     8       386    395     41428   -        4      330.126
2_webp_a.png  lossless  libwebp     9       386    395     40058   -        1      1598.342
2_webp_a.png  lossless  nativewebp  0       386    395     51760   -        48     20.937
2_webp_a.png  lossless  nativewebp  4       386    395     49924   -        28     35.781
2_webp_a.png  lossless  nativewebp  6       386    395     49924   -        28     35.930
2_webp_a.png  lossless  ours        0       386    395     48054   -        190    5.281
2_webp_a.png  lossless  ours        1       386    395     47046   -        153    6.566
2_webp_a.png  lossless  ours        2       386    395     46876   -        61     16.444
2_webp_a.png  lossless  ours        3       386    395     46078   -        48     20.916
2_webp_a.png  lossless  ours        4       386    395     45618   -        42     24.123
2_webp_a.png  lossless  ours        5       386    395     44412   -        31     33.228
2_webp_a.png  lossless  ours        6       386    395     43988   -        25     40.565
2_webp_a.png  lossless  wasm        0       386    395     56180   -        105    9.564
2_webp_a.png  lossless  wasm        1       386    395     44974   -        17     60.151
2_webp_a.png  lossless  wasm        2       386    395     43436   -        13     78.092
2_webp_a.png  lossless  wasm        3       386    395     43056   -        11     97.629
2_webp_a.png  lossless  wasm        4       386    395     41648   -        6      198.142
2_webp_a.png  lossless  wasm        5       386    395     41428   -        3      475.772
2_webp_a.png  lossless  wasm        6       386    395     41428   -        3      451.451
2_webp_a.png  lossy     libwebp     0       386    395     18844   43.02    266    3.767
2_webp_a.png  lossy     libwebp     1       386    395     16172   43.00    215    4.655
2_webp_a.png  lossy     libwebp     2       386    395     15124   42.88    217    4.627
2_webp_a.png  lossy     libwebp     3       386    395     14534   43.45    130    7.746
2_webp_a.png  lossy     libwebp     4       386    395     14046   43.36    54     18.656
2_webp_a.png  lossy     libwebp     5       386    395     14000   43.53    5      212.575
2_webp_a.png  lossy     libwebp     6       386    395     13994   43.38    1      1995.165
2_webp_a.png  lossy     ours        0       386    395     20230   42.42    49     20.547
2_webp_a.png  lossy     ours        1       386    395     19772   42.54    32     31.990
2_webp_a.png  lossy     ours        2       386    395     19606   42.54    22     46.991
2_webp_a.png  lossy     ours        3       386    395     15772   42.82    16     63.744
2_webp_a.png  lossy     ours        4       386    395     15772   42.82    8      132.641
2_webp_a.png  lossy     ours        5       386    395     15484   42.82    7      159.588
2_webp_a.png  lossy     ours        6       386    395     15104   42.77    6      188.681
2_webp_a.png  lossy     ours        7       386    395     15104   42.77    6      188.896
2_webp_a.png  lossy     ours        8       386    395     14732   42.83    5      220.418
2_webp_a.png  lossy     ours        9       386    395     14732   42.83    5      219.079
2_webp_a.png  lossy     wasm        0       386    395     18844   43.02    103    9.724
2_webp_a.png  lossy     wasm        1       386    395     16172   43.00    79     12.710
2_webp_a.png  lossy     wasm        2       386    395     15124   42.88    78     12.882
2_webp_a.png  lossy     wasm        3       386    395     14534   43.45    36     28.218
2_webp_a.png  lossy     wasm        4       386    395     14046   43.36    21     48.361
2_webp_a.png  lossy     wasm        5       386    395     14000   43.53    4      324.371
2_webp_a.png  lossy     wasm        6       386    395     13994   43.38    1      3019.780
3_webp_a.png  lossless  libwebp     0       800    600     266952  -        138    7.294
3_webp_a.png  lossless  libwebp     1       800    600     189276  -        18     56.335
3_webp_a.png  lossless  libwebp     2       800    600     184600  -        12     87.750
3_webp_a.png  lossless  libwebp     3       800    600     181092  -        8      136.662
3_webp_a.png  lossless  libwebp     4       800    600     181208  -        8      139.963
3_webp_a.png  lossless  libwebp     5       800    600     181208  -        8      139.344
3_webp_a.png  lossless  libwebp     6       800    600     181124  -        7      157.915
3_webp_a.png  lossless  libwebp     7       800    600     179208  -        6      190.302
3_webp_a.png  lossless  libwebp     8       800    600     177714  -        3      374.677
3_webp_a.png  lossless  libwebp     9       800    600     174890  -        1      2144.520
3_webp_a.png  lossless  nativewebp  0       800    600     203914  -        15     68.796
3_webp_a.png  lossless  nativewebp  4       800    600     202886  -        13     77.104
3_webp_a.png  lossless  nativewebp  6       800    600     202886  -        13     77.307
3_webp_a.png  lossless  ours        0       800    600     194012  -        61     16.408
3_webp_a.png  lossless  ours        1       800    600     193846  -        50     20.393
3_webp_a.png  lossless  ours        2       800    600     193450  -        21     49.953
3_webp_a.png  lossless  ours        3       800    600     193450  -        17     62.261
3_webp_a.png  lossless  ours        4       800    600     191848  -        14     76.903
3_webp_a.png  lossless  ours        5       800    600     189530  -        10     101.912
3_webp_a.png  lossless  ours        6       800    600     188144  -        9      122.975
3_webp_a.png  lossless  wasm        0       800    600     267190  -        47     21.409
3_webp_a.png  lossless  wasm        1       800    600     186782  -        7      157.632
3_webp_a.png  lossless  wasm        2       800    600     183524  -        6      186.192
3_webp_a.png  lossless  wasm        3       800    600     181124  -        4      267.612
3_webp_a.png  lossless  wasm        4       800    600     181124  -        4      267.208
3_webp_a.png  lossless  wasm        5       800    600     178918  -        2      527.268
3_webp_a.png  lossless  wasm        6       800    600     178918  -        3      496.352
3_webp_a.png  lossy     libwebp     0       800    600     80676   41.56    58     17.330
3_webp_a.png  lossy     libwebp     1       800    600     63372   41.57    26     39.299
3_webp_a.png  lossy     libwebp     2       800    600     58678   41.23    26     39.177
3_webp_a.png  lossy     libwebp     3       800    600     55434   41.60    21     47.887
3_webp_a.png  lossy     libwebp     4       800    600     55128   41.61    18     57.701
3_webp_a.png  lossy     libwebp     5       800    600     53732   41.57    5      203.294
3_webp_a.png  lossy     libwebp     6       800    600     52308   41.61    1      2655.979
3_webp_a.png  lossy     ours        0       800    600     73228   40.82    20     50.980
3_webp_a.png  lossy     ours        1       800    600     72536   40.91    16     63.524
3_webp_a.png  lossy     ours        2       800    600     71036   40.91    9      119.839
3_webp_a.png  lossy     ours        3       800    600     64238   41.12    7      159.136
3_webp_a.png  lossy     ours        4       800    600     64238   41.12    3      335.633
3_webp_a.png  lossy     ours        5       800    600     60602   41.12    3      420.328
3_webp_a.png  lossy     ours        6       800    600     59934   41.31    2      546.658
3_webp_a.png  lossy     ours        7       800    600     59934   41.31    2      551.886
3_webp_a.png  lossy     ours        8       800    600     59116   41.18    2      564.393
3_webp_a.png  lossy     ours        9       800    600     59116   41.18    2      630.872
3_webp_a.png  lossy     wasm        0       800    600     80676   41.56    26     38.523
3_webp_a.png  lossy     wasm        1       800    600     63372   41.57    12     86.176
3_webp_a.png  lossy     wasm        2       800    600     58678   41.23    12     87.710
3_webp_a.png  lossy     wasm        3       800    600     55434   41.60    9      124.436
3_webp_a.png  lossy     wasm        4       800    600     55128   41.61    7      146.135
3_webp_a.png  lossy     wasm        5       800    600     53732   41.57    3      376.972
3_webp_a.png  lossy     wasm        6       800    600     52308   41.61    1      4546.780
4_webp_a.png  lossless  libwebp     0       421    163     47380   -        500    1.220
4_webp_a.png  lossless  libwebp     1       421    163     37608   -        111    9.040
4_webp_a.png  lossless  libwebp     2       421    163     36502   -        63     16.041
4_webp_a.png  lossless  libwebp     3       421    163     35266   -        40     25.055
4_webp_a.png  lossless  libwebp     4       421    163     35248   -        40     25.509
4_webp_a.png  lossless  libwebp     5       421    163     34610   -        19     54.639
4_webp_a.png  lossless  libwebp     6       421    163     34420   -        17     61.941
4_webp_a.png  lossless  libwebp     7       421    163     34440   -        16     65.082
4_webp_a.png  lossless  libwebp     8       421    163     33498   -        8      141.296
4_webp_a.png  lossless  libwebp     9       421    163     33018   -        2      837.962
4_webp_a.png  lossless  nativewebp  0       421    163     39072   -        100    10.091
4_webp_a.png  lossless  nativewebp  4       421    163     37928   -        59     17.020
4_webp_a.png  lossless  nativewebp  6       421    163     37928   -        59     16.981
4_webp_a.png  lossless  ours        0       421    163     38012   -        387    2.586
4_webp_a.png  lossless  ours        1       421    163     37896   -        309    3.236
4_webp_a.png  lossless  ours        2       421    163     37896   -        113    8.872
4_webp_a.png  lossless  ours        3       421    163     37896   -        88     11.490
4_webp_a.png  lossless  ours        4       421    163     37896   -        74     13.590
4_webp_a.png  lossless  ours        5       421    163     37896   -        56     18.084
4_webp_a.png  lossless  ours        6       421    163     38046   -        46     21.800
4_webp_a.png  lossless  wasm        0       421    163     45042   -        334    2.995
4_webp_a.png  lossless  wasm        1       421    163     36876   -        41     24.484
4_webp_a.png  lossless  wasm        2       421    163     36062   -        26     38.525
4_webp_a.png  lossless  wasm        3       421    163     35114   -        21     48.218
4_webp_a.png  lossless  wasm        4       421    163     34420   -        11     94.968
4_webp_a.png  lossless  wasm        5       421    163     33558   -        5      211.189
4_webp_a.png  lossless  wasm        6       421    163     33558   -        5      200.943
4_webp_a.png  lossy     libwebp     0       421    163     23740   39.28    416    2.405
4_webp_a.png  lossy     libwebp     1       421    163     19908   39.23    341    2.940
4_webp_a.png  lossy     libwebp     2       421    163     19524   39.12    340    2.943
4_webp_a.png  lossy     libwebp     3       421    163     18786   39.77    220    4.554
4_webp_a.png  lossy     libwebp     4       421    163     18806   39.76    181    5.553
4_webp_a.png  lossy     libwebp     5       421    163     18830   39.74    173    5.781
4_webp_a.png  lossy     libwebp     6       421    163     18758   39.73    2      662.307
4_webp_a.png  lossy     ours        0       421    163     22468   38.92    83     12.110
4_webp_a.png  lossy     ours        1       421    163     22142   38.94    51     19.900
4_webp_a.png  lossy     ours        2       421    163     22150   38.94    32     31.817
4_webp_a.png  lossy     ours        3       421    163     19774   39.22    21     48.128
4_webp_a.png  lossy     ours        4       421    163     19774   39.22    11     93.863
4_webp_a.png  lossy     ours        5       421    163     19774   39.22    8      131.237
4_webp_a.png  lossy     ours        6       421    163     19380   39.16    6      169.244
4_webp_a.png  lossy     ours        7       421    163     19380   39.16    6      166.763
4_webp_a.png  lossy     ours        8       421    163     19190   39.33    5      202.866
4_webp_a.png  lossy     ours        9       421    163     19190   39.33    5      201.546
4_webp_a.png  lossy     wasm        0       421    163     23740   39.28    172    5.844
4_webp_a.png  lossy     wasm        1       421    163     19908   39.23    138    7.260
4_webp_a.png  lossy     wasm        2       421    163     19524   39.12    134    7.473
4_webp_a.png  lossy     wasm        3       421    163     18786   39.77    64     15.749
4_webp_a.png  lossy     wasm        4       421    163     18806   39.76    58     17.329
4_webp_a.png  lossy     wasm        5       421    163     18830   39.74    55     18.516
4_webp_a.png  lossy     wasm        6       421    163     18758   39.73    1      1105.490
5_webp_a.png  lossless  libwebp     0       300    300     166624  -        416    2.405
5_webp_a.png  lossless  libwebp     1       300    300     157058  -        94     10.668
5_webp_a.png  lossless  libwebp     2       300    300     149780  -        47     21.479
5_webp_a.png  lossless  libwebp     3       300    300     144870  -        27     38.060
5_webp_a.png  lossless  libwebp     4       300    300     144778  -        26     39.349
5_webp_a.png  lossless  libwebp     5       300    300     140030  -        11     96.314
5_webp_a.png  lossless  libwebp     6       300    300     140018  -        10     109.675
5_webp_a.png  lossless  libwebp     7       300    300     140116  -        9      113.369
5_webp_a.png  lossless  libwebp     8       300    300     137608  -        4      253.012
5_webp_a.png  lossless  libwebp     9       300    300     137786  -        1      3066.399
5_webp_a.png  lossless  nativewebp  0       300    300     164800  -        68     14.712
5_webp_a.png  lossless  nativewebp  4       300    300     154970  -        43     23.282
5_webp_a.png  lossless  nativewebp  6       300    300     154970  -        44     23.224
5_webp_a.png  lossless  ours        0       300    300     161478  -        183    5.468
5_webp_a.png  lossless  ours        1       300    300     156970  -        160    6.256
5_webp_a.png  lossless  ours        2       300    300     156970  -        61     16.564
5_webp_a.png  lossless  ours        3       300    300     155772  -        50     20.138
5_webp_a.png  lossless  ours        4       300    300     152988  -        44     22.845
5_webp_a.png  lossless  ours        5       300    300     149932  -        35     29.294
5_webp_a.png  lossless  ours        6       300    300     149708  -        27     37.941
5_webp_a.png  lossless  wasm        0       300    300     166008  -        152    6.580
5_webp_a.png  lossless  wasm        1       300    300     154618  -        24     43.012
5_webp_a.png  lossless  wasm        2       300    300     149320  -        16     63.364
5_webp_a.png  lossless  wasm        3       300    300     144768  -        12     85.178
5_webp_a.png  lossless  wasm        4       300    300     140018  -        6      185.980
5_webp_a.png  lossless  wasm        5       300    300     137538  -        3      411.792
5_webp_a.png  lossless  wasm        6       300    300     137538  -        3      352.455
5_webp_a.png  lossy     libwebp     0       300    300     64290   32.77    196    5.125
5_webp_a.png  lossy     libwebp     1       300    300     63902   32.78    158    6.350
5_webp_a.png  lossy     libwebp     2       300    300     62690   32.49    150    6.702
5_webp_a.png  lossy     libwebp     3       300    300     61738   32.64    102    9.871
5_webp_a.png  lossy     libwebp     4       300    300     58682   32.63    40     25.393
5_webp_a.png  lossy     libwebp     5       300    300     57038   32.64    8      139.695
5_webp_a.png  lossy     libwebp     6       300    300     55926   32.67    1      3235.437
5_webp_a.png  lossy     ours        0       300    300     70196   33.07    42     23.852
5_webp_a.png  lossy     ours        1       300    300     69260   33.09    26     39.934
5_webp_a.png  lossy     ours        2       300    300     68560   33.09    17     62.314
5_webp_a.png  lossy     ours        3       300    300     64666   33.22    11     98.140
5_webp_a.png  lossy     ours        4       300    300     64666   33.22    6      196.045
5_webp_a.png  lossy     ours        5       300    300     60232   33.22    6      169.132
5_webp_a.png  lossy     ours        6       300    300     59748   33.18    5      216.523
5_webp_a.png  lossy     ours        7       300    300     59748   33.18    5      216.662
5_webp_a.png  lossy     ours        8       300    300     59610   33.23    4      267.683
5_webp_a.png  lossy     ours        9       300    300     59610   33.23    4      271.957
5_webp_a.png  lossy     wasm        0       300    300     64290   32.77    94     10.647
5_webp_a.png  lossy     wasm        1       300    300     63902   32.78    73     13.754
5_webp_a.png  lossy     wasm        2       300    300     62690   32.49    68     14.762
5_webp_a.png  lossy     wasm        3       300    300     61738   32.64    34     30.265
5_webp_a.png  lossy     wasm        4       300    300     58682   32.63    17     61.344
5_webp_a.png  lossy     wasm        5       300    300     57038   32.64    5      223.762
5_webp_a.png  lossy     wasm        6       300    300     55926   32.67    1      5536.075
```

## amd64 (AMD Ryzen 7 5700G) / transparent

```
file          mode        engine      width  height  bytes   psnr_db  iters  ms_per_op
1_webp_a.png  lossless    libwebp     400    301     75254   -        21     95.319
1_webp_a.png  lossless    nativewebp  400    301     81934   -        20     100.673
1_webp_a.png  lossless    ours        400    301     79910   -        33     61.638
1_webp_a.png  lossless    wasm        400    301     74350   -        4      609.787
1_webp_a.png  lossy-fast  libwebp     400    301     20134   41.94    455    4.401
1_webp_a.png  lossy-fast  ours        400    301     21748   41.48    71     28.207
1_webp_a.png  lossy-fast  wasm        400    301     20134   41.94    112    17.915
1_webp_a.png  lossy-slow  libwebp     400    301     17032   42.23    2      1220.639
1_webp_a.png  lossy-slow  ours        400    301     18610   42.09    6      387.710
1_webp_a.png  lossy-slow  wasm        400    301     17032   42.23    1      4616.616
2_webp_a.png  lossless    libwebp     386    395     41648   -        20     103.473
2_webp_a.png  lossless    nativewebp  386    395     49924   -        23     87.265
2_webp_a.png  lossless    ours        386    395     43988   -        26     79.482
2_webp_a.png  lossless    wasm        386    395     41428   -        3      712.828
2_webp_a.png  lossy-fast  libwebp     386    395     18844   43.02    476    4.202
2_webp_a.png  lossy-fast  ours        386    395     20230   42.42    46     43.649
2_webp_a.png  lossy-fast  wasm        386    395     18844   43.02    105    19.172
2_webp_a.png  lossy-slow  libwebp     386    395     13994   43.38    2      1064.744
2_webp_a.png  lossy-slow  ours        386    395     14732   42.83    5      414.107
2_webp_a.png  lossy-slow  wasm        386    395     13994   43.38    1      5147.145
3_webp_a.png  lossless    libwebp     800    600     181124  -        13     155.862
3_webp_a.png  lossless    nativewebp  800    600     202886  -        13     161.111
3_webp_a.png  lossless    ours        800    600     188144  -        12     181.413
3_webp_a.png  lossless    wasm        800    600     178918  -        3      790.744
3_webp_a.png  lossy-fast  libwebp     800    600     80676   41.56    107    18.859
3_webp_a.png  lossy-fast  ours        800    600     73228   40.82    28     73.681
3_webp_a.png  lossy-fast  wasm        800    600     80676   41.56    30     68.806
3_webp_a.png  lossy-slow  libwebp     800    600     52308   41.61    1      2083.369
3_webp_a.png  lossy-slow  ours        800    600     59116   41.18    3      985.328
3_webp_a.png  lossy-slow  wasm        800    600     52308   41.61    1      8190.893
4_webp_a.png  lossless    libwebp     421    163     34420   -        41     49.673
4_webp_a.png  lossless    nativewebp  421    163     37928   -        48     42.142
4_webp_a.png  lossless    ours        421    163     38046   -        58     34.909
4_webp_a.png  lossless    wasm        421    163     33558   -        7      321.724
4_webp_a.png  lossy-fast  libwebp     421    163     23740   39.28    500    2.851
4_webp_a.png  lossy-fast  ours        421    163     22468   38.92    110    18.255
4_webp_a.png  lossy-fast  wasm        421    163     23740   39.28    169    11.850
4_webp_a.png  lossy-slow  libwebp     421    163     18758   39.73    5      489.488
4_webp_a.png  lossy-slow  ours        421    163     19190   39.33    7      298.331
4_webp_a.png  lossy-slow  wasm        421    163     18758   39.73    1      2023.512
5_webp_a.png  lossless    libwebp     300    300     140018  -        18     112.421
5_webp_a.png  lossless    nativewebp  300    300     154970  -        37     54.802
5_webp_a.png  lossless    ours        300    300     149708  -        35     58.677
5_webp_a.png  lossless    wasm        300    300     137538  -        4      580.730
5_webp_a.png  lossy-fast  libwebp     300    300     64290   32.77    341    5.869
5_webp_a.png  lossy-fast  ours        300    300     70196   33.07    60     33.570
5_webp_a.png  lossy-fast  wasm        300    300     64290   32.77    105    19.182
5_webp_a.png  lossy-slow  libwebp     300    300     55926   32.67    1      2458.368
5_webp_a.png  lossy-slow  ours        300    300     59610   33.23    6      368.323
5_webp_a.png  lossy-slow  wasm        300    300     55926   32.67    1      10958.703
```

Peak RSS, one encode per process:

```
file          mode        engine      width  height  megapixels  peak_rss_mib  mib_per_mp
1_webp_a.png  lossless    libwebp     400    301     0.12        32.2          267.8
1_webp_a.png  lossless    nativewebp  400    301     0.12        15.9          132.4
1_webp_a.png  lossless    ours        400    301     0.12        18.0          149.3
1_webp_a.png  lossless    wasm        400    301     0.12        63.4          526.7
1_webp_a.png  lossy-fast  libwebp     400    301     0.12        13.5          111.9
1_webp_a.png  lossy-fast  ours        400    301     0.12        18.0          149.7
1_webp_a.png  lossy-fast  wasm        400    301     0.12        27.7          229.9
1_webp_a.png  lossy-slow  libwebp     400    301     0.12        44.4          368.7
1_webp_a.png  lossy-slow  ours        400    301     0.12        28.5          236.7
1_webp_a.png  lossy-slow  wasm        400    301     0.12        102.1         848.0
2_webp_a.png  lossless    libwebp     386    395     0.15        43.2          283.3
2_webp_a.png  lossless    nativewebp  386    395     0.15        17.5          115.0
2_webp_a.png  lossless    ours        386    395     0.15        22.1          145.1
2_webp_a.png  lossless    wasm        386    395     0.15        95.4          625.9
2_webp_a.png  lossy-fast  libwebp     386    395     0.15        13.8          90.8
2_webp_a.png  lossy-fast  ours        386    395     0.15        18.1          118.6
2_webp_a.png  lossy-fast  wasm        386    395     0.15        33.5          219.9
2_webp_a.png  lossy-slow  libwebp     386    395     0.15        54.3          356.0
2_webp_a.png  lossy-slow  ours        386    395     0.15        30.5          200.0
2_webp_a.png  lossy-slow  wasm        386    395     0.15        132.2         867.1
3_webp_a.png  lossless    libwebp     800    600     0.48        38.1          79.3
3_webp_a.png  lossless    nativewebp  800    600     0.48        29.9          62.4
3_webp_a.png  lossless    ours        800    600     0.48        42.5          88.5
3_webp_a.png  lossless    wasm        800    600     0.48        109.5         228.1
3_webp_a.png  lossy-fast  libwebp     800    600     0.48        25.9          53.9
3_webp_a.png  lossy-fast  ours        800    600     0.48        37.7          78.5
3_webp_a.png  lossy-fast  wasm        800    600     0.48        66.0          137.5
3_webp_a.png  lossy-slow  libwebp     800    600     0.48        56.9          118.5
3_webp_a.png  lossy-slow  ours        800    600     0.48        84.4          175.8
3_webp_a.png  lossy-slow  wasm        800    600     0.48        228.2         475.5
4_webp_a.png  lossless    libwebp     421    163     0.07        25.7          373.8
4_webp_a.png  lossless    nativewebp  421    163     0.07        17.6          256.4
4_webp_a.png  lossless    ours        421    163     0.07        14.1          206.0
4_webp_a.png  lossless    wasm        421    163     0.07        39.2          571.2
4_webp_a.png  lossy-fast  libwebp     421    163     0.07        12.0          175.0
4_webp_a.png  lossy-fast  ours        421    163     0.07        14.0          203.4
4_webp_a.png  lossy-fast  wasm        421    163     0.07        23.7          345.9
4_webp_a.png  lossy-slow  libwebp     421    163     0.07        30.5          444.2
4_webp_a.png  lossy-slow  ours        421    163     0.07        22.5          328.4
4_webp_a.png  lossy-slow  wasm        421    163     0.07        89.8          1308.7
5_webp_a.png  lossless    libwebp     300    300     0.09        25.2          279.8
5_webp_a.png  lossless    nativewebp  300    300     0.09        17.7          196.5
5_webp_a.png  lossless    ours        300    300     0.09        18.4          204.3
5_webp_a.png  lossless    wasm        300    300     0.09        49.4          548.7
5_webp_a.png  lossy-fast  libwebp     300    300     0.09        12.6          140.5
5_webp_a.png  lossy-fast  ours        300    300     0.09        15.8          175.4
5_webp_a.png  lossy-fast  wasm        300    300     0.09        23.7          262.8
5_webp_a.png  lossy-slow  libwebp     300    300     0.09        36.7          407.6
5_webp_a.png  lossy-slow  ours        300    300     0.09        24.5          271.7
5_webp_a.png  lossy-slow  wasm        300    300     0.09        108.0         1199.9
```

Decode, one file per mode encoded by libwebp:

```
file          mode      engine   width  height  bytes   psnr_db  iters  ms_per_op
1_webp_a.png  lossless  libwebp  400    301     75254   -        500    1.144
1_webp_a.png  lossless  ours     400    301     75254   -        500    2.494
1_webp_a.png  lossless  wasm     400    301     75254   29.40    360    5.556
1_webp_a.png  lossless  x/image  400    301     75254   -        500    2.924
1_webp_a.png  lossy     libwebp  400    301     17032   -        500    1.234
1_webp_a.png  lossy     ours     400    301     17032   -        500    3.132
1_webp_a.png  lossy     wasm     400    301     17032   29.63    500    3.965
1_webp_a.png  lossy     x/image  400    301     17032   29.63    500    3.816
2_webp_a.png  lossless  libwebp  386    395     41648   -        500    0.985
2_webp_a.png  lossless  ours     386    395     41648   -        500    2.386
2_webp_a.png  lossless  wasm     386    395     41648   25.35    326    6.137
2_webp_a.png  lossless  x/image  386    395     41648   -        500    2.591
2_webp_a.png  lossy     libwebp  386    395     13994   -        500    1.444
2_webp_a.png  lossy     ours     386    395     13994   -        500    3.727
2_webp_a.png  lossy     wasm     386    395     13994   25.46    379    5.279
2_webp_a.png  lossy     x/image  386    395     13994   25.46    421    4.755
3_webp_a.png  lossless  libwebp  800    600     181124  -        500    2.958
3_webp_a.png  lossless  ours     800    600     181124  -        256    7.833
3_webp_a.png  lossless  wasm     800    600     181124  27.66    111    18.050
3_webp_a.png  lossless  x/image  800    600     181124  -        235    8.517
3_webp_a.png  lossy     libwebp  800    600     52308   -        500    3.738
3_webp_a.png  lossy     ours     800    600     52308   -        197    10.177
3_webp_a.png  lossy     wasm     800    600     52308   27.81    142    14.169
3_webp_a.png  lossy     x/image  800    600     52308   27.81    149    13.489
4_webp_a.png  lossless  libwebp  421    163     34420   -        500    0.552
4_webp_a.png  lossless  ours     421    163     34420   -        500    1.270
4_webp_a.png  lossless  wasm     421    163     34420   30.27    500    3.100
4_webp_a.png  lossless  x/image  421    163     34420   -        500    1.518
4_webp_a.png  lossy     libwebp  421    163     18758   -        500    0.953
4_webp_a.png  lossy     ours     421    163     18758   -        500    2.333
4_webp_a.png  lossy     wasm     421    163     18758   31.62    500    3.006
4_webp_a.png  lossy     x/image  421    163     18758   31.62    500    2.808
5_webp_a.png  lossless  libwebp  300    300     140018  -        500    1.566
5_webp_a.png  lossless  ours     300    300     140018  -        500    2.936
5_webp_a.png  lossless  wasm     300    300     140018  24.63    369    5.432
5_webp_a.png  lossless  x/image  300    300     140018  -        500    3.763
5_webp_a.png  lossy     libwebp  300    300     55926   -        500    2.368
5_webp_a.png  lossy     ours     300    300     55926   -        391    5.126
5_webp_a.png  lossy     wasm     300    300     55926   25.61    342    5.860
5_webp_a.png  lossy     x/image  300    300     55926   25.61    347    5.772
```

Effort sweep, every setting of every engine:

```
file          mode      engine      effort  width  height  bytes   psnr_db  iters  ms_per_op
1_webp_a.png  lossless  libwebp     0       400    301     89358   -        443    2.257
1_webp_a.png  lossless  libwebp     1       400    301     80522   -        75     13.505
1_webp_a.png  lossless  libwebp     2       400    301     77132   -        46     21.998
1_webp_a.png  lossless  libwebp     3       400    301     77242   -        28     35.968
1_webp_a.png  lossless  libwebp     4       400    301     76798   -        28     36.641
1_webp_a.png  lossless  libwebp     5       400    301     75278   -        12     83.756
1_webp_a.png  lossless  libwebp     6       400    301     75254   -        11     94.241
1_webp_a.png  lossless  libwebp     7       400    301     75162   -        11     97.249
1_webp_a.png  lossless  libwebp     8       400    301     73858   -        5      230.011
1_webp_a.png  lossless  libwebp     9       400    301     71932   -        1      1785.595
1_webp_a.png  lossless  nativewebp  0       400    301     83958   -        31     33.359
1_webp_a.png  lossless  nativewebp  4       400    301     81934   -        14     71.914
1_webp_a.png  lossless  nativewebp  6       400    301     81934   -        14     71.431
1_webp_a.png  lossless  ours        0       400    301     83468   -        127    7.895
1_webp_a.png  lossless  ours        1       400    301     82496   -        104    9.692
1_webp_a.png  lossless  ours        2       400    301     82476   -        41     24.940
1_webp_a.png  lossless  ours        3       400    301     82476   -        33     30.829
1_webp_a.png  lossless  ours        4       400    301     81832   -        27     37.179
1_webp_a.png  lossless  ours        5       400    301     80522   -        20     50.017
1_webp_a.png  lossless  ours        6       400    301     79910   -        17     62.026
1_webp_a.png  lossless  wasm        0       400    301     88930   -        87     11.569
1_webp_a.png  lossless  wasm        1       400    301     78950   -        15     70.644
1_webp_a.png  lossless  wasm        2       400    301     77160   -        11     91.628
1_webp_a.png  lossless  wasm        3       400    301     76576   -        8      127.161
1_webp_a.png  lossless  wasm        4       400    301     75254   -        4      288.788
1_webp_a.png  lossless  wasm        5       400    301     73842   -        2      654.391
1_webp_a.png  lossless  wasm        6       400    301     74350   -        2      607.151
1_webp_a.png  lossy     libwebp     0       400    301     20134   41.94    238    4.216
1_webp_a.png  lossy     libwebp     1       400    301     18496   41.98    193    5.194
1_webp_a.png  lossy     libwebp     2       400    301     17446   41.40    189    5.301
1_webp_a.png  lossy     libwebp     3       400    301     17122   42.23    112    8.970
1_webp_a.png  lossy     libwebp     4       400    301     17142   42.25    97     10.343
1_webp_a.png  lossy     libwebp     5       400    301     17228   42.22    74     13.640
1_webp_a.png  lossy     libwebp     6       400    301     17032   42.23    1      1224.703
1_webp_a.png  lossy     ours        0       400    301     21748   41.48    38     26.777
1_webp_a.png  lossy     ours        1       400    301     21436   41.55    25     41.325
1_webp_a.png  lossy     ours        2       400    301     21448   41.55    16     62.714
1_webp_a.png  lossy     ours        3       400    301     19330   41.88    11     93.302
1_webp_a.png  lossy     ours        4       400    301     19330   41.88    6      191.719
1_webp_a.png  lossy     ours        5       400    301     19330   41.88    4      251.204
1_webp_a.png  lossy     ours        6       400    301     18766   41.80    4      311.348
1_webp_a.png  lossy     ours        7       400    301     18766   41.80    4      313.224
1_webp_a.png  lossy     ours        8       400    301     18610   42.09    3      378.151
1_webp_a.png  lossy     ours        9       400    301     18610   42.09    3      378.924
1_webp_a.png  lossy     wasm        0       400    301     20134   41.94    57     17.820
1_webp_a.png  lossy     wasm        1       400    301     18496   41.98    43     23.292
1_webp_a.png  lossy     wasm        2       400    301     17446   41.40    43     23.410
1_webp_a.png  lossy     wasm        3       400    301     17122   42.23    23     45.026
1_webp_a.png  lossy     wasm        4       400    301     17142   42.25    21     48.547
1_webp_a.png  lossy     wasm        5       400    301     17228   42.22    17     62.140
1_webp_a.png  lossy     wasm        6       400    301     17032   42.23    1      4556.104
2_webp_a.png  lossless  libwebp     0       386    395     57368   -        398    2.518
2_webp_a.png  lossless  libwebp     1       386    395     49780   -        54     18.737
2_webp_a.png  lossless  libwebp     2       386    395     44900   -        35     28.716
2_webp_a.png  lossless  libwebp     3       386    395     42970   -        24     42.062
2_webp_a.png  lossless  libwebp     4       386    395     43076   -        24     42.910
2_webp_a.png  lossless  libwebp     5       386    395     41610   -        12     87.646
2_webp_a.png  lossless  libwebp     6       386    395     41648   -        11     97.492
2_webp_a.png  lossless  libwebp     7       386    395     41610   -        10     100.992
2_webp_a.png  lossless  libwebp     8       386    395     41428   -        5      230.234
2_webp_a.png  lossless  libwebp     9       386    395     40058   -        1      1354.904
2_webp_a.png  lossless  nativewebp  0       386    395     51760   -        25     40.084
2_webp_a.png  lossless  nativewebp  4       386    395     49924   -        11     91.182
2_webp_a.png  lossless  nativewebp  6       386    395     49924   -        12     89.558
2_webp_a.png  lossless  ours        0       386    395     48054   -        117    8.560
2_webp_a.png  lossless  ours        1       386    395     47046   -        95     10.577
2_webp_a.png  lossless  ours        2       386    395     46876   -        41     24.443
2_webp_a.png  lossless  ours        3       386    395     46078   -        32     32.137
2_webp_a.png  lossless  ours        4       386    395     45618   -        26     39.571
2_webp_a.png  lossless  ours        5       386    395     44412   -        20     51.972
2_webp_a.png  lossless  ours        6       386    395     43988   -        16     62.528
2_webp_a.png  lossless  wasm        0       386    395     56180   -        65     15.460
2_webp_a.png  lossless  wasm        1       386    395     44974   -        11     98.081
2_webp_a.png  lossless  wasm        2       386    395     43436   -        8      133.970
2_webp_a.png  lossless  wasm        3       386    395     43056   -        7      161.766
2_webp_a.png  lossless  wasm        4       386    395     41648   -        4      332.614
2_webp_a.png  lossless  wasm        5       386    395     41428   -        2      779.113
2_webp_a.png  lossless  wasm        6       386    395     41428   -        2      701.359
2_webp_a.png  lossy     libwebp     0       386    395     18844   43.02    241    4.163
2_webp_a.png  lossy     libwebp     1       386    395     16172   43.00    190    5.279
2_webp_a.png  lossy     libwebp     2       386    395     15124   42.88    192    5.217
2_webp_a.png  lossy     libwebp     3       386    395     14534   43.45    107    9.419
2_webp_a.png  lossy     libwebp     4       386    395     14046   43.36    52     19.408
2_webp_a.png  lossy     libwebp     5       386    395     14000   43.53    11     94.695
2_webp_a.png  lossy     libwebp     6       386    395     13994   43.38    1      1067.911
2_webp_a.png  lossy     ours        0       386    395     20230   42.42    32     31.290
2_webp_a.png  lossy     ours        1       386    395     19772   42.54    23     44.706
2_webp_a.png  lossy     ours        2       386    395     19606   42.54    15     69.601
2_webp_a.png  lossy     ours        3       386    395     15772   42.82    11     98.004
2_webp_a.png  lossy     ours        4       386    395     15772   42.82    5      209.640
2_webp_a.png  lossy     ours        5       386    395     15484   42.82    5      247.717
2_webp_a.png  lossy     ours        6       386    395     15104   42.77    4      298.892
2_webp_a.png  lossy     ours        7       386    395     15104   42.77    4      300.071
2_webp_a.png  lossy     ours        8       386    395     14732   42.83    3      338.480
2_webp_a.png  lossy     ours        9       386    395     14732   42.83    3      335.765
2_webp_a.png  lossy     wasm        0       386    395     18844   43.02    49     20.448
2_webp_a.png  lossy     wasm        1       386    395     16172   43.00    39     26.262
2_webp_a.png  lossy     wasm        2       386    395     15124   42.88    40     25.353
2_webp_a.png  lossy     wasm        3       386    395     14534   43.45    20     50.364
2_webp_a.png  lossy     wasm        4       386    395     14046   43.36    12     89.686
2_webp_a.png  lossy     wasm        5       386    395     14000   43.53    2      528.354
2_webp_a.png  lossy     wasm        6       386    395     13994   43.38    1      5198.389
3_webp_a.png  lossless  libwebp     0       800    600     266952  -        159    6.311
3_webp_a.png  lossless  libwebp     1       800    600     189276  -        21     49.874
3_webp_a.png  lossless  libwebp     2       800    600     184600  -        14     76.673
3_webp_a.png  lossless  libwebp     3       800    600     181092  -        8      132.711
3_webp_a.png  lossless  libwebp     4       800    600     181208  -        8      135.408
3_webp_a.png  lossless  libwebp     5       800    600     181208  -        8      135.206
3_webp_a.png  lossless  libwebp     6       800    600     181124  -        7      153.211
3_webp_a.png  lossless  libwebp     7       800    600     179208  -        6      180.282
3_webp_a.png  lossless  libwebp     8       800    600     177714  -        3      348.150
3_webp_a.png  lossless  libwebp     9       800    600     174890  -        1      2182.016
3_webp_a.png  lossless  nativewebp  0       800    600     203914  -        8      126.988
3_webp_a.png  lossless  nativewebp  4       800    600     202886  -        7      161.733
3_webp_a.png  lossless  nativewebp  6       800    600     202886  -        7      167.003
3_webp_a.png  lossless  ours        0       800    600     194012  -        42     23.926
3_webp_a.png  lossless  ours        1       800    600     193846  -        34     30.277
3_webp_a.png  lossless  ours        2       800    600     193450  -        14     72.742
3_webp_a.png  lossless  ours        3       800    600     193450  -        12     90.499
3_webp_a.png  lossless  ours        4       800    600     191848  -        9      117.827
3_webp_a.png  lossless  ours        5       800    600     189530  -        7      155.484
3_webp_a.png  lossless  ours        6       800    600     188144  -        6      183.138
3_webp_a.png  lossless  wasm        0       800    600     267190  -        30     33.590
3_webp_a.png  lossless  wasm        1       800    600     186782  -        4      257.424
3_webp_a.png  lossless  wasm        2       800    600     183524  -        4      302.866
3_webp_a.png  lossless  wasm        3       800    600     181124  -        3      425.768
3_webp_a.png  lossless  wasm        4       800    600     181124  -        3      427.421
3_webp_a.png  lossless  wasm        5       800    600     178918  -        2      858.205
3_webp_a.png  lossless  wasm        6       800    600     178918  -        2      791.837
3_webp_a.png  lossy     libwebp     0       800    600     80676   41.56    53     19.015
3_webp_a.png  lossy     libwebp     1       800    600     63372   41.57    32     32.219
3_webp_a.png  lossy     libwebp     2       800    600     58678   41.23    31     32.332
3_webp_a.png  lossy     libwebp     3       800    600     55434   41.60    23     45.362
3_webp_a.png  lossy     libwebp     4       800    600     55128   41.61    19     52.892
3_webp_a.png  lossy     libwebp     5       800    600     53732   41.57    8      131.798
3_webp_a.png  lossy     libwebp     6       800    600     52308   41.61    1      2026.337
3_webp_a.png  lossy     ours        0       800    600     73228   40.82    14     74.114
3_webp_a.png  lossy     ours        1       800    600     72536   40.91    11     91.079
3_webp_a.png  lossy     ours        2       800    600     71036   40.91    6      187.849
3_webp_a.png  lossy     ours        3       800    600     64238   41.12    5      243.193
3_webp_a.png  lossy     ours        4       800    600     64238   41.12    2      532.328
3_webp_a.png  lossy     ours        5       800    600     60602   41.12    2      665.855
3_webp_a.png  lossy     ours        6       800    600     59934   41.31    2      881.483
3_webp_a.png  lossy     ours        7       800    600     59934   41.31    2      869.615
3_webp_a.png  lossy     ours        8       800    600     59116   41.18    2      893.195
3_webp_a.png  lossy     ours        9       800    600     59116   41.18    2      982.992
3_webp_a.png  lossy     wasm        0       800    600     80676   41.56    15     70.064
3_webp_a.png  lossy     wasm        1       800    600     63372   41.57    7      151.734
3_webp_a.png  lossy     wasm        2       800    600     58678   41.23    7      147.883
3_webp_a.png  lossy     wasm        3       800    600     55434   41.60    5      210.521
3_webp_a.png  lossy     wasm        4       800    600     55128   41.61    5      247.124
3_webp_a.png  lossy     wasm        5       800    600     53732   41.57    2      632.277
3_webp_a.png  lossy     wasm        6       800    600     52308   41.61    1      8131.281
4_webp_a.png  lossless  libwebp     0       421    163     47380   -        500    1.146
4_webp_a.png  lossless  libwebp     1       421    163     37608   -        126    7.989
4_webp_a.png  lossless  libwebp     2       421    163     36502   -        74     13.515
4_webp_a.png  lossless  libwebp     3       421    163     35266   -        46     21.794
4_webp_a.png  lossless  libwebp     4       421    163     35248   -        45     22.273
4_webp_a.png  lossless  libwebp     5       421    163     34610   -        23     45.161
4_webp_a.png  lossless  libwebp     6       421    163     34420   -        21     49.570
4_webp_a.png  lossless  libwebp     7       421    163     34440   -        19     53.247
4_webp_a.png  lossless  libwebp     8       421    163     33498   -        9      116.726
4_webp_a.png  lossless  libwebp     9       421    163     33018   -        2      753.981
4_webp_a.png  lossless  nativewebp  0       421    163     39072   -        50     20.114
4_webp_a.png  lossless  nativewebp  4       421    163     37928   -        24     41.846
4_webp_a.png  lossless  nativewebp  6       421    163     37928   -        24     42.231
4_webp_a.png  lossless  ours        0       421    163     38012   -        227    4.418
4_webp_a.png  lossless  ours        1       421    163     37896   -        184    5.443
4_webp_a.png  lossless  ours        2       421    163     37896   -        70     14.400
4_webp_a.png  lossless  ours        3       421    163     37896   -        55     18.496
4_webp_a.png  lossless  ours        4       421    163     37896   -        45     22.503
4_webp_a.png  lossless  ours        5       421    163     37896   -        35     28.845
4_webp_a.png  lossless  ours        6       421    163     38046   -        29     35.070
4_webp_a.png  lossless  wasm        0       421    163     45042   -        193    5.181
4_webp_a.png  lossless  wasm        1       421    163     36876   -        26     39.060
4_webp_a.png  lossless  wasm        2       421    163     36062   -        16     65.985
4_webp_a.png  lossless  wasm        3       421    163     35114   -        13     81.274
4_webp_a.png  lossless  wasm        4       421    163     34420   -        7      157.923
4_webp_a.png  lossless  wasm        5       421    163     33558   -        3      354.275
4_webp_a.png  lossless  wasm        6       421    163     33558   -        4      322.790
4_webp_a.png  lossy     libwebp     0       421    163     23740   39.28    354    2.826
4_webp_a.png  lossy     libwebp     1       421    163     19908   39.23    291    3.437
4_webp_a.png  lossy     libwebp     2       421    163     19524   39.12    290    3.456
4_webp_a.png  lossy     libwebp     3       421    163     18786   39.77    181    5.554
4_webp_a.png  lossy     libwebp     4       421    163     18806   39.76    153    6.543
4_webp_a.png  lossy     libwebp     5       421    163     18830   39.74    145    6.913
4_webp_a.png  lossy     libwebp     6       421    163     18758   39.73    3      488.554
4_webp_a.png  lossy     ours        0       421    163     22468   38.92    55     18.280
4_webp_a.png  lossy     ours        1       421    163     22142   38.94    36     28.179
4_webp_a.png  lossy     ours        2       421    163     22150   38.94    21     47.816
4_webp_a.png  lossy     ours        3       421    163     19774   39.22    15     69.997
4_webp_a.png  lossy     ours        4       421    163     19774   39.22    8      142.426
4_webp_a.png  lossy     ours        5       421    163     19774   39.22    5      200.750
4_webp_a.png  lossy     ours        6       421    163     19380   39.16    4      259.765
4_webp_a.png  lossy     ours        7       421    163     19380   39.16    4      259.989
4_webp_a.png  lossy     ours        8       421    163     19190   39.33    4      298.922
4_webp_a.png  lossy     ours        9       421    163     19190   39.33    4      296.790
4_webp_a.png  lossy     wasm        0       421    163     23740   39.28    86     11.733
4_webp_a.png  lossy     wasm        1       421    163     19908   39.23    66     15.210
4_webp_a.png  lossy     wasm        2       421    163     19524   39.12    66     15.309
4_webp_a.png  lossy     wasm        3       421    163     18786   39.77    36     27.951
4_webp_a.png  lossy     wasm        4       421    163     18806   39.76    33     30.786
4_webp_a.png  lossy     wasm        5       421    163     18830   39.74    30     34.499
4_webp_a.png  lossy     wasm        6       421    163     18758   39.73    1      2022.725
5_webp_a.png  lossless  libwebp     0       300    300     166624  -        436    2.298
5_webp_a.png  lossless  libwebp     1       300    300     157058  -        89     11.315
5_webp_a.png  lossless  libwebp     2       300    300     149780  -        46     22.132
5_webp_a.png  lossless  libwebp     3       300    300     144870  -        26     39.821
5_webp_a.png  lossless  libwebp     4       300    300     144778  -        25     41.208
5_webp_a.png  lossless  libwebp     5       300    300     140030  -        10     100.769
5_webp_a.png  lossless  libwebp     6       300    300     140018  -        9      112.328
5_webp_a.png  lossless  libwebp     7       300    300     140116  -        9      117.933
5_webp_a.png  lossless  libwebp     8       300    300     137608  -        4      256.978
5_webp_a.png  lossless  libwebp     9       300    300     137786  -        1      2991.057
5_webp_a.png  lossless  nativewebp  0       300    300     164800  -        37     27.065
5_webp_a.png  lossless  nativewebp  4       300    300     154970  -        19     55.384
5_webp_a.png  lossless  nativewebp  6       300    300     154970  -        18     55.774
5_webp_a.png  lossless  ours        0       300    300     161478  -        115    8.720
5_webp_a.png  lossless  ours        1       300    300     156970  -        101    9.951
5_webp_a.png  lossless  ours        2       300    300     156970  -        40     25.309
5_webp_a.png  lossless  ours        3       300    300     155772  -        33     31.161
5_webp_a.png  lossless  ours        4       300    300     152988  -        29     35.315
5_webp_a.png  lossless  ours        5       300    300     149932  -        22     45.977
5_webp_a.png  lossless  ours        6       300    300     149708  -        17     59.329
5_webp_a.png  lossless  wasm        0       300    300     166008  -        82     12.292
5_webp_a.png  lossless  wasm        1       300    300     154618  -        15     71.320
5_webp_a.png  lossless  wasm        2       300    300     149320  -        10     110.741
5_webp_a.png  lossless  wasm        3       300    300     144768  -        8      138.618
5_webp_a.png  lossless  wasm        4       300    300     140018  -        4      313.234
5_webp_a.png  lossless  wasm        5       300    300     137538  -        2      693.474
5_webp_a.png  lossless  wasm        6       300    300     137538  -        2      578.864
5_webp_a.png  lossy     libwebp     0       300    300     64290   32.77    172    5.830
5_webp_a.png  lossy     libwebp     1       300    300     63902   32.78    136    7.406
5_webp_a.png  lossy     libwebp     2       300    300     62690   32.49    129    7.786
5_webp_a.png  lossy     libwebp     3       300    300     61738   32.64    87     11.500
5_webp_a.png  lossy     libwebp     4       300    300     58682   32.63    40     25.387
5_webp_a.png  lossy     libwebp     5       300    300     57038   32.64    13     81.781
5_webp_a.png  lossy     libwebp     6       300    300     55926   32.67    1      2461.638
5_webp_a.png  lossy     ours        0       300    300     70196   33.07    31     33.304
5_webp_a.png  lossy     ours        1       300    300     69260   33.09    20     52.318
5_webp_a.png  lossy     ours        2       300    300     68560   33.09    12     85.586
5_webp_a.png  lossy     ours        3       300    300     64666   33.22    8      133.291
5_webp_a.png  lossy     ours        4       300    300     64666   33.22    4      283.384
5_webp_a.png  lossy     ours        5       300    300     60232   33.22    5      244.848
5_webp_a.png  lossy     ours        6       300    300     59748   33.18    4      309.174
5_webp_a.png  lossy     ours        7       300    300     59748   33.18    4      310.580
5_webp_a.png  lossy     ours        8       300    300     59610   33.23    3      369.073
5_webp_a.png  lossy     ours        9       300    300     59610   33.23    3      369.196
5_webp_a.png  lossy     wasm        0       300    300     64290   32.77    53     19.011
5_webp_a.png  lossy     wasm        1       300    300     63902   32.78    42     24.240
5_webp_a.png  lossy     wasm        2       300    300     62690   32.49    39     26.001
5_webp_a.png  lossy     wasm        3       300    300     61738   32.64    20     50.970
5_webp_a.png  lossy     wasm        4       300    300     58682   32.63    10     102.889
5_webp_a.png  lossy     wasm        5       300    300     57038   32.64    3      366.234
5_webp_a.png  lossy     wasm        6       300    300     55926   32.67    1      10961.343
```

