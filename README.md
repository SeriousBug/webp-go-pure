# webp-go

Pure Go WebP decoder and encoder. No cgo, no external dependencies.

    go get github.com/SeriousBug/webp-go-pure

This is a Go port of [webp-rust](https://github.com/mith-mmk/webp-rust) by
MITH@mmk. It is essentially a fork of that project, translated to Go. The
original Rust implementation and its design are the work of the upstream author;
this port keeps the same codec behavior while presenting an idiomatic Go API.

## Status

- Still image decode: `VP8` lossy, `VP8L` lossless
- Still image encode: lossy `VP8` and lossless `VP8L` from RGBA
- Alpha: `ALPH` for lossy still images and lossy animation frames
- Animation: compositing to RGBA frame sequence
- Library output: RGBA only

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

Animated WebP is not accepted by `Decode`. For animation, use the animation
decode entry point:

```go
anim, err := webp.DecodeAnimation(data)
fmt.Println(len(anim.Frames))
```

## License

MIT. See `LICENSE`. Original work (C) MITH@mmk 2026; Go port (C) Kaan
Barmore-Genc 2026.
