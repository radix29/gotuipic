// Package gotuipic converts images (PNG, JPEG, GIF out of the box; more via
// the image registry) into colored UTF-8 text art for terminals.
//
// The pipeline: decode → alpha-composite → box-resize → map pixel blocks to
// glyphs → quantize colors → encode as ANSI (or read the cell grid
// directly). Two modes ship:
//
//   - [ModeSextant] (default): two colors in six subpixels per cell using
//     U+1FB00 block sextants, split by an exact minimum-error partition.
//   - [ModeASCII]: one FG-colored ramp glyph per cell, chosen so the cell's
//     own color and the glyph's coverage hit the target luminance; it needs
//     no non-ASCII glyph, so it works everywhere.
//
// The library never reads the environment, queries the terminal, or writes
// anywhere unasked. The caller decides width and color depth; [Art] is the
// product, and [Art.String] is one rendering of it.
//
//	art, err := gotuipic.ConvertFile("cat.jpg", gotuipic.WithWidth(80))
//	if err != nil {
//		return err
//	}
//	fmt.Print(art)
package gotuipic
