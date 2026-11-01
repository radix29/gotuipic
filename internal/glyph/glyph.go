// Package glyph maps blocks of pixels to terminal cells: the ASCII ramp and
// the sextant exact two-color split, which are the two shipped modes.
package glyph

import (
	"image"
	"image/color"

	"github.com/radix29/gotuipic/internal/palette"
)

// Mode is a rendering style. The zero value is the library default.
type Mode int

const (
	// ModeSextant samples 2x3 pixels per cell into two colors with U+1FB00
	// block sextants.
	ModeSextant Mode = iota
	// ModeASCII samples 1x2 pixels per cell into one FG-colored ramp glyph.
	ModeASCII
)

// Size returns the pixels sampled per cell column and row.
func Size(m Mode) (pw, ph int) {
	switch m {
	case ModeASCII:
		return 1, 2
	default:
		return 2, 3
	}
}

// Valid reports whether m is a defined mode.
func Valid(m Mode) bool { return m == ModeSextant || m == ModeASCII }

// Cell is one rendered character. BG.A == 0 means "no background" (the
// terminal default); sextant sets BG, ASCII does not.
type Cell struct {
	Rune   rune
	FG, BG color.NRGBA
}

// Render slices grid (which must be cols*pw x rows*ph, the output of the
// raster resize) into cells.
//
// ramp holds the ASCII glyph ramp (len >= 2, ordered dark to light).
// weighted picks ramp positions so the cell's own FG color is taken into
// account (correct when the terminal will actually print FG); when false a
// plain luma-index pick is used (correct for ColorNone, where only the
// glyph's ink carries the picture).
func Render(grid *image.NRGBA, m Mode, cols, rows int, ramp []rune, weighted bool) []Cell {
	pw, ph := Size(m)
	cells := make([]Cell, cols*rows)
	sub := make([]color.NRGBA, pw*ph)
	var buf splitBuf
	for cy := 0; cy < rows; cy++ {
		for cx := 0; cx < cols; cx++ {
			for sy := 0; sy < ph; sy++ {
				for sx := 0; sx < pw; sx++ {
					sub[sy*pw+sx] = grid.NRGBAAt(cx*pw+sx, cy*ph+sy)
				}
			}
			i := cy*cols + cx
			switch m {
			case ModeASCII:
				fg := meanColor(sub)
				t := meanLum(sub)
				var idx int
				if weighted {
					idx = pickWeighted(t, palette.Luma(fg.R, fg.G, fg.B), len(ramp))
				} else {
					idx = pickPlain(t, len(ramp))
				}
				cells[i] = Cell{Rune: ramp[idx], FG: fg}
			default:
				mask, fg, bg := exactSplit(sub, &buf)
				cells[i] = Cell{Rune: sextantGlyph(mask), FG: fg, BG: bg}
			}
		}
	}
	return cells
}

// pickPlain maps target luma straight onto the ramp (classic ASCII art).
func pickPlain(t float64, n int) int {
	i := int(t/255*float64(n-1) + 0.5)
	if i < 0 {
		i = 0
	}
	if i > n-1 {
		i = n - 1
	}
	return i
}

// pickWeighted returns the ramp index whose light - FG luma scaled by the
// glyph's coverage, approximated by its ramp position - lands closest on
// the target luma t. The pickPlain rule ignores the FG term and renders
// colored areas up to twice too dark; this halves RMSE and Oklab dE on
// measured photos. A near-black FG can emit almost no light whatever the
// glyph, so the densest glyph wins there (the cell reads as its FG either
// way).
func pickWeighted(t, lfg float64, n int) int {
	gain := lfg / 255
	if gain < 0.05 {
		return n - 1
	}
	i := int(t/255/gain*float64(n-1) + 0.5)
	if i < 0 {
		i = 0
	}
	if i > n-1 {
		i = n - 1
	}
	return i
}

// sextantGlyph maps a row-major membership mask (bit i = subpixel i, bit 0
// top-left ... bit 5 bottom-right) to a rune.
//
// Unicode numbers its sextants row-major - 1=TL 2=TR 3=ML 4=MR 5=BL 6=BR -
// and U+1FB00 counts the membership bitmask N (bit k-1 = sextant k) up in
// binary. That makes N exactly this function's mask. Two values are absent
// from the block because older glyphs already cover their shapes: N=21
// ("135", the whole left column, = U+258C ▌) and N=42 ("246", right column,
// = U+2590 ▐); N=0 is space and N=63 is U+2588 █. Code points above each
// gap shift down. Verified against UnicodeData.txt names: 1FB00=SEXTANT-1,
// 1FB06=SEXTANT-123, 1FB13=SEXTANT-35, 1FB14=SEXTANT-235 (gap at 21),
// 1FB27=SEXTANT-146, 1FB28=SEXTANT-1246 (gap at 42), 1FB39=SEXTANT-3456,
// 1FB3B=SEXTANT-23456.
func sextantGlyph(mask uint32) rune {
	switch mask {
	case 0:
		return ' '
	case 21: // "135" = left column
		return '▌'
	case 42: // "246" = right column
		return '▐'
	case 63:
		return '█'
	}
	off := rune(1) // code points are 1-based into the block
	if mask > 21 {
		off++
	}
	if mask > 42 {
		off++
	}
	return 0x1FB00 + rune(mask) - off
}

// ---------------------------------------------------------------------------
// block stats

func meanColor(sub []color.NRGBA) color.NRGBA {
	var r, g, b, n uint64
	for _, c := range sub {
		r += uint64(c.R)
		g += uint64(c.G)
		b += uint64(c.B)
		n++
	}
	if n == 0 {
		return color.NRGBA{}
	}
	return color.NRGBA{R: uint8((r + n/2) / n), G: uint8((g + n/2) / n), B: uint8((b + n/2) / n), A: 255}
}

func meanLum(sub []color.NRGBA) float64 {
	var s float64
	for _, c := range sub {
		s += palette.Luma(c.R, c.G, c.B)
	}
	return s / float64(len(sub))
}
