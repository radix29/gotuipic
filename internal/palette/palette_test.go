package palette

import (
	"image/color"
	"testing"
)

func TestLumaEndpoints(t *testing.T) {
	if got := Luma(0, 0, 0); got != 0 {
		t.Errorf("black luma = %v", got)
	}
	if got := Luma(255, 255, 255); got-255 > 1e-6 || got-255 < -1e-6 {
		t.Errorf("white luma = %v, want 255", got)
	}
}

func TestTable256Structure(t *testing.T) {
	wantCube := [6]uint8{0, 95, 135, 175, 215, 255}
	for r := 0; r < 6; r++ {
		for g := 0; g < 6; g++ {
			for b := 0; b < 6; b++ {
				c := Table256[16+36*r+6*g+b]
				if c.R != wantCube[r] || c.G != wantCube[g] || c.B != wantCube[b] {
					t.Fatalf("cube color %d%d%d = %v", r, g, b, c)
				}
			}
		}
	}
	for i := 0; i < 24; i++ {
		c := Table256[232+i]
		if want := uint8(8 + 10*i); c.R != want || c.G != want || c.B != want {
			t.Fatalf("gray %d = %v, want %d", i, c, want)
		}
	}
	if Table256[0] != (color.RGBA{0, 0, 0, 255}) {
		t.Fatal("256 palette must start with the ANSI16 block")
	}
}

// The palette contains duplicate colors (the 16 base colors also exist in
// the cube and gray ramp), so identity is checked as a color, not an
// index: nearest search may never round a palette color to a different
// color.
func TestNearestIsIdentityOnPalette(t *testing.T) {
	for _, c := range Table16 {
		if got := Nearest16(c.R, c.G, c.B); Table16[got] != c {
			t.Errorf("Nearest16(%v) = %d (%v), want itself", c, got, Table16[got])
		}
	}
	// Indices 0-15 are themed by terminals and are never chosen.
	for i := 16; i < 256; i++ {
		c := Table256[i]
		if got := Nearest256(c.R, c.G, c.B); Table256[got] != c {
			t.Errorf("Nearest256(#%d=%v) = %d (%v), want itself", i, c, got, Table256[got])
		}
	}
}

func TestNearest256SkipsThemedColors(t *testing.T) {
	for _, v := range [][3]uint8{{0, 0, 0}, {205, 0, 0}, {255, 255, 255}, {127, 127, 127}} {
		if got := Nearest256(v[0], v[1], v[2]); got < 16 {
			t.Errorf("Nearest256(%v) = %d, want >= 16", v, got)
		}
	}
}

func TestNearestFindsPureColors(t *testing.T) {
	if c := Table256[Nearest256(255, 0, 0)]; c != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("red -> %v", c)
	}
	if c := Table256[Nearest256(0, 255, 0)]; c != (color.RGBA{0, 255, 0, 255}) {
		t.Errorf("green -> %v", c)
	}
	if c := Table16[Nearest16(250, 5, 5)]; c != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("near-red -> %v", c)
	}
	// Mid gray must pick the gray ramp, not a dark cube level.
	if c := Table256[Nearest256(128, 128, 128)]; c.R < 118 || c.R > 138 {
		t.Errorf("gray -> %v", c)
	}
}
