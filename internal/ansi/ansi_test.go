package ansi

import (
	"bytes"
	"image/color"
	"regexp"
	"strings"
	"testing"
)

var sgr = regexp.MustCompile("\x1b\\[[0-9;]*m")

func cell(c rune, fg, bg color.RGBA) Cell {
	return Cell{Rune: c, FG: fg, BG: bg, IdxFG: -1, IdxBG: -1}
}

var (
	red   = color.RGBA{255, 0, 0, 255}
	blue  = color.RGBA{0, 0, 255, 255}
	noBG  = color.RGBA{0, 0, 0, 0}
	white = color.RGBA{255, 255, 255, 255}
)

func encodeStr(t *testing.T, p Profile, cols int, cells []Cell) string {
	t.Helper()
	var b bytes.Buffer
	if _, err := Encode(&b, p, cols, cells); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestChangeOnlyEmissionAndReset(t *testing.T) {
	// Two identical cells share one FG change; every row ends reset.
	same := cell('█', red, blue)
	out := encodeStr(t, TrueColor, 2, []Cell{same, same, same, same})
	if got := strings.Count(out, "38;2;255;0;0"); got != 2 { // once per row
		t.Errorf("FG emissions = %d, want 2: %q", got, out)
	}
	if got := strings.Count(out, "\x1b[0m\n"); got != 2 {
		t.Errorf("row resets = %d, want 2", got)
	}
	// Different FG emits again mid-row.
	out = encodeStr(t, TrueColor, 2, []Cell{cell('█', red, noBG), cell('█', blue, noBG)})
	if !strings.Contains(out, "38;2;255;0;0") || !strings.Contains(out, "38;2;0;0;255") {
		t.Errorf("missing a color change: %q", out)
	}
}

func TestPlainRuneCount(t *testing.T) {
	cells := []Cell{cell('🬀', white, white), cell('▐', white, white), cell('x', white, white), cell('ß', white, white)}
	out := encodeStr(t, TrueColor, 2, cells)
	stripped := sgr.ReplaceAllString(out, "")
	if stripped != "🬀▐\nxß\n" {
		t.Fatalf("text = %q", stripped)
	}
}

func TestNoBackgroundMode(t *testing.T) {
	out := encodeStr(t, TrueColor, 2, []Cell{cell('a', red, noBG), cell('b', blue, noBG)})
	if strings.Contains(out, "48;") || strings.Contains(out, "49m") {
		t.Errorf("bg SGR leaked into an ascii row: %q", out)
	}
}

func TestBackgroundRestoredWhenItStops(t *testing.T) {
	out := encodeStr(t, TrueColor, 2, []Cell{cell('a', red, blue), cell('b', red, noBG)})
	if !strings.Contains(out, "\x1b[49m") {
		t.Errorf("expected default-bg restore: %q", out)
	}
}

func TestPaletteDialects(t *testing.T) {
	a := Cell{Rune: 'x', FG: red, BG: blue, IdxFG: 9, IdxBG: 9}
	out := encodeStr(t, ANSI256, 1, []Cell{a})
	if !strings.Contains(out, "\x1b[38;5;9m") || !strings.Contains(out, "\x1b[48;5;9m") {
		t.Errorf("256 dialect: %q", out)
	}
	out = encodeStr(t, ANSI16, 1, []Cell{a})
	if !strings.Contains(out, "\x1b[91m") || !strings.Contains(out, "\x1b[101m") {
		t.Errorf("16 dialect: %q", out)
	}
	if strings.Contains(out, ";2;") || strings.Contains(out, ";5;") {
		t.Errorf("16 dialect used 256/truecolor forms: %q", out)
	}
}

func TestEmptyInput(t *testing.T) {
	var b bytes.Buffer
	n, err := Encode(&b, TrueColor, 0, nil)
	if n != 0 || err != nil || b.Len() != 0 {
		t.Fatalf("empty encode wrote %d bytes, err %v", n, err)
	}
}
