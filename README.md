# webp-go

Pure Go WebP decoder and encoder. No cgo, no external dependencies.

    go get github.com/SeriousBug/webp-go-pure

Decodes lossy `VP8` and lossless `VP8L` still images, decodes animated WebP into
a composited RGBA frame sequence, and encodes still images as lossy or lossless
from RGBA. Alpha comes through `ALPH` chunks on lossy still images and on lossy
animation frames. All pixel data in and out of the library is packed 8-bit RGBA.

This started as a Go port of [webp-rust](https://github.com/mith-mmk/webp-rust)
by MITH@mmk, and the container handling and decoders still follow it. The
encoders have since diverged: parts are ported from
[libwebp](https://chromium.googlesource.com/webm/libwebp/), the C reference
implementation, and the rest is our own work, down to hand-written arm64 NEON
and amd64 SSE assembly. The port also found and fixed a bitstream bug still
present in webp-rust v0.2.1, described in
[benchmark/results.md](benchmark/results.md).

## Performance

If you can use cgo, use libwebp itself. It is the C reference implementation,
and it encodes faster than we do at slightly better quality per byte.

Without cgo, the option is libwebp compiled to WebAssembly and run through
wazero, as [gen2brain/webp](https://github.com/gen2brain/webp) does. Running it
that way costs both time and memory, and we come out ahead of it:

- **Lossy:** 1.1-3.4x faster, at 1.7-3.8x lower peak memory.
- **Lossless:** roughly even on time (0.9-2.3x), at 1.2-1.8x lower peak memory.

![Encode time per image for each engine, one panel per mode and machine](benchmark/charts/encode-time-light.svg#gh-light-mode-only)
![Encode time per image for each engine, one panel per mode and machine](benchmark/charts/encode-time-dark.svg#gh-dark-mode-only)

Full tables, PSNR and peak-memory figures, the test corpus and the method are in
[benchmark/results.md](benchmark/results.md).

## Library API

Top-level still-image decode:

```go
img, err := webp.Decode(data)
if err != nil {
    return err
}
fmt.Printf("%dx%d\n", img.Width, img.Height)
```

`Decode` returns a `webp.Image` (packed 8-bit RGBA, plus `Width`/`Height`).
`DecodeFile` does the same from a path, and `Features` reports dimensions and
format without a full decode.

Still-image encode takes a `*webp.Image` and an options struct; pass `nil` for
the defaults (lossy quality 90 effort 0, lossless effort 6). `Effort` runs 0..9,
trading speed for size:

```go
lossy, err := webp.EncodeLossy(&img, &webp.LossyOptions{Quality: 90, Effort: 4})
lossless, err := webp.EncodeLossless(&img, nil)
```

To embed raw EXIF metadata, set it on the options:

```go
out, err := webp.EncodeLossless(&img, &webp.LosslessOptions{EXIF: exifBytes})
```

Animated WebP goes through `DecodeAnimation`, which returns composited RGBA
frames. `Decode` does not accept it, so check `Features` first to pick the entry
point:

```go
features, err := webp.Features(data)
if err != nil {
    return err
}
if features.HasAnimation {
    anim, err := webp.DecodeAnimation(data)
    if err != nil {
        return err
    }
    fmt.Println(len(anim.Frames))
    return nil
}
img, err := webp.Decode(data)
```
