# Image attribution

Test inputs with an alpha channel, for benchmarking and testing transparency
handling. The `photos/` corpus next to this one is fully opaque, so nothing
there exercises alpha.

All five files come from Google's WebP
[Lossless and Alpha Gallery](https://developers.google.com/speed/webp/gallery2),
downloaded unmodified from `https://www.gstatic.com/webp/gallery3/`. Filenames
are kept as downloaded; the number in each name is the gallery's own numbering.

| File | Title | Author | License |
| --- | --- | --- | --- |
| `1_webp_a.png` | Yellow Rose 3 - Flowers | Jon Sullivan | public domain |
| `2_webp_a.png` | baby tux for my user page | Fizyplankton | public domain |
| `3_webp_a.png` | PNG transparency demonstration | POV-Ray source code | [CC BY-SA 3.0](https://creativecommons.org/licenses/by-sa/3.0/) |
| `4_webp_a.png` | Gregor Mendel's 189th Birthday | Google Doodle team | Google Doodle |
| `5_webp_a.png` | Transparent compass card for overlays | Denelson83 | [CC BY-SA 3.0](https://creativecommons.org/licenses/by-sa/3.0/) |

Alpha coverage, as a share of pixels:

| File | Size | Fully opaque | Fully transparent | Partial |
| --- | --- | --- | --- | --- |
| `1_webp_a.png` | 400x301 | 46.2% | 52.1% | 1.7% |
| `2_webp_a.png` | 386x395 | 58.1% | 37.7% | 4.2% |
| `3_webp_a.png` | 800x600 | 15.7% | 67.5% | 16.7% |
| `4_webp_a.png` | 421x163 | 19.5% | 74.8% | 5.7% |
| `5_webp_a.png` | 300x300 | 0.0% | 18.3% | 81.7% |
