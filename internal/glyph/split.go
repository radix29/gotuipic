package glyph

import (
	"image/color"
	"math"
)

// subpixels is the block size the split handles: the 2x3 sextant cell.
const subpixels = 6

// exactSplit finds the two-color split of a 2x3 block with the least total
// squared sRGB error. Every non-trivial split (62 of them, counting a split
// and its complement once each) is tried; subset sums give each split's
// error in O(1). This is the provably best fg/bg choice for one sextant
// glyph - 2-means stalls at local minima and measures strictly worse. fg
// ends up the brighter cluster.
//
// A block whose two means round to the same color is a flat block: it
// returns mask 0 (a space) with both colors set to that mean.
func exactSplit(sub *[subpixels]color.NRGBA) (uint32, color.NRGBA, color.NRGBA) {
	const lim = 1 << subpixels
	var p [subpixels][3]float64
	for i, c := range sub {
		p[i] = [3]float64{float64(c.R), float64(c.G), float64(c.B)}
	}
	var sr, sg, sb, q [lim]float64
	var cnt [lim]uint8
	for m := 1; m < lim; m++ {
		lsb := m & -m
		prev := m ^ lsb
		i := trailingZeros6(lsb)
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
		nk := float64(subpixels) - k
		dr, dg, db := tr-sr[m], tg-sg[m], tb-sb[m]
		sse := q[m] - (sr[m]*sr[m]+sg[m]*sg[m]+sb[m]*sb[m])/k +
			(tq - q[m]) - (dr*dr+dg*dg+db*db)/nk
		if sse < best {
			best, bm = sse, m
		}
	}
	mF := div3(sr[bm], sg[bm], sb[bm], float64(cnt[bm]))
	mC := div3(tr-sr[bm], tg-sg[bm], tb-sb[bm], float64(subpixels)-float64(cnt[bm]))
	fg, bg := roundColor(mF), roundColor(mC)
	if fg == bg {
		return 0, fg, bg
	}
	if lum3(mC) > lum3(mF) {
		return uint32((lim - 1) &^ bm), roundColor(mC), roundColor(mF)
	}
	return uint32(bm), fg, bg
}

// trailingZeros6 returns the index of the single set bit in a power of two
// below 64.
func trailingZeros6(x int) int {
	n := 0
	for x > 1 {
		x >>= 1
		n++
	}
	return n
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
