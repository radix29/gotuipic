# gotuipic

Convert an image to UTF-8 art: colored terminal pictures in pure Go
(stdlib only, no CGO, no dependencies). Two styles:

- **sextant** (default) - two colors in six subpixels per cell using the
  U+1FB00 block-sextant glyphs, split per cell by an exact minimum-error
  partition;
- **ascii** - one truecolor glyph per cell from a luminance ramp, picked so
  the cell's own color actually hits the target brightness; needs no
  non-ASCII glyph, so it survives every terminal, font, and pipe.

The input can be a file, an `io.Reader`, a `[]byte` or an `image.Image`.
The output is an `Art` - a grid of cells you can print as ANSI, flatten to
runes, write to any `io.Writer`, or hand to a TUI toolkit.

```sh
go get github.com/radix29/gotuipic   # Go 1.27+, zero dependencies
```

## Quick start

```go
art, err := gotuipic.ConvertFile("tools/stylepreview/testdata/sample.jpg",
	gotuipic.WithWidth(80),                      // columns
	gotuipic.WithMode(gotuipic.ModeSextant),     // or ModeASCII
	gotuipic.WithColor(gotuipic.ColorTrueColor), // 256/16/None too
)
if err != nil {
	return err
}
fmt.Print(art) // ANSI; art.Plain() for runes; art.Cells for TUIs
```

Or run the shipped CLI:

```sh
go run ./examples/basic -w 100 tools/stylepreview/testdata/sample.jpg | less -R
```

The library never reads `$TERM`, `$COLORTERM` or `$NO_COLOR`, queries the
terminal size, or checks for a TTY - pass a width, and pick a profile
(`gotuipic.ProfileFromEnv` helps programs that want auto-detection).

## Getting an image in

Four entry points, same options, same `*Art` back. The format is sniffed
from the stream header, never from a name or extension, and nothing needs
`Seek` or `Stat`:

| Function | Takes | Use for |
|---|---|---|
| `ConvertFile(path, ...)` | file path | an image on disk |
| `ConvertReader(r, ...)` | `io.Reader` | stdin, pipes, sockets, HTTP bodies, `gzip.Reader`, `embed.FS` files, tar entries - anything |
| `ConvertBytes(b, ...)` | `[]byte` | data already whole in memory: a buffered response body, `fs.ReadFile`, a base64-decoded blob, a WebSocket frame payload |
| `Convert(img, ...)` | `image.Image` | images you decoded yourself, drew in code, or got from a library gotuipic knows nothing about |

### From a byte stream (`io.Reader`)

```go
// Standard input: `gotuipic - < cat.png`, or `curl -sL URL | gotuipic -`
art, err := gotuipic.ConvertReader(os.Stdin, gotuipic.WithWidth(100))
```

```go
// An HTTP response body. The stream is consumed once, decoded from its
// header, and never buffered by you; Go's transport already inflates gzip
// responses it asked for, so this reads real PNG/JPEG bytes.
resp, err := http.Get("https://picsum.photos/800/600")
if err != nil {
	return err
}
defer resp.Body.Close()
if resp.StatusCode != http.StatusOK {
	return errors.New(resp.Status)
}
art, err := gotuipic.ConvertReader(resp.Body,
	gotuipic.WithWidth(100),
	gotuipic.WithMaxPixels(32<<20)) // header is checked before any pixel is allocated
```

```go
// A producer goroutine feeding a pipe - no temp file, no full copy.
r, w := io.Pipe()
go func() { _ = png.Encode(w, img); w.Close() }()
art, err := gotuipic.ConvertReader(r, gotuipic.WithWidth(80))
```

It works the same for `gzip.NewReader(f)`, `archive/tar` entry bodies,
`net.Conn`, `io.LimitReader(r, n)` when you want a hard byte cap, or an
`embed.FS` file handle. See `examples/stream` for a complete URL-or-stdin
program.

### From bytes in memory (`[]byte`)

```go
data, err := fs.ReadFile(files, "static/cat.png") // or base64.StdEncoding.DecodeString(s)
if err != nil {
	return err
}
art, err := gotuipic.ConvertBytes(data, gotuipic.WithWidth(80))
```

The slice is only read, never retained, so you may reuse the buffer.

### From a decoded image (`image.Image`)

```go
art, err := gotuipic.Convert(img, gotuipic.WithWidth(80))

// Or draw one from scratch - gradients, charts, noise, a captured frame.
im := image.NewNRGBA(image.Rect(0, 0, 64, 32))
/* ... fill im ... */
art, err := gotuipic.Convert(im, gotuipic.WithMode(gotuipic.ModeASCII))
```

### Formats

PNG, JPEG and GIF are registered by the core. Anything else that plugs into
the standard `image` registry works by importing its decoder for the side
effect - `ConvertFile`, `ConvertReader` and `ConvertBytes` all pick it up
automatically:

```go
import (
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)
```

That is exactly what the planned opt-in `formats` package will wrap, so the
core stays standard-library-only. Animated GIFs give the first frame for
now; `ConvertFrames` is planned (see `PLAN.md`).

## Getting the art out

`Art` is the product - a `Cols x Rows` grid of `Cell{Rune, FG, BG}`. ANSI
text is one rendering of it, not the only one.

```go
fmt.Print(art)                  // ANSI truecolor/256/16 SGR; Art.String() is the same text
plain := art.Plain()            // runes only, no escape sequences, paste-able anywhere
_, _ = art.WriteTo(os.Stdout)   // io.WriterTo: a file, a socket, an http.ResponseWriter, a log
_, _ = io.Copy(w, strings.NewReader(art.String())) // or anything that wants a Reader
```

Writing art somewhere is the mirror image of reading an image in:

```go
func handleIndex(w http.ResponseWriter, r *http.Request) {
	art, err := gotuipic.ConvertReader(r.Body, gotuipic.WithWidth(60),
		gotuipic.WithColor(gotuipic.ColorANSI256))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = art.WriteTo(w)
}
```

For TUIs, read the cells instead of parsing escape sequences back out of a
string. Every glyph is single-width, so the column index equals the rune
index:

```go
for row := 0; row < art.Rows; row++ {
	for col := 0; col < art.Cols; col++ {
		c := art.At(col, row) // never panics: out-of-range gives the zero Cell
		// c.Rune, c.FG, c.BG is all a screen backend needs. c.BG.A == 0
		// means "no background": paint with the terminal's own, not black.
		screen.Set(col+offX, row+offY, c)
	}
}
```

An `Art` is not mutated after `Convert` returns, so it is safe to render the
same one from several goroutines.

## Options

| Option | Default | Effect |
|---|---|---|
| `WithWidth(n)` / `WithHeight(n)` | 80 x auto | output size in cells |
| `WithFit(FitContain\|FitStretch\|FitCrop)` | `FitContain` | what to do when both width and height are given |
| `WithCellAspect(w/h)` | `0.5` | cell shape; set it to match a font that is not 1:2 |
| `WithMode(ModeSextant\|ModeASCII)` | `ModeSextant` | glyph style |
| `WithColor(ColorTrueColor\|ColorANSI256\|ColorANSI16\|ColorNone)` | `ColorTrueColor` | color depth |
| `WithRamp(" .:-=+*#%@")` | that ramp | ASCII glyphs, dark to light |
| `WithInvert(true)` | false | reverse the ramp, for terminals that show glyphs as ink on a light background |
| `WithBackground(c)` | black | what transparent pixels composite onto |
| `WithMaxPixels(n)` | 64 Mi pixels | decompression-bomb guard, checked on the header before decoding |

Bad option values are collected and returned together as one
`ErrInvalidOption` (`errors.Is` works), not at construction.

## Gallery

Everything below is the actual output on the three example photos that ship
in `tools/stylepreview/testdata` - `sample.jpg` (a cat), `sample-fruit.jpg`
and `sample-landscape.jpg`, all 800x600. The blocks are byte-for-byte what
the command prints, trailing blanks included.

`go run ./examples/basic -mode ascii -color none -w 56 tools/stylepreview/testdata/sample.jpg`

```
**#*+*#########**#####*##########*#######*++**#******#**
##*+*##########=+++###*#####*+==########*+*##**++*##****
##++*##########=++=-++++++*+++=-*#####***##***####*****#
#*=*###########+-------::---=-:-*###**+****#####****####
*+=**##########+-=-----:----==:-*********######*########
*=***######*###++==--=--------::+*****##################
+=*#*######*##*=-=+=-+==-:---:::=***####################
=*##***#######*=+=--=-+=:=-:-=--*######**###############
*##****######**+==++==--::=+=--+*#######################
###**+***####****+++++==++++=--**#######################
###****#######***++++====------=########################
###*#######*#+**++++=--:::::--:.=#%#####################
##*#######**#*++***++==-:.::--:...-=+*##################
#**##########***++*++==-----:-::...:::=*#######*########
#*##*########***#*+==+===---::::.....::::+####++########
*###########%#***++=+==----=--:::....::::::-*#+*#*######
*#%#%%###*##%#+**+=--==-:-::--:::. ..::::::::-*#*#####*#
#####%###*%#++##*++=-:-==-:----::. ..::::::::.:*#####**#
#%%##%%##**+*###*+==--:==+--:........:::::::::::+####**#
%%%%#%%#=+#%%##**+++--:-*=::--:::::::::--=---::.:+#**#**
%%%%#*+**######**#++=-=:+=++==---::---:::-----:::.**###*
```

`go run ./examples/basic -mode ascii -color none -w 40 tools/stylepreview/testdata/sample-fruit.jpg`

```
-=--==++:----=*-====*+****+**=*++=------
=--===++.===:=*+==++++****=**=*+*=:-----
======*+.=*+-+***+*#*#####+**=##%-.-----
-=+===*+.===-=*+-=-=+####*#**+*#*=:-----
==+++**+-----+-  :...####*+##*+++==*#*-=
=:-::-*+.-==-=.      =###=:=***+++===--=
-.:::=*+.-++-==      =*##=:=**++=+=:-=--
:....:=+.--=:++.... .=####****+-=--..=--
:::::::=----==========+***+===-......::-
.............::::::.::::::::::...... .::
----::---========-:-##%%%%%###-:::.  ...
=-----++++++++++++--#%%%%%%%%#-::.      
:::::-++++=====+++--###**#%###=-::....  
.:-:.:--------===-:-*%%%%###*#=:::......
..:---::::-===--::::-=====----::........
```

**Sextant** carries its picture in the two colors of each cell. Markdown
viewers drop the escape sequences, so the two blocks below are the same
renders minus the color - run the commands in a real terminal to see them.
Six rows of the landscape, `FitCrop` into a 40x6 box, `ModeASCII` then
`ModeSextant`:

`go run ./examples/basic -w 40 -h 6 -fit crop -mode ascii -color none tools/stylepreview/testdata/sample-landscape.jpg`

```
--===++***+*++++********++++++=======+==
---===+++===+==-=====+*#+-:::::::::-----
=++-==-===+=+==+=+*****+-:::::::::::--==
+***++**++++====*+=====++=--:::::-::::--
####%#++=++*+=+++*==--=======--:::.:::::
*#+++###+++*++++++*#=---------=--::::...
```

`go run ./examples/basic -w 40 -h 6 -fit crop -mode sextant -color none tools/stylepreview/testdata/sample-landscape.jpg`

```
🬂🬂🬂🬂🬂🬂🬂🬂🬂🬂🬂🬂🬂🬂🬂🬂🬂🬂🬆🬂🬂🬊🬂🬂🬂🬂🬂🬎🬂🬂🬎🬎🬎🬂🬂🬑🬂🬂🬊🬂
🬝🬋🬱🬵🬩🬅🬇🬚🬝🬆🬵🬩🬹🬹🬭🬭🬊🬀🬞🬏🬞🬭🬻🬹🬓🬆🬂🬎🬁🬎🬂🬎🬂🬂🬊🬎🬁🬂🬂🬂
🬭🬹🬲🬏🬂🬙🬶🬞🬹🬹🬹🬱🬓🬎🬂🬨🬐🬵🬆🬂🬂🬂🬂🬲🬭🬏🬎🬎🬂🬀🬟🬭🬹🬞🬭🬭🬉🬂🬩🬥
🬵🬱🬭🬹🬹🬹🬹🬹🬏🬭🬭🬭🬭🬭🬭🬰▐🬏🬂🬂🬂🬂🬂🬂🬀🬺🬹🬹🬭🬭🬒🬁🬀🬊🬆🬦🬵🬉🬊🬄
🬷🬍🬝🬷🬛🬄🬒🬂🬦🬍🬬🬮🬶🬐🬖🬎🬎🬎🬭🬏🬂🬊🬊🬂🬎🬂🬂🬎🬋🬹🬱🬏🬒🬂🬀🬡🬭🬜🬊🬵
🬡🬍🬂🬂🬡🬷🬺🬹🬹🬭🬞🬏🬦🬏🬭🬖🬯🬵🬥🬺🬱🬏🬊🬎🬎🬎🬎🬎🬊🬊🬎🬌🬹🬹🬹🬹🬹🬹🬭🬮
```

`tools/stylepreview` writes full-color montages of every style it measured,
shipped and rejected, into `tools/stylepreview/preview-*.png`.

## Runnable examples

| Path | Shows |
|---|---|
| `examples/basic` | file or stdin → art, with every flag (`-w -h -mode -color -fit -ramp -invert`) |
| `examples/stream` | URL, pipe or file → art: the `io.Reader` path, HTTP status handling, `MaxPixels` |
| `example_test.go` | short `// Output:`-checked Go examples for godoc: `Convert`, `ConvertReader`, `ConvertBytes`, `ConvertFile`, `Art.WriteTo`, `Art.Cells` |

The smallest complete program:

```go
package main

import (
	"io"
	"os"

	"github.com/radix29/gotuipic"
)

func main() {
	// `cat cat.png | go run . -`, or `go run . cat.png`
	var src io.Reader = os.Stdin
	if len(os.Args) > 1 && os.Args[1] != "-" {
		f, err := os.Open(os.Args[1])
		if err != nil {
			panic(err)
		}
		defer f.Close()
		src = f
	}
	art, err := gotuipic.ConvertReader(src,
		gotuipic.WithWidth(80),
		gotuipic.WithColor(gotuipic.ProfileFromEnv(os.Getenv)))
	if err != nil {
		panic(err)
	}
	_, _ = art.WriteTo(os.Stdout)
}
```

```sh
go run ./examples/basic -w 100 tools/stylepreview/testdata/sample.jpg | less -R
go run ./examples/basic -mode ascii -color 256 tools/stylepreview/testdata/sample-fruit.jpg
go run ./examples/stream -mode ascii https://picsum.photos/800/600 | less -R
go doc github.com/radix29/gotuipic     # API tour
```

## Terminal notes

- **sextant** needs a terminal font that covers the U+1FB00 block-sextant
  range, and plenty of monospace fonts do not. The library cannot check for
  you (it never sees the terminal), so if the glyphs come out as boxes or
  blanks, switch to `ModeASCII`: it emits only ASCII glyphs and always
  renders.
- **Color detection is the caller's job**, because the library never reads
  the environment:

  ```go
  prof := gotuipic.ProfileFromEnv(os.Getenv) // NO_COLOR, COLORTERM, TERM
  ```

- Pipe output to a file or a pager safely: `ColorNone` emits no escape
  sequences at all, and `Art.Plain()` is always escape-free.

## Errors and limits

```go
_, err := gotuipic.ConvertReader(r)
switch {
case err == nil:                                // rendered
case errors.Is(err, gotuipic.ErrInvalidOption): // bad option values, reported together
case errors.Is(err, gotuipic.ErrTooLarge):      // header claims more pixels than MaxPixels
case errors.Is(err, image.ErrFormat):           // not a decodable image (nil/empty/garbage)
}
```

`MaxPixels` is evaluated from the image header *before* any pixel buffer is
allocated, so a 100000x100000 PNG header is rejected cheaply.

## Status

Alpha, untagged. Design: `ARCHITECTURE.md`; build order and progress:
`PLAN.md`; mode measurements (why sextant + ascii won):
`tools/stylepreview/`. MIT license.
