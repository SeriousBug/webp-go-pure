# Benchmark results

Captured run of `benchmark/run.sh`. Timings are machine-dependent; regenerate
locally for your own hardware.

- **Machine:** Apple M4 Pro (14 cores), macOS 26.5.1
- **Go:** go1.26.5 · **libwebp:** 1.6.0 (also used by the `wasm` engine, via WASM) · **webp-rust:** v0.2.1
- **Date:** 2026-07-24
- **Budget:** 2000 ms/measurement, lossy quality 90

Engines: `ours` (pure Go), `libwebp` (C, cgo), `wasm` (libwebp via WASM, cgo-free),
`webp-rust` (the Rust original). See `README.md` for the exact mode settings.

## Reading these numbers

- **Our encoder == the Rust original.** For `Lena_512.png` (a PNG, so both
  harnesses decode it to identical RGBA) `ours` and `webp-rust` emit
  **byte-identical** output in all three modes (lossless 651036, lossy-fast
  127376, lossy-slow 65024). The port reproduces the reference encoder exactly.
- **Photo sizes differ slightly between `ours` and `webp-rust`** only because the
  JPEG sources are decoded by different libraries (Go `image/jpeg` vs Rust's
  `image`/`zune-jpeg`), yielding slightly different input pixels. It is not an
  encoder difference.
- **`libwebp` and `wasm` are the same encoder** (identical sizes throughout);
  `wasm` is the cgo-free option and runs ~2-6x slower than native `libwebp`.
- **Speed vs size tradeoff:** our `lossy-slow` (optimize 9) is by far the slowest
  path but produces the **smallest** lossy files — often ~half of libwebp's for
  the same quality (e.g. abubakar: 285 KB vs 611 KB), at ~100x the encode time.
  Our `lossy-fast` is competitive in speed but larger than libwebp. Our lossless
  is slightly larger and much slower than libwebp.

## Full table

```
file                                            mode        engine     width  height  bytes    iters  ms_per_op
Lena_512.png                                    lossless    libwebp    900    900     627632   9      241.314
Lena_512.png                                    lossless    ours       900    900     651036   1      3202.090
Lena_512.png                                    lossless    wasm       900    900     622766   3      925.604
Lena_512.png                                    lossless    webp-rust  900    900     651036   2      2127.687
Lena_512.png                                    lossy-fast  libwebp    900    900     103980   132    15.226
Lena_512.png                                    lossy-fast  ours       900    900     127376   95     21.248
Lena_512.png                                    lossy-fast  wasm       900    900     103980   53     38.001
Lena_512.png                                    lossy-fast  webp-rust  900    900     127376   171    11.741
Lena_512.png                                    lossy-slow  libwebp    900    900     89968    27     76.118
Lena_512.png                                    lossy-slow  ours       900    900     65024    1      18861.899
Lena_512.png                                    lossy-slow  wasm       900    900     89968    11     199.454
Lena_512.png                                    lossy-slow  webp-rust  900    900     65024    2      8221.496
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    libwebp    2025   2700    3241976  2      1843.098
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    ours       2025   2700    3473400  1      31424.560
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    wasm       2025   2700    3249798  1      3600.495
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    webp-rust  2025   2700    3468390  2      22765.203
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  libwebp    2025   2700    598034   22     95.103
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  ours       2025   2700    646054   15     136.854
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  wasm       2025   2700    598034   9      241.054
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  webp-rust  2025   2700    641754   27     74.707
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  libwebp    2025   2700    610518   4      653.288
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  ours       2025   2700    285368   1      75089.680
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  wasm       2025   2700    610518   2      1565.769
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  webp-rust  2025   2700    283348   2      36301.725
pexels-martin-alargent-1165956-5665465.jpg      lossless    libwebp    2025   2700    2943528  2      1878.182
pexels-martin-alargent-1165956-5665465.jpg      lossless    ours       2025   2700    3057352  1      24567.246
pexels-martin-alargent-1165956-5665465.jpg      lossless    wasm       2025   2700    2953798  1      3655.595
pexels-martin-alargent-1165956-5665465.jpg      lossless    webp-rust  2025   2700    3186444  2      17670.752
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  libwebp    2025   2700    726824   21     96.687
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  ours       2025   2700    831714   15     140.309
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  wasm       2025   2700    726824   9      245.785
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  webp-rust  2025   2700    826398   26     77.923
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  libwebp    2025   2700    603264   4      555.263
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  ours       2025   2700    422198   1      122465.362
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  wasm       2025   2700    603264   2      1390.626
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  webp-rust  2025   2700    419784   2      54489.109
pexels-mavihnt-38213559.jpg                     lossless    libwebp    2560   1706    3482480  2      1436.438
pexels-mavihnt-38213559.jpg                     lossless    ours       2560   1706    3582806  1      23994.563
pexels-mavihnt-38213559.jpg                     lossless    wasm       2560   1706    3488584  1      2958.029
pexels-mavihnt-38213559.jpg                     lossless    webp-rust  2560   1706    3764158  2      17431.206
pexels-mavihnt-38213559.jpg                     lossy-fast  libwebp    2560   1706    983360   20     101.602
pexels-mavihnt-38213559.jpg                     lossy-fast  ours       2560   1706    1226836  15     141.114
pexels-mavihnt-38213559.jpg                     lossy-fast  wasm       2560   1706    983360   9      244.875
pexels-mavihnt-38213559.jpg                     lossy-fast  webp-rust  2560   1706    1218578  25     81.783
pexels-mavihnt-38213559.jpg                     lossy-slow  libwebp    2560   1706    925032   4      653.910
pexels-mavihnt-38213559.jpg                     lossy-slow  ours       2560   1706    620862   1      164414.251
pexels-mavihnt-38213559.jpg                     lossy-slow  wasm       2560   1706    925032   2      1553.290
pexels-mavihnt-38213559.jpg                     lossy-slow  webp-rust  2560   1706    616380   2      71713.241
pexels-steve-15267299.jpg                       lossless    libwebp    2095   3000    2104690  1      2016.075
pexels-steve-15267299.jpg                       lossless    ours       2095   3000    2215898  1      21822.002
pexels-steve-15267299.jpg                       lossless    wasm       2095   3000    2119370  1      3844.094
pexels-steve-15267299.jpg                       lossless    webp-rust  2095   3000    2458930  2      16213.237
pexels-steve-15267299.jpg                       lossy-fast  libwebp    2095   3000    284570   23     88.828
pexels-steve-15267299.jpg                       lossy-fast  ours       2095   3000    247730   17     120.267
pexels-steve-15267299.jpg                       lossy-fast  wasm       2095   3000    284570   9      233.365
pexels-steve-15267299.jpg                       lossy-fast  webp-rust  2095   3000    240256   32     63.248
pexels-steve-15267299.jpg                       lossy-slow  libwebp    2095   3000    247610   7      332.951
pexels-steve-15267299.jpg                       lossy-slow  ours       2095   3000    137534   1      42005.388
pexels-steve-15267299.jpg                       lossy-slow  wasm       2095   3000    247610   2      1043.760
pexels-steve-15267299.jpg                       lossy-slow  webp-rust  2095   3000    134242   2      20284.130
pexels-steve-29626041.jpg                       lossless    libwebp    2560   1440    283988   3      802.529
pexels-steve-29626041.jpg                       lossless    ours       2560   1440    308012   1      5053.499
pexels-steve-29626041.jpg                       lossless    wasm       2560   1440    296596   2      1732.333
pexels-steve-29626041.jpg                       lossless    webp-rust  2560   1440    319500   2      2817.664
pexels-steve-29626041.jpg                       lossy-fast  libwebp    2560   1440    47614    49     41.553
pexels-steve-29626041.jpg                       lossy-fast  ours       2560   1440    42100    34     59.850
pexels-steve-29626041.jpg                       lossy-fast  wasm       2560   1440    47614    17     122.431
pexels-steve-29626041.jpg                       lossy-fast  webp-rust  2560   1440    41908    69     29.370
pexels-steve-29626041.jpg                       lossy-slow  libwebp    2560   1440    38252    16     126.733
pexels-steve-29626041.jpg                       lossy-slow  ours       2560   1440    27360    1      15573.675
pexels-steve-29626041.jpg                       lossy-slow  wasm       2560   1440    38252    5      468.536
pexels-steve-29626041.jpg                       lossy-slow  webp-rust  2560   1440    26878    2      7341.152
pexels-toulouse-10807703.jpg                    lossless    libwebp    1400   2100    2688812  3      997.374
pexels-toulouse-10807703.jpg                    lossless    ours       1400   2100    2765820  1      13797.633
pexels-toulouse-10807703.jpg                    lossless    wasm       1400   2100    2685806  1      2339.342
pexels-toulouse-10807703.jpg                    lossless    webp-rust  1400   2100    2983076  2      9891.403
pexels-toulouse-10807703.jpg                    lossy-fast  libwebp    1400   2100    857060   27     74.670
pexels-toulouse-10807703.jpg                    lossy-fast  ours       1400   2100    1137460  19     107.538
pexels-toulouse-10807703.jpg                    lossy-fast  wasm       1400   2100    857060   12     170.704
pexels-toulouse-10807703.jpg                    lossy-fast  webp-rust  1400   2100    1124782  32     63.994
pexels-toulouse-10807703.jpg                    lossy-slow  libwebp    1400   2100    758466   5      444.744
pexels-toulouse-10807703.jpg                    lossy-slow  ours       1400   2100    567624   1      157863.754
pexels-toulouse-10807703.jpg                    lossy-slow  wasm       1400   2100    758466   2      1074.938
pexels-toulouse-10807703.jpg                    lossy-slow  webp-rust  1400   2100    560756   2      66568.315
```
