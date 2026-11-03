package gotuipic

import (
	"bytes"
	"image/png"
	"testing"
)

func FuzzConvertReader(f *testing.F) {
	var buf bytes.Buffer
	_ = png.Encode(&buf, halfRedBlue(6, 6))
	f.Add(buf.Bytes())
	f.Add([]byte("not an image"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		art, err := ConvertBytes(data, WithWidth(8), WithMaxPixels(1<<20))
		if err != nil {
			return
		}
		if len(art.Cells) != art.Cols*art.Rows {
			t.Fatalf("cells %d != %dx%d", len(art.Cells), art.Cols, art.Rows)
		}
		_ = art.String()
	})
}
