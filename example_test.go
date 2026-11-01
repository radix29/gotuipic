package gotuipic_test

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log"
	"os"
	"strings"

	"github.com/radix29/gotuipic"
)

// gradient returns an 8x8 vertical black-to-white ramp, the fixture these
// examples convert. ColorNone keeps every // Output: deterministic - the
// glyphs carry the picture on their own, with no escape sequences in it.
func gradient() image.Image {
	im := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		v := uint8(y * 255 / 7)
		for x := 0; x < 8; x++ {
			im.SetNRGBA(x, y, color.NRGBA{v, v, v, 255})
		}
	}
	return im
}

func pngBytes(im image.Image) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		panic(err)
	}
	return b.Bytes()
}

func ExampleConvert() {
	// An image.Image you already have: one you decoded yourself, drew in
	// code, or got from a library gotuipic knows nothing about.
	art, err := gotuipic.Convert(gradient(),
		gotuipic.WithWidth(8),
		gotuipic.WithMode(gotuipic.ModeASCII),
		gotuipic.WithColor(gotuipic.ColorNone))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(art)
	// Output:
	// ........
	// --------
	// ********
	// %%%%%%%%
}

func ExampleConvertReader() {
	// A stream: a file, os.Stdin, a socket, an HTTP response body, a
	// gzip.Reader, a fs.File from an embed.FS. Anything io.Reader works,
	// and the format is sniffed from the header, not the file name. Here
	// the writer produces PNG bytes on the other end of a pipe.
	r, w := io.Pipe()
	go func() {
		_ = png.Encode(w, gradient())
		w.Close()
	}()

	art, err := gotuipic.ConvertReader(r, gotuipic.WithWidth(8),
		gotuipic.WithMode(gotuipic.ModeASCII), gotuipic.WithColor(gotuipic.ColorNone))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(art)
	// Output:
	// ........
	// --------
	// ********
	// %%%%%%%%
}

func ExampleConvertBytes() {
	// Bytes already whole in memory - a response body you read into a
	// buffer, an embed.FS file, a base64-decoded blob. The slice is only
	// read, never kept.
	data := pngBytes(gradient())

	art, err := gotuipic.ConvertBytes(data, gotuipic.WithWidth(8),
		gotuipic.WithMode(gotuipic.ModeASCII), gotuipic.WithColor(gotuipic.ColorNone))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(art)
	// Output:
	// ........
	// --------
	// ********
	// %%%%%%%%
}

// ExampleConvertFile shows the convenience path for a named file. The output
// is unchecked because it is a real photograph; the repo's example photos
// live under tools/stylepreview/testdata (see the README gallery).
func ExampleConvertFile() {
	art, err := gotuipic.ConvertFile("tools/stylepreview/testdata/sample.jpg",
		gotuipic.WithWidth(32),
		gotuipic.WithMode(gotuipic.ModeASCII),
		gotuipic.WithColor(gotuipic.ColorNone))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(art.Cols, art.Rows)
	fmt.Print(art.Plain())
}

func ExampleArt_WriteTo() {
	// Art writes itself to any io.Writer - a file, os.Stdout, a net.Conn,
	// an http.ResponseWriter, a log. Use Art.Plain for runes only.
	art, err := gotuipic.Convert(gradient(), gotuipic.WithWidth(8),
		gotuipic.WithMode(gotuipic.ModeASCII), gotuipic.WithColor(gotuipic.ColorNone))
	if err != nil {
		log.Fatal(err)
	}
	if _, err := art.WriteTo(os.Stdout); err != nil {
		log.Fatal(err)
	}
	// Output:
	// ........
	// --------
	// ********
	// %%%%%%%%
}

func ExampleArt_Cells() {
	// A TUI reads the cell grid instead of parsing escape sequences back
	// out of a string. Cells are row-major, Cols wide, and every rune is
	// single-width, so column == index in the line.
	art, err := gotuipic.Convert(gradient(), gotuipic.WithWidth(8),
		gotuipic.WithColor(gotuipic.ColorANSI256))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(art.Cols, art.Rows, len(art.Cells))
	fmt.Printf("%q fg=%v bg=%v\n", art.At(0, 0).Rune, art.At(0, 0).FG, art.At(0, 0).BG)
	fmt.Println(strings.Count(art.String(), "\n") == art.Rows)
	// Output:
	// 8 4 32
	// '🬭' fg={38 38 38 255} bg={0 0 0 255}
	// true
}
