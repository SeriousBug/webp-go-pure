# The webp-rust lossy-fast bug

This library started as a port of [webp-rust](https://github.com/mith-mmk/webp-rust).
Porting it to Go and testing the result turned up a bug in the original that its
own test suite did not catch: at effort 0, the fastest lossy setting, it can
write WebP files that no decoder can read back correctly.

Three of the seven images in `testdata/photos` hit it. The figures are PSNR
against the source image, from a run at quality 90:

| image | webp-rust | ours | libwebp |
| --- | --- | --- | --- |
| toulouse | **6.35 dB** | 39.93 dB | 39.84 dB |
| martin-alargent | **9.38 dB** | 42.39 dB | 42.63 dB |
| abubakar-mamman | **16.44 dB** | 42.02 dB | 41.96 dB |
| the other four | 39.7-49.0 dB | 40.9-49.3 dB | 40.9-49.1 dB |

Higher PSNR is better: a normal encode at quality 90 lands near 40 dB, so
webp-rust's 6.35 dB on toulouse is a garbled image rather than a tighter file.

We found it by scoring quality, not size. Encoding each image, decoding it back
and comparing against the source turned up three files that came back garbled,
which size alone had never shown. Decoding those files with webp-rust's own
decoder and with ours gave figures agreeing to the hundredth of a dB, including
on the images that encode fine, which pointed at the encoder rather than at
either decoder.

This port inherited the same bug and fixed it in commit 7e5e084, with a
regression test covering the case. The bug is still present in webp-rust v0.2.1.

webp-rust is no longer part of the benchmarks: it was a check on whether the port
lost ground against the code it came from, which is not a choice anyone picking a
Go library has. This note is the part of that comparison worth keeping.
