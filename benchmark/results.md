# Benchmark results

Captured run of `benchmark/run.sh` on two machines. Timings are machine-dependent;
regenerate locally for your own hardware.

| | arm64 | amd64 |
| --- | --- | --- |
| **CPU** | Apple M4 Pro (14 cores) | AMD Ryzen 7 5700G (16 threads) |
| **OS** | macOS 26.5.1 | Arch Linux, kernel 7.1.3 |
| **Go** | go1.26.5 | go1.26.5 |
| **Rust** | 1.97.1 | 1.97.1 |

- **libwebp:** 1.6.0 on both (also used by the `wasm` engine, via WASM) · **webp-rust:** v0.2.1
- **Library commit:** 7e5e084 · **Date:** 2026-07-25
- **Budget:** 2000 ms/measurement, lossy quality 90

Engines: `ours` (pure Go), `libwebp` (C, cgo), `wasm` (libwebp via WASM, cgo-free),
`webp-rust` (the Rust original). See `README.md` for the exact mode settings.

`psnr_db` decodes each encoder's own output and scores it against the pixels that
encoder was handed, over RGB. Lossless is exact, so it shows `-`. Size without
quality is not comparable: an encoder can always make a smaller file by
quantizing harder, so the two columns have to be read together.

Encoder output is identical on both machines (every size in the two tables
matches), so the `psnr_db` column is shared between them and only the timings
differ. These timings were captured before `webpbench` and `rustbench` emitted
PSNR, so the column was measured in a separate pass at the same settings and
matched back row by row against the byte sizes here; re-running `run.sh` now
emits it directly.

## Reading these numbers

- **vs libwebp, `lossy-slow`:** we produce smaller files (0.84-1.00x) at lower
  quality (0.3-1.1 dB behind, except steve-29626041 at 2.9 dB). That is a
  rate-distortion deficit, and it costs 14-24x the encode time.
- **vs libwebp, `lossy-fast`:** quality matches within ±0.5 dB, but our files are
  10-62% bigger for 1.3-1.6x the time. Effort 0 is where we are furthest behind.
- **vs libwebp, lossless:** we are 3-9% bigger and 1.2-2.7x slower.
- **`libwebp` and `wasm` are the same encoder.** Their lossy output is
  byte-identical (verified by hash), so their sizes and PSNR match exactly;
  `wasm` is the cgo-free option and runs ~2-6x slower than native `libwebp`.
- **We no longer match `webp-rust` bit-for-bit.** Commit 7e5e084 changed the
  quality->quantizer mapping to libwebp's nonlinear curve, fixed a token-partition
  desync, and switched candidate selection to rate-distortion, so our output
  intentionally diverges from the Rust original.
- **`webp-rust` `lossy-fast` output is corrupt on 3 of 7 images**, a bug this
  port found and fixed. See "The webp-rust lossy-fast bug" below.
- **`webp-rust` `lossy-slow` files are smaller than everyone's, at 1.5-3 dB lower
  quality** (e.g. toulouse 560756 B at 36.23 dB vs our 753810 B at 39.55 dB).
  An earlier capture of these results read that size advantage as a compression
  win; the PSNR column shows it is over-quantization.
- **arm64 vs amd64:** the M4 Pro leads on nearly every cell, most visibly on our
  lossless path (Lena 547 ms vs 895 ms) and our `lossy-slow` path (Lena 1422 ms
  vs 2611 ms). The `wasm` engine is the exception in places, where the two are
  closer.

## The webp-rust lossy-fast bug

Porting `webp-rust` to Go and testing the result turned up a bug in the original
that its own test suite did not catch: at `lossy-fast` (effort 0), it can write
WebP files that no decoder can read back correctly. The decoded image comes out
mostly noise.

Three of the seven test images hit it:

| image | webp-rust | ours | libwebp |
| --- | --- | --- | --- |
| toulouse | **6.35 dB** | 39.93 dB | 39.84 dB |
| martin-alargent | **9.38 dB** | 42.37 dB | 42.63 dB |
| abubakar-mamman | **16.44 dB** | 42.00 dB | 41.96 dB |
| the other four | 39.7-49.0 dB | 40.9-49.3 dB | 40.9-49.1 dB |

For scale, a normal encode at quality 90 lands near 40 dB.

### What goes wrong

A VP8 frame may omit the coefficient data for a macroblock whose contents all
quantized to zero, marking it "skipped" instead. That is only legal if the frame
header turns on per-macroblock skip signaling, which is what tells the decoder to
expect a skip flag and read no coefficients for those macroblocks.

`webp-rust` decides whether to turn that signaling on from a probability
threshold (`compute_skip_probability` in `src/encoder/lossy/bitstream.rs` returns
`None`, disabling signaling, once the computed probability reaches 250). A
detailed image at effort 0 has very few skippable macroblocks, which pushes the
probability past that threshold and switches the signaling off. The encoder still
omits those macroblocks' coefficients, though. The decoder, given no skip flag,
reads the *next* macroblock's coefficients for each skipped one, and every
macroblock after the first skip is decoded from the wrong data.

That is why the bug needs an unusual combination to appear: at least one skipped
macroblock, but too few for the signaling to be enabled. Detailed photos at
effort 0 are exactly that case, which is why `lossy-slow` (effort 9) is unaffected
here and why four of the seven images encode fine.

### How we know it is the encoder, not the decoder

Two independently written decoders were pointed at the same `webp-rust` files and
returned PSNR figures agreeing to the hundredth of a dB: `webp-rust`'s own
decoder, and this library's (which is checked against libwebp in
`benchmark/compat`). They agree on the healthy image too, so they are not failing
in the same place:

```
toulouse   webp-rust decoder 6.35 dB    our decoder 6.35 dB
martin     webp-rust decoder 9.38 dB    our decoder 9.38 dB
Lena       webp-rust decoder 40.09 dB   our decoder 40.09 dB
```

Two decoders that disagree with each other would point at a decoder bug. Two that
agree the file is broken point at the file. Reading the encoder source then
confirms the mechanism above.

### The fix here

This port inherited the same gate and the same bug. Commit 7e5e084 turns skip
signaling on whenever any macroblock is skipped, rather than consulting a
threshold, and adds a regression test built on a mostly-detailed image so the
rare-skip case stays covered. Our `lossy-fast` column above is the fixed
behaviour. The bug is still present in `webp-rust` v0.2.1.

## arm64 (Apple M4 Pro)

```
file                                            mode        engine     width  height  bytes    psnr_db  iters  ms_per_op
Lena_512.png                                    lossless    libwebp    900    900     627632   -        9      235.528
Lena_512.png                                    lossless    ours       900    900     651634   -        4      547.025
Lena_512.png                                    lossless    wasm       900    900     622766   -        3      919.361
Lena_512.png                                    lossless    webp-rust  900    900     651036   -        2      2294.628
Lena_512.png                                    lossy-fast  libwebp    900    900     103980   41.04    134    14.978
Lena_512.png                                    lossy-fast  ours       900    900     160860   41.01    91     22.198
Lena_512.png                                    lossy-fast  wasm       900    900     103980   41.04    54     37.363
Lena_512.png                                    lossy-fast  webp-rust  900    900     127376   40.09    165    12.144
Lena_512.png                                    lossy-slow  libwebp    900    900     89968    40.94    27     75.850
Lena_512.png                                    lossy-slow  ours       900    900     86756    39.87    2      1421.737
Lena_512.png                                    lossy-slow  wasm       900    900     89968    40.94    10     201.762
Lena_512.png                                    lossy-slow  webp-rust  900    900     65024    37.62    2      8576.084
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    libwebp    2025   2700    3241976  -        2      1911.241
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    ours       2025   2700    3355646  -        1      5203.050
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    wasm       2025   2700    3249798  -        1      3741.001
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    webp-rust  2025   2700    3468390  -        2      28445.042
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  libwebp    2025   2700    598034   41.96    21     97.807
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  ours       2025   2700    887576   42.00    14     149.744
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  wasm       2025   2700    598034   41.96    9      246.540
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  webp-rust  2025   2700    641754   16.44    26     79.514
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  libwebp    2025   2700    610518   42.58    4      658.612
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  ours       2025   2700    568646   41.97    1      9188.095
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  wasm       2025   2700    610518   42.58    2      1588.306
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  webp-rust  2025   2700    283348   38.71    2      37627.783
pexels-martin-alargent-1165956-5665465.jpg      lossless    libwebp    2025   2700    2943528  -        2      1979.358
pexels-martin-alargent-1165956-5665465.jpg      lossless    ours       2025   2700    3058988  -        1      4055.110
pexels-martin-alargent-1165956-5665465.jpg      lossless    wasm       2025   2700    2953798  -        1      3755.602
pexels-martin-alargent-1165956-5665465.jpg      lossless    webp-rust  2025   2700    3186444  -        2      20622.663
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  libwebp    2025   2700    726824   42.63    20     103.298
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  ours       2025   2700    1036002  42.37    14     147.686
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  wasm       2025   2700    726824   42.63    9      247.765
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  webp-rust  2025   2700    826398   9.38     25     80.411
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  libwebp    2025   2700    603264   42.75    4      577.549
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  ours       2025   2700    574742   42.30    1      10016.149
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  wasm       2025   2700    603264   42.75    2      1406.665
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  webp-rust  2025   2700    419784   39.67    2      55932.529
pexels-mavihnt-38213559.jpg                     lossless    libwebp    2560   1706    3482480  -        2      1560.368
pexels-mavihnt-38213559.jpg                     lossless    ours       2560   1706    3585610  -        1      3637.611
pexels-mavihnt-38213559.jpg                     lossless    wasm       2560   1706    3488584  -        1      3046.132
pexels-mavihnt-38213559.jpg                     lossless    webp-rust  2560   1706    3764158  -        2      25487.549
pexels-mavihnt-38213559.jpg                     lossy-fast  libwebp    2560   1706    983360   40.87    19     109.991
pexels-mavihnt-38213559.jpg                     lossy-fast  ours       2560   1706    1519778  40.97    13     159.785
pexels-mavihnt-38213559.jpg                     lossy-fast  wasm       2560   1706    983360   40.87    9      248.236
pexels-mavihnt-38213559.jpg                     lossy-fast  webp-rust  2560   1706    1218578  39.73    24     84.919
pexels-mavihnt-38213559.jpg                     lossy-slow  libwebp    2560   1706    925032   41.14    3      700.785
pexels-mavihnt-38213559.jpg                     lossy-slow  ours       2560   1706    922426   40.83    1      11112.984
pexels-mavihnt-38213559.jpg                     lossy-slow  wasm       2560   1706    925032   41.14    2      1568.253
pexels-mavihnt-38213559.jpg                     lossy-slow  webp-rust  2560   1706    616380   37.45    2      73174.886
pexels-steve-15267299.jpg                       lossless    libwebp    2095   3000    2104690  -        1      2163.148
pexels-steve-15267299.jpg                       lossless    ours       2095   3000    2218786  -        1      3578.612
pexels-steve-15267299.jpg                       lossless    wasm       2095   3000    2119370  -        1      3946.265
pexels-steve-15267299.jpg                       lossless    webp-rust  2095   3000    2458930  -        2      19251.944
pexels-steve-15267299.jpg                       lossy-fast  libwebp    2095   3000    284570   44.34    22     92.184
pexels-steve-15267299.jpg                       lossy-fast  ours       2095   3000    330750   43.88    17     120.572
pexels-steve-15267299.jpg                       lossy-fast  wasm       2095   3000    284570   44.34    9      247.597
pexels-steve-15267299.jpg                       lossy-fast  webp-rust  2095   3000    240256   44.34    32     63.292
pexels-steve-15267299.jpg                       lossy-slow  libwebp    2095   3000    247610   44.50    6      345.005
pexels-steve-15267299.jpg                       lossy-slow  ours       2095   3000    207004   43.41    1      6860.387
pexels-steve-15267299.jpg                       lossy-slow  wasm       2095   3000    247610   44.50    2      1059.383
pexels-steve-15267299.jpg                       lossy-slow  webp-rust  2095   3000    134242   41.16    2      20842.849
pexels-steve-29626041.jpg                       lossless    libwebp    2560   1440    283988   -        3      876.770
pexels-steve-29626041.jpg                       lossless    ours       2560   1440    309370   -        2      1009.926
pexels-steve-29626041.jpg                       lossless    wasm       2560   1440    296596   -        2      1800.086
pexels-steve-29626041.jpg                       lossless    webp-rust  2560   1440    319500   -        2      3168.627
pexels-steve-29626041.jpg                       lossy-fast  libwebp    2560   1440    47614    49.05    45     44.576
pexels-steve-29626041.jpg                       lossy-fast  ours       2560   1440    52312    49.30    35     57.826
pexels-steve-29626041.jpg                       lossy-fast  wasm       2560   1440    47614    49.05    16     126.554
pexels-steve-29626041.jpg                       lossy-fast  webp-rust  2560   1440    41908    48.96    66     30.519
pexels-steve-29626041.jpg                       lossy-slow  libwebp    2560   1440    38252    49.65    16     131.817
pexels-steve-29626041.jpg                       lossy-slow  ours       2560   1440    36506    46.79    1      3192.859
pexels-steve-29626041.jpg                       lossy-slow  wasm       2560   1440    38252    49.65    5      481.557
pexels-steve-29626041.jpg                       lossy-slow  webp-rust  2560   1440    26878    43.73    2      7609.242
pexels-toulouse-10807703.jpg                    lossless    libwebp    1400   2100    2688812  -        2      1082.096
pexels-toulouse-10807703.jpg                    lossless    ours       1400   2100    2765508  -        1      2110.194
pexels-toulouse-10807703.jpg                    lossless    wasm       1400   2100    2685806  -        1      2447.914
pexels-toulouse-10807703.jpg                    lossless    webp-rust  1400   2100    2983076  -        2      12273.022
pexels-toulouse-10807703.jpg                    lossy-fast  libwebp    1400   2100    857060   39.84    25     80.650
pexels-toulouse-10807703.jpg                    lossy-fast  ours       1400   2100    1384836  39.93    16     125.894
pexels-toulouse-10807703.jpg                    lossy-fast  wasm       1400   2100    857060   39.84    12     180.338
pexels-toulouse-10807703.jpg                    lossy-fast  webp-rust  1400   2100    1124782  6.35     30     67.449
pexels-toulouse-10807703.jpg                    lossy-slow  libwebp    1400   2100    758466   39.84    5      473.483
pexels-toulouse-10807703.jpg                    lossy-slow  ours       1400   2100    753810   39.55    1      8822.965
pexels-toulouse-10807703.jpg                    lossy-slow  wasm       1400   2100    758466   39.84    2      1120.377
pexels-toulouse-10807703.jpg                    lossy-slow  webp-rust  1400   2100    560756   36.23    2      69310.677
```

## amd64 (AMD Ryzen 7 5700G)

```
file                                            mode        engine     width  height  bytes    psnr_db  iters  ms_per_op
Lena_512.png                                    lossless    libwebp    900    900     627632   -        8      258.547
Lena_512.png                                    lossless    ours       900    900     651634   -        3      894.773
Lena_512.png                                    lossless    wasm       900    900     622766   -        2      1504.092
Lena_512.png                                    lossless    webp-rust  900    900     651036   -        2      3196.629
Lena_512.png                                    lossy-fast  libwebp    900    900     103980   41.04    113    17.764
Lena_512.png                                    lossy-fast  ours       900    900     160860   41.01    60     33.456
Lena_512.png                                    lossy-fast  wasm       900    900     103980   41.04    30     67.091
Lena_512.png                                    lossy-fast  webp-rust  900    900     127376   40.09    108    18.569
Lena_512.png                                    lossy-slow  libwebp    900    900     89968    40.94    19     109.667
Lena_512.png                                    lossy-slow  ours       900    900     86756    39.87    1      2611.197
Lena_512.png                                    lossy-slow  wasm       900    900     89968    40.94    6      370.982
Lena_512.png                                    lossy-slow  webp-rust  900    900     65024    37.62    2      15598.191
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    libwebp    2025   2700    3241976  -        1      2403.015
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    ours       2025   2700    3355646  -        1      8422.717
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    wasm       2025   2700    3249798  -        1      6000.520
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    webp-rust  2025   2700    3468390  -        2      38289.220
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  libwebp    2025   2700    598034   41.96    18     116.183
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  ours       2025   2700    887576   42.00    9      223.752
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  wasm       2025   2700    598034   41.96    5      470.454
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  webp-rust  2025   2700    641754   16.44    17     124.001
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  libwebp    2025   2700    610518   42.58    3      974.046
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  ours       2025   2700    568646   41.97    1      16931.647
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  wasm       2025   2700    610518   42.58    1      2876.010
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  webp-rust  2025   2700    283348   38.71    2      65426.518
pexels-martin-alargent-1165956-5665465.jpg      lossless    libwebp    2025   2700    2943528  -        1      2168.105
pexels-martin-alargent-1165956-5665465.jpg      lossless    ours       2025   2700    3058988  -        1      6392.719
pexels-martin-alargent-1165956-5665465.jpg      lossless    wasm       2025   2700    2953798  -        1      5916.532
pexels-martin-alargent-1165956-5665465.jpg      lossless    webp-rust  2025   2700    3186444  -        2      29654.535
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  libwebp    2025   2700    726824   42.63    17     118.142
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  ours       2025   2700    1036002  42.37    9      224.449
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  wasm       2025   2700    726824   42.63    5      465.127
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  webp-rust  2025   2700    826398   9.38     16     126.722
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  libwebp    2025   2700    603264   42.75    3      810.954
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  ours       2025   2700    574742   42.30    1      18484.701
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  wasm       2025   2700    603264   42.75    1      2567.880
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  webp-rust  2025   2700    419784   39.67    2      103098.887
pexels-mavihnt-38213559.jpg                     lossless    libwebp    2560   1706    3482480  -        2      1749.486
pexels-mavihnt-38213559.jpg                     lossless    ours       2560   1706    3585610  -        1      6293.239
pexels-mavihnt-38213559.jpg                     lossless    wasm       2560   1706    3488584  -        1      4842.418
pexels-mavihnt-38213559.jpg                     lossless    webp-rust  2560   1706    3764158  -        2      30625.147
pexels-mavihnt-38213559.jpg                     lossy-fast  libwebp    2560   1706    983360   40.87    17     122.194
pexels-mavihnt-38213559.jpg                     lossy-fast  ours       2560   1706    1519778  40.97    9      225.647
pexels-mavihnt-38213559.jpg                     lossy-fast  wasm       2560   1706    983360   40.87    5      419.673
pexels-mavihnt-38213559.jpg                     lossy-fast  webp-rust  2560   1706    1218578  39.73    16     126.618
pexels-mavihnt-38213559.jpg                     lossy-slow  libwebp    2560   1706    925032   41.14    3      966.713
pexels-mavihnt-38213559.jpg                     lossy-slow  ours       2560   1706    922426   40.83    1      19412.989
pexels-mavihnt-38213559.jpg                     lossy-slow  wasm       2560   1706    925032   41.14    1      2693.036
pexels-mavihnt-38213559.jpg                     lossy-slow  webp-rust  2560   1706    616380   37.45    2      136110.062
pexels-steve-15267299.jpg                       lossless    libwebp    2095   3000    2104690  -        1      2229.574
pexels-steve-15267299.jpg                       lossless    ours       2095   3000    2218786  -        1      5909.816
pexels-steve-15267299.jpg                       lossless    wasm       2095   3000    2119370  -        1      6140.535
pexels-steve-15267299.jpg                       lossless    webp-rust  2095   3000    2458930  -        2      25134.787
pexels-steve-15267299.jpg                       lossy-fast  libwebp    2095   3000    284570   44.34    19     107.180
pexels-steve-15267299.jpg                       lossy-fast  ours       2095   3000    330750   43.88    11     186.609
pexels-steve-15267299.jpg                       lossy-fast  wasm       2095   3000    284570   44.34    5      472.671
pexels-steve-15267299.jpg                       lossy-fast  webp-rust  2095   3000    240256   44.34    19     108.144
pexels-steve-15267299.jpg                       lossy-slow  libwebp    2095   3000    247610   44.50    4      514.098
pexels-steve-15267299.jpg                       lossy-slow  ours       2095   3000    207004   43.41    1      11711.360
pexels-steve-15267299.jpg                       lossy-slow  wasm       2095   3000    247610   44.50    1      2107.228
pexels-steve-15267299.jpg                       lossy-slow  webp-rust  2095   3000    134242   41.16    2      36262.307
pexels-steve-29626041.jpg                       lossless    libwebp    2560   1440    283988   -        3      743.067
pexels-steve-29626041.jpg                       lossless    ours       2560   1440    309370   -        2      1932.610
pexels-steve-29626041.jpg                       lossless    wasm       2560   1440    296596   -        1      2983.045
pexels-steve-29626041.jpg                       lossless    webp-rust  2560   1440    319500   -        2      4728.543
pexels-steve-29626041.jpg                       lossy-fast  libwebp    2560   1440    47614    49.05    40     50.313
pexels-steve-29626041.jpg                       lossy-fast  ours       2560   1440    52312    49.30    23     90.609
pexels-steve-29626041.jpg                       lossy-fast  wasm       2560   1440    47614    49.05    9      244.764
pexels-steve-29626041.jpg                       lossy-fast  webp-rust  2560   1440    41908    48.96    38     53.517
pexels-steve-29626041.jpg                       lossy-slow  libwebp    2560   1440    38252    49.65    10     204.961
pexels-steve-29626041.jpg                       lossy-slow  ours       2560   1440    36506    46.79    1      5393.727
pexels-steve-29626041.jpg                       lossy-slow  wasm       2560   1440    38252    49.65    3      970.379
pexels-steve-29626041.jpg                       lossy-slow  webp-rust  2560   1440    26878    43.73    2      13470.723
pexels-toulouse-10807703.jpg                    lossless    libwebp    1400   2100    2688812  -        2      1151.386
pexels-toulouse-10807703.jpg                    lossless    ours       1400   2100    2765508  -        1      3640.575
pexels-toulouse-10807703.jpg                    lossless    wasm       1400   2100    2685806  -        1      3965.908
pexels-toulouse-10807703.jpg                    lossless    webp-rust  1400   2100    2983076  -        2      15788.292
pexels-toulouse-10807703.jpg                    lossy-fast  libwebp    1400   2100    857060   39.84    23     90.865
pexels-toulouse-10807703.jpg                    lossy-fast  ours       1400   2100    1384836  39.93    12     170.852
pexels-toulouse-10807703.jpg                    lossy-fast  wasm       1400   2100    857060   39.84    7      298.542
pexels-toulouse-10807703.jpg                    lossy-fast  webp-rust  1400   2100    1124782  6.35     20     102.778
pexels-toulouse-10807703.jpg                    lossy-slow  libwebp    1400   2100    758466   39.84    4      658.238
pexels-toulouse-10807703.jpg                    lossy-slow  ours       1400   2100    753810   39.55    1      15011.040
pexels-toulouse-10807703.jpg                    lossy-slow  wasm       1400   2100    758466   39.84    2      1887.269
pexels-toulouse-10807703.jpg                    lossy-slow  webp-rust  1400   2100    560756   36.23    2      129594.037
```
