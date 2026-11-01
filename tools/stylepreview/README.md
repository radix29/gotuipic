# stylepreview

Throwaway design aid for `gotuipic`. Renders the example images as PNG
montages and ANSI text so the two shipping modes (`ascii`, `sextant`) and
every variant that was considered can be compared by eye **and** by metric.
It lives in its own Go module precisely so the library core never has to
import it.

## Usage

```sh
cd tools/stylepreview
go run .                              # preview-<name>.png for each testdata image
go run . -in /path/to/file.jpg        # preview one of your own images
go run . -rmse                        # RMSE / PSNR / Oklab dE table for all styles
go run . -ansi sextantx -cols 100     # real ANSI truecolor to stdout
go run . -ansi asciiw -cols 100 | less -R
go run . -h                           # flags and the full style id list
```

The default montages use `testdata/*.jpg`; add or remove files there to
change the set. A PNG is a font-independent approximation of terminal output,
so use it for A/B decisions; use `-ansi` for what your actual terminal shows.
RMSE treats an ASCII cell as its FG attenuated by the glyph's ramp position,
which is the same model the picker uses.

## Styles in the montages

The image on the far left of each montage is the source downscaled to exactly
the canvas each strip covers, so any difference you see is loss caused by the
style at equal terminal size.

| # | id | what it is |
|---|---|---|
| 1 | `ascii` | baseline: ramp glyph chosen by luma index |
| 2 | `asciiw` | ramp glyph chosen so FG luma x coverage hits the target luma |
| 3 | `asciied` | asciiw + Floyd-Steinberg error diffusion on luma |
| 4 | `asciiwl` | asciiw with linear-light resize |
| 5 | `sextant` | 2x3 sextants, 2-means color split |
| 6 | `sextantx` | same glyphs, exhaustive exact 2-color partition (ships) |
| 7 | `sextantp` | exact split measured in linear light |
| 8 | `sextanted` | linear-light exact + Floyd-Steinberg across cells |
| 9 | `sextantlr` | linear-light resize + exact split |

Measured verdicts (all three example images, `-rmse`):

- `asciiw` roughly halves RMSE/dE vs `ascii`; ED and linear-light resize add
  nothing on top (`asciiw` wins overall).
- `sextantx` is the best 2x3 variant; 2-means is close but not equal, and
  the linear-light/ED experiments all lose.
- Earlier candidates (half/quad/braille/shades/U8GFX) were pruned from both
  the tool and the mode set; see ARCHITECTURE.md "Modes" for the numbers.

`-rmse` and `-ansi` skip no styles; compare rows 1-4 to pick the ASCII
algorithm, and 5-9 for the sextant family.

## Samples

See `CREDITS.md` for example-image sources and licenses. `preview-*.png`
files are generated output, kept as design references; rerun `go run .`
after changing rendering.
