package glyph

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func TestSextantGlyphKnownShapes(t *testing.T) {
	// Expected code points cross-checked against UnicodeData.txt names.
	// Unicode numbers sextants row-major (1=TL 2=TR 3=ML 4=MR 5=BL 6=BR)
	// with bitmask weight 2^(k-1), which is exactly this package's mask:
	// 1FB00=SEXTANT-1, 1FB01=SEXTANT-2, 1FB03=SEXTANT-3, 1FB06=SEXTANT-123,
	// 1FB0C=SEXTANT-134, 1FB33=SEXTANT-2356, 1FB39=SEXTANT-3456,
	// 1FB3B=SEXTANT-23456, and the gaps at 21/42 are covered by legacy
	// U+258C/U+2590 (and 63 by U+2588).
	cases := []struct {
		mask uint32
		want rune
	}{
		{0, ' '},
		{0b000001, 0x1FB00},  // "1" = TL
		{0b000010, 0x1FB01},  // "2" = TR
		{0b000100, 0x1FB03},  // "3" = ML
		{0b001000, 0x1FB07},  // "4" = MR
		{0b010000, 0x1FB0F},  // "5" = BL
		{0b100000, 0x1FB1E},  // "6" = BR
		{0b000011, 0x1FB02},  // "12" = top row
		{0b000111, 0x1FB06},  // "123"
		{0b010101, '\u258c'}, // "135" = left column -> legacy ▌
		{0b101010, '\u2590'}, // "246" = right column -> legacy ▐
		{0b111111, '\u2588'}, // full -> █
		{0b001101, 0x1FB0C},  // "134"
		{0b110110, 0x1FB33},  // "2356"
		{0b111100, 0x1FB39},  // "3456"
		{0b111110, 0x1FB3B},  // "23456" = all but TL
	}
	for _, c := range cases {
		if got := sextantGlyph(c.mask); got != c.want {
			t.Errorf("mask %06b: got %#U, want %#U", c.mask, got, c.want)
		}
	}
}

func TestSextantGlyphBlockCoverage(t *testing.T) {
	// Every mask renders a rune; masks other than the four covered by
	// legacy glyphs (space, ▌, ▐, █) land inside U+1FB00..U+1FB3B and all
	// 60 are distinct.
	seen := map[rune]uint32{}
	for m := uint32(0); m < 64; m++ {
		r := sextantGlyph(m)
		if r == 0 && m != 0 {
			t.Fatalf("mask %d has no glyph", m)
		}
		switch m {
		case 0, 21, 42, 63:
			continue
		}
		if r < 0x1FB00 || r > 0x1FB3B {
			t.Fatalf("mask %06b: %#U outside sextant block", m, r)
		}
		if prev, dup := seen[r]; dup {
			t.Fatalf("masks %d and %d share %#U", prev, m, r)
		}
		seen[r] = m
	}
	if len(seen) != 60 {
		t.Fatalf("got %d block glyphs, want 60", len(seen))
	}
}

func TestPickWeightedIsOptimal(t *testing.T) {
	const n = 10
	for lfg := 0.0; lfg <= 255; lfg += 8 {
		gain := lfg / 255
		for tl := 0.0; tl <= 255; tl += 8 {
			i := pickWeighted(tl, lfg, n)
			if i < 0 || i >= n {
				t.Fatalf("index %d out of range", i)
			}
			if gain < 0.05 {
				continue // densest glyph by rule; nothing to optimize
			}
			out := func(j int) float64 { return gain * float64(j) / float64(n-1) * 255 }
			best := math.Inf(1)
			for j := 0; j < n; j++ {
				if e := math.Abs(out(j) - tl); e < best {
					best = e
				}
			}
			if e := math.Abs(out(i) - tl); e > best+1e-9 {
				t.Fatalf("lfg=%v target=%v: pick %d err %v, optimum %v", lfg, tl, i, e, best)
			}
		}
	}
}

func TestPickPlain(t *testing.T) {
	if got := pickPlain(0, 10); got != 0 {
		t.Errorf("black -> %d", got)
	}
	if got := pickPlain(255, 10); got != 9 {
		t.Errorf("white -> %d", got)
	}
}

func gridFrom(xpix func(x, y int) color.NRGBA, w, h int) *image.NRGBA {
	g := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			g.SetNRGBA(x, y, xpix(x, y))
		}
	}
	return g
}

func TestRenderSextantLeftRight(t *testing.T) {
	red := color.NRGBA{255, 0, 0, 255}
	blue := color.NRGBA{0, 0, 255, 255}
	g := gridFrom(func(x, y int) color.NRGBA {
		if x == 0 {
			return red
		}
		return blue
	}, 2, 3)
	ramp := []rune("ab")
	cells := Render(g, ModeSextant, 1, 1, ramp, true)
	c := cells[0]
	// Red (luma 54) is brighter than blue (luma 18), so FG is the left
	// column: mask 0b010101 = 21 -> the legacy left-half glyph.
	if c.Rune != '▌' || c.FG != red || c.BG != blue {
		t.Fatalf("cell = %+v, want ▌ FG red BG blue", c)
	}
}

func TestRenderSextantSolidImageHasEqualColors(t *testing.T) {
	red := color.NRGBA{255, 0, 0, 255}
	g := gridFrom(func(x, y int) color.NRGBA { return red }, 2, 3)
	c := Render(g, ModeSextant, 1, 1, []rune("ab"), true)[0]
	// A solid block is a space on a red background, never a rare sextant.
	if c.Rune != ' ' || c.FG != red || c.BG != red {
		t.Fatalf("solid cell = %+v, want FG == BG == red", c)
	}
}

func TestRenderASCIIWeightedVsPlain(t *testing.T) {
	red := color.NRGBA{255, 0, 0, 255} // luma 0.2126*255 = 54.2
	g := gridFrom(func(x, y int) color.NRGBA { return red }, 1, 2)
	ramp := []rune(" .:-=+*#%@")
	plain := Render(g, ModeASCII, 1, 1, ramp, false)[0]
	weighted := Render(g, ModeASCII, 1, 1, ramp, true)[0]
	// Plain maps luma 54 straight to a ramp position (index 2 = ':');
	// weighted notices that pure red only emits 21% of white's light and
	// picks the densest glyph.
	if want := ':'; plain.Rune != want {
		t.Fatalf("plain rune = %q, want %q", plain.Rune, want)
	}
	if want := '@'; weighted.Rune != want {
		t.Fatalf("weighted rune = %q, want %q", weighted.Rune, want)
	}
	if weighted.FG != red || plain.FG != red {
		t.Fatalf("FG = %+v / %+v, want red", plain, weighted)
	}
}
