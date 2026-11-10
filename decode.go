package gotuipic

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif" // the three formats the core always supports
	_ "image/jpeg"
	_ "image/png"
	"io"

	"github.com/radix29/gotuipic/internal/raster"
)

// Decode reads r fully and decodes it with the same guards the Convert
// functions use - WithMaxBytes on the input, WithMaxPixels checked from the
// header before any pixel is allocated - and returns the image with its
// registered format name ("png", "jpeg", ...). Only those two options apply;
// the rest are validated but ignored.
//
// It is for a caller that renders the same image more than once, at several
// sizes or in both modes: decode once, then call [Convert] per rendering,
// which skips the decode that dominates ConvertReader's cost.
func Decode(r io.Reader, opts ...Option) (image.Image, string, error) {
	o, err := resolve(opts)
	if err != nil {
		return nil, "", err
	}
	return decode(r, o.maxPix, o.maxBytes)
}

// DecodeBytes is [Decode] over a byte slice, which is only read, never
// retained.
func DecodeBytes(b []byte, opts ...Option) (image.Image, string, error) {
	return Decode(bytes.NewReader(b), opts...)
}

// decode reads the full stream, checks dimensions from the header before
// allocating pixels, then decodes. Supported formats are whatever the
// image registry holds; the core pre-registers PNG, JPEG and GIF, and the
// opt-in formats package adds WebP/BMP/TIFF.
func decode(r io.Reader, maxPixels, maxBytes int64) (image.Image, string, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("gotuipic: read: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, "", fmt.Errorf("gotuipic: input is over MaxBytes %d: %w", maxBytes, ErrTooLarge)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("gotuipic: decode config: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, "", fmt.Errorf("gotuipic: invalid image dimensions %dx%d: %w",
			cfg.Width, cfg.Height, image.ErrFormat)
	}
	if n := int64(cfg.Width) * int64(cfg.Height); n > maxPixels {
		return nil, "", fmt.Errorf("gotuipic: image is %d pixels, over MaxPixels %d: %w",
			n, maxPixels, ErrTooLarge)
	}
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("gotuipic: decode: %w", err)
	}
	return img, format, nil
}

// Flatten composites img over the WithBackground color (black by default)
// into an opaque *image.NRGBA with its top-left at the origin - the form the
// pipeline's first stage produces. [Convert] passes such an image through
// without converting a pixel, so a program rendering one picture many times
// calls Decode, then Flatten once, then Convert per rendering, and pays only
// the resize and glyph stages each time. Only WithBackground and
// WithMaxPixels apply; the rest are validated but ignored.
func Flatten(img image.Image, opts ...Option) (out *image.NRGBA, err error) {
	o, err := resolve(opts)
	if err != nil {
		return nil, err
	}
	if img == nil {
		return nil, fmt.Errorf("%w: nil image", ErrInvalidOption)
	}
	defer func() {
		// A typed-nil image panics in Bounds, as in Convert.
		if recover() != nil {
			out, err = nil, fmt.Errorf("%w: invalid image", ErrInvalidOption)
		}
	}()
	b := img.Bounds()
	w, h := int64(b.Dx()), int64(b.Dy())
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("%w: image has empty bounds", ErrInvalidOption)
	}
	if w > o.maxPix || h > o.maxPix || w*h > o.maxPix {
		return nil, fmt.Errorf("gotuipic: image is %dx%d, over MaxPixels %d: %w",
			w, h, o.maxPix, ErrTooLarge)
	}
	return raster.Prepare(img, o.bg), nil
}
