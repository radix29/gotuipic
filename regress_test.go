package gotuipic

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func TestConvertRejectsBadImages(t *testing.T) {
	var typedNil *image.NRGBA
	cases := map[string]image.Image{
		"nil":       nil,
		"typed nil": typedNil,
		"empty":     image.NewNRGBA(image.Rect(0, 0, 0, 0)),
		"zero rows": image.NewNRGBA(image.Rect(0, 0, 5, 0)),
	}
	for name, img := range cases {
		if _, err := Convert(img); !errors.Is(err, ErrInvalidOption) {
			t.Errorf("%s: err = %v, want ErrInvalidOption", name, err)
		}
	}
	if _, err := Convert(image.NewUniform(color.White)); !errors.Is(err, ErrTooLarge) {
		t.Errorf("uniform: err = %v, want ErrTooLarge", err)
	}
	if _, err := Convert(halfRedBlue(10, 10), WithMaxPixels(50)); !errors.Is(err, ErrTooLarge) {
		t.Errorf("MaxPixels on image.Image: err = %v, want ErrTooLarge", err)
	}
}

func TestMaxCells(t *testing.T) {
	img := halfRedBlue(8, 8)
	for _, opts := range [][]Option{
		{WithWidth(1 << 30)},
		{WithWidth(1 << 62)},
		{WithWidth(1 << 30), WithHeight(1 << 30), WithFit(FitStretch)},
		{WithCellAspect(1e300)},
		{WithCellAspect(1e-300), WithHeight(10)},
	} {
		if _, err := Convert(img, opts...); !errors.Is(err, ErrInvalidOption) {
			t.Errorf("err = %v, want ErrInvalidOption", err)
		}
	}
	if _, err := Convert(img, WithWidth(10), WithMaxCells(5)); !errors.Is(err, ErrInvalidOption) {
		t.Errorf("small cap: err = %v", err)
	}
	if _, err := Convert(img, WithMaxCells(0)); !errors.Is(err, ErrInvalidOption) {
		t.Errorf("zero cap: err = %v", err)
	}
}

func TestWithBackgroundNilKeepsDefault(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 6)) // fully transparent
	a, err := Convert(img, WithBackground(nil), WithWidth(2), WithMode(ModeASCII))
	if err != nil {
		t.Fatal(err)
	}
	if c := a.At(0, 0); c.FG != (color.RGBA{0, 0, 0, 255}) {
		t.Fatalf("FG = %v, want default black", c.FG)
	}
}

func TestWithBackgroundAndAspect(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 6))
	a, err := Convert(img, WithBackground(color.NRGBA{10, 20, 30, 7}), WithWidth(2))
	if err != nil {
		t.Fatal(err)
	}
	if c := a.At(0, 0); c.FG != (color.RGBA{10, 20, 30, 255}) {
		t.Fatalf("FG = %v, want the background color", c.FG)
	}
	sq, _ := Convert(halfRedBlue(10, 10), WithWidth(10), WithCellAspect(1))
	half, _ := Convert(halfRedBlue(10, 10), WithWidth(10))
	if sq.Rows != 10 || half.Rows != 5 {
		t.Fatalf("rows: aspect 1 = %d, default = %d; want 10 and 5", sq.Rows, half.Rows)
	}
}

func TestArtWritersAreTotal(t *testing.T) {
	cells := []Cell{{Rune: 'a'}, {Rune: 'b'}, {Rune: 'c'}}
	for _, a := range []*Art{
		{Rows: 1, Cells: cells},
		{Cols: 2, Rows: 1, Cells: cells},
		{Cols: 3, Rows: 1},
		{},
	} {
		for _, p := range []ColorProfile{ColorTrueColor, ColorANSI256, ColorANSI16, ColorNone} {
			a.profile = p
			_ = a.String()
			_ = a.Plain()
		}
	}
}

func TestEncodeCellWithoutColorDoesNotPanic(t *testing.T) {
	for _, p := range []ColorProfile{ColorANSI256, ColorANSI16} {
		a := &Art{Cols: 1, Rows: 1, Cells: []Cell{{Rune: 'x'}}, profile: p}
		s := a.String()
		if strings.Contains(s, "-1") || !strings.Contains(s, "x") {
			t.Errorf("profile %d: %q", p, s)
		}
	}
}

func TestSolidSextantIsSpace(t *testing.T) {
	solid := image.NewNRGBA(image.Rect(0, 0, 8, 12))
	for i := range solid.Pix {
		solid.Pix[i] = 200
	}
	for _, p := range []ColorProfile{ColorTrueColor, ColorANSI256, ColorANSI16, ColorNone} {
		a, err := Convert(solid, WithWidth(4), WithColor(p))
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Trim(a.Plain(), " \n"); got != "" {
			t.Errorf("profile %d: solid image rendered %q, want spaces", p, got)
		}
	}
}

func TestInvertIsASCIIOnly(t *testing.T) {
	img := vGradient(8, 8)
	norm, _ := Convert(img, WithWidth(8), WithMode(ModeASCII), WithColor(ColorNone))
	inv, _ := Convert(img, WithWidth(8), WithMode(ModeASCII), WithColor(ColorNone), WithInvert(true))
	if norm.Plain() == inv.Plain() {
		t.Error("WithInvert did not change ASCII output")
	}
	a, _ := Convert(img, WithWidth(8))
	b, _ := Convert(img, WithWidth(8), WithInvert(true))
	if a.String() != b.String() {
		t.Error("WithInvert changed sextant output")
	}
}

func TestMaxBytes(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, halfRedBlue(8, 8)); err != nil {
		t.Fatal(err)
	}
	if _, err := ConvertBytes(buf.Bytes(), WithMaxBytes(int64(buf.Len()))); err != nil {
		t.Fatalf("exact size: %v", err)
	}
	if _, err := ConvertBytes(buf.Bytes(), WithMaxBytes(int64(buf.Len())-1)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("one byte over: err = %v, want ErrTooLarge", err)
	}
	if _, err := ConvertBytes(buf.Bytes(), WithMaxBytes(0)); !errors.Is(err, ErrInvalidOption) {
		t.Fatalf("zero: err = %v", err)
	}
}

func TestArtModeProfileAndModeString(t *testing.T) {
	a, err := Convert(halfRedBlue(8, 8), WithWidth(4), WithMode(ModeASCII), WithColor(ColorANSI16))
	if err != nil {
		t.Fatal(err)
	}
	if a.Mode() != ModeASCII || a.Profile() != ColorANSI16 {
		t.Fatalf("Mode/Profile = %v/%v", a.Mode(), a.Profile())
	}
	if ModeSextant.String() != "sextant" || ModeASCII.String() != "ascii" || Mode(9).String() != "Mode(9)" {
		t.Fatal("Mode.String mismatch")
	}
}
