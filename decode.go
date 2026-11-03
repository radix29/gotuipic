package gotuipic

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif" // the three formats the core always supports
	_ "image/jpeg"
	_ "image/png"
	"io"
)

// decode reads the full stream, checks dimensions from the header before
// allocating pixels, then decodes. Supported formats are whatever the
// image registry holds; the core pre-registers PNG, JPEG and GIF, and the
// opt-in formats package adds WebP/BMP/TIFF (a later phase).
func decode(r io.Reader, maxPixels, maxBytes int64) (image.Image, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("gotuipic: read: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("gotuipic: input is over MaxBytes %d: %w", maxBytes, ErrTooLarge)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gotuipic: decode config: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, fmt.Errorf("gotuipic: invalid image dimensions %dx%d: %w",
			cfg.Width, cfg.Height, image.ErrFormat)
	}
	if n := int64(cfg.Width) * int64(cfg.Height); n > maxPixels {
		return nil, fmt.Errorf("gotuipic: image is %d pixels, over MaxPixels %d: %w",
			n, maxPixels, ErrTooLarge)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gotuipic: decode: %w", err)
	}
	return img, nil
}
