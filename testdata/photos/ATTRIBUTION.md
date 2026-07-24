# Image attribution

Sample photos used as codec test inputs. Filenames are kept as downloaded.
Sources larger than 1 MB were scaled down (via `sips`) to keep the repo small;
pixel content is otherwise unmodified.

## Pexels

Licensed under the [Pexels License](https://www.pexels.com/license/) (free to use,
attribution not required but provided here).

| File | Photographer | Source |
| --- | --- | --- |
| `pexels-abubakar-mamman-2148132108-38602599.jpg` | Abubakar Mamman | https://www.pexels.com/photo/woman-gently-framed-by-tree-trunks-outdoors-38602599/ |
| `pexels-mavihnt-38213559.jpg` | mavihnt | https://www.pexels.com/photo/serene-pine-forest-captured-in-daylight-38213559/ |
| `pexels-toulouse-10807703.jpg` | Toulouse | https://www.pexels.com/photo/hiker-carrying-twigs-and-branches-down-mountain-road-10807703/ |
| `pexels-martin-alargent-1165956-5665465.jpg` | Martin Alargent | https://www.pexels.com/photo/close-up-photo-of-fig-cake-5665465/ |
| `pexels-steve-15267299.jpg` | Steve | https://www.pexels.com/photo/colorful-art-on-canvas-15267299/ |
| `pexels-steve-29626041.jpg` | Steve | https://www.pexels.com/photo/abstract-3d-render-with-geometric-shapes-29626041/ |

## Recreated Lena

`Lena_512.png` is Morten Rieger Hannemose's recreated Lena picture, from
https://mortenhannemose.github.io/lena/

```bibtex
@misc{hannemoselena,
    author = {Morten Rieger Hannemose},
    title = {Recreated Lena Picture},
    year = {2019},
    url = {https://mortenhannemose.github.io/lena/}
}
```

## WebP files

The `.webp` files in this directory were produced from the sources above with
`cmd/webpcli` (lossy, quality 90). They inherit the license of their source image.
