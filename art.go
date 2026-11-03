package gotuipic

import (
	"bytes"
	"image/color"
	"io"
	"unicode/utf8"

	"github.com/radix29/gotuipic/internal/ansi"
	"github.com/radix29/gotuipic/internal/palette"
)

// Cell is one character of the finished art.
type Cell struct {
	Rune rune
	// FG and BG are the final colors after quantization. A == 0 means the
	// cell carries no background (terminal default) - always the case for
	// ModeASCII, and for ColorNone where both are zero because no color is
	// emitted anywhere.
	FG, BG color.RGBA
}

// Art is the product: a grid of cells. ANSI text is just one rendering of
// it; TUI programs read Cells directly.
type Art struct {
	Cols, Rows int
	Cells      []Cell // row-major, len == Cols*Rows

	mode    Mode
	profile ColorProfile
}

// Mode reports which style produced the art.
func (a *Art) Mode() Mode { return a.mode }

// Profile reports the color profile the art was quantized to.
func (a *Art) Profile() ColorProfile { return a.profile }

// At returns the cell at (col, row), or the zero Cell when out of range,
// so render loops never panic.
func (a *Art) At(col, row int) Cell {
	if col < 0 || row < 0 || col >= a.Cols || row >= a.Rows {
		return Cell{}
	}
	return a.Cells[row*a.Cols+col]
}

// String renders the art as ANSI text, one row per line (each line ends
// with a newline). For ColorNone this equals Plain.
func (a *Art) String() string {
	var b bytes.Buffer
	_, _ = a.WriteTo(&b)
	return b.String()
}

// Plain returns the runes only, one row per line, no escape sequences.
func (a *Art) Plain() string {
	var b bytes.Buffer
	_, _ = a.writePlain(&b)
	return b.String()
}

// WriteTo implements io.WriterTo; it writes the same text as String.
func (a *Art) WriteTo(w io.Writer) (int64, error) {
	if a.profile == ColorNone {
		return a.writePlain(w)
	}
	if !a.valid() {
		return 0, nil
	}
	var prof ansi.Profile
	switch a.profile {
	case ColorANSI256:
		prof = ansi.ANSI256
	case ColorANSI16:
		prof = ansi.ANSI16
	default:
		prof = ansi.TrueColor
	}
	cells := make([]ansi.Cell, len(a.Cells))
	for i, c := range a.Cells {
		ac := ansi.Cell{Rune: c.Rune, FG: c.FG, BG: c.BG, IdxFG: -1, IdxBG: -1}
		switch a.profile {
		case ColorANSI256:
			if c.FG.A != 0 {
				ac.IdxFG = int16(palette.Nearest256(c.FG.R, c.FG.G, c.FG.B))
			}
			if c.BG.A != 0 {
				ac.IdxBG = int16(palette.Nearest256(c.BG.R, c.BG.G, c.BG.B))
			}
		case ColorANSI16:
			if c.FG.A != 0 {
				ac.IdxFG = int16(palette.Nearest16(c.FG.R, c.FG.G, c.FG.B))
			}
			if c.BG.A != 0 {
				ac.IdxBG = int16(palette.Nearest16(c.BG.R, c.BG.G, c.BG.B))
			}
		}
		cells[i] = ac
	}
	return ansi.Encode(w, prof, a.Cols, cells)
}

// valid reports whether Cols, Rows and Cells agree. The fields are
// exported, so the writers treat an inconsistent Art as empty rather than
// panic.
func (a *Art) valid() bool {
	return a.Cols > 0 && a.Rows > 0 && len(a.Cells) == a.Cols*a.Rows
}

func (a *Art) writePlain(w io.Writer) (int64, error) {
	if !a.valid() {
		return 0, nil
	}
	out := make([]byte, 0, len(a.Cells)*utf8.UTFMax+1)
	for i, c := range a.Cells {
		if i > 0 && i%a.Cols == 0 {
			out = append(out, '\n')
		}
		out = utf8.AppendRune(out, c.Rune)
	}
	if len(out) > 0 {
		out = append(out, '\n')
		n, err := w.Write(out)
		return int64(n), err
	}
	return 0, nil
}
