# Test fixture attribution

This directory holds the small fixtures the root module's own tests read. The
benchmark image corpora live in `benchmark/testdata/`, which is a separate Go
module and so is not part of the published library zip.

## `lena_crop_512.png`

The top-left 512x512 pixels of Morten Rieger Hannemose's recreated Lena picture
(a 900x900 PNG), from https://mortenhannemose.github.io/lena/. The crop is
lossless: the pixels are byte-identical to the corresponding region of the
source. The full-size original is kept at
`benchmark/testdata/photos-jpeg/Lena_512.png`.

```bibtex
@misc{hannemoselena,
    author = {Morten Rieger Hannemose},
    title = {Recreated Lena Picture},
    year = {2019},
    url = {https://mortenhannemose.github.io/lena/}
}
```

One encoder test needs real photographic macroblock activity to make the lossy
segmentation candidates diverge, which synthetic gradients and noise do not
reproduce. 512x512 is the crop that test asks for, so only that much is stored
here.

## `sample.webp`, `sample_lossy.webp`, `sample_lossless.webp`, `sample_animation.webp`

Decoder fixtures. These predate this file and their provenance was not recorded
at the time, so no source or license is claimed for them here.
