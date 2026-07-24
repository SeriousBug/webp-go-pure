# webp-go

Pure Go WebP decoder and partial encoder.

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

Top-level still-image encode:

```go
out, err := webp.Encode(img, 2, 100, webp.Lossless, nil)
lossy, err := webp.EncodeLossy(img, 0, 90, nil)
lossless, err := webp.EncodeLossless(img, 2, nil)
```

To embed raw EXIF metadata, pass the chunk payload directly:

```go
out, err := webp.EncodeLossless(img, 2, exifBytes)
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
