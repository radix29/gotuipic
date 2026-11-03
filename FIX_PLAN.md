# FIX_PLAN.md - review findings and fix order

Review of the tree at `1f0f991`. `go build`, `go vet`, `gofmt`, `staticcheck`
and `go test -race ./...` are all clean (coverage 87-100%). The problems below
are things the tests do not cover. Bugs marked **(repro)** were reproduced
with a throwaway program.

## Decisions (from the user)

- `PLAN.md`: **drop**. Remove every reference; do not restore the file.
- `WithInvert`: **document** as ASCII-only. No behaviour change.
- Performance: **not a concern**. All optimization items are removed from this
  plan. Only changes that fix correctness, safety or clarity remain.
- Still open: B10 (256-color search scope). Default if unanswered: search
  16..255 only.

## 1. Bugs

| ID | Where | Problem | Fix |
|---|---|---|---|
| B1 **(repro)** | `raster.BoxResize`, `gotuipic.run` | A zero-area image (`image.NewNRGBA(Rect(0,0,0,0))`) panics with an integer divide by zero. `run` guards the geometry (`w>0 && h>0`) but still calls `BoxResize` with `sw == 0`. | Reject empty bounds in `Convert` with `ErrInvalidOption`, matching `decode`'s `image.ErrFormat`. Or return a 1x1 blank `Art`. Pick one and document it. |
| B2 **(repro)** | `resolve`, `run` | No upper bound on `WithWidth`/`WithHeight`. `WithWidth(1<<30)` panics in `makeslice`, and a merely large value allocates GBs. | Add `MaxCells`, a default cap with an option, and report `ErrInvalidOption`. Also bound the `rnd()` float-to-int conversion (an extreme `WithCellAspect` is only safe by luck today). |
| B3 **(repro)** | `Convert` | `MaxPixels` is never applied to a caller-supplied `image.Image`. `image.NewUniform` has infinite bounds and panics in `NewNRGBA`. A typed-nil `*image.NRGBA` panics. | Check `Bounds().Dx()*Dy()` against `maxPix` (overflow-safe, `int64`) and return `ErrTooLarge`. Recover or guard typed nil. Document the behaviour. |
| B4 **(repro)** | `WithBackground(nil)` | Nil pointer dereference inside `color.NRGBAModel.Convert(nil)`. | Treat nil as "keep default", or report `ErrInvalidOption`. |
| B5 **(repro)** | `Art.writePlain`, `Art.WriteTo` | `Cols` and `Cells` are exported. `Art{Rows:1, Cells: 3 cells}` divides by zero (`i % a.Cols`). In `ansi.Encode`, `Idx == -1` (cell `FG.A == 0`) indexes `fg16[-1]` and panics for `ColorANSI16`, and prints `38;5;-1` for 256. | Make the writers total: treat `Cols <= 0` or a length mismatch as empty, or treat the whole slice as one row. Skip the index lookup when the color has no index. |
| B6 **(repro)** | `glyph.exactSplit` | A solid block picks mask 1 on an sse tie, so flat regions print the rare glyph U+1FB00 (`🬀`) instead of a space. In ANSI16 it emits `ESC[90m ESC[100m 🬀`. This hurts fonts missing the sextant range and bloats output. | After the split, if `fg == bg` (and again after quantization) emit `' '` with BG only. Prefer mask 0 or 63 on ties. Update `TestRenderSextantSolidImageHasEqualColors` to assert the rune is a space. |
| B7 | `raster.CropAspect` | `rowStart := (y0+y)*src.Stride + x0*4` ignores `Rect.Min`, so it is wrong for a non-origin source. It only works because `Prepare` always moves the image to the origin. | Use `src.PixOffset(b.Min.X+x0, b.Min.Y+y0+y)`. |
| B8 **(repro)** | `WithInvert` | It reverses only the ASCII ramp; it does nothing for `ModeSextant`. | **Document** it as ASCII-only on the option's godoc and in the README options table. No code change. |
| B9 | `decode`, README | `io.ReadAll` is unbounded and is done before any limit check. The README says "never buffered" and "no full copy", but the whole stream is buffered, then wrapped twice. An HTTP body (`handleIndex`) can exhaust memory. A GIF's `MaxPixels` check covers only the canvas, not frames. | Add `WithMaxBytes` (default e.g. 64 MiB) and enforce it with `io.LimitedReader` (error on the limit). Correct the README wording ("buffered in memory, capped by `WithMaxBytes`"). No streaming rewrite. |
| B10 | `palette.Nearest256` vs ARCHITECTURE | The doc says it searches "the cube and the gray ramp". The code searches all 256 entries, including indices 0-15, which terminal themes redefine. 256-color output can therefore pick a themed color. | Search 16..255 only (the common practice), or update the doc to say the 16 are included. Update the golden files and tests accordingly. |

## 2. Doc / design inconsistencies

- **D1** `PLAN.md` was deleted in `1f0f991` but is still referenced from
  `CLAUDE.md`, `README.md` (twice), `ARCHITECTURE.md` and `examples/README.md`.
  Decision: drop it. Remove those references (and the "see PLAN.md" notes
  about `ConvertFrames`/planned examples; keep the planned items as plain
  text), and fix `CLAUDE.md`'s stale "design stage. No code exists yet".
- **D1b** `CLAUDE.md` also tells readers to follow `PLAN.md`'s phases; replace
  with a pointer to `ARCHITECTURE.md`/`FIX_PLAN.md`.
- **D2** `ARCHITECTURE.md` describes things that do not exist: `WithDither`,
  `DitherBayer4`, `cmd/gotuipic`, `formats`, EXIF orientation, `testdata/golden`,
  `FuzzConvertReader`, `examples/tcell`. Mark each "planned", or implement
  per the order in section 5. The README also calls `examples/basic` "the
  shipped CLI".
- **D3** `ARCHITECTURE.md` says the encoder uses "one `bufio.Writer`, no
  per-cell allocations". The code builds one `[]byte` and uses `fmt.Appendf`.
  Since performance is not a concern, fix the **doc**.
- **D4** The pixel-aspect paragraph in "Geometry" is misleading. Sextant
  pixels are not "close to square"; the image is rescaled to
  `cols*2 x rows*3` regardless. Reword it.
- **D5** `BoxResize` is documented as "area-averaging", but its boundaries
  are integer floors, so edge pixels are not fractionally weighted. Say so,
  because golden files depend on the exact behaviour (no change in code).
- **D6** README says `NO_COLOR` is not read by the library but then lists it
  under `ProfileFromEnv`. That is consistent, but `TERM=""` giving `ColorNone`
  is undocumented.
- **D7** Typos and stale comments: `BoxResize` doc ("upscale).dw"), `glyph.Cell`
  vs the "(NRGBA)" description, the `examples/basic` comment about
  `cmd/gotuipic`.

## 3. Architecture and simplification

- **S1 Triple cell copy.** The pipeline builds `glyph.Cell` (NRGBA), then
  `gotuipic.Cell` (RGBA) in `quantize`, then `ansi.Cell` (plus indices) in
  `WriteTo` for every call. Introduce one shared cell type (`internal/cell`,
  aliased as `gotuipic.Cell`) with an optional palette index, let `quantize`
  work in place, and make `ansi.Encode` read `[]Cell` directly. This removes
  the redundant second `Nearest256/Nearest16` run in `WriteTo`. Justified as
  simplification only; optional.
- **S2 Public types alias internal ones.** `type Mode = glyph.Mode` exposes
  an internal package in godoc and gives `Mode` no `String`. Define
  `type Mode int` in the root, with `String()`, and map it to `glyph`.
  `glyph.Render`'s `default:` branch treats any unknown mode as sextant;
  make it explicit.
- **S3 Over-general split code.** `exactSplit` supports n up to 16 plus a
  "defensive" `n > 16` branch, and `exactSplitCore` is described as
  "space-agnostic", all left over from the rejected linear-light experiment.
  No mode uses more than 6 subpixels. Specialise to fixed `[6][3]float64`
  arrays (the 63-split loop with no `make`), delete the dead branches
  (`n == 0`, `n == 1`, `n > 16`, `meanColor` n == 0), and drop `splitBuf`.
- **S4 Option plumbing.** `resolve` formats errors with ad-hoc strings. Group
  validation next to each option (or table-drive it), and give `WithWidth`,
  `WithHeight`, `WithFit`, `WithMode`, `WithColor`, `WithInvert`,
  `WithMaxPixels` doc comments (they currently share one line with no godoc).
- **S5 The three entry points** (`ConvertReader`, `ConvertBytes`, and
  `ConvertFile` through the reader) duplicate `resolve` plus `decode` plus `run`.
  Route them through one private helper.
- **S6 Examples.** `examples/basic` and `examples/stream` duplicate flag
  parsing (mode/color/fit switches). Share it, or fold the stream logic into
  `basic`'s flags, or wait for `cmd/gotuipic`.
- **S7 Untested exported API** (deadcode reports it only because nothing in
  `main` uses it): `Art.Mode`, `Art.Profile`, `WithCellAspect`,
  `WithBackground`. Add tests for each.

## 4. Optimizations

Dropped by decision (performance is not an issue). The review did find
cheap wins (per-pixel interface conversion in `Prepare`, brute-force
`Nearest256`, per-cell allocation in `exactSplit`) if that ever changes.

## 5. Tests and tooling to add

- Regression tests for B1-B6, B8 (the probe cases above).
- `FuzzConvertReader` (promised in `CLAUDE.md`), then `testdata/golden`.
- A `go vet`/`staticcheck`/`gofmt -l` run as the final step of each phase.

## 6. Recommended implementation order

After each step run `go build ./... && go test ./... && gofmt -l . &&
go vet ./... && staticcheck ./...`.

1. **D1/D1b**: drop `PLAN.md` references, fix stale `CLAUDE.md` status.
2. **B1, B2, B3, B4**: input-validation panics, with regression tests (the
   only user-reachable crashes).
3. **B5, B7**: make the `Art` writers and `CropAspect` robust.
4. **B6**: solid-block glyph becomes a space (visible output change; update
   the solid-image test).
5. **B9**: `WithMaxBytes` limit and the README correction.
6. **S3**: simplify the split code (fixed 6-subpixel arrays, delete dead
   branches). Output must stay identical.
7. **B10**: palette search scope (pending decision), with test updates.
8. **B8 + S2, S4, S5, S7**: public API polish: document `WithInvert` as
   ASCII-only, `Mode.String`, option godoc, shared convert helper, tests for
   `Mode`/`Profile`/`WithCellAspect`/`WithBackground`. Check `go doc` after.
9. **S1** (optional): single cell type.
10. **D2-D7**: align `ARCHITECTURE.md` and the READMEs with what exists;
    mark unbuilt parts "planned".
11. **S6** and remaining features (`FuzzConvertReader`, golden files,
    `cmd/gotuipic`, `formats`, `WithDither`, `tcell` example) as later work.

Steps 1-5 are correctness and safety. Steps 6-9 are cleanup. Step 8 is the
only one touching the public surface, so land it before any tag.

Constraints from `CLAUDE.md` still apply: no environment access in the
library, stdlib-only core, and the box filter's output must not change. No
commits are made as part of this plan.
