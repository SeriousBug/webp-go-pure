# Benchmark results

Captured runs of `benchmark/run.sh`, `benchmark/run-mem.sh`, `benchmark/run-sweep.sh`
and `benchmark/run-decode.sh` on two machines, over two corpora. Timings are
machine-dependent; regenerate locally for your own hardware.

| | arm64 | amd64 |
| --- | --- | --- |
| **CPU** | Apple M4 Pro (14 cores) | AMD Ryzen 7 5700G (16 threads) |
| **OS** | macOS 26.5.1 | Arch Linux, kernel 7.1.3 |
| **Go** | go1.26.5 | go1.26.5 |

- **Library commit:** 2040f5c · captured 2026-08-08
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

Encoder output is identical on both machines (every size in the two machines'
tables matches), so `psnr_db` reads the same in both and only the timings differ.

Each machine and corpus also has a peak-RSS table. Those measurements run one
encode per process and report that process's own `ru_maxrss`, so the figure is
what an application pays to encode one image: source bitmap, language runtime and
encoder together. `mib_per_mp` divides it by the image's megapixels so images of
different sizes are comparable; a 1080p frame is 2.1 MP, a 4K frame roughly
10 MP, and a phone photo about 12 MP, so multiply that column by those to size a
workload. The alpha corpus images are small enough (0.05-0.48 MP) that the Go
runtime's own floor dominates their per-megapixel figures, so read that corpus's
memory table as MiB, not as MiB per megapixel.

## Reading these numbers: the photos corpus

- **vs libwebp, lossless: we are level on size and behind on time.** Per image
  our files run 0.96-1.04x libwebp's, geometric mean 1.005x; over the corpus
  14.67 MiB against 14.66 MiB. The geometric mean of our per-image output is
  1.660 MB against libwebp's 1.652 MB, 0.5% apart. Time is where the two
  separate, and by how much depends on the machine: 0.99-1.27x libwebp's on
  arm64 (geometric mean 1.07x) and 1.55-1.92x on amd64 (1.68x).
- **vs libwebp, `lossy-slow`:** sizes land at 0.95-1.06x libwebp's (geometric
  mean 1.013x) within -0.25 to +0.22 dB, for 1.5-2.4x the encode time on arm64
  and 1.3-2.2x on amd64.
- **vs libwebp, `lossy-fast`:** quality lands within -0.43 to +0.26 dB, sizes run
  0.96-1.21x, and we take 1.1-1.7x the time on arm64, 1.4-1.8x on amd64. Effort 0
  is still where our files are furthest behind on size.
- **vs `wasm`, lossless: we are now smaller as well as faster.** 0.96-1.03x its
  size, geometric mean 0.998x, at 0.34-0.60x its time. `wasm` exposes only
  libwebp's `Method` for lossless, not the preset level, which is why the same C
  encoder lands slightly larger there than in the `libwebp` rows.
- **vs `nativewebp`, lossless: 8-23% smaller files.** Geometric mean 0.835x its
  size, for 1.25-1.88x its time on arm64 and 1.07-1.60x on amd64 at the fixed
  settings. The effort sweep is the fairer comparison and it is not close; see
  below. `nativewebp` has no lossy mode, so the choice only exists for VP8L.
- **`libwebp` and `wasm` are the same encoder.** Their lossy output is
  byte-identical at every method, so their sizes and PSNR match exactly; `wasm`
  is the cgo-free option and runs 2.3-3.6x slower than native `libwebp` on arm64
  and 2.9-4.8x on amd64 in the lossy modes, 1.8-3.7x and 2.7-5.6x on lossless.

On effort. The modes above are two points on each encoder's curve; `run-sweep.sh`
walks every setting. The figures below are totals over the seven photos, arm64
first and amd64 second, and the effort numbers are each engine's own scale.

- **Our lossless effort 0 beats `nativewebp` at every level it has, on both
  size and time.** 15.69 MiB in 1.5 s / 2.3 s against its best output of
  17.39 MiB in 7.5 s / 14.2 s: 9.8% smaller for a fifth of the time on arm64 and
  a sixth on amd64. Against its fastest level, 17.40 MiB in 6.8 s / 11.9 s, we
  are 9.8% smaller and 4.6x / 5.1x faster. Our efforts 0 to 4 all clear its best
  setting on both axes on both machines.
- **`nativewebp`'s three levels are one point on this corpus.** Level 0 to level
  6 moves the total by 0.07% (17.40 to 17.39 MiB) for 10% more time on arm64 and
  19% on amd64. That is a property of the content, not of the encoder: on the
  alpha corpus the same three levels do move, by 2.9%. See that section.
- **Our lossless curve is now useful from effort 0.** The whole ladder spans
  15.69 to 14.67 MiB, 6.5% end to end, for 1.5 s to 10.4 s on arm64. Effort 0 is
  a working setting rather than a placeholder, effort 2 is where the corpus drops
  under 15 MiB, and effort 6 is where it meets libwebp.
- **libwebp's lossless curve flattens at level 3**, 14.69 MiB in 5.9 s / 6.0 s.
  Levels 4 to 9 stay within 0.4% of it and are not monotonic: level 7 is its
  smallest at 14.63 MiB, level 8 comes back up to 14.67 MiB, and level 9 spends
  97 s / 103 s to land at 14.66 MiB. Our effort 6 is 0.03% above its level 6 and
  0.24% above its level 7.
- **Lossy: our curve now sits inside libwebp's quality band at every setting.**
  We run 3.65 to 3.18 MiB at 42.80-43.11 dB across efforts 0 to 9, against
  libwebp's 3.44 to 3.12 MiB at 42.74-43.10 dB across methods 0 to 6, and our
  PSNR rises with effort.
- **We are close to libwebp on lossy, and slightly behind on the tradeoff.**
  Effort 8 is 3.18 MiB at 43.04 dB in 3.4 s / 4.3 s against libwebp method 6's
  3.12 MiB at 43.06 dB in 2.9 s / 4.2 s: 1.8% larger at the same quality, for
  1.17x the time on arm64 and 1.03x on amd64. Effort 9 writes the same total at
  43.11 dB for 5.7 s / 7.2 s. libwebp's method 3 is the setting to beat: 3.19 MiB
  at 43.10 dB in 1.5 s / 1.9 s, which our effort 9 undercuts by 0.4% on size at
  the same quality but for 3.8x the time.
- **Several of our lossy settings are aliases.** Efforts 1 and 2, 3 through 5,
  and 6 and 7 each write byte-identical output. Efforts 8 and 9 write different
  files that happen to total the same bytes, at 43.04 and 43.11 dB. Our lossless
  ladder has no aliases on this corpus, and lossless effort caps at 6: 7 through
  9 are accepted and behave as 6.
- **`wasm` is libwebp's curve shifted right**, 2.4-3.8x on arm64 and 3.2-5.4x on
  amd64 across the lossy settings, at the same sizes. Its lossless knob is `Method` rather than the
  preset level, so its curve stops at 6 and never reaches libwebp's level 9, and
  its methods 5 and 6 are not monotonic in either time or size.

On memory:

- **Lossy, we are the lightest of the three Go options:** 0.74-0.96x libwebp's
  peak on arm64 and 0.82-0.93x on amd64, 15-19 MiB per megapixel on the
  geometric mean. On the 5.5+ MP images that is 69-78 MiB against libwebp's
  92-106 MiB. Encoding a 4K frame costs on the order of 150-190 MiB.
- **`wasm` costs 1.4-2.5x libwebp's peak in the lossy modes:** it carries a
  WebAssembly runtime and its own linear memory on top of the encode. That is the
  memory half of the cgo-free tradeoff, next to the 2.3-4.8x on time.
- **Lossless is where we are expensive, and it got worse with the new encoder.**
  We sit at 2.4-3.8x libwebp's peak, 127 MiB per megapixel on the geometric mean
  and up to 150: 725-821 MiB on the 5.5+ MP images on arm64 and 659-757 MiB on
  amd64, against libwebp's 201-219 MiB. `wasm` is now *below* us there, at
  576-700 MiB. If lossless peak memory is a constraint, encode at a lower effort
  or use another engine; this is the clearest cost in the file.
- **`nativewebp` is the lightest lossless encoder here**, at 0.23-0.40x our peak
  and slightly under libwebp's: 36 MiB per megapixel on the geometric mean,
  181-193 MiB on the 5.5+ MP images. Alongside its larger files and slower
  encodes, it is the memory-for-nothing-else corner of the pure-Go range.
- Per-megapixel figures run higher on small images, because a fixed runtime floor
  is spread over fewer pixels: in the lossy modes Lena at 0.81 MP reads about
  twice the per-megapixel cost of the 5.5 MP images.

On decoding. Every engine decodes the same libwebp-encoded file and has to end at
straight RGBA, so an engine that returns YCbCr planes pays for that conversion
inside the measurement, as an application would:

- **vs `x/image`, the other pure-Go decoder: we are ahead in every mode on both
  machines on the geometric mean.** Lossy 0.93x its time on arm64 and 0.91x on
  amd64; lossless 0.98x and 0.86x. Per image it goes both ways on arm64 lossless
  (0.78-1.13x), and is a consistent win everywhere else.
- **vs libwebp: 2.6-5.8x slower on lossy** (2.4-5.4x on amd64), and 1.5-2.1x
  (arm64) / 1.9-2.9x (amd64) on lossless.
- **vs `wasm`, the other cgo-free libwebp: we are faster on lossless**,
  0.39-0.80x its time on arm64 and 0.38-0.50x on amd64; lossy is a wash on arm64
  (0.92-1.16x) and ours on amd64 (0.76-0.87x).
- **The `psnr_db` column in the decode tables is not zero, and that is about the
  other engines' output format.** `gen2brain/webp` hands back `*image.NYCbCrA`
  even for a lossless VP8L file, so its RGB differs from libwebp's own decode of
  the same bytes by up to 35/255 on a channel, 25-32 dB. `x/image` is exact on
  lossless and shows the same YUV-to-RGB rounding difference on lossy. Our decode
  agrees with libwebp byte for byte in every row of every table here.

## Reading these numbers: the alpha corpus

This corpus is small, flat-shaded and mostly transparent, which is a different
encoding problem from the photos above. The images are 0.05-0.48 MP, so
per-megapixel memory and sub-millisecond decode times are dominated by fixed
costs; read the absolute columns here.

- **vs `nativewebp`, lossless: our effort 0 is smaller than anything it can
  produce, in a third of the time.** 525024 B in 0.04 s / 0.06 s against its best
  output of 527642 B in 0.18 s / 0.42 s, and against its fastest level's
  543504 B in 0.13 s / 0.25 s: 3.4% smaller and 3.3x / 4.2x faster than its
  fastest. Our efforts 0 to 2 beat its fastest level on size and time at once on
  both machines, and efforts 0 to 4 beat its best level on both. At the fixed
  settings in the tables (our effort 6 against its BestCompression) we run
  between 12% smaller and 0.3% larger per image, 5% smaller over the corpus, for
  1.16-1.73x the time on arm64 and 0.77-1.28x on amd64.
- **`nativewebp`'s compression level does move here**, unlike on photos: level 0
  writes 543504 B and levels 4 and 6 both write 527642 B, 2.9% smaller, for 38%
  more time on arm64 and 68% on amd64. Levels 4 and 6 are byte-identical, so it
  has two distinct points on this corpus rather than one.
- **vs libwebp, lossless: this is where we still lose.** Our effort 6 writes
  499796 B against libwebp's 472464 B at level 6, 5.8% larger, and 9.2% larger
  than its level 9's 457684 B. Per image we run 1.04-1.11x its size. We are
  faster than it here, 0.32-0.80x its time on arm64 and 0.58-1.31x on amd64,
  which is the opposite of the photos corpus.
- **Lossy with alpha now works, and lands within half a decibel of libwebp.**
  `lossy-fast` writes 207870 B at 39.34 dB against libwebp's 207684 B at
  39.71 dB, a 0.1% size difference at 0.37 dB lower quality; per image we run
  0.91-1.09x its size within -0.74 to +0.30 dB. `lossy-slow` writes 171452 B at
  39.73 dB against 158018 B at 39.92 dB, 8.5% larger. These rows did not exist
  before this branch: lossy encoding returned `ErrLossyAlpha` on any input with
  an alpha channel.
- **libwebp's lossy method 6 is extraordinarily slow on alpha content**, and it
  dominates the `lossy-slow` row. Over this corpus its methods 0 to 5 cost 0.03 s
  to 0.57 s and method 6 costs 9.99 s on arm64: 17x its own method 5 for 1.3%
  less output. On a single 400x301 image it is 12.6 ms at method 5 and 1638 ms at
  method 6. That is libwebp's exhaustive alpha-filter search, and it is why our
  `lossy-slow` reads as 0.09-0.32x its time on arm64 and 0.16-0.67x on amd64
  while our photos `lossy-slow` is 1.3-2.4x slower than it. Compared against
  libwebp's method 5 instead, we are the slower encoder here too.
- **Our lossy effort ladder has three alias pairs on this corpus:** 3 and 4, 6
  and 7, and 8 and 9 each write byte-identical output.
- **`wasm` carries the same method 6 cost and adds the WASM tax:** 16.5 s / 30.9 s
  for the corpus at method 6, against libwebp's 10.0 s / 7.3 s.

On memory, at these image sizes the Go runtime's floor is a large share of every
figure:

- Lossless, we peak at 20-53 MiB on arm64 and 22-68 MiB on amd64, against
  libwebp's 25-45 MiB and `nativewebp`'s 16-30 MiB. We are 0.66-1.33x libwebp's
  peak and 1.21-2.26x `nativewebp`'s.
- `lossy-fast` is the one mode on either corpus where we are heavier than libwebp,
  1.45-1.97x its peak: 18-54 MiB against its 12-27 MiB.
- `lossy-slow` we are lighter than libwebp on the geometric mean (0.59x arm64,
  0.89x amd64), though per image it ranges 0.39-1.86x.
- `wasm` is the heaviest in every mode, up to 228 MiB on a 0.48 MP image.

On decoding, every figure is 1-6 ms, so these compare ratios rather than costs:
we are 0.79-0.93x `x/image`'s time on the geometric mean, 0.45-0.63x `wasm`'s,
and 2.0-3.1x libwebp's.

## One measured non-determinism

Our lossy output on this corpus is not byte-identical across the two machines.
Three of the five images differ, by at most 0.35% of the file: on `3_webp_a.png`
at effort 8 the arm64 build writes 59312 B and the amd64 build 59116 B. The VP8
chunk is identical on both (27283 B), and the whole difference is in the `ALPH`
chunk (31981 B against 31786 B), so the lossy image data is deterministic and the
alpha plane's compression is not. Lossless output is identical on both machines
on both corpora, and the photos corpus is identical in every mode.

Each machine's tables below hold that machine's own bytes, so the alpha corpus is
the one place where the two sections' `bytes` columns do not match. This is
reported, not fixed: no encoder change was made for this capture.

## Charts

One set of figures per corpus, regenerated from the tables below with
`benchmark/chart/chart.go`.

### photos

![What effort buys on the photos corpus: one line per engine through its effort settings, with encode time on the x axis and output size or mean PSNR on the y axis, three panels per machine and settings that are off the size axis marked on the frame](charts/effort-sweep-photos-light.svg#gh-light-mode-only)
![What effort buys on the photos corpus: one line per engine through its effort settings, with encode time on the x axis and output size or mean PSNR on the y axis, three panels per machine and settings that are off the size axis marked on the frame](charts/effort-sweep-photos-dark.svg#gh-dark-mode-only)

![Size and quality against libwebp on the photos corpus: one point per test image, with output size relative to libwebp on the x axis and PSNR difference on the y axis, faceted by lossy mode](charts/rate-distortion-photos-light.svg#gh-light-mode-only)
![Size and quality against libwebp on the photos corpus: one point per test image, with output size relative to libwebp on the x axis and PSNR difference on the y axis, faceted by lossy mode](charts/rate-distortion-photos-dark.svg#gh-dark-mode-only)

![Encode time per image for each engine on the photos corpus, one panel per mode and machine, with bars that run off the panel drawn fading out under an arrow](charts/encode-time-photos-light.svg#gh-light-mode-only)
![Encode time per image for each engine on the photos corpus, one panel per mode and machine, with bars that run off the panel drawn fading out under an arrow](charts/encode-time-photos-dark.svg#gh-dark-mode-only)

![Decode time per image for each engine on the photos corpus, one panel per mode and machine, on the same bar layout as the encode time figure, with x/image added](charts/decode-time-photos-light.svg#gh-light-mode-only)
![Decode time per image for each engine on the photos corpus, one panel per mode and machine, on the same bar layout as the encode time figure, with x/image added](charts/decode-time-photos-dark.svg#gh-dark-mode-only)

![Peak memory per megapixel for each engine on the photos corpus, one panel per mode and machine, on the same bar layout as the encode time figure](charts/peak-memory-photos-light.svg#gh-light-mode-only)
![Peak memory per megapixel for each engine on the photos corpus, one panel per mode and machine, on the same bar layout as the encode time figure](charts/peak-memory-photos-dark.svg#gh-dark-mode-only)

### transparent

![What effort buys on the alpha corpus: one line per engine through its effort settings, with encode time on the x axis and output size or mean PSNR on the y axis, three panels per machine](charts/effort-sweep-transparent-light.svg#gh-light-mode-only)
![What effort buys on the alpha corpus: one line per engine through its effort settings, with encode time on the x axis and output size or mean PSNR on the y axis, three panels per machine](charts/effort-sweep-transparent-dark.svg#gh-dark-mode-only)

![Size and quality against libwebp on the alpha corpus: one point per test image, with output size relative to libwebp on the x axis and PSNR difference on the y axis, faceted by lossy mode](charts/rate-distortion-transparent-light.svg#gh-light-mode-only)
![Size and quality against libwebp on the alpha corpus: one point per test image, with output size relative to libwebp on the x axis and PSNR difference on the y axis, faceted by lossy mode](charts/rate-distortion-transparent-dark.svg#gh-dark-mode-only)

![Encode time per image for each engine on the alpha corpus, one panel per mode and machine](charts/encode-time-transparent-light.svg#gh-light-mode-only)
![Encode time per image for each engine on the alpha corpus, one panel per mode and machine](charts/encode-time-transparent-dark.svg#gh-dark-mode-only)

![Decode time per image for each engine on the alpha corpus, one panel per mode and machine, with x/image added](charts/decode-time-transparent-light.svg#gh-light-mode-only)
![Decode time per image for each engine on the alpha corpus, one panel per mode and machine, with x/image added](charts/decode-time-transparent-dark.svg#gh-dark-mode-only)

![Peak memory per megapixel for each engine on the alpha corpus, one panel per mode and machine](charts/peak-memory-transparent-light.svg#gh-light-mode-only)
![Peak memory per megapixel for each engine on the alpha corpus, one panel per mode and machine](charts/peak-memory-transparent-dark.svg#gh-dark-mode-only)

## arm64 (Apple M4 Pro) / photos

```
file                                            mode        engine      width  height  bytes    psnr_db  iters  ms_per_op
Lena_512.png                                    lossless    libwebp     900    900     627632   -        8      250.954
Lena_512.png                                    lossless    nativewebp  900    900     738176   -        12     169.037
Lena_512.png                                    lossless    ours        900    900     638220   -        7      318.557
Lena_512.png                                    lossless    wasm        900    900     622766   -        3      934.366
Lena_512.png                                    lossy-fast  libwebp     900    900     103980   41.04    128    15.664
Lena_512.png                                    lossy-fast  ours        900    900     113182   41.02    87     23.110
Lena_512.png                                    lossy-fast  wasm        900    900     103980   41.04    52     38.546
Lena_512.png                                    lossy-slow  libwebp     900    900     89968    40.94    26     77.139
Lena_512.png                                    lossy-slow  ours        900    900     95710    41.13    14     151.513
Lena_512.png                                    lossy-slow  wasm        900    900     89968    40.94    10     202.649
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    libwebp     2025   2700    3241976  -        1      2040.036
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    nativewebp  2025   2700    3926412  -        2      1702.918
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    ours        2025   2700    3269944  -        1      2127.547
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    wasm        2025   2700    3249798  -        1      3770.279
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  libwebp     2025   2700    598034   41.96    20     102.228
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  ours        2025   2700    723064   42.02    14     152.641
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  wasm        2025   2700    598034   41.96    9      249.671
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  libwebp     2025   2700    610518   42.58    3      681.440
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  ours        2025   2700    603030   42.33    2      1337.332
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  wasm        2025   2700    610518   42.58    2      1584.605
pexels-martin-alargent-1165956-5665465.jpg      lossless    libwebp     2025   2700    2943528  -        1      2036.050
pexels-martin-alargent-1165956-5665465.jpg      lossless    nativewebp  2025   2700    3563584  -        2      1507.407
pexels-martin-alargent-1165956-5665465.jpg      lossless    ours        2025   2700    2919664  -        1      2012.524
pexels-martin-alargent-1165956-5665465.jpg      lossless    wasm        2025   2700    2953798  -        1      3847.366
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  libwebp     2025   2700    726824   42.63    20     102.119
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  ours        2025   2700    756806   42.39    14     151.019
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  wasm        2025   2700    726824   42.63    8      251.638
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  libwebp     2025   2700    603264   42.75    4      570.340
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  ours        2025   2700    616898   42.97    2      1038.757
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  wasm        2025   2700    603264   42.75    2      1428.916
pexels-mavihnt-38213559.jpg                     lossless    libwebp     2560   1706    3482480  -        2      1547.241
pexels-mavihnt-38213559.jpg                     lossless    nativewebp  2560   1706    4009534  -        2      1201.232
pexels-mavihnt-38213559.jpg                     lossless    ours        2560   1706    3509912  -        2      1742.732
pexels-mavihnt-38213559.jpg                     lossless    wasm        2560   1706    3488584  -        1      3072.061
pexels-mavihnt-38213559.jpg                     lossy-fast  libwebp     2560   1706    983360   40.87    19     107.331
pexels-mavihnt-38213559.jpg                     lossy-fast  ours        2560   1706    1037210  40.99    12     172.998
pexels-mavihnt-38213559.jpg                     lossy-fast  wasm        2560   1706    983360   40.87    9      245.224
pexels-mavihnt-38213559.jpg                     lossy-slow  libwebp     2560   1706    925032   41.14    3      681.768
pexels-mavihnt-38213559.jpg                     lossy-slow  ours        2560   1706    948446   41.27    2      1315.198
pexels-mavihnt-38213559.jpg                     lossy-slow  wasm        2560   1706    925032   41.14    2      1564.017
pexels-steve-15267299.jpg                       lossless    libwebp     2095   3000    2104690  -        1      2209.345
pexels-steve-15267299.jpg                       lossless    nativewebp  2095   3000    2622528  -        2      1605.667
pexels-steve-15267299.jpg                       lossless    ours        2095   3000    2024034  -        1      2183.776
pexels-steve-15267299.jpg                       lossless    wasm        2095   3000    2119370  -        1      4002.475
pexels-steve-15267299.jpg                       lossy-fast  libwebp     2095   3000    284570   44.34    22     93.515
pexels-steve-15267299.jpg                       lossy-fast  ours        2095   3000    294606   43.91    18     113.165
pexels-steve-15267299.jpg                       lossy-fast  wasm        2095   3000    284570   44.34    9      242.593
pexels-steve-15267299.jpg                       lossy-slow  libwebp     2095   3000    247610   44.50    6      347.473
pexels-steve-15267299.jpg                       lossy-slow  ours        2095   3000    252782   44.41    3      828.842
pexels-steve-15267299.jpg                       lossy-slow  wasm        2095   3000    247610   44.50    2      1070.251
pexels-steve-29626041.jpg                       lossless    libwebp     2560   1440    283988   -        3      868.505
pexels-steve-29626041.jpg                       lossless    nativewebp  2560   1440    379050   -        4      651.193
pexels-steve-29626041.jpg                       lossless    ours        2560   1440    295168   -        3      856.099
pexels-steve-29626041.jpg                       lossless    wasm        2560   1440    296596   -        2      1766.941
pexels-steve-29626041.jpg                       lossy-fast  libwebp     2560   1440    47614    49.05    46     43.566
pexels-steve-29626041.jpg                       lossy-fast  ours        2560   1440    45686    49.31    42     47.751
pexels-steve-29626041.jpg                       lossy-fast  wasm        2560   1440    47614    49.05    17     124.340
pexels-steve-29626041.jpg                       lossy-slow  libwebp     2560   1440    38252    49.65    16     131.049
pexels-steve-29626041.jpg                       lossy-slow  ours        2560   1440    36254    49.62    9      230.710
pexels-steve-29626041.jpg                       lossy-slow  wasm        2560   1440    38252    49.65    5      471.633
pexels-toulouse-10807703.jpg                    lossless    libwebp     1400   2100    2688812  -        2      1056.458
pexels-toulouse-10807703.jpg                    lossless    nativewebp  1400   2100    2996214  -        4      663.957
pexels-toulouse-10807703.jpg                    lossless    ours        1400   2100    2721246  -        2      1176.307
pexels-toulouse-10807703.jpg                    lossless    wasm        1400   2100    2685806  -        1      2428.468
pexels-toulouse-10807703.jpg                    lossy-fast  libwebp     1400   2100    857060   39.84    26     78.262
pexels-toulouse-10807703.jpg                    lossy-fast  ours        1400   2100    854742   39.93    15     133.517
pexels-toulouse-10807703.jpg                    lossy-fast  wasm        1400   2100    857060   39.84    12     178.108
pexels-toulouse-10807703.jpg                    lossy-slow  libwebp     1400   2100    758466   39.84    5      454.115
pexels-toulouse-10807703.jpg                    lossy-slow  ours        1400   2100    776930   40.03    3      672.571
pexels-toulouse-10807703.jpg                    lossy-slow  wasm        1400   2100    758466   39.84    2      1098.522
```

Peak RSS, one encode per process:

```
file                                            mode        engine      width  height  megapixels  peak_rss_mib  mib_per_mp
Lena_512.png                                    lossless    libwebp     900    900     0.81        49.1          60.6
Lena_512.png                                    lossless    nativewebp  900    900     0.81        45.0          55.6
Lena_512.png                                    lossless    ours        900    900     0.81        115.2         142.2
Lena_512.png                                    lossless    wasm        900    900     0.81        116.8         144.2
Lena_512.png                                    lossy-fast  libwebp     900    900     0.81        25.1          30.9
Lena_512.png                                    lossy-fast  ours        900    900     0.81        24.1          29.7
Lena_512.png                                    lossy-fast  wasm        900    900     0.81        38.8          48.0
Lena_512.png                                    lossy-slow  libwebp     900    900     0.81        26.6          32.9
Lena_512.png                                    lossy-slow  ours        900    900     0.81        25.4          31.4
Lena_512.png                                    lossy-slow  wasm        900    900     0.81        49.0          60.5
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    libwebp     2025   2700    5.47        218.8         40.0
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    nativewebp  2025   2700    5.47        187.6         34.3
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    ours        2025   2700    5.47        820.9         150.1
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    wasm        2025   2700    5.47        700.1         128.0
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  libwebp     2025   2700    5.47        92.2          16.9
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  ours        2025   2700    5.47        69.2          12.7
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  wasm        2025   2700    5.47        198.0         36.2
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  libwebp     2025   2700    5.47        101.2         18.5
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  ours        2025   2700    5.47        75.3          13.8
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  wasm        2025   2700    5.47        198.0         36.2
pexels-martin-alargent-1165956-5665465.jpg      lossless    libwebp     2025   2700    5.47        210.6         38.5
pexels-martin-alargent-1165956-5665465.jpg      lossless    nativewebp  2025   2700    5.47        183.1         33.5
pexels-martin-alargent-1165956-5665465.jpg      lossless    ours        2025   2700    5.47        725.0         132.6
pexels-martin-alargent-1165956-5665465.jpg      lossless    wasm        2025   2700    5.47        700.0         128.0
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  libwebp     2025   2700    5.47        92.6          16.9
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  ours        2025   2700    5.47        69.0          12.6
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  wasm        2025   2700    5.47        198.0         36.2
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  libwebp     2025   2700    5.47        100.5         18.4
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  ours        2025   2700    5.47        74.5          13.6
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  wasm        2025   2700    5.47        198.2         36.3
pexels-mavihnt-38213559.jpg                     lossless    libwebp     2560   1706    4.37        184.7         42.3
pexels-mavihnt-38213559.jpg                     lossless    nativewebp  2560   1706    4.37        160.9         36.8
pexels-mavihnt-38213559.jpg                     lossless    ours        2560   1706    4.37        610.3         139.7
pexels-mavihnt-38213559.jpg                     lossless    wasm        2560   1706    4.37        562.9         128.9
pexels-mavihnt-38213559.jpg                     lossy-fast  libwebp     2560   1706    4.37        77.7          17.8
pexels-mavihnt-38213559.jpg                     lossy-fast  ours        2560   1706    4.37        57.4          13.2
pexels-mavihnt-38213559.jpg                     lossy-fast  wasm        2560   1706    4.37        160.8         36.8
pexels-mavihnt-38213559.jpg                     lossy-slow  libwebp     2560   1706    4.37        92.1          21.1
pexels-mavihnt-38213559.jpg                     lossy-slow  ours        2560   1706    4.37        59.0          13.5
pexels-mavihnt-38213559.jpg                     lossy-slow  wasm        2560   1706    4.37        225.2         51.6
pexels-steve-15267299.jpg                       lossless    libwebp     2095   3000    6.29        213.6         34.0
pexels-steve-15267299.jpg                       lossless    nativewebp  2095   3000    6.29        193.1         30.7
pexels-steve-15267299.jpg                       lossless    ours        2095   3000    6.29        754.8         120.1
pexels-steve-15267299.jpg                       lossless    wasm        2095   3000    6.29        577.5         91.9
pexels-steve-15267299.jpg                       lossy-fast  libwebp     2095   3000    6.29        102.4         16.3
pexels-steve-15267299.jpg                       lossy-fast  ours        2095   3000    6.29        77.5          12.3
pexels-steve-15267299.jpg                       lossy-fast  wasm        2095   3000    6.29        225.6         35.9
pexels-steve-15267299.jpg                       lossy-slow  libwebp     2095   3000    6.29        106.3         16.9
pexels-steve-15267299.jpg                       lossy-slow  ours        2095   3000    6.29        77.8          12.4
pexels-steve-15267299.jpg                       lossy-slow  wasm        2095   3000    6.29        225.9         35.9
pexels-steve-29626041.jpg                       lossless    libwebp     2560   1440    3.69        134.7         36.5
pexels-steve-29626041.jpg                       lossless    nativewebp  2560   1440    3.69        105.2         28.6
pexels-steve-29626041.jpg                       lossless    ours        2560   1440    3.69        357.2         96.9
pexels-steve-29626041.jpg                       lossless    wasm        2560   1440    3.69        344.4         93.4
pexels-steve-29626041.jpg                       lossy-fast  libwebp     2560   1440    3.69        64.4          17.5
pexels-steve-29626041.jpg                       lossy-fast  ours        2560   1440    3.69        50.1          13.6
pexels-steve-29626041.jpg                       lossy-fast  wasm        2560   1440    3.69        94.1          25.5
pexels-steve-29626041.jpg                       lossy-slow  libwebp     2560   1440    3.69        65.5          17.8
pexels-steve-29626041.jpg                       lossy-slow  ours        2560   1440    3.69        50.5          13.7
pexels-steve-29626041.jpg                       lossy-slow  wasm        2560   1440    3.69        137.5         37.3
pexels-toulouse-10807703.jpg                    lossless    libwebp     1400   2100    2.94        129.6         44.1
pexels-toulouse-10807703.jpg                    lossless    nativewebp  1400   2100    2.94        120.6         41.0
pexels-toulouse-10807703.jpg                    lossless    ours        1400   2100    2.94        351.9         119.7
pexels-toulouse-10807703.jpg                    lossless    wasm        1400   2100    2.94        384.7         130.9
pexels-toulouse-10807703.jpg                    lossy-fast  libwebp     1400   2100    2.94        56.5          19.2
pexels-toulouse-10807703.jpg                    lossy-fast  ours        1400   2100    2.94        42.3          14.4
pexels-toulouse-10807703.jpg                    lossy-fast  wasm        1400   2100    2.94        112.5         38.3
pexels-toulouse-10807703.jpg                    lossy-slow  libwebp     1400   2100    2.94        69.8          23.7
pexels-toulouse-10807703.jpg                    lossy-slow  ours        1400   2100    2.94        45.9          15.6
pexels-toulouse-10807703.jpg                    lossy-slow  wasm        1400   2100    2.94        156.1         53.1
```

Decode, one file per mode encoded by libwebp:

```
file                                            mode      engine   width  height  bytes    psnr_db  iters  ms_per_op
Lena_512.png                                    lossless  libwebp  900    900     627632   -        205    9.800
Lena_512.png                                    lossless  ours     900    900     627632   -        110    18.267
Lena_512.png                                    lossless  wasm     900    900     627632   28.40    84     23.998
Lena_512.png                                    lossless  x/image  900    900     627632   -        108    18.640
Lena_512.png                                    lossy     libwebp  900    900     89968    -        424    4.719
Lena_512.png                                    lossy     ours     900    900     89968    -        121    16.542
Lena_512.png                                    lossy     wasm     900    900     89968    28.34    130    15.420
Lena_512.png                                    lossy     x/image  900    900     89968    28.34    111    18.096
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp  2025   2700    3241976  -        36     55.896
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours     2025   2700    3241976  -        17     118.802
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm     2025   2700    3241976  29.06    14     147.798
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  x/image  2025   2700    3241976  -        20     105.016
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp  2025   2700    610518   -        59     33.953
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours     2025   2700    610518   -        17     122.640
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm     2025   2700    610518   29.09    18     113.183
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     x/image  2025   2700    610518   29.09    16     128.726
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp  2025   2700    2943528  -        36     56.758
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours     2025   2700    2943528  -        19     108.024
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm     2025   2700    2943528  30.36    14     149.261
pexels-martin-alargent-1165956-5665465.jpg      lossless  x/image  2025   2700    2943528  -        20     102.621
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp  2025   2700    603264   -        60     33.551
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours     2025   2700    603264   -        18     115.411
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm     2025   2700    603264   30.34    19     107.476
pexels-martin-alargent-1165956-5665465.jpg      lossy     x/image  2025   2700    603264   30.34    17     123.761
pexels-mavihnt-38213559.jpg                     lossless  libwebp  2560   1706    3482480  -        40     50.545
pexels-mavihnt-38213559.jpg                     lossless  ours     2560   1706    3482480  -        21     98.738
pexels-mavihnt-38213559.jpg                     lossless  wasm     2560   1706    3482480  27.55    16     128.148
pexels-mavihnt-38213559.jpg                     lossless  x/image  2560   1706    3482480  -        21     98.105
pexels-mavihnt-38213559.jpg                     lossy     libwebp  2560   1706    925032   -        49     41.505
pexels-mavihnt-38213559.jpg                     lossy     ours     2560   1706    925032   -        18     111.490
pexels-mavihnt-38213559.jpg                     lossy     wasm     2560   1706    925032   27.55    21     99.969
pexels-mavihnt-38213559.jpg                     lossy     x/image  2560   1706    925032   27.55    16     128.323
pexels-steve-15267299.jpg                       lossless  libwebp  2095   3000    2104690  -        36     56.794
pexels-steve-15267299.jpg                       lossless  ours     2095   3000    2104690  -        20     102.474
pexels-steve-15267299.jpg                       lossless  wasm     2095   3000    2104690  28.98    13     156.798
pexels-steve-15267299.jpg                       lossless  x/image  2095   3000    2104690  -        20     100.848
pexels-steve-15267299.jpg                       lossy     libwebp  2095   3000    247610   -        100    20.185
pexels-steve-15267299.jpg                       lossy     ours     2095   3000    247610   -        21     96.448
pexels-steve-15267299.jpg                       lossy     wasm     2095   3000    247610   28.98    22     93.509
pexels-steve-15267299.jpg                       lossy     x/image  2095   3000    247610   28.98    21     97.135
pexels-steve-29626041.jpg                       lossless  libwebp  2560   1440    283988   -        113    17.755
pexels-steve-29626041.jpg                       lossless  ours     2560   1440    283988   -        76     26.566
pexels-steve-29626041.jpg                       lossless  wasm     2560   1440    283988   32.17    30     67.551
pexels-steve-29626041.jpg                       lossless  x/image  2560   1440    283988   -        59     33.928
pexels-steve-29626041.jpg                       lossy     libwebp  2560   1440    38252    -        286    7.000
pexels-steve-29626041.jpg                       lossy     ours     2560   1440    38252    -        50     40.216
pexels-steve-29626041.jpg                       lossy     wasm     2560   1440    38252    32.10    46     43.503
pexels-steve-29626041.jpg                       lossy     x/image  2560   1440    38252    32.10    49     41.106
pexels-toulouse-10807703.jpg                    lossless  libwebp  1400   2100    2688812  -        56     36.130
pexels-toulouse-10807703.jpg                    lossless  ours     1400   2100    2688812  -        31     66.324
pexels-toulouse-10807703.jpg                    lossless  wasm     1400   2100    2688812  27.59    23     88.286
pexels-toulouse-10807703.jpg                    lossless  x/image  1400   2100    2688812  -        28     73.546
pexels-toulouse-10807703.jpg                    lossy     libwebp  1400   2100    758466   -        61     33.141
pexels-toulouse-10807703.jpg                    lossy     ours     1400   2100    758466   -        24     85.429
pexels-toulouse-10807703.jpg                    lossy     wasm     1400   2100    758466   27.54    28     73.466
pexels-toulouse-10807703.jpg                    lossy     x/image  1400   2100    758466   27.54    21     96.116
```

Effort sweep, every setting of every engine:

```
file                                            mode      engine      effort  width  height  bytes    psnr_db  iters  ms_per_op
Lena_512.png                                    lossless  libwebp     0       900    900     734910   -        48     21.265
Lena_512.png                                    lossless  libwebp     1       900    900     661194   -        11     99.407
Lena_512.png                                    lossless  libwebp     2       900    900     625874   -        7      149.009
Lena_512.png                                    lossless  libwebp     3       900    900     628862   -        6      174.771
Lena_512.png                                    lossless  libwebp     4       900    900     628612   -        6      186.393
Lena_512.png                                    lossless  libwebp     5       900    900     628612   -        6      189.102
Lena_512.png                                    lossless  libwebp     6       900    900     627632   -        4      253.548
Lena_512.png                                    lossless  libwebp     7       900    900     625628   -        3      356.222
Lena_512.png                                    lossless  libwebp     8       900    900     616842   -        2      755.730
Lena_512.png                                    lossless  libwebp     9       900    900     609124   -        1      3972.955
Lena_512.png                                    lossless  nativewebp  0       900    900     741562   -        7      152.349
Lena_512.png                                    lossless  nativewebp  4       900    900     739400   -        7      158.357
Lena_512.png                                    lossless  nativewebp  6       900    900     738176   -        6      172.155
Lena_512.png                                    lossless  ours        0       900    900     695082   -        22     46.654
Lena_512.png                                    lossless  ours        1       900    900     691746   -        19     54.725
Lena_512.png                                    lossless  ours        2       900    900     661658   -        7      147.474
Lena_512.png                                    lossless  ours        3       900    900     661658   -        7      163.423
Lena_512.png                                    lossless  ours        4       900    900     660788   -        6      190.633
Lena_512.png                                    lossless  ours        5       900    900     657842   -        4      260.153
Lena_512.png                                    lossless  ours        6       900    900     638220   -        4      317.556
Lena_512.png                                    lossless  wasm        0       900    900     731954   -        10     106.173
Lena_512.png                                    lossless  wasm        1       900    900     639584   -        3      400.078
Lena_512.png                                    lossless  wasm        2       900    900     627632   -        3      451.250
Lena_512.png                                    lossless  wasm        3       900    900     627632   -        3      445.187
Lena_512.png                                    lossless  wasm        4       900    900     627632   -        3      444.467
Lena_512.png                                    lossless  wasm        5       900    900     618010   -        1      1001.719
Lena_512.png                                    lossless  wasm        6       900    900     622766   -        2      952.994
Lena_512.png                                    lossy     libwebp     0       900    900     103980   41.04    64     15.703
Lena_512.png                                    lossy     libwebp     1       900    900     102500   41.05    49     20.650
Lena_512.png                                    lossy     libwebp     2       900    900     94342    40.70    47     21.307
Lena_512.png                                    lossy     libwebp     3       900    900     91788    40.98    24     42.316
Lena_512.png                                    lossy     libwebp     4       900    900     92124    40.97    24     42.465
Lena_512.png                                    lossy     libwebp     5       900    900     91586    40.91    22     47.434
Lena_512.png                                    lossy     libwebp     6       900    900     89968    40.94    13     77.979
Lena_512.png                                    lossy     ours        0       900    900     113182   41.02    43     23.481
Lena_512.png                                    lossy     ours        1       900    900     111048   41.05    29     35.239
Lena_512.png                                    lossy     ours        2       900    900     111048   41.05    33     30.992
Lena_512.png                                    lossy     ours        3       900    900     101404   41.15    20     52.511
Lena_512.png                                    lossy     ours        4       900    900     101404   41.15    22     46.691
Lena_512.png                                    lossy     ours        5       900    900     101404   41.15    21     48.730
Lena_512.png                                    lossy     ours        6       900    900     96566    41.08    18     56.218
Lena_512.png                                    lossy     ours        7       900    900     96566    41.08    18     56.831
Lena_512.png                                    lossy     ours        8       900    900     95710    41.21    12     90.272
Lena_512.png                                    lossy     ours        9       900    900     95710    41.13    7      150.514
Lena_512.png                                    lossy     wasm        0       900    900     103980   41.04    27     38.018
Lena_512.png                                    lossy     wasm        1       900    900     102500   41.05    19     52.932
Lena_512.png                                    lossy     wasm        2       900    900     94342    40.70    18     56.182
Lena_512.png                                    lossy     wasm        3       900    900     91788    40.98    7      161.787
Lena_512.png                                    lossy     wasm        4       900    900     92124    40.97    7      162.243
Lena_512.png                                    lossy     wasm        5       900    900     91586    40.91    6      177.952
Lena_512.png                                    lossy     wasm        6       900    900     89968    40.94    5      201.477
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     0       2025   2700    3987548  -        6      176.876
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     1       2025   2700    3992712  -        2      684.294
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     2       2025   2700    3993532  -        2      877.962
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     3       2025   2700    3246280  -        1      1113.536
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     4       2025   2700    3246304  -        1      1253.666
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     5       2025   2700    3242976  -        1      1362.048
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     6       2025   2700    3241976  -        1      2038.568
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     7       2025   2700    3237862  -        1      3296.149
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     8       2025   2700    3246580  -        1      3703.311
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     9       2025   2700    3245966  -        1      20955.456
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  nativewebp  0       2025   2700    3944492  -        1      1599.253
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  nativewebp  4       2025   2700    3938686  -        1      1629.021
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  nativewebp  6       2025   2700    3926412  -        1      1717.303
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        0       2025   2700    3496516  -        4      306.091
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        1       2025   2700    3486442  -        3      351.210
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        2       2025   2700    3398814  -        2      889.359
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        3       2025   2700    3288396  -        1      1035.457
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        4       2025   2700    3289656  -        1      1229.083
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        5       2025   2700    3289656  -        1      1662.513
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        6       2025   2700    3269944  -        1      2112.783
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        0       2025   2700    3961206  -        1      1399.147
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        1       2025   2700    3244586  -        1      3136.612
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        2       2025   2700    3244586  -        1      3142.588
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        3       2025   2700    3244586  -        1      3138.928
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        4       2025   2700    3241976  -        1      3327.313
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        5       2025   2700    3249798  -        1      3955.184
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        6       2025   2700    3249798  -        1      3802.404
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     0       2025   2700    598034   41.96    10     100.359
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     1       2025   2700    588954   41.97    8      131.157
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     2       2025   2700    614528   42.20    7      152.695
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     3       2025   2700    627994   42.39    4      321.467
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     4       2025   2700    632636   42.42    4      323.112
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     5       2025   2700    625540   42.27    3      370.909
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     6       2025   2700    610518   42.58    2      680.120
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        0       2025   2700    723064   42.02    7      154.800
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        1       2025   2700    694902   42.09    5      235.774
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        2       2025   2700    694902   42.09    5      213.491
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        3       2025   2700    686064   42.28    3      392.207
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        4       2025   2700    686064   42.28    3      417.846
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        5       2025   2700    686064   42.28    3      394.033
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        6       2025   2700    611704   42.07    3      432.661
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        7       2025   2700    611704   42.07    3      432.920
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        8       2025   2700    603030   42.33    2      761.904
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        9       2025   2700    603030   42.33    1      1341.180
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        0       2025   2700    598034   41.96    5      247.205
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        1       2025   2700    588954   41.97    3      344.530
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        2       2025   2700    614528   42.20    3      401.207
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        3       2025   2700    627994   42.39    1      1183.669
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        4       2025   2700    632636   42.42    1      1177.589
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        5       2025   2700    625540   42.27    1      1295.899
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        6       2025   2700    610518   42.58    1      1609.304
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     0       2025   2700    3673696  -        6      186.429
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     1       2025   2700    3494146  -        2      759.131
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     2       2025   2700    3493206  -        2      979.305
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     3       2025   2700    2949858  -        1      1167.326
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     4       2025   2700    2950040  -        1      1296.232
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     5       2025   2700    2943056  -        1      1414.764
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     6       2025   2700    2943528  -        1      2052.969
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     7       2025   2700    2929290  -        1      2855.713
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     8       2025   2700    2938176  -        1      3474.222
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     9       2025   2700    2940364  -        1      24489.164
pexels-martin-alargent-1165956-5665465.jpg      lossless  nativewebp  0       2025   2700    3554880  -        1      1315.561
pexels-martin-alargent-1165956-5665465.jpg      lossless  nativewebp  4       2025   2700    3559008  -        1      1357.570
pexels-martin-alargent-1165956-5665465.jpg      lossless  nativewebp  6       2025   2700    3563584  -        1      1459.664
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        0       2025   2700    3178224  -        4      288.305
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        1       2025   2700    3178224  -        3      340.503
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        2       2025   2700    2936002  -        2      862.005
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        3       2025   2700    2936002  -        1      1002.683
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        4       2025   2700    2936002  -        1      1183.519
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        5       2025   2700    2936002  -        1      1602.874
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        6       2025   2700    2919664  -        1      2020.066
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        0       2025   2700    3625218  -        1      1311.852
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        1       2025   2700    2948150  -        1      3085.370
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        2       2025   2700    2948150  -        1      3099.756
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        3       2025   2700    2948150  -        1      3106.416
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        4       2025   2700    2943528  -        1      3339.066
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        5       2025   2700    2953798  -        1      4117.723
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        6       2025   2700    2953798  -        1      3770.110
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     0       2025   2700    726824   42.63    10     102.222
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     1       2025   2700    664274   42.64    8      134.911
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     2       2025   2700    626708   42.42    7      151.775
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     3       2025   2700    616320   42.96    4      301.323
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     4       2025   2700    618856   42.83    4      300.775
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     5       2025   2700    615274   42.71    3      337.912
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     6       2025   2700    603264   42.75    2      568.402
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        0       2025   2700    756806   42.39    7      154.599
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        1       2025   2700    740538   42.48    5      240.675
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        2       2025   2700    740538   42.48    5      241.010
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        3       2025   2700    662858   42.65    3      357.959
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        4       2025   2700    662858   42.65    3      357.279
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        5       2025   2700    662858   42.65    3      357.849
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        6       2025   2700    622244   42.52    3      418.515
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        7       2025   2700    622244   42.52    3      411.171
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        8       2025   2700    616898   42.71    2      735.894
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        9       2025   2700    616898   42.97    1      1103.096
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        0       2025   2700    726824   42.63    5      246.975
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        1       2025   2700    664274   42.64    3      352.335
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        2       2025   2700    626708   42.42    3      394.377
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        3       2025   2700    616320   42.96    1      1115.794
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        4       2025   2700    618856   42.83    1      1137.729
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        5       2025   2700    615274   42.71    1      1193.790
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        6       2025   2700    603264   42.75    1      1410.871
pexels-mavihnt-38213559.jpg                     lossless  libwebp     0       2560   1706    4156742  -        8      132.841
pexels-mavihnt-38213559.jpg                     lossless  libwebp     1       2560   1706    3973816  -        2      529.841
pexels-mavihnt-38213559.jpg                     lossless  libwebp     2       2560   1706    3988280  -        2      656.010
pexels-mavihnt-38213559.jpg                     lossless  libwebp     3       2560   1706    3487548  -        2      876.627
pexels-mavihnt-38213559.jpg                     lossless  libwebp     4       2560   1706    3488224  -        2      952.741
pexels-mavihnt-38213559.jpg                     lossless  libwebp     5       2560   1706    3482892  -        1      1054.948
pexels-mavihnt-38213559.jpg                     lossless  libwebp     6       2560   1706    3482480  -        1      1549.732
pexels-mavihnt-38213559.jpg                     lossless  libwebp     7       2560   1706    3481722  -        1      2493.687
pexels-mavihnt-38213559.jpg                     lossless  libwebp     8       2560   1706    3485234  -        1      2906.014
pexels-mavihnt-38213559.jpg                     lossless  libwebp     9       2560   1706    3485216  -        1      13994.034
pexels-mavihnt-38213559.jpg                     lossless  nativewebp  0       2560   1706    4004276  -        1      1107.018
pexels-mavihnt-38213559.jpg                     lossless  nativewebp  4       2560   1706    4007402  -        1      1091.633
pexels-mavihnt-38213559.jpg                     lossless  nativewebp  6       2560   1706    4009534  -        1      1187.766
pexels-mavihnt-38213559.jpg                     lossless  ours        0       2560   1706    3667900  -        5      246.860
pexels-mavihnt-38213559.jpg                     lossless  ours        1       2560   1706    3667900  -        4      292.651
pexels-mavihnt-38213559.jpg                     lossless  ours        2       2560   1706    3515976  -        2      749.829
pexels-mavihnt-38213559.jpg                     lossless  ours        3       2560   1706    3515976  -        2      853.156
pexels-mavihnt-38213559.jpg                     lossless  ours        4       2560   1706    3515976  -        1      1024.228
pexels-mavihnt-38213559.jpg                     lossless  ours        5       2560   1706    3515976  -        1      1372.123
pexels-mavihnt-38213559.jpg                     lossless  ours        6       2560   1706    3509912  -        1      1754.065
pexels-mavihnt-38213559.jpg                     lossless  wasm        0       2560   1706    4116810  -        2      1000.944
pexels-mavihnt-38213559.jpg                     lossless  wasm        1       2560   1706    3484606  -        1      2500.062
pexels-mavihnt-38213559.jpg                     lossless  wasm        2       2560   1706    3484606  -        1      2466.514
pexels-mavihnt-38213559.jpg                     lossless  wasm        3       2560   1706    3484606  -        1      2464.846
pexels-mavihnt-38213559.jpg                     lossless  wasm        4       2560   1706    3482480  -        1      2664.049
pexels-mavihnt-38213559.jpg                     lossless  wasm        5       2560   1706    3488584  -        1      3237.197
pexels-mavihnt-38213559.jpg                     lossless  wasm        6       2560   1706    3488584  -        1      3077.179
pexels-mavihnt-38213559.jpg                     lossy     libwebp     0       2560   1706    983360   40.87    10     108.671
pexels-mavihnt-38213559.jpg                     lossy     libwebp     1       2560   1706    976610   40.88    8      134.555
pexels-mavihnt-38213559.jpg                     lossy     libwebp     2       2560   1706    935824   40.67    7      147.302
pexels-mavihnt-38213559.jpg                     lossy     libwebp     3       2560   1706    931074   41.15    4      293.471
pexels-mavihnt-38213559.jpg                     lossy     libwebp     4       2560   1706    934782   41.17    4      282.101
pexels-mavihnt-38213559.jpg                     lossy     libwebp     5       2560   1706    933892   41.05    4      327.707
pexels-mavihnt-38213559.jpg                     lossy     libwebp     6       2560   1706    925032   41.14    2      668.980
pexels-mavihnt-38213559.jpg                     lossy     ours        0       2560   1706    1037210  40.99    6      170.392
pexels-mavihnt-38213559.jpg                     lossy     ours        1       2560   1706    1019438  41.05    5      229.430
pexels-mavihnt-38213559.jpg                     lossy     ours        2       2560   1706    1019438  41.05    5      246.702
pexels-mavihnt-38213559.jpg                     lossy     ours        3       2560   1706    987826   41.15    3      377.087
pexels-mavihnt-38213559.jpg                     lossy     ours        4       2560   1706    987826   41.15    3      396.477
pexels-mavihnt-38213559.jpg                     lossy     ours        5       2560   1706    987826   41.15    3      377.321
pexels-mavihnt-38213559.jpg                     lossy     ours        6       2560   1706    952402   41.01    3      423.610
pexels-mavihnt-38213559.jpg                     lossy     ours        7       2560   1706    952402   41.01    3      422.568
pexels-mavihnt-38213559.jpg                     lossy     ours        8       2560   1706    948446   41.27    2      732.622
pexels-mavihnt-38213559.jpg                     lossy     ours        9       2560   1706    948446   41.27    1      1329.100
pexels-mavihnt-38213559.jpg                     lossy     wasm        0       2560   1706    983360   40.87    5      247.043
pexels-mavihnt-38213559.jpg                     lossy     wasm        1       2560   1706    976610   40.88    3      337.334
pexels-mavihnt-38213559.jpg                     lossy     wasm        2       2560   1706    935824   40.67    3      373.244
pexels-mavihnt-38213559.jpg                     lossy     wasm        3       2560   1706    931074   41.15    1      1079.690
pexels-mavihnt-38213559.jpg                     lossy     wasm        4       2560   1706    934782   41.17    1      1077.689
pexels-mavihnt-38213559.jpg                     lossy     wasm        5       2560   1706    933892   41.05    1      1169.230
pexels-mavihnt-38213559.jpg                     lossy     wasm        6       2560   1706    925032   41.14    1      1575.819
pexels-steve-15267299.jpg                       lossless  libwebp     0       2095   3000    2603112  -        6      177.228
pexels-steve-15267299.jpg                       lossless  libwebp     1       2095   3000    2557750  -        2      808.891
pexels-steve-15267299.jpg                       lossless  libwebp     2       2095   3000    2550344  -        1      1082.798
pexels-steve-15267299.jpg                       lossless  libwebp     3       2095   3000    2104000  -        1      1319.111
pexels-steve-15267299.jpg                       lossless  libwebp     4       2095   3000    2104010  -        1      1453.469
pexels-steve-15267299.jpg                       lossless  libwebp     5       2095   3000    2104010  -        1      1566.618
pexels-steve-15267299.jpg                       lossless  libwebp     6       2095   3000    2104690  -        1      2170.270
pexels-steve-15267299.jpg                       lossless  libwebp     7       2095   3000    2096804  -        1      2733.971
pexels-steve-15267299.jpg                       lossless  libwebp     8       2095   3000    2111270  -        1      3884.366
pexels-steve-15267299.jpg                       lossless  libwebp     9       2095   3000    2111200  -        1      21126.578
pexels-steve-15267299.jpg                       lossless  nativewebp  0       2095   3000    2635282  -        1      1448.943
pexels-steve-15267299.jpg                       lossless  nativewebp  4       2095   3000    2627962  -        1      1480.341
pexels-steve-15267299.jpg                       lossless  nativewebp  6       2095   3000    2622528  -        1      1613.185
pexels-steve-15267299.jpg                       lossless  ours        0       2095   3000    2233424  -        4      294.655
pexels-steve-15267299.jpg                       lossless  ours        1       2095   3000    2224752  -        3      369.424
pexels-steve-15267299.jpg                       lossless  ours        2       2095   3000    2045866  -        2      888.130
pexels-steve-15267299.jpg                       lossless  ours        3       2095   3000    2045866  -        1      1024.626
pexels-steve-15267299.jpg                       lossless  ours        4       2095   3000    2036060  -        1      1212.812
pexels-steve-15267299.jpg                       lossless  ours        5       2095   3000    2030150  -        1      1683.930
pexels-steve-15267299.jpg                       lossless  ours        6       2095   3000    2024034  -        1      2143.396
pexels-steve-15267299.jpg                       lossless  wasm        0       2095   3000    2546112  -        1      1111.720
pexels-steve-15267299.jpg                       lossless  wasm        1       2095   3000    2103668  -        1      3173.827
pexels-steve-15267299.jpg                       lossless  wasm        2       2095   3000    2103668  -        1      3167.880
pexels-steve-15267299.jpg                       lossless  wasm        3       2095   3000    2103668  -        1      3187.124
pexels-steve-15267299.jpg                       lossless  wasm        4       2095   3000    2104690  -        1      3433.725
pexels-steve-15267299.jpg                       lossless  wasm        5       2095   3000    2119370  -        1      4147.299
pexels-steve-15267299.jpg                       lossless  wasm        6       2095   3000    2119370  -        1      3980.956
pexels-steve-15267299.jpg                       lossy     libwebp     0       2095   3000    284570   44.34    11     91.553
pexels-steve-15267299.jpg                       lossy     libwebp     1       2095   3000    275444   44.38    9      121.684
pexels-steve-15267299.jpg                       lossy     libwebp     2       2095   3000    261602   44.30    9      119.362
pexels-steve-15267299.jpg                       lossy     libwebp     3       2095   3000    255364   44.59    4      255.106
pexels-steve-15267299.jpg                       lossy     libwebp     4       2095   3000    256038   44.58    4      262.975
pexels-steve-15267299.jpg                       lossy     libwebp     5       2095   3000    253192   44.51    4      281.559
pexels-steve-15267299.jpg                       lossy     libwebp     6       2095   3000    247610   44.50    3      343.539
pexels-steve-15267299.jpg                       lossy     ours        0       2095   3000    294606   43.91    9      112.436
pexels-steve-15267299.jpg                       lossy     ours        1       2095   3000    287430   44.01    6      172.184
pexels-steve-15267299.jpg                       lossy     ours        2       2095   3000    287430   44.01    6      199.979
pexels-steve-15267299.jpg                       lossy     ours        3       2095   3000    264092   44.12    4      261.015
pexels-steve-15267299.jpg                       lossy     ours        4       2095   3000    264092   44.12    4      263.270
pexels-steve-15267299.jpg                       lossy     ours        5       2095   3000    264092   44.12    4      259.136
pexels-steve-15267299.jpg                       lossy     ours        6       2095   3000    247744   44.09    5      227.677
pexels-steve-15267299.jpg                       lossy     ours        7       2095   3000    247744   44.09    5      230.132
pexels-steve-15267299.jpg                       lossy     ours        8       2095   3000    252782   44.23    3      460.975
pexels-steve-15267299.jpg                       lossy     ours        9       2095   3000    252782   44.41    2      821.813
pexels-steve-15267299.jpg                       lossy     wasm        0       2095   3000    284570   44.34    5      238.056
pexels-steve-15267299.jpg                       lossy     wasm        1       2095   3000    275444   44.38    3      337.587
pexels-steve-15267299.jpg                       lossy     wasm        2       2095   3000    261602   44.30    3      341.318
pexels-steve-15267299.jpg                       lossy     wasm        3       2095   3000    255364   44.59    1      1044.028
pexels-steve-15267299.jpg                       lossy     wasm        4       2095   3000    256038   44.58    1      1031.302
pexels-steve-15267299.jpg                       lossy     wasm        5       2095   3000    253192   44.51    1      1097.358
pexels-steve-15267299.jpg                       lossy     wasm        6       2095   3000    247610   44.50    1      1062.287
pexels-steve-29626041.jpg                       lossless  libwebp     0       2560   1440    367064   -        22     46.576
pexels-steve-29626041.jpg                       lossless  libwebp     1       2560   1440    360562   -        3      377.501
pexels-steve-29626041.jpg                       lossless  libwebp     2       2560   1440    309376   -        2      578.345
pexels-steve-29626041.jpg                       lossless  libwebp     3       2560   1440    292034   -        2      612.275
pexels-steve-29626041.jpg                       lossless  libwebp     4       2560   1440    287166   -        2      639.977
pexels-steve-29626041.jpg                       lossless  libwebp     5       2560   1440    287662   -        2      703.372
pexels-steve-29626041.jpg                       lossless  libwebp     6       2560   1440    283988   -        2      871.146
pexels-steve-29626041.jpg                       lossless  libwebp     7       2560   1440    282342   -        2      910.895
pexels-steve-29626041.jpg                       lossless  libwebp     8       2560   1440    294462   -        1      1179.646
pexels-steve-29626041.jpg                       lossless  libwebp     9       2560   1440    292264   -        1      4093.991
pexels-steve-29626041.jpg                       lossless  nativewebp  0       2560   1440    378544   -        2      587.970
pexels-steve-29626041.jpg                       lossless  nativewebp  4       2560   1440    378536   -        2      588.001
pexels-steve-29626041.jpg                       lossless  nativewebp  6       2560   1440    379050   -        2      657.264
pexels-steve-29626041.jpg                       lossless  ours        0       2560   1440    312250   -        9      121.774
pexels-steve-29626041.jpg                       lossless  ours        1       2560   1440    312250   -        7      158.295
pexels-steve-29626041.jpg                       lossless  ours        2       2560   1440    300532   -        4      329.447
pexels-steve-29626041.jpg                       lossless  ours        3       2560   1440    299726   -        3      393.388
pexels-steve-29626041.jpg                       lossless  ours        4       2560   1440    299726   -        2      500.680
pexels-steve-29626041.jpg                       lossless  ours        5       2560   1440    297802   -        2      717.124
pexels-steve-29626041.jpg                       lossless  ours        6       2560   1440    295168   -        2      854.311
pexels-steve-29626041.jpg                       lossless  wasm        0       2560   1440    346824   -        5      232.682
pexels-steve-29626041.jpg                       lossless  wasm        1       2560   1440    283552   -        1      1239.225
pexels-steve-29626041.jpg                       lossless  wasm        2       2560   1440    283552   -        1      1225.722
pexels-steve-29626041.jpg                       lossless  wasm        3       2560   1440    283552   -        1      1226.315
pexels-steve-29626041.jpg                       lossless  wasm        4       2560   1440    283988   -        1      1347.003
pexels-steve-29626041.jpg                       lossless  wasm        5       2560   1440    296596   -        1      1773.614
pexels-steve-29626041.jpg                       lossless  wasm        6       2560   1440    296596   -        1      1784.153
pexels-steve-29626041.jpg                       lossy     libwebp     0       2560   1440    47614    49.05    23     43.848
pexels-steve-29626041.jpg                       lossy     libwebp     1       2560   1440    47302    49.05    17     59.244
pexels-steve-29626041.jpg                       lossy     libwebp     2       2560   1440    39488    49.30    20     51.307
pexels-steve-29626041.jpg                       lossy     libwebp     3       2560   1440    39096    49.71    9      112.101
pexels-steve-29626041.jpg                       lossy     libwebp     4       2560   1440    39310    49.71    9      112.932
pexels-steve-29626041.jpg                       lossy     libwebp     5       2560   1440    38754    49.65    9      123.731
pexels-steve-29626041.jpg                       lossy     libwebp     6       2560   1440    38252    49.65    8      134.848
pexels-steve-29626041.jpg                       lossy     ours        0       2560   1440    45686    49.31    21     48.540
pexels-steve-29626041.jpg                       lossy     ours        1       2560   1440    44820    49.35    11     96.386
pexels-steve-29626041.jpg                       lossy     ours        2       2560   1440    44820    49.35    11     96.357
pexels-steve-29626041.jpg                       lossy     ours        3       2560   1440    39426    49.40    14     72.526
pexels-steve-29626041.jpg                       lossy     ours        4       2560   1440    39426    49.40    11     97.547
pexels-steve-29626041.jpg                       lossy     ours        5       2560   1440    39426    49.40    11     98.139
pexels-steve-29626041.jpg                       lossy     ours        6       2560   1440    37562    49.37    14     75.217
pexels-steve-29626041.jpg                       lossy     ours        7       2560   1440    37562    49.37    12     88.803
pexels-steve-29626041.jpg                       lossy     ours        8       2560   1440    36254    49.45    8      131.100
pexels-steve-29626041.jpg                       lossy     ours        9       2560   1440    36254    49.62    4      289.790
pexels-steve-29626041.jpg                       lossy     wasm        0       2560   1440    47614    49.05    9      124.062
pexels-steve-29626041.jpg                       lossy     wasm        1       2560   1440    47302    49.05    6      181.225
pexels-steve-29626041.jpg                       lossy     wasm        2       2560   1440    39488    49.30    7      161.440
pexels-steve-29626041.jpg                       lossy     wasm        3       2560   1440    39096    49.71    3      491.291
pexels-steve-29626041.jpg                       lossy     wasm        4       2560   1440    39310    49.71    3      490.948
pexels-steve-29626041.jpg                       lossy     wasm        5       2560   1440    38754    49.65    2      545.099
pexels-steve-29626041.jpg                       lossy     wasm        6       2560   1440    38252    49.65    3      480.546
pexels-toulouse-10807703.jpg                    lossless  libwebp     0       1400   2100    3044142  -        13     83.248
pexels-toulouse-10807703.jpg                    lossless  libwebp     1       1400   2100    2807602  -        3      415.671
pexels-toulouse-10807703.jpg                    lossless  libwebp     2       1400   2100    2780320  -        2      527.949
pexels-toulouse-10807703.jpg                    lossless  libwebp     3       1400   2100    2693662  -        2      598.460
pexels-toulouse-10807703.jpg                    lossless  libwebp     4       1400   2100    2693662  -        2      644.163
pexels-toulouse-10807703.jpg                    lossless  libwebp     5       1400   2100    2691936  -        2      718.779
pexels-toulouse-10807703.jpg                    lossless  libwebp     6       1400   2100    2688812  -        1      1093.591
pexels-toulouse-10807703.jpg                    lossless  libwebp     7       1400   2100    2688410  -        1      1349.838
pexels-toulouse-10807703.jpg                    lossless  libwebp     8       1400   2100    2686410  -        1      2004.928
pexels-toulouse-10807703.jpg                    lossless  libwebp     9       1400   2100    2685794  -        1      8318.149
pexels-toulouse-10807703.jpg                    lossless  nativewebp  0       1400   2100    2989804  -        2      586.944
pexels-toulouse-10807703.jpg                    lossless  nativewebp  4       1400   2100    2992710  -        2      610.667
pexels-toulouse-10807703.jpg                    lossless  nativewebp  6       1400   2100    2996214  -        2      656.906
pexels-toulouse-10807703.jpg                    lossless  ours        0       1400   2100    2868724  -        7      165.850
pexels-toulouse-10807703.jpg                    lossless  ours        1       1400   2100    2868724  -        6      195.523
pexels-toulouse-10807703.jpg                    lossless  ours        2       1400   2100    2718058  -        2      511.829
pexels-toulouse-10807703.jpg                    lossless  ours        3       1400   2100    2718058  -        2      596.174
pexels-toulouse-10807703.jpg                    lossless  ours        4       1400   2100    2718058  -        2      698.438
pexels-toulouse-10807703.jpg                    lossless  ours        5       1400   2100    2718058  -        2      927.869
pexels-toulouse-10807703.jpg                    lossless  ours        6       1400   2100    2721246  -        1      1207.268
pexels-toulouse-10807703.jpg                    lossless  wasm        0       1400   2100    3029106  -        3      486.165
pexels-toulouse-10807703.jpg                    lossless  wasm        1       1400   2100    2690886  -        1      1764.189
pexels-toulouse-10807703.jpg                    lossless  wasm        2       1400   2100    2690886  -        1      1749.638
pexels-toulouse-10807703.jpg                    lossless  wasm        3       1400   2100    2690886  -        1      1730.983
pexels-toulouse-10807703.jpg                    lossless  wasm        4       1400   2100    2688812  -        1      1876.903
pexels-toulouse-10807703.jpg                    lossless  wasm        5       1400   2100    2685806  -        1      2828.845
pexels-toulouse-10807703.jpg                    lossless  wasm        6       1400   2100    2685806  -        1      2374.204
pexels-toulouse-10807703.jpg                    lossy     libwebp     0       1400   2100    857060   39.84    13     80.757
pexels-toulouse-10807703.jpg                    lossy     libwebp     1       1400   2100    812040   39.84    10     106.523
pexels-toulouse-10807703.jpg                    lossy     libwebp     2       1400   2100    796814   39.60    9      114.085
pexels-toulouse-10807703.jpg                    lossy     libwebp     3       1400   2100    783482   39.95    6      197.921
pexels-toulouse-10807703.jpg                    lossy     libwebp     4       1400   2100    784734   39.96    6      194.912
pexels-toulouse-10807703.jpg                    lossy     libwebp     5       1400   2100    768426   39.81    5      223.956
pexels-toulouse-10807703.jpg                    lossy     libwebp     6       1400   2100    758466   39.84    3      453.719
pexels-toulouse-10807703.jpg                    lossy     ours        0       1400   2100    854742   39.93    8      134.131
pexels-toulouse-10807703.jpg                    lossy     ours        1       1400   2100    844538   39.95    6      185.180
pexels-toulouse-10807703.jpg                    lossy     ours        2       1400   2100    844538   39.95    6      171.047
pexels-toulouse-10807703.jpg                    lossy     ours        3       1400   2100    812788   40.06    4      273.873
pexels-toulouse-10807703.jpg                    lossy     ours        4       1400   2100    812788   40.06    4      277.285
pexels-toulouse-10807703.jpg                    lossy     ours        5       1400   2100    812788   40.06    4      262.327
pexels-toulouse-10807703.jpg                    lossy     ours        6       1400   2100    778956   39.92    4      299.091
pexels-toulouse-10807703.jpg                    lossy     ours        7       1400   2100    778956   39.92    4      298.787
pexels-toulouse-10807703.jpg                    lossy     ours        8       1400   2100    776930   40.10    2      513.566
pexels-toulouse-10807703.jpg                    lossy     ours        9       1400   2100    776930   40.03    2      697.992
pexels-toulouse-10807703.jpg                    lossy     wasm        0       1400   2100    857060   39.84    6      178.585
pexels-toulouse-10807703.jpg                    lossy     wasm        1       1400   2100    812040   39.84    5      246.056
pexels-toulouse-10807703.jpg                    lossy     wasm        2       1400   2100    796814   39.60    4      263.605
pexels-toulouse-10807703.jpg                    lossy     wasm        3       1400   2100    783482   39.95    2      752.387
pexels-toulouse-10807703.jpg                    lossy     wasm        4       1400   2100    784734   39.96    2      746.763
pexels-toulouse-10807703.jpg                    lossy     wasm        5       1400   2100    768426   39.81    2      804.406
pexels-toulouse-10807703.jpg                    lossy     wasm        6       1400   2100    758466   39.84    1      1077.808
```


## amd64 (AMD Ryzen 7 5700G) / photos

```
file                                            mode        engine      width  height  bytes    psnr_db  iters  ms_per_op
Lena_512.png                                    lossless    libwebp     900    900     627632   -        8      276.154
Lena_512.png                                    lossless    nativewebp  900    900     738176   -        7      331.280
Lena_512.png                                    lossless    ours        900    900     638220   -        4      530.262
Lena_512.png                                    lossless    wasm        900    900     622766   -        2      1538.277
Lena_512.png                                    lossy-fast  libwebp     900    900     103980   41.04    115    17.508
Lena_512.png                                    lossy-fast  ours        900    900     113182   41.02    68     29.841
Lena_512.png                                    lossy-fast  wasm        900    900     103980   41.04    30     67.254
Lena_512.png                                    lossy-slow  libwebp     900    900     89968    40.94    19     109.727
Lena_512.png                                    lossy-slow  ours        900    900     95710    41.13    11     197.235
Lena_512.png                                    lossy-slow  wasm        900    900     89968    40.94    6      366.688
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    libwebp     2025   2700    3241976  -        1      2193.094
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    nativewebp  2025   2700    3926412  -        1      3139.574
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    ours        2025   2700    3269944  -        1      3405.910
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    wasm        2025   2700    3249798  -        1      5855.370
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  libwebp     2025   2700    598034   41.96    18     115.499
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  ours        2025   2700    723064   42.02    11     196.728
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  wasm        2025   2700    598034   41.96    5      463.216
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  libwebp     2025   2700    610518   42.58    3      975.573
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  ours        2025   2700    603030   42.33    2      1676.465
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  wasm        2025   2700    610518   42.58    1      2900.400
pexels-martin-alargent-1165956-5665465.jpg      lossless    libwebp     2025   2700    2943528  -        1      2149.014
pexels-martin-alargent-1165956-5665465.jpg      lossless    nativewebp  2025   2700    3563584  -        1      2692.343
pexels-martin-alargent-1165956-5665465.jpg      lossless    ours        2025   2700    2919664  -        1      3328.983
pexels-martin-alargent-1165956-5665465.jpg      lossless    wasm        2025   2700    2953798  -        1      5836.763
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  libwebp     2025   2700    726824   42.63    17     118.514
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  ours        2025   2700    756806   42.39    11     195.814
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  wasm        2025   2700    726824   42.63    5      467.379
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  libwebp     2025   2700    603264   42.75    3      810.009
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  ours        2025   2700    616898   42.97    2      1369.044
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  wasm        2025   2700    603264   42.75    1      2596.537
pexels-mavihnt-38213559.jpg                     lossless    libwebp     2560   1706    3482480  -        2      1680.703
pexels-mavihnt-38213559.jpg                     lossless    nativewebp  2560   1706    4009534  -        1      2343.682
pexels-mavihnt-38213559.jpg                     lossless    ours        2560   1706    3509912  -        1      2913.392
pexels-mavihnt-38213559.jpg                     lossless    wasm        2560   1706    3488584  -        1      4869.690
pexels-mavihnt-38213559.jpg                     lossy-fast  libwebp     2560   1706    983360   40.87    17     120.524
pexels-mavihnt-38213559.jpg                     lossy-fast  ours        2560   1706    1037210  40.99    10     210.843
pexels-mavihnt-38213559.jpg                     lossy-fast  wasm        2560   1706    983360   40.87    5      430.887
pexels-mavihnt-38213559.jpg                     lossy-slow  libwebp     2560   1706    925032   41.14    3      964.977
pexels-mavihnt-38213559.jpg                     lossy-slow  ours        2560   1706    948446   41.27    2      1583.706
pexels-mavihnt-38213559.jpg                     lossy-slow  wasm        2560   1706    925032   41.14    1      2896.175
pexels-steve-15267299.jpg                       lossless    libwebp     2095   3000    2104690  -        1      2187.655
pexels-steve-15267299.jpg                       lossless    nativewebp  2095   3000    2622528  -        1      3056.962
pexels-steve-15267299.jpg                       lossless    ours        2095   3000    2024034  -        1      3638.833
pexels-steve-15267299.jpg                       lossless    wasm        2095   3000    2119370  -        1      6264.006
pexels-steve-15267299.jpg                       lossy-fast  libwebp     2095   3000    284570   44.34    19     106.384
pexels-steve-15267299.jpg                       lossy-fast  ours        2095   3000    294606   43.91    13     155.996
pexels-steve-15267299.jpg                       lossy-fast  wasm        2095   3000    284570   44.34    5      461.665
pexels-steve-15267299.jpg                       lossy-slow  libwebp     2095   3000    247610   44.50    4      515.498
pexels-steve-15267299.jpg                       lossy-slow  ours        2095   3000    252782   44.41    2      1129.746
pexels-steve-15267299.jpg                       lossy-slow  wasm        2095   3000    247610   44.50    1      2167.219
pexels-steve-29626041.jpg                       lossless    libwebp     2560   1440    283988   -        3      826.249
pexels-steve-29626041.jpg                       lossless    nativewebp  2560   1440    379050   -        2      1371.335
pexels-steve-29626041.jpg                       lossless    ours        2560   1440    295168   -        2      1464.711
pexels-steve-29626041.jpg                       lossless    wasm        2560   1440    296596   -        1      2895.740
pexels-steve-29626041.jpg                       lossy-fast  libwebp     2560   1440    47614    49.05    40     50.741
pexels-steve-29626041.jpg                       lossy-fast  ours        2560   1440    45686    49.31    29     70.951
pexels-steve-29626041.jpg                       lossy-fast  wasm        2560   1440    47614    49.05    9      244.380
pexels-steve-29626041.jpg                       lossy-slow  libwebp     2560   1440    38252    49.65    10     205.612
pexels-steve-29626041.jpg                       lossy-slow  ours        2560   1440    36254    49.62    6      350.024
pexels-steve-29626041.jpg                       lossy-slow  wasm        2560   1440    38252    49.65    3      963.032
pexels-toulouse-10807703.jpg                    lossless    libwebp     1400   2100    2688812  -        2      1160.370
pexels-toulouse-10807703.jpg                    lossless    nativewebp  1400   2100    2996214  -        2      1332.382
pexels-toulouse-10807703.jpg                    lossless    ours        1400   2100    2721246  -        2      1843.306
pexels-toulouse-10807703.jpg                    lossless    wasm        1400   2100    2685806  -        1      4007.330
pexels-toulouse-10807703.jpg                    lossy-fast  libwebp     1400   2100    857060   39.84    22     91.726
pexels-toulouse-10807703.jpg                    lossy-fast  ours        1400   2100    854742   39.93    13     161.411
pexels-toulouse-10807703.jpg                    lossy-fast  wasm        1400   2100    857060   39.84    7      295.263
pexels-toulouse-10807703.jpg                    lossy-slow  libwebp     1400   2100    758466   39.84    4      662.266
pexels-toulouse-10807703.jpg                    lossy-slow  ours        1400   2100    776930   40.03    3      831.328
pexels-toulouse-10807703.jpg                    lossy-slow  wasm        1400   2100    758466   39.84    2      1900.399
```

Peak RSS, one encode per process:

```
file                                            mode        engine      width  height  megapixels  peak_rss_mib  mib_per_mp
Lena_512.png                                    lossless    libwebp     900    900     0.81        46.2          57.1
Lena_512.png                                    lossless    nativewebp  900    900     0.81        46.2          57.1
Lena_512.png                                    lossless    ours        900    900     0.81        116.0         143.3
Lena_512.png                                    lossless    wasm        900    900     0.81        115.6         142.7
Lena_512.png                                    lossy-fast  libwebp     900    900     0.81        25.1          30.9
Lena_512.png                                    lossy-fast  ours        900    900     0.81        22.0          27.1
Lena_512.png                                    lossy-fast  wasm        900    900     0.81        39.3          48.5
Lena_512.png                                    lossy-slow  libwebp     900    900     0.81        28.3          35.0
Lena_512.png                                    lossy-slow  ours        900    900     0.81        26.2          32.3
Lena_512.png                                    lossy-slow  wasm        900    900     0.81        49.2          60.7
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    libwebp     2025   2700    5.47        209.6         38.3
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    nativewebp  2025   2700    5.47        186.8         34.2
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    ours        2025   2700    5.47        756.7         138.4
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    wasm        2025   2700    5.47        699.6         128.0
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  libwebp     2025   2700    5.47        90.7          16.6
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  ours        2025   2700    5.47        76.1          13.9
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  wasm        2025   2700    5.47        197.6         36.1
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  libwebp     2025   2700    5.47        99.5          18.2
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  ours        2025   2700    5.47        92.4          16.9
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  wasm        2025   2700    5.47        195.3         35.7
pexels-martin-alargent-1165956-5665465.jpg      lossless    libwebp     2025   2700    5.47        201.2         36.8
pexels-martin-alargent-1165956-5665465.jpg      lossless    nativewebp  2025   2700    5.47        181.1         33.1
pexels-martin-alargent-1165956-5665465.jpg      lossless    ours        2025   2700    5.47        658.5         120.4
pexels-martin-alargent-1165956-5665465.jpg      lossless    wasm        2025   2700    5.47        699.4         127.9
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  libwebp     2025   2700    5.47        90.9          16.6
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  ours        2025   2700    5.47        78.2          14.3
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  wasm        2025   2700    5.47        197.3         36.1
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  libwebp     2025   2700    5.47        99.0          18.1
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  ours        2025   2700    5.47        90.3          16.5
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  wasm        2025   2700    5.47        196.4         35.9
pexels-mavihnt-38213559.jpg                     lossless    libwebp     2560   1706    4.37        173.5         39.7
pexels-mavihnt-38213559.jpg                     lossless    nativewebp  2560   1706    4.37        159.4         36.5
pexels-mavihnt-38213559.jpg                     lossless    ours        2560   1706    4.37        609.1         139.5
pexels-mavihnt-38213559.jpg                     lossless    wasm        2560   1706    4.37        562.1         128.7
pexels-mavihnt-38213559.jpg                     lossy-fast  libwebp     2560   1706    4.37        75.7          17.3
pexels-mavihnt-38213559.jpg                     lossy-fast  ours        2560   1706    4.37        66.1          15.1
pexels-mavihnt-38213559.jpg                     lossy-fast  wasm        2560   1706    4.37        161.3         36.9
pexels-mavihnt-38213559.jpg                     lossy-slow  libwebp     2560   1706    4.37        89.4          20.5
pexels-mavihnt-38213559.jpg                     lossy-slow  ours        2560   1706    4.37        78.1          17.9
pexels-mavihnt-38213559.jpg                     lossy-slow  wasm        2560   1706    4.37        224.3         51.4
pexels-steve-15267299.jpg                       lossless    libwebp     2095   3000    6.29        207.6         33.0
pexels-steve-15267299.jpg                       lossless    nativewebp  2095   3000    6.29        191.3         30.4
pexels-steve-15267299.jpg                       lossless    ours        2095   3000    6.29        744.3         118.4
pexels-steve-15267299.jpg                       lossless    wasm        2095   3000    6.29        576.0         91.6
pexels-steve-15267299.jpg                       lossy-fast  libwebp     2095   3000    6.29        102.6         16.3
pexels-steve-15267299.jpg                       lossy-fast  ours        2095   3000    6.29        86.3          13.7
pexels-steve-15267299.jpg                       lossy-fast  wasm        2095   3000    6.29        224.2         35.7
pexels-steve-15267299.jpg                       lossy-slow  libwebp     2095   3000    6.29        106.0         16.9
pexels-steve-15267299.jpg                       lossy-slow  ours        2095   3000    6.29        96.1          15.3
pexels-steve-15267299.jpg                       lossy-slow  wasm        2095   3000    6.29        224.0         35.6
pexels-steve-29626041.jpg                       lossless    libwebp     2560   1440    3.69        127.9         34.7
pexels-steve-29626041.jpg                       lossless    nativewebp  2560   1440    3.69        106.5         28.9
pexels-steve-29626041.jpg                       lossless    ours        2560   1440    3.69        354.7         96.2
pexels-steve-29626041.jpg                       lossless    wasm        2560   1440    3.69        345.5         93.7
pexels-steve-29626041.jpg                       lossy-fast  libwebp     2560   1440    3.69        66.2          18.0
pexels-steve-29626041.jpg                       lossy-fast  ours        2560   1440    3.69        54.2          14.7
pexels-steve-29626041.jpg                       lossy-fast  wasm        2560   1440    3.69        93.4          25.3
pexels-steve-29626041.jpg                       lossy-slow  libwebp     2560   1440    3.69        65.1          17.7
pexels-steve-29626041.jpg                       lossy-slow  ours        2560   1440    3.69        60.3          16.3
pexels-steve-29626041.jpg                       lossy-slow  wasm        2560   1440    3.69        137.2         37.2
pexels-toulouse-10807703.jpg                    lossless    libwebp     1400   2100    2.94        120.4         41.0
pexels-toulouse-10807703.jpg                    lossless    nativewebp  1400   2100    2.94        118.4         40.3
pexels-toulouse-10807703.jpg                    lossless    ours        1400   2100    2.94        397.8         135.3
pexels-toulouse-10807703.jpg                    lossless    wasm        1400   2100    2.94        383.8         130.6
pexels-toulouse-10807703.jpg                    lossy-fast  libwebp     1400   2100    2.94        55.7          18.9
pexels-toulouse-10807703.jpg                    lossy-fast  ours        1400   2100    2.94        50.3          17.1
pexels-toulouse-10807703.jpg                    lossy-fast  wasm        1400   2100    2.94        113.4         38.6
pexels-toulouse-10807703.jpg                    lossy-slow  libwebp     1400   2100    2.94        67.4          22.9
pexels-toulouse-10807703.jpg                    lossy-slow  ours        1400   2100    2.94        56.2          19.1
pexels-toulouse-10807703.jpg                    lossy-slow  wasm        1400   2100    2.94        157.2         53.5
```

Decode, one file per mode encoded by libwebp:

```
file                                            mode      engine   width  height  bytes    psnr_db  iters  ms_per_op
Lena_512.png                                    lossless  libwebp  900    900     627632   -        204    9.834
Lena_512.png                                    lossless  ours     900    900     627632   -        104    19.232
Lena_512.png                                    lossless  wasm     900    900     627632   28.40    52     38.591
Lena_512.png                                    lossless  x/image  900    900     627632   -        89     22.535
Lena_512.png                                    lossy     libwebp  900    900     89968    -        304    6.601
Lena_512.png                                    lossy     ours     900    900     89968    -        91     22.143
Lena_512.png                                    lossy     wasm     900    900     89968    28.34    76     26.351
Lena_512.png                                    lossy     x/image  900    900     89968    28.34    83     24.169
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp  2025   2700    3241976  -        40     50.987
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours     2025   2700    3241976  -        17     118.503
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm     2025   2700    3241976  29.06    9      246.671
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  x/image  2025   2700    3241976  -        15     135.104
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp  2025   2700    610518   -        43     46.608
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours     2025   2700    610518   -        14     151.665
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm     2025   2700    610518   29.09    12     181.584
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     x/image  2025   2700    610518   29.09    12     168.295
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp  2025   2700    2943528  -        40     50.792
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours     2025   2700    2943528  -        18     112.180
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm     2025   2700    2943528  30.36    9      246.103
pexels-martin-alargent-1165956-5665465.jpg      lossless  x/image  2025   2700    2943528  -        16     126.982
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp  2025   2700    603264   -        45     45.264
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours     2025   2700    603264   -        14     149.020
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm     2025   2700    603264   30.34    12     173.655
pexels-martin-alargent-1165956-5665465.jpg      lossy     x/image  2025   2700    603264   30.34    13     162.134
pexels-mavihnt-38213559.jpg                     lossless  libwebp  2560   1706    3482480  -        42     48.178
pexels-mavihnt-38213559.jpg                     lossless  ours     2560   1706    3482480  -        21     96.477
pexels-mavihnt-38213559.jpg                     lossless  wasm     2560   1706    3482480  27.55    10     206.043
pexels-mavihnt-38213559.jpg                     lossless  x/image  2560   1706    3482480  -        18     115.183
pexels-mavihnt-38213559.jpg                     lossy     libwebp  2560   1706    925032   -        39     52.260
pexels-mavihnt-38213559.jpg                     lossy     ours     2560   1706    925032   -        16     132.496
pexels-mavihnt-38213559.jpg                     lossy     wasm     2560   1706    925032   27.55    13     162.972
pexels-mavihnt-38213559.jpg                     lossy     x/image  2560   1706    925032   27.55    14     148.279
pexels-steve-15267299.jpg                       lossless  libwebp  2095   3000    2104690  -        40     51.049
pexels-steve-15267299.jpg                       lossless  ours     2095   3000    2104690  -        16     125.078
pexels-steve-15267299.jpg                       lossless  wasm     2095   3000    2104690  28.98    8      258.347
pexels-steve-15267299.jpg                       lossless  x/image  2095   3000    2104690  -        15     137.582
pexels-steve-15267299.jpg                       lossy     libwebp  2095   3000    247610   -        66     30.734
pexels-steve-15267299.jpg                       lossy     ours     2095   3000    247610   -        15     136.576
pexels-steve-15267299.jpg                       lossy     wasm     2095   3000    247610   28.98    13     164.323
pexels-steve-15267299.jpg                       lossy     x/image  2095   3000    247610   28.98    14     148.353
pexels-steve-29626041.jpg                       lossless  libwebp  2560   1440    283988   -        130    15.448
pexels-steve-29626041.jpg                       lossless  ours     2560   1440    283988   -        45     45.132
pexels-steve-29626041.jpg                       lossless  wasm     2560   1440    283988   32.17    17     118.919
pexels-steve-29626041.jpg                       lossless  x/image  2560   1440    283988   -        38     53.452
pexels-steve-29626041.jpg                       lossy     libwebp  2560   1440    38252    -        180    11.142
pexels-steve-29626041.jpg                       lossy     ours     2560   1440    38252    -        33     60.652
pexels-steve-29626041.jpg                       lossy     wasm     2560   1440    38252    32.10    25     80.080
pexels-steve-29626041.jpg                       lossy     x/image  2560   1440    38252    32.10    29     69.202
pexels-toulouse-10807703.jpg                    lossless  libwebp  1400   2100    2688812  -        57     35.636
pexels-toulouse-10807703.jpg                    lossless  ours     1400   2100    2688812  -        30     68.717
pexels-toulouse-10807703.jpg                    lossless  wasm     1400   2100    2688812  27.59    15     142.040
pexels-toulouse-10807703.jpg                    lossless  x/image  1400   2100    2688812  -        24     85.833
pexels-toulouse-10807703.jpg                    lossy     libwebp  1400   2100    758466   -        49     41.642
pexels-toulouse-10807703.jpg                    lossy     ours     1400   2100    758466   -        20     100.134
pexels-toulouse-10807703.jpg                    lossy     wasm     1400   2100    758466   27.54    18     115.009
pexels-toulouse-10807703.jpg                    lossy     x/image  1400   2100    758466   27.54    19     109.695
```

Effort sweep, every setting of every engine:

```
file                                            mode      engine      effort  width  height  bytes    psnr_db  iters  ms_per_op
Lena_512.png                                    lossless  libwebp     0       900    900     734910   -        46     22.161
Lena_512.png                                    lossless  libwebp     1       900    900     661194   -        9      113.058
Lena_512.png                                    lossless  libwebp     2       900    900     625874   -        7      148.571
Lena_512.png                                    lossless  libwebp     3       900    900     628862   -        6      177.083
Lena_512.png                                    lossless  libwebp     4       900    900     628612   -        6      188.295
Lena_512.png                                    lossless  libwebp     5       900    900     628612   -        6      188.242
Lena_512.png                                    lossless  libwebp     6       900    900     627632   -        4      258.802
Lena_512.png                                    lossless  libwebp     7       900    900     625628   -        3      373.428
Lena_512.png                                    lossless  libwebp     8       900    900     616842   -        2      657.347
Lena_512.png                                    lossless  libwebp     9       900    900     609124   -        1      3957.304
Lena_512.png                                    lossless  nativewebp  0       900    900     741562   -        4      257.493
Lena_512.png                                    lossless  nativewebp  4       900    900     739400   -        4      271.312
Lena_512.png                                    lossless  nativewebp  6       900    900     738176   -        4      319.799
Lena_512.png                                    lossless  ours        0       900    900     695082   -        14     74.295
Lena_512.png                                    lossless  ours        1       900    900     691746   -        12     88.595
Lena_512.png                                    lossless  ours        2       900    900     661658   -        5      237.759
Lena_512.png                                    lossless  ours        3       900    900     661658   -        4      260.255
Lena_512.png                                    lossless  ours        4       900    900     660788   -        4      308.585
Lena_512.png                                    lossless  ours        5       900    900     657842   -        3      412.926
Lena_512.png                                    lossless  ours        6       900    900     638220   -        2      507.970
Lena_512.png                                    lossless  wasm        0       900    900     731954   -        8      140.732
Lena_512.png                                    lossless  wasm        1       900    900     639584   -        2      632.880
Lena_512.png                                    lossless  wasm        2       900    900     627632   -        2      724.039
Lena_512.png                                    lossless  wasm        3       900    900     627632   -        2      730.254
Lena_512.png                                    lossless  wasm        4       900    900     627632   -        2      717.448
Lena_512.png                                    lossless  wasm        5       900    900     618010   -        1      1622.496
Lena_512.png                                    lossless  wasm        6       900    900     622766   -        1      1515.732
Lena_512.png                                    lossy     libwebp     0       900    900     103980   41.04    58     17.490
Lena_512.png                                    lossy     libwebp     1       900    900     102500   41.05    44     22.993
Lena_512.png                                    lossy     libwebp     2       900    900     94342    40.70    42     24.198
Lena_512.png                                    lossy     libwebp     3       900    900     91788    40.98    20     52.458
Lena_512.png                                    lossy     libwebp     4       900    900     92124    40.97    20     52.605
Lena_512.png                                    lossy     libwebp     5       900    900     91586    40.91    18     58.716
Lena_512.png                                    lossy     libwebp     6       900    900     89968    40.94    10     109.035
Lena_512.png                                    lossy     ours        0       900    900     113182   41.02    34     29.670
Lena_512.png                                    lossy     ours        1       900    900     111048   41.05    25     41.521
Lena_512.png                                    lossy     ours        2       900    900     111048   41.05    25     41.567
Lena_512.png                                    lossy     ours        3       900    900     101404   41.15    17     60.843
Lena_512.png                                    lossy     ours        4       900    900     101404   41.15    17     60.402
Lena_512.png                                    lossy     ours        5       900    900     101404   41.15    17     60.136
Lena_512.png                                    lossy     ours        6       900    900     96566    41.08    15     67.257
Lena_512.png                                    lossy     ours        7       900    900     96566    41.08    15     68.016
Lena_512.png                                    lossy     ours        8       900    900     95710    41.21    9      117.523
Lena_512.png                                    lossy     ours        9       900    900     95710    41.13    6      198.770
Lena_512.png                                    lossy     wasm        0       900    900     103980   41.04    15     66.989
Lena_512.png                                    lossy     wasm        1       900    900     102500   41.05    11     95.557
Lena_512.png                                    lossy     wasm        2       900    900     94342    40.70    11     97.493
Lena_512.png                                    lossy     wasm        3       900    900     91788    40.98    4      282.542
Lena_512.png                                    lossy     wasm        4       900    900     92124    40.97    4      282.848
Lena_512.png                                    lossy     wasm        5       900    900     91586    40.91    4      313.028
Lena_512.png                                    lossy     wasm        6       900    900     89968    40.94    3      378.590
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     0       2025   2700    3987548  -        5      215.768
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     1       2025   2700    3992712  -        2      779.344
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     2       2025   2700    3993532  -        2      904.405
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     3       2025   2700    3246280  -        1      1143.646
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     4       2025   2700    3246304  -        1      1294.373
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     5       2025   2700    3242976  -        1      1381.967
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     6       2025   2700    3241976  -        1      2032.674
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     7       2025   2700    3237862  -        1      3299.881
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     8       2025   2700    3246580  -        1      3553.058
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     9       2025   2700    3245966  -        1      22145.265
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  nativewebp  0       2025   2700    3944492  -        1      2790.638
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  nativewebp  4       2025   2700    3938686  -        1      2855.443
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  nativewebp  6       2025   2700    3926412  -        1      3168.228
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        0       2025   2700    3496516  -        3      480.899
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        1       2025   2700    3486442  -        2      545.492
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        2       2025   2700    3398814  -        1      1461.153
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        3       2025   2700    3288396  -        1      1609.390
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        4       2025   2700    3289656  -        1      1951.719
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        5       2025   2700    3289656  -        1      2666.679
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        6       2025   2700    3269944  -        1      3432.931
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        0       2025   2700    3961206  -        1      2015.699
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        1       2025   2700    3244586  -        1      4883.059
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        2       2025   2700    3244586  -        1      4957.104
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        3       2025   2700    3244586  -        1      4835.942
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        4       2025   2700    3241976  -        1      5188.607
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        5       2025   2700    3249798  -        1      6210.043
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        6       2025   2700    3249798  -        1      5794.963
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     0       2025   2700    598034   41.96    9      115.429
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     1       2025   2700    588954   41.97    7      151.533
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     2       2025   2700    614528   42.20    6      174.843
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     3       2025   2700    627994   42.39    3      389.939
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     4       2025   2700    632636   42.42    3      391.004
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     5       2025   2700    625540   42.27    3      450.971
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     6       2025   2700    610518   42.58    2      969.040
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        0       2025   2700    723064   42.02    6      196.298
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        1       2025   2700    694902   42.09    4      274.264
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        2       2025   2700    694902   42.09    4      279.035
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        3       2025   2700    686064   42.28    3      496.369
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        4       2025   2700    686064   42.28    3      498.228
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        5       2025   2700    686064   42.28    2      503.245
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        6       2025   2700    611704   42.07    2      562.602
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        7       2025   2700    611704   42.07    2      561.810
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        8       2025   2700    603030   42.33    2      967.923
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        9       2025   2700    603030   42.33    1      1710.970
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        0       2025   2700    598034   41.96    3      448.239
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        1       2025   2700    588954   41.97    2      645.381
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        2       2025   2700    614528   42.20    2      762.271
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        3       2025   2700    627994   42.39    1      2042.144
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        4       2025   2700    632636   42.42    1      2050.169
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        5       2025   2700    625540   42.27    1      2235.859
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        6       2025   2700    610518   42.58    1      2877.828
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     0       2025   2700    3673696  -        5      244.683
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     1       2025   2700    3494146  -        2      863.691
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     2       2025   2700    3493206  -        1      1008.284
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     3       2025   2700    2949858  -        1      1227.835
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     4       2025   2700    2950040  -        1      1368.285
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     5       2025   2700    2943056  -        1      1483.264
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     6       2025   2700    2943528  -        1      2141.191
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     7       2025   2700    2929290  -        1      3018.588
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     8       2025   2700    2938176  -        1      3516.548
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     9       2025   2700    2940364  -        1      26573.628
pexels-martin-alargent-1165956-5665465.jpg      lossless  nativewebp  0       2025   2700    3554880  -        1      2249.831
pexels-martin-alargent-1165956-5665465.jpg      lossless  nativewebp  4       2025   2700    3559008  -        1      2381.812
pexels-martin-alargent-1165956-5665465.jpg      lossless  nativewebp  6       2025   2700    3563584  -        1      2690.681
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        0       2025   2700    3178224  -        3      427.240
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        1       2025   2700    3178224  -        2      550.499
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        2       2025   2700    2936002  -        1      1438.836
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        3       2025   2700    2936002  -        1      1634.419
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        4       2025   2700    2936002  -        1      1895.701
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        5       2025   2700    2936002  -        1      2573.372
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        6       2025   2700    2919664  -        1      3246.471
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        0       2025   2700    3625218  -        1      1812.167
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        1       2025   2700    2948150  -        1      4680.593
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        2       2025   2700    2948150  -        1      4666.599
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        3       2025   2700    2948150  -        1      4746.744
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        4       2025   2700    2943528  -        1      5066.336
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        5       2025   2700    2953798  -        1      6510.635
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        6       2025   2700    2953798  -        1      5849.654
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     0       2025   2700    726824   42.63    9      117.196
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     1       2025   2700    664274   42.64    7      155.704
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     2       2025   2700    626708   42.42    6      175.099
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     3       2025   2700    616320   42.96    3      372.677
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     4       2025   2700    618856   42.83    3      372.969
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     5       2025   2700    615274   42.71    3      421.711
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     6       2025   2700    603264   42.75    2      811.386
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        0       2025   2700    756806   42.39    6      194.900
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        1       2025   2700    740538   42.48    4      273.001
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        2       2025   2700    740538   42.48    4      279.759
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        3       2025   2700    662858   42.65    3      464.862
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        4       2025   2700    662858   42.65    3      460.085
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        5       2025   2700    662858   42.65    3      457.274
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        6       2025   2700    622244   42.52    2      511.030
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        7       2025   2700    622244   42.52    2      509.780
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        8       2025   2700    616898   42.71    2      877.778
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        9       2025   2700    616898   42.97    1      1384.002
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        0       2025   2700    726824   42.63    3      470.092
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        1       2025   2700    664274   42.64    2      677.000
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        2       2025   2700    626708   42.42    2      747.689
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        3       2025   2700    616320   42.96    1      1951.100
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        4       2025   2700    618856   42.83    1      1942.935
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        5       2025   2700    615274   42.71    1      2125.238
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        6       2025   2700    603264   42.75    1      2574.571
pexels-mavihnt-38213559.jpg                     lossless  libwebp     0       2560   1706    4156742  -        6      172.815
pexels-mavihnt-38213559.jpg                     lossless  libwebp     1       2560   1706    3973816  -        2      628.072
pexels-mavihnt-38213559.jpg                     lossless  libwebp     2       2560   1706    3988280  -        2      710.518
pexels-mavihnt-38213559.jpg                     lossless  libwebp     3       2560   1706    3487548  -        2      962.943
pexels-mavihnt-38213559.jpg                     lossless  libwebp     4       2560   1706    3488224  -        1      1052.171
pexels-mavihnt-38213559.jpg                     lossless  libwebp     5       2560   1706    3482892  -        1      1159.301
pexels-mavihnt-38213559.jpg                     lossless  libwebp     6       2560   1706    3482480  -        1      1686.784
pexels-mavihnt-38213559.jpg                     lossless  libwebp     7       2560   1706    3481722  -        1      2725.253
pexels-mavihnt-38213559.jpg                     lossless  libwebp     8       2560   1706    3485234  -        1      3128.178
pexels-mavihnt-38213559.jpg                     lossless  libwebp     9       2560   1706    3485216  -        1      15441.331
pexels-mavihnt-38213559.jpg                     lossless  nativewebp  0       2560   1706    4004276  -        1      2045.946
pexels-mavihnt-38213559.jpg                     lossless  nativewebp  4       2560   1706    4007402  -        1      2083.062
pexels-mavihnt-38213559.jpg                     lossless  nativewebp  6       2560   1706    4009534  -        1      2451.676
pexels-mavihnt-38213559.jpg                     lossless  ours        0       2560   1706    3667900  -        3      392.426
pexels-mavihnt-38213559.jpg                     lossless  ours        1       2560   1706    3667900  -        3      451.897
pexels-mavihnt-38213559.jpg                     lossless  ours        2       2560   1706    3515976  -        1      1252.045
pexels-mavihnt-38213559.jpg                     lossless  ours        3       2560   1706    3515976  -        1      1398.290
pexels-mavihnt-38213559.jpg                     lossless  ours        4       2560   1706    3515976  -        1      1618.858
pexels-mavihnt-38213559.jpg                     lossless  ours        5       2560   1706    3515976  -        1      2197.019
pexels-mavihnt-38213559.jpg                     lossless  ours        6       2560   1706    3509912  -        1      2709.344
pexels-mavihnt-38213559.jpg                     lossless  wasm        0       2560   1706    4116810  -        1      1432.385
pexels-mavihnt-38213559.jpg                     lossless  wasm        1       2560   1706    3484606  -        1      4002.898
pexels-mavihnt-38213559.jpg                     lossless  wasm        2       2560   1706    3484606  -        1      3837.386
pexels-mavihnt-38213559.jpg                     lossless  wasm        3       2560   1706    3484606  -        1      3840.235
pexels-mavihnt-38213559.jpg                     lossless  wasm        4       2560   1706    3482480  -        1      4161.802
pexels-mavihnt-38213559.jpg                     lossless  wasm        5       2560   1706    3488584  -        1      5264.237
pexels-mavihnt-38213559.jpg                     lossless  wasm        6       2560   1706    3488584  -        1      4740.839
pexels-mavihnt-38213559.jpg                     lossy     libwebp     0       2560   1706    983360   40.87    9      121.514
pexels-mavihnt-38213559.jpg                     lossy     libwebp     1       2560   1706    976610   40.88    7      156.700
pexels-mavihnt-38213559.jpg                     lossy     libwebp     2       2560   1706    935824   40.67    6      175.422
pexels-mavihnt-38213559.jpg                     lossy     libwebp     3       2560   1706    931074   41.15    3      344.642
pexels-mavihnt-38213559.jpg                     lossy     libwebp     4       2560   1706    934782   41.17    3      343.009
pexels-mavihnt-38213559.jpg                     lossy     libwebp     5       2560   1706    933892   41.05    3      405.758
pexels-mavihnt-38213559.jpg                     lossy     libwebp     6       2560   1706    925032   41.14    2      959.944
pexels-mavihnt-38213559.jpg                     lossy     ours        0       2560   1706    1037210  40.99    5      209.348
pexels-mavihnt-38213559.jpg                     lossy     ours        1       2560   1706    1019438  41.05    4      282.387
pexels-mavihnt-38213559.jpg                     lossy     ours        2       2560   1706    1019438  41.05    4      285.057
pexels-mavihnt-38213559.jpg                     lossy     ours        3       2560   1706    987826   41.15    3      466.934
pexels-mavihnt-38213559.jpg                     lossy     ours        4       2560   1706    987826   41.15    3      470.532
pexels-mavihnt-38213559.jpg                     lossy     ours        5       2560   1706    987826   41.15    3      462.355
pexels-mavihnt-38213559.jpg                     lossy     ours        6       2560   1706    952402   41.01    2      541.486
pexels-mavihnt-38213559.jpg                     lossy     ours        7       2560   1706    952402   41.01    2      538.662
pexels-mavihnt-38213559.jpg                     lossy     ours        8       2560   1706    948446   41.27    2      905.513
pexels-mavihnt-38213559.jpg                     lossy     ours        9       2560   1706    948446   41.27    1      1589.286
pexels-mavihnt-38213559.jpg                     lossy     wasm        0       2560   1706    983360   40.87    3      417.716
pexels-mavihnt-38213559.jpg                     lossy     wasm        1       2560   1706    976610   40.88    2      596.904
pexels-mavihnt-38213559.jpg                     lossy     wasm        2       2560   1706    935824   40.67    2      656.508
pexels-mavihnt-38213559.jpg                     lossy     wasm        3       2560   1706    931074   41.15    1      1794.785
pexels-mavihnt-38213559.jpg                     lossy     wasm        4       2560   1706    934782   41.17    1      1798.167
pexels-mavihnt-38213559.jpg                     lossy     wasm        5       2560   1706    933892   41.05    1      2134.635
pexels-mavihnt-38213559.jpg                     lossy     wasm        6       2560   1706    925032   41.14    1      2740.899
pexels-steve-15267299.jpg                       lossless  libwebp     0       2095   3000    2603112  -        5      208.919
pexels-steve-15267299.jpg                       lossless  libwebp     1       2095   3000    2557750  -        2      881.822
pexels-steve-15267299.jpg                       lossless  libwebp     2       2095   3000    2550344  -        1      1065.007
pexels-steve-15267299.jpg                       lossless  libwebp     3       2095   3000    2104000  -        1      1334.929
pexels-steve-15267299.jpg                       lossless  libwebp     4       2095   3000    2104010  -        1      1465.608
pexels-steve-15267299.jpg                       lossless  libwebp     5       2095   3000    2104010  -        1      1601.656
pexels-steve-15267299.jpg                       lossless  libwebp     6       2095   3000    2104690  -        1      2149.627
pexels-steve-15267299.jpg                       lossless  libwebp     7       2095   3000    2096804  -        1      2771.216
pexels-steve-15267299.jpg                       lossless  libwebp     8       2095   3000    2111270  -        1      3896.280
pexels-steve-15267299.jpg                       lossless  libwebp     9       2095   3000    2111200  -        1      22098.082
pexels-steve-15267299.jpg                       lossless  nativewebp  0       2095   3000    2635282  -        1      2454.670
pexels-steve-15267299.jpg                       lossless  nativewebp  4       2095   3000    2627962  -        1      2704.337
pexels-steve-15267299.jpg                       lossless  nativewebp  6       2095   3000    2622528  -        1      2936.989
pexels-steve-15267299.jpg                       lossless  ours        0       2095   3000    2233424  -        3      497.440
pexels-steve-15267299.jpg                       lossless  ours        1       2095   3000    2224752  -        2      610.640
pexels-steve-15267299.jpg                       lossless  ours        2       2095   3000    2045866  -        1      1468.067
pexels-steve-15267299.jpg                       lossless  ours        3       2095   3000    2045866  -        1      1620.548
pexels-steve-15267299.jpg                       lossless  ours        4       2095   3000    2036060  -        1      2022.976
pexels-steve-15267299.jpg                       lossless  ours        5       2095   3000    2030150  -        1      2766.095
pexels-steve-15267299.jpg                       lossless  ours        6       2095   3000    2024034  -        1      3561.443
pexels-steve-15267299.jpg                       lossless  wasm        0       2095   3000    2546112  -        1      1371.045
pexels-steve-15267299.jpg                       lossless  wasm        1       2095   3000    2103668  -        1      4749.185
pexels-steve-15267299.jpg                       lossless  wasm        2       2095   3000    2103668  -        1      4783.339
pexels-steve-15267299.jpg                       lossless  wasm        3       2095   3000    2103668  -        1      4965.471
pexels-steve-15267299.jpg                       lossless  wasm        4       2095   3000    2104690  -        1      5241.911
pexels-steve-15267299.jpg                       lossless  wasm        5       2095   3000    2119370  -        1      6456.484
pexels-steve-15267299.jpg                       lossless  wasm        6       2095   3000    2119370  -        1      5956.372
pexels-steve-15267299.jpg                       lossy     libwebp     0       2095   3000    284570   44.34    10     105.168
pexels-steve-15267299.jpg                       lossy     libwebp     1       2095   3000    275444   44.38    8      140.679
pexels-steve-15267299.jpg                       lossy     libwebp     2       2095   3000    261602   44.30    8      138.863
pexels-steve-15267299.jpg                       lossy     libwebp     3       2095   3000    255364   44.59    3      350.487
pexels-steve-15267299.jpg                       lossy     libwebp     4       2095   3000    256038   44.58    3      341.924
pexels-steve-15267299.jpg                       lossy     libwebp     5       2095   3000    253192   44.51    3      376.507
pexels-steve-15267299.jpg                       lossy     libwebp     6       2095   3000    247610   44.50    2      510.534
pexels-steve-15267299.jpg                       lossy     ours        0       2095   3000    294606   43.91    7      159.416
pexels-steve-15267299.jpg                       lossy     ours        1       2095   3000    287430   44.01    5      238.580
pexels-steve-15267299.jpg                       lossy     ours        2       2095   3000    287430   44.01    5      236.502
pexels-steve-15267299.jpg                       lossy     ours        3       2095   3000    264092   44.12    4      308.788
pexels-steve-15267299.jpg                       lossy     ours        4       2095   3000    264092   44.12    4      305.509
pexels-steve-15267299.jpg                       lossy     ours        5       2095   3000    264092   44.12    4      316.382
pexels-steve-15267299.jpg                       lossy     ours        6       2095   3000    247744   44.09    3      355.394
pexels-steve-15267299.jpg                       lossy     ours        7       2095   3000    247744   44.09    3      335.598
pexels-steve-15267299.jpg                       lossy     ours        8       2095   3000    252782   44.23    2      647.763
pexels-steve-15267299.jpg                       lossy     ours        9       2095   3000    252782   44.41    1      1134.825
pexels-steve-15267299.jpg                       lossy     wasm        0       2095   3000    284570   44.34    3      457.567
pexels-steve-15267299.jpg                       lossy     wasm        1       2095   3000    275444   44.38    2      662.677
pexels-steve-15267299.jpg                       lossy     wasm        2       2095   3000    261602   44.30    2      621.131
pexels-steve-15267299.jpg                       lossy     wasm        3       2095   3000    255364   44.59    1      1992.549
pexels-steve-15267299.jpg                       lossy     wasm        4       2095   3000    256038   44.58    1      1917.302
pexels-steve-15267299.jpg                       lossy     wasm        5       2095   3000    253192   44.51    1      2008.429
pexels-steve-15267299.jpg                       lossy     wasm        6       2095   3000    247610   44.50    1      2179.975
pexels-steve-29626041.jpg                       lossless  libwebp     0       2560   1440    367064   -        18     57.469
pexels-steve-29626041.jpg                       lossless  libwebp     1       2560   1440    360562   -        3      383.559
pexels-steve-29626041.jpg                       lossless  libwebp     2       2560   1440    309376   -        2      515.881
pexels-steve-29626041.jpg                       lossless  libwebp     3       2560   1440    292034   -        2      549.108
pexels-steve-29626041.jpg                       lossless  libwebp     4       2560   1440    287166   -        2      574.138
pexels-steve-29626041.jpg                       lossless  libwebp     5       2560   1440    287662   -        2      622.573
pexels-steve-29626041.jpg                       lossless  libwebp     6       2560   1440    283988   -        2      726.010
pexels-steve-29626041.jpg                       lossless  libwebp     7       2560   1440    282342   -        2      769.816
pexels-steve-29626041.jpg                       lossless  libwebp     8       2560   1440    294462   -        1      1002.538
pexels-steve-29626041.jpg                       lossless  libwebp     9       2560   1440    292264   -        1      3885.289
pexels-steve-29626041.jpg                       lossless  nativewebp  0       2560   1440    378544   -        1      1080.192
pexels-steve-29626041.jpg                       lossless  nativewebp  4       2560   1440    378536   -        1      1137.782
pexels-steve-29626041.jpg                       lossless  nativewebp  6       2560   1440    379050   -        1      1370.164
pexels-steve-29626041.jpg                       lossless  ours        0       2560   1440    312250   -        5      209.843
pexels-steve-29626041.jpg                       lossless  ours        1       2560   1440    312250   -        4      266.552
pexels-steve-29626041.jpg                       lossless  ours        2       2560   1440    300532   -        2      549.379
pexels-steve-29626041.jpg                       lossless  ours        3       2560   1440    299726   -        2      670.863
pexels-steve-29626041.jpg                       lossless  ours        4       2560   1440    299726   -        2      858.527
pexels-steve-29626041.jpg                       lossless  ours        5       2560   1440    297802   -        1      1164.846
pexels-steve-29626041.jpg                       lossless  ours        6       2560   1440    295168   -        1      1456.562
pexels-steve-29626041.jpg                       lossless  wasm        0       2560   1440    346824   -        4      331.564
pexels-steve-29626041.jpg                       lossless  wasm        1       2560   1440    283552   -        1      1925.385
pexels-steve-29626041.jpg                       lossless  wasm        2       2560   1440    283552   -        1      1949.920
pexels-steve-29626041.jpg                       lossless  wasm        3       2560   1440    283552   -        1      1907.047
pexels-steve-29626041.jpg                       lossless  wasm        4       2560   1440    283988   -        1      2095.353
pexels-steve-29626041.jpg                       lossless  wasm        5       2560   1440    296596   -        1      2904.106
pexels-steve-29626041.jpg                       lossless  wasm        6       2560   1440    296596   -        1      2914.489
pexels-steve-29626041.jpg                       lossy     libwebp     0       2560   1440    47614    49.05    20     50.238
pexels-steve-29626041.jpg                       lossy     libwebp     1       2560   1440    47302    49.05    15     69.700
pexels-steve-29626041.jpg                       lossy     libwebp     2       2560   1440    39488    49.30    17     59.028
pexels-steve-29626041.jpg                       lossy     libwebp     3       2560   1440    39096    49.71    7      157.255
pexels-steve-29626041.jpg                       lossy     libwebp     4       2560   1440    39310    49.71    7      157.763
pexels-steve-29626041.jpg                       lossy     libwebp     5       2560   1440    38754    49.65    6      170.068
pexels-steve-29626041.jpg                       lossy     libwebp     6       2560   1440    38252    49.65    5      204.718
pexels-steve-29626041.jpg                       lossy     ours        0       2560   1440    45686    49.31    15     70.314
pexels-steve-29626041.jpg                       lossy     ours        1       2560   1440    44820    49.35    10     108.067
pexels-steve-29626041.jpg                       lossy     ours        2       2560   1440    44820    49.35    10     105.102
pexels-steve-29626041.jpg                       lossy     ours        3       2560   1440    39426    49.40    10     110.736
pexels-steve-29626041.jpg                       lossy     ours        4       2560   1440    39426    49.40    10     109.663
pexels-steve-29626041.jpg                       lossy     ours        5       2560   1440    39426    49.40    10     110.405
pexels-steve-29626041.jpg                       lossy     ours        6       2560   1440    37562    49.37    9      123.023
pexels-steve-29626041.jpg                       lossy     ours        7       2560   1440    37562    49.37    9      118.874
pexels-steve-29626041.jpg                       lossy     ours        8       2560   1440    36254    49.45    6      190.680
pexels-steve-29626041.jpg                       lossy     ours        9       2560   1440    36254    49.62    3      358.073
pexels-steve-29626041.jpg                       lossy     wasm        0       2560   1440    47614    49.05    5      242.525
pexels-steve-29626041.jpg                       lossy     wasm        1       2560   1440    47302    49.05    3      357.504
pexels-steve-29626041.jpg                       lossy     wasm        2       2560   1440    39488    49.30    4      290.202
pexels-steve-29626041.jpg                       lossy     wasm        3       2560   1440    39096    49.71    2      923.785
pexels-steve-29626041.jpg                       lossy     wasm        4       2560   1440    39310    49.71    2      895.016
pexels-steve-29626041.jpg                       lossy     wasm        5       2560   1440    38754    49.65    2      966.794
pexels-steve-29626041.jpg                       lossy     wasm        6       2560   1440    38252    49.65    2      958.995
pexels-toulouse-10807703.jpg                    lossless  libwebp     0       1400   2100    3044142  -        12     90.040
pexels-toulouse-10807703.jpg                    lossless  libwebp     1       1400   2100    2807602  -        3      469.842
pexels-toulouse-10807703.jpg                    lossless  libwebp     2       1400   2100    2780320  -        2      569.987
pexels-toulouse-10807703.jpg                    lossless  libwebp     3       1400   2100    2693662  -        2      624.494
pexels-toulouse-10807703.jpg                    lossless  libwebp     4       1400   2100    2693662  -        2      663.264
pexels-toulouse-10807703.jpg                    lossless  libwebp     5       1400   2100    2691936  -        2      750.102
pexels-toulouse-10807703.jpg                    lossless  libwebp     6       1400   2100    2688812  -        1      1133.425
pexels-toulouse-10807703.jpg                    lossless  libwebp     7       1400   2100    2688410  -        1      1412.517
pexels-toulouse-10807703.jpg                    lossless  libwebp     8       1400   2100    2686410  -        1      2087.801
pexels-toulouse-10807703.jpg                    lossless  libwebp     9       1400   2100    2685794  -        1      9034.709
pexels-toulouse-10807703.jpg                    lossless  nativewebp  0       1400   2100    2989804  -        1      1052.446
pexels-toulouse-10807703.jpg                    lossless  nativewebp  4       1400   2100    2992710  -        1      1122.404
pexels-toulouse-10807703.jpg                    lossless  nativewebp  6       1400   2100    2996214  -        1      1272.715
pexels-toulouse-10807703.jpg                    lossless  ours        0       1400   2100    2868724  -        4      250.273
pexels-toulouse-10807703.jpg                    lossless  ours        1       1400   2100    2868724  -        4      295.577
pexels-toulouse-10807703.jpg                    lossless  ours        2       1400   2100    2718058  -        2      752.007
pexels-toulouse-10807703.jpg                    lossless  ours        3       1400   2100    2718058  -        2      902.657
pexels-toulouse-10807703.jpg                    lossless  ours        4       1400   2100    2718058  -        1      1032.319
pexels-toulouse-10807703.jpg                    lossless  ours        5       1400   2100    2718058  -        1      1386.410
pexels-toulouse-10807703.jpg                    lossless  ours        6       1400   2100    2721246  -        1      1784.351
pexels-toulouse-10807703.jpg                    lossless  wasm        0       1400   2100    3029106  -        2      658.728
pexels-toulouse-10807703.jpg                    lossless  wasm        1       1400   2100    2690886  -        1      2775.579
pexels-toulouse-10807703.jpg                    lossless  wasm        2       1400   2100    2690886  -        1      2777.368
pexels-toulouse-10807703.jpg                    lossless  wasm        3       1400   2100    2690886  -        1      2791.176
pexels-toulouse-10807703.jpg                    lossless  wasm        4       1400   2100    2688812  -        1      3015.039
pexels-toulouse-10807703.jpg                    lossless  wasm        5       1400   2100    2685806  -        1      4758.011
pexels-toulouse-10807703.jpg                    lossless  wasm        6       1400   2100    2685806  -        1      3840.432
pexels-toulouse-10807703.jpg                    lossy     libwebp     0       1400   2100    857060   39.84    12     90.324
pexels-toulouse-10807703.jpg                    lossy     libwebp     1       1400   2100    812040   39.84    9      117.245
pexels-toulouse-10807703.jpg                    lossy     libwebp     2       1400   2100    796814   39.60    8      126.797
pexels-toulouse-10807703.jpg                    lossy     libwebp     3       1400   2100    783482   39.95    5      242.570
pexels-toulouse-10807703.jpg                    lossy     libwebp     4       1400   2100    784734   39.96    5      244.920
pexels-toulouse-10807703.jpg                    lossy     libwebp     5       1400   2100    768426   39.81    4      286.512
pexels-toulouse-10807703.jpg                    lossy     libwebp     6       1400   2100    758466   39.84    2      658.001
pexels-toulouse-10807703.jpg                    lossy     ours        0       1400   2100    854742   39.93    7      160.433
pexels-toulouse-10807703.jpg                    lossy     ours        1       1400   2100    844538   39.95    5      211.472
pexels-toulouse-10807703.jpg                    lossy     ours        2       1400   2100    844538   39.95    5      209.213
pexels-toulouse-10807703.jpg                    lossy     ours        3       1400   2100    812788   40.06    4      319.836
pexels-toulouse-10807703.jpg                    lossy     ours        4       1400   2100    812788   40.06    4      324.154
pexels-toulouse-10807703.jpg                    lossy     ours        5       1400   2100    812788   40.06    4      320.085
pexels-toulouse-10807703.jpg                    lossy     ours        6       1400   2100    778956   39.92    3      369.693
pexels-toulouse-10807703.jpg                    lossy     ours        7       1400   2100    778956   39.92    3      372.230
pexels-toulouse-10807703.jpg                    lossy     ours        8       1400   2100    776930   40.10    2      627.649
pexels-toulouse-10807703.jpg                    lossy     ours        9       1400   2100    776930   40.03    2      834.300
pexels-toulouse-10807703.jpg                    lossy     wasm        0       1400   2100    857060   39.84    4      288.863
pexels-toulouse-10807703.jpg                    lossy     wasm        1       1400   2100    812040   39.84    3      416.969
pexels-toulouse-10807703.jpg                    lossy     wasm        2       1400   2100    796814   39.60    3      442.856
pexels-toulouse-10807703.jpg                    lossy     wasm        3       1400   2100    783482   39.95    1      1235.120
pexels-toulouse-10807703.jpg                    lossy     wasm        4       1400   2100    784734   39.96    1      1234.278
pexels-toulouse-10807703.jpg                    lossy     wasm        5       1400   2100    768426   39.81    1      1378.676
pexels-toulouse-10807703.jpg                    lossy     wasm        6       1400   2100    758466   39.84    1      1845.782
```


## arm64 (Apple M4 Pro) / transparent

```
file          mode        engine      width  height  bytes   psnr_db  iters  ms_per_op
1_webp_a.png  lossless    libwebp     400    301     75254   -        20     104.608
1_webp_a.png  lossless    nativewebp  400    301     81934   -        69     28.986
1_webp_a.png  lossless    ours        400    301     79910   -        50     40.656
1_webp_a.png  lossless    wasm        400    301     74350   -        6      373.823
1_webp_a.png  lossy-fast  libwebp     400    301     20134   41.94    500    3.604
1_webp_a.png  lossy-fast  ours        400    301     21748   41.48    107    18.726
1_webp_a.png  lossy-fast  wasm        400    301     20134   41.94    229    8.745
1_webp_a.png  lossy-slow  libwebp     400    301     17032   42.23    2      1610.790
1_webp_a.png  lossy-slow  ours        400    301     18610   42.09    8      259.799
1_webp_a.png  lossy-slow  wasm        400    301     17032   42.23    1      2571.228
2_webp_a.png  lossless    libwebp     386    395     41648   -        16     130.383
2_webp_a.png  lossless    nativewebp  386    395     49924   -        56     35.927
2_webp_a.png  lossless    ours        386    395     43988   -        49     41.516
2_webp_a.png  lossless    wasm        386    395     41428   -        5      435.011
2_webp_a.png  lossy-fast  libwebp     386    395     18844   43.02    500    3.592
2_webp_a.png  lossy-fast  ours        386    395     20230   42.42    95     21.128
2_webp_a.png  lossy-fast  wasm        386    395     18844   43.02    219    9.152
2_webp_a.png  lossy-slow  libwebp     386    395     13994   43.38    2      1949.521
2_webp_a.png  lossy-slow  ours        386    395     14732   42.83    9      222.422
2_webp_a.png  lossy-slow  wasm        386    395     13994   43.38    1      2958.840
3_webp_a.png  lossless    libwebp     800    600     181124  -        13     157.816
3_webp_a.png  lossless    nativewebp  800    600     202886  -        27     75.903
3_webp_a.png  lossless    ours        800    600     188144  -        16     125.568
3_webp_a.png  lossless    wasm        800    600     178918  -        5      488.242
3_webp_a.png  lossy-fast  libwebp     800    600     80676   41.56    119    16.831
3_webp_a.png  lossy-fast  ours        800    600     73228   40.82    38     53.283
3_webp_a.png  lossy-fast  wasm        800    600     80676   41.56    53     37.824
3_webp_a.png  lossy-slow  libwebp     800    600     52308   41.61    1      2589.973
3_webp_a.png  lossy-slow  ours        800    600     59312   41.18    4      659.816
3_webp_a.png  lossy-slow  wasm        800    600     52308   41.61    1      4474.206
4_webp_a.png  lossless    libwebp     421    163     34420   -        34     59.970
4_webp_a.png  lossless    nativewebp  421    163     37928   -        121    16.620
4_webp_a.png  lossless    ours        421    163     38046   -        89     22.623
4_webp_a.png  lossless    wasm        421    163     33558   -        11     196.073
4_webp_a.png  lossy-fast  libwebp     421    163     23740   39.28    500    2.359
4_webp_a.png  lossy-fast  ours        421    163     22468   38.92    163    12.273
4_webp_a.png  lossy-fast  wasm        421    163     23740   39.28    355    5.649
4_webp_a.png  lossy-slow  libwebp     421    163     18758   39.73    4      652.304
4_webp_a.png  lossy-slow  ours        421    163     19190   39.33    10     207.055
4_webp_a.png  lossy-slow  wasm        421    163     18758   39.73    2      1093.453
5_webp_a.png  lossless    libwebp     300    300     140018  -        19     108.634
5_webp_a.png  lossless    nativewebp  300    300     154970  -        86     23.297
5_webp_a.png  lossless    ours        300    300     149708  -        50     40.359
5_webp_a.png  lossless    wasm        300    300     137538  -        6      352.985
5_webp_a.png  lossy-fast  libwebp     300    300     64290   32.77    399    5.014
5_webp_a.png  lossy-fast  ours        300    300     70196   33.07    83     24.238
5_webp_a.png  lossy-fast  wasm        300    300     64290   32.77    192    10.458
5_webp_a.png  lossy-slow  libwebp     300    300     55926   32.67    1      3208.426
5_webp_a.png  lossy-slow  ours        300    300     59608   33.23    8      276.537
5_webp_a.png  lossy-slow  wasm        300    300     55926   32.67    1      5442.844
```

Peak RSS, one encode per process:

```
file          mode        engine      width  height  megapixels  peak_rss_mib  mib_per_mp
1_webp_a.png  lossless    libwebp     400    301     0.12        34.5          286.5
1_webp_a.png  lossless    nativewebp  400    301     0.12        19.2          159.4
1_webp_a.png  lossless    ours        400    301     0.12        26.3          218.2
1_webp_a.png  lossless    wasm        400    301     0.12        62.9          522.1
1_webp_a.png  lossy-fast  libwebp     400    301     0.12        14.6          121.0
1_webp_a.png  lossy-fast  ours        400    301     0.12        21.1          174.9
1_webp_a.png  lossy-fast  wasm        400    301     0.12        24.9          207.1
1_webp_a.png  lossy-slow  libwebp     400    301     0.12        64.8          538.1
1_webp_a.png  lossy-slow  ours        400    301     0.12        36.7          304.5
1_webp_a.png  lossy-slow  wasm        400    301     0.12        102.4         850.3
2_webp_a.png  lossless    libwebp     386    395     0.15        45.3          296.9
2_webp_a.png  lossless    nativewebp  386    395     0.15        20.0          131.0
2_webp_a.png  lossless    ours        386    395     0.15        29.8          195.5
2_webp_a.png  lossless    wasm        386    395     0.15        96.0          629.7
2_webp_a.png  lossy-fast  libwebp     386    395     0.15        15.6          102.5
2_webp_a.png  lossy-fast  ours        386    395     0.15        26.9          176.6
2_webp_a.png  lossy-fast  wasm        386    395     0.15        36.5          239.3
2_webp_a.png  lossy-slow  libwebp     386    395     0.15        102.6         673.0
2_webp_a.png  lossy-slow  ours        386    395     0.15        45.9          301.1
2_webp_a.png  lossy-slow  wasm        386    395     0.15        131.0         859.3
3_webp_a.png  lossless    libwebp     800    600     0.48        40.2          83.7
3_webp_a.png  lossless    nativewebp  800    600     0.48        28.2          58.8
3_webp_a.png  lossless    ours        800    600     0.48        53.3          111.0
3_webp_a.png  lossless    wasm        800    600     0.48        108.4         225.8
3_webp_a.png  lossy-fast  libwebp     800    600     0.48        27.2          56.7
3_webp_a.png  lossy-fast  ours        800    600     0.48        50.9          106.0
3_webp_a.png  lossy-fast  wasm        800    600     0.48        64.7          134.7
3_webp_a.png  lossy-slow  libwebp     800    600     0.48        106.6         222.1
3_webp_a.png  lossy-slow  ours        800    600     0.48        111.0         231.3
3_webp_a.png  lossy-slow  wasm        800    600     0.48        228.0         475.1
4_webp_a.png  lossless    libwebp     421    163     0.07        27.6          402.8
4_webp_a.png  lossless    nativewebp  421    163     0.07        16.7          242.7
4_webp_a.png  lossless    ours        421    163     0.07        20.1          293.5
4_webp_a.png  lossless    wasm        421    163     0.07        39.8          580.2
4_webp_a.png  lossy-fast  libwebp     421    163     0.07        12.9          187.4
4_webp_a.png  lossy-fast  ours        421    163     0.07        19.3          280.7
4_webp_a.png  lossy-fast  wasm        421    163     0.07        20.7          301.0
4_webp_a.png  lossy-slow  libwebp     421    163     0.07        42.9          625.0
4_webp_a.png  lossy-slow  ours        421    163     0.07        29.8          433.8
4_webp_a.png  lossy-slow  wasm        421    163     0.07        87.5          1274.9
5_webp_a.png  lossless    libwebp     300    300     0.09        27.6          306.8
5_webp_a.png  lossless    nativewebp  300    300     0.09        19.5          216.5
5_webp_a.png  lossless    ours        300    300     0.09        25.7          285.8
5_webp_a.png  lossless    wasm        300    300     0.09        52.3          580.9
5_webp_a.png  lossy-fast  libwebp     300    300     0.09        13.8          153.1
5_webp_a.png  lossy-fast  ours        300    300     0.09        20.5          228.3
5_webp_a.png  lossy-fast  wasm        300    300     0.09        25.9          288.0
5_webp_a.png  lossy-slow  libwebp     300    300     0.09        73.5          816.1
5_webp_a.png  lossy-slow  ours        300    300     0.09        28.6          317.5
5_webp_a.png  lossy-slow  wasm        300    300     0.09        109.0         1210.6
```

Decode, one file per mode encoded by libwebp:

```
file          mode      engine   width  height  bytes   psnr_db  iters  ms_per_op
1_webp_a.png  lossless  libwebp  400    301     75254   -        500    1.069
1_webp_a.png  lossless  ours     400    301     75254   -        500    2.070
1_webp_a.png  lossless  wasm     400    301     75254   29.40    500    3.197
1_webp_a.png  lossless  x/image  400    301     75254   -        500    2.263
1_webp_a.png  lossy     libwebp  400    301     17032   -        500    0.611
1_webp_a.png  lossy     ours     400    301     17032   -        500    2.220
1_webp_a.png  lossy     wasm     400    301     17032   29.63    500    2.105
1_webp_a.png  lossy     x/image  400    301     17032   29.63    500    2.699
2_webp_a.png  lossless  libwebp  386    395     41648   -        500    0.817
2_webp_a.png  lossless  ours     386    395     41648   -        500    1.635
2_webp_a.png  lossless  wasm     386    395     41648   25.35    500    3.248
2_webp_a.png  lossless  x/image  386    395     41648   -        500    1.787
2_webp_a.png  lossy     libwebp  386    395     13994   -        500    0.771
2_webp_a.png  lossy     ours     386    395     13994   -        500    2.548
2_webp_a.png  lossy     wasm     386    395     13994   25.46    500    2.798
2_webp_a.png  lossy     x/image  386    395     13994   25.46    500    3.211
3_webp_a.png  lossless  libwebp  800    600     181124  -        500    3.009
3_webp_a.png  lossless  ours     800    600     181124  -        312    6.424
3_webp_a.png  lossless  wasm     800    600     181124  27.66    194    10.340
3_webp_a.png  lossless  x/image  800    600     181124  -        305    6.563
3_webp_a.png  lossy     libwebp  800    600     52308   -        500    2.164
3_webp_a.png  lossy     ours     800    600     52308   -        287    6.972
3_webp_a.png  lossy     wasm     800    600     52308   27.81    256    7.831
3_webp_a.png  lossy     x/image  800    600     52308   27.81    211    9.492
4_webp_a.png  lossless  libwebp  421    163     34420   -        500    0.469
4_webp_a.png  lossless  ours     421    163     34420   -        500    0.938
4_webp_a.png  lossless  wasm     421    163     34420   30.27    500    1.670
4_webp_a.png  lossless  x/image  421    163     34420   -        500    1.026
4_webp_a.png  lossy     libwebp  421    163     18758   -        500    0.551
4_webp_a.png  lossy     ours     421    163     18758   -        500    1.669
4_webp_a.png  lossy     wasm     421    163     18758   31.62    500    1.565
4_webp_a.png  lossy     x/image  421    163     18758   31.62    500    2.077
5_webp_a.png  lossless  libwebp  300    300     140018  -        500    1.339
5_webp_a.png  lossless  ours     300    300     140018  -        500    2.904
5_webp_a.png  lossless  wasm     300    300     140018  24.63    500    3.398
5_webp_a.png  lossless  x/image  300    300     140018  -        500    3.192
5_webp_a.png  lossy     libwebp  300    300     55926   -        500    1.722
5_webp_a.png  lossy     ours     300    300     55926   -        484    4.139
5_webp_a.png  lossy     wasm     300    300     55926   25.61    500    3.597
5_webp_a.png  lossy     x/image  300    300     55926   25.61    397    5.046
```

Effort sweep, every setting of every engine:

```
file          mode      engine      effort  width  height  bytes   psnr_db  iters  ms_per_op
1_webp_a.png  lossless  libwebp     0       400    301     89358   -        474    2.111
1_webp_a.png  lossless  libwebp     1       400    301     80522   -        69     14.621
1_webp_a.png  lossless  libwebp     2       400    301     77132   -        42     24.192
1_webp_a.png  lossless  libwebp     3       400    301     77242   -        28     36.964
1_webp_a.png  lossless  libwebp     4       400    301     76798   -        27     37.873
1_webp_a.png  lossless  libwebp     5       400    301     75278   -        12     90.725
1_webp_a.png  lossless  libwebp     6       400    301     75254   -        10     104.588
1_webp_a.png  lossless  libwebp     7       400    301     75162   -        10     108.327
1_webp_a.png  lossless  libwebp     8       400    301     73858   -        4      256.201
1_webp_a.png  lossless  libwebp     9       400    301     71932   -        1      1834.989
1_webp_a.png  lossless  nativewebp  0       400    301     83958   -        58     17.360
1_webp_a.png  lossless  nativewebp  4       400    301     81934   -        35     28.990
1_webp_a.png  lossless  nativewebp  6       400    301     81934   -        35     28.785
1_webp_a.png  lossless  ours        0       400    301     83468   -        187    5.361
1_webp_a.png  lossless  ours        1       400    301     82496   -        156    6.412
1_webp_a.png  lossless  ours        2       400    301     82476   -        56     17.884
1_webp_a.png  lossless  ours        3       400    301     82476   -        47     21.579
1_webp_a.png  lossless  ours        4       400    301     81832   -        41     24.656
1_webp_a.png  lossless  ours        5       400    301     80522   -        30     33.470
1_webp_a.png  lossless  ours        6       400    301     79910   -        25     41.251
1_webp_a.png  lossless  wasm        0       400    301     88930   -        156    6.414
1_webp_a.png  lossless  wasm        1       400    301     78950   -        23     43.675
1_webp_a.png  lossless  wasm        2       400    301     77160   -        19     55.348
1_webp_a.png  lossless  wasm        3       400    301     76576   -        13     77.214
1_webp_a.png  lossless  wasm        4       400    301     75254   -        7      166.137
1_webp_a.png  lossless  wasm        5       400    301     73842   -        3      398.161
1_webp_a.png  lossless  wasm        6       400    301     74350   -        3      371.872
1_webp_a.png  lossy     libwebp     0       400    301     20134   41.94    279    3.585
1_webp_a.png  lossy     libwebp     1       400    301     18496   41.98    230    4.357
1_webp_a.png  lossy     libwebp     2       400    301     17446   41.40    219    4.583
1_webp_a.png  lossy     libwebp     3       400    301     17122   42.23    134    7.488
1_webp_a.png  lossy     libwebp     4       400    301     17142   42.25    111    9.031
1_webp_a.png  lossy     libwebp     5       400    301     17228   42.22    83     12.186
1_webp_a.png  lossy     libwebp     6       400    301     17032   42.23    1      1613.213
1_webp_a.png  lossy     ours        0       400    301     21748   41.48    53     19.184
1_webp_a.png  lossy     ours        1       400    301     21436   41.55    33     30.807
1_webp_a.png  lossy     ours        2       400    301     21448   41.55    22     45.637
1_webp_a.png  lossy     ours        3       400    301     19330   41.88    15     67.785
1_webp_a.png  lossy     ours        4       400    301     19330   41.88    8      130.246
1_webp_a.png  lossy     ours        5       400    301     19330   41.88    6      176.427
1_webp_a.png  lossy     ours        6       400    301     18766   41.80    5      213.078
1_webp_a.png  lossy     ours        7       400    301     18766   41.80    5      211.340
1_webp_a.png  lossy     ours        8       400    301     18610   42.09    4      262.138
1_webp_a.png  lossy     ours        9       400    301     18610   42.09    4      261.023
1_webp_a.png  lossy     wasm        0       400    301     20134   41.94    117    8.617
1_webp_a.png  lossy     wasm        1       400    301     18496   41.98    88     11.490
1_webp_a.png  lossy     wasm        2       400    301     17446   41.40    86     11.688
1_webp_a.png  lossy     wasm        3       400    301     17122   42.23    41     24.977
1_webp_a.png  lossy     wasm        4       400    301     17142   42.25    37     27.301
1_webp_a.png  lossy     wasm        5       400    301     17228   42.22    30     33.727
1_webp_a.png  lossy     wasm        6       400    301     17032   42.23    1      2545.334
2_webp_a.png  lossless  libwebp     0       386    395     57368   -        395    2.537
2_webp_a.png  lossless  libwebp     1       386    395     49780   -        54     18.708
2_webp_a.png  lossless  libwebp     2       386    395     44900   -        31     33.237
2_webp_a.png  lossless  libwebp     3       386    395     42970   -        21     49.699
2_webp_a.png  lossless  libwebp     4       386    395     43076   -        20     50.564
2_webp_a.png  lossless  libwebp     5       386    395     41610   -        9      113.212
2_webp_a.png  lossless  libwebp     6       386    395     41648   -        8      129.546
2_webp_a.png  lossless  libwebp     7       386    395     41610   -        8      133.935
2_webp_a.png  lossless  libwebp     8       386    395     41428   -        4      321.998
2_webp_a.png  lossless  libwebp     9       386    395     40058   -        1      1532.631
2_webp_a.png  lossless  nativewebp  0       386    395     51760   -        49     20.559
2_webp_a.png  lossless  nativewebp  4       386    395     49924   -        29     35.400
2_webp_a.png  lossless  nativewebp  6       386    395     49924   -        29     35.355
2_webp_a.png  lossless  ours        0       386    395     48054   -        184    5.446
2_webp_a.png  lossless  ours        1       386    395     47046   -        152    6.608
2_webp_a.png  lossless  ours        2       386    395     46876   -        59     16.958
2_webp_a.png  lossless  ours        3       386    395     46078   -        48     21.175
2_webp_a.png  lossless  ours        4       386    395     45618   -        40     25.023
2_webp_a.png  lossless  ours        5       386    395     44412   -        30     33.734
2_webp_a.png  lossless  ours        6       386    395     43988   -        24     41.780
2_webp_a.png  lossless  wasm        0       386    395     56180   -        107    9.354
2_webp_a.png  lossless  wasm        1       386    395     44974   -        18     58.408
2_webp_a.png  lossless  wasm        2       386    395     43436   -        14     76.861
2_webp_a.png  lossless  wasm        3       386    395     43056   -        11     94.321
2_webp_a.png  lossless  wasm        4       386    395     41648   -        6      195.658
2_webp_a.png  lossless  wasm        5       386    395     41428   -        3      469.574
2_webp_a.png  lossless  wasm        6       386    395     41428   -        3      433.697
2_webp_a.png  lossy     libwebp     0       386    395     18844   43.02    279    3.585
2_webp_a.png  lossy     libwebp     1       386    395     16172   43.00    224    4.468
2_webp_a.png  lossy     libwebp     2       386    395     15124   42.88    222    4.525
2_webp_a.png  lossy     libwebp     3       386    395     14534   43.45    135    7.435
2_webp_a.png  lossy     libwebp     4       386    395     14046   43.36    55     18.375
2_webp_a.png  lossy     libwebp     5       386    395     14000   43.53    5      211.086
2_webp_a.png  lossy     libwebp     6       386    395     13994   43.38    1      1944.819
2_webp_a.png  lossy     ours        0       386    395     20230   42.42    48     20.876
2_webp_a.png  lossy     ours        1       386    395     19772   42.54    32     31.679
2_webp_a.png  lossy     ours        2       386    395     19606   42.54    21     48.600
2_webp_a.png  lossy     ours        3       386    395     15716   42.82    16     66.627
2_webp_a.png  lossy     ours        4       386    395     15716   42.82    8      138.202
2_webp_a.png  lossy     ours        5       386    395     15484   42.82    7      164.245
2_webp_a.png  lossy     ours        6       386    395     15104   42.77    6      192.332
2_webp_a.png  lossy     ours        7       386    395     15104   42.77    6      194.139
2_webp_a.png  lossy     ours        8       386    395     14732   42.83    5      222.479
2_webp_a.png  lossy     ours        9       386    395     14732   42.83    5      222.375
2_webp_a.png  lossy     wasm        0       386    395     18844   43.02    108    9.317
2_webp_a.png  lossy     wasm        1       386    395     16172   43.00    84     11.926
2_webp_a.png  lossy     wasm        2       386    395     15124   42.88    82     12.277
2_webp_a.png  lossy     wasm        3       386    395     14534   43.45    38     26.791
2_webp_a.png  lossy     wasm        4       386    395     14046   43.36    22     47.219
2_webp_a.png  lossy     wasm        5       386    395     14000   43.53    4      317.961
2_webp_a.png  lossy     wasm        6       386    395     13994   43.38    1      2981.925
3_webp_a.png  lossless  libwebp     0       800    600     266952  -        141    7.124
3_webp_a.png  lossless  libwebp     1       800    600     189276  -        18     56.646
3_webp_a.png  lossless  libwebp     2       800    600     184600  -        12     85.587
3_webp_a.png  lossless  libwebp     3       800    600     181092  -        8      133.843
3_webp_a.png  lossless  libwebp     4       800    600     181208  -        8      136.567
3_webp_a.png  lossless  libwebp     5       800    600     181208  -        8      136.454
3_webp_a.png  lossless  libwebp     6       800    600     181124  -        7      155.011
3_webp_a.png  lossless  libwebp     7       800    600     179208  -        6      184.754
3_webp_a.png  lossless  libwebp     8       800    600     177714  -        3      363.582
3_webp_a.png  lossless  libwebp     9       800    600     174890  -        1      2078.838
3_webp_a.png  lossless  nativewebp  0       800    600     203914  -        16     66.284
3_webp_a.png  lossless  nativewebp  4       800    600     202886  -        14     75.917
3_webp_a.png  lossless  nativewebp  6       800    600     202886  -        14     76.207
3_webp_a.png  lossless  ours        0       800    600     194012  -        60     16.948
3_webp_a.png  lossless  ours        1       800    600     193846  -        48     20.866
3_webp_a.png  lossless  ours        2       800    600     193450  -        20     52.092
3_webp_a.png  lossless  ours        3       800    600     193450  -        16     63.833
3_webp_a.png  lossless  ours        4       800    600     191848  -        13     79.605
3_webp_a.png  lossless  ours        5       800    600     189530  -        10     108.602
3_webp_a.png  lossless  ours        6       800    600     188144  -        8      125.303
3_webp_a.png  lossless  wasm        0       800    600     267190  -        48     21.064
3_webp_a.png  lossless  wasm        1       800    600     186782  -        7      155.369
3_webp_a.png  lossless  wasm        2       800    600     183524  -        6      181.704
3_webp_a.png  lossless  wasm        3       800    600     181124  -        4      260.916
3_webp_a.png  lossless  wasm        4       800    600     181124  -        4      260.690
3_webp_a.png  lossless  wasm        5       800    600     178918  -        2      519.021
3_webp_a.png  lossless  wasm        6       800    600     178918  -        3      488.060
3_webp_a.png  lossy     libwebp     0       800    600     80676   41.56    60     16.895
3_webp_a.png  lossy     libwebp     1       800    600     63372   41.57    26     39.054
3_webp_a.png  lossy     libwebp     2       800    600     58678   41.23    26     39.415
3_webp_a.png  lossy     libwebp     3       800    600     55434   41.60    21     47.891
3_webp_a.png  lossy     libwebp     4       800    600     55128   41.61    18     57.035
3_webp_a.png  lossy     libwebp     5       800    600     53732   41.57    5      200.734
3_webp_a.png  lossy     libwebp     6       800    600     52308   41.61    1      2597.733
3_webp_a.png  lossy     ours        0       800    600     73228   40.82    19     52.636
3_webp_a.png  lossy     ours        1       800    600     72536   40.91    16     65.276
3_webp_a.png  lossy     ours        2       800    600     70934   40.91    8      126.193
3_webp_a.png  lossy     ours        3       800    600     64136   41.12    6      169.631
3_webp_a.png  lossy     ours        4       800    600     64136   41.12    3      360.880
3_webp_a.png  lossy     ours        5       800    600     60956   41.12    3      439.830
3_webp_a.png  lossy     ours        6       800    600     60130   41.31    2      574.659
3_webp_a.png  lossy     ours        7       800    600     60130   41.31    2      575.090
3_webp_a.png  lossy     ours        8       800    600     59312   41.18    2      593.749
3_webp_a.png  lossy     ours        9       800    600     59312   41.18    2      657.319
3_webp_a.png  lossy     wasm        0       800    600     80676   41.56    27     37.653
3_webp_a.png  lossy     wasm        1       800    600     63372   41.57    12     84.357
3_webp_a.png  lossy     wasm        2       800    600     58678   41.23    12     85.378
3_webp_a.png  lossy     wasm        3       800    600     55434   41.60    9      122.271
3_webp_a.png  lossy     wasm        4       800    600     55128   41.61    8      141.782
3_webp_a.png  lossy     wasm        5       800    600     53732   41.57    3      365.488
3_webp_a.png  lossy     wasm        6       800    600     52308   41.61    1      4478.093
4_webp_a.png  lossless  libwebp     0       421    163     47380   -        500    1.165
4_webp_a.png  lossless  libwebp     1       421    163     37608   -        115    8.742
4_webp_a.png  lossless  libwebp     2       421    163     36502   -        64     15.738
4_webp_a.png  lossless  libwebp     3       421    163     35266   -        41     24.459
4_webp_a.png  lossless  libwebp     4       421    163     35248   -        40     25.046
4_webp_a.png  lossless  libwebp     5       421    163     34610   -        19     53.660
4_webp_a.png  lossless  libwebp     6       421    163     34420   -        17     60.661
4_webp_a.png  lossless  libwebp     7       421    163     34440   -        16     63.674
4_webp_a.png  lossless  libwebp     8       421    163     33498   -        8      138.692
4_webp_a.png  lossless  libwebp     9       421    163     33018   -        2      811.759
4_webp_a.png  lossless  nativewebp  0       421    163     39072   -        102    9.896
4_webp_a.png  lossless  nativewebp  4       421    163     37928   -        60     16.691
4_webp_a.png  lossless  nativewebp  6       421    163     37928   -        61     16.654
4_webp_a.png  lossless  ours        0       421    163     38012   -        376    2.664
4_webp_a.png  lossless  ours        1       421    163     37896   -        301    3.328
4_webp_a.png  lossless  ours        2       421    163     37896   -        109    9.176
4_webp_a.png  lossless  ours        3       421    163     37896   -        84     11.914
4_webp_a.png  lossless  ours        4       421    163     37896   -        72     14.057
4_webp_a.png  lossless  ours        5       421    163     37896   -        55     18.247
4_webp_a.png  lossless  ours        6       421    163     38046   -        45     22.428
4_webp_a.png  lossless  wasm        0       421    163     45042   -        344    2.911
4_webp_a.png  lossless  wasm        1       421    163     36876   -        42     24.006
4_webp_a.png  lossless  wasm        2       421    163     36062   -        26     38.581
4_webp_a.png  lossless  wasm        3       421    163     35114   -        21     48.009
4_webp_a.png  lossless  wasm        4       421    163     34420   -        11     93.395
4_webp_a.png  lossless  wasm        5       421    163     33558   -        5      207.965
4_webp_a.png  lossless  wasm        6       421    163     33558   -        6      195.933
4_webp_a.png  lossy     libwebp     0       421    163     23740   39.28    424    2.361
4_webp_a.png  lossy     libwebp     1       421    163     19908   39.23    358    2.795
4_webp_a.png  lossy     libwebp     2       421    163     19524   39.12    349    2.872
4_webp_a.png  lossy     libwebp     3       421    163     18786   39.77    227    4.421
4_webp_a.png  lossy     libwebp     4       421    163     18806   39.76    185    5.419
4_webp_a.png  lossy     libwebp     5       421    163     18830   39.74    180    5.577
4_webp_a.png  lossy     libwebp     6       421    163     18758   39.73    2      648.176
4_webp_a.png  lossy     ours        0       421    163     22468   38.92    81     12.485
4_webp_a.png  lossy     ours        1       421    163     22142   38.94    50     20.154
4_webp_a.png  lossy     ours        2       421    163     22150   38.94    31     32.776
4_webp_a.png  lossy     ours        3       421    163     19774   39.22    21     48.905
4_webp_a.png  lossy     ours        4       421    163     19774   39.22    11     95.296
4_webp_a.png  lossy     ours        5       421    163     19774   39.22    8      135.328
4_webp_a.png  lossy     ours        6       421    163     19380   39.16    6      178.340
4_webp_a.png  lossy     ours        7       421    163     19380   39.16    6      178.030
4_webp_a.png  lossy     ours        8       421    163     19190   39.33    5      207.502
4_webp_a.png  lossy     ours        9       421    163     19190   39.33    5      208.156
4_webp_a.png  lossy     wasm        0       421    163     23740   39.28    181    5.552
4_webp_a.png  lossy     wasm        1       421    163     19908   39.23    139    7.197
4_webp_a.png  lossy     wasm        2       421    163     19524   39.12    134    7.504
4_webp_a.png  lossy     wasm        3       421    163     18786   39.77    65     15.434
4_webp_a.png  lossy     wasm        4       421    163     18806   39.76    59     17.103
4_webp_a.png  lossy     wasm        5       421    163     18830   39.74    56     18.004
4_webp_a.png  lossy     wasm        6       421    163     18758   39.73    1      1071.732
5_webp_a.png  lossless  libwebp     0       300    300     166624  -        433    2.313
5_webp_a.png  lossless  libwebp     1       300    300     157058  -        97     10.392
5_webp_a.png  lossless  libwebp     2       300    300     149780  -        49     20.807
5_webp_a.png  lossless  libwebp     3       300    300     144870  -        27     37.369
5_webp_a.png  lossless  libwebp     4       300    300     144778  -        27     38.414
5_webp_a.png  lossless  libwebp     5       300    300     140030  -        11     95.018
5_webp_a.png  lossless  libwebp     6       300    300     140018  -        10     109.090
5_webp_a.png  lossless  libwebp     7       300    300     140116  -        9      114.295
5_webp_a.png  lossless  libwebp     8       300    300     137608  -        4      255.022
5_webp_a.png  lossless  libwebp     9       300    300     137786  -        1      3034.645
5_webp_a.png  lossless  nativewebp  0       300    300     164800  -        70     14.453
5_webp_a.png  lossless  nativewebp  4       300    300     154970  -        44     23.261
5_webp_a.png  lossless  nativewebp  6       300    300     154970  -        43     23.646
5_webp_a.png  lossless  ours        0       300    300     161478  -        183    5.482
5_webp_a.png  lossless  ours        1       300    300     156970  -        160    6.273
5_webp_a.png  lossless  ours        2       300    300     156970  -        57     17.734
5_webp_a.png  lossless  ours        3       300    300     155772  -        48     20.934
5_webp_a.png  lossless  ours        4       300    300     152988  -        44     23.184
5_webp_a.png  lossless  ours        5       300    300     149932  -        34     29.986
5_webp_a.png  lossless  ours        6       300    300     149708  -        25     40.281
5_webp_a.png  lossless  wasm        0       300    300     166008  -        157    6.399
5_webp_a.png  lossless  wasm        1       300    300     154618  -        24     43.263
5_webp_a.png  lossless  wasm        2       300    300     149320  -        16     62.941
5_webp_a.png  lossless  wasm        3       300    300     144768  -        13     83.012
5_webp_a.png  lossless  wasm        4       300    300     140018  -        6      183.955
5_webp_a.png  lossless  wasm        5       300    300     137538  -        3      408.271
5_webp_a.png  lossless  wasm        6       300    300     137538  -        3      350.099
5_webp_a.png  lossy     libwebp     0       300    300     64290   32.77    202    4.970
5_webp_a.png  lossy     libwebp     1       300    300     63902   32.78    159    6.293
5_webp_a.png  lossy     libwebp     2       300    300     62690   32.49    152    6.602
5_webp_a.png  lossy     libwebp     3       300    300     61738   32.64    104    9.672
5_webp_a.png  lossy     libwebp     4       300    300     58682   32.63    40     25.095
5_webp_a.png  lossy     libwebp     5       300    300     57038   32.64    8      138.977
5_webp_a.png  lossy     libwebp     6       300    300     55926   32.67    1      3191.000
5_webp_a.png  lossy     ours        0       300    300     70196   33.07    42     23.867
5_webp_a.png  lossy     ours        1       300    300     69260   33.09    26     39.622
5_webp_a.png  lossy     ours        2       300    300     68644   33.09    16     64.632
5_webp_a.png  lossy     ours        3       300    300     64750   33.22    11     99.367
5_webp_a.png  lossy     ours        4       300    300     64750   33.22    5      201.245
5_webp_a.png  lossy     ours        5       300    300     60202   33.22    6      172.567
5_webp_a.png  lossy     ours        6       300    300     59746   33.18    5      222.245
5_webp_a.png  lossy     ours        7       300    300     59746   33.18    5      222.462
5_webp_a.png  lossy     ours        8       300    300     59608   33.23    4      270.516
5_webp_a.png  lossy     ours        9       300    300     59608   33.23    4      271.133
5_webp_a.png  lossy     wasm        0       300    300     64290   32.77    96     10.520
5_webp_a.png  lossy     wasm        1       300    300     63902   32.78    74     13.651
5_webp_a.png  lossy     wasm        2       300    300     62690   32.49    69     14.518
5_webp_a.png  lossy     wasm        3       300    300     61738   32.64    34     29.817
5_webp_a.png  lossy     wasm        4       300    300     58682   32.63    17     60.612
5_webp_a.png  lossy     wasm        5       300    300     57038   32.64    5      226.652
5_webp_a.png  lossy     wasm        6       300    300     55926   32.67    1      5434.567
```


## amd64 (AMD Ryzen 7 5700G) / transparent

```
file          mode        engine      width  height  bytes   psnr_db  iters  ms_per_op
1_webp_a.png  lossless    libwebp     400    301     75254   -        21     95.746
1_webp_a.png  lossless    nativewebp  400    301     81934   -        29     70.612
1_webp_a.png  lossless    ours        400    301     79910   -        31     64.902
1_webp_a.png  lossless    wasm        400    301     74350   -        4      610.343
1_webp_a.png  lossy-fast  libwebp     400    301     20134   41.94    470    4.258
1_webp_a.png  lossy-fast  ours        400    301     21748   41.48    69     29.098
1_webp_a.png  lossy-fast  wasm        400    301     20134   41.94    117    17.224
1_webp_a.png  lossy-slow  libwebp     400    301     17032   42.23    2      1222.480
1_webp_a.png  lossy-slow  ours        400    301     18610   42.09    5      412.408
1_webp_a.png  lossy-slow  wasm        400    301     17032   42.23    1      4611.356
2_webp_a.png  lossless    libwebp     386    395     41648   -        21     98.577
2_webp_a.png  lossless    nativewebp  386    395     49924   -        23     87.103
2_webp_a.png  lossless    ours        386    395     43988   -        30     67.174
2_webp_a.png  lossless    wasm        386    395     41428   -        3      705.428
2_webp_a.png  lossy-fast  libwebp     386    395     18844   43.02    480    4.170
2_webp_a.png  lossy-fast  ours        386    395     20230   42.42    61     33.190
2_webp_a.png  lossy-fast  wasm        386    395     18844   43.02    103    19.482
2_webp_a.png  lossy-slow  libwebp     386    395     13994   43.38    2      1049.845
2_webp_a.png  lossy-slow  ours        386    395     14732   42.83    6      363.698
2_webp_a.png  lossy-slow  wasm        386    395     13994   43.38    1      5134.569
3_webp_a.png  lossless    libwebp     800    600     181124  -        13     155.583
3_webp_a.png  lossless    nativewebp  800    600     202886  -        13     158.851
3_webp_a.png  lossless    ours        800    600     188144  -        10     203.463
3_webp_a.png  lossless    wasm        800    600     178918  -        3      798.334
3_webp_a.png  lossy-fast  libwebp     800    600     80676   41.56    107    18.843
3_webp_a.png  lossy-fast  ours        800    600     73228   40.82    24     85.320
3_webp_a.png  lossy-fast  wasm        800    600     80676   41.56    29     69.034
3_webp_a.png  lossy-slow  libwebp     800    600     52308   41.61    1      2045.516
3_webp_a.png  lossy-slow  ours        800    600     59116   41.18    2      1093.428
3_webp_a.png  lossy-slow  wasm        800    600     52308   41.61    1      8149.857
4_webp_a.png  lossless    libwebp     421    163     34420   -        41     49.703
4_webp_a.png  lossless    nativewebp  421    163     37928   -        48     42.286
4_webp_a.png  lossless    ours        421    163     38046   -        53     37.894
4_webp_a.png  lossless    wasm        421    163     33558   -        7      323.164
4_webp_a.png  lossy-fast  libwebp     421    163     23740   39.28    500    2.846
4_webp_a.png  lossy-fast  ours        421    163     22468   38.92    105    19.140
4_webp_a.png  lossy-fast  wasm        421    163     23740   39.28    169    11.880
4_webp_a.png  lossy-slow  libwebp     421    163     18758   39.73    5      493.728
4_webp_a.png  lossy-slow  ours        421    163     19190   39.33    7      328.547
4_webp_a.png  lossy-slow  wasm        421    163     18758   39.73    1      2071.994
5_webp_a.png  lossless    libwebp     300    300     140018  -        18     113.313
5_webp_a.png  lossless    nativewebp  300    300     154970  -        36     55.626
5_webp_a.png  lossless    ours        300    300     149708  -        31     65.549
5_webp_a.png  lossless    wasm        300    300     137538  -        4      580.180
5_webp_a.png  lossy-fast  libwebp     300    300     64290   32.77    340    5.893
5_webp_a.png  lossy-fast  ours        300    300     70196   33.07    57     35.089
5_webp_a.png  lossy-fast  wasm        300    300     64290   32.77    103    19.432
5_webp_a.png  lossy-slow  libwebp     300    300     55926   32.67    1      2468.038
5_webp_a.png  lossy-slow  ours        300    300     59610   33.23    6      391.773
5_webp_a.png  lossy-slow  wasm        300    300     55926   32.67    1      10972.937
```

Peak RSS, one encode per process:

```
file          mode        engine      width  height  megapixels  peak_rss_mib  mib_per_mp
1_webp_a.png  lossless    libwebp     400    301     0.12        34.3          284.6
1_webp_a.png  lossless    nativewebp  400    301     0.12        19.8          164.3
1_webp_a.png  lossless    ours        400    301     0.12        28.4          235.6
1_webp_a.png  lossless    wasm        400    301     0.12        59.4          493.2
1_webp_a.png  lossy-fast  libwebp     400    301     0.12        15.4          128.2
1_webp_a.png  lossy-fast  ours        400    301     0.12        24.1          200.5
1_webp_a.png  lossy-fast  wasm        400    301     0.12        29.8          247.9
1_webp_a.png  lossy-slow  libwebp     400    301     0.12        46.2          383.9
1_webp_a.png  lossy-slow  ours        400    301     0.12        33.4          277.2
1_webp_a.png  lossy-slow  wasm        400    301     0.12        104.1         864.7
2_webp_a.png  lossless    libwebp     386    395     0.15        43.2          283.0
2_webp_a.png  lossless    nativewebp  386    395     0.15        19.8          129.5
2_webp_a.png  lossless    ours        386    395     0.15        34.1          223.7
2_webp_a.png  lossless    wasm        386    395     0.15        99.4          652.2
2_webp_a.png  lossy-fast  libwebp     386    395     0.15        16.0          105.2
2_webp_a.png  lossy-fast  ours        386    395     0.15        26.1          171.1
2_webp_a.png  lossy-fast  wasm        386    395     0.15        37.4          245.4
2_webp_a.png  lossy-slow  libwebp     386    395     0.15        54.1          355.0
2_webp_a.png  lossy-slow  ours        386    395     0.15        38.1          250.2
2_webp_a.png  lossy-slow  wasm        386    395     0.15        130.0         852.6
3_webp_a.png  lossless    libwebp     800    600     0.48        40.1          83.6
3_webp_a.png  lossless    nativewebp  800    600     0.48        30.3          63.1
3_webp_a.png  lossless    ours        800    600     0.48        68.4          142.6
3_webp_a.png  lossless    wasm        800    600     0.48        107.6         224.1
3_webp_a.png  lossy-fast  libwebp     800    600     0.48        27.4          57.2
3_webp_a.png  lossy-fast  ours        800    600     0.48        54.2          112.9
3_webp_a.png  lossy-fast  wasm        800    600     0.48        65.9          137.3
3_webp_a.png  lossy-slow  libwebp     800    600     0.48        57.1          119.0
3_webp_a.png  lossy-slow  ours        800    600     0.48        106.5         221.8
3_webp_a.png  lossy-slow  wasm        800    600     0.48        228.2         475.4
4_webp_a.png  lossless    libwebp     421    163     0.07        25.7          373.9
4_webp_a.png  lossless    nativewebp  421    163     0.07        15.9          231.2
4_webp_a.png  lossless    ours        421    163     0.07        22.1          322.5
4_webp_a.png  lossless    wasm        421    163     0.07        39.4          573.7
4_webp_a.png  lossy-fast  libwebp     421    163     0.07        12.1          176.4
4_webp_a.png  lossy-fast  ours        421    163     0.07        17.9          261.1
4_webp_a.png  lossy-fast  wasm        421    163     0.07        23.8          346.3
4_webp_a.png  lossy-slow  libwebp     421    163     0.07        30.4          443.1
4_webp_a.png  lossy-slow  ours        421    163     0.07        26.5          385.7
4_webp_a.png  lossy-slow  wasm        421    163     0.07        86.1          1255.0
5_webp_a.png  lossless    libwebp     300    300     0.09        25.1          279.2
5_webp_a.png  lossless    nativewebp  300    300     0.09        19.8          220.4
5_webp_a.png  lossless    ours        300    300     0.09        25.9          287.8
5_webp_a.png  lossless    wasm        300    300     0.09        49.5          550.0
5_webp_a.png  lossy-fast  libwebp     300    300     0.09        12.6          139.5
5_webp_a.png  lossy-fast  ours        300    300     0.09        22.1          246.0
5_webp_a.png  lossy-fast  wasm        300    300     0.09        25.6          284.7
5_webp_a.png  lossy-slow  libwebp     300    300     0.09        38.7          429.7
5_webp_a.png  lossy-slow  ours        300    300     0.09        26.2          291.2
5_webp_a.png  lossy-slow  wasm        300    300     0.09        108.0         1200.4
```

Decode, one file per mode encoded by libwebp:

```
file          mode      engine   width  height  bytes   psnr_db  iters  ms_per_op
1_webp_a.png  lossless  libwebp  400    301     75254   -        500    1.213
1_webp_a.png  lossless  ours     400    301     75254   -        500    2.503
1_webp_a.png  lossless  wasm     400    301     75254   29.40    358    5.588
1_webp_a.png  lossless  x/image  400    301     75254   -        500    2.923
1_webp_a.png  lossy     libwebp  400    301     17032   -        500    1.222
1_webp_a.png  lossy     ours     400    301     17032   -        500    3.111
1_webp_a.png  lossy     wasm     400    301     17032   29.63    500    3.968
1_webp_a.png  lossy     x/image  400    301     17032   29.63    500    3.844
2_webp_a.png  lossless  libwebp  386    395     41648   -        500    0.990
2_webp_a.png  lossless  ours     386    395     41648   -        500    2.396
2_webp_a.png  lossless  wasm     386    395     41648   25.35    326    6.143
2_webp_a.png  lossless  x/image  386    395     41648   -        500    2.594
2_webp_a.png  lossy     libwebp  386    395     13994   -        500    1.447
2_webp_a.png  lossy     ours     386    395     13994   -        500    3.724
2_webp_a.png  lossy     wasm     386    395     13994   25.46    376    5.326
2_webp_a.png  lossy     x/image  386    395     13994   25.46    416    4.814
3_webp_a.png  lossless  libwebp  800    600     181124  -        500    2.924
3_webp_a.png  lossless  ours     800    600     181124  -        252    7.960
3_webp_a.png  lossless  wasm     800    600     181124  27.66    112    17.990
3_webp_a.png  lossless  x/image  800    600     181124  -        246    8.131
3_webp_a.png  lossy     libwebp  800    600     52308   -        500    3.671
3_webp_a.png  lossy     ours     800    600     52308   -        203    9.892
3_webp_a.png  lossy     wasm     800    600     52308   27.81    142    14.092
3_webp_a.png  lossy     x/image  800    600     52308   27.81    149    13.477
4_webp_a.png  lossless  libwebp  421    163     34420   -        500    0.542
4_webp_a.png  lossless  ours     421    163     34420   -        500    1.281
4_webp_a.png  lossless  wasm     421    163     34420   30.27    500    3.114
4_webp_a.png  lossless  x/image  421    163     34420   -        500    1.530
4_webp_a.png  lossy     libwebp  421    163     18758   -        500    0.948
4_webp_a.png  lossy     ours     421    163     18758   -        500    2.313
4_webp_a.png  lossy     wasm     421    163     18758   31.62    500    2.984
4_webp_a.png  lossy     x/image  421    163     18758   31.62    500    2.760
5_webp_a.png  lossless  libwebp  300    300     140018  -        500    1.541
5_webp_a.png  lossless  ours     300    300     140018  -        500    2.951
5_webp_a.png  lossless  wasm     300    300     140018  24.63    375    5.341
5_webp_a.png  lossless  x/image  300    300     140018  -        500    3.755
5_webp_a.png  lossy     libwebp  300    300     55926   -        500    2.366
5_webp_a.png  lossy     ours     300    300     55926   -        395    5.067
5_webp_a.png  lossy     wasm     300    300     55926   25.61    340    5.893
5_webp_a.png  lossy     x/image  300    300     55926   25.61    350    5.728
```

Effort sweep, every setting of every engine:

```
file          mode      engine      effort  width  height  bytes   psnr_db  iters  ms_per_op
1_webp_a.png  lossless  libwebp     0       400    301     89358   -        422    2.372
1_webp_a.png  lossless  libwebp     1       400    301     80522   -        73     13.874
1_webp_a.png  lossless  libwebp     2       400    301     77132   -        46     22.104
1_webp_a.png  lossless  libwebp     3       400    301     77242   -        28     35.876
1_webp_a.png  lossless  libwebp     4       400    301     76798   -        28     36.766
1_webp_a.png  lossless  libwebp     5       400    301     75278   -        12     84.614
1_webp_a.png  lossless  libwebp     6       400    301     75254   -        11     94.552
1_webp_a.png  lossless  libwebp     7       400    301     75162   -        11     97.607
1_webp_a.png  lossless  libwebp     8       400    301     73858   -        5      228.675
1_webp_a.png  lossless  libwebp     9       400    301     71932   -        1      1778.166
1_webp_a.png  lossless  nativewebp  0       400    301     83958   -        31     33.287
1_webp_a.png  lossless  nativewebp  4       400    301     81934   -        15     71.384
1_webp_a.png  lossless  nativewebp  6       400    301     81934   -        15     71.268
1_webp_a.png  lossless  ours        0       400    301     83468   -        117    8.555
1_webp_a.png  lossless  ours        1       400    301     82496   -        100    10.091
1_webp_a.png  lossless  ours        2       400    301     82476   -        36     28.559
1_webp_a.png  lossless  ours        3       400    301     82476   -        30     33.884
1_webp_a.png  lossless  ours        4       400    301     81832   -        25     40.099
1_webp_a.png  lossless  ours        5       400    301     80522   -        18     55.890
1_webp_a.png  lossless  ours        6       400    301     79910   -        16     66.403
1_webp_a.png  lossless  wasm        0       400    301     88930   -        85     11.807
1_webp_a.png  lossless  wasm        1       400    301     78950   -        15     71.067
1_webp_a.png  lossless  wasm        2       400    301     77160   -        11     91.586
1_webp_a.png  lossless  wasm        3       400    301     76576   -        8      128.032
1_webp_a.png  lossless  wasm        4       400    301     75254   -        4      283.012
1_webp_a.png  lossless  wasm        5       400    301     73842   -        2      660.164
1_webp_a.png  lossless  wasm        6       400    301     74350   -        2      611.177
1_webp_a.png  lossy     libwebp     0       400    301     20134   41.94    238    4.216
1_webp_a.png  lossy     libwebp     1       400    301     18496   41.98    193    5.194
1_webp_a.png  lossy     libwebp     2       400    301     17446   41.40    190    5.286
1_webp_a.png  lossy     libwebp     3       400    301     17122   42.23    113    8.866
1_webp_a.png  lossy     libwebp     4       400    301     17142   42.25    95     10.528
1_webp_a.png  lossy     libwebp     5       400    301     17228   42.22    73     13.704
1_webp_a.png  lossy     libwebp     6       400    301     17032   42.23    1      1206.627
1_webp_a.png  lossy     ours        0       400    301     21748   41.48    35     28.772
1_webp_a.png  lossy     ours        1       400    301     21436   41.55    24     43.016
1_webp_a.png  lossy     ours        2       400    301     21448   41.55    15     71.031
1_webp_a.png  lossy     ours        3       400    301     19330   41.88    10     103.205
1_webp_a.png  lossy     ours        4       400    301     19330   41.88    5      209.971
1_webp_a.png  lossy     ours        5       400    301     19330   41.88    4      283.551
1_webp_a.png  lossy     ours        6       400    301     18766   41.80    3      356.004
1_webp_a.png  lossy     ours        7       400    301     18766   41.80    3      350.534
1_webp_a.png  lossy     ours        8       400    301     18610   42.09    3      411.663
1_webp_a.png  lossy     ours        9       400    301     18610   42.09    3      413.189
1_webp_a.png  lossy     wasm        0       400    301     20134   41.94    56     18.097
1_webp_a.png  lossy     wasm        1       400    301     18496   41.98    44     23.024
1_webp_a.png  lossy     wasm        2       400    301     17446   41.40    43     23.596
1_webp_a.png  lossy     wasm        3       400    301     17122   42.23    22     45.814
1_webp_a.png  lossy     wasm        4       400    301     17142   42.25    21     48.937
1_webp_a.png  lossy     wasm        5       400    301     17228   42.22    16     62.570
1_webp_a.png  lossy     wasm        6       400    301     17032   42.23    1      4561.053
2_webp_a.png  lossless  libwebp     0       386    395     57368   -        399    2.508
2_webp_a.png  lossless  libwebp     1       386    395     49780   -        54     18.752
2_webp_a.png  lossless  libwebp     2       386    395     44900   -        35     28.832
2_webp_a.png  lossless  libwebp     3       386    395     42970   -        24     42.088
2_webp_a.png  lossless  libwebp     4       386    395     43076   -        24     43.075
2_webp_a.png  lossless  libwebp     5       386    395     41610   -        12     87.130
2_webp_a.png  lossless  libwebp     6       386    395     41648   -        11     97.392
2_webp_a.png  lossless  libwebp     7       386    395     41610   -        10     101.009
2_webp_a.png  lossless  libwebp     8       386    395     41428   -        5      227.258
2_webp_a.png  lossless  libwebp     9       386    395     40058   -        1      1343.524
2_webp_a.png  lossless  nativewebp  0       386    395     51760   -        25     40.215
2_webp_a.png  lossless  nativewebp  4       386    395     49924   -        12     87.913
2_webp_a.png  lossless  nativewebp  6       386    395     49924   -        12     88.003
2_webp_a.png  lossless  ours        0       386    395     48054   -        108    9.298
2_webp_a.png  lossless  ours        1       386    395     47046   -        91     11.076
2_webp_a.png  lossless  ours        2       386    395     46876   -        36     28.162
2_webp_a.png  lossless  ours        3       386    395     46078   -        29     35.376
2_webp_a.png  lossless  ours        4       386    395     45618   -        24     42.197
2_webp_a.png  lossless  ours        5       386    395     44412   -        18     56.684
2_webp_a.png  lossless  ours        6       386    395     43988   -        15     69.021
2_webp_a.png  lossless  wasm        0       386    395     56180   -        66     15.319
2_webp_a.png  lossless  wasm        1       386    395     44974   -        11     97.707
2_webp_a.png  lossless  wasm        2       386    395     43436   -        8      133.007
2_webp_a.png  lossless  wasm        3       386    395     43056   -        7      162.999
2_webp_a.png  lossless  wasm        4       386    395     41648   -        3      335.844
2_webp_a.png  lossless  wasm        5       386    395     41428   -        2      780.056
2_webp_a.png  lossless  wasm        6       386    395     41428   -        2      706.787
2_webp_a.png  lossy     libwebp     0       386    395     18844   43.02    240    4.167
2_webp_a.png  lossy     libwebp     1       386    395     16172   43.00    192    5.230
2_webp_a.png  lossy     libwebp     2       386    395     15124   42.88    192    5.224
2_webp_a.png  lossy     libwebp     3       386    395     14534   43.45    107    9.369
2_webp_a.png  lossy     libwebp     4       386    395     14046   43.36    53     19.179
2_webp_a.png  lossy     libwebp     5       386    395     14000   43.53    12     89.112
2_webp_a.png  lossy     libwebp     6       386    395     13994   43.38    1      1058.614
2_webp_a.png  lossy     ours        0       386    395     20230   42.42    31     33.099
2_webp_a.png  lossy     ours        1       386    395     19772   42.54    22     46.360
2_webp_a.png  lossy     ours        2       386    395     19606   42.54    13     78.786
2_webp_a.png  lossy     ours        3       386    395     15772   42.82    10     106.016
2_webp_a.png  lossy     ours        4       386    395     15772   42.82    5      233.310
2_webp_a.png  lossy     ours        5       386    395     15484   42.82    4      272.864
2_webp_a.png  lossy     ours        6       386    395     15104   42.77    4      324.364
2_webp_a.png  lossy     ours        7       386    395     15104   42.77    4      326.410
2_webp_a.png  lossy     ours        8       386    395     14732   42.83    3      365.985
2_webp_a.png  lossy     ours        9       386    395     14732   42.83    3      364.003
2_webp_a.png  lossy     wasm        0       386    395     18844   43.02    51     19.922
2_webp_a.png  lossy     wasm        1       386    395     16172   43.00    40     25.346
2_webp_a.png  lossy     wasm        2       386    395     15124   42.88    40     25.315
2_webp_a.png  lossy     wasm        3       386    395     14534   43.45    20     50.672
2_webp_a.png  lossy     wasm        4       386    395     14046   43.36    12     88.724
2_webp_a.png  lossy     wasm        5       386    395     14000   43.53    2      524.357
2_webp_a.png  lossy     wasm        6       386    395     13994   43.38    1      5138.524
3_webp_a.png  lossless  libwebp     0       800    600     266952  -        158    6.344
3_webp_a.png  lossless  libwebp     1       800    600     189276  -        20     50.576
3_webp_a.png  lossless  libwebp     2       800    600     184600  -        13     77.078
3_webp_a.png  lossless  libwebp     3       800    600     181092  -        8      133.372
3_webp_a.png  lossless  libwebp     4       800    600     181208  -        8      142.332
3_webp_a.png  lossless  libwebp     5       800    600     181208  -        8      137.212
3_webp_a.png  lossless  libwebp     6       800    600     181124  -        7      157.113
3_webp_a.png  lossless  libwebp     7       800    600     179208  -        6      183.696
3_webp_a.png  lossless  libwebp     8       800    600     177714  -        3      349.651
3_webp_a.png  lossless  libwebp     9       800    600     174890  -        1      2178.061
3_webp_a.png  lossless  nativewebp  0       800    600     203914  -        8      126.350
3_webp_a.png  lossless  nativewebp  4       800    600     202886  -        7      161.894
3_webp_a.png  lossless  nativewebp  6       800    600     202886  -        7      161.225
3_webp_a.png  lossless  ours        0       800    600     194012  -        36     27.890
3_webp_a.png  lossless  ours        1       800    600     193846  -        31     32.940
3_webp_a.png  lossless  ours        2       800    600     193450  -        12     85.802
3_webp_a.png  lossless  ours        3       800    600     193450  -        10     103.759
3_webp_a.png  lossless  ours        4       800    600     191848  -        8      129.283
3_webp_a.png  lossless  ours        5       800    600     189530  -        6      174.125
3_webp_a.png  lossless  ours        6       800    600     188144  -        5      203.500
3_webp_a.png  lossless  wasm        0       800    600     267190  -        30     34.369
3_webp_a.png  lossless  wasm        1       800    600     186782  -        4      250.339
3_webp_a.png  lossless  wasm        2       800    600     183524  -        4      301.452
3_webp_a.png  lossless  wasm        3       800    600     181124  -        3      424.709
3_webp_a.png  lossless  wasm        4       800    600     181124  -        3      426.388
3_webp_a.png  lossless  wasm        5       800    600     178918  -        2      854.657
3_webp_a.png  lossless  wasm        6       800    600     178918  -        2      797.528
3_webp_a.png  lossy     libwebp     0       800    600     80676   41.56    53     18.970
3_webp_a.png  lossy     libwebp     1       800    600     63372   41.57    31     32.418
3_webp_a.png  lossy     libwebp     2       800    600     58678   41.23    31     32.530
3_webp_a.png  lossy     libwebp     3       800    600     55434   41.60    23     43.532
3_webp_a.png  lossy     libwebp     4       800    600     55128   41.61    19     53.753
3_webp_a.png  lossy     libwebp     5       800    600     53732   41.57    8      136.569
3_webp_a.png  lossy     libwebp     6       800    600     52308   41.61    1      2053.163
3_webp_a.png  lossy     ours        0       800    600     73228   40.82    12     85.937
3_webp_a.png  lossy     ours        1       800    600     72536   40.91    10     102.913
3_webp_a.png  lossy     ours        2       800    600     71036   40.91    5      211.280
3_webp_a.png  lossy     ours        3       800    600     64238   41.12    4      291.370
3_webp_a.png  lossy     ours        4       800    600     64238   41.12    2      612.129
3_webp_a.png  lossy     ours        5       800    600     60602   41.12    2      769.993
3_webp_a.png  lossy     ours        6       800    600     59934   41.31    1      1000.002
3_webp_a.png  lossy     ours        7       800    600     59934   41.31    2      998.901
3_webp_a.png  lossy     ours        8       800    600     59116   41.18    1      1042.493
3_webp_a.png  lossy     ours        9       800    600     59116   41.18    1      1118.799
3_webp_a.png  lossy     wasm        0       800    600     80676   41.56    15     69.235
3_webp_a.png  lossy     wasm        1       800    600     63372   41.57    7      148.363
3_webp_a.png  lossy     wasm        2       800    600     58678   41.23    7      146.164
3_webp_a.png  lossy     wasm        3       800    600     55434   41.60    5      210.809
3_webp_a.png  lossy     wasm        4       800    600     55128   41.61    5      248.294
3_webp_a.png  lossy     wasm        5       800    600     53732   41.57    2      637.245
3_webp_a.png  lossy     wasm        6       800    600     52308   41.61    1      8233.498
4_webp_a.png  lossless  libwebp     0       421    163     47380   -        500    1.143
4_webp_a.png  lossless  libwebp     1       421    163     37608   -        125    8.058
4_webp_a.png  lossless  libwebp     2       421    163     36502   -        74     13.554
4_webp_a.png  lossless  libwebp     3       421    163     35266   -        46     21.961
4_webp_a.png  lossless  libwebp     4       421    163     35248   -        45     22.522
4_webp_a.png  lossless  libwebp     5       421    163     34610   -        22     45.535
4_webp_a.png  lossless  libwebp     6       421    163     34420   -        20     50.150
4_webp_a.png  lossless  libwebp     7       421    163     34440   -        19     53.764
4_webp_a.png  lossless  libwebp     8       421    163     33498   -        9      118.082
4_webp_a.png  lossless  libwebp     9       421    163     33018   -        2      757.230
4_webp_a.png  lossless  nativewebp  0       421    163     39072   -        49     20.410
4_webp_a.png  lossless  nativewebp  4       421    163     37928   -        24     42.660
4_webp_a.png  lossless  nativewebp  6       421    163     37928   -        24     42.397
4_webp_a.png  lossless  ours        0       421    163     38012   -        208    4.832
4_webp_a.png  lossless  ours        1       421    163     37896   -        173    5.796
4_webp_a.png  lossless  ours        2       421    163     37896   -        64     15.647
4_webp_a.png  lossless  ours        3       421    163     37896   -        50     20.158
4_webp_a.png  lossless  ours        4       421    163     37896   -        42     23.935
4_webp_a.png  lossless  ours        5       421    163     37896   -        33     30.592
4_webp_a.png  lossless  ours        6       421    163     38046   -        27     37.801
4_webp_a.png  lossless  wasm        0       421    163     45042   -        192    5.227
4_webp_a.png  lossless  wasm        1       421    163     36876   -        26     39.168
4_webp_a.png  lossless  wasm        2       421    163     36062   -        16     65.495
4_webp_a.png  lossless  wasm        3       421    163     35114   -        12     83.925
4_webp_a.png  lossless  wasm        4       421    163     34420   -        7      160.008
4_webp_a.png  lossless  wasm        5       421    163     33558   -        3      348.820
4_webp_a.png  lossless  wasm        6       421    163     33558   -        4      321.545
4_webp_a.png  lossy     libwebp     0       421    163     23740   39.28    351    2.849
4_webp_a.png  lossy     libwebp     1       421    163     19908   39.23    292    3.431
4_webp_a.png  lossy     libwebp     2       421    163     19524   39.12    290    3.456
4_webp_a.png  lossy     libwebp     3       421    163     18786   39.77    179    5.593
4_webp_a.png  lossy     libwebp     4       421    163     18806   39.76    152    6.607
4_webp_a.png  lossy     libwebp     5       421    163     18830   39.74    147    6.826
4_webp_a.png  lossy     libwebp     6       421    163     18758   39.73    3      489.256
4_webp_a.png  lossy     ours        0       421    163     22468   38.92    54     18.871
4_webp_a.png  lossy     ours        1       421    163     22142   38.94    35     28.923
4_webp_a.png  lossy     ours        2       421    163     22150   38.94    20     50.493
4_webp_a.png  lossy     ours        3       421    163     19774   39.22    14     75.614
4_webp_a.png  lossy     ours        4       421    163     19774   39.22    7      152.060
4_webp_a.png  lossy     ours        5       421    163     19774   39.22    5      223.773
4_webp_a.png  lossy     ours        6       421    163     19380   39.16    4      290.796
4_webp_a.png  lossy     ours        7       421    163     19380   39.16    4      292.050
4_webp_a.png  lossy     ours        8       421    163     19190   39.33    4      328.182
4_webp_a.png  lossy     ours        9       421    163     19190   39.33    4      329.139
4_webp_a.png  lossy     wasm        0       421    163     23740   39.28    83     12.105
4_webp_a.png  lossy     wasm        1       421    163     19908   39.23    70     14.492
4_webp_a.png  lossy     wasm        2       421    163     19524   39.12    70     14.355
4_webp_a.png  lossy     wasm        3       421    163     18786   39.77    36     28.173
4_webp_a.png  lossy     wasm        4       421    163     18806   39.76    33     31.030
4_webp_a.png  lossy     wasm        5       421    163     18830   39.74    30     34.406
4_webp_a.png  lossy     wasm        6       421    163     18758   39.73    1      2040.686
5_webp_a.png  lossless  libwebp     0       300    300     166624  -        434    2.306
5_webp_a.png  lossless  libwebp     1       300    300     157058  -        88     11.396
5_webp_a.png  lossless  libwebp     2       300    300     149780  -        45     22.268
5_webp_a.png  lossless  libwebp     3       300    300     144870  -        25     40.308
5_webp_a.png  lossless  libwebp     4       300    300     144778  -        25     41.561
5_webp_a.png  lossless  libwebp     5       300    300     140030  -        10     101.212
5_webp_a.png  lossless  libwebp     6       300    300     140018  -        9      112.604
5_webp_a.png  lossless  libwebp     7       300    300     140116  -        9      117.515
5_webp_a.png  lossless  libwebp     8       300    300     137608  -        4      256.762
5_webp_a.png  lossless  libwebp     9       300    300     137786  -        1      3016.833
5_webp_a.png  lossless  nativewebp  0       300    300     164800  -        37     27.173
5_webp_a.png  lossless  nativewebp  4       300    300     154970  -        19     55.308
5_webp_a.png  lossless  nativewebp  6       300    300     154970  -        19     55.092
5_webp_a.png  lossless  ours        0       300    300     161478  -        110    9.095
5_webp_a.png  lossless  ours        1       300    300     156970  -        94     10.638
5_webp_a.png  lossless  ours        2       300    300     156970  -        36     28.452
5_webp_a.png  lossless  ours        3       300    300     155772  -        29     34.519
5_webp_a.png  lossless  ours        4       300    300     152988  -        26     38.743
5_webp_a.png  lossless  ours        5       300    300     149932  -        20     50.579
5_webp_a.png  lossless  ours        6       300    300     149708  -        16     65.874
5_webp_a.png  lossless  wasm        0       300    300     166008  -        80     12.533
5_webp_a.png  lossless  wasm        1       300    300     154618  -        14     72.857
5_webp_a.png  lossless  wasm        2       300    300     149320  -        9      112.559
5_webp_a.png  lossless  wasm        3       300    300     144768  -        8      141.071
5_webp_a.png  lossless  wasm        4       300    300     140018  -        4      313.898
5_webp_a.png  lossless  wasm        5       300    300     137538  -        2      703.852
5_webp_a.png  lossless  wasm        6       300    300     137538  -        2      580.312
5_webp_a.png  lossy     libwebp     0       300    300     64290   32.77    171    5.878
5_webp_a.png  lossy     libwebp     1       300    300     63902   32.78    135    7.435
5_webp_a.png  lossy     libwebp     2       300    300     62690   32.49    129    7.809
5_webp_a.png  lossy     libwebp     3       300    300     61738   32.64    87     11.527
5_webp_a.png  lossy     libwebp     4       300    300     58682   32.63    40     25.281
5_webp_a.png  lossy     libwebp     5       300    300     57038   32.64    13     81.494
5_webp_a.png  lossy     libwebp     6       300    300     55926   32.67    1      2479.815
5_webp_a.png  lossy     ours        0       300    300     70196   33.07    29     34.730
5_webp_a.png  lossy     ours        1       300    300     69260   33.09    19     53.270
5_webp_a.png  lossy     ours        2       300    300     68560   33.09    11     96.242
5_webp_a.png  lossy     ours        3       300    300     64666   33.22    7      151.266
5_webp_a.png  lossy     ours        4       300    300     64666   33.22    4      316.338
5_webp_a.png  lossy     ours        5       300    300     60232   33.22    4      263.882
5_webp_a.png  lossy     ours        6       300    300     59748   33.18    3      339.108
5_webp_a.png  lossy     ours        7       300    300     59748   33.18    3      338.738
5_webp_a.png  lossy     ours        8       300    300     59610   33.23    3      393.415
5_webp_a.png  lossy     ours        9       300    300     59610   33.23    3      402.243
5_webp_a.png  lossy     wasm        0       300    300     64290   32.77    54     18.861
5_webp_a.png  lossy     wasm        1       300    300     63902   32.78    40     25.022
5_webp_a.png  lossy     wasm        2       300    300     62690   32.49    39     25.836
5_webp_a.png  lossy     wasm        3       300    300     61738   32.64    20     51.195
5_webp_a.png  lossy     wasm        4       300    300     58682   32.63    10     104.809
5_webp_a.png  lossy     wasm        5       300    300     57038   32.64    3      365.498
5_webp_a.png  lossy     wasm        6       300    300     55926   32.67    1      10947.980
```
