package gotuipic

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"regexp"
	"strings"
	"testing"
)

var sgr = regexp.MustCompile("\x1b\\[[0-9;]*m")

// halfRedBlue returns a w x h image, left half red, right half blue.
func halfRedBlue(w, h int) *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if 2*x < w {
				im.SetNRGBA(x, y, color.NRGBA{255, 0, 0, 255})
			} else {
				im.SetNRGBA(x, y, color.NRGBA{0, 0, 255, 255})
			}
		}
	}
	return im
}

func vGradient(w, h int) *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		v := uint8(y * 255 / (h - 1))
		for x := 0; x < w; x++ {
			im.SetNRGBA(x, y, color.NRGBA{v, v, v, 255})
		}
	}
	return im
}

func TestInvalidOptions(t *testing.T) {
	_, err := Convert(vGradient(4, 4), WithWidth(-3), WithCellAspect(0))
	if !errors.Is(err, ErrInvalidOption) {
		t.Fatalf("err = %v, want ErrInvalidOption", err)
	}
	if !strings.Contains(err.Error(), "WithWidth") || !strings.Contains(err.Error(), "CellAspect") {
		t.Errorf("both problems should be reported together: %v", err)
	}
	_, err = Convert(vGradient(4, 4), WithRamp("x"))
	if !errors.Is(err, ErrInvalidOption) {
		t.Fatalf("short ramp: %v", err)
	}
	_, err = Convert(nil)
	if !errors.Is(err, ErrInvalidOption) {
		t.Fatalf("nil image: %v", err)
	}
	_, err = Convert(vGradient(4, 4), WithMode(42))
	if !errors.Is(err, ErrInvalidOption) {
		t.Fatalf("bogus mode: %v", err)
	}
}

func TestGeometry(t *testing.T) {
	cases := []struct {
		name                string
		w, h, width, height int
		fit                 FitMode
		cols, rows          int
	}{
		{"width only tall", 4, 8, 4, 0, FitContain, 4, 4},
		{"width only wide", 1000, 1, 80, 0, FitContain, 80, 1},
		{"default width", 100, 100, 0, 0, FitContain, DefaultWidth, 40},
		{"height only", 4, 8, 0, 4, FitContain, 4, 4},
		{"contain tall shrinks cols", 4, 60, 20, 20, FitContain, 3, 20},
		{"stretch fills box", 100, 1, 10, 10, FitStretch, 10, 10},
		{"crop fills box", 100, 1, 10, 10, FitCrop, 10, 10},
	}
	for _, c := range cases {
		o, err := resolve([]Option{WithWidth(c.width), WithHeight(c.height), WithFit(c.fit)})
		if err != nil {
			t.Fatalf("%s: resolve: %v", c.name, err)
		}
		cols, rows := o.geom(c.w, c.h)
		if cols != c.cols || rows != c.rows {
			t.Errorf("%s: geom(%d,%d) = (%d,%d), want (%d,%d)", c.name, c.w, c.h, cols, rows, c.cols, c.rows)
		}
	}
}

func TestConvertSextantExactBlock(t *testing.T) {
	art, err := Convert(halfRedBlue(2, 3), WithWidth(1), WithMode(ModeSextant))
	if err != nil {
		t.Fatal(err)
	}
	if art.Cols != 1 || art.Rows != 1 {
		t.Fatalf("grid = %dx%d, want 1x1", art.Cols, art.Rows)
	}
	c := art.At(0, 0)
	if c.Rune != '▌' || c.FG != (color.RGBA{255, 0, 0, 255}) || c.BG != (color.RGBA{0, 0, 255, 255}) {
		t.Fatalf("cell = %+v", c)
	}
	if art.At(1, 0) != (Cell{}) || art.At(-1, 0) != (Cell{}) {
		t.Error("At out of range must return the zero cell")
	}
}

func TestConvertASCIIColorlessIsPlainRamps(t *testing.T) {
	art, err := Convert(vGradient(1, 8), WithWidth(1), WithMode(ModeASCII), WithColor(ColorNone))
	if err != nil {
		t.Fatal(err)
	}
	// One column, four rows: each cell covers two gradient rows, means
	// ~18, 91, 164, 237 - ramp positions 1, 3, 6, 8 of " .:-=+*#%@".
	if got := art.Plain(); got != ".\n-\n*\n%\n" {
		t.Fatalf("Plain() = %q, want %q", got, ".\n-\n*\n%\n")
	}
	if strings.Contains(art.String(), "\x1b") {
		t.Error("ColorNone must contain no escapes")
	}
}

func TestConvertInvert(t *testing.T) {
	art, err := Convert(vGradient(1, 8), WithWidth(1), WithMode(ModeASCII), WithColor(ColorNone), WithInvert(true))
	if err != nil {
		t.Fatal(err)
	}
	// Same targets, reversed ramp: the same positions read from the
	// other end ("@%#*+=-:. "[1,3,6,8]).
	if got := art.Plain(); got != "%\n*\n-\n.\n" {
		t.Fatalf("Plain() = %q, want %q", got, "%\n*\n-\n.\n")
	}
}

func TestInvariantsAllModesProfiles(t *testing.T) {
	for _, mode := range []Mode{ModeSextant, ModeASCII} {
		for _, prof := range []ColorProfile{ColorTrueColor, ColorANSI256, ColorANSI16, ColorNone} {
			art, err := Convert(halfRedBlue(64, 32), WithWidth(13), WithMode(mode), WithColor(prof))
			if err != nil {
				t.Fatalf("mode %d profile %d: %v", mode, prof, err)
			}
			if len(art.Cells) != art.Cols*art.Rows {
				t.Fatalf("cells %d != %d*%d", len(art.Cells), art.Cols, art.Rows)
			}
			lines := strings.Split(strings.TrimSuffix(art.Plain(), "\n"), "\n")
			if len(lines) != art.Rows {
				t.Fatalf("plain has %d lines, want %d", len(lines), art.Rows)
			}
			for _, l := range lines {
				if n := len([]rune(l)); n != art.Cols {
					t.Fatalf("plain line %q has %d runes, want %d", l, n, art.Cols)
				}
			}
			ansiText := art.String()
			if prof == ColorNone {
				if strings.Contains(ansiText, "\x1b") {
					t.Fatal("escapes in ColorNone output")
				}
				continue
			}
			for _, l := range strings.Split(strings.TrimSuffix(ansiText, "\n"), "\n") {
				if !strings.HasSuffix(l, "\x1b[0m") {
					t.Fatalf("row does not end in reset: %q", l)
				}
				stripped := sgr.ReplaceAllString(l, "")
				if n := len([]rune(stripped)); n != art.Cols {
					t.Fatalf("ansi row %q has %d runes, want %d", l, n, art.Cols)
				}
			}
		}
	}
}

func TestAlphaCompositedOverBackground(t *testing.T) {
	tr := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	tr.SetNRGBA(0, 0, color.NRGBA{0, 0, 0, 0})
	art, err := Convert(tr, WithWidth(1), WithHeight(1), WithFit(FitStretch), WithMode(ModeASCII))
	if err != nil {
		t.Fatal(err)
	}
	if got := art.At(0, 0).FG; got != (color.RGBA{0, 0, 0, 255}) {
		t.Errorf("transparent over default black = %v", got)
	}
	art, _ = Convert(tr, WithWidth(1), WithHeight(1), WithFit(FitStretch), WithMode(ModeASCII), WithBackground(color.White))
	if got := art.At(0, 0).FG; got != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("transparent over white = %v", got)
	}
}

// pngHeader builds a valid PNG signature + IHDR claiming w x h, with no
// image data after it. DecodeConfig can read it; decoding pixels would
// fail, so a MaxPixels rejection proves the guard ran first.
func pngHeader(w, h int) []byte {
	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	data := make([]byte, 13)
	binary.BigEndian.PutUint32(data[0:], uint32(w))
	binary.BigEndian.PutUint32(data[4:], uint32(h))
	data[8], data[9], data[10], data[11], data[12] = 8, 2, 0, 0, 0
	binary.Write(&b, binary.BigEndian, uint32(13))
	seg := append([]byte("IHDR"), data...)
	b.Write(seg)
	binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(seg))
	return b.Bytes()
}

func TestDecodeGuards(t *testing.T) {
	_, err := ConvertReader(bytes.NewReader(pngHeader(100000, 100000)))
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("bomb header: err = %v, want ErrTooLarge", err)
	}
	_, err = ConvertReader(bytes.NewReader(pngHeader(100000, 100000)), WithMaxPixels(1<<24))
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("custom MaxPixels must also reject: %v", err)
	}
	_, err = ConvertReader(bytes.NewReader([]byte("definitely not an image")))
	if !errors.Is(err, image.ErrFormat) {
		t.Fatalf("garbage: err = %v, want image.ErrFormat", err)
	}
	_, err = ConvertFile("/nonexistent/definitely/nope.png")
	if err == nil || errors.Is(err, ErrInvalidOption) {
		t.Fatalf("missing file: err = %v", err)
	}
}

func TestConvertReaderRealPNG(t *testing.T) {
	var buf bytes.Buffer
	src := halfRedBlue(40, 20)
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	art, err := ConvertReader(bytes.NewReader(buf.Bytes()), WithWidth(10))
	if err != nil {
		t.Fatal(err)
	}
	if art.Cols != 10 || art.Rows != 3 {
		t.Fatalf("grid = %dx%d, want 10x3", art.Cols, art.Rows)
	}
	var wb bytes.Buffer
	n, err := art.WriteTo(&wb)
	if err != nil {
		t.Fatal(err)
	}
	if int(n) != wb.Len() || wb.String() != art.String() {
		t.Fatal("WriteTo and String must agree")
	}
}

func TestProfileFromEnv(t *testing.T) {
	env := func(kv map[string]string) func(string) string {
		return func(k string) string { return kv[k] }
	}
	cases := []struct {
		kv   map[string]string
		want ColorProfile
	}{
		{map[string]string{}, ColorNone}, // no TERM
		{map[string]string{"TERM": "xterm-256color"}, ColorANSI256},
		{map[string]string{"TERM": "xterm-256color", "COLORTERM": "truecolor"}, ColorTrueColor},
		{map[string]string{"TERM": "xterm", "COLORTERM": "24bit"}, ColorTrueColor},
		{map[string]string{"TERM": "xterm"}, ColorANSI16},
		{map[string]string{"TERM": "dumb"}, ColorNone},
		{map[string]string{"TERM": "xterm-256color", "NO_COLOR": "1"}, ColorNone},
	}
	for _, c := range cases {
		if got := ProfileFromEnv(env(c.kv)); got != c.want {
			t.Errorf("%v -> %d, want %d", c.kv, got, c.want)
		}
	}
}

func BenchmarkConvert(b *testing.B) {
	im := image.NewNRGBA(image.Rect(0, 0, 640, 480))
	for y := 0; y < 480; y++ {
		for x := 0; x < 640; x++ {
			im.SetNRGBA(x, y, color.NRGBA{uint8(x), uint8(y), uint8((x + y) / 2), 255})
		}
	}
	for _, mode := range []Mode{ModeSextant, ModeASCII} {
		b.Run(map[Mode]string{ModeSextant: "sextant", ModeASCII: "ascii"}[mode], func(b *testing.B) {
			for b.Loop() {
				if _, err := Convert(im, WithWidth(120), WithMode(mode)); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
