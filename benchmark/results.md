# Benchmark results

Captured runs of `benchmark/run.sh` and `benchmark/run-decode.sh` on two
machines. Timings are machine-dependent; regenerate locally for your own
hardware.

| | arm64 | amd64 |
| --- | --- | --- |
| **CPU** | Apple M4 Pro (14 cores) | AMD Ryzen 7 5700G (16 threads) |
| **OS** | macOS 26.5.1 | Arch Linux, kernel 7.1.3 |
| **Go** | go1.26.5 | go1.26.5 |
| **Rust** | 1.97.1 encode, 1.96.0 decode | 1.97.1 |

- **libwebp:** 1.6.0 on both (also used by the `wasm` engine, via WASM)
- **Encoding:** webp-rust v0.2.1 · library commit 0009929 · 2026-07-26
- **nativewebp rows:** v1.3.0 · library commit 6e35fbe · 2026-08-07
- **Decoding:** webp-rust v0.2.0 · library commit bcab3e6 · 2026-08-07
- **Budget:** 2000 ms/measurement, lossy quality 90

Engines: `ours` (pure Go), `libwebp` (C, cgo), `wasm` (libwebp via WASM, cgo-free),
`webp-rust` (the Rust original), `nativewebp` (HugoSmits86/nativewebp, pure Go,
lossless only), and in the decode tables `x/image` (golang.org/x/image/webp,
decode only). See `README.md` for the exact mode settings.

The two passes were captured on different dates, so the encode and decode tables
are not one run of one commit. Encoder output is unchanged between those two
commits (verified by hashing every mode of every test image), and the decode
pass adds a `webp-rust` version bump downwards: v0.2.1 is not tagged upstream, so
the decode capture uses the v0.2.0 tag on both machines.

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
- **`webp-rust` `lossy-fast` output is corrupt on 3 of 7 images**, a bug this
  port found and fixed. See "The webp-rust lossy-fast bug" below.
- **`webp-rust` `lossy-slow` files are smaller than everyone's, at 1.5-3 dB lower
  quality** (e.g. toulouse 560756 B at 36.23 dB vs our 761160 B at 39.75 dB).
  An earlier capture of these results read that size advantage as a compression
  win; the PSNR column shows it is over-quantization.

On memory:

- **Lossy, we are the lightest of the three Go options:** 0.66-0.95x libwebp's
  peak, and roughly 12-15 MiB per megapixel on the larger images. Encoding a 4K
  frame costs on the order of 130 MiB.
- **`wasm` costs 1.5-2.7x libwebp's peak**, the widest gap in the lossy modes: it
  carries a WebAssembly runtime and its own linear memory on top of the encode.
  That is the memory half of the cgo-free tradeoff, next to the 2-6x on time.
- **Lossless costs everyone several times what lossy does**, and we sit at
  1.7-2.4x libwebp's peak, up to 113 MiB per megapixel: 420-484 MiB on the 5.5+ MP
  test images against libwebp's 194-215 MiB. `wasm` (2.6-3.8x, 572-697 MiB) and
  `webp-rust` (1.8-3.5x, 482-725 MiB) are both above us.
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
- **vs `webp-rust`, the original this decoder was ported from: we are 1.3-1.6x
  slower on lossy and 1.4-3.7x faster on lossless** (0.27-0.69x its time). The
  lossy gap is ported code that lost ground, not an algorithmic difference; the
  lossless lead is the table-driven VP8L rewrite, which the Rust original does
  not have.

## Charts

![Size and quality against libwebp: one point per test image, with output size relative to libwebp on the x axis and PSNR difference on the y axis, faceted by lossy mode](charts/rate-distortion-light.svg#gh-light-mode-only)
![Size and quality against libwebp: one point per test image, with output size relative to libwebp on the x axis and PSNR difference on the y axis, faceted by lossy mode](charts/rate-distortion-dark.svg#gh-dark-mode-only)

![Encode time per image for each engine, one panel per mode and machine, with bars that run off the panel drawn fading out under an arrow](charts/encode-time-light.svg#gh-light-mode-only)
![Encode time per image for each engine, one panel per mode and machine, with bars that run off the panel drawn fading out under an arrow](charts/encode-time-dark.svg#gh-dark-mode-only)

![Decode time per image for each engine, one panel per mode and machine, on the same bar layout as the encode time figure, with x/image added](charts/decode-time-light.svg#gh-light-mode-only)
![Decode time per image for each engine, one panel per mode and machine, on the same bar layout as the encode time figure, with x/image added](charts/decode-time-dark.svg#gh-dark-mode-only)

![Peak memory per megapixel for each engine, one panel per mode and machine, on the same bar layout as the encode time figure](charts/peak-memory-light.svg#gh-light-mode-only)
![Peak memory per megapixel for each engine, one panel per mode and machine, on the same bar layout as the encode time figure](charts/peak-memory-dark.svg#gh-dark-mode-only)

Regenerate them from the tables below with `benchmark/chart/chart.go`.

## The webp-rust lossy-fast bug

Porting `webp-rust` to Go and testing the result turned up a bug in the original
that its own test suite did not catch: at `lossy-fast` (effort 0), it can write
WebP files that no decoder can read back correctly.

Three of the seven test images hit it:

| image | webp-rust | ours | libwebp |
| --- | --- | --- | --- |
| toulouse | **6.35 dB** | 39.93 dB | 39.84 dB |
| martin-alargent | **9.38 dB** | 42.39 dB | 42.63 dB |
| abubakar-mamman | **16.44 dB** | 42.02 dB | 41.96 dB |
| the other four | 39.7-49.0 dB | 40.9-49.3 dB | 40.9-49.1 dB |

Higher PSNR is better: a normal encode at quality 90 lands near 40 dB, so
`webp-rust`'s 6.35 dB on toulouse is a garbled image rather than a tighter file.

We found it through the PSNR column. Encoding each image, decoding it back and
scoring the result against the source turned up three files that came back
garbled, which size alone had never shown. Decoding those files with `webp-rust`'s
own decoder and with ours gave figures agreeing to the hundredth of a dB,
including on the images that encode fine, which pointed at the encoder rather
than at either decoder.

This port inherited the same bug and fixed it in commit 7e5e084, with a
regression test covering the case. The bug is still present in `webp-rust`
v0.2.1.

## arm64 (Apple M4 Pro)

```
file                                            mode        engine      width  height  bytes    psnr_db  iters  ms_per_op
Lena_512.png                                    lossless    libwebp     900    900     627632   -        9      243.325
Lena_512.png                                    lossless    nativewebp  900    900     738176   -        11     185.640
Lena_512.png                                    lossless    ours        900    900     651634   -        5      447.615
Lena_512.png                                    lossless    wasm        900    900     622766   -        3      920.717
Lena_512.png                                    lossless    webp-rust   900    900     651036   -        2      2120.281
Lena_512.png                                    lossy-fast  libwebp     900    900     103980   41.04    133    15.097
Lena_512.png                                    lossy-fast  ours        900    900     113182   41.02    90     22.266
Lena_512.png                                    lossy-fast  wasm        900    900     103980   41.04    54     37.452
Lena_512.png                                    lossy-fast  webp-rust   900    900     127376   40.09    173    11.603
Lena_512.png                                    lossy-slow  libwebp     900    900     89968    40.94    27     75.090
Lena_512.png                                    lossy-slow  ours        900    900     88890    40.16    14     150.384
Lena_512.png                                    lossy-slow  wasm        900    900     89968    40.94    11     196.100
Lena_512.png                                    lossy-slow  webp-rust   900    900     65024    37.62    2      8312.755
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    libwebp     2025   2700    3241976  -        2      1921.796
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    nativewebp  2025   2700    3926412  -        1      2203.418
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    ours        2025   2700    3355646  -        1      4260.643
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    wasm        2025   2700    3249798  -        1      3673.480
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    webp-rust   2025   2700    3468390  -        2      23492.734
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  libwebp     2025   2700    598034   41.96    21     97.182
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  ours        2025   2700    723064   42.02    14     151.822
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  wasm        2025   2700    598034   41.96    9      245.926
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  webp-rust   2025   2700    641754   16.44    27     75.511
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  libwebp     2025   2700    610518   42.58    4      660.263
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  ours        2025   2700    592722   42.26    2      1414.258
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  wasm        2025   2700    610518   42.58    2      1582.991
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  webp-rust   2025   2700    283348   38.71    2      36444.822
pexels-martin-alargent-1165956-5665465.jpg      lossless    libwebp     2025   2700    2943528  -        2      1957.118
pexels-martin-alargent-1165956-5665465.jpg      lossless    nativewebp  2025   2700    3563584  -        2      1636.355
pexels-martin-alargent-1165956-5665465.jpg      lossless    ours        2025   2700    3058988  -        1      3447.409
pexels-martin-alargent-1165956-5665465.jpg      lossless    wasm        2025   2700    2953798  -        1      3711.016
pexels-martin-alargent-1165956-5665465.jpg      lossless    webp-rust   2025   2700    3186444  -        2      17833.623
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  libwebp     2025   2700    726824   42.63    20     100.425
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  ours        2025   2700    756806   42.39    14     151.496
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  wasm        2025   2700    726824   42.63    9      244.538
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  webp-rust   2025   2700    826398   9.38     26     78.302
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  libwebp     2025   2700    603264   42.75    4      560.010
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  ours        2025   2700    601454   42.73    2      1046.475
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  wasm        2025   2700    603264   42.75    2      1391.643
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  webp-rust   2025   2700    419784   39.67    2      54700.729
pexels-mavihnt-38213559.jpg                     lossless    libwebp     2560   1706    3482480  -        2      1518.171
pexels-mavihnt-38213559.jpg                     lossless    nativewebp  2560   1706    4009534  -        2      1449.032
pexels-mavihnt-38213559.jpg                     lossless    ours        2560   1706    3585610  -        1      3072.590
pexels-mavihnt-38213559.jpg                     lossless    wasm        2560   1706    3488584  -        1      2976.316
pexels-mavihnt-38213559.jpg                     lossless    webp-rust   2560   1706    3764158  -        2      17649.517
pexels-mavihnt-38213559.jpg                     lossy-fast  libwebp     2560   1706    983360   40.87    20     104.915
pexels-mavihnt-38213559.jpg                     lossy-fast  ours        2560   1706    1037210  40.99    12     168.339
pexels-mavihnt-38213559.jpg                     lossy-fast  wasm        2560   1706    983360   40.87    9      243.078
pexels-mavihnt-38213559.jpg                     lossy-fast  webp-rust   2560   1706    1218578  39.73    25     82.492
pexels-mavihnt-38213559.jpg                     lossy-slow  libwebp     2560   1706    925032   41.14    4      660.783
pexels-mavihnt-38213559.jpg                     lossy-slow  ours        2560   1706    935652   41.05    2      1294.103
pexels-mavihnt-38213559.jpg                     lossy-slow  wasm        2560   1706    925032   41.14    2      1574.205
pexels-mavihnt-38213559.jpg                     lossy-slow  webp-rust   2560   1706    616380   37.45    2      71397.210
pexels-steve-15267299.jpg                       lossless    libwebp     2095   3000    2104690  -        1      2165.929
pexels-steve-15267299.jpg                       lossless    nativewebp  2095   3000    2622528  -        2      1712.520
pexels-steve-15267299.jpg                       lossless    ours        2095   3000    2218786  -        1      3650.701
pexels-steve-15267299.jpg                       lossless    wasm        2095   3000    2119370  -        1      4075.713
pexels-steve-15267299.jpg                       lossless    webp-rust   2095   3000    2458930  -        2      15783.613
pexels-steve-15267299.jpg                       lossy-fast  libwebp     2095   3000    284570   44.34    22     92.023
pexels-steve-15267299.jpg                       lossy-fast  ours        2095   3000    294606   43.91    18     111.213
pexels-steve-15267299.jpg                       lossy-fast  wasm        2095   3000    284570   44.34    9      243.792
pexels-steve-15267299.jpg                       lossy-fast  webp-rust   2095   3000    240256   44.34    33     62.409
pexels-steve-15267299.jpg                       lossy-slow  libwebp     2095   3000    247610   44.50    6      339.614
pexels-steve-15267299.jpg                       lossy-slow  ours        2095   3000    233446   44.04    3      919.133
pexels-steve-15267299.jpg                       lossy-slow  wasm        2095   3000    247610   44.50    2      1067.493
pexels-steve-15267299.jpg                       lossy-slow  webp-rust   2095   3000    134242   41.16    2      20344.411
pexels-steve-29626041.jpg                       lossless    libwebp     2560   1440    283988   -        3      868.113
pexels-steve-29626041.jpg                       lossless    nativewebp  2560   1440    379050   -        4      664.656
pexels-steve-29626041.jpg                       lossless    ours        2560   1440    309370   -        3      935.965
pexels-steve-29626041.jpg                       lossless    wasm        2560   1440    296596   -        2      1754.224
pexels-steve-29626041.jpg                       lossless    webp-rust   2560   1440    319500   -        2      2910.897
pexels-steve-29626041.jpg                       lossy-fast  libwebp     2560   1440    47614    49.05    47     43.349
pexels-steve-29626041.jpg                       lossy-fast  ours        2560   1440    45686    49.31    43     47.279
pexels-steve-29626041.jpg                       lossy-fast  wasm        2560   1440    47614    49.05    17     123.823
pexels-steve-29626041.jpg                       lossy-fast  webp-rust   2560   1440    41908    48.96    68     29.835
pexels-steve-29626041.jpg                       lossy-slow  libwebp     2560   1440    38252    49.65    16     130.558
pexels-steve-29626041.jpg                       lossy-slow  ours        2560   1440    42614    49.25    8      264.209
pexels-steve-29626041.jpg                       lossy-slow  wasm        2560   1440    38252    49.65    5      469.668
pexels-steve-29626041.jpg                       lossy-slow  webp-rust   2560   1440    26878    43.73    2      7601.611
pexels-toulouse-10807703.jpg                    lossless    libwebp     1400   2100    2688812  -        2      1058.420
pexels-toulouse-10807703.jpg                    lossless    nativewebp  1400   2100    2996214  -        3      727.733
pexels-toulouse-10807703.jpg                    lossless    ours        1400   2100    2765508  -        2      1778.688
pexels-toulouse-10807703.jpg                    lossless    wasm        1400   2100    2685806  -        1      2347.664
pexels-toulouse-10807703.jpg                    lossless    webp-rust   1400   2100    2983076  -        2      10189.558
pexels-toulouse-10807703.jpg                    lossy-fast  libwebp     1400   2100    857060   39.84    26     78.470
pexels-toulouse-10807703.jpg                    lossy-fast  ours        1400   2100    854742   39.93    16     132.534
pexels-toulouse-10807703.jpg                    lossy-fast  wasm        1400   2100    857060   39.84    12     171.643
pexels-toulouse-10807703.jpg                    lossy-fast  webp-rust   1400   2100    1124782  6.35     32     64.064
pexels-toulouse-10807703.jpg                    lossy-slow  libwebp     1400   2100    758466   39.84    5      453.195
pexels-toulouse-10807703.jpg                    lossy-slow  ours        1400   2100    761160   39.75    3      719.818
pexels-toulouse-10807703.jpg                    lossy-slow  wasm        1400   2100    758466   39.84    2      1065.503
pexels-toulouse-10807703.jpg                    lossy-slow  webp-rust   1400   2100    560756   36.23    2      67008.317
```

Peak RSS, one encode per process:

```
file                                            mode        engine      width  height  megapixels  peak_rss_mib  mib_per_mp
Lena_512.png                                    lossless    libwebp     900    900     0.81        48.2          59.5
Lena_512.png                                    lossless    nativewebp  900    900     0.81        44.8          55.4
Lena_512.png                                    lossless    ours        900    900     0.81        91.3          112.7
Lena_512.png                                    lossless    wasm        900    900     0.81        147.9         182.6
Lena_512.png                                    lossless    webp-rust   900    900     0.81        129.0         159.2
Lena_512.png                                    lossy-fast  libwebp     900    900     0.81        24.4          30.1
Lena_512.png                                    lossy-fast  ours        900    900     0.81        22.5          27.8
Lena_512.png                                    lossy-fast  wasm        900    900     0.81        38.0          47.0
Lena_512.png                                    lossy-fast  webp-rust   900    900     0.81        10.3          12.7
Lena_512.png                                    lossy-slow  libwebp     900    900     0.81        25.8          31.8
Lena_512.png                                    lossy-slow  ours        900    900     0.81        23.2          28.6
Lena_512.png                                    lossy-slow  wasm        900    900     0.81        48.1          59.4
Lena_512.png                                    lossy-slow  webp-rust   900    900     0.81        16.1          19.9
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    libwebp     2025   2700    5.47        215.0         39.3
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    nativewebp  2025   2700    5.47        204.6         37.4
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    ours        2025   2700    5.47        472.6         86.4
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    wasm        2025   2700    5.47        696.1         127.3
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    webp-rust   2025   2700    5.47        626.2         114.5
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  libwebp     2025   2700    5.47        88.3          16.1
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  ours        2025   2700    5.47        70.0          12.8
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  wasm        2025   2700    5.47        194.1         35.5
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  webp-rust   2025   2700    5.47        56.5          10.3
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  libwebp     2025   2700    5.47        97.2          17.8
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  ours        2025   2700    5.47        70.4          12.9
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  wasm        2025   2700    5.47        194.2         35.5
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  webp-rust   2025   2700    5.47        81.6          14.9
pexels-martin-alargent-1165956-5665465.jpg      lossless    libwebp     2025   2700    5.47        205.4         37.6
pexels-martin-alargent-1165956-5665465.jpg      lossless    nativewebp  2025   2700    5.47        200.2         36.6
pexels-martin-alargent-1165956-5665465.jpg      lossless    ours        2025   2700    5.47        423.7         77.5
pexels-martin-alargent-1165956-5665465.jpg      lossless    wasm        2025   2700    5.47        696.5         127.4
pexels-martin-alargent-1165956-5665465.jpg      lossless    webp-rust   2025   2700    5.47        590.2         107.9
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  libwebp     2025   2700    5.47        88.7          16.2
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  ours        2025   2700    5.47        69.9          12.8
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  wasm        2025   2700    5.47        194.1         35.5
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  webp-rust   2025   2700    5.47        56.9          10.4
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  libwebp     2025   2700    5.47        96.6          17.7
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  ours        2025   2700    5.47        70.2          12.8
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  wasm        2025   2700    5.47        194.2         35.5
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  webp-rust   2025   2700    5.47        78.7          14.4
pexels-mavihnt-38213559.jpg                     lossless    libwebp     2560   1706    4.37        181.3         41.5
pexels-mavihnt-38213559.jpg                     lossless    nativewebp  2560   1706    4.37        174.3         39.9
pexels-mavihnt-38213559.jpg                     lossless    ours        2560   1706    4.37        396.9         90.9
pexels-mavihnt-38213559.jpg                     lossless    wasm        2560   1706    4.37        560.0         128.2
pexels-mavihnt-38213559.jpg                     lossless    webp-rust   2560   1706    4.37        581.0         133.0
pexels-mavihnt-38213559.jpg                     lossy-fast  libwebp     2560   1706    4.37        74.3          17.0
pexels-mavihnt-38213559.jpg                     lossy-fast  ours        2560   1706    4.37        58.4          13.4
pexels-mavihnt-38213559.jpg                     lossy-fast  wasm        2560   1706    4.37        157.5         36.1
pexels-mavihnt-38213559.jpg                     lossy-fast  webp-rust   2560   1706    4.37        48.3          11.1
pexels-mavihnt-38213559.jpg                     lossy-slow  libwebp     2560   1706    4.37        88.8          20.3
pexels-mavihnt-38213559.jpg                     lossy-slow  ours        2560   1706    4.37        58.6          13.4
pexels-mavihnt-38213559.jpg                     lossy-slow  wasm        2560   1706    4.37        221.9         50.8
pexels-mavihnt-38213559.jpg                     lossy-slow  webp-rust   2560   1706    4.37        65.5          15.0
pexels-steve-15267299.jpg                       lossless    libwebp     2095   3000    6.29        208.2         33.1
pexels-steve-15267299.jpg                       lossless    nativewebp  2095   3000    6.29        212.7         33.8
pexels-steve-15267299.jpg                       lossless    ours        2095   3000    6.29        483.7         77.0
pexels-steve-15267299.jpg                       lossless    wasm        2095   3000    6.29        573.4         91.2
pexels-steve-15267299.jpg                       lossless    webp-rust   2095   3000    6.29        725.0         115.3
pexels-steve-15267299.jpg                       lossy-fast  libwebp     2095   3000    6.29        98.1          15.6
pexels-steve-15267299.jpg                       lossy-fast  ours        2095   3000    6.29        78.8          12.5
pexels-steve-15267299.jpg                       lossy-fast  wasm        2095   3000    6.29        221.4         35.2
pexels-steve-15267299.jpg                       lossy-fast  webp-rust   2095   3000    6.29        63.9          10.2
pexels-steve-15267299.jpg                       lossy-slow  libwebp     2095   3000    6.29        102.0         16.2
pexels-steve-15267299.jpg                       lossy-slow  ours        2095   3000    6.29        79.9          12.7
pexels-steve-15267299.jpg                       lossy-slow  wasm        2095   3000    6.29        221.3         35.2
pexels-steve-15267299.jpg                       lossy-slow  webp-rust   2095   3000    6.29        94.6          15.1
pexels-steve-29626041.jpg                       lossless    libwebp     2560   1440    3.69        131.8         35.8
pexels-steve-29626041.jpg                       lossless    nativewebp  2560   1440    3.69        116.3         31.6
pexels-steve-29626041.jpg                       lossless    ours        2560   1440    3.69        283.0         76.8
pexels-steve-29626041.jpg                       lossless    wasm        2560   1440    3.69        341.5         92.6
pexels-steve-29626041.jpg                       lossless    webp-rust   2560   1440    3.69        382.3         103.7
pexels-steve-29626041.jpg                       lossy-fast  libwebp     2560   1440    3.69        61.5          16.7
pexels-steve-29626041.jpg                       lossy-fast  ours        2560   1440    3.69        50.6          13.7
pexels-steve-29626041.jpg                       lossy-fast  wasm        2560   1440    3.69        91.1          24.7
pexels-steve-29626041.jpg                       lossy-fast  webp-rust   2560   1440    3.69        38.2          10.4
pexels-steve-29626041.jpg                       lossy-slow  libwebp     2560   1440    3.69        62.4          16.9
pexels-steve-29626041.jpg                       lossy-slow  ours        2560   1440    3.69        50.9          13.8
pexels-steve-29626041.jpg                       lossy-slow  wasm        2560   1440    3.69        134.7         36.5
pexels-steve-29626041.jpg                       lossy-slow  webp-rust   2560   1440    3.69        55.0          14.9
pexels-toulouse-10807703.jpg                    lossless    libwebp     1400   2100    2.94        127.2         43.3
pexels-toulouse-10807703.jpg                    lossless    nativewebp  1400   2100    2.94        124.4         42.3
pexels-toulouse-10807703.jpg                    lossless    ours        1400   2100    2.94        245.7         83.6
pexels-toulouse-10807703.jpg                    lossless    wasm        1400   2100    2.94        382.6         130.1
pexels-toulouse-10807703.jpg                    lossless    webp-rust   1400   2100    2.94        362.0         123.1
pexels-toulouse-10807703.jpg                    lossy-fast  libwebp     1400   2100    2.94        54.0          18.4
pexels-toulouse-10807703.jpg                    lossy-fast  ours        1400   2100    2.94        42.5          14.5
pexels-toulouse-10807703.jpg                    lossy-fast  wasm        1400   2100    2.94        110.0         37.4
pexels-toulouse-10807703.jpg                    lossy-fast  webp-rust   1400   2100    2.94        32.8          11.1
pexels-toulouse-10807703.jpg                    lossy-slow  libwebp     1400   2100    2.94        66.2          22.5
pexels-toulouse-10807703.jpg                    lossy-slow  ours        1400   2100    2.94        44.2          15.0
pexels-toulouse-10807703.jpg                    lossy-slow  wasm        1400   2100    2.94        153.7         52.3
pexels-toulouse-10807703.jpg                    lossy-slow  webp-rust   1400   2100    2.94        45.2          15.4
```

Decode, one file per mode encoded by libwebp:

```
file                                            mode      engine     width  height  bytes    iters  ms_per_op
Lena_512.png                                    lossless  libwebp    900    900     627632   195    10.299
Lena_512.png                                    lossless  ours       900    900     627632   105    19.133
Lena_512.png                                    lossless  wasm       900    900     627632   81     24.773
Lena_512.png                                    lossless  webp-rust  900    900     627632   36     56.835
Lena_512.png                                    lossless  x/image    900    900     627632   106    18.998
Lena_512.png                                    lossy     libwebp    900    900     89968    407    4.916
Lena_512.png                                    lossy     ours       900    900     89968    116    17.341
Lena_512.png                                    lossy     wasm       900    900     89968    126    15.911
Lena_512.png                                    lossy     webp-rust  900    900     89968    180    11.164
Lena_512.png                                    lossy     x/image    900    900     89968    110    18.346
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp    2025   2700    3241976  36     55.652
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours       2025   2700    3241976  17     121.314
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm       2025   2700    3241976  14     149.590
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  webp-rust  2025   2700    3241976  7      318.183
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  x/image    2025   2700    3241976  19     105.726
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp    2025   2700    610518   58     34.626
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours       2025   2700    610518   17     124.246
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm       2025   2700    610518   18     116.024
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     webp-rust  2025   2700    610518   24     86.365
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     x/image    2025   2700    610518   16     131.437
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp    2025   2700    2943528  36     55.637
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours       2025   2700    2943528  19     110.110
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm       2025   2700    2943528  14     146.953
pexels-martin-alargent-1165956-5665465.jpg      lossless  webp-rust  2025   2700    2943528  7      320.205
pexels-martin-alargent-1165956-5665465.jpg      lossless  x/image    2025   2700    2943528  20     101.312
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp    2025   2700    603264   61     32.986
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours       2025   2700    603264   18     113.610
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm       2025   2700    603264   19     107.279
pexels-martin-alargent-1165956-5665465.jpg      lossy     webp-rust  2025   2700    603264   26     78.075
pexels-martin-alargent-1165956-5665465.jpg      lossy     x/image    2025   2700    603264   17     122.982
pexels-mavihnt-38213559.jpg                     lossless  libwebp    2560   1706    3482480  40     50.293
pexels-mavihnt-38213559.jpg                     lossless  ours       2560   1706    3482480  20     100.147
pexels-mavihnt-38213559.jpg                     lossless  wasm       2560   1706    3482480  16     128.166
pexels-mavihnt-38213559.jpg                     lossless  webp-rust  2560   1706    3482480  7      304.858
pexels-mavihnt-38213559.jpg                     lossless  x/image    2560   1706    3482480  21     98.758
pexels-mavihnt-38213559.jpg                     lossy     libwebp    2560   1706    925032   48     42.135
pexels-mavihnt-38213559.jpg                     lossy     ours       2560   1706    925032   18     113.236
pexels-mavihnt-38213559.jpg                     lossy     wasm       2560   1706    925032   20     100.201
pexels-mavihnt-38213559.jpg                     lossy     webp-rust  2560   1706    925032   27     75.344
pexels-mavihnt-38213559.jpg                     lossy     x/image    2560   1706    925032   16     128.026
pexels-steve-15267299.jpg                       lossless  libwebp    2095   3000    2104690  36     56.624
pexels-steve-15267299.jpg                       lossless  ours       2095   3000    2104690  20     103.508
pexels-steve-15267299.jpg                       lossless  wasm       2095   3000    2104690  13     156.757
pexels-steve-15267299.jpg                       lossless  webp-rust  2095   3000    2104690  8      270.135
pexels-steve-15267299.jpg                       lossless  x/image    2095   3000    2104690  20     102.349
pexels-steve-15267299.jpg                       lossy     libwebp    2095   3000    247610   98     20.533
pexels-steve-15267299.jpg                       lossy     ours       2095   3000    247610   21     97.708
pexels-steve-15267299.jpg                       lossy     wasm       2095   3000    247610   21     95.333
pexels-steve-15267299.jpg                       lossy     webp-rust  2095   3000    247610   31     65.035
pexels-steve-15267299.jpg                       lossy     x/image    2095   3000    247610   21     98.499
pexels-steve-29626041.jpg                       lossless  libwebp    2560   1440    283988   111    18.045
pexels-steve-29626041.jpg                       lossless  ours       2560   1440    283988   74     27.061
pexels-steve-29626041.jpg                       lossless  wasm       2560   1440    283988   29     69.194
pexels-steve-29626041.jpg                       lossless  webp-rust  2560   1440    283988   30     68.091
pexels-steve-29626041.jpg                       lossless  x/image    2560   1440    283988   58     34.549
pexels-steve-29626041.jpg                       lossy     libwebp    2560   1440    38252    281    7.136
pexels-steve-29626041.jpg                       lossy     ours       2560   1440    38252    49     41.070
pexels-steve-29626041.jpg                       lossy     wasm       2560   1440    38252    46     43.989
pexels-steve-29626041.jpg                       lossy     webp-rust  2560   1440    38252    77     26.294
pexels-steve-29626041.jpg                       lossy     x/image    2560   1440    38252    48     41.906
pexels-toulouse-10807703.jpg                    lossless  libwebp    1400   2100    2688812  56     36.064
pexels-toulouse-10807703.jpg                    lossless  ours       1400   2100    2688812  30     67.248
pexels-toulouse-10807703.jpg                    lossless  wasm       1400   2100    2688812  23     88.447
pexels-toulouse-10807703.jpg                    lossless  webp-rust  1400   2100    2688812  10     220.875
pexels-toulouse-10807703.jpg                    lossless  x/image    1400   2100    2688812  28     73.078
pexels-toulouse-10807703.jpg                    lossy     libwebp    1400   2100    758466   61     33.112
pexels-toulouse-10807703.jpg                    lossy     ours       1400   2100    758466   24     85.657
pexels-toulouse-10807703.jpg                    lossy     wasm       1400   2100    758466   28     73.887
pexels-toulouse-10807703.jpg                    lossy     webp-rust  1400   2100    758466   35     57.144
pexels-toulouse-10807703.jpg                    lossy     x/image    1400   2100    758466   21     97.053
```

## amd64 (AMD Ryzen 7 5700G)

```
file                                            mode        engine      width  height  bytes    psnr_db  iters  ms_per_op
Lena_512.png                                    lossless    libwebp     900    900     627632   -        8      254.507
Lena_512.png                                    lossless    nativewebp  900    900     738176   -        7      311.701
Lena_512.png                                    lossless    ours        900    900     651634   -        4      660.579
Lena_512.png                                    lossless    wasm        900    900     622766   -        2      1494.367
Lena_512.png                                    lossless    webp-rust   900    900     651036   -        2      3263.003
Lena_512.png                                    lossy-fast  libwebp     900    900     103980   41.04    116    17.337
Lena_512.png                                    lossy-fast  ours        900    900     113182   41.02    69     29.292
Lena_512.png                                    lossy-fast  wasm        900    900     103980   41.04    30     67.542
Lena_512.png                                    lossy-fast  webp-rust   900    900     127376   40.09    108    18.635
Lena_512.png                                    lossy-slow  libwebp     900    900     89968    40.94    19     109.055
Lena_512.png                                    lossy-slow  ours        900    900     88890    40.16    10     203.948
Lena_512.png                                    lossy-slow  wasm        900    900     89968    40.94    6      363.770
Lena_512.png                                    lossy-slow  webp-rust   900    900     65024    37.62    2      15640.261
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    libwebp     2025   2700    3241976  -        1      2151.529
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    nativewebp  2025   2700    3926412  -        1      3088.770
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    ours        2025   2700    3355646  -        1      6401.795
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    wasm        2025   2700    3249798  -        1      5844.929
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    webp-rust   2025   2700    3468390  -        2      40594.528
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  libwebp     2025   2700    598034   41.96    18     114.498
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  ours        2025   2700    723064   42.02    11     195.500
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  wasm        2025   2700    598034   41.96    5      453.489
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  webp-rust   2025   2700    641754   16.44    17     123.391
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  libwebp     2025   2700    610518   42.58    3      969.852
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  ours        2025   2700    592722   42.26    2      1712.864
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  wasm        2025   2700    610518   42.58    1      2888.397
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  webp-rust   2025   2700    283348   38.71    2      65417.581
pexels-martin-alargent-1165956-5665465.jpg      lossless    libwebp     2025   2700    2943528  -        1      2067.281
pexels-martin-alargent-1165956-5665465.jpg      lossless    nativewebp  2025   2700    3563584  -        1      2570.167
pexels-martin-alargent-1165956-5665465.jpg      lossless    ours        2025   2700    3058988  -        1      4717.529
pexels-martin-alargent-1165956-5665465.jpg      lossless    wasm        2025   2700    2953798  -        1      5847.951
pexels-martin-alargent-1165956-5665465.jpg      lossless    webp-rust   2025   2700    3186444  -        2      29823.236
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  libwebp     2025   2700    726824   42.63    17     118.643
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  ours        2025   2700    756806   42.39    11     196.845
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  wasm        2025   2700    726824   42.63    5      454.450
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  webp-rust   2025   2700    826398   9.38     16     127.306
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  libwebp     2025   2700    603264   42.75    3      814.518
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  ours        2025   2700    601454   42.73    2      1410.005
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  wasm        2025   2700    603264   42.75    1      2577.092
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  webp-rust   2025   2700    419784   39.67    2      103548.238
pexels-mavihnt-38213559.jpg                     lossless    libwebp     2560   1706    3482480  -        2      1641.486
pexels-mavihnt-38213559.jpg                     lossless    nativewebp  2560   1706    4009534  -        1      2128.291
pexels-mavihnt-38213559.jpg                     lossless    ours        2560   1706    3585610  -        1      4472.428
pexels-mavihnt-38213559.jpg                     lossless    wasm        2560   1706    3488584  -        1      4764.905
pexels-mavihnt-38213559.jpg                     lossless    webp-rust   2560   1706    3764158  -        2      31400.684
pexels-mavihnt-38213559.jpg                     lossy-fast  libwebp     2560   1706    983360   40.87    17     120.111
pexels-mavihnt-38213559.jpg                     lossy-fast  ours        2560   1706    1037210  40.99    10     211.491
pexels-mavihnt-38213559.jpg                     lossy-fast  wasm        2560   1706    983360   40.87    5      415.651
pexels-mavihnt-38213559.jpg                     lossy-fast  webp-rust   2560   1706    1218578  39.73    15     133.834
pexels-mavihnt-38213559.jpg                     lossy-slow  libwebp     2560   1706    925032   41.14    3      965.200
pexels-mavihnt-38213559.jpg                     lossy-slow  ours        2560   1706    935652   41.05    2      1630.513
pexels-mavihnt-38213559.jpg                     lossy-slow  wasm        2560   1706    925032   41.14    1      2700.096
pexels-mavihnt-38213559.jpg                     lossy-slow  webp-rust   2560   1706    616380   37.45    2      136575.077
pexels-steve-15267299.jpg                       lossless    libwebp     2095   3000    2104690  -        1      2267.576
pexels-steve-15267299.jpg                       lossless    nativewebp  2095   3000    2622528  -        1      2887.666
pexels-steve-15267299.jpg                       lossless    ours        2095   3000    2218786  -        1      4583.212
pexels-steve-15267299.jpg                       lossless    wasm        2095   3000    2119370  -        1      6046.492
pexels-steve-15267299.jpg                       lossless    webp-rust   2095   3000    2458930  -        2      25154.261
pexels-steve-15267299.jpg                       lossy-fast  libwebp     2095   3000    284570   44.34    19     105.369
pexels-steve-15267299.jpg                       lossy-fast  ours        2095   3000    294606   43.91    13     157.594
pexels-steve-15267299.jpg                       lossy-fast  wasm        2095   3000    284570   44.34    5      460.934
pexels-steve-15267299.jpg                       lossy-fast  webp-rust   2095   3000    240256   44.34    19     108.272
pexels-steve-15267299.jpg                       lossy-slow  libwebp     2095   3000    247610   44.50    4      515.194
pexels-steve-15267299.jpg                       lossy-slow  ours        2095   3000    233446   44.04    2      1160.375
pexels-steve-15267299.jpg                       lossy-slow  wasm        2095   3000    247610   44.50    1      2130.103
pexels-steve-15267299.jpg                       lossy-slow  webp-rust   2095   3000    134242   41.16    2      36574.182
pexels-steve-29626041.jpg                       lossless    libwebp     2560   1440    283988   -        3      729.844
pexels-steve-29626041.jpg                       lossless    nativewebp  2560   1440    379050   -        2      1385.597
pexels-steve-29626041.jpg                       lossless    ours        2560   1440    309370   -        2      1466.488
pexels-steve-29626041.jpg                       lossless    wasm        2560   1440    296596   -        1      2846.717
pexels-steve-29626041.jpg                       lossless    webp-rust   2560   1440    319500   -        2      4763.599
pexels-steve-29626041.jpg                       lossy-fast  libwebp     2560   1440    47614    49.05    40     50.422
pexels-steve-29626041.jpg                       lossy-fast  ours        2560   1440    45686    49.31    28     72.064
pexels-steve-29626041.jpg                       lossy-fast  wasm        2560   1440    47614    49.05    9      243.690
pexels-steve-29626041.jpg                       lossy-fast  webp-rust   2560   1440    41908    48.96    39     52.381
pexels-steve-29626041.jpg                       lossy-slow  libwebp     2560   1440    38252    49.65    10     206.648
pexels-steve-29626041.jpg                       lossy-slow  ours        2560   1440    42614    49.25    6      384.144
pexels-steve-29626041.jpg                       lossy-slow  wasm        2560   1440    38252    49.65    3      967.678
pexels-steve-29626041.jpg                       lossy-slow  webp-rust   2560   1440    26878    43.73    2      13632.822
pexels-toulouse-10807703.jpg                    lossless    libwebp     1400   2100    2688812  -        2      1148.804
pexels-toulouse-10807703.jpg                    lossless    nativewebp  1400   2100    2996214  -        2      1295.368
pexels-toulouse-10807703.jpg                    lossless    ours        1400   2100    2765508  -        1      2631.769
pexels-toulouse-10807703.jpg                    lossless    wasm        1400   2100    2685806  -        1      3935.107
pexels-toulouse-10807703.jpg                    lossless    webp-rust   1400   2100    2983076  -        2      15565.682
pexels-toulouse-10807703.jpg                    lossy-fast  libwebp     1400   2100    857060   39.84    23     90.429
pexels-toulouse-10807703.jpg                    lossy-fast  ours        1400   2100    854742   39.93    13     164.322
pexels-toulouse-10807703.jpg                    lossy-fast  wasm        1400   2100    857060   39.84    7      293.128
pexels-toulouse-10807703.jpg                    lossy-fast  webp-rust   1400   2100    1124782  6.35     21     96.731
pexels-toulouse-10807703.jpg                    lossy-slow  libwebp     1400   2100    758466   39.84    4      658.388
pexels-toulouse-10807703.jpg                    lossy-slow  ours        1400   2100    761160   39.75    3      845.620
pexels-toulouse-10807703.jpg                    lossy-slow  wasm        1400   2100    758466   39.84    2      1937.546
pexels-toulouse-10807703.jpg                    lossy-slow  webp-rust   1400   2100    560756   36.23    2      131516.197
```

Peak RSS, one encode per process:

```
file                                            mode        engine      width  height  megapixels  peak_rss_mib  mib_per_mp
Lena_512.png                                    lossless    libwebp     900    900     0.81        44.2          54.6
Lena_512.png                                    lossless    nativewebp  900    900     0.81        42.1          52.0
Lena_512.png                                    lossless    ours        900    900     0.81        77.1          95.2
Lena_512.png                                    lossless    wasm        900    900     0.81        139.2         171.9
Lena_512.png                                    lossless    webp-rust   900    900     0.81        79.2          97.8
Lena_512.png                                    lossy-fast  libwebp     900    900     0.81        22.8          28.2
Lena_512.png                                    lossy-fast  ours        900    900     0.81        21.7          26.8
Lena_512.png                                    lossy-fast  wasm        900    900     0.81        38.8          47.8
Lena_512.png                                    lossy-fast  webp-rust   900    900     0.81        8.3           10.3
Lena_512.png                                    lossy-slow  libwebp     900    900     0.81        24.1          29.8
Lena_512.png                                    lossy-slow  ours        900    900     0.81        21.6          26.7
Lena_512.png                                    lossy-slow  wasm        900    900     0.81        49.0          60.5
Lena_512.png                                    lossy-slow  webp-rust   900    900     0.81        11.8          14.6
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    libwebp     2025   2700    5.47        203.5         37.2
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    nativewebp  2025   2700    5.47        203.2         37.2
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    ours        2025   2700    5.47        466.9         85.4
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    wasm        2025   2700    5.47        696.7         127.4
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    webp-rust   2025   2700    5.47        499.8         91.4
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  libwebp     2025   2700    5.47        84.8          15.5
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  ours        2025   2700    5.47        68.4          12.5
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  wasm        2025   2700    5.47        193.5         35.4
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  webp-rust   2025   2700    5.47        40.9          7.5
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  libwebp     2025   2700    5.47        93.6          17.1
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  ours        2025   2700    5.47        72.6          13.3
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  wasm        2025   2700    5.47        193.7         35.4
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  webp-rust   2025   2700    5.47        60.8          11.1
pexels-martin-alargent-1165956-5665465.jpg      lossless    libwebp     2025   2700    5.47        194.2         35.5
pexels-martin-alargent-1165956-5665465.jpg      lossless    nativewebp  2025   2700    5.47        198.9         36.4
pexels-martin-alargent-1165956-5665465.jpg      lossless    ours        2025   2700    5.47        420.2         76.9
pexels-martin-alargent-1165956-5665465.jpg      lossless    wasm        2025   2700    5.47        695.1         127.1
pexels-martin-alargent-1165956-5665465.jpg      lossless    webp-rust   2025   2700    5.47        481.6         88.1
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  libwebp     2025   2700    5.47        85.3          15.6
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  ours        2025   2700    5.47        70.2          12.8
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  wasm        2025   2700    5.47        193.7         35.4
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  webp-rust   2025   2700    5.47        41.0          7.5
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  libwebp     2025   2700    5.47        95.2          17.4
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  ours        2025   2700    5.47        70.5          12.9
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  wasm        2025   2700    5.47        195.6         35.8
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  webp-rust   2025   2700    5.47        60.6          11.1
pexels-mavihnt-38213559.jpg                     lossless    libwebp     2560   1706    4.37        169.8         38.9
pexels-mavihnt-38213559.jpg                     lossless    nativewebp  2560   1706    4.37        174.8         40.0
pexels-mavihnt-38213559.jpg                     lossless    ours        2560   1706    4.37        391.1         89.5
pexels-mavihnt-38213559.jpg                     lossless    wasm        2560   1706    4.37        557.4         127.6
pexels-mavihnt-38213559.jpg                     lossless    webp-rust   2560   1706    4.37        405.3         92.8
pexels-mavihnt-38213559.jpg                     lossy-fast  libwebp     2560   1706    4.37        72.1          16.5
pexels-mavihnt-38213559.jpg                     lossy-fast  ours        2560   1706    4.37        58.3          13.3
pexels-mavihnt-38213559.jpg                     lossy-fast  wasm        2560   1706    4.37        157.2         36.0
pexels-mavihnt-38213559.jpg                     lossy-fast  webp-rust   2560   1706    4.37        35.9          8.2
pexels-mavihnt-38213559.jpg                     lossy-slow  libwebp     2560   1706    4.37        84.1          19.2
pexels-mavihnt-38213559.jpg                     lossy-slow  ours        2560   1706    4.37        58.3          13.3
pexels-mavihnt-38213559.jpg                     lossy-slow  wasm        2560   1706    4.37        223.2         51.1
pexels-mavihnt-38213559.jpg                     lossy-slow  webp-rust   2560   1706    4.37        50.3          11.5
pexels-steve-15267299.jpg                       lossless    libwebp     2095   3000    6.29        203.1         32.3
pexels-steve-15267299.jpg                       lossless    nativewebp  2095   3000    6.29        213.3         33.9
pexels-steve-15267299.jpg                       lossless    ours        2095   3000    6.29        483.7         77.0
pexels-steve-15267299.jpg                       lossless    wasm        2095   3000    6.29        572.0         91.0
pexels-steve-15267299.jpg                       lossless    webp-rust   2095   3000    6.29        506.9         80.7
pexels-steve-15267299.jpg                       lossy-fast  libwebp     2095   3000    6.29        94.9          15.1
pexels-steve-15267299.jpg                       lossy-fast  ours        2095   3000    6.29        76.2          12.1
pexels-steve-15267299.jpg                       lossy-fast  wasm        2095   3000    6.29        221.7         35.3
pexels-steve-15267299.jpg                       lossy-fast  webp-rust   2095   3000    6.29        45.9          7.3
pexels-steve-15267299.jpg                       lossy-slow  libwebp     2095   3000    6.29        100.4         16.0
pexels-steve-15267299.jpg                       lossy-slow  ours        2095   3000    6.29        78.5          12.5
pexels-steve-15267299.jpg                       lossy-slow  wasm        2095   3000    6.29        219.6         34.9
pexels-steve-15267299.jpg                       lossy-slow  webp-rust   2095   3000    6.29        68.7          10.9
pexels-steve-29626041.jpg                       lossless    libwebp     2560   1440    3.69        124.7         33.8
pexels-steve-29626041.jpg                       lossless    nativewebp  2560   1440    3.69        116.8         31.7
pexels-steve-29626041.jpg                       lossless    ours        2560   1440    3.69        260.3         70.6
pexels-steve-29626041.jpg                       lossless    wasm        2560   1440    3.69        471.8         128.0
pexels-steve-29626041.jpg                       lossless    webp-rust   2560   1440    3.69        263.6         71.5
pexels-steve-29626041.jpg                       lossy-fast  libwebp     2560   1440    3.69        60.2          16.3
pexels-steve-29626041.jpg                       lossy-fast  ours        2560   1440    3.69        48.2          13.1
pexels-steve-29626041.jpg                       lossy-fast  wasm        2560   1440    3.69        91.2          24.7
pexels-steve-29626041.jpg                       lossy-fast  webp-rust   2560   1440    3.69        28.0          7.6
pexels-steve-29626041.jpg                       lossy-slow  libwebp     2560   1440    3.69        61.2          16.6
pexels-steve-29626041.jpg                       lossy-slow  ours        2560   1440    3.69        49.8          13.5
pexels-steve-29626041.jpg                       lossy-slow  wasm        2560   1440    3.69        135.4         36.7
pexels-steve-29626041.jpg                       lossy-slow  webp-rust   2560   1440    3.69        41.3          11.2
pexels-toulouse-10807703.jpg                    lossless    libwebp     1400   2100    2.94        116.6         39.6
pexels-toulouse-10807703.jpg                    lossless    nativewebp  1400   2100    2.94        124.7         42.4
pexels-toulouse-10807703.jpg                    lossless    ours        1400   2100    2.94        243.6         82.8
pexels-toulouse-10807703.jpg                    lossless    wasm        1400   2100    2.94        381.9         129.9
pexels-toulouse-10807703.jpg                    lossless    webp-rust   1400   2100    2.94        290.8         98.9
pexels-toulouse-10807703.jpg                    lossy-fast  libwebp     1400   2100    2.94        52.0          17.7
pexels-toulouse-10807703.jpg                    lossy-fast  ours        1400   2100    2.94        42.3          14.4
pexels-toulouse-10807703.jpg                    lossy-fast  wasm        1400   2100    2.94        111.0         37.7
pexels-toulouse-10807703.jpg                    lossy-fast  webp-rust   1400   2100    2.94        24.6          8.4
pexels-toulouse-10807703.jpg                    lossy-slow  libwebp     1400   2100    2.94        63.4          21.6
pexels-toulouse-10807703.jpg                    lossy-slow  ours        1400   2100    2.94        42.2          14.4
pexels-toulouse-10807703.jpg                    lossy-slow  wasm        1400   2100    2.94        153.4         52.2
pexels-toulouse-10807703.jpg                    lossy-slow  webp-rust   1400   2100    2.94        35.1          11.9
```

Decode, one file per mode encoded by libwebp:

```
file                                            mode      engine     width  height  bytes    iters  ms_per_op
Lena_512.png                                    lossless  libwebp    900    900     627632   208    9.649
Lena_512.png                                    lossless  ours       900    900     627632   105    19.199
Lena_512.png                                    lossless  wasm       900    900     627632   51     39.455
Lena_512.png                                    lossless  webp-rust  900    900     627632   31     65.620
Lena_512.png                                    lossless  x/image    900    900     627632   87     23.081
Lena_512.png                                    lossy     libwebp    900    900     89968    309    6.474
Lena_512.png                                    lossy     ours       900    900     89968    92     21.758
Lena_512.png                                    lossy     wasm       900    900     89968    77     26.053
Lena_512.png                                    lossy     webp-rust  900    900     89968    123    16.352
Lena_512.png                                    lossy     x/image    900    900     89968    83     24.258
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  libwebp    2025   2700    3241976  40     50.876
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  ours       2025   2700    3241976  17     117.651
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  wasm       2025   2700    3241976  8      251.238
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  webp-rust  2025   2700    3241976  6      362.250
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless  x/image    2025   2700    3241976  15     135.414
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     libwebp    2025   2700    610518   44     46.006
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     ours       2025   2700    610518   14     150.958
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     wasm       2025   2700    610518   11     183.466
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     webp-rust  2025   2700    610518   17     118.118
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy     x/image    2025   2700    610518   12     168.832
pexels-martin-alargent-1165956-5665465.jpg      lossless  libwebp    2025   2700    2943528  40     50.437
pexels-martin-alargent-1165956-5665465.jpg      lossless  ours       2025   2700    2943528  19     110.706
pexels-martin-alargent-1165956-5665465.jpg      lossless  wasm       2025   2700    2943528  9      247.181
pexels-martin-alargent-1165956-5665465.jpg      lossless  webp-rust  2025   2700    2943528  6      354.380
pexels-martin-alargent-1165956-5665465.jpg      lossless  x/image    2025   2700    2943528  16     131.572
pexels-martin-alargent-1165956-5665465.jpg      lossy     libwebp    2025   2700    603264   45     45.084
pexels-martin-alargent-1165956-5665465.jpg      lossy     ours       2025   2700    603264   14     147.439
pexels-martin-alargent-1165956-5665465.jpg      lossy     wasm       2025   2700    603264   12     176.742
pexels-martin-alargent-1165956-5665465.jpg      lossy     webp-rust  2025   2700    603264   18     112.955
pexels-martin-alargent-1165956-5665465.jpg      lossy     x/image    2025   2700    603264   13     162.343
pexels-mavihnt-38213559.jpg                     lossless  libwebp    2560   1706    3482480  43     46.721
pexels-mavihnt-38213559.jpg                     lossless  ours       2560   1706    3482480  21     95.888
pexels-mavihnt-38213559.jpg                     lossless  wasm       2560   1706    3482480  10     210.976
pexels-mavihnt-38213559.jpg                     lossless  webp-rust  2560   1706    3482480  6      353.600
pexels-mavihnt-38213559.jpg                     lossless  x/image    2560   1706    3482480  18     117.567
pexels-mavihnt-38213559.jpg                     lossy     libwebp    2560   1706    925032   39     51.930
pexels-mavihnt-38213559.jpg                     lossy     ours       2560   1706    925032   16     130.746
pexels-mavihnt-38213559.jpg                     lossy     wasm       2560   1706    925032   13     157.976
pexels-mavihnt-38213559.jpg                     lossy     webp-rust  2560   1706    925032   20     101.008
pexels-mavihnt-38213559.jpg                     lossy     x/image    2560   1706    925032   14     146.848
pexels-steve-15267299.jpg                       lossless  libwebp    2095   3000    2104690  40     50.988
pexels-steve-15267299.jpg                       lossless  ours       2095   3000    2104690  16     125.463
pexels-steve-15267299.jpg                       lossless  wasm       2095   3000    2104690  8      264.620
pexels-steve-15267299.jpg                       lossless  webp-rust  2095   3000    2104690  7      296.554
pexels-steve-15267299.jpg                       lossless  x/image    2095   3000    2104690  14     144.306
pexels-steve-15267299.jpg                       lossy     libwebp    2095   3000    247610   65     30.796
pexels-steve-15267299.jpg                       lossy     ours       2095   3000    247610   15     136.235
pexels-steve-15267299.jpg                       lossy     wasm       2095   3000    247610   12     169.643
pexels-steve-15267299.jpg                       lossy     webp-rust  2095   3000    247610   20     102.209
pexels-steve-15267299.jpg                       lossy     x/image    2095   3000    247610   14     151.540
pexels-steve-29626041.jpg                       lossless  libwebp    2560   1440    283988   130    15.468
pexels-steve-29626041.jpg                       lossless  ours       2560   1440    283988   45     45.285
pexels-steve-29626041.jpg                       lossless  wasm       2560   1440    283988   17     121.297
pexels-steve-29626041.jpg                       lossless  webp-rust  2560   1440    283988   31     65.619
pexels-steve-29626041.jpg                       lossless  x/image    2560   1440    283988   34     59.657
pexels-steve-29626041.jpg                       lossy     libwebp    2560   1440    38252    179    11.228
pexels-steve-29626041.jpg                       lossy     ours       2560   1440    38252    34     60.229
pexels-steve-29626041.jpg                       lossy     wasm       2560   1440    38252    25     81.710
pexels-steve-29626041.jpg                       lossy     webp-rust  2560   1440    38252    46     44.188
pexels-steve-29626041.jpg                       lossy     x/image    2560   1440    38252    29     69.925
pexels-toulouse-10807703.jpg                    lossless  libwebp    1400   2100    2688812  56     35.880
pexels-toulouse-10807703.jpg                    lossless  ours       1400   2100    2688812  29     69.151
pexels-toulouse-10807703.jpg                    lossless  wasm       1400   2100    2688812  14     144.787
pexels-toulouse-10807703.jpg                    lossless  webp-rust  1400   2100    2688812  8      254.969
pexels-toulouse-10807703.jpg                    lossless  x/image    1400   2100    2688812  23     88.610
pexels-toulouse-10807703.jpg                    lossy     libwebp    1400   2100    758466   48     41.737
pexels-toulouse-10807703.jpg                    lossy     ours       1400   2100    758466   21     99.752
pexels-toulouse-10807703.jpg                    lossy     wasm       1400   2100    758466   18     116.677
pexels-toulouse-10807703.jpg                    lossy     webp-rust  1400   2100    758466   27     76.411
pexels-toulouse-10807703.jpg                    lossy     x/image    1400   2100    758466   19     109.494
```
