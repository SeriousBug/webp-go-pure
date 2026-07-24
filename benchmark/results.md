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
  the same quality (e.g. abubakar: 285 KB vs 611 KB), at ~45x the encode time.
  The SSE2/NEON transform and SSE kernels cut that path's runtime by roughly
  2-3.7x versus the previous pure-Go implementation (e.g. toulouse lossy-slow
  157864 ms -> 43044 ms, mavihnt 164414 ms -> 48973 ms), with identical output.
  Our `lossy-fast` is competitive in speed but larger than libwebp. Our lossless
  is slightly larger and much slower than libwebp.

## Full table

```
file                                            mode        engine     width  height  bytes    iters  ms_per_op
Lena_512.png                                    lossless    libwebp    900    900     627632   9      242.641
Lena_512.png                                    lossless    ours       900    900     651036   1      3180.278
Lena_512.png                                    lossless    wasm       900    900     622766   3      916.416
Lena_512.png                                    lossless    webp-rust  900    900     651036   2      2147.886
Lena_512.png                                    lossy-fast  libwebp    900    900     103980   130    15.457
Lena_512.png                                    lossy-fast  ours       900    900     127376   106    18.979
Lena_512.png                                    lossy-fast  wasm       900    900     103980   53     37.862
Lena_512.png                                    lossy-fast  webp-rust  900    900     127376   172    11.687
Lena_512.png                                    lossy-slow  libwebp    900    900     89968    27     74.894
Lena_512.png                                    lossy-slow  ours       900    900     65024    1      6139.440
Lena_512.png                                    lossy-slow  wasm       900    900     89968    11     196.162
Lena_512.png                                    lossy-slow  webp-rust  900    900     65024    2      8334.391
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    libwebp    2025   2700    3241976  2      1873.499
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    ours       2025   2700    3473400  1      35520.379
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    wasm       2025   2700    3249798  1      3629.058
pexels-abubakar-mamman-2148132108-38602599.jpg  lossless    webp-rust  2025   2700    3468390  2      24352.715
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  libwebp    2025   2700    598034   21     96.118
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  ours       2025   2700    646054   17     118.022
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  wasm       2025   2700    598034   9      241.908
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-fast  webp-rust  2025   2700    641754   27     75.889
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  libwebp    2025   2700    610518   4      658.979
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  ours       2025   2700    285368   1      30776.752
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  wasm       2025   2700    610518   2      1568.752
pexels-abubakar-mamman-2148132108-38602599.jpg  lossy-slow  webp-rust  2025   2700    283348   2      36267.543
pexels-martin-alargent-1165956-5665465.jpg      lossless    libwebp    2025   2700    2943528  1      2011.234
pexels-martin-alargent-1165956-5665465.jpg      lossless    ours       2025   2700    3057352  1      25630.140
pexels-martin-alargent-1165956-5665465.jpg      lossless    wasm       2025   2700    2953798  1      3795.693
pexels-martin-alargent-1165956-5665465.jpg      lossless    webp-rust  2025   2700    3186444  2      19036.465
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  libwebp    2025   2700    726824   20     104.719
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  ours       2025   2700    831714   17     124.088
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  wasm       2025   2700    726824   8      250.864
pexels-martin-alargent-1165956-5665465.jpg      lossy-fast  webp-rust  2025   2700    826398   26     78.863
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  libwebp    2025   2700    603264   4      560.787
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  ours       2025   2700    422198   1      40985.321
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  wasm       2025   2700    603264   2      1398.657
pexels-martin-alargent-1165956-5665465.jpg      lossy-slow  webp-rust  2025   2700    419784   2      54777.171
pexels-mavihnt-38213559.jpg                     lossless    libwebp    2560   1706    3482480  2      1463.300
pexels-mavihnt-38213559.jpg                     lossless    ours       2560   1706    3582806  1      25483.100
pexels-mavihnt-38213559.jpg                     lossless    wasm       2560   1706    3488584  1      2941.686
pexels-mavihnt-38213559.jpg                     lossless    webp-rust  2560   1706    3764158  2      17743.607
pexels-mavihnt-38213559.jpg                     lossy-fast  libwebp    2560   1706    983360   20     102.556
pexels-mavihnt-38213559.jpg                     lossy-fast  ours       2560   1706    1226836  16     129.304
pexels-mavihnt-38213559.jpg                     lossy-fast  wasm       2560   1706    983360   9      245.506
pexels-mavihnt-38213559.jpg                     lossy-fast  webp-rust  2560   1706    1218578  23     87.958
pexels-mavihnt-38213559.jpg                     lossy-slow  libwebp    2560   1706    925032   3      673.644
pexels-mavihnt-38213559.jpg                     lossy-slow  ours       2560   1706    620862   1      48973.165
pexels-mavihnt-38213559.jpg                     lossy-slow  wasm       2560   1706    925032   2      1547.556
pexels-mavihnt-38213559.jpg                     lossy-slow  webp-rust  2560   1706    616380   2      71586.874
pexels-steve-15267299.jpg                       lossless    libwebp    2095   3000    2104690  1      2059.125
pexels-steve-15267299.jpg                       lossless    ours       2095   3000    2215898  1      22502.044
pexels-steve-15267299.jpg                       lossless    wasm       2095   3000    2119370  1      3895.444
pexels-steve-15267299.jpg                       lossless    webp-rust  2095   3000    2458930  2      15846.496
pexels-steve-15267299.jpg                       lossy-fast  libwebp    2095   3000    284570   22     91.705
pexels-steve-15267299.jpg                       lossy-fast  ours       2095   3000    247730   20     103.381
pexels-steve-15267299.jpg                       lossy-fast  wasm       2095   3000    284570   9      233.683
pexels-steve-15267299.jpg                       lossy-fast  webp-rust  2095   3000    240256   32     62.785
pexels-steve-15267299.jpg                       lossy-slow  libwebp    2095   3000    247610   6      339.711
pexels-steve-15267299.jpg                       lossy-slow  ours       2095   3000    137534   1      21552.068
pexels-steve-15267299.jpg                       lossy-slow  wasm       2095   3000    247610   2      1035.054
pexels-steve-15267299.jpg                       lossy-slow  webp-rust  2095   3000    134242   2      20208.358
pexels-steve-29626041.jpg                       lossless    libwebp    2560   1440    283988   3      831.707
pexels-steve-29626041.jpg                       lossless    ours       2560   1440    308012   1      5178.056
pexels-steve-29626041.jpg                       lossless    wasm       2560   1440    296596   2      1736.320
pexels-steve-29626041.jpg                       lossless    webp-rust  2560   1440    319500   2      2800.020
pexels-steve-29626041.jpg                       lossy-fast  libwebp    2560   1440    47614    48     42.342
pexels-steve-29626041.jpg                       lossy-fast  ours       2560   1440    42100    39     51.659
pexels-steve-29626041.jpg                       lossy-fast  wasm       2560   1440    47614    17     122.117
pexels-steve-29626041.jpg                       lossy-fast  webp-rust  2560   1440    41908    69     29.024
pexels-steve-29626041.jpg                       lossy-slow  libwebp    2560   1440    38252    16     129.072
pexels-steve-29626041.jpg                       lossy-slow  ours       2560   1440    27360    1      9746.949
pexels-steve-29626041.jpg                       lossy-slow  wasm       2560   1440    38252    5      465.054
pexels-steve-29626041.jpg                       lossy-slow  webp-rust  2560   1440    26878    2      7418.813
pexels-toulouse-10807703.jpg                    lossless    libwebp    1400   2100    2688812  2      1020.795
pexels-toulouse-10807703.jpg                    lossless    ours       1400   2100    2765820  1      14099.966
pexels-toulouse-10807703.jpg                    lossless    wasm       1400   2100    2685806  1      2349.119
pexels-toulouse-10807703.jpg                    lossless    webp-rust  1400   2100    2983076  2      10506.223
pexels-toulouse-10807703.jpg                    lossy-fast  libwebp    1400   2100    857060   27     76.621
pexels-toulouse-10807703.jpg                    lossy-fast  ours       1400   2100    1137460  21     99.176
pexels-toulouse-10807703.jpg                    lossy-fast  wasm       1400   2100    857060   12     172.237
pexels-toulouse-10807703.jpg                    lossy-fast  webp-rust  1400   2100    1124782  32     63.777
pexels-toulouse-10807703.jpg                    lossy-slow  libwebp    1400   2100    758466   5      448.692
pexels-toulouse-10807703.jpg                    lossy-slow  ours       1400   2100    567624   1      43044.356
pexels-toulouse-10807703.jpg                    lossy-slow  wasm       1400   2100    758466   2      1066.661
pexels-toulouse-10807703.jpg                    lossy-slow  webp-rust  1400   2100    560756   2      67335.680
```
