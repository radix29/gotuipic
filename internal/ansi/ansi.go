// Package ansi turns a grid of rendered cells into SGR-escaped terminal
// text. It emits an escape only when the color actually changes and ends
// every row with ESC[0m so color never bleeds into the next line.
package ansi

import (
	"fmt"
	"image/color"
	"io"
	"unicode/utf8"
)

// Profile selects the escape dialect. The values mirror
// gotuipic.ColorProfile minus ColorNone (plain output never comes through
// here).
type Profile int

const (
	TrueColor Profile = iota // 38;2;r;g;b / 48;2;r;g;b
	ANSI256                  // 38;5;n / 48;5;n, using the Idx fields
	ANSI16                   // 30-37,90-97 / 40-47,100-107, using the Idx fields
)

// Cell is one character to write. FG is always honored. BG is honored only
// when BG.A != 0; otherwise the writer makes sure the terminal default is
// restored. IdxFG/IdxBG are the palette indices for ANSI256/ANSI16 and are
// unused for TrueColor.
type Cell struct {
	Rune         rune
	FG, BG       color.RGBA
	IdxFG, IdxBG int16
}

// fg16 maps a palette index 0-15 to its foreground SGR code; background is
// always +10.
var fg16 = [16]int{30, 31, 32, 33, 34, 35, 36, 37, 90, 91, 92, 93, 94, 95, 96, 97}

// Encode writes cells as rows of cols characters (len(cells) must be a
// multiple of cols; when it is not, the ragged tail is still emitted) to w,
// and reports the byte count.
func Encode(w io.Writer, p Profile, cols int, cells []Cell) (int64, error) {
	if cols <= 0 || len(cells) == 0 {
		return 0, nil
	}
	out := make([]byte, 0, len(cells)*12+64)
	var prevFG, prevBG color.RGBA
	var haveFG, haveBG bool
	for i, c := range cells {
		if i%cols == 0 {
			if i > 0 {
				out = append(out, "\x1b[0m\n"...)
			}
			haveFG, haveBG = false, false
		}
		if !haveFG || c.FG != prevFG {
			switch p {
			case TrueColor:
				out = fmt.Appendf(out, "\x1b[38;2;%d;%d;%dm", c.FG.R, c.FG.G, c.FG.B)
			case ANSI256:
				if c.IdxFG < 0 || c.IdxFG > 255 {
					out = append(out, "\x1b[39m"...)
				} else {
					out = fmt.Appendf(out, "\x1b[38;5;%dm", c.IdxFG)
				}
			case ANSI16:
				if c.IdxFG < 0 || c.IdxFG > 15 {
					out = append(out, "\x1b[39m"...)
				} else {
					out = fmt.Appendf(out, "\x1b[%dm", fg16[c.IdxFG])
				}
			}
			prevFG, haveFG = c.FG, true
		}
		if c.BG.A != 0 && (p == TrueColor || c.IdxBG >= 0) {
			if !haveBG || c.BG != prevBG {
				switch p {
				case TrueColor:
					out = fmt.Appendf(out, "\x1b[48;2;%d;%d;%dm", c.BG.R, c.BG.G, c.BG.B)
				case ANSI256:
					out = fmt.Appendf(out, "\x1b[48;5;%dm", min(c.IdxBG, 255))
				case ANSI16:
					out = fmt.Appendf(out, "\x1b[%dm", fg16[min(c.IdxBG, 15)]+10)
				}
				prevBG, haveBG = c.BG, true
			}
		} else if haveBG {
			out = append(out, "\x1b[49m"...)
			haveBG = false
		}
		out = utf8Rune(out, c.Rune)
	}
	out = append(out, "\x1b[0m\n"...)
	n, err := w.Write(out)
	return int64(n), err
}

func utf8Rune(out []byte, r rune) []byte {
	return utf8.AppendRune(out, r)
}
