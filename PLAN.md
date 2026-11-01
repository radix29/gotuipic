# Plan

How to build gotuipic from an empty repo to a v0.1.0 release.
`ARCHITECTURE.md` is the design; this file is the order of build. Two modes
ship: `ModeSextant` (default; the exact 2-color partition that
`tools/stylepreview` calls `sextantx`) and `ModeASCII` (compatibility; every
terminal renders it). Each phase ends with green tests and a usable library,
and lands its tests and examples in the same change as its code.

> **Open decision — the ASCII algorithm.** `tools/stylepreview` rows 1-4
> (`ascii`, `asciiw`, `asciied`, `asciiwl`) are the candidates; `-rmse`
> scores them on all three example images. `asciiw` (pick the ramp glyph so
> FG luma x coverage hits the target luma) wins by a wide margin over the
> naive baseline and ties-or-beats ED and linear-light resize. This plan
> implements `asciiw`; if the final pick differs, only the `internal/glyph`
> ASCII picker changes.

## Phase 0: Scaffolding

- `go.mod` exists (`github.com/radix29/gotuipic`, go 1.27).
- `doc.go` package overview; README install + usage snippet.
- CI (`.github/workflows/ci.yml`): Linux/macOS/Windows, `gofmt -l`,
  `go vet`, `staticcheck`, `go test -race ./...`.
- `testdata/` fixtures: `gradient.png`, `photo.jpg`, `alpha.png`,
  `truncated.png`, `not-an-image.txt` (small ones; the big example images
  stay in `tools/stylepreview/testdata`).

**Done when:** CI is green against a `Convert` stub.

## Phase 1: Pipeline core + ASCII

- `Convert`, `ConvertReader`, `ConvertFile`; `Options`: `WithWidth`,
  `WithHeight`, `WithFit`, `WithCellAspect`, `WithMode`, `WithRamp`,
  `WithInvert`, `WithBackground`, `WithMaxPixels`. `ModeASCII` works;
  `ModeSextant` returns `ErrInvalidOption` until phase 2.
- `internal/raster`: to-NRGBA, alpha compositing over `WithBackground`,
  box-filter resize in sRGB (float accumulators). No linear-light path -
  it measured worse (ARCHITECTURE "Modes").
- Geometry: `Cols`/`Rows` from width/height/cell aspect (default 1:2 gives
  square pixels at 1x2 sampling), `Fit` contain/stretch/crop.
- `internal/glyph` ASCII cell: block mean color -> FG, mean luma -> target,
  weighted picker `i = round(target / (fgLuma/255) * (len(ramp)-1))`
  clamped, near-black FG falls back to the last glyph. Port from
  `stylepreview.pickRuneIdx`. Default ramp `" .:-=+*#%@"`.
- `Art` (`Cells`, `Cols`, `Rows`, `Mode`), `Art.Plain/String/WriteTo`.
- Errors: `ErrInvalidOption`, wrapped `image.ErrFormat`, `ErrTooLarge`.

**Tests:**
- Geometry table incl. 1x1, 1x10000, width larger than the image.
- Resize: solid stays solid; 2x2 -> 1x1 average exact; never zero-sized.
- Picker: for every fg in a fixed grid x every target luma, no other ramp
  index has smaller `|fgLuma*pos - target|` (brute force over the 10
  glyphs); black -> first rune, white -> `@`; `WithInvert` swaps.
- Decode: fixtures ok; truncated/garbage satisfy `errors.Is`; a header
  claiming 100000x100000 is rejected before allocation (`DecodeConfig`).
- Invariant: every row has exactly `Cols` runes, all single-width.
- Golden: `gradient.png`, 40 cols, `ModeASCII` + `ColorNone`.

**Example:** `ExampleConvertReader` prints a 16-col gradient (`// Output:`
deterministic because ColorNone).

## Phase 2: Sextant (becomes the default)

- `internal/glyph` sextant cell: exact 2-color partition over the six
  subpixels (all 63 non-trivial splits, subset-sum DP for O(1) split
  errors - port `exactSplit`/`exactSplitCore` from `stylepreview`), brighter
  mean = FG, membership bitmask -> glyph.
- `sextantGlyph` lookup: U+1FB00-U+1FB3B with the Unicode gaps covered by
  ` `, `▌`, `▐`, `█` (shifts at masks 21 and 42).
- Flip the default mode to `ModeSextant`.

**Tests:**
- All 64 masks map to the expected rune, checking both gap shifts.
- `left-column-red.png`-style fixtures: red left / blue right halves give
  FG = brighter, mask = right column (`▐`-family), etc.
- Property: `exactSplit` total error <= 2-means error for every block of
  `photo.jpg` (2-means kept only as a test oracle), seeded randoms included.
- Solid red image -> `█` (mask 63); 1-pixel-wide degenerate blocks -> solid
  glyph with FG = BG = the pixel.
- Invariants for both modes. Golden: `photo.jpg`, 40 cols, `ColorNone`.

**Example:** `ExampleWithMode` (ascii vs sextant), `examples/modes`.

## Phase 3: Color + ANSI encoder

- `internal/palette`: sRGB -> Oklab, xterm 16/256 tables, nearest-color
  search; `ProfileFromEnv(getenv)`.
- `internal/ansi`: SGR writer, change-only emission, `ESC[0m` at row end,
  no `ESC` bytes under `ColorNone`; `Art.ANSI()`.
- `WithColor(ColorNone|ColorANSI16|ColorANSI256|ColorTrueColor)`; FG and BG
  quantized independently. `WithDither(DitherNone|DitherBayer4)` applies to
  the 16/256 palettes only (no FS: cell-space diffusion measured worse).

**Tests:**
- xterm 256 table matches known RGBs; each palette color maps to itself.
- Encoder: identical-cell runs emit one SGR; rows end reset; `ColorNone`
  has no escapes. `ProfileFromEnv` table (`COLORTERM`, `TERM`, `NO_COLOR`).
- Dither a flat mid-gray to ANSI16: mean luma within tolerance; Bayer
  output stable across runs.
- Golden: `photo.jpg` x 2 modes x 4 profiles.

**Example:** `examples/basic` (file -> stdout, `-w`, `-color`).

## Phase 4: Robustness + formats

- `formats` package: blank-import WebP/BMP/TIFF from `golang.org/x/image`.
- EXIF orientation: minimal APP1 parser reading tag `0x0112`,
  `WithOrientation(bool)`, on by default.
- `ConvertFrames(r) ([]*Art, []time.Duration, error)` for animated GIFs,
  compositing disposal methods.
- `FuzzConvertReader` seeded with every fixture; nightly 60 s run.

**Tests:** orientation fixtures for EXIF 1-8 match pre-rotated references;
3-frame GIF -> 3 `Art` with delays; fuzz clean.

**Benchmarks:** `BenchmarkConvert` 4000x3000 -> 120 cols per mode; target
< 50 ms, and allocation count scales with `Art` cells only, not per-pixel.

## Phase 5: CLI + TUI examples

- `cmd/gotuipic` (uses `golang.org/x/term` for width, `ProfileFromEnv` for
  color): `gotuipic [-w N] [-mode ascii|sextant]
  [-color auto|true|256|16|none] [-dither none|bayer] [-fit ...] FILE|-`,
  `-` = stdin.
- `examples/tcell` (own module) renders `Art.Cells` into a tcell v3 screen
  on resize; `examples/lipgloss` if time permits.

**Tests:** flag parsing, stdin, golden output of the built binary on a
fixture.

## Phase 6: Release v0.1.0

- Godoc pass; README gallery (reuse `tools/stylepreview` montages).
- `CHANGELOG.md`; tag `v0.1.0`; optional GoReleaser + `radix29/tap`.

## Definition of done (v0.1.0)

- `go build && go test -race ./... && go vet && staticcheck` green on the
  CI matrix; `gofmt -l` empty.
- Both modes x 4 profiles render the three example images; goldens under
  `testdata/golden/` regenerate only via `go test ./... -update`.
- Single-width invariant test green for every mode x profile.
- Fuzz target clean for 60 s; no crashes from CI nightlies for a week.

## Out of scope for v0.1

- Sixel/Kitty/iTerm2 graphics protocols (a later `Encoder`).
- More glyph families: half/quad/braille/shades/U8GFX were rendered and
  scored in `tools/stylepreview` and rejected (see ARCHITECTURE "Modes").
- Video, vector input, terminal querying inside the library.
