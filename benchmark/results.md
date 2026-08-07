# Benchmark results

Captured runs of `benchmark/run.sh`, `benchmark/run-sweep.sh` and
`benchmark/run-decode.sh` on two machines. Timings are machine-dependent; regenerate locally for your own
hardware.

| | arm64 | amd64 |
| --- | --- | --- |
| **CPU** | Apple M4 Pro (14 cores) | AMD Ryzen 7 5700G (16 threads) |
| **OS** | macOS 26.5.1 | Arch Linux, kernel 7.1.3 |
| **Go** | go1.26.5 | go1.26.5 |

- **libwebp:** 1.6.0 on both (also used by the `wasm` engine, via WASM)
- **Encoding:** library commit 0009929 · 2026-07-26
- **nativewebp rows:** v1.3.0 · library commit 6e35fbe · 2026-08-07
- **Effort sweep:** library commit a622eb2 · 2026-08-07
- **Decoding:** library commit bcab3e6 · 2026-08-07
- **Budget:** 2000 ms/measurement, lossy quality 90

Engines: `ours` (pure Go), `libwebp` (C, cgo), `wasm` (libwebp via WASM, cgo-free),
`nativewebp` (HugoSmits86/nativewebp, pure Go, lossless only), and in the decode
tables `x/image` (golang.org/x/image/webp, decode only). See `README.md` for the
exact mode settings.

The two passes were captured on different dates, so the encode and decode tables
are not one run of one commit. Encoder output is unchanged between those two
commits (verified by hashing every mode of every test image).

`psnr_db` decodes each encoder's own output and scores it against the pixels that
encoder was handed, over RGB. Higher is better, and quality 90 lands near 40 dB.
Lossless is exact, so it shows `-`. Size without quality is not comparable: an
encoder can always make a smaller file by quantizing harder, so the two columns
have to be read together.

Encoder output is identical on both machines (every size in the two tables
matches), so the `psnr_db` column reads the same in both and only the timings
differ.

Each machine also has a peak-RSS table. Those measurements run one encode per
process and report that process's own `ru_maxrss`, so the figure is what an
application pays to encode one image: source bitmap, language runtime and
encoder together. `mib_per_mp` divides it by the image's megapixels so images of
different sizes are comparable; a 1080p frame is 2.1 MP, a 4K frame roughly
10 MP, and a phone photo about 12 MP, so multiply that column by those to size
a workload.

## Reading these numbers

- **vs libwebp, `lossy-slow`:** sizes land at 0.94-1.11x libwebp's, at 0.0-0.8 dB
  lower quality, for 1.3-2.7x the encode time.
- **vs libwebp, `lossy-fast`:** quality matches within ±0.5 dB, sizes run
  0.96-1.21x, and we take 1.1-1.8x the time. Effort 0 is still where our files
  are furthest behind on size.
- **vs libwebp, lossless:** we are 3-9% bigger and 1.1-3.0x slower.
- **vs `nativewebp`, lossless:** the other pure-Go encoder makes files 8-23%
  bigger than ours (11-33% bigger than libwebp's) and encodes them in 0.41-0.71x
  our time on arm64, 0.47-0.94x on amd64. It is the size-for-speed end of the
  pure-Go range and we are the size end: on abubakar it writes 3926412 B in
  2203 ms against our 3355646 B in 4261 ms. It has no lossy mode, so the choice
  only exists for VP8L.
- **`nativewebp` is fast because it does not search.** Its compression level
  picks histogram and transform block sizes and nothing else: the cross-color
  transform is never applied, the palette transform runs only when the input is
  already an `*image.Paletted`, the color cache size is fixed, and no candidate
  encode is ever tried and discarded. libwebp at level 6 searches over palette,
  cross-color, entropy configurations and cache sizes and keeps the smallest
  result, which is both where its time goes and why its files are smaller. On
  arm64 that leaves `nativewebp` ahead of even the C reference on the geometric
  mean (969 ms against 1.2 s); on amd64, where libwebp's SIMD lands better, it
  does not (1.6 s against 1.2 s).
- **`libwebp` and `wasm` are the same encoder.** Their lossy output is
  byte-identical (verified by hash), so their sizes and PSNR match exactly;
  `wasm` is the cgo-free option and runs ~2-6x slower than native `libwebp`.

On effort. The modes above are two points on each encoder's curve; `run-sweep.sh`
walks every setting. The figures are totals over the seven test images, arm64
first and amd64 second, and the effort numbers are each engine's own scale.

- **Lossless, against `nativewebp`: our effort 3 is smaller at the same time.**
  15.99 MiB in 7.3 s / 10.9 s, against its 17.40 MiB in 6.9 s / 12.0 s at its
  fastest level. That is 8% smaller for 1.05x the time on arm64 and 0.91x on
  amd64. The fixed-mode tables compare its level 6 against our effort 6, which is
  our encoder searching four times as long as it has to for that size.
- **`nativewebp`'s three levels are one point.** Level 0 to level 6 moves the
  corpus by 0.06% (17.40 to 17.39 MiB) for 9% more time on arm64 and 15% on
  amd64. It has no size-for-speed curve to trade along.
- **Our lossless efforts 7 to 9 are not worth their time.** 488 s / 634 s against
  17 s / 25 s at effort 6, for 1.2% smaller output, and 7, 8 and 9 write
  byte-identical files. That is 29x the time on arm64 for the last 1%.
- **Our lossless efforts 0 and 1 are off the useful curve too**, at 54.24 and
  45.24 MiB against libwebp's 17.71 MiB at a comparable 0.8 s / 1.0 s. Effort 2
  is the first setting that compresses: 19.65 MiB.
- **libwebp's lossless curve flattens at level 3**, 14.69 MiB in 5.8 s / 6.0 s.
  Levels 4 to 9 stay within 0.2% of it, and level 9 spends 97 s / 103 s to land
  at 14.66 MiB. It is the smallest lossless output here at any setting.
- **Lossy: our efforts 1 to 8 buy size by giving up quality.** They write 2.71 to
  3.17 MiB at 41.6-42.0 dB, where libwebp holds 42.7-43.1 dB across its whole
  range. Effort 0 (3.65 MiB, 42.80 dB) and effort 9 (3.11 MiB, 42.75 dB) are the
  two that answer quality 90 the way libwebp does, and effort 9 costs 5.6 s / 7.2 s
  against libwebp method 6's 2.9 s / 4.2 s for the same 3.1 MiB. Reading the size
  panel alone would credit effort 6 with a 12% win it did not earn.
- **Several of our settings are aliases.** Lossy 1 and 2, 3 and 4, and 6 and 7
  each write identical output in identical time, as do lossless 7, 8 and 9.
- **`wasm` is libwebp's curve shifted right**, 2-3x on lossy and 1.5-3x on
  lossless, at the same sizes. Its lossless knob is `Method` rather than the
  preset level, so its curve stops at 6 and never reaches libwebp's level 9.

On memory:

- **Lossy, we are the lightest of the three Go options:** 0.66-0.95x libwebp's
  peak, and roughly 12-15 MiB per megapixel on the larger images. Encoding a 4K
  frame costs on the order of 130 MiB.
- **`wasm` costs 1.5-2.7x libwebp's peak**, the widest gap in the lossy modes: it
  carries a WebAssembly runtime and its own linear memory on top of the encode.
  That is the memory half of the cgo-free tradeoff, next to the 2-6x on time.
- **Lossless costs everyone several times what lossy does**, and we sit at
  1.7-2.4x libwebp's peak, up to 113 MiB per megapixel: 420-484 MiB on the 5.5+ MP
  test images against libwebp's 194-215 MiB. `wasm` is above us too, at 2.6-3.8x
  and 572-697 MiB.
- **`nativewebp` is the lightest lossless encoder here**, at 0.41-0.55x our peak
  and level with libwebp's (0.88-1.07x): 32-55 MiB per megapixel against our
  71-113, or 199-213 MiB on the 5.5+ MP images against our 420-484. Alongside its
  8-23% larger files, that is one encoder trading compression for both time and
  memory, and it is the reason the lossless memory gap is a property of our
  encoder rather than of Go.
- Per-megapixel figures run higher on small images, because a fixed runtime floor
  is spread over fewer pixels: in the lossy modes Lena at 0.81 MP reads about twice
  the per-megapixel cost of the 5.5 MP images.

On decoding. Every engine decodes the same libwebp-encoded file and has to end at
packed RGBA, so the YCbCr-returning engines (`x/image` on lossy, `wasm` on
everything) pay for that conversion inside the measurement, as an application
would:

- **vs `x/image`, lossy: we are slightly faster**, 0.88-0.99x its time on arm64
  and 0.86-0.91x on amd64. This is the comparison to make if you are choosing
  between the two pure-Go decoders for photos.
- **vs `x/image`, lossless: we are slightly ahead on the mean and level per
  image.** 0.78-1.15x its time on arm64 and 0.76-0.87x on amd64, which is 0.99x
  and 0.82x on the geometric mean. amd64 is a consistent win; arm64 comes down to
  the image, where we are ahead on the two smallest files and behind by up to 15%
  on the largest. On the 5.5 MP images that is 100-121 ms against its 99-106 ms on
  arm64, and 96-126 ms against its 118-144 ms on amd64.
- **vs libwebp: 2.4-5.8x slower on lossy**, and 1.5-2.2x (arm64) / 1.9-2.9x
  (amd64) on lossless.
- **vs `wasm`, the other cgo-free libwebp: we are faster on lossless**,
  0.37-0.81x its time; lossy is a wash on arm64 (0.93-1.16x) and ours on amd64
  (0.74-0.85x).

## Charts

![Effort against time and size: one line per engine through its effort settings, with encode time on the x axis and output size on the y axis, one panel per mode and machine](charts/effort-sweep-light.svg#gh-light-mode-only)
![Effort against time and size: one line per engine through its effort settings, with encode time on the x axis and output size on the y axis, one panel per mode and machine](charts/effort-sweep-dark.svg#gh-dark-mode-only)

![Size and quality against libwebp: one point per test image, with output size relative to libwebp on the x axis and PSNR difference on the y axis, faceted by lossy mode](charts/rate-distortion-light.svg#gh-light-mode-only)
![Size and quality against libwebp: one point per test image, with output size relative to libwebp on the x axis and PSNR difference on the y axis, faceted by lossy mode](charts/rate-distortion-dark.svg#gh-dark-mode-only)

![Encode time per image for each engine, one panel per mode and machine, with bars that run off the panel drawn fading out under an arrow](charts/encode-time-light.svg#gh-light-mode-only)
![Encode time per image for each engine, one panel per mode and machine, with bars that run off the panel drawn fading out under an arrow](charts/encode-time-dark.svg#gh-dark-mode-only)

![Decode time per image for each engine, one panel per mode and machine, on the same bar layout as the encode time figure, with x/image added](charts/decode-time-light.svg#gh-light-mode-only)
![Decode time per image for each engine, one panel per mode and machine, on the same bar layout as the encode time figure, with x/image added](charts/decode-time-dark.svg#gh-dark-mode-only)

![Peak memory per megapixel for each engine, one panel per mode and machine, on the same bar layout as the encode time figure](charts/peak-memory-light.svg#gh-light-mode-only)
![Peak memory per megapixel for each engine, one panel per mode and machine, on the same bar layout as the encode time figure](charts/peak-memory-dark.svg#gh-dark-mode-only)

Regenerate them from the tables below with `benchmark/chart/chart.go`.

## arm64 (Apple M4 Pro)

```
file                                            mode        engine      width  height  bytes    psnr_db  iters  ms_per_op
Lena_512.png                                    lossless    libwebp     900    900     627632   -        9      243.325
Lena_512.png                                    lossless    nativewebp  900    900     738176   -        11     185.640
Lena_512.png                                    lossless    ours        900    900     651634   -        5      447.615
Lena_512.png                                    lossless    wasm        900    900     622766   -        3      920.717
Lena_512.png                                    lossy-fast  libwebp     900    900     103980   41.04    133    15.097
Lena_512.png                                    lossy-fast  ours        900    900     113182   41.02    90     22.266
Lena_512.png                                    lossy-fast  wasm        900    900     103980   41.04    54     37.452
Lena_512.png                                    lossy-slow  libwebp     900    900     89968    40.94    27     75.090
Lena_512.png                                    lossy-slow  ours        900    900     88890    40.16    14     150.384
Lena_512.png                                    lossy-slow  wasm        900    900     89968    40.94    11     196.100
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    libwebp     2025   2700    3241976  -        2      1921.796
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    nativewebp  2025   2700    3926412  -        1      2203.418
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    ours        2025   2700    3355646  -        1      4260.643
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    wasm        2025   2700    3249798  -        1      3673.480
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  libwebp     2025   2700    598034   41.96    21     97.182
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  ours        2025   2700    723064   42.02    14     151.822
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  wasm        2025   2700    598034   41.96    9      245.926
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  libwebp     2025   2700    610518   42.58    4      660.263
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  ours        2025   2700    592722   42.26    2      1414.258
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  wasm        2025   2700    610518   42.58    2      1582.991
pexels-martin-alargent-1165956-5665465.jpg      lossless    libwebp     2025   2700    2943528  -        2      1957.118
pexels-martin-alargent-1165956-5665465.jpg      lossless    nativewebp  2025   2700    3563584  -        2      1636.355
pexels-martin-alargent-1165956-5665465.jpg      lossless    ours        2025   2700    3058988  -        1      3447.409
pexels-martin-alargent-1165956-5665465.jpg      lossless    wasm        2025   2700    2953798  -        1      3711.016
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  libwebp     2025   2700    726824   42.63    20     100.425
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  ours        2025   2700    756806   42.39    14     151.496
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  wasm        2025   2700    726824   42.63    9      244.538
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  libwebp     2025   2700    603264   42.75    4      560.010
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  ours        2025   2700    601454   42.73    2      1046.475
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  wasm        2025   2700    603264   42.75    2      1391.643
pexels-mavihnt-38213559.jpg                     lossless    libwebp     2560   1706    3482480  -        2      1518.171
pexels-mavihnt-38213559.jpg                     lossless    nativewebp  2560   1706    4009534  -        2      1449.032
pexels-mavihnt-38213559.jpg                     lossless    ours        2560   1706    3585610  -        1      3072.590
pexels-mavihnt-38213559.jpg                     lossless    wasm        2560   1706    3488584  -        1      2976.316
pexels-mavihnt-38213559.jpg                     lossy-fast  libwebp     2560   1706    983360   40.87    20     104.915
pexels-mavihnt-38213559.jpg                     lossy-fast  ours        2560   1706    1037210  40.99    12     168.339
pexels-mavihnt-38213559.jpg                     lossy-fast  wasm        2560   1706    983360   40.87    9      243.078
pexels-mavihnt-38213559.jpg                     lossy-slow  libwebp     2560   1706    925032   41.14    4      660.783
pexels-mavihnt-38213559.jpg                     lossy-slow  ours        2560   1706    935652   41.05    2      1294.103
pexels-mavihnt-38213559.jpg                     lossy-slow  wasm        2560   1706    925032   41.14    2      1574.205
pexels-steve-15267299.jpg                       lossless    libwebp     2095   3000    2104690  -        1      2165.929
pexels-steve-15267299.jpg                       lossless    nativewebp  2095   3000    2622528  -        2      1712.520
pexels-steve-15267299.jpg                       lossless    ours        2095   3000    2218786  -        1      3650.701
pexels-steve-15267299.jpg                       lossless    wasm        2095   3000    2119370  -        1      4075.713
pexels-steve-15267299.jpg                       lossy-fast  libwebp     2095   3000    284570   44.34    22     92.023
pexels-steve-15267299.jpg                       lossy-fast  ours        2095   3000    294606   43.91    18     111.213
pexels-steve-15267299.jpg                       lossy-fast  wasm        2095   3000    284570   44.34    9      243.792
pexels-steve-15267299.jpg                       lossy-slow  libwebp     2095   3000    247610   44.50    6      339.614
pexels-steve-15267299.jpg                       lossy-slow  ours        2095   3000    233446   44.04    3      919.133
pexels-steve-15267299.jpg                       lossy-slow  wasm        2095   3000    247610   44.50    2      1067.493
pexels-steve-29626041.jpg                       lossless    libwebp     2560   1440    283988   -        3      868.113
pexels-steve-29626041.jpg                       lossless    nativewebp  2560   1440    379050   -        4      664.656
pexels-steve-29626041.jpg                       lossless    ours        2560   1440    309370   -        3      935.965
pexels-steve-29626041.jpg                       lossless    wasm        2560   1440    296596   -        2      1754.224
pexels-steve-29626041.jpg                       lossy-fast  libwebp     2560   1440    47614    49.05    47     43.349
pexels-steve-29626041.jpg                       lossy-fast  ours        2560   1440    45686    49.31    43     47.279
pexels-steve-29626041.jpg                       lossy-fast  wasm        2560   1440    47614    49.05    17     123.823
pexels-steve-29626041.jpg                       lossy-slow  libwebp     2560   1440    38252    49.65    16     130.558
pexels-steve-29626041.jpg                       lossy-slow  ours        2560   1440    42614    49.25    8      264.209
pexels-steve-29626041.jpg                       lossy-slow  wasm        2560   1440    38252    49.65    5      469.668
pexels-toulouse-10807703.jpg                    lossless    libwebp     1400   2100    2688812  -        2      1058.420
pexels-toulouse-10807703.jpg                    lossless    nativewebp  1400   2100    2996214  -        3      727.733
pexels-toulouse-10807703.jpg                    lossless    ours        1400   2100    2765508  -        2      1778.688
pexels-toulouse-10807703.jpg                    lossless    wasm        1400   2100    2685806  -        1      2347.664
pexels-toulouse-10807703.jpg                    lossy-fast  libwebp     1400   2100    857060   39.84    26     78.470
pexels-toulouse-10807703.jpg                    lossy-fast  ours        1400   2100    854742   39.93    16     132.534
pexels-toulouse-10807703.jpg                    lossy-fast  wasm        1400   2100    857060   39.84    12     171.643
pexels-toulouse-10807703.jpg                    lossy-slow  libwebp     1400   2100    758466   39.84    5      453.195
pexels-toulouse-10807703.jpg                    lossy-slow  ours        1400   2100    761160   39.75    3      719.818
pexels-toulouse-10807703.jpg                    lossy-slow  wasm        1400   2100    758466   39.84    2      1065.503
```

Peak RSS, one encode per process:

```
file                                            mode        engine      width  height  megapixels  peak_rss_mib  mib_per_mp
Lena_512.png                                    lossless    libwebp     900    900     0.81        48.2          59.5
Lena_512.png                                    lossless    nativewebp  900    900     0.81        44.8          55.4
Lena_512.png                                    lossless    ours        900    900     0.81        91.3          112.7
Lena_512.png                                    lossless    wasm        900    900     0.81        147.9         182.6
Lena_512.png                                    lossy-fast  libwebp     900    900     0.81        24.4          30.1
Lena_512.png                                    lossy-fast  ours        900    900     0.81        22.5          27.8
Lena_512.png                                    lossy-fast  wasm        900    900     0.81        38.0          47.0
Lena_512.png                                    lossy-slow  libwebp     900    900     0.81        25.8          31.8
Lena_512.png                                    lossy-slow  ours        900    900     0.81        23.2          28.6
Lena_512.png                                    lossy-slow  wasm        900    900     0.81        48.1          59.4
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    libwebp     2025   2700    5.47        215.0         39.3
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    nativewebp  2025   2700    5.47        204.6         37.4
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    ours        2025   2700    5.47        472.6         86.4
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    wasm        2025   2700    5.47        696.1         127.3
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  libwebp     2025   2700    5.47        88.3          16.1
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  ours        2025   2700    5.47        70.0          12.8
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  wasm        2025   2700    5.47        194.1         35.5
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  libwebp     2025   2700    5.47        97.2          17.8
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  ours        2025   2700    5.47        70.4          12.9
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  wasm        2025   2700    5.47        194.2         35.5
pexels-martin-alargent-1165956-5665465.jpg      lossless    libwebp     2025   2700    5.47        205.4         37.6
pexels-martin-alargent-1165956-5665465.jpg      lossless    nativewebp  2025   2700    5.47        200.2         36.6
pexels-martin-alargent-1165956-5665465.jpg      lossless    ours        2025   2700    5.47        423.7         77.5
pexels-martin-alargent-1165956-5665465.jpg      lossless    wasm        2025   2700    5.47        696.5         127.4
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  libwebp     2025   2700    5.47        88.7          16.2
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  ours        2025   2700    5.47        69.9          12.8
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  wasm        2025   2700    5.47        194.1         35.5
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  libwebp     2025   2700    5.47        96.6          17.7
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  ours        2025   2700    5.47        70.2          12.8
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  wasm        2025   2700    5.47        194.2         35.5
pexels-mavihnt-38213559.jpg                     lossless    libwebp     2560   1706    4.37        181.3         41.5
pexels-mavihnt-38213559.jpg                     lossless    nativewebp  2560   1706    4.37        174.3         39.9
pexels-mavihnt-38213559.jpg                     lossless    ours        2560   1706    4.37        396.9         90.9
pexels-mavihnt-38213559.jpg                     lossless    wasm        2560   1706    4.37        560.0         128.2
pexels-mavihnt-38213559.jpg                     lossy-fast  libwebp     2560   1706    4.37        74.3          17.0
pexels-mavihnt-38213559.jpg                     lossy-fast  ours        2560   1706    4.37        58.4          13.4
pexels-mavihnt-38213559.jpg                     lossy-fast  wasm        2560   1706    4.37        157.5         36.1
pexels-mavihnt-38213559.jpg                     lossy-slow  libwebp     2560   1706    4.37        88.8          20.3
pexels-mavihnt-38213559.jpg                     lossy-slow  ours        2560   1706    4.37        58.6          13.4
pexels-mavihnt-38213559.jpg                     lossy-slow  wasm        2560   1706    4.37        221.9         50.8
pexels-steve-15267299.jpg                       lossless    libwebp     2095   3000    6.29        208.2         33.1
pexels-steve-15267299.jpg                       lossless    nativewebp  2095   3000    6.29        212.7         33.8
pexels-steve-15267299.jpg                       lossless    ours        2095   3000    6.29        483.7         77.0
pexels-steve-15267299.jpg                       lossless    wasm        2095   3000    6.29        573.4         91.2
pexels-steve-15267299.jpg                       lossy-fast  libwebp     2095   3000    6.29        98.1          15.6
pexels-steve-15267299.jpg                       lossy-fast  ours        2095   3000    6.29        78.8          12.5
pexels-steve-15267299.jpg                       lossy-fast  wasm        2095   3000    6.29        221.4         35.2
pexels-steve-15267299.jpg                       lossy-slow  libwebp     2095   3000    6.29        102.0         16.2
pexels-steve-15267299.jpg                       lossy-slow  ours        2095   3000    6.29        79.9          12.7
pexels-steve-15267299.jpg                       lossy-slow  wasm        2095   3000    6.29        221.3         35.2
pexels-steve-29626041.jpg                       lossless    libwebp     2560   1440    3.69        131.8         35.8
pexels-steve-29626041.jpg                       lossless    nativewebp  2560   1440    3.69        116.3         31.6
pexels-steve-29626041.jpg                       lossless    ours        2560   1440    3.69        283.0         76.8
pexels-steve-29626041.jpg                       lossless    wasm        2560   1440    3.69        341.5         92.6
pexels-steve-29626041.jpg                       lossy-fast  libwebp     2560   1440    3.69        61.5          16.7
pexels-steve-29626041.jpg                       lossy-fast  ours        2560   1440    3.69        50.6          13.7
pexels-steve-29626041.jpg                       lossy-fast  wasm        2560   1440    3.69        91.1          24.7
pexels-steve-29626041.jpg                       lossy-slow  libwebp     2560   1440    3.69        62.4          16.9
pexels-steve-29626041.jpg                       lossy-slow  ours        2560   1440    3.69        50.9          13.8
pexels-steve-29626041.jpg                       lossy-slow  wasm        2560   1440    3.69        134.7         36.5
pexels-toulouse-10807703.jpg                    lossless    libwebp     1400   2100    2.94        127.2         43.3
pexels-toulouse-10807703.jpg                    lossless    nativewebp  1400   2100    2.94        124.4         42.3
pexels-toulouse-10807703.jpg                    lossless    ours        1400   2100    2.94        245.7         83.6
pexels-toulouse-10807703.jpg                    lossless    wasm        1400   2100    2.94        382.6         130.1
pexels-toulouse-10807703.jpg                    lossy-fast  libwebp     1400   2100    2.94        54.0          18.4
pexels-toulouse-10807703.jpg                    lossy-fast  ours        1400   2100    2.94        42.5          14.5
pexels-toulouse-10807703.jpg                    lossy-fast  wasm        1400   2100    2.94        110.0         37.4
pexels-toulouse-10807703.jpg                    lossy-slow  libwebp     1400   2100    2.94        66.2          22.5
pexels-toulouse-10807703.jpg                    lossy-slow  ours        1400   2100    2.94        44.2          15.0
pexels-toulouse-10807703.jpg                    lossy-slow  wasm        1400   2100    2.94        153.7         52.3
```

Decode, one file per mode encoded by libwebp:

```
file                                            mode      engine     width  height  bytes    iters  ms_per_op
Lena_512.png                                    lossless  libwebp    900    900     627632   195    10.299
Lena_512.png                                    lossless  ours       900    900     627632   105    19.133
Lena_512.png                                    lossless  wasm       900    900     627632   81     24.773
Lena_512.png                                    lossless  x/image    900    900     627632   106    18.998
Lena_512.png                                    lossy     libwebp    900    900     89968    407    4.916
Lena_512.png                                    lossy     ours       900    900     89968    116    17.341
Lena_512.png                                    lossy     wasm       900    900     89968    126    15.911
Lena_512.png                                    lossy     x/image    900    900     89968    110    18.346
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp    2025   2700    3241976  36     55.652
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours       2025   2700    3241976  17     121.314
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm       2025   2700    3241976  14     149.590
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  x/image    2025   2700    3241976  19     105.726
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp    2025   2700    610518   58     34.626
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours       2025   2700    610518   17     124.246
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm       2025   2700    610518   18     116.024
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     x/image    2025   2700    610518   16     131.437
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp    2025   2700    2943528  36     55.637
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours       2025   2700    2943528  19     110.110
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm       2025   2700    2943528  14     146.953
pexels-martin-alargent-1165956-5665465.jpg      lossless  x/image    2025   2700    2943528  20     101.312
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp    2025   2700    603264   61     32.986
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours       2025   2700    603264   18     113.610
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm       2025   2700    603264   19     107.279
pexels-martin-alargent-1165956-5665465.jpg      lossy     x/image    2025   2700    603264   17     122.982
pexels-mavihnt-38213559.jpg                     lossless  libwebp    2560   1706    3482480  40     50.293
pexels-mavihnt-38213559.jpg                     lossless  ours       2560   1706    3482480  20     100.147
pexels-mavihnt-38213559.jpg                     lossless  wasm       2560   1706    3482480  16     128.166
pexels-mavihnt-38213559.jpg                     lossless  x/image    2560   1706    3482480  21     98.758
pexels-mavihnt-38213559.jpg                     lossy     libwebp    2560   1706    925032   48     42.135
pexels-mavihnt-38213559.jpg                     lossy     ours       2560   1706    925032   18     113.236
pexels-mavihnt-38213559.jpg                     lossy     wasm       2560   1706    925032   20     100.201
pexels-mavihnt-38213559.jpg                     lossy     x/image    2560   1706    925032   16     128.026
pexels-steve-15267299.jpg                       lossless  libwebp    2095   3000    2104690  36     56.624
pexels-steve-15267299.jpg                       lossless  ours       2095   3000    2104690  20     103.508
pexels-steve-15267299.jpg                       lossless  wasm       2095   3000    2104690  13     156.757
pexels-steve-15267299.jpg                       lossless  x/image    2095   3000    2104690  20     102.349
pexels-steve-15267299.jpg                       lossy     libwebp    2095   3000    247610   98     20.533
pexels-steve-15267299.jpg                       lossy     ours       2095   3000    247610   21     97.708
pexels-steve-15267299.jpg                       lossy     wasm       2095   3000    247610   21     95.333
pexels-steve-15267299.jpg                       lossy     x/image    2095   3000    247610   21     98.499
pexels-steve-29626041.jpg                       lossless  libwebp    2560   1440    283988   111    18.045
pexels-steve-29626041.jpg                       lossless  ours       2560   1440    283988   74     27.061
pexels-steve-29626041.jpg                       lossless  wasm       2560   1440    283988   29     69.194
pexels-steve-29626041.jpg                       lossless  x/image    2560   1440    283988   58     34.549
pexels-steve-29626041.jpg                       lossy     libwebp    2560   1440    38252    281    7.136
pexels-steve-29626041.jpg                       lossy     ours       2560   1440    38252    49     41.070
pexels-steve-29626041.jpg                       lossy     wasm       2560   1440    38252    46     43.989
pexels-steve-29626041.jpg                       lossy     x/image    2560   1440    38252    48     41.906
pexels-toulouse-10807703.jpg                    lossless  libwebp    1400   2100    2688812  56     36.064
pexels-toulouse-10807703.jpg                    lossless  ours       1400   2100    2688812  30     67.248
pexels-toulouse-10807703.jpg                    lossless  wasm       1400   2100    2688812  23     88.447
pexels-toulouse-10807703.jpg                    lossless  x/image    1400   2100    2688812  28     73.078
pexels-toulouse-10807703.jpg                    lossy     libwebp    1400   2100    758466   61     33.112
pexels-toulouse-10807703.jpg                    lossy     ours       1400   2100    758466   24     85.657
pexels-toulouse-10807703.jpg                    lossy     wasm       1400   2100    758466   28     73.887
pexels-toulouse-10807703.jpg                    lossy     x/image    1400   2100    758466   21     97.053
```

Effort sweep, every setting of every engine:

```
file                                            mode      engine      effort  width  height  bytes     psnr_db  iters  ms_per_op
Lena_512.png                                    lossless  libwebp     0       900    900     734910    -        48     21.122
Lena_512.png                                    lossless  libwebp     1       900    900     661194    -        11     99.736
Lena_512.png                                    lossless  libwebp     2       900    900     625874    -        7      149.824
Lena_512.png                                    lossless  libwebp     3       900    900     628862    -        6      172.726
Lena_512.png                                    lossless  libwebp     4       900    900     628612    -        6      184.363
Lena_512.png                                    lossless  libwebp     5       900    900     628612    -        6      184.992
Lena_512.png                                    lossless  libwebp     6       900    900     627632    -        4      253.510
Lena_512.png                                    lossless  libwebp     7       900    900     625628    -        3      356.267
Lena_512.png                                    lossless  libwebp     8       900    900     616842    -        2      746.810
Lena_512.png                                    lossless  libwebp     9       900    900     609124    -        1      3905.650
Lena_512.png                                    lossless  nativewebp  0       900    900     741562    -        7      152.418
Lena_512.png                                    lossless  nativewebp  4       900    900     739400    -        7      157.531
Lena_512.png                                    lossless  nativewebp  6       900    900     738176    -        6      171.620
Lena_512.png                                    lossless  ours        0       900    900     1707902   -        34     29.568
Lena_512.png                                    lossless  ours        1       900    900     1481022   -        27     38.078
Lena_512.png                                    lossless  ours        2       900    900     754084    -        6      176.936
Lena_512.png                                    lossless  ours        3       900    900     698252    -        6      186.301
Lena_512.png                                    lossless  ours        4       900    900     697206    -        4      272.541
Lena_512.png                                    lossless  ours        5       900    900     697926    -        3      334.386
Lena_512.png                                    lossless  ours        6       900    900     651634    -        3      467.047
Lena_512.png                                    lossless  ours        7       900    900     650456    -        1      10176.262
Lena_512.png                                    lossless  ours        8       900    900     650456    -        1      10151.507
Lena_512.png                                    lossless  ours        9       900    900     650456    -        1      10155.057
Lena_512.png                                    lossless  wasm        0       900    900     731954    -        10     105.953
Lena_512.png                                    lossless  wasm        1       900    900     639584    -        3      402.340
Lena_512.png                                    lossless  wasm        2       900    900     627632    -        3      444.960
Lena_512.png                                    lossless  wasm        3       900    900     627632    -        3      444.688
Lena_512.png                                    lossless  wasm        4       900    900     627632    -        3      447.476
Lena_512.png                                    lossless  wasm        5       900    900     618010    -        2      989.780
Lena_512.png                                    lossless  wasm        6       900    900     622766    -        2      931.754
Lena_512.png                                    lossy     libwebp     0       900    900     103980    41.04    64     15.756
Lena_512.png                                    lossy     libwebp     1       900    900     102500    41.05    49     20.520
Lena_512.png                                    lossy     libwebp     2       900    900     94342     40.70    47     21.280
Lena_512.png                                    lossy     libwebp     3       900    900     91788     40.98    24     42.813
Lena_512.png                                    lossy     libwebp     4       900    900     92124     40.97    24     43.022
Lena_512.png                                    lossy     libwebp     5       900    900     91586     40.91    21     47.676
Lena_512.png                                    lossy     libwebp     6       900    900     89968     40.94    13     77.061
Lena_512.png                                    lossy     ours        0       900    900     113182    41.02    44     23.142
Lena_512.png                                    lossy     ours        1       900    900     111076    40.68    31     33.253
Lena_512.png                                    lossy     ours        2       900    900     111076    40.68    30     33.780
Lena_512.png                                    lossy     ours        3       900    900     102242    40.80    22     45.676
Lena_512.png                                    lossy     ours        4       900    900     102242    40.80    23     45.268
Lena_512.png                                    lossy     ours        5       900    900     95932     39.93    22     46.894
Lena_512.png                                    lossy     ours        6       900    900     91360     39.89    20     51.365
Lena_512.png                                    lossy     ours        7       900    900     91360     39.89    20     51.696
Lena_512.png                                    lossy     ours        8       900    900     90952     40.05    12     86.058
Lena_512.png                                    lossy     ours        9       900    900     88890     40.16    7      154.803
Lena_512.png                                    lossy     wasm        0       900    900     103980    41.04    27     38.354
Lena_512.png                                    lossy     wasm        1       900    900     102500    41.05    19     53.612
Lena_512.png                                    lossy     wasm        2       900    900     94342     40.70    18     56.368
Lena_512.png                                    lossy     wasm        3       900    900     91788     40.98    7      163.177
Lena_512.png                                    lossy     wasm        4       900    900     92124     40.97    7      163.281
Lena_512.png                                    lossy     wasm        5       900    900     91586     40.91    6      174.533
Lena_512.png                                    lossy     wasm        6       900    900     89968     40.94    6      199.637
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     0       2025   2700    3987548   -        6      175.785
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     1       2025   2700    3992712   -        2      673.934
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     2       2025   2700    3993532   -        2      877.412
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     3       2025   2700    3246280   -        1      1098.458
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     4       2025   2700    3246304   -        1      1242.278
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     5       2025   2700    3242976   -        1      1338.124
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     6       2025   2700    3241976   -        1      2025.428
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     7       2025   2700    3237862   -        1      3268.207
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     8       2025   2700    3246580   -        1      3635.668
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     9       2025   2700    3245966   -        1      20995.218
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  nativewebp  0       2025   2700    3944492   -        1      1605.586
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  nativewebp  4       2025   2700    3938686   -        1      1608.950
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  nativewebp  6       2025   2700    3926412   -        1      1721.519
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        0       2025   2700    13935672  -        5      216.343
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        1       2025   2700    11352054  -        2      722.516
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        2       2025   2700    3977946   -        1      1813.618
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        3       2025   2700    3359674   -        1      1683.498
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        4       2025   2700    3359902   -        1      2614.656
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        5       2025   2700    3365290   -        1      3677.119
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        6       2025   2700    3355646   -        1      4157.264
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        7       2025   2700    3351552   -        1      30429.045
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        8       2025   2700    3351552   -        1      29818.174
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        9       2025   2700    3351552   -        1      29896.689
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        0       2025   2700    3961206   -        1      1432.509
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        1       2025   2700    3244586   -        1      3114.771
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        2       2025   2700    3244586   -        1      3137.413
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        3       2025   2700    3244586   -        1      3131.742
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        4       2025   2700    3241976   -        1      3343.200
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        5       2025   2700    3249798   -        1      3953.679
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        6       2025   2700    3249798   -        1      3742.602
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     0       2025   2700    598034    41.96    10     102.271
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     1       2025   2700    588954    41.97    8      132.154
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     2       2025   2700    614528    42.20    7      153.757
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     3       2025   2700    627994    42.39    4      325.197
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     4       2025   2700    632636    42.42    4      326.044
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     5       2025   2700    625540    42.27    3      370.356
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     6       2025   2700    610518    42.58    2      671.386
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        0       2025   2700    723064    42.02    7      152.914
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        1       2025   2700    547196    40.42    5      210.199
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        2       2025   2700    547196    40.42    5      206.958
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        3       2025   2700    537122    40.76    3      362.763
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        4       2025   2700    537122    40.76    3      367.222
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        5       2025   2700    528042    40.65    3      371.927
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        6       2025   2700    468046    40.53    3      395.416
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        7       2025   2700    468046    40.53    3      398.471
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        8       2025   2700    456656    40.62    2      670.807
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        9       2025   2700    592722    42.26    1      1342.880
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        0       2025   2700    598034    41.96    5      249.034
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        1       2025   2700    588954    41.97    3      347.769
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        2       2025   2700    614528    42.20    3      401.980
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        3       2025   2700    627994    42.39    1      1185.075
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        4       2025   2700    632636    42.42    1      1185.973
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        5       2025   2700    625540    42.27    1      1279.853
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        6       2025   2700    610518    42.58    1      1594.421
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     0       2025   2700    3673696   -        6      186.733
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     1       2025   2700    3494146   -        2      766.853
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     2       2025   2700    3493206   -        2      982.173
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     3       2025   2700    2949858   -        1      1151.041
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     4       2025   2700    2950040   -        1      1288.936
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     5       2025   2700    2943056   -        1      1406.412
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     6       2025   2700    2943528   -        1      2053.980
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     7       2025   2700    2929290   -        1      2861.164
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     8       2025   2700    2938176   -        1      3450.125
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     9       2025   2700    2940364   -        1      24701.687
pexels-martin-alargent-1165956-5665465.jpg      lossless  nativewebp  0       2025   2700    3554880   -        1      1399.699
pexels-martin-alargent-1165956-5665465.jpg      lossless  nativewebp  4       2025   2700    3559008   -        1      1401.866
pexels-martin-alargent-1165956-5665465.jpg      lossless  nativewebp  6       2025   2700    3563584   -        1      1485.056
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        0       2025   2700    13256232  -        6      196.762
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        1       2025   2700    10676794  -        2      686.084
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        2       2025   2700    4054172   -        1      1612.122
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        3       2025   2700    3341756   -        1      1488.895
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        4       2025   2700    3318656   -        1      2188.589
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        5       2025   2700    3287048   -        1      2699.901
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        6       2025   2700    3058988   -        1      3368.894
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        7       2025   2700    3003328   -        1      21230.510
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        8       2025   2700    3003328   -        1      21306.649
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        9       2025   2700    3003328   -        1      21339.660
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        0       2025   2700    3625218   -        1      1307.295
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        1       2025   2700    2948150   -        1      3132.164
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        2       2025   2700    2948150   -        1      3094.038
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        3       2025   2700    2948150   -        1      3099.708
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        4       2025   2700    2943528   -        1      3345.951
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        5       2025   2700    2953798   -        1      4147.495
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        6       2025   2700    2953798   -        1      3827.062
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     0       2025   2700    726824    42.63    10     103.949
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     1       2025   2700    664274    42.64    8      134.714
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     2       2025   2700    626708    42.42    7      152.455
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     3       2025   2700    616320    42.96    4      304.995
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     4       2025   2700    618856    42.83    4      305.154
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     5       2025   2700    615274    42.71    3      340.819
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     6       2025   2700    603264    42.75    2      578.330
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        0       2025   2700    756806    42.39    7      151.912
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        1       2025   2700    686954    41.34    5      214.912
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        2       2025   2700    686954    41.34    5      221.302
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        3       2025   2700    617046    41.56    4      309.178
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        4       2025   2700    617046    41.56    4      310.939
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        5       2025   2700    603032    41.39    4      319.452
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        6       2025   2700    570570    41.33    3      347.855
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        7       2025   2700    570570    41.33    3      349.384
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        8       2025   2700    570042    41.64    2      630.069
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        9       2025   2700    601454    42.73    1      1067.225
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        0       2025   2700    726824    42.63    4      250.852
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        1       2025   2700    664274    42.64    3      353.988
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        2       2025   2700    626708    42.42    3      397.729
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        3       2025   2700    616320    42.96    1      1119.375
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        4       2025   2700    618856    42.83    1      1121.817
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        5       2025   2700    615274    42.71    1      1220.793
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        6       2025   2700    603264    42.75    1      1433.102
pexels-mavihnt-38213559.jpg                     lossless  libwebp     0       2560   1706    4156742   -        8      135.023
pexels-mavihnt-38213559.jpg                     lossless  libwebp     1       2560   1706    3973816   -        2      541.041
pexels-mavihnt-38213559.jpg                     lossless  libwebp     2       2560   1706    3988280   -        2      661.906
pexels-mavihnt-38213559.jpg                     lossless  libwebp     3       2560   1706    3487548   -        2      867.476
pexels-mavihnt-38213559.jpg                     lossless  libwebp     4       2560   1706    3488224   -        2      943.315
pexels-mavihnt-38213559.jpg                     lossless  libwebp     5       2560   1706    3482892   -        1      1047.284
pexels-mavihnt-38213559.jpg                     lossless  libwebp     6       2560   1706    3482480   -        1      1543.657
pexels-mavihnt-38213559.jpg                     lossless  libwebp     7       2560   1706    3481722   -        1      2501.297
pexels-mavihnt-38213559.jpg                     lossless  libwebp     8       2560   1706    3485234   -        1      2941.765
pexels-mavihnt-38213559.jpg                     lossless  libwebp     9       2560   1706    3485216   -        1      13844.946
pexels-mavihnt-38213559.jpg                     lossless  nativewebp  0       2560   1706    4004276   -        1      1141.420
pexels-mavihnt-38213559.jpg                     lossless  nativewebp  4       2560   1706    4007402   -        1      1174.897
pexels-mavihnt-38213559.jpg                     lossless  nativewebp  6       2560   1706    4009534   -        1      1219.116
pexels-mavihnt-38213559.jpg                     lossless  ours        0       2560   1706    11064720  -        6      181.228
pexels-mavihnt-38213559.jpg                     lossless  ours        1       2560   1706    9463456   -        2      517.766
pexels-mavihnt-38213559.jpg                     lossless  ours        2       2560   1706    4986210   -        1      1516.733
pexels-mavihnt-38213559.jpg                     lossless  ours        3       2560   1706    3768140   -        1      1516.062
pexels-mavihnt-38213559.jpg                     lossless  ours        4       2560   1706    3730772   -        1      1994.123
pexels-mavihnt-38213559.jpg                     lossless  ours        5       2560   1706    3733318   -        1      2673.574
pexels-mavihnt-38213559.jpg                     lossless  ours        6       2560   1706    3585610   -        1      3104.339
pexels-mavihnt-38213559.jpg                     lossless  ours        7       2560   1706    3575284   -        1      17561.439
pexels-mavihnt-38213559.jpg                     lossless  ours        8       2560   1706    3575284   -        1      17920.130
pexels-mavihnt-38213559.jpg                     lossless  ours        9       2560   1706    3575284   -        1      18070.840
pexels-mavihnt-38213559.jpg                     lossless  wasm        0       2560   1706    4116810   -        1      1014.886
pexels-mavihnt-38213559.jpg                     lossless  wasm        1       2560   1706    3484606   -        1      2463.986
pexels-mavihnt-38213559.jpg                     lossless  wasm        2       2560   1706    3484606   -        1      2475.875
pexels-mavihnt-38213559.jpg                     lossless  wasm        3       2560   1706    3484606   -        1      2474.175
pexels-mavihnt-38213559.jpg                     lossless  wasm        4       2560   1706    3482480   -        1      2672.367
pexels-mavihnt-38213559.jpg                     lossless  wasm        5       2560   1706    3488584   -        1      3246.805
pexels-mavihnt-38213559.jpg                     lossless  wasm        6       2560   1706    3488584   -        1      3013.253
pexels-mavihnt-38213559.jpg                     lossy     libwebp     0       2560   1706    983360    40.87    10     106.164
pexels-mavihnt-38213559.jpg                     lossy     libwebp     1       2560   1706    976610    40.88    8      133.714
pexels-mavihnt-38213559.jpg                     lossy     libwebp     2       2560   1706    935824    40.67    7      148.129
pexels-mavihnt-38213559.jpg                     lossy     libwebp     3       2560   1706    931074    41.15    4      285.531
pexels-mavihnt-38213559.jpg                     lossy     libwebp     4       2560   1706    934782    41.17    4      286.583
pexels-mavihnt-38213559.jpg                     lossy     libwebp     5       2560   1706    933892    41.05    4      325.691
pexels-mavihnt-38213559.jpg                     lossy     libwebp     6       2560   1706    925032    41.14    2      681.911
pexels-mavihnt-38213559.jpg                     lossy     ours        0       2560   1706    1037210   40.99    6      170.891
pexels-mavihnt-38213559.jpg                     lossy     ours        1       2560   1706    889590    39.11    5      226.687
pexels-mavihnt-38213559.jpg                     lossy     ours        2       2560   1706    889590    39.11    5      229.579
pexels-mavihnt-38213559.jpg                     lossy     ours        3       2560   1706    859500    39.31    3      364.261
pexels-mavihnt-38213559.jpg                     lossy     ours        4       2560   1706    859500    39.31    3      371.654
pexels-mavihnt-38213559.jpg                     lossy     ours        5       2560   1706    848646    39.18    3      370.999
pexels-mavihnt-38213559.jpg                     lossy     ours        6       2560   1706    798104    38.97    3      415.672
pexels-mavihnt-38213559.jpg                     lossy     ours        7       2560   1706    798104    38.97    3      411.127
pexels-mavihnt-38213559.jpg                     lossy     ours        8       2560   1706    784808    38.99    2      678.395
pexels-mavihnt-38213559.jpg                     lossy     ours        9       2560   1706    935652    41.05    1      1311.615
pexels-mavihnt-38213559.jpg                     lossy     wasm        0       2560   1706    983360    40.87    5      248.874
pexels-mavihnt-38213559.jpg                     lossy     wasm        1       2560   1706    976610    40.88    3      334.769
pexels-mavihnt-38213559.jpg                     lossy     wasm        2       2560   1706    935824    40.67    3      376.402
pexels-mavihnt-38213559.jpg                     lossy     wasm        3       2560   1706    931074    41.15    1      1081.229
pexels-mavihnt-38213559.jpg                     lossy     wasm        4       2560   1706    934782    41.17    1      1087.496
pexels-mavihnt-38213559.jpg                     lossy     wasm        5       2560   1706    933892    41.05    1      1182.683
pexels-mavihnt-38213559.jpg                     lossy     wasm        6       2560   1706    925032    41.14    1      1575.720
pexels-steve-15267299.jpg                       lossless  libwebp     0       2095   3000    2603112   -        6      179.537
pexels-steve-15267299.jpg                       lossless  libwebp     1       2095   3000    2557750   -        2      800.190
pexels-steve-15267299.jpg                       lossless  libwebp     2       2095   3000    2550344   -        1      1077.731
pexels-steve-15267299.jpg                       lossless  libwebp     3       2095   3000    2104000   -        1      1304.611
pexels-steve-15267299.jpg                       lossless  libwebp     4       2095   3000    2104010   -        1      1425.119
pexels-steve-15267299.jpg                       lossless  libwebp     5       2095   3000    2104010   -        1      1545.412
pexels-steve-15267299.jpg                       lossless  libwebp     6       2095   3000    2104690   -        1      2145.883
pexels-steve-15267299.jpg                       lossless  libwebp     7       2095   3000    2096804   -        1      2739.347
pexels-steve-15267299.jpg                       lossless  libwebp     8       2095   3000    2111270   -        1      3922.537
pexels-steve-15267299.jpg                       lossless  libwebp     9       2095   3000    2111200   -        1      21169.316
pexels-steve-15267299.jpg                       lossless  nativewebp  0       2095   3000    2635282   -        1      1458.404
pexels-steve-15267299.jpg                       lossless  nativewebp  4       2095   3000    2627962   -        1      1515.004
pexels-steve-15267299.jpg                       lossless  nativewebp  6       2095   3000    2622528   -        1      1606.103
pexels-steve-15267299.jpg                       lossless  ours        0       2095   3000    8710684   -        6      172.746
pexels-steve-15267299.jpg                       lossless  ours        1       2095   3000    7739368   -        3      432.195
pexels-steve-15267299.jpg                       lossless  ours        2       2095   3000    2499600   -        1      1208.169
pexels-steve-15267299.jpg                       lossless  ours        3       2095   3000    2316408   -        1      1243.896
pexels-steve-15267299.jpg                       lossless  ours        4       2095   3000    2309372   -        1      1807.480
pexels-steve-15267299.jpg                       lossless  ours        5       2095   3000    2253044   -        1      2671.876
pexels-steve-15267299.jpg                       lossless  ours        6       2095   3000    2218786   -        1      3223.192
pexels-steve-15267299.jpg                       lossless  ours        7       2095   3000    2146874   -        1      142178.114
pexels-steve-15267299.jpg                       lossless  ours        8       2095   3000    2146874   -        1      141471.293
pexels-steve-15267299.jpg                       lossless  ours        9       2095   3000    2146874   -        1      139079.425
pexels-steve-15267299.jpg                       lossless  wasm        0       2095   3000    2546112   -        1      1089.347
pexels-steve-15267299.jpg                       lossless  wasm        1       2095   3000    2103668   -        1      3139.217
pexels-steve-15267299.jpg                       lossless  wasm        2       2095   3000    2103668   -        1      3140.243
pexels-steve-15267299.jpg                       lossless  wasm        3       2095   3000    2103668   -        1      3198.558
pexels-steve-15267299.jpg                       lossless  wasm        4       2095   3000    2104690   -        1      3431.954
pexels-steve-15267299.jpg                       lossless  wasm        5       2095   3000    2119370   -        1      4091.579
pexels-steve-15267299.jpg                       lossless  wasm        6       2095   3000    2119370   -        1      3878.851
pexels-steve-15267299.jpg                       lossy     libwebp     0       2095   3000    284570    44.34    12     90.348
pexels-steve-15267299.jpg                       lossy     libwebp     1       2095   3000    275444    44.38    9      120.192
pexels-steve-15267299.jpg                       lossy     libwebp     2       2095   3000    261602    44.30    9      117.663
pexels-steve-15267299.jpg                       lossy     libwebp     3       2095   3000    255364    44.59    4      257.861
pexels-steve-15267299.jpg                       lossy     libwebp     4       2095   3000    256038    44.58    4      261.033
pexels-steve-15267299.jpg                       lossy     libwebp     5       2095   3000    253192    44.51    4      282.044
pexels-steve-15267299.jpg                       lossy     libwebp     6       2095   3000    247610    44.50    3      341.255
pexels-steve-15267299.jpg                       lossy     ours        0       2095   3000    294606    43.91    10     109.614
pexels-steve-15267299.jpg                       lossy     ours        1       2095   3000    277974    43.52    6      167.402
pexels-steve-15267299.jpg                       lossy     ours        2       2095   3000    277974    43.52    6      167.040
pexels-steve-15267299.jpg                       lossy     ours        3       2095   3000    249954    43.62    5      226.149
pexels-steve-15267299.jpg                       lossy     ours        4       2095   3000    249954    43.62    5      226.090
pexels-steve-15267299.jpg                       lossy     ours        5       2095   3000    232944    43.31    5      239.201
pexels-steve-15267299.jpg                       lossy     ours        6       2095   3000    216996    43.29    4      250.367
pexels-steve-15267299.jpg                       lossy     ours        7       2095   3000    216996    43.29    4      251.994
pexels-steve-15267299.jpg                       lossy     ours        8       2095   3000    212368    43.36    3      381.764
pexels-steve-15267299.jpg                       lossy     ours        9       2095   3000    233446    44.04    2      825.575
pexels-steve-15267299.jpg                       lossy     wasm        0       2095   3000    284570    44.34    5      236.453
pexels-steve-15267299.jpg                       lossy     wasm        1       2095   3000    275444    44.38    3      336.586
pexels-steve-15267299.jpg                       lossy     wasm        2       2095   3000    261602    44.30    3      339.400
pexels-steve-15267299.jpg                       lossy     wasm        3       2095   3000    255364    44.59    1      1040.498
pexels-steve-15267299.jpg                       lossy     wasm        4       2095   3000    256038    44.58    1      1046.232
pexels-steve-15267299.jpg                       lossy     wasm        5       2095   3000    253192    44.51    1      1099.386
pexels-steve-15267299.jpg                       lossy     wasm        6       2095   3000    247610    44.50    1      1054.574
pexels-steve-29626041.jpg                       lossless  libwebp     0       2560   1440    367064    -        22     46.954
pexels-steve-29626041.jpg                       lossless  libwebp     1       2560   1440    360562    -        3      383.073
pexels-steve-29626041.jpg                       lossless  libwebp     2       2560   1440    309376    -        2      585.489
pexels-steve-29626041.jpg                       lossless  libwebp     3       2560   1440    292034    -        2      619.606
pexels-steve-29626041.jpg                       lossless  libwebp     4       2560   1440    287166    -        2      648.905
pexels-steve-29626041.jpg                       lossless  libwebp     5       2560   1440    287662    -        2      716.289
pexels-steve-29626041.jpg                       lossless  libwebp     6       2560   1440    283988    -        2      883.732
pexels-steve-29626041.jpg                       lossless  libwebp     7       2560   1440    282342    -        2      922.671
pexels-steve-29626041.jpg                       lossless  libwebp     8       2560   1440    294462    -        1      1183.340
pexels-steve-29626041.jpg                       lossless  libwebp     9       2560   1440    292264    -        1      4068.198
pexels-steve-29626041.jpg                       lossless  nativewebp  0       2560   1440    378544    -        2      592.266
pexels-steve-29626041.jpg                       lossless  nativewebp  4       2560   1440    378536    -        2      615.710
pexels-steve-29626041.jpg                       lossless  nativewebp  6       2560   1440    379050    -        2      665.652
pexels-steve-29626041.jpg                       lossless  ours        0       2560   1440    827088    -        22     46.046
pexels-steve-29626041.jpg                       lossless  ours        1       2560   1440    532928    -        17     62.059
pexels-steve-29626041.jpg                       lossless  ours        2       2560   1440    356676    -        4      289.815
pexels-steve-29626041.jpg                       lossless  ours        3       2560   1440    335034    -        4      332.661
pexels-steve-29626041.jpg                       lossless  ours        4       2560   1440    327568    -        3      480.649
pexels-steve-29626041.jpg                       lossless  ours        5       2560   1440    309738    -        2      690.700
pexels-steve-29626041.jpg                       lossless  ours        6       2560   1440    309370    -        2      924.900
pexels-steve-29626041.jpg                       lossless  ours        7       2560   1440    286174    -        1      244124.112
pexels-steve-29626041.jpg                       lossless  ours        8       2560   1440    286174    -        1      244251.783
pexels-steve-29626041.jpg                       lossless  ours        9       2560   1440    286174    -        1      245152.022
pexels-steve-29626041.jpg                       lossless  wasm        0       2560   1440    346824    -        5      230.188
pexels-steve-29626041.jpg                       lossless  wasm        1       2560   1440    283552    -        1      1262.277
pexels-steve-29626041.jpg                       lossless  wasm        2       2560   1440    283552    -        1      1256.373
pexels-steve-29626041.jpg                       lossless  wasm        3       2560   1440    283552    -        1      1247.009
pexels-steve-29626041.jpg                       lossless  wasm        4       2560   1440    283988    -        1      1358.694
pexels-steve-29626041.jpg                       lossless  wasm        5       2560   1440    296596    -        1      1767.447
pexels-steve-29626041.jpg                       lossless  wasm        6       2560   1440    296596    -        1      1753.924
pexels-steve-29626041.jpg                       lossy     libwebp     0       2560   1440    47614     49.05    24     43.267
pexels-steve-29626041.jpg                       lossy     libwebp     1       2560   1440    47302     49.05    17     59.969
pexels-steve-29626041.jpg                       lossy     libwebp     2       2560   1440    39488     49.30    20     51.580
pexels-steve-29626041.jpg                       lossy     libwebp     3       2560   1440    39096     49.71    9      113.678
pexels-steve-29626041.jpg                       lossy     libwebp     4       2560   1440    39310     49.71    9      113.781
pexels-steve-29626041.jpg                       lossy     libwebp     5       2560   1440    38754     49.65    9      121.347
pexels-steve-29626041.jpg                       lossy     libwebp     6       2560   1440    38252     49.65    8      131.940
pexels-steve-29626041.jpg                       lossy     ours        0       2560   1440    45686     49.31    21     47.642
pexels-steve-29626041.jpg                       lossy     ours        1       2560   1440    46846     49.19    15     69.459
pexels-steve-29626041.jpg                       lossy     ours        2       2560   1440    46846     49.19    15     69.499
pexels-steve-29626041.jpg                       lossy     ours        3       2560   1440    40660     49.23    14     75.478
pexels-steve-29626041.jpg                       lossy     ours        4       2560   1440    40660     49.23    14     75.406
pexels-steve-29626041.jpg                       lossy     ours        5       2560   1440    38994     49.02    14     76.199
pexels-steve-29626041.jpg                       lossy     ours        6       2560   1440    36774     49.01    13     82.208
pexels-steve-29626041.jpg                       lossy     ours        7       2560   1440    36774     49.01    13     82.221
pexels-steve-29626041.jpg                       lossy     ours        8       2560   1440    36512     49.08    8      129.218
pexels-steve-29626041.jpg                       lossy     ours        9       2560   1440    42614     49.25    4      257.589
pexels-steve-29626041.jpg                       lossy     wasm        0       2560   1440    47614     49.05    8      125.809
pexels-steve-29626041.jpg                       lossy     wasm        1       2560   1440    47302     49.05    6      182.301
pexels-steve-29626041.jpg                       lossy     wasm        2       2560   1440    39488     49.30    7      163.209
pexels-steve-29626041.jpg                       lossy     wasm        3       2560   1440    39096     49.71    3      496.543
pexels-steve-29626041.jpg                       lossy     wasm        4       2560   1440    39310     49.71    3      498.888
pexels-steve-29626041.jpg                       lossy     wasm        5       2560   1440    38754     49.65    2      522.913
pexels-steve-29626041.jpg                       lossy     wasm        6       2560   1440    38252     49.65    3      472.053
pexels-toulouse-10807703.jpg                    lossless  libwebp     0       1400   2100    3044142   -        13     81.559
pexels-toulouse-10807703.jpg                    lossless  libwebp     1       1400   2100    2807602   -        3      403.765
pexels-toulouse-10807703.jpg                    lossless  libwebp     2       1400   2100    2780320   -        2      517.994
pexels-toulouse-10807703.jpg                    lossless  libwebp     3       1400   2100    2693662   -        2      565.205
pexels-toulouse-10807703.jpg                    lossless  libwebp     4       1400   2100    2693662   -        2      621.746
pexels-toulouse-10807703.jpg                    lossless  libwebp     5       1400   2100    2691936   -        2      696.894
pexels-toulouse-10807703.jpg                    lossless  libwebp     6       1400   2100    2688812   -        1      1070.728
pexels-toulouse-10807703.jpg                    lossless  libwebp     7       1400   2100    2688410   -        1      1329.354
pexels-toulouse-10807703.jpg                    lossless  libwebp     8       1400   2100    2686410   -        1      1977.199
pexels-toulouse-10807703.jpg                    lossless  libwebp     9       1400   2100    2685794   -        1      8080.313
pexels-toulouse-10807703.jpg                    lossless  nativewebp  0       1400   2100    2989804   -        2      594.630
pexels-toulouse-10807703.jpg                    lossless  nativewebp  4       1400   2100    2992710   -        2      610.494
pexels-toulouse-10807703.jpg                    lossless  nativewebp  6       1400   2100    2996214   -        2      680.585
pexels-toulouse-10807703.jpg                    lossless  ours        0       1400   2100    7375806   -        10     108.593
pexels-toulouse-10807703.jpg                    lossless  ours        1       1400   2100    6193432   -        5      212.502
pexels-toulouse-10807703.jpg                    lossless  ours        2       1400   2100    3973924   -        2      810.510
pexels-toulouse-10807703.jpg                    lossless  ours        3       1400   2100    2942470   -        2      833.356
pexels-toulouse-10807703.jpg                    lossless  ours        4       1400   2100    2912778   -        1      1101.399
pexels-toulouse-10807703.jpg                    lossless  ours        5       1400   2100    2912610   -        1      1333.306
pexels-toulouse-10807703.jpg                    lossless  ours        6       1400   2100    2765508   -        1      1788.296
pexels-toulouse-10807703.jpg                    lossless  ours        7       1400   2100    2746828   -        1      22450.640
pexels-toulouse-10807703.jpg                    lossless  ours        8       1400   2100    2746828   -        1      22705.375
pexels-toulouse-10807703.jpg                    lossless  ours        9       1400   2100    2746828   -        1      22630.101
pexels-toulouse-10807703.jpg                    lossless  wasm        0       1400   2100    3029106   -        3      483.340
pexels-toulouse-10807703.jpg                    lossless  wasm        1       1400   2100    2690886   -        1      1731.236
pexels-toulouse-10807703.jpg                    lossless  wasm        2       1400   2100    2690886   -        1      1728.253
pexels-toulouse-10807703.jpg                    lossless  wasm        3       1400   2100    2690886   -        1      1746.170
pexels-toulouse-10807703.jpg                    lossless  wasm        4       1400   2100    2688812   -        1      1891.576
pexels-toulouse-10807703.jpg                    lossless  wasm        5       1400   2100    2685806   -        1      2854.844
pexels-toulouse-10807703.jpg                    lossless  wasm        6       1400   2100    2685806   -        1      2383.043
pexels-toulouse-10807703.jpg                    lossy     libwebp     0       1400   2100    857060    39.84    13     78.027
pexels-toulouse-10807703.jpg                    lossy     libwebp     1       1400   2100    812040    39.84    11     99.792
pexels-toulouse-10807703.jpg                    lossy     libwebp     2       1400   2100    796814    39.60    10     107.284
pexels-toulouse-10807703.jpg                    lossy     libwebp     3       1400   2100    783482    39.95    6      195.207
pexels-toulouse-10807703.jpg                    lossy     libwebp     4       1400   2100    784734    39.96    6      194.807
pexels-toulouse-10807703.jpg                    lossy     libwebp     5       1400   2100    768426    39.81    5      227.010
pexels-toulouse-10807703.jpg                    lossy     libwebp     6       1400   2100    758466    39.84    3      453.061
pexels-toulouse-10807703.jpg                    lossy     ours        0       1400   2100    854742    39.93    8      136.540
pexels-toulouse-10807703.jpg                    lossy     ours        1       1400   2100    763158    38.22    6      173.388
pexels-toulouse-10807703.jpg                    lossy     ours        2       1400   2100    763158    38.22    6      171.728
pexels-toulouse-10807703.jpg                    lossy     ours        3       1400   2100    732726    38.37    4      251.718
pexels-toulouse-10807703.jpg                    lossy     ours        4       1400   2100    732726    38.37    5      249.733
pexels-toulouse-10807703.jpg                    lossy     ours        5       1400   2100    719110    38.10    4      258.507
pexels-toulouse-10807703.jpg                    lossy     ours        6       1400   2100    689980    37.96    4      290.810
pexels-toulouse-10807703.jpg                    lossy     ours        7       1400   2100    689980    37.96    4      287.023
pexels-toulouse-10807703.jpg                    lossy     ours        8       1400   2100    686416    38.08    3      487.069
pexels-toulouse-10807703.jpg                    lossy     ours        9       1400   2100    761160    39.75    2      674.493
pexels-toulouse-10807703.jpg                    lossy     wasm        0       1400   2100    857060    39.84    6      174.468
pexels-toulouse-10807703.jpg                    lossy     wasm        1       1400   2100    812040    39.84    5      240.452
pexels-toulouse-10807703.jpg                    lossy     wasm        2       1400   2100    796814    39.60    4      262.909
pexels-toulouse-10807703.jpg                    lossy     wasm        3       1400   2100    783482    39.95    2      750.825
pexels-toulouse-10807703.jpg                    lossy     wasm        4       1400   2100    784734    39.96    2      760.438
pexels-toulouse-10807703.jpg                    lossy     wasm        5       1400   2100    768426    39.81    2      820.264
pexels-toulouse-10807703.jpg                    lossy     wasm        6       1400   2100    758466    39.84    1      1089.049
```

## amd64 (AMD Ryzen 7 5700G)

```
file                                            mode        engine      width  height  bytes    psnr_db  iters  ms_per_op
Lena_512.png                                    lossless    libwebp     900    900     627632   -        8      254.507
Lena_512.png                                    lossless    nativewebp  900    900     738176   -        7      311.701
Lena_512.png                                    lossless    ours        900    900     651634   -        4      660.579
Lena_512.png                                    lossless    wasm        900    900     622766   -        2      1494.367
Lena_512.png                                    lossy-fast  libwebp     900    900     103980   41.04    116    17.337
Lena_512.png                                    lossy-fast  ours        900    900     113182   41.02    69     29.292
Lena_512.png                                    lossy-fast  wasm        900    900     103980   41.04    30     67.542
Lena_512.png                                    lossy-slow  libwebp     900    900     89968    40.94    19     109.055
Lena_512.png                                    lossy-slow  ours        900    900     88890    40.16    10     203.948
Lena_512.png                                    lossy-slow  wasm        900    900     89968    40.94    6      363.770
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    libwebp     2025   2700    3241976  -        1      2151.529
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    nativewebp  2025   2700    3926412  -        1      3088.770
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    ours        2025   2700    3355646  -        1      6401.795
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    wasm        2025   2700    3249798  -        1      5844.929
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  libwebp     2025   2700    598034   41.96    18     114.498
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  ours        2025   2700    723064   42.02    11     195.500
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  wasm        2025   2700    598034   41.96    5      453.489
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  libwebp     2025   2700    610518   42.58    3      969.852
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  ours        2025   2700    592722   42.26    2      1712.864
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  wasm        2025   2700    610518   42.58    1      2888.397
pexels-martin-alargent-1165956-5665465.jpg      lossless    libwebp     2025   2700    2943528  -        1      2067.281
pexels-martin-alargent-1165956-5665465.jpg      lossless    nativewebp  2025   2700    3563584  -        1      2570.167
pexels-martin-alargent-1165956-5665465.jpg      lossless    ours        2025   2700    3058988  -        1      4717.529
pexels-martin-alargent-1165956-5665465.jpg      lossless    wasm        2025   2700    2953798  -        1      5847.951
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  libwebp     2025   2700    726824   42.63    17     118.643
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  ours        2025   2700    756806   42.39    11     196.845
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  wasm        2025   2700    726824   42.63    5      454.450
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  libwebp     2025   2700    603264   42.75    3      814.518
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  ours        2025   2700    601454   42.73    2      1410.005
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  wasm        2025   2700    603264   42.75    1      2577.092
pexels-mavihnt-38213559.jpg                     lossless    libwebp     2560   1706    3482480  -        2      1641.486
pexels-mavihnt-38213559.jpg                     lossless    nativewebp  2560   1706    4009534  -        1      2128.291
pexels-mavihnt-38213559.jpg                     lossless    ours        2560   1706    3585610  -        1      4472.428
pexels-mavihnt-38213559.jpg                     lossless    wasm        2560   1706    3488584  -        1      4764.905
pexels-mavihnt-38213559.jpg                     lossy-fast  libwebp     2560   1706    983360   40.87    17     120.111
pexels-mavihnt-38213559.jpg                     lossy-fast  ours        2560   1706    1037210  40.99    10     211.491
pexels-mavihnt-38213559.jpg                     lossy-fast  wasm        2560   1706    983360   40.87    5      415.651
pexels-mavihnt-38213559.jpg                     lossy-slow  libwebp     2560   1706    925032   41.14    3      965.200
pexels-mavihnt-38213559.jpg                     lossy-slow  ours        2560   1706    935652   41.05    2      1630.513
pexels-mavihnt-38213559.jpg                     lossy-slow  wasm        2560   1706    925032   41.14    1      2700.096
pexels-steve-15267299.jpg                       lossless    libwebp     2095   3000    2104690  -        1      2267.576
pexels-steve-15267299.jpg                       lossless    nativewebp  2095   3000    2622528  -        1      2887.666
pexels-steve-15267299.jpg                       lossless    ours        2095   3000    2218786  -        1      4583.212
pexels-steve-15267299.jpg                       lossless    wasm        2095   3000    2119370  -        1      6046.492
pexels-steve-15267299.jpg                       lossy-fast  libwebp     2095   3000    284570   44.34    19     105.369
pexels-steve-15267299.jpg                       lossy-fast  ours        2095   3000    294606   43.91    13     157.594
pexels-steve-15267299.jpg                       lossy-fast  wasm        2095   3000    284570   44.34    5      460.934
pexels-steve-15267299.jpg                       lossy-slow  libwebp     2095   3000    247610   44.50    4      515.194
pexels-steve-15267299.jpg                       lossy-slow  ours        2095   3000    233446   44.04    2      1160.375
pexels-steve-15267299.jpg                       lossy-slow  wasm        2095   3000    247610   44.50    1      2130.103
pexels-steve-29626041.jpg                       lossless    libwebp     2560   1440    283988   -        3      729.844
pexels-steve-29626041.jpg                       lossless    nativewebp  2560   1440    379050   -        2      1385.597
pexels-steve-29626041.jpg                       lossless    ours        2560   1440    309370   -        2      1466.488
pexels-steve-29626041.jpg                       lossless    wasm        2560   1440    296596   -        1      2846.717
pexels-steve-29626041.jpg                       lossy-fast  libwebp     2560   1440    47614    49.05    40     50.422
pexels-steve-29626041.jpg                       lossy-fast  ours        2560   1440    45686    49.31    28     72.064
pexels-steve-29626041.jpg                       lossy-fast  wasm        2560   1440    47614    49.05    9      243.690
pexels-steve-29626041.jpg                       lossy-slow  libwebp     2560   1440    38252    49.65    10     206.648
pexels-steve-29626041.jpg                       lossy-slow  ours        2560   1440    42614    49.25    6      384.144
pexels-steve-29626041.jpg                       lossy-slow  wasm        2560   1440    38252    49.65    3      967.678
pexels-toulouse-10807703.jpg                    lossless    libwebp     1400   2100    2688812  -        2      1148.804
pexels-toulouse-10807703.jpg                    lossless    nativewebp  1400   2100    2996214  -        2      1295.368
pexels-toulouse-10807703.jpg                    lossless    ours        1400   2100    2765508  -        1      2631.769
pexels-toulouse-10807703.jpg                    lossless    wasm        1400   2100    2685806  -        1      3935.107
pexels-toulouse-10807703.jpg                    lossy-fast  libwebp     1400   2100    857060   39.84    23     90.429
pexels-toulouse-10807703.jpg                    lossy-fast  ours        1400   2100    854742   39.93    13     164.322
pexels-toulouse-10807703.jpg                    lossy-fast  wasm        1400   2100    857060   39.84    7      293.128
pexels-toulouse-10807703.jpg                    lossy-slow  libwebp     1400   2100    758466   39.84    4      658.388
pexels-toulouse-10807703.jpg                    lossy-slow  ours        1400   2100    761160   39.75    3      845.620
pexels-toulouse-10807703.jpg                    lossy-slow  wasm        1400   2100    758466   39.84    2      1937.546
```

Peak RSS, one encode per process:

```
file                                            mode        engine      width  height  megapixels  peak_rss_mib  mib_per_mp
Lena_512.png                                    lossless    libwebp     900    900     0.81        44.2          54.6
Lena_512.png                                    lossless    nativewebp  900    900     0.81        42.1          52.0
Lena_512.png                                    lossless    ours        900    900     0.81        77.1          95.2
Lena_512.png                                    lossless    wasm        900    900     0.81        139.2         171.9
Lena_512.png                                    lossy-fast  libwebp     900    900     0.81        22.8          28.2
Lena_512.png                                    lossy-fast  ours        900    900     0.81        21.7          26.8
Lena_512.png                                    lossy-fast  wasm        900    900     0.81        38.8          47.8
Lena_512.png                                    lossy-slow  libwebp     900    900     0.81        24.1          29.8
Lena_512.png                                    lossy-slow  ours        900    900     0.81        21.6          26.7
Lena_512.png                                    lossy-slow  wasm        900    900     0.81        49.0          60.5
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    libwebp     2025   2700    5.47        203.5         37.2
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    nativewebp  2025   2700    5.47        203.2         37.2
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    ours        2025   2700    5.47        466.9         85.4
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    wasm        2025   2700    5.47        696.7         127.4
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  libwebp     2025   2700    5.47        84.8          15.5
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  ours        2025   2700    5.47        68.4          12.5
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  wasm        2025   2700    5.47        193.5         35.4
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  libwebp     2025   2700    5.47        93.6          17.1
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  ours        2025   2700    5.47        72.6          13.3
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  wasm        2025   2700    5.47        193.7         35.4
pexels-martin-alargent-1165956-5665465.jpg      lossless    libwebp     2025   2700    5.47        194.2         35.5
pexels-martin-alargent-1165956-5665465.jpg      lossless    nativewebp  2025   2700    5.47        198.9         36.4
pexels-martin-alargent-1165956-5665465.jpg      lossless    ours        2025   2700    5.47        420.2         76.9
pexels-martin-alargent-1165956-5665465.jpg      lossless    wasm        2025   2700    5.47        695.1         127.1
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  libwebp     2025   2700    5.47        85.3          15.6
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  ours        2025   2700    5.47        70.2          12.8
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  wasm        2025   2700    5.47        193.7         35.4
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  libwebp     2025   2700    5.47        95.2          17.4
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  ours        2025   2700    5.47        70.5          12.9
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  wasm        2025   2700    5.47        195.6         35.8
pexels-mavihnt-38213559.jpg                     lossless    libwebp     2560   1706    4.37        169.8         38.9
pexels-mavihnt-38213559.jpg                     lossless    nativewebp  2560   1706    4.37        174.8         40.0
pexels-mavihnt-38213559.jpg                     lossless    ours        2560   1706    4.37        391.1         89.5
pexels-mavihnt-38213559.jpg                     lossless    wasm        2560   1706    4.37        557.4         127.6
pexels-mavihnt-38213559.jpg                     lossy-fast  libwebp     2560   1706    4.37        72.1          16.5
pexels-mavihnt-38213559.jpg                     lossy-fast  ours        2560   1706    4.37        58.3          13.3
pexels-mavihnt-38213559.jpg                     lossy-fast  wasm        2560   1706    4.37        157.2         36.0
pexels-mavihnt-38213559.jpg                     lossy-slow  libwebp     2560   1706    4.37        84.1          19.2
pexels-mavihnt-38213559.jpg                     lossy-slow  ours        2560   1706    4.37        58.3          13.3
pexels-mavihnt-38213559.jpg                     lossy-slow  wasm        2560   1706    4.37        223.2         51.1
pexels-steve-15267299.jpg                       lossless    libwebp     2095   3000    6.29        203.1         32.3
pexels-steve-15267299.jpg                       lossless    nativewebp  2095   3000    6.29        213.3         33.9
pexels-steve-15267299.jpg                       lossless    ours        2095   3000    6.29        483.7         77.0
pexels-steve-15267299.jpg                       lossless    wasm        2095   3000    6.29        572.0         91.0
pexels-steve-15267299.jpg                       lossy-fast  libwebp     2095   3000    6.29        94.9          15.1
pexels-steve-15267299.jpg                       lossy-fast  ours        2095   3000    6.29        76.2          12.1
pexels-steve-15267299.jpg                       lossy-fast  wasm        2095   3000    6.29        221.7         35.3
pexels-steve-15267299.jpg                       lossy-slow  libwebp     2095   3000    6.29        100.4         16.0
pexels-steve-15267299.jpg                       lossy-slow  ours        2095   3000    6.29        78.5          12.5
pexels-steve-15267299.jpg                       lossy-slow  wasm        2095   3000    6.29        219.6         34.9
pexels-steve-29626041.jpg                       lossless    libwebp     2560   1440    3.69        124.7         33.8
pexels-steve-29626041.jpg                       lossless    nativewebp  2560   1440    3.69        116.8         31.7
pexels-steve-29626041.jpg                       lossless    ours        2560   1440    3.69        260.3         70.6
pexels-steve-29626041.jpg                       lossless    wasm        2560   1440    3.69        471.8         128.0
pexels-steve-29626041.jpg                       lossy-fast  libwebp     2560   1440    3.69        60.2          16.3
pexels-steve-29626041.jpg                       lossy-fast  ours        2560   1440    3.69        48.2          13.1
pexels-steve-29626041.jpg                       lossy-fast  wasm        2560   1440    3.69        91.2          24.7
pexels-steve-29626041.jpg                       lossy-slow  libwebp     2560   1440    3.69        61.2          16.6
pexels-steve-29626041.jpg                       lossy-slow  ours        2560   1440    3.69        49.8          13.5
pexels-steve-29626041.jpg                       lossy-slow  wasm        2560   1440    3.69        135.4         36.7
pexels-toulouse-10807703.jpg                    lossless    libwebp     1400   2100    2.94        116.6         39.6
pexels-toulouse-10807703.jpg                    lossless    nativewebp  1400   2100    2.94        124.7         42.4
pexels-toulouse-10807703.jpg                    lossless    ours        1400   2100    2.94        243.6         82.8
pexels-toulouse-10807703.jpg                    lossless    wasm        1400   2100    2.94        381.9         129.9
pexels-toulouse-10807703.jpg                    lossy-fast  libwebp     1400   2100    2.94        52.0          17.7
pexels-toulouse-10807703.jpg                    lossy-fast  ours        1400   2100    2.94        42.3          14.4
pexels-toulouse-10807703.jpg                    lossy-fast  wasm        1400   2100    2.94        111.0         37.7
pexels-toulouse-10807703.jpg                    lossy-slow  libwebp     1400   2100    2.94        63.4          21.6
pexels-toulouse-10807703.jpg                    lossy-slow  ours        1400   2100    2.94        42.2          14.4
pexels-toulouse-10807703.jpg                    lossy-slow  wasm        1400   2100    2.94        153.4         52.2
```

Decode, one file per mode encoded by libwebp:

```
file                                            mode      engine     width  height  bytes    iters  ms_per_op
Lena_512.png                                    lossless  libwebp    900    900     627632   208    9.649
Lena_512.png                                    lossless  ours       900    900     627632   105    19.199
Lena_512.png                                    lossless  wasm       900    900     627632   51     39.455
Lena_512.png                                    lossless  x/image    900    900     627632   87     23.081
Lena_512.png                                    lossy     libwebp    900    900     89968    309    6.474
Lena_512.png                                    lossy     ours       900    900     89968    92     21.758
Lena_512.png                                    lossy     wasm       900    900     89968    77     26.053
Lena_512.png                                    lossy     x/image    900    900     89968    83     24.258
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp    2025   2700    3241976  40     50.876
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours       2025   2700    3241976  17     117.651
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm       2025   2700    3241976  8      251.238
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  x/image    2025   2700    3241976  15     135.414
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp    2025   2700    610518   44     46.006
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours       2025   2700    610518   14     150.958
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm       2025   2700    610518   11     183.466
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     x/image    2025   2700    610518   12     168.832
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp    2025   2700    2943528  40     50.437
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours       2025   2700    2943528  19     110.706
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm       2025   2700    2943528  9      247.181
pexels-martin-alargent-1165956-5665465.jpg      lossless  x/image    2025   2700    2943528  16     131.572
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp    2025   2700    603264   45     45.084
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours       2025   2700    603264   14     147.439
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm       2025   2700    603264   12     176.742
pexels-martin-alargent-1165956-5665465.jpg      lossy     x/image    2025   2700    603264   13     162.343
pexels-mavihnt-38213559.jpg                     lossless  libwebp    2560   1706    3482480  43     46.721
pexels-mavihnt-38213559.jpg                     lossless  ours       2560   1706    3482480  21     95.888
pexels-mavihnt-38213559.jpg                     lossless  wasm       2560   1706    3482480  10     210.976
pexels-mavihnt-38213559.jpg                     lossless  x/image    2560   1706    3482480  18     117.567
pexels-mavihnt-38213559.jpg                     lossy     libwebp    2560   1706    925032   39     51.930
pexels-mavihnt-38213559.jpg                     lossy     ours       2560   1706    925032   16     130.746
pexels-mavihnt-38213559.jpg                     lossy     wasm       2560   1706    925032   13     157.976
pexels-mavihnt-38213559.jpg                     lossy     x/image    2560   1706    925032   14     146.848
pexels-steve-15267299.jpg                       lossless  libwebp    2095   3000    2104690  40     50.988
pexels-steve-15267299.jpg                       lossless  ours       2095   3000    2104690  16     125.463
pexels-steve-15267299.jpg                       lossless  wasm       2095   3000    2104690  8      264.620
pexels-steve-15267299.jpg                       lossless  x/image    2095   3000    2104690  14     144.306
pexels-steve-15267299.jpg                       lossy     libwebp    2095   3000    247610   65     30.796
pexels-steve-15267299.jpg                       lossy     ours       2095   3000    247610   15     136.235
pexels-steve-15267299.jpg                       lossy     wasm       2095   3000    247610   12     169.643
pexels-steve-15267299.jpg                       lossy     x/image    2095   3000    247610   14     151.540
pexels-steve-29626041.jpg                       lossless  libwebp    2560   1440    283988   130    15.468
pexels-steve-29626041.jpg                       lossless  ours       2560   1440    283988   45     45.285
pexels-steve-29626041.jpg                       lossless  wasm       2560   1440    283988   17     121.297
pexels-steve-29626041.jpg                       lossless  x/image    2560   1440    283988   34     59.657
pexels-steve-29626041.jpg                       lossy     libwebp    2560   1440    38252    179    11.228
pexels-steve-29626041.jpg                       lossy     ours       2560   1440    38252    34     60.229
pexels-steve-29626041.jpg                       lossy     wasm       2560   1440    38252    25     81.710
pexels-steve-29626041.jpg                       lossy     x/image    2560   1440    38252    29     69.925
pexels-toulouse-10807703.jpg                    lossless  libwebp    1400   2100    2688812  56     35.880
pexels-toulouse-10807703.jpg                    lossless  ours       1400   2100    2688812  29     69.151
pexels-toulouse-10807703.jpg                    lossless  wasm       1400   2100    2688812  14     144.787
pexels-toulouse-10807703.jpg                    lossless  x/image    1400   2100    2688812  23     88.610
pexels-toulouse-10807703.jpg                    lossy     libwebp    1400   2100    758466   48     41.737
pexels-toulouse-10807703.jpg                    lossy     ours       1400   2100    758466   21     99.752
pexels-toulouse-10807703.jpg                    lossy     wasm       1400   2100    758466   18     116.677
pexels-toulouse-10807703.jpg                    lossy     x/image    1400   2100    758466   19     109.494
```

Effort sweep, every setting of every engine:

```
file                                            mode      engine      effort  width  height  bytes     psnr_db  iters  ms_per_op
Lena_512.png                                    lossless  libwebp     0       900    900     734910    -        45     22.457
Lena_512.png                                    lossless  libwebp     1       900    900     661194    -        9      118.294
Lena_512.png                                    lossless  libwebp     2       900    900     625874    -        7      155.947
Lena_512.png                                    lossless  libwebp     3       900    900     628862    -        6      183.509
Lena_512.png                                    lossless  libwebp     4       900    900     628612    -        6      189.926
Lena_512.png                                    lossless  libwebp     5       900    900     628612    -        6      193.152
Lena_512.png                                    lossless  libwebp     6       900    900     627632    -        4      263.503
Lena_512.png                                    lossless  libwebp     7       900    900     625628    -        3      372.312
Lena_512.png                                    lossless  libwebp     8       900    900     616842    -        2      665.653
Lena_512.png                                    lossless  libwebp     9       900    900     609124    -        1      3950.902
Lena_512.png                                    lossless  nativewebp  0       900    900     741562    -        4      265.953
Lena_512.png                                    lossless  nativewebp  4       900    900     739400    -        4      276.079
Lena_512.png                                    lossless  nativewebp  6       900    900     738176    -        4      321.496
Lena_512.png                                    lossless  ours        0       900    900     1707902   -        20     50.984
Lena_512.png                                    lossless  ours        1       900    900     1481022   -        15     69.572
Lena_512.png                                    lossless  ours        2       900    900     754084    -        4      261.177
Lena_512.png                                    lossless  ours        3       900    900     698252    -        4      283.926
Lena_512.png                                    lossless  ours        4       900    900     697206    -        3      406.047
Lena_512.png                                    lossless  ours        5       900    900     697926    -        3      501.742
Lena_512.png                                    lossless  ours        6       900    900     651634    -        2      678.463
Lena_512.png                                    lossless  ours        7       900    900     650456    -        1      13114.273
Lena_512.png                                    lossless  ours        8       900    900     650456    -        1      13132.930
Lena_512.png                                    lossless  ours        9       900    900     650456    -        1      13085.549
Lena_512.png                                    lossless  wasm        0       900    900     731954    -        8      141.759
Lena_512.png                                    lossless  wasm        1       900    900     639584    -        2      642.475
Lena_512.png                                    lossless  wasm        2       900    900     627632    -        2      733.833
Lena_512.png                                    lossless  wasm        3       900    900     627632    -        2      714.259
Lena_512.png                                    lossless  wasm        4       900    900     627632    -        2      733.229
Lena_512.png                                    lossless  wasm        5       900    900     618010    -        1      1649.966
Lena_512.png                                    lossless  wasm        6       900    900     622766    -        1      1530.800
Lena_512.png                                    lossy     libwebp     0       900    900     103980    41.04    59     17.212
Lena_512.png                                    lossy     libwebp     1       900    900     102500    41.05    44     22.786
Lena_512.png                                    lossy     libwebp     2       900    900     94342     40.70    43     23.470
Lena_512.png                                    lossy     libwebp     3       900    900     91788     40.98    20     52.229
Lena_512.png                                    lossy     libwebp     4       900    900     92124     40.97    20     52.432
Lena_512.png                                    lossy     libwebp     5       900    900     91586     40.91    18     58.620
Lena_512.png                                    lossy     libwebp     6       900    900     89968     40.94    10     108.574
Lena_512.png                                    lossy     ours        0       900    900     113182    41.02    35     29.166
Lena_512.png                                    lossy     ours        1       900    900     111076    40.68    24     41.752
Lena_512.png                                    lossy     ours        2       900    900     111076    40.68    25     41.534
Lena_512.png                                    lossy     ours        3       900    900     102242    40.80    18     56.630
Lena_512.png                                    lossy     ours        4       900    900     102242    40.80    18     56.496
Lena_512.png                                    lossy     ours        5       900    900     95932     39.93    17     58.901
Lena_512.png                                    lossy     ours        6       900    900     91360     39.89    16     65.558
Lena_512.png                                    lossy     ours        7       900    900     91360     39.89    16     65.553
Lena_512.png                                    lossy     ours        8       900    900     90952     40.05    10     107.416
Lena_512.png                                    lossy     ours        9       900    900     88890     40.16    6      198.316
Lena_512.png                                    lossy     wasm        0       900    900     103980    41.04    15     67.879
Lena_512.png                                    lossy     wasm        1       900    900     102500    41.05    11     95.656
Lena_512.png                                    lossy     wasm        2       900    900     94342     40.70    11     97.860
Lena_512.png                                    lossy     wasm        3       900    900     91788     40.98    4      283.285
Lena_512.png                                    lossy     wasm        4       900    900     92124     40.97    4      284.522
Lena_512.png                                    lossy     wasm        5       900    900     91586     40.91    4      309.132
Lena_512.png                                    lossy     wasm        6       900    900     89968     40.94    3      367.302
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     0       2025   2700    3987548   -        5      211.878
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     1       2025   2700    3992712   -        2      768.669
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     2       2025   2700    3993532   -        2      883.373
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     3       2025   2700    3246280   -        1      1132.627
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     4       2025   2700    3246304   -        1      1289.779
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     5       2025   2700    3242976   -        1      1382.644
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     6       2025   2700    3241976   -        1      2038.527
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     7       2025   2700    3237862   -        1      3292.485
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     8       2025   2700    3246580   -        1      3586.661
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp     9       2025   2700    3245966   -        1      21633.525
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  nativewebp  0       2025   2700    3944492   -        1      2647.792
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  nativewebp  4       2025   2700    3938686   -        1      2696.713
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  nativewebp  6       2025   2700    3926412   -        1      3055.252
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        0       2025   2700    13935672  -        3      346.367
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        1       2025   2700    11352054  -        1      1715.136
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        2       2025   2700    3977946   -        1      2703.528
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        3       2025   2700    3359674   -        1      2636.771
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        4       2025   2700    3359902   -        1      3853.284
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        5       2025   2700    3365290   -        1      5243.713
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        6       2025   2700    3355646   -        1      6166.840
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        7       2025   2700    3351552   -        1      42109.034
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        8       2025   2700    3351552   -        1      40035.129
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours        9       2025   2700    3351552   -        1      40171.670
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        0       2025   2700    3961206   -        1      1941.593
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        1       2025   2700    3244586   -        1      4828.285
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        2       2025   2700    3244586   -        1      4804.701
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        3       2025   2700    3244586   -        1      4797.235
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        4       2025   2700    3241976   -        1      5108.566
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        5       2025   2700    3249798   -        1      6190.181
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm        6       2025   2700    3249798   -        1      5787.317
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     0       2025   2700    598034    41.96    9      113.715
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     1       2025   2700    588954    41.97    7      151.251
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     2       2025   2700    614528    42.20    6      173.914
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     3       2025   2700    627994    42.39    3      388.781
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     4       2025   2700    632636    42.42    3      391.163
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     5       2025   2700    625540    42.27    3      449.349
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp     6       2025   2700    610518    42.58    2      967.645
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        0       2025   2700    723064    42.02    6      195.566
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        1       2025   2700    547196    40.42    4      262.452
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        2       2025   2700    547196    40.42    4      267.344
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        3       2025   2700    537122    40.76    3      465.032
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        4       2025   2700    537122    40.76    3      464.684
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        5       2025   2700    528042    40.65    3      476.086
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        6       2025   2700    468046    40.53    2      516.115
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        7       2025   2700    468046    40.53    2      515.187
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        8       2025   2700    456656    40.62    2      844.882
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours        9       2025   2700    592722    42.26    1      1673.067
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        0       2025   2700    598034    41.96    3      446.457
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        1       2025   2700    588954    41.97    2      631.058
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        2       2025   2700    614528    42.20    2      733.417
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        3       2025   2700    627994    42.39    1      2027.791
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        4       2025   2700    632636    42.42    1      2030.998
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        5       2025   2700    625540    42.27    1      2216.853
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm        6       2025   2700    610518    42.58    1      2870.860
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     0       2025   2700    3673696   -        4      250.952
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     1       2025   2700    3494146   -        2      882.177
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     2       2025   2700    3493206   -        1      1024.970
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     3       2025   2700    2949858   -        1      1241.681
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     4       2025   2700    2950040   -        1      1396.685
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     5       2025   2700    2943056   -        1      1515.945
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     6       2025   2700    2943528   -        1      2175.759
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     7       2025   2700    2929290   -        1      3038.677
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     8       2025   2700    2938176   -        1      3553.782
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp     9       2025   2700    2940364   -        1      26527.454
pexels-martin-alargent-1165956-5665465.jpg      lossless  nativewebp  0       2025   2700    3554880   -        1      2232.369
pexels-martin-alargent-1165956-5665465.jpg      lossless  nativewebp  4       2025   2700    3559008   -        1      2345.997
pexels-martin-alargent-1165956-5665465.jpg      lossless  nativewebp  6       2025   2700    3563584   -        1      2578.729
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        0       2025   2700    13256232  -        3      335.851
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        1       2025   2700    10676794  -        1      1433.761
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        2       2025   2700    4054172   -        1      2206.765
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        3       2025   2700    3341756   -        1      2115.539
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        4       2025   2700    3318656   -        1      3011.203
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        5       2025   2700    3287048   -        1      3851.692
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        6       2025   2700    3058988   -        1      4767.327
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        7       2025   2700    3003328   -        1      27512.203
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        8       2025   2700    3003328   -        1      27405.838
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours        9       2025   2700    3003328   -        1      27446.989
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        0       2025   2700    3625218   -        1      1717.460
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        1       2025   2700    2948150   -        1      4672.591
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        2       2025   2700    2948150   -        1      4618.100
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        3       2025   2700    2948150   -        1      4616.510
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        4       2025   2700    2943528   -        1      5079.014
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        5       2025   2700    2953798   -        1      6411.437
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm        6       2025   2700    2953798   -        1      5701.768
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     0       2025   2700    726824    42.63    9      117.893
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     1       2025   2700    664274    42.64    7      154.028
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     2       2025   2700    626708    42.42    6      173.088
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     3       2025   2700    616320    42.96    3      371.785
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     4       2025   2700    618856    42.83    3      370.572
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     5       2025   2700    615274    42.71    3      418.698
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp     6       2025   2700    603264    42.75    2      808.123
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        0       2025   2700    756806    42.39    6      194.530
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        1       2025   2700    686954    41.34    4      270.423
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        2       2025   2700    686954    41.34    4      275.552
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        3       2025   2700    617046    41.56    3      396.015
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        4       2025   2700    617046    41.56    3      393.531
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        5       2025   2700    603032    41.39    3      405.558
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        6       2025   2700    570570    41.33    3      449.563
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        7       2025   2700    570570    41.33    3      452.817
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        8       2025   2700    570042    41.64    2      804.077
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours        9       2025   2700    601454    42.73    1      1380.822
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        0       2025   2700    726824    42.63    3      451.857
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        1       2025   2700    664274    42.64    2      656.232
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        2       2025   2700    626708    42.42    2      735.324
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        3       2025   2700    616320    42.96    1      1930.410
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        4       2025   2700    618856    42.83    1      1932.289
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        5       2025   2700    615274    42.71    1      2098.265
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm        6       2025   2700    603264    42.75    1      2561.293
pexels-mavihnt-38213559.jpg                     lossless  libwebp     0       2560   1706    4156742   -        7      152.484
pexels-mavihnt-38213559.jpg                     lossless  libwebp     1       2560   1706    3973816   -        2      585.454
pexels-mavihnt-38213559.jpg                     lossless  libwebp     2       2560   1706    3988280   -        2      668.872
pexels-mavihnt-38213559.jpg                     lossless  libwebp     3       2560   1706    3487548   -        2      930.893
pexels-mavihnt-38213559.jpg                     lossless  libwebp     4       2560   1706    3488224   -        1      1018.194
pexels-mavihnt-38213559.jpg                     lossless  libwebp     5       2560   1706    3482892   -        1      1127.196
pexels-mavihnt-38213559.jpg                     lossless  libwebp     6       2560   1706    3482480   -        1      1659.737
pexels-mavihnt-38213559.jpg                     lossless  libwebp     7       2560   1706    3481722   -        1      2669.482
pexels-mavihnt-38213559.jpg                     lossless  libwebp     8       2560   1706    3485234   -        1      3047.604
pexels-mavihnt-38213559.jpg                     lossless  libwebp     9       2560   1706    3485216   -        1      15237.039
pexels-mavihnt-38213559.jpg                     lossless  nativewebp  0       2560   1706    4004276   -        1      1982.136
pexels-mavihnt-38213559.jpg                     lossless  nativewebp  4       2560   1706    4007402   -        1      1979.819
pexels-mavihnt-38213559.jpg                     lossless  nativewebp  6       2560   1706    4009534   -        1      2131.436
pexels-mavihnt-38213559.jpg                     lossless  ours        0       2560   1706    11064720  -        4      284.398
pexels-mavihnt-38213559.jpg                     lossless  ours        1       2560   1706    9463456   -        1      1103.050
pexels-mavihnt-38213559.jpg                     lossless  ours        2       2560   1706    4986210   -        1      2201.636
pexels-mavihnt-38213559.jpg                     lossless  ours        3       2560   1706    3768140   -        1      2165.176
pexels-mavihnt-38213559.jpg                     lossless  ours        4       2560   1706    3730772   -        1      2946.547
pexels-mavihnt-38213559.jpg                     lossless  ours        5       2560   1706    3733318   -        1      3659.612
pexels-mavihnt-38213559.jpg                     lossless  ours        6       2560   1706    3585610   -        1      4445.789
pexels-mavihnt-38213559.jpg                     lossless  ours        7       2560   1706    3575284   -        1      24417.866
pexels-mavihnt-38213559.jpg                     lossless  ours        8       2560   1706    3575284   -        1      24347.230
pexels-mavihnt-38213559.jpg                     lossless  ours        9       2560   1706    3575284   -        1      24446.041
pexels-mavihnt-38213559.jpg                     lossless  wasm        0       2560   1706    4116810   -        1      1381.718
pexels-mavihnt-38213559.jpg                     lossless  wasm        1       2560   1706    3484606   -        1      3865.454
pexels-mavihnt-38213559.jpg                     lossless  wasm        2       2560   1706    3484606   -        1      3771.118
pexels-mavihnt-38213559.jpg                     lossless  wasm        3       2560   1706    3484606   -        1      3820.526
pexels-mavihnt-38213559.jpg                     lossless  wasm        4       2560   1706    3482480   -        1      4161.897
pexels-mavihnt-38213559.jpg                     lossless  wasm        5       2560   1706    3488584   -        1      5101.064
pexels-mavihnt-38213559.jpg                     lossless  wasm        6       2560   1706    3488584   -        1      4725.738
pexels-mavihnt-38213559.jpg                     lossy     libwebp     0       2560   1706    983360    40.87    9      120.410
pexels-mavihnt-38213559.jpg                     lossy     libwebp     1       2560   1706    976610    40.88    7      154.952
pexels-mavihnt-38213559.jpg                     lossy     libwebp     2       2560   1706    935824    40.67    6      168.372
pexels-mavihnt-38213559.jpg                     lossy     libwebp     3       2560   1706    931074    41.15    3      343.501
pexels-mavihnt-38213559.jpg                     lossy     libwebp     4       2560   1706    934782    41.17    3      344.051
pexels-mavihnt-38213559.jpg                     lossy     libwebp     5       2560   1706    933892    41.05    3      402.555
pexels-mavihnt-38213559.jpg                     lossy     libwebp     6       2560   1706    925032    41.14    2      962.201
pexels-mavihnt-38213559.jpg                     lossy     ours        0       2560   1706    1037210   40.99    5      208.496
pexels-mavihnt-38213559.jpg                     lossy     ours        1       2560   1706    889590    39.11    4      271.611
pexels-mavihnt-38213559.jpg                     lossy     ours        2       2560   1706    889590    39.11    4      270.909
pexels-mavihnt-38213559.jpg                     lossy     ours        3       2560   1706    859500    39.31    3      448.473
pexels-mavihnt-38213559.jpg                     lossy     ours        4       2560   1706    859500    39.31    3      447.457
pexels-mavihnt-38213559.jpg                     lossy     ours        5       2560   1706    848646    39.18    3      454.986
pexels-mavihnt-38213559.jpg                     lossy     ours        6       2560   1706    798104    38.97    2      511.434
pexels-mavihnt-38213559.jpg                     lossy     ours        7       2560   1706    798104    38.97    2      514.091
pexels-mavihnt-38213559.jpg                     lossy     ours        8       2560   1706    784808    38.99    2      828.983
pexels-mavihnt-38213559.jpg                     lossy     ours        9       2560   1706    935652    41.05    1      1597.855
pexels-mavihnt-38213559.jpg                     lossy     wasm        0       2560   1706    983360    40.87    3      411.863
pexels-mavihnt-38213559.jpg                     lossy     wasm        1       2560   1706    976610    40.88    2      575.159
pexels-mavihnt-38213559.jpg                     lossy     wasm        2       2560   1706    935824    40.67    2      647.418
pexels-mavihnt-38213559.jpg                     lossy     wasm        3       2560   1706    931074    41.15    1      1790.304
pexels-mavihnt-38213559.jpg                     lossy     wasm        4       2560   1706    934782    41.17    1      1784.086
pexels-mavihnt-38213559.jpg                     lossy     wasm        5       2560   1706    933892    41.05    1      1955.120
pexels-mavihnt-38213559.jpg                     lossy     wasm        6       2560   1706    925032    41.14    1      2686.912
pexels-steve-15267299.jpg                       lossless  libwebp     0       2095   3000    2603112   -        5      211.017
pexels-steve-15267299.jpg                       lossless  libwebp     1       2095   3000    2557750   -        2      896.366
pexels-steve-15267299.jpg                       lossless  libwebp     2       2095   3000    2550344   -        1      1081.472
pexels-steve-15267299.jpg                       lossless  libwebp     3       2095   3000    2104000   -        1      1369.623
pexels-steve-15267299.jpg                       lossless  libwebp     4       2095   3000    2104010   -        1      1501.137
pexels-steve-15267299.jpg                       lossless  libwebp     5       2095   3000    2104010   -        1      1638.842
pexels-steve-15267299.jpg                       lossless  libwebp     6       2095   3000    2104690   -        1      2208.481
pexels-steve-15267299.jpg                       lossless  libwebp     7       2095   3000    2096804   -        1      2800.042
pexels-steve-15267299.jpg                       lossless  libwebp     8       2095   3000    2111270   -        1      3944.192
pexels-steve-15267299.jpg                       lossless  libwebp     9       2095   3000    2111200   -        1      22335.527
pexels-steve-15267299.jpg                       lossless  nativewebp  0       2095   3000    2635282   -        1      2563.453
pexels-steve-15267299.jpg                       lossless  nativewebp  4       2095   3000    2627962   -        1      2666.988
pexels-steve-15267299.jpg                       lossless  nativewebp  6       2095   3000    2622528   -        1      3028.670
pexels-steve-15267299.jpg                       lossless  ours        0       2095   3000    8710684   -        4      275.161
pexels-steve-15267299.jpg                       lossless  ours        1       2095   3000    7739368   -        2      804.799
pexels-steve-15267299.jpg                       lossless  ours        2       2095   3000    2499600   -        1      1630.402
pexels-steve-15267299.jpg                       lossless  ours        3       2095   3000    2316408   -        1      1783.432
pexels-steve-15267299.jpg                       lossless  ours        4       2095   3000    2309372   -        1      2573.262
pexels-steve-15267299.jpg                       lossless  ours        5       2095   3000    2253044   -        1      3436.459
pexels-steve-15267299.jpg                       lossless  ours        6       2095   3000    2218786   -        1      4459.917
pexels-steve-15267299.jpg                       lossless  ours        7       2095   3000    2146874   -        1      178883.876
pexels-steve-15267299.jpg                       lossless  ours        8       2095   3000    2146874   -        1      178719.235
pexels-steve-15267299.jpg                       lossless  ours        9       2095   3000    2146874   -        1      179389.246
pexels-steve-15267299.jpg                       lossless  wasm        0       2095   3000    2546112   -        1      1420.540
pexels-steve-15267299.jpg                       lossless  wasm        1       2095   3000    2103668   -        1      4967.641
pexels-steve-15267299.jpg                       lossless  wasm        2       2095   3000    2103668   -        1      4959.407
pexels-steve-15267299.jpg                       lossless  wasm        3       2095   3000    2103668   -        1      4968.979
pexels-steve-15267299.jpg                       lossless  wasm        4       2095   3000    2104690   -        1      5349.134
pexels-steve-15267299.jpg                       lossless  wasm        5       2095   3000    2119370   -        1      6492.886
pexels-steve-15267299.jpg                       lossless  wasm        6       2095   3000    2119370   -        1      6000.443
pexels-steve-15267299.jpg                       lossy     libwebp     0       2095   3000    284570    44.34    10     106.517
pexels-steve-15267299.jpg                       lossy     libwebp     1       2095   3000    275444    44.38    8      141.545
pexels-steve-15267299.jpg                       lossy     libwebp     2       2095   3000    261602    44.30    8      142.393
pexels-steve-15267299.jpg                       lossy     libwebp     3       2095   3000    255364    44.59    3      368.748
pexels-steve-15267299.jpg                       lossy     libwebp     4       2095   3000    256038    44.58    3      342.385
pexels-steve-15267299.jpg                       lossy     libwebp     5       2095   3000    253192    44.51    3      372.686
pexels-steve-15267299.jpg                       lossy     libwebp     6       2095   3000    247610    44.50    2      518.133
pexels-steve-15267299.jpg                       lossy     ours        0       2095   3000    294606    43.91    7      153.489
pexels-steve-15267299.jpg                       lossy     ours        1       2095   3000    277974    43.52    5      235.989
pexels-steve-15267299.jpg                       lossy     ours        2       2095   3000    277974    43.52    5      229.510
pexels-steve-15267299.jpg                       lossy     ours        3       2095   3000    249954    43.62    4      309.387
pexels-steve-15267299.jpg                       lossy     ours        4       2095   3000    249954    43.62    4      317.865
pexels-steve-15267299.jpg                       lossy     ours        5       2095   3000    232944    43.31    4      324.967
pexels-steve-15267299.jpg                       lossy     ours        6       2095   3000    216996    43.29    3      347.654
pexels-steve-15267299.jpg                       lossy     ours        7       2095   3000    216996    43.29    3      347.093
pexels-steve-15267299.jpg                       lossy     ours        8       2095   3000    212368    43.36    2      523.865
pexels-steve-15267299.jpg                       lossy     ours        9       2095   3000    233446    44.04    1      1148.076
pexels-steve-15267299.jpg                       lossy     wasm        0       2095   3000    284570    44.34    3      463.864
pexels-steve-15267299.jpg                       lossy     wasm        1       2095   3000    275444    44.38    2      662.744
pexels-steve-15267299.jpg                       lossy     wasm        2       2095   3000    261602    44.30    2      636.860
pexels-steve-15267299.jpg                       lossy     wasm        3       2095   3000    255364    44.59    1      1865.931
pexels-steve-15267299.jpg                       lossy     wasm        4       2095   3000    256038    44.58    1      1912.706
pexels-steve-15267299.jpg                       lossy     wasm        5       2095   3000    253192    44.51    1      2039.194
pexels-steve-15267299.jpg                       lossy     wasm        6       2095   3000    247610    44.50    1      2132.964
pexels-steve-29626041.jpg                       lossless  libwebp     0       2560   1440    367064    -        17     59.011
pexels-steve-29626041.jpg                       lossless  libwebp     1       2560   1440    360562    -        3      400.303
pexels-steve-29626041.jpg                       lossless  libwebp     2       2560   1440    309376    -        2      523.203
pexels-steve-29626041.jpg                       lossless  libwebp     3       2560   1440    292034    -        2      552.099
pexels-steve-29626041.jpg                       lossless  libwebp     4       2560   1440    287166    -        2      576.611
pexels-steve-29626041.jpg                       lossless  libwebp     5       2560   1440    287662    -        2      615.374
pexels-steve-29626041.jpg                       lossless  libwebp     6       2560   1440    283988    -        2      727.847
pexels-steve-29626041.jpg                       lossless  libwebp     7       2560   1440    282342    -        2      772.638
pexels-steve-29626041.jpg                       lossless  libwebp     8       2560   1440    294462    -        1      1029.027
pexels-steve-29626041.jpg                       lossless  libwebp     9       2560   1440    292264    -        1      3991.303
pexels-steve-29626041.jpg                       lossless  nativewebp  0       2560   1440    378544    -        1      1109.831
pexels-steve-29626041.jpg                       lossless  nativewebp  4       2560   1440    378536    -        1      1163.211
pexels-steve-29626041.jpg                       lossless  nativewebp  6       2560   1440    379050    -        1      1327.874
pexels-steve-29626041.jpg                       lossless  ours        0       2560   1440    827088    -        12     86.957
pexels-steve-29626041.jpg                       lossless  ours        1       2560   1440    532928    -        10     102.343
pexels-steve-29626041.jpg                       lossless  ours        2       2560   1440    356676    -        3      457.693
pexels-steve-29626041.jpg                       lossless  ours        3       2560   1440    335034    -        2      546.543
pexels-steve-29626041.jpg                       lossless  ours        4       2560   1440    327568    -        2      812.304
pexels-steve-29626041.jpg                       lossless  ours        5       2560   1440    309738    -        1      1048.105
pexels-steve-29626041.jpg                       lossless  ours        6       2560   1440    309370    -        1      1452.851
pexels-steve-29626041.jpg                       lossless  ours        7       2560   1440    286174    -        1      320676.952
pexels-steve-29626041.jpg                       lossless  ours        8       2560   1440    286174    -        1      319207.237
pexels-steve-29626041.jpg                       lossless  ours        9       2560   1440    286174    -        1      318834.289
pexels-steve-29626041.jpg                       lossless  wasm        0       2560   1440    346824    -        3      355.226
pexels-steve-29626041.jpg                       lossless  wasm        1       2560   1440    283552    -        1      1989.403
pexels-steve-29626041.jpg                       lossless  wasm        2       2560   1440    283552    -        1      2025.324
pexels-steve-29626041.jpg                       lossless  wasm        3       2560   1440    283552    -        1      1968.737
pexels-steve-29626041.jpg                       lossless  wasm        4       2560   1440    283988    -        1      2192.395
pexels-steve-29626041.jpg                       lossless  wasm        5       2560   1440    296596    -        1      2865.509
pexels-steve-29626041.jpg                       lossless  wasm        6       2560   1440    296596    -        1      2867.058
pexels-steve-29626041.jpg                       lossy     libwebp     0       2560   1440    47614     49.05    20     50.466
pexels-steve-29626041.jpg                       lossy     libwebp     1       2560   1440    47302     49.05    15     70.023
pexels-steve-29626041.jpg                       lossy     libwebp     2       2560   1440    39488     49.30    17     59.029
pexels-steve-29626041.jpg                       lossy     libwebp     3       2560   1440    39096     49.71    7      156.710
pexels-steve-29626041.jpg                       lossy     libwebp     4       2560   1440    39310     49.71    7      156.747
pexels-steve-29626041.jpg                       lossy     libwebp     5       2560   1440    38754     49.65    6      170.080
pexels-steve-29626041.jpg                       lossy     libwebp     6       2560   1440    38252     49.65    5      203.327
pexels-steve-29626041.jpg                       lossy     ours        0       2560   1440    45686     49.31    14     71.968
pexels-steve-29626041.jpg                       lossy     ours        1       2560   1440    46846     49.19    10     104.627
pexels-steve-29626041.jpg                       lossy     ours        2       2560   1440    46846     49.19    10     105.522
pexels-steve-29626041.jpg                       lossy     ours        3       2560   1440    40660     49.23    10     110.641
pexels-steve-29626041.jpg                       lossy     ours        4       2560   1440    40660     49.23    9      112.635
pexels-steve-29626041.jpg                       lossy     ours        5       2560   1440    38994     49.02    9      114.395
pexels-steve-29626041.jpg                       lossy     ours        6       2560   1440    36774     49.01    8      125.470
pexels-steve-29626041.jpg                       lossy     ours        7       2560   1440    36774     49.01    9      124.301
pexels-steve-29626041.jpg                       lossy     ours        8       2560   1440    36512     49.08    6      195.309
pexels-steve-29626041.jpg                       lossy     ours        9       2560   1440    42614     49.25    3      388.538
pexels-steve-29626041.jpg                       lossy     wasm        0       2560   1440    47614     49.05    5      248.403
pexels-steve-29626041.jpg                       lossy     wasm        1       2560   1440    47302     49.05    3      356.099
pexels-steve-29626041.jpg                       lossy     wasm        2       2560   1440    39488     49.30    4      289.682
pexels-steve-29626041.jpg                       lossy     wasm        3       2560   1440    39096     49.71    2      899.732
pexels-steve-29626041.jpg                       lossy     wasm        4       2560   1440    39310     49.71    2      904.373
pexels-steve-29626041.jpg                       lossy     wasm        5       2560   1440    38754     49.65    2      977.498
pexels-steve-29626041.jpg                       lossy     wasm        6       2560   1440    38252     49.65    2      962.649
pexels-toulouse-10807703.jpg                    lossless  libwebp     0       1400   2100    3044142   -        12     90.661
pexels-toulouse-10807703.jpg                    lossless  libwebp     1       1400   2100    2807602   -        3      462.051
pexels-toulouse-10807703.jpg                    lossless  libwebp     2       1400   2100    2780320   -        2      550.172
pexels-toulouse-10807703.jpg                    lossless  libwebp     3       1400   2100    2693662   -        2      607.379
pexels-toulouse-10807703.jpg                    lossless  libwebp     4       1400   2100    2693662   -        2      656.664
pexels-toulouse-10807703.jpg                    lossless  libwebp     5       1400   2100    2691936   -        2      764.021
pexels-toulouse-10807703.jpg                    lossless  libwebp     6       1400   2100    2688812   -        1      1152.017
pexels-toulouse-10807703.jpg                    lossless  libwebp     7       1400   2100    2688410   -        1      1401.460
pexels-toulouse-10807703.jpg                    lossless  libwebp     8       1400   2100    2686410   -        1      2066.676
pexels-toulouse-10807703.jpg                    lossless  libwebp     9       1400   2100    2685794   -        1      9021.670
pexels-toulouse-10807703.jpg                    lossless  nativewebp  0       1400   2100    2989804   -        1      1197.637
pexels-toulouse-10807703.jpg                    lossless  nativewebp  4       1400   2100    2992710   -        1      1185.632
pexels-toulouse-10807703.jpg                    lossless  nativewebp  6       1400   2100    2996214   -        1      1311.533
pexels-toulouse-10807703.jpg                    lossless  ours        0       1400   2100    7375806   -        6      185.408
pexels-toulouse-10807703.jpg                    lossless  ours        1       1400   2100    6193432   -        2      530.280
pexels-toulouse-10807703.jpg                    lossless  ours        2       1400   2100    3973924   -        1      1379.384
pexels-toulouse-10807703.jpg                    lossless  ours        3       1400   2100    2942470   -        1      1369.592
pexels-toulouse-10807703.jpg                    lossless  ours        4       1400   2100    2912778   -        1      1770.595
pexels-toulouse-10807703.jpg                    lossless  ours        5       1400   2100    2912610   -        1      2096.586
pexels-toulouse-10807703.jpg                    lossless  ours        6       1400   2100    2765508   -        1      2762.823
pexels-toulouse-10807703.jpg                    lossless  ours        7       1400   2100    2746828   -        1      31219.449
pexels-toulouse-10807703.jpg                    lossless  ours        8       1400   2100    2746828   -        1      31244.226
pexels-toulouse-10807703.jpg                    lossless  ours        9       1400   2100    2746828   -        1      31248.078
pexels-toulouse-10807703.jpg                    lossless  wasm        0       1400   2100    3029106   -        2      703.266
pexels-toulouse-10807703.jpg                    lossless  wasm        1       1400   2100    2690886   -        1      2919.642
pexels-toulouse-10807703.jpg                    lossless  wasm        2       1400   2100    2690886   -        1      2909.285
pexels-toulouse-10807703.jpg                    lossless  wasm        3       1400   2100    2690886   -        1      2914.293
pexels-toulouse-10807703.jpg                    lossless  wasm        4       1400   2100    2688812   -        1      3154.716
pexels-toulouse-10807703.jpg                    lossless  wasm        5       1400   2100    2685806   -        1      4808.299
pexels-toulouse-10807703.jpg                    lossless  wasm        6       1400   2100    2685806   -        1      3855.431
pexels-toulouse-10807703.jpg                    lossy     libwebp     0       1400   2100    857060    39.84    12     90.850
pexels-toulouse-10807703.jpg                    lossy     libwebp     1       1400   2100    812040    39.84    9      117.505
pexels-toulouse-10807703.jpg                    lossy     libwebp     2       1400   2100    796814    39.60    8      125.383
pexels-toulouse-10807703.jpg                    lossy     libwebp     3       1400   2100    783482    39.95    5      241.265
pexels-toulouse-10807703.jpg                    lossy     libwebp     4       1400   2100    784734    39.96    5      246.362
pexels-toulouse-10807703.jpg                    lossy     libwebp     5       1400   2100    768426    39.81    4      284.129
pexels-toulouse-10807703.jpg                    lossy     libwebp     6       1400   2100    758466    39.84    2      659.453
pexels-toulouse-10807703.jpg                    lossy     ours        0       1400   2100    854742    39.93    7      162.580
pexels-toulouse-10807703.jpg                    lossy     ours        1       1400   2100    763158    38.22    5      207.192
pexels-toulouse-10807703.jpg                    lossy     ours        2       1400   2100    763158    38.22    5      207.817
pexels-toulouse-10807703.jpg                    lossy     ours        3       1400   2100    732726    38.37    4      305.706
pexels-toulouse-10807703.jpg                    lossy     ours        4       1400   2100    732726    38.37    4      304.443
pexels-toulouse-10807703.jpg                    lossy     ours        5       1400   2100    719110    38.10    4      316.720
pexels-toulouse-10807703.jpg                    lossy     ours        6       1400   2100    689980    37.96    3      358.844
pexels-toulouse-10807703.jpg                    lossy     ours        7       1400   2100    689980    37.96    3      354.681
pexels-toulouse-10807703.jpg                    lossy     ours        8       1400   2100    686416    38.08    2      581.262
pexels-toulouse-10807703.jpg                    lossy     ours        9       1400   2100    761160    39.75    2      840.473
pexels-toulouse-10807703.jpg                    lossy     wasm        0       1400   2100    857060    39.84    4      296.520
pexels-toulouse-10807703.jpg                    lossy     wasm        1       1400   2100    812040    39.84    3      415.520
pexels-toulouse-10807703.jpg                    lossy     wasm        2       1400   2100    796814    39.60    3      442.832
pexels-toulouse-10807703.jpg                    lossy     wasm        3       1400   2100    783482    39.95    1      1238.681
pexels-toulouse-10807703.jpg                    lossy     wasm        4       1400   2100    784734    39.96    1      1240.047
pexels-toulouse-10807703.jpg                    lossy     wasm        5       1400   2100    768426    39.81    1      1362.749
pexels-toulouse-10807703.jpg                    lossy     wasm        6       1400   2100    758466    39.84    1      1872.753
```
