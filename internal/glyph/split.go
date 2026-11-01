package glyph

import (
	"image/color"
	"math"
	"math/bits"
)

// splitBuf holds the reusable subset-sum arrays for exactSplit so Render
// allocates once per call instead of once per cell.
type splitBuf struct {
	sr, sg, sb, q []float64
	cnt           []uint8
}

// exactSplit finds the two-color split of a block with the least total
// squared RGB error. Every non-trivial split is tried; a subset-sum DP
// gives each split's error in O(1), so the 2x3 block (63 splits) is cheap.
// This is the provably best fg/bg choice for one sextant glyph - 2-means
// stalls at local minima and measures strictly worse. fg ends up the
// brighter cluster.
//
// Blocks of 16 subpixels or fewer run the DP; anything larger degrades to
// a single average color (defensive only - no shipped mode exceeds 6).
func exactSplit(sub []color.NRGBA, buf *splitBuf) (uint32, color.NRGBA, color.NRGBA) {
	n := len(sub)
	switch {
	case n == 0:
		return 0, color.NRGBA{}, color.NRGBA{}
	case n == 1:
		return 1, sub[0], sub[0]
	case n > 16:
		m := meanColor(sub)
		return uint32(1<<16 - 1), m, m
	}
	p := make([][3]float64, n)
	for i, c := range sub {
		p[i] = [3]float64{float64(c.R), float64(c.G), float64(c.B)}
	}
	mask, f, b := exactSplitCore(p, buf)
	return mask, roundColor(f), roundColor(b)
}

// exactSplitCore is the space-agnostic DP: p holds the subpixels in
// whatever RGB space the metric lives in (sRGB for the shipped mode).
func exactSplitCore(p [][3]float64, buf *splitBuf) (uint32, [3]float64, [3]float64) {
	n := len(p)
	lim := 1 << uint(n)
	if len(buf.sr) < lim {
		buf.sr = make([]float64, lim)
		buf.sg = make([]float64, lim)
		buf.sb = make([]float64, lim)
		buf.q = make([]float64, lim)
		buf.cnt = make([]uint8, lim)
	}
	sr, sg, sb, q, cnt := buf.sr[:lim], buf.sg[:lim], buf.sb[:lim], buf.q[:lim], buf.cnt[:lim]
	sr[0], sg[0], sb[0], q[0], cnt[0] = 0, 0, 0, 0, 0
	for m := 1; m < lim; m++ {
		lsb := m & -m
		prev := m ^ lsb
		i := bits.TrailingZeros(uint(lsb))
		r, g, b := p[i][0], p[i][1], p[i][2]
		sr[m] = sr[prev] + r
		sg[m] = sg[prev] + g
		sb[m] = sb[prev] + b
		q[m] = q[prev] + r*r + g*g + b*b
		cnt[m] = cnt[prev] + 1
	}
	tr, tg, tb, tq := sr[lim-1], sg[lim-1], sb[lim-1], q[lim-1]
	best := math.Inf(1)
	bm := 1
	for m := 1; m < lim-1; m++ {
		k := float64(cnt[m])
		nk := float64(n) - k
		dr, dg, db := tr-sr[m], tg-sg[m], tb-sb[m]
		sse := q[m] - (sr[m]*sr[m]+sg[m]*sg[m]+sb[m]*sb[m])/k +
			(tq - q[m]) - (dr*dr+dg*dg+db*db)/nk
		if sse < best {
			best, bm = sse, m
		}
	}
	mF := div3(sr[bm], sg[bm], sb[bm], float64(cnt[bm]))
	mC := div3(tr-sr[bm], tg-sg[bm], tb-sb[bm], float64(n)-float64(cnt[bm]))
	if lum3(mC) > lum3(mF) {
		return uint32((lim - 1) &^ bm), mC, mF
	}
	return uint32(bm), mF, mC
}

func div3(r, g, b, k float64) [3]float64 { return [3]float64{r / k, g / k, b / k} }

func lum3(v [3]float64) float64 { return 0.2126*v[0] + 0.7152*v[1] + 0.0722*v[2] }

func roundColor(v [3]float64) color.NRGBA {
	q := func(x float64) uint8 {
		switch {
		case x <= 0:
			return 0
		case x >= 255:
			return 255
		default:
			return uint8(x + 0.5)
		}
	}
	return color.NRGBA{R: q(v[0]), G: q(v[1]), B: q(v[2]), A: 255}
}
