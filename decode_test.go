package gotuipic

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"slices"
	"testing"
)

// translucent is an image the generic Prepare path has to composite: a
// gradient with varying alpha, offset from the origin.
func translucent() image.Image {
	im := image.NewNRGBA(image.Rect(3, 5, 3+40, 5+30))
	for y := 5; y < 35; y++ {
		for x := 3; x < 43; x++ {
			im.SetNRGBA(x, y, color.NRGBA{uint8(x * 6), uint8(y * 7), 90, uint8(x*5 + y)})
		}
	}
	return im
}

// TestFlattenThenConvertMatchesConvert pins that the decode-once path gives
// exactly the art Convert gives on the original image: Flatten's output must
// take Prepare's pass-through, and the pass-through must change nothing.
func TestFlattenThenConvertMatchesConvert(t *testing.T) {
	bg := color.NRGBA{20, 40, 60, 255}
	for _, m := range []Mode{ModeSextant, ModeASCII} {
		opts := []Option{WithWidth(17), WithMode(m), WithBackground(bg)}
		want, err := Convert(translucent(), opts...)
		if err != nil {
			t.Fatal(err)
		}
		flat, err := Flatten(translucent(), WithBackground(bg))
		if err != nil {
			t.Fatal(err)
		}
		if flat.Rect.Min != (image.Point{}) || !flat.Opaque() {
			t.Fatalf("Flatten gave %v, opaque=%v", flat.Rect, flat.Opaque())
		}
		got, err := Convert(flat, opts...)
		if err != nil {
			t.Fatal(err)
		}
		if got.Cols != want.Cols || got.Rows != want.Rows || !slices.Equal(got.Cells, want.Cells) {
			t.Fatalf("%v: flattened art differs from direct art", m)
		}
	}
}

func TestDecodeBytesReportsFormatAndGuards(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, translucent()); err != nil {
		t.Fatal(err)
	}
	img, format, err := DecodeBytes(buf.Bytes())
	if err != nil || format != "png" || img.Bounds().Dx() != 40 {
		t.Fatalf("DecodeBytes = %v, %q, %v", img.Bounds(), format, err)
	}
	if _, _, err := DecodeBytes(buf.Bytes(), WithMaxPixels(100)); err == nil {
		t.Fatal("DecodeBytes ignored WithMaxPixels")
	}
	if _, _, err := DecodeBytes([]byte("not an image")); err == nil {
		t.Fatal("DecodeBytes accepted garbage")
	}
}
