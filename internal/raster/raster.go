// Package raster prepares images for glyph rendering: alpha compositing to
// an opaque NRGBA image, center-cropping to an aspect ratio, and the
// deterministic sRGB box downscale that every mode relies on.
package raster

import (
	"image"
	"image/color"
	"math"
)

// Prepare converts src to an opaque *image.NRGBA with its top-left at the
// origin, compositing any alpha over bg. Channels are blended straight
// (unpremultiplied), which is good enough for background matting.
func Prepare(src image.Image, bg color.NRGBA) *image.NRGBA {
	// Already in the output form: nothing to convert or composite, and every
	// later stage only reads its input, so it is passed through uncopied. This
	// is what makes Convert cheap on an image a caller flattened once and
	// renders many times (a viewer re-fitting on resize); the generic loop
	// below costs an interface call and a color conversion per pixel.
	if n, ok := src.(*image.NRGBA); ok && n.Rect.Min == (image.Point{}) && n.Opaque() {
		return n
	}
	sb := src.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, sb.Dx(), sb.Dy()))
	obl := out.Bounds()
	for y := 0; y < obl.Dy(); y++ {
		for x := 0; x < obl.Dx(); x++ {
			n := color.NRGBAModel.Convert(src.At(sb.Min.X+x, sb.Min.Y+y)).(color.NRGBA)
			if n.A == 255 {
				out.SetNRGBA(x, y, color.NRGBA{n.R, n.G, n.B, 255})
				continue
			}
			a := float64(n.A) / 255
			blend := func(s, d uint8) uint8 {
				return uint8(a*float64(s) + (1-a)*float64(d) + 0.5)
			}
			out.SetNRGBA(x, y, color.NRGBA{
				R: blend(n.R, bg.R), G: blend(n.G, bg.G), B: blend(n.B, bg.B), A: 255,
			})
		}
	}
	return out
}

// BoxResize averages src down to exactly dw x dh pixels in sRGB. Each output
// pixel averages an integer-floored window of source pixels (at least one),
// so edge pixels are not fractionally weighted. Growing repeats pixels (a
// nearest-style upscale). dw and dh must be >= 1, and src must not be empty.
//
// This filter is part of the public behavior: golden files depend on its
// exact output, so never swap in a different resampler. Linear-light
// averaging was measured and scored worse for both shipped modes.
func BoxResize(src *image.NRGBA, dw, dh int) *image.NRGBA {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	out := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	for oy := 0; oy < dh; oy++ {
		y0 := oy * sh / dh
		y1 := (oy + 1) * sh / dh
		if y1 <= y0 {
			y1 = y0 + 1
		}
		if y1 > sh {
			y1 = sh
		}
		for ox := 0; ox < dw; ox++ {
			x0 := ox * sw / dw
			x1 := (ox + 1) * sw / dw
			if x1 <= x0 {
				x1 = x0 + 1
			}
			if x1 > sw {
				x1 = sw
			}
			var r, g, b, n uint64
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					c := src.NRGBAAt(sb.Min.X+x, sb.Min.Y+y)
					r += uint64(c.R)
					g += uint64(c.G)
					b += uint64(c.B)
					n++
				}
			}
			out.SetNRGBA(ox, oy, color.NRGBA{
				R: uint8((r + n/2) / n),
				G: uint8((g + n/2) / n),
				B: uint8((b + n/2) / n),
				A: 255,
			})
		}
	}
	return out
}

// CropAspect center-crops src so its width/height ratio equals aspect
// (w divided by h, > 0). It returns src unchanged when the image is already
// at or narrower than the requested ratio after rounding, so it never
// allocates for a no-op.
func CropAspect(src *image.NRGBA, aspect float64) *image.NRGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if aspect <= 0 {
		return src
	}
	nw, nh := w, h
	if float64(w)/float64(h) > aspect {
		nw = int(math.Round(float64(h) * aspect))
	} else if float64(w)/float64(h) < aspect {
		nh = int(math.Round(float64(w) / aspect))
	}
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	if nw >= w && nh >= h {
		return src
	}
	x0 := b.Min.X + (w-nw)/2
	y0 := b.Min.Y + (h-nh)/2
	out := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		rowStart := src.PixOffset(x0, y0+y)
		copy(out.Pix[y*out.Stride:], src.Pix[rowStart:rowStart+nw*4])
	}
	return out
}
