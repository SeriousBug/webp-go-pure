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
- **Library commit:** ca39c92 · **Date:** 2026-07-25
- **Budget:** 2000 ms/measurement, lossy quality 90

Engines: `ours` (pure Go), `libwebp` (C, cgo), `wasm` (libwebp via WASM, cgo-free),
`webp-rust` (the Rust original). See `README.md` for the exact mode settings.

`psnr_db` decodes each encoder's own output and scores it against the pixels that
encoder was handed, over RGB. Higher is better, and quality 90 lands near 40 dB.
Lossless is exact, so it shows `-`. Size without quality is not comparable: an
encoder can always make a smaller file by quantizing harder, so the two columns
have to be read together.

Encoder output is identical on both machines (every size in the two tables
matches), so the `psnr_db` column reads the same in both and only the timings
differ.

## Reading these numbers

- **vs libwebp, `lossy-slow`:** sizes land at 0.94-1.11x libwebp's, at 0.0-0.8 dB
  lower quality, for 1.3-2.4x the encode time.
- **vs libwebp, `lossy-fast`:** quality matches within ±0.5 dB, sizes run
  0.96-1.21x, and we take 1.1-1.8x the time. Effort 0 is still where our files
  are furthest behind on size.
- **vs libwebp, lossless:** we are 3-9% bigger and 1.1-3.1x slower.
- **`libwebp` and `wasm` are the same encoder.** Their lossy output is
  byte-identical (verified by hash), so their sizes and PSNR match exactly;
  `wasm` is the cgo-free option and runs ~2-6x slower than native `libwebp`.
- **`webp-rust` `lossy-fast` output is corrupt on 3 of 7 images**, a bug this
  port found and fixed. See "The webp-rust lossy-fast bug" below.
- **`webp-rust` `lossy-slow` files are smaller than everyone's, at 1.5-3 dB lower
  quality** (e.g. toulouse 560756 B at 36.23 dB vs our 761160 B at 39.75 dB).
  An earlier capture of these results read that size advantage as a compression
  win; the PSNR column shows it is over-quantization.

## Charts

![Size and quality against libwebp: one point per test image, with output size relative to libwebp on the x axis and PSNR difference on the y axis, faceted by lossy mode](charts/rate-distortion-light.svg#gh-light-mode-only)
![Size and quality against libwebp: one point per test image, with output size relative to libwebp on the x axis and PSNR difference on the y axis, faceted by lossy mode](charts/rate-distortion-dark.svg#gh-dark-mode-only)

![Encode time per image for each engine, one panel per mode and machine, with bars that run off the panel drawn fading out under an arrow](charts/encode-time-light.svg#gh-light-mode-only)
![Encode time per image for each engine, one panel per mode and machine, with bars that run off the panel drawn fading out under an arrow](charts/encode-time-dark.svg#gh-dark-mode-only)

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
file                                            mode        engine     width  height  bytes    psnr_db  iters  ms_per_op
Lena_512.png                                    lossless    libwebp    900    900     627632   -        9      244.224
Lena_512.png                                    lossless    ours       900    900     651634   -        5      456.901
Lena_512.png                                    lossless    wasm       900    900     622766   -        3      922.323
Lena_512.png                                    lossless    webp-rust  900    900     651036   -        2      2176.986
Lena_512.png                                    lossy-fast  libwebp    900    900     103980   41.04    136    14.794
Lena_512.png                                    lossy-fast  ours       900    900     113182   41.02    88     22.737
Lena_512.png                                    lossy-fast  wasm       900    900     103980   41.04    54     37.080
Lena_512.png                                    lossy-fast  webp-rust  900    900     127376   40.09    157    12.767
Lena_512.png                                    lossy-slow  libwebp    900    900     89968    40.94    27     74.293
Lena_512.png                                    lossy-slow  ours       900    900     88890    40.16    14     151.193
Lena_512.png                                    lossy-slow  wasm       900    900     89968    40.94    11     195.335
Lena_512.png                                    lossy-slow  webp-rust  900    900     65024    37.62    2      8401.528
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    libwebp    2025   2700    3241976  -        2      1834.335
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    ours       2025   2700    3355646  -        1      3785.395
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    wasm       2025   2700    3249798  -        1      3620.912
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    webp-rust  2025   2700    3468390  -        2      24945.454
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  libwebp    2025   2700    598034   41.96    22     94.398
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  ours       2025   2700    723064   42.02    14     150.473
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  wasm       2025   2700    598034   41.96    9      241.963
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  webp-rust  2025   2700    641754   16.44    26     76.931
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  libwebp    2025   2700    610518   42.58    4      651.967
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  ours       2025   2700    592722   42.26    2      1351.379
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  wasm       2025   2700    610518   42.58    2      1615.864
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  webp-rust  2025   2700    283348   38.71    2      36872.957
pexels-martin-alargent-1165956-5665465.jpg      lossless    libwebp    2025   2700    2943528  -        2      2005.608
pexels-martin-alargent-1165956-5665465.jpg      lossless    ours       2025   2700    3058988  -        1      3214.884
pexels-martin-alargent-1165956-5665465.jpg      lossless    wasm       2025   2700    2953798  -        1      3722.848
pexels-martin-alargent-1165956-5665465.jpg      lossless    webp-rust  2025   2700    3186444  -        2      18925.894
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  libwebp    2025   2700    726824   42.63    20     104.215
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  ours       2025   2700    756806   42.39    14     148.486
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  wasm       2025   2700    726824   42.63    9      247.973
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  webp-rust  2025   2700    826398   9.38     25     80.045
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  libwebp    2025   2700    603264   42.75    4      569.387
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  ours       2025   2700    601454   42.73    2      1033.090
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  wasm       2025   2700    603264   42.75    2      1403.076
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  webp-rust  2025   2700    419784   39.67    2      55476.772
pexels-mavihnt-38213559.jpg                     lossless    libwebp    2560   1706    3482480  -        2      1526.860
pexels-mavihnt-38213559.jpg                     lossless    ours       2560   1706    3585610  -        1      3190.672
pexels-mavihnt-38213559.jpg                     lossless    wasm       2560   1706    3488584  -        1      2970.564
pexels-mavihnt-38213559.jpg                     lossless    webp-rust  2560   1706    3764158  -        2      18135.590
pexels-mavihnt-38213559.jpg                     lossy-fast  libwebp    2560   1706    983360   40.87    19     105.550
pexels-mavihnt-38213559.jpg                     lossy-fast  ours       2560   1706    1037210  40.99    12     168.550
pexels-mavihnt-38213559.jpg                     lossy-fast  wasm       2560   1706    983360   40.87    9      243.168
pexels-mavihnt-38213559.jpg                     lossy-fast  webp-rust  2560   1706    1218578  39.73    24     84.026
pexels-mavihnt-38213559.jpg                     lossy-slow  libwebp    2560   1706    925032   41.14    3      668.768
pexels-mavihnt-38213559.jpg                     lossy-slow  ours       2560   1706    935652   41.05    2      1310.432
pexels-mavihnt-38213559.jpg                     lossy-slow  wasm       2560   1706    925032   41.14    2      1554.691
pexels-mavihnt-38213559.jpg                     lossy-slow  webp-rust  2560   1706    616380   37.45    2      71533.276
pexels-steve-15267299.jpg                       lossless    libwebp    2095   3000    2104690  -        1      2097.022
pexels-steve-15267299.jpg                       lossless    ours       2095   3000    2218786  -        1      3083.895
pexels-steve-15267299.jpg                       lossless    wasm       2095   3000    2119370  -        1      3886.210
pexels-steve-15267299.jpg                       lossless    webp-rust  2095   3000    2458930  -        2      16273.216
pexels-steve-15267299.jpg                       lossy-fast  libwebp    2095   3000    284570   44.34    23     89.355
pexels-steve-15267299.jpg                       lossy-fast  ours       2095   3000    294606   43.91    19     110.760
pexels-steve-15267299.jpg                       lossy-fast  wasm       2095   3000    284570   44.34    9      239.214
pexels-steve-15267299.jpg                       lossy-fast  webp-rust  2095   3000    240256   44.34    32     63.948
pexels-steve-15267299.jpg                       lossy-slow  libwebp    2095   3000    247610   44.50    6      336.652
pexels-steve-15267299.jpg                       lossy-slow  ours       2095   3000    233446   44.04    3      814.609
pexels-steve-15267299.jpg                       lossy-slow  wasm       2095   3000    247610   44.50    2      1046.448
pexels-steve-15267299.jpg                       lossy-slow  webp-rust  2095   3000    134242   41.16    2      20255.249
pexels-steve-29626041.jpg                       lossless    libwebp    2560   1440    283988   -        3      832.333
pexels-steve-29626041.jpg                       lossless    ours       2560   1440    309370   -        3      893.327
pexels-steve-29626041.jpg                       lossless    wasm       2560   1440    296596   -        2      1750.028
pexels-steve-29626041.jpg                       lossless    webp-rust  2560   1440    319500   -        2      2826.569
pexels-steve-29626041.jpg                       lossy-fast  libwebp    2560   1440    47614    49.05    47     42.586
pexels-steve-29626041.jpg                       lossy-fast  ours       2560   1440    45686    49.31    43     47.087
pexels-steve-29626041.jpg                       lossy-fast  wasm       2560   1440    47614    49.05    17     123.776
pexels-steve-29626041.jpg                       lossy-fast  webp-rust  2560   1440    41908    48.96    69     29.218
pexels-steve-29626041.jpg                       lossy-slow  libwebp    2560   1440    38252    49.65    16     129.734
pexels-steve-29626041.jpg                       lossy-slow  ours       2560   1440    42614    49.25    9      249.803
pexels-steve-29626041.jpg                       lossy-slow  wasm       2560   1440    38252    49.65    5      473.415
pexels-steve-29626041.jpg                       lossy-slow  webp-rust  2560   1440    26878    43.73    2      7380.680
pexels-toulouse-10807703.jpg                    lossless    libwebp    1400   2100    2688812  -        2      1024.124
pexels-toulouse-10807703.jpg                    lossless    ours       1400   2100    2765508  -        2      1783.025
pexels-toulouse-10807703.jpg                    lossless    wasm       1400   2100    2685806  -        1      2357.668
pexels-toulouse-10807703.jpg                    lossless    webp-rust  1400   2100    2983076  -        2      9778.844
pexels-toulouse-10807703.jpg                    lossy-fast  libwebp    1400   2100    857060   39.84    27     76.474
pexels-toulouse-10807703.jpg                    lossy-fast  ours       1400   2100    854742   39.93    16     130.536
pexels-toulouse-10807703.jpg                    lossy-fast  wasm       1400   2100    857060   39.84    12     175.230
pexels-toulouse-10807703.jpg                    lossy-fast  webp-rust  1400   2100    1124782  6.35     32     63.932
pexels-toulouse-10807703.jpg                    lossy-slow  libwebp    1400   2100    758466   39.84    5      451.014
pexels-toulouse-10807703.jpg                    lossy-slow  ours       1400   2100    761160   39.75    4      662.210
pexels-toulouse-10807703.jpg                    lossy-slow  wasm       1400   2100    758466   39.84    2      1074.568
pexels-toulouse-10807703.jpg                    lossy-slow  webp-rust  1400   2100    560756   36.23    2      67502.223
```

## amd64 (AMD Ryzen 7 5700G)

```
file                                            mode        engine     width  height  bytes    psnr_db  iters  ms_per_op
Lena_512.png                                    lossless    libwebp    900    900     627632   -        8      254.225
Lena_512.png                                    lossless    ours       900    900     651634   -        3      690.926
Lena_512.png                                    lossless    wasm       900    900     622766   -        2      1526.018
Lena_512.png                                    lossless    webp-rust  900    900     651036   -        2      3241.434
Lena_512.png                                    lossy-fast  libwebp    900    900     103980   41.04    113    17.834
Lena_512.png                                    lossy-fast  ours       900    900     113182   41.02    69     29.001
Lena_512.png                                    lossy-fast  wasm       900    900     103980   41.04    30     67.600
Lena_512.png                                    lossy-fast  webp-rust  900    900     127376   40.09    108    18.554
Lena_512.png                                    lossy-slow  libwebp    900    900     89968    40.94    19     110.734
Lena_512.png                                    lossy-slow  ours       900    900     88890    40.16    10     201.715
Lena_512.png                                    lossy-slow  wasm       900    900     89968    40.94    6      364.717
Lena_512.png                                    lossy-slow  webp-rust  900    900     65024    37.62    2      15705.937
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    libwebp    2025   2700    3241976  -        1      2168.112
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    ours       2025   2700    3355646  -        1      6692.831
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    wasm       2025   2700    3249798  -        1      5933.806
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    webp-rust  2025   2700    3468390  -        2      39600.994
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  libwebp    2025   2700    598034   41.96    18     115.944
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  ours       2025   2700    723064   42.02    10     202.135
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  wasm       2025   2700    598034   41.96    5      462.124
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  webp-rust  2025   2700    641754   16.44    17     123.677
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  libwebp    2025   2700    610518   42.58    3      979.744
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  ours       2025   2700    592722   42.26    2      1696.579
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  wasm       2025   2700    610518   42.58    1      2882.819
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  webp-rust  2025   2700    283348   38.71    2      65479.124
pexels-martin-alargent-1165956-5665465.jpg      lossless    libwebp    2025   2700    2943528  -        1      2193.645
pexels-martin-alargent-1165956-5665465.jpg      lossless    ours       2025   2700    3058988  -        1      5107.244
pexels-martin-alargent-1165956-5665465.jpg      lossless    wasm       2025   2700    2953798  -        1      5841.734
pexels-martin-alargent-1165956-5665465.jpg      lossless    webp-rust  2025   2700    3186444  -        2      28541.315
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  libwebp    2025   2700    726824   42.63    17     118.638
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  ours       2025   2700    756806   42.39    11     199.889
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  wasm       2025   2700    726824   42.63    5      476.840
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  webp-rust  2025   2700    826398   9.38     16     126.867
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  libwebp    2025   2700    603264   42.75    3      921.591
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  ours       2025   2700    601454   42.73    2      1392.372
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  wasm       2025   2700    603264   42.75    1      2573.792
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  webp-rust  2025   2700    419784   39.67    2      103594.188
pexels-mavihnt-38213559.jpg                     lossless    libwebp    2560   1706    3482480  -        2      1631.890
pexels-mavihnt-38213559.jpg                     lossless    ours       2560   1706    3585610  -        1      4888.119
pexels-mavihnt-38213559.jpg                     lossless    wasm       2560   1706    3488584  -        1      4711.360
pexels-mavihnt-38213559.jpg                     lossless    webp-rust  2560   1706    3764158  -        2      29992.056
pexels-mavihnt-38213559.jpg                     lossy-fast  libwebp    2560   1706    983360   40.87    17     122.403
pexels-mavihnt-38213559.jpg                     lossy-fast  ours       2560   1706    1037210  40.99    10     213.710
pexels-mavihnt-38213559.jpg                     lossy-fast  wasm       2560   1706    983360   40.87    5      415.427
pexels-mavihnt-38213559.jpg                     lossy-fast  webp-rust  2560   1706    1218578  39.73    15     135.000
pexels-mavihnt-38213559.jpg                     lossy-slow  libwebp    2560   1706    925032   41.14    3      970.375
pexels-mavihnt-38213559.jpg                     lossy-slow  ours       2560   1706    935652   41.05    2      1611.168
pexels-mavihnt-38213559.jpg                     lossy-slow  wasm       2560   1706    925032   41.14    1      2723.815
pexels-mavihnt-38213559.jpg                     lossy-slow  webp-rust  2560   1706    616380   37.45    2      136788.552
pexels-steve-15267299.jpg                       lossless    libwebp    2095   3000    2104690  -        1      2170.022
pexels-steve-15267299.jpg                       lossless    ours       2095   3000    2218786  -        1      4604.314
pexels-steve-15267299.jpg                       lossless    wasm       2095   3000    2119370  -        1      6154.199
pexels-steve-15267299.jpg                       lossless    webp-rust  2095   3000    2458930  -        2      25127.369
pexels-steve-15267299.jpg                       lossy-fast  libwebp    2095   3000    284570   44.34    19     106.293
pexels-steve-15267299.jpg                       lossy-fast  ours       2095   3000    294606   43.91    13     155.951
pexels-steve-15267299.jpg                       lossy-fast  wasm       2095   3000    284570   44.34    5      460.404
pexels-steve-15267299.jpg                       lossy-fast  webp-rust  2095   3000    240256   44.34    19     108.581
pexels-steve-15267299.jpg                       lossy-slow  libwebp    2095   3000    247610   44.50    4      517.374
pexels-steve-15267299.jpg                       lossy-slow  ours       2095   3000    233446   44.04    2      1143.575
pexels-steve-15267299.jpg                       lossy-slow  wasm       2095   3000    247610   44.50    1      2115.149
pexels-steve-15267299.jpg                       lossy-slow  webp-rust  2095   3000    134242   41.16    2      36662.993
pexels-steve-29626041.jpg                       lossless    libwebp    2560   1440    283988   -        3      735.407
pexels-steve-29626041.jpg                       lossless    ours       2560   1440    309370   -        2      1405.749
pexels-steve-29626041.jpg                       lossless    wasm       2560   1440    296596   -        1      2824.592
pexels-steve-29626041.jpg                       lossless    webp-rust  2560   1440    319500   -        2      4778.824
pexels-steve-29626041.jpg                       lossy-fast  libwebp    2560   1440    47614    49.05    38     53.303
pexels-steve-29626041.jpg                       lossy-fast  ours       2560   1440    45686    49.31    30     68.128
pexels-steve-29626041.jpg                       lossy-fast  wasm       2560   1440    47614    49.05    9      246.164
pexels-steve-29626041.jpg                       lossy-fast  webp-rust  2560   1440    41908    48.96    39     52.278
pexels-steve-29626041.jpg                       lossy-slow  libwebp    2560   1440    38252    49.65    10     213.348
pexels-steve-29626041.jpg                       lossy-slow  ours       2560   1440    42614    49.25    6      376.725
pexels-steve-29626041.jpg                       lossy-slow  wasm       2560   1440    38252    49.65    3      998.723
pexels-steve-29626041.jpg                       lossy-slow  webp-rust  2560   1440    26878    43.73    2      13587.041
pexels-toulouse-10807703.jpg                    lossless    libwebp    1400   2100    2688812  -        2      1145.269
pexels-toulouse-10807703.jpg                    lossless    ours       1400   2100    2765508  -        1      2788.712
pexels-toulouse-10807703.jpg                    lossless    wasm       1400   2100    2685806  -        1      3878.861
pexels-toulouse-10807703.jpg                    lossless    webp-rust  1400   2100    2983076  -        2      16069.557
pexels-toulouse-10807703.jpg                    lossy-fast  libwebp    1400   2100    857060   39.84    22     91.233
pexels-toulouse-10807703.jpg                    lossy-fast  ours       1400   2100    854742   39.93    13     161.430
pexels-toulouse-10807703.jpg                    lossy-fast  wasm       1400   2100    857060   39.84    7      293.750
pexels-toulouse-10807703.jpg                    lossy-fast  webp-rust  1400   2100    1124782  6.35     21     97.063
pexels-toulouse-10807703.jpg                    lossy-slow  libwebp    1400   2100    758466   39.84    4      663.444
pexels-toulouse-10807703.jpg                    lossy-slow  ours       1400   2100    761160   39.75    3      846.206
pexels-toulouse-10807703.jpg                    lossy-slow  wasm       1400   2100    758466   39.84    2      1853.874
pexels-toulouse-10807703.jpg                    lossy-slow  webp-rust  1400   2100    560756   36.23    2      130192.840
```
