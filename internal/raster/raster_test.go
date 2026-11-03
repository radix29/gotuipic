package raster

import (
	"image"
	"image/color"
	"testing"
)

func solid(w, h int, c color.NRGBA) *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			im.SetNRGBA(x, y, c)
		}
	}
	return im
}

func near(c, want color.NRGBA) bool {
	d := func(a, b uint8) bool {
		if a > b {
			return a-b <= 1
		}
		return b-a <= 1
	}
	return d(c.R, want.R) && d(c.G, want.G) && d(c.B, want.B)
}

func TestPrepareCompositesOverBackground(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	src.SetNRGBA(0, 0, color.NRGBA{R: 0, G: 255, B: 0, A: 128})
	out := Prepare(src, color.NRGBA{R: 255, G: 0, B: 0, A: 255})
	got := out.NRGBAAt(0, 0)
	if !near(got, color.NRGBA{R: 127, G: 128, B: 0, A: 255}) {
		t.Fatalf("blend = %v, want ~{127,128,0,255}", got)
	}
}

func TestPrepareMovesSubRectToOrigin(t *testing.T) {
	src := image.NewNRGBA(image.Rect(3, 4, 5, 6))
	src.SetNRGBA(3, 4, color.NRGBA{R: 7, G: 7, B: 7, A: 255})
	out := Prepare(src, color.NRGBA{})
	if b := out.Bounds(); b.Dx() != 2 || b.Dy() != 2 || b.Min != (image.Point{}) {
		t.Fatalf("bounds = %v, want 2x2 at origin", b)
	}
	if got := out.NRGBAAt(0, 0); got != (color.NRGBA{7, 7, 7, 255}) {
		t.Fatalf("pixel = %v", got)
	}
}

func TestBoxResizeSolidStaysSolid(t *testing.T) {
	red := color.NRGBA{200, 10, 10, 255}
	out := BoxResize(solid(10, 8, red), 3, 3)
	for y := 0; y < 3; y++ {
		for x := 0; x < 3; x++ {
			if got := out.NRGBAAt(x, y); !near(got, red) {
				t.Fatalf("(%d,%d) = %v, want ~%v", x, y, got, red)
			}
		}
	}
}

func TestBoxResizeExactAverage(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	src.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 255})
	src.SetNRGBA(1, 0, color.NRGBA{0, 0, 0, 255})
	src.SetNRGBA(0, 1, color.NRGBA{255, 0, 0, 255})
	src.SetNRGBA(1, 1, color.NRGBA{0, 0, 0, 255})
	got := BoxResize(src, 1, 1).NRGBAAt(0, 0)
	if !near(got, color.NRGBA{128, 0, 0, 255}) {
		t.Fatalf("average = %v, want ~{128,0,0,255}", got)
	}
}

func TestCropAspectCenters(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 100, 50))
	green := color.NRGBA{0, 255, 0, 255}
	for y := 0; y < 50; y++ {
		for x := 25; x < 75; x++ {
			src.SetNRGBA(x, y, green)
		}
	}
	out := CropAspect(src, 1.0)
	if b := out.Bounds(); b.Dx() != 50 || b.Dy() != 50 {
		t.Fatalf("cropped %v, want 50x50", b.Size())
	}
	if out.NRGBAAt(0, 0) != green || out.NRGBAAt(49, 49) != green {
		t.Fatal("crop did not keep the centered green band")
	}
	if wide := CropAspect(src, 2.0); wide != src {
		t.Fatal("no-op crop must return the input image")
	}
}

func TestCropAspectNonOriginSource(t *testing.T) {
	src := image.NewNRGBA(image.Rect(10, 20, 14, 22)) // 4x2, not at the origin
	for y := 20; y < 22; y++ {
		for x := 10; x < 14; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), A: 255})
		}
	}
	out := CropAspect(src, 1) // 4x2 -> 2x2, columns 11 and 12
	if got := out.NRGBAAt(0, 0); got.R != 11 || got.G != 20 {
		t.Fatalf("top-left = %+v, want R=11 G=20", got)
	}
}
