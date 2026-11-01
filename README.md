# gotuipic

Convert an image to UTF-8 art: colored terminal pictures in pure Go
(stdlib only, no CGO). Two styles:

- **sextant** (default) - two colors in six subpixels per cell using the
  U+1FB00 block-sextant glyphs, split per cell by an exact minimum-error
  partition;
- **ascii** - one truecolor glyph per cell from a luminance ramp, picked so
  the cell's own color actually hits the target brightness; needs no
  non-ASCII glyph, so it survives every terminal, font, and pipe.

```go
art, err := gotuipic.ConvertFile("cat.jpg",
	gotuipic.WithWidth(80),                   // columns
	gotuipic.WithMode(gotuipic.ModeSextant),  // or ModeASCII
	gotuipic.WithColor(gotuipic.ColorTrueColor), // 256/16/None too
)
if err != nil {
	return err
}
fmt.Print(art) // ANSI; art.Plain() for runes; art.Cells for TUIs
```

Try it:

```sh
go run ./examples/basic -w 100 cat.jpg | less -R
```

The library never reads `$TERM`, queries the terminal, or touches your
environment - pass a width, pick a profile (`gotuipic.ProfileFromEnv` helps
programs that want auto-detection).

Status: alpha, untagged. Design: `ARCHITECTURE.md`; build order and
progress: `PLAN.md`; mode measurements: `tools/stylepreview/`.
