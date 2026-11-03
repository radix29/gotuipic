package glyph

import (
	"image/color"
	"math"
	"math/rand"
	"testing"
)

// bruteSplit recomputes the best split the slow, obvious way: every
// partition, means recomputed from scratch.
func bruteSplit(p [][3]float64) (mask uint32, fg, bg [3]float64, sse float64) {
	n := len(p)
	sse = math.Inf(1)
	for m := 1; m < (1<<uint(n))-1; m++ {
		var mr, mg, mb, kr, kg, kb float64
		var k int
		for i := 0; i < n; i++ {
			if m>>uint(i)&1 == 1 {
				mr += p[i][0]
				mg += p[i][1]
				mb += p[i][2]
				k++
			} else {
				kr += p[i][0]
				kg += p[i][1]
				kb += p[i][2]
			}
		}
		f := [3]float64{mr / float64(k), mg / float64(k), mb / float64(k)}
		bf := [3]float64{kr / float64(n-k), kg / float64(n-k), kb / float64(n-k)}
		var e float64
		for i := 0; i < n; i++ {
			t := bf
			if m>>uint(i)&1 == 1 {
				t = f
			}
			for c := 0; c < 3; c++ {
				d := p[i][c] - t[c]
				e += d * d
			}
		}
		if e < sse {
			sse, mask, fg, bg = e, uint32(m), f, bf
		}
	}
	return
}

func sseOf(p [][3]float64, mask uint32, fg, bg [3]float64) float64 {
	var e float64
	for i := range p {
		t := bg
		if mask>>uint(i)&1 == 1 {
			t = fg
		}
		for c := 0; c < 3; c++ {
			d := p[i][c] - t[c]
			e += d * d
		}
	}
	return e
}

func TestExactSplitMatchesBruteForce(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for trial := 0; trial < 300; trial++ {
		sub := make([]color.NRGBA, 6)
		p := make([][3]float64, 6)
		for i := range sub {
			sub[i] = color.NRGBA{
				R: uint8(rng.Intn(256)), G: uint8(rng.Intn(256)),
				B: uint8(rng.Intn(256)), A: 255,
			}
			p[i] = [3]float64{float64(sub[i].R), float64(sub[i].G), float64(sub[i].B)}
		}
		mask, fgN, bgN := exactSplit((*[subpixels]color.NRGBA)(sub))
		fg := [3]float64{float64(fgN.R), float64(fgN.G), float64(fgN.B)}
		bg := [3]float64{float64(bgN.R), float64(bgN.G), float64(bgN.B)}
		got := sseOf(p, mask, fg, bg)

		_, _, _, bsse := bruteSplit(p)
		// The DP picks the same partition as brute force; the shipped means
		// are then rounded to integers, which can cost at most ~n*3*0.25.
		if got > bsse+5.0 {
			t.Fatalf("trial %d: exact %v (mask %06b) worse than brute %v",
				trial, got, mask, bsse)
		}
		if lum3(fg)+1.0 < lum3(bg) {
			t.Fatalf("trial %d: fg %v not the brighter cluster than bg %v", trial, fg, bg)
		}
	}
}

func TestExactSplitFlatBlockIsBlank(t *testing.T) {
	red := color.NRGBA{255, 0, 0, 255}
	var sub [subpixels]color.NRGBA
	for i := range sub {
		sub[i] = red
	}
	m, f, b := exactSplit(&sub)
	if m != 0 || f != red || b != red {
		t.Fatalf("flat block: mask %d fg %v bg %v, want mask 0 and red", m, f, b)
	}
}
