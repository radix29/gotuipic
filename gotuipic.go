package gotuipic

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"os"
	"strings"

	"github.com/radix29/gotuipic/internal/glyph"
	"github.com/radix29/gotuipic/internal/palette"
	"github.com/radix29/gotuipic/internal/raster"
)

// Mode selects the glyph style. Modes are rendered at runtime from the same
// pipeline; see ARCHITECTURE.md "Modes".
type Mode int

const (
	// ModeSextant: 2x3 subpixels, two colors per cell, U+1FB00 sextants.
	// The default and most detailed shipped mode.
	ModeSextant Mode = Mode(glyph.ModeSextant)
	// ModeASCII: one FG-colored ramp glyph per 1x2 block. Needs no
	// non-ASCII glyph at all, so it survives any terminal, font, or pipe.
	ModeASCII Mode = Mode(glyph.ModeASCII)
)

// String returns the mode's name.
func (m Mode) String() string {
	switch m {
	case ModeSextant:
		return "sextant"
	case ModeASCII:
		return "ascii"
	}
	return fmt.Sprintf("Mode(%d)", int(m))
}

// ColorProfile is the color depth of rendered output.
type ColorProfile int

const (
	// ColorTrueColor emits 24-bit SGR (38;2/48;2). The default.
	ColorTrueColor ColorProfile = iota
	// ColorANSI256 snaps colors to the xterm 256 palette.
	ColorANSI256
	// ColorANSI16 snaps colors to the 16 ANSI colors (a themed-terminal
	// approximation of the xterm defaults).
	ColorANSI16
	// ColorNone emits runes with no escape sequences at all.
	ColorNone
)

// FitMode decides what happens when both Width and Height are given.
type FitMode int

const (
	// FitContain fits the whole image inside the box, leaving the other
	// dimension smaller.
	FitContain FitMode = iota
	// FitStretch uses exactly Width x Height cells, distorting the image.
	FitStretch
	// FitCrop fills the whole box, center-cropping the image.
	FitCrop
)

// Errors. Wrap chains preserve errors.Is for all of them.
var (
	ErrInvalidOption = errors.New("gotuipic: invalid option")
	ErrTooLarge      = errors.New("gotuipic: image too large")
)

// Defaults.
const (
	// DefaultWidth is the column count used when neither Width nor Height
	// is set. It is a plain default, not a terminal query - the library
	// never inspects the environment.
	DefaultWidth = 80
	// DefaultRamp is the ASCII ramp, ordered dark to light.
	DefaultRamp = " .:-=+*#%@"
	// DefaultMaxPixels bounds decoded pixels (decompression-bomb guard).
	DefaultMaxPixels int64 = 64 << 20
	// DefaultMaxCells bounds the output grid (Cols*Rows).
	DefaultMaxCells int64 = 1 << 20
	// DefaultMaxBytes bounds how much of a stream ConvertReader buffers.
	DefaultMaxBytes int64 = 64 << 20
)

const defaultAspect = 0.5

// maxDim clamps a computed grid dimension so extreme aspects cannot
// overflow int; WithMaxCells rejects such sizes afterwards.
const maxDim = 1 << 30

var defaultBG = color.NRGBA{0, 0, 0, 255}

type options struct {
	width, height int
	fit           FitMode
	aspect        float64
	mode          Mode
	prof          ColorProfile
	ramp          string
	invert        bool
	bg            color.NRGBA
	maxPix        int64
	maxCells      int64
	maxBytes      int64
}

// Option customizes a conversion. Invalid values are reported together as
// ErrInvalidOption by the Convert functions, not at construction.
type Option func(*options)

// WithWidth sets the output width in columns. Negative or zero is an
// invalid option; when unset with WithHeight, DefaultWidth is used.
func WithWidth(n int) Option { return func(o *options) { o.width = n } }

// WithHeight sets the output height in rows. When unset with WithWidth, the
// height follows the image's aspect ratio.
func WithHeight(n int) Option { return func(o *options) { o.height = n } }

// WithFit chooses what happens when both width and height are given. The
// default is FitContain.
func WithFit(f FitMode) Option { return func(o *options) { o.fit = f } }

// WithMode selects the glyph style. The default is ModeSextant.
func WithMode(m Mode) Option { return func(o *options) { o.mode = m } }

// WithColor selects the color depth of the output. The default is
// ColorTrueColor.
func WithColor(p ColorProfile) Option { return func(o *options) { o.prof = p } }

// WithInvert reverses the ASCII ramp (light to dark). It affects ModeASCII
// only and does nothing for ModeSextant.
func WithInvert(on bool) Option { return func(o *options) { o.invert = on } }

// WithMaxPixels bounds the decoded image area; larger images fail with
// ErrTooLarge. It applies to every Convert function. Default
// DefaultMaxPixels.
func WithMaxPixels(n int64) Option { return func(o *options) { o.maxPix = n } }

// WithMaxCells bounds the output grid (columns times rows); a request for
// more fails with ErrInvalidOption. Default DefaultMaxCells.
func WithMaxCells(n int64) Option { return func(o *options) { o.maxCells = n } }

// WithMaxBytes bounds how many bytes ConvertReader, ConvertBytes and
// ConvertFile accept; longer input fails with ErrTooLarge. Default
// DefaultMaxBytes.
func WithMaxBytes(n int64) Option { return func(o *options) { o.maxBytes = n } }

// WithCellAspect sets the cell's width divided by its height, default 0.5
// (typical character cells). It controls how many rows the image gets, not
// the glyphs, so changing it is how you compensate for a font whose cells
// are fatter or thinner than 1:2.
func WithCellAspect(wDividedByH float64) Option {
	return func(o *options) { o.aspect = wDividedByH }
}

// WithRamp sets the ASCII glyph ramp, ordered dark to light, at least two
// runes. It has no effect on ModeSextant.
func WithRamp(s string) Option { return func(o *options) { o.ramp = s } }

// WithBackground composites transparent pixels over this color before
// rendering; the default is black. Its own alpha is ignored. A nil color
// keeps the default.
func WithBackground(c color.Color) Option {
	return func(o *options) {
		if c == nil {
			return
		}
		n, _ := color.NRGBAModel.Convert(c).(color.NRGBA)
		n.A = 255
		o.bg = n
	}
}

// resolve applies defaults and options and validates them.
func resolve(opts []Option) (options, error) {
	o := options{
		aspect: defaultAspect,
		mode:   ModeSextant,
		prof:   ColorTrueColor,
		ramp:   DefaultRamp,
		bg:     defaultBG,
		maxPix: DefaultMaxPixels,

		maxCells: DefaultMaxCells,
		maxBytes: DefaultMaxBytes,
	}
	for _, f := range opts {
		if f != nil {
			f(&o)
		}
	}
	var bad []string
	if o.width < 0 || o.height < 0 {
		bad = append(bad, "WithWidth/WithHeight must not be negative")
	}
	if o.aspect <= 0 || math.IsNaN(o.aspect) || math.IsInf(o.aspect, 0) {
		bad = append(bad, "WithCellAspect must be > 0")
	}
	if !glyph.Valid(glyph.Mode(o.mode)) {
		bad = append(bad, fmt.Sprintf("WithMode(%d) is not a defined mode", int(o.mode)))
	}
	if o.prof < ColorTrueColor || o.prof > ColorNone {
		bad = append(bad, fmt.Sprintf("WithColor(%d) is not a defined profile", int(o.prof)))
	}
	if o.fit < FitContain || o.fit > FitCrop {
		bad = append(bad, fmt.Sprintf("WithFit(%d) is not a defined fit mode", int(o.fit)))
	}
	if len([]rune(o.ramp)) < 2 {
		bad = append(bad, "WithRamp needs at least two runes")
	}
	if o.maxPix < 1 {
		bad = append(bad, "WithMaxPixels must be >= 1")
	}
	if o.maxCells < 1 {
		bad = append(bad, "WithMaxCells must be >= 1")
	}
	if o.maxBytes < 1 {
		bad = append(bad, "WithMaxBytes must be >= 1")
	}
	if len(bad) > 0 {
		return o, fmt.Errorf("%w: %s", ErrInvalidOption, strings.Join(bad, "; "))
	}
	return o, nil
}

// Convert renders an already-decoded image. An image with empty bounds or
// a nil image (including a typed-nil pointer) is ErrInvalidOption; one
// larger than MaxPixels is ErrTooLarge.
func Convert(img image.Image, opts ...Option) (*Art, error) {
	o, err := resolve(opts)
	if err != nil {
		return nil, err
	}
	return o.runImage(img)
}

// ConvertReader decodes r (image registry decides formats; import a decoder
// to add one) and renders it. The reader is read fully into memory, capped
// by WithMaxBytes, and MaxPixels is checked from the header before any
// pixel allocation.
func ConvertReader(r io.Reader, opts ...Option) (*Art, error) {
	o, err := resolve(opts)
	if err != nil {
		return nil, err
	}
	return o.runReader(r)
}

// ConvertBytes renders an image that is already whole in memory - an HTTP
// response body read into a buffer, a file read from an embed.FS, a
// base64-decoded blob - without the caller wrapping it in a reader. It is
// ConvertReader over a bytes.Reader: the slice is only read, never retained.
func ConvertBytes(b []byte, opts ...Option) (*Art, error) {
	return ConvertReader(bytes.NewReader(b), opts...)
}

// ConvertFile opens path and runs ConvertReader on it.
func ConvertFile(path string, opts ...Option) (*Art, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("gotuipic: open: %w", err)
	}
	defer f.Close()
	return ConvertReader(f, opts...)
}

func (o *options) runReader(r io.Reader) (*Art, error) {
	img, _, err := decode(r, o.maxPix, o.maxBytes)
	if err != nil {
		return nil, err
	}
	return o.runImage(img)
}

// runImage validates img's bounds, then runs the pipeline.
func (o *options) runImage(img image.Image) (art *Art, err error) {
	if img == nil {
		return nil, fmt.Errorf("%w: nil image", ErrInvalidOption)
	}
	defer func() {
		// A typed-nil image (e.g. (*image.NRGBA)(nil)) panics in Bounds.
		if recover() != nil {
			art, err = nil, fmt.Errorf("%w: invalid image", ErrInvalidOption)
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
	return o.run(img)
}

// run is the total pipeline: prepare, geometry, crop, resize, render,
// quantize. img must have non-empty bounds within maxPix.
func (o *options) run(img image.Image) (*Art, error) {
	b := img.Bounds()
	cols, rows := o.geom(b.Dx(), b.Dy())
	if int64(cols) > o.maxCells || int64(rows) > o.maxCells ||
		int64(cols)*int64(rows) > o.maxCells {
		return nil, fmt.Errorf("%w: output %dx%d cells is over WithMaxCells %d",
			ErrInvalidOption, cols, rows, o.maxCells)
	}
	prep := raster.Prepare(img, o.bg)
	pw, ph := glyph.Size(glyph.Mode(o.mode))
	src := prep
	if o.fit == FitCrop {
		src = raster.CropAspect(prep, float64(cols)*o.aspect/float64(rows))
	}
	grid := raster.BoxResize(src, cols*pw, rows*ph)
	ramp := []rune(o.ramp)
	if o.invert {
		for i, j := 0, len(ramp)-1; i < j; i, j = i+1, j-1 {
			ramp[i], ramp[j] = ramp[j], ramp[i]
		}
	}
	raw := glyph.Render(grid, glyph.Mode(o.mode), cols, rows, ramp, o.prof != ColorNone)
	return &Art{
		Cols:    cols,
		Rows:    rows,
		Cells:   o.quantize(raw),
		mode:    o.mode,
		profile: o.prof,
	}, nil
}

// geom computes the cell grid from the source size (pixels) and the
// options. Rows per column = imgH/imgW * aspect.
func (o *options) geom(w, h int) (cols, rows int) {
	ratio := float64(h) / float64(w) * o.aspect
	rnd := func(x float64) int {
		if !(x >= 1) { // also catches NaN
			return 1
		}
		if x > maxDim {
			return maxDim
		}
		return int(math.Round(x))
	}
	switch {
	case o.width > 0 && o.height > 0:
		if o.fit != FitContain { // stretch and crop fill the requested box
			return o.width, o.height
		}
		cols, rows = o.width, rnd(float64(o.width)*ratio)
		if rows > o.height {
			rows = o.height
			cols = rnd(float64(o.height) / ratio)
		}
		if cols > o.width {
			cols = o.width
		}
	case o.width > 0:
		cols, rows = o.width, rnd(float64(o.width)*ratio)
	case o.height > 0:
		cols, rows = rnd(float64(o.height)/ratio), o.height
	default:
		cols, rows = DefaultWidth, rnd(DefaultWidth*ratio)
	}
	return cols, rows
}

// quantize maps raw cell colors through the active color profile.
func (o *options) quantize(raw []glyph.Cell) []Cell {
	out := make([]Cell, len(raw))
	for i, c := range raw {
		cell := Cell{Rune: c.Rune}
		switch o.prof {
		case ColorTrueColor:
			cell.FG = color.RGBA{R: c.FG.R, G: c.FG.G, B: c.FG.B, A: 255}
			if c.BG.A != 0 {
				cell.BG = color.RGBA{R: c.BG.R, G: c.BG.G, B: c.BG.B, A: 255}
			}
		case ColorANSI256:
			cell.FG = palette.Table256[palette.Nearest256(c.FG.R, c.FG.G, c.FG.B)]
			if c.BG.A != 0 {
				cell.BG = palette.Table256[palette.Nearest256(c.BG.R, c.BG.G, c.BG.B)]
			}
		case ColorANSI16:
			cell.FG = palette.Table16[palette.Nearest16(c.FG.R, c.FG.G, c.FG.B)]
			if c.BG.A != 0 {
				cell.BG = palette.Table16[palette.Nearest16(c.BG.R, c.BG.G, c.BG.B)]
			}
		default: // ColorNone: colors are not emitted; leave them zero.
		}
		// Two colors that quantize to the same one make the glyph
		// invisible; a space is cheaper and survives fonts without sextants.
		if cell.BG.A != 0 && cell.FG == cell.BG {
			cell.Rune = ' '
		}
		out[i] = cell
	}
	return out
}
