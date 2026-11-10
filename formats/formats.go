// Package formats registers the WebP, BMP and TIFF decoders from
// golang.org/x/image with the standard image registry. Import it for its
// side effect:
//
//	import _ "github.com/radix29/gotuipic/formats"
//
// after which [gotuipic.ConvertReader], [gotuipic.ConvertBytes],
// [gotuipic.ConvertFile] and [gotuipic.Decode] accept those formats too,
// sniffed from the stream header like the core's PNG, JPEG and GIF.
//
// It is a separate package so the core stays standard-library-only: a
// program that never imports it never links golang.org/x/image.
package formats

import (
	_ "golang.org/x/image/bmp"  // "bmp"
	_ "golang.org/x/image/tiff" // "tiff"
	_ "golang.org/x/image/webp" // "webp"
)

// Names lists the format names this package registers, as image.Decode
// reports them.
var Names = []string{"bmp", "tiff", "webp"}
