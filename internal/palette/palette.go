// Package palette owns color math: luminance, Oklab distance, and the xterm
// 16/256 palettes with nearest-color search. It is the only place that
// knows what a terminal palette looks like.
package palette

import (
	"image/color"
	"math"
)

// Luma is the Rec. 709 luma of an sRGB color, on a 0-255 scale. It is what
// the ASCII ramp picker matches against.
func Luma(r, g, b uint8) float64 {
	return 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
}

// toLinear converts an sRGB byte channel to linear light.
var toLinear [256]float64

func init() {
	for i := range toLinear {
		c := float64(i) / 255
		if c <= 0.04045 {
			toLinear[i] = c / 12.92
		} else {
			toLinear[i] = math.Pow((c+0.055)/1.055, 2.4)
		}
	}
}

// lab is the Oklab of an sRGB byte triple. Nearest-color search runs in
// Oklab because Euclidean distance in RGB tracks neither lightness nor hue
// the way the eye does.
func lab(r, g, b uint8) (l, a, bb float64) {
	rl, gl, bl := toLinear[r], toLinear[g], toLinear[b]
	l_ := math.Cbrt(0.4122214708*rl + 0.5363325363*gl + 0.0514459929*bl)
	m_ := math.Cbrt(0.2119034982*rl + 0.6806995451*gl + 0.1073969566*bl)
	s_ := math.Cbrt(0.0883024619*rl + 0.2817188376*gl + 0.6299787005*bl)
	return 0.2104542553*l_ + 0.7936177850*m_ - 0.0040720468*s_,
		1.9779984951*l_ - 2.4285922051*m_ + 0.4505937099*s_,
		0.0259040371*l_ + 0.7827717662*m_ - 0.8086757660*s_
}

// Table16 is the xterm default system palette. Terminals let users theme
// these, so any use is an approximation; that is documented.
var Table16 = [16]color.RGBA{
	{0, 0, 0, 255}, {205, 0, 0, 255}, {0, 205, 0, 255}, {205, 205, 0, 255},
	{0, 0, 238, 255}, {205, 0, 205, 255}, {0, 205, 205, 255}, {229, 229, 229, 255},
	{127, 127, 127, 255}, {255, 0, 0, 255}, {0, 255, 0, 255}, {255, 255, 0, 255},
	{92, 92, 255, 255}, {255, 0, 255, 255}, {0, 255, 255, 255}, {255, 255, 255, 255},
}

// Table256 is the xterm 256-color palette: the 16 colors above, then a
// 6x6x6 cube with levels 0,95,135,175,215,255, then 24 grays 8+10i.
var Table256 [256]color.RGBA

var (
	lab16   [16][3]float64
	lab256  [256][3]float64
	cubelev = [6]uint8{0, 95, 135, 175, 215, 255}
)

func init() {
	Table256 = [256]color.RGBA{}
	copy(Table256[:16], Table16[:])
	for i := 0; i < 216; i++ {
		r, g, b := cubelev[i/36], cubelev[(i/6)%6], cubelev[i%6]
		Table256[16+i] = color.RGBA{r, g, b, 255}
	}
	for i := 0; i < 24; i++ {
		v := uint8(8 + 10*i)
		Table256[232+i] = color.RGBA{v, v, v, 255}
	}
	for i, c := range Table16 {
		lab16[i][0], lab16[i][1], lab16[i][2] = lab(c.R, c.G, c.B)
	}
	for i, c := range Table256 {
		lab256[i][0], lab256[i][1], lab256[i][2] = lab(c.R, c.G, c.B)
	}
}

// Nearest16 returns the palette index whose Oklab distance to the color is
// smallest.
func Nearest16(r, g, b uint8) int {
	l, a, bb := lab(r, g, b)
	best, bi := math.MaxFloat64, 0
	for i, p := range lab16 {
		d := d2(l, a, bb, p)
		if d < best {
			best, bi = d, i
		}
	}
	return bi
}

// Nearest256 returns the palette index whose Oklab distance to the color is
// smallest, searching both the cube and the gray ramp.
func Nearest256(r, g, b uint8) int {
	l, a, bb := lab(r, g, b)
	best, bi := math.MaxFloat64, 0
	for i, p := range lab256 {
		d := d2(l, a, bb, p)
		if d < best {
			best, bi = d, i
		}
	}
	return bi
}

func d2(l, a, b float64, p [3]float64) float64 {
	dl, da, db := l-p[0], a-p[1], b-p[2]
	return dl*dl + da*da + db*db
}
