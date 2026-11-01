# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

gotuipic is a pure-Go (no CGO) library that converts an image (PNG, JPEG,
GIF; WebP/BMP/TIFF opt-in) from a file, an `io.Reader` or an `image.Image`
into colored UTF-8 text art for terminals. Module
`github.com/radix29/gotuipic`, Go 1.27, MIT license.

**Status: design stage.** No code exists yet.
- `PLAN.md` gives the phased build order. Implement one phase at a time,
  with its tests and examples in the same change.
- `ARCHITECTURE.md` is the design: pipeline, geometry, modes, color, API and
  package map. Read the relevant section before touching a stage, and update
  it when the code changes the design.

## Build & verify

Plain `go`, no Makefile (once `go.mod` exists):

```sh
go build ./... && go test ./... && gofmt -w . && go vet ./...
go test ./... -update                       # regenerate testdata/golden/*; review the diff
go test -run TestName ./internal/glyph      # single test
go test -fuzz FuzzConvertReader -fuzztime 60s .
~/go/bin/staticcheck ./...
```

## Rules that shape the code

- **The library never touches the environment.** No reading `$TERM` or
  `$COLORTERM`, no terminal-size queries, no TTY detection inside `Convert`.
  Only `cmd/gotuipic` does that.
- **The core depends only on the standard library.** `golang.org/x/image`
  is used only in the opt-in `formats` package, and `golang.org/x/term` only
  in the CLI. tcell appears only in `examples/tcell`, which is its own module.
- `Art` (a grid of `Cell{Rune, FG, BG}`) is the product. ANSI text is one
  rendering of it. TUI users read `Cells`; nothing should require parsing
  escape sequences.
- Every emitted glyph is single-width, so column count equals rune count.
  Tests check this for every mode.
- Resizing is the in-house sRGB box filter; a linear-light variant was
  measured and scored worse for both shipped modes. Don't swap in a
  third-party resampler, because golden files depend on exact pixel values.
- Call `image.DecodeConfig` and check `MaxPixels` before `image.Decode`.

## Repo hygiene

- Don't commit, amend, push or tag. The user does that.
- `~/go` is the GOPATH, and the sibling directories are unrelated projects.
  Stay inside `gotuipic/`.
