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
type Mode = glyph.Mode

const (
	// ModeSextant: 2x3 subpixels, two colors per cell, U+1FB00 sextants.
	// The default and most detailed shipped mode.
	ModeSextant = glyph.ModeSextant
	// ModeASCII: one FG-colored ramp glyph per 1x2 block. Needs no
	// non-ASCII glyph at all, so it survives any terminal, font, or pipe.
	ModeASCII = glyph.ModeASCII
)

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
)

const defaultAspect = 0.5

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
}

// Option customizes a conversion. Invalid values are reported together as
// ErrInvalidOption by the Convert functions, not at construction.
type Option func(*options)

// WithWidth sets the output width in columns. Negative or zero is an
// invalid option; when unset with WithHeight, DefaultWidth is used.
func WithWidth(n int) Option          { return func(o *options) { o.width = n } }
func WithHeight(n int) Option         { return func(o *options) { o.height = n } }
func WithFit(f FitMode) Option        { return func(o *options) { o.fit = f } }
func WithMode(m Mode) Option          { return func(o *options) { o.mode = m } }
func WithColor(p ColorProfile) Option { return func(o *options) { o.prof = p } }
func WithInvert(on bool) Option       { return func(o *options) { o.invert = on } }
func WithMaxPixels(n int64) Option    { return func(o *options) { o.maxPix = n } }

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
// rendering; the default is black. Its own alpha is ignored.
func WithBackground(c color.Color) Option {
	return func(o *options) {
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
	if !glyph.Valid(o.mode) {
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
	if len(bad) > 0 {
		return o, fmt.Errorf("%w: %s", ErrInvalidOption, strings.Join(bad, "; "))
	}
	return o, nil
}

// Convert renders an already-decoded image.
func Convert(img image.Image, opts ...Option) (*Art, error) {
	o, err := resolve(opts)
	if err != nil {
		return nil, err
	}
	if img == nil {
		return nil, fmt.Errorf("%w: nil image", ErrInvalidOption)
	}
	return o.run(img), nil
}

// ConvertReader decodes r (image registry decides formats; import a decoder
// to add one) and renders it. The reader is consumed fully. MaxPixels is
// checked from the header before any pixel allocation.
func ConvertReader(r io.Reader, opts ...Option) (*Art, error) {
	o, err := resolve(opts)
	if err != nil {
		return nil, err
	}
	img, err := decode(r, o.maxPix)
	if err != nil {
		return nil, err
	}
	return o.run(img), nil
}

// ConvertBytes renders an image that is already whole in memory - an HTTP
// response body read into a buffer, a file read from an embed.FS, a
// base64-decoded blob - without the caller wrapping it in a reader. It is
// ConvertReader over a bytes.Reader: the slice is only read, never retained.
func ConvertBytes(b []byte, opts ...Option) (*Art, error) {
	o, err := resolve(opts)
	if err != nil {
		return nil, err
	}
	img, err := decode(bytes.NewReader(b), o.maxPix)
	if err != nil {
		return nil, err
	}
	return o.run(img), nil
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

// run is the total pipeline: prepare, geometry, crop, resize, render,
// quantize. It cannot fail on validated options.
func (o *options) run(img image.Image) *Art {
	prep := raster.Prepare(img, o.bg)
	w, h := prep.Bounds().Dx(), prep.Bounds().Dy()
	cols, rows := 1, 1
	if w > 0 && h > 0 {
		cols, rows = o.geom(w, h)
	}
	pw, ph := glyph.Size(o.mode)
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
	raw := glyph.Render(grid, o.mode, cols, rows, ramp, o.prof != ColorNone)
	return &Art{
		Cols:    cols,
		Rows:    rows,
		Cells:   o.quantize(raw),
		mode:    o.mode,
		profile: o.prof,
	}
}

// geom computes the cell grid from the source size (pixels) and the
// options. Rows per column = imgH/imgW * aspect.
func (o *options) geom(w, h int) (cols, rows int) {
	ratio := float64(h) / float64(w) * o.aspect
	rnd := func(x float64) int {
		r := int(math.Round(x))
		if r < 1 {
			r = 1
		}
		return r
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
		out[i] = cell
	}
	return out
}
