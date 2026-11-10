package formats_test

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"

	"github.com/radix29/gotuipic"
	_ "github.com/radix29/gotuipic/formats"
)

func sample() image.Image {
	im := image.NewNRGBA(image.Rect(0, 0, 16, 12))
	for y := range 12 {
		for x := range 16 {
			im.SetNRGBA(x, y, color.NRGBA{uint8(x * 16), uint8(y * 20), 128, 255})
		}
	}
	return im
}

// TestRegistered checks that BMP and TIFF bytes decode through the core's
// entry points once the package is imported. (WebP has no encoder in
// x/image; its decoder is registered the same way.)
func TestRegistered(t *testing.T) {
	for _, tc := range []struct {
		name string
		enc  func(*bytes.Buffer, image.Image) error
	}{
		{"bmp", func(b *bytes.Buffer, m image.Image) error { return bmp.Encode(b, m) }},
		{"tiff", func(b *bytes.Buffer, m image.Image) error { return tiff.Encode(b, m, nil) }},
	} {
		var buf bytes.Buffer
		if err := tc.enc(&buf, sample()); err != nil {
			t.Fatalf("%s encode: %v", tc.name, err)
		}
		img, format, err := gotuipic.DecodeBytes(buf.Bytes())
		if err != nil {
			t.Fatalf("%s decode: %v", tc.name, err)
		}
		if format != tc.name || img.Bounds().Dx() != 16 {
			t.Fatalf("%s: got format %q bounds %v", tc.name, format, img.Bounds())
		}
		art, err := gotuipic.ConvertBytes(buf.Bytes(), gotuipic.WithWidth(8))
		if err != nil || art.Cols != 8 {
			t.Fatalf("%s convert: %v %+v", tc.name, err, art)
		}
	}
}
