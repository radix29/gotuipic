// Command stylepreview renders the example images in every ASCII and
// sextant variant so the two shipping modes can be tuned by eye and by
// metric (-rmse prints sRGB RMSE and Oklab dE).
//
// Default mode writes one PNG montage per example image (black terminal
// background, glyph geometry drawn exactly). With -ansi it instead prints
// real ANSI truecolor text for one style to stdout, so you can check how
// your own terminal font renders it:
//
//	go run ./tools/stylepreview -ansi sextantx | less -R
//	go run ./tools/stylepreview                    # writes preview-*.png here
//	go run ./tools/stylepreview -rmse              # scores all styles
//
// It is a throwaway design aid, not part of the library. It lives in its own
// module so the library core stays standard-library-only and unbuildable-by-accident code never
// depends on it.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"io"
	"math"
	"math/bits"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// ----- terminal cell geometry (px on the PNG canvas, 1:2 w:h ratio) -----

const (
	cellW   = 8
	cellH   = 16
	pad     = 16
	labelH  = 30
	gap     = 14
	refMaxH = 180
)

type kind int

const (
	kindAscii  kind = iota
	kindBlocks      // per-subpixel fg/bg mask, row-major bit order
)

type algo int

const (
	algo2means    algo = iota
	algoExact          // exhaustive minimum-error split (see exactSplit)
	algoSmartChar      // ascii: ramp pick weighted by FG luminance
	algoASCIIED        // ascii: smart pick + Floyd-Steinberg on luma
	algoLinExact       // sextant: exact split in linear-light RGB
	algoLinED          // sextant: linear light + Floyd-Steinberg across cells
	algoLinRes         // linear-light resize, then exact/smart pick
)

type style struct {
	id   string
	name string
	desc string
	pw   int // subpixels sampled per cell column
	ph   int // subpixels sampled per cell row
	kind kind
	algo algo
}

var styles = []style{
	{"ascii", "1  ASCII RAMP 1x2", "BASELINE - LUMA-INDEX PICK - FG ONLY", 1, 2, kindAscii, algo2means},
	{"asciiw", "2  ASCII WEIGHTED 1x2", "PICK WEIGHTED BY FG LUMINANCE", 1, 2, kindAscii, algoSmartChar},
	{"asciied", "3  ASCII ED 1x2", "WEIGHTED + FLOYD-STEINBERG ON LUMA", 1, 2, kindAscii, algoASCIIED},
	{"asciiwl", "4  ASCII WEIGHTED+LIN", "SMART PICK + LINEAR-LIGHT RESIZE", 1, 2, kindAscii, algoLinRes},
	{"sextant", "5  SEXTANT 2x3", "2-MEANS SPLIT - BASELINE", 2, 3, kindBlocks, algo2means},
	{"sextantx", "6  SEXTANT EXACT 2x3", "ALL 63 SPLITS - SHIPPED PICK", 2, 3, kindBlocks, algoExact},
	{"sextantp", "7  SEXTANT LIGHT 2x3", "EXACT IN LINEAR-LIGHT RGB", 2, 3, kindBlocks, algoLinExact},
	{"sextanted", "8  SEXTANT ED 2x3", "LIGHT EXACT + FLOYD-STEINBERG", 2, 3, kindBlocks, algoLinED},
	{"sextantlr", "9  SEXTANT LINRESIZE", "LINEAR-LIGHT RESIZE + EXACT", 2, 3, kindBlocks, algoLinRes},
}

const ramp = " .:-=+*#%@"

// rampCover approximates what the terminal shows for one ascii cell: the FG
// color attenuated by the glyph's ink coverage, taken as the ramp position -
// the classic ASCII-art assumption, shared by the pickers and the scorer.
func rampCover(ch rune) float64 {
	i := strings.IndexRune(ramp, ch)
	if i <= 0 {
		return 0
	}
	return float64(i) / float64(len(ramp)-1)
}

func scaleColor(c color.NRGBA, k float64) color.NRGBA {
	return color.NRGBA{
		R: uint8(float64(c.R)*k + 0.5),
		G: uint8(float64(c.G)*k + 0.5),
		B: uint8(float64(c.B)*k + 0.5),
		A: 255,
	}
}

// pickRuneIdx returns the ramp index whose output luminance (FG luminance x
// ramp position) lands closest on the target luminance t. The baseline
// picker ignores the FG term and just maps t to a ramp position, which
// systematically renders too dark whenever the block color is not white.
func pickRuneIdx(t, lfg float64) int {
	gain := lfg / 255
	if gain < 0.05 {
		return len(ramp) - 1 // a near-black FG makes no light either way
	}
	i := int(t/255/gain*float64(len(ramp)-1) + 0.5)
	if i < 0 {
		i = 0
	}
	if i > len(ramp)-1 {
		i = len(ramp) - 1
	}
	return i
}

// renderASCIIED is asciiw plus Floyd-Steinberg error diffusion on the luma
// channel - glyph choice is the only knob an ascii cell has, so diffuse its
// error - serpentine over cells. Color stays the plain block mean.
func renderASCIIED(resized *image.NRGBA, st style, cols, rows int) []cell {
	sub := make([]color.NRGBA, st.pw*st.ph)
	er := make([]float64, cols*rows)
	cells := make([]cell, cols*rows)
	for cy := 0; cy < rows; cy++ {
		d := 1
		if cy%2 == 1 {
			d = -1
		}
		for i := 0; i < cols; i++ {
			cx := i
			if d < 0 {
				cx = cols - 1 - i
			}
			for sy := 0; sy < st.ph; sy++ {
				for sx := 0; sx < st.pw; sx++ {
					sub[sy*st.pw+sx] = resized.NRGBAAt(cx*st.pw+sx, cy*st.ph+sy)
				}
			}
			col := meanColor(sub)
			t := meanLum(sub) + er[cy*cols+cx]
			lfg := lum(col)
			bi := pickRuneIdx(t, lfg)
			lout := lfg / 255 * float64(bi) / float64(len(ramp)-1) * 255
			e := t - lout
			spread := func(tcx, tcy int, w float64) {
				if tcx < 0 || tcx >= cols || tcy < 0 || tcy >= rows {
					return
				}
				er[tcy*cols+tcx] += w * e
			}
			spread(cx+d, cy, 7.0/16)
			spread(cx-d, cy+1, 3.0/16)
			spread(cx, cy+1, 5.0/16)
			spread(cx+d, cy+1, 1.0/16)
			cells[cy*cols+cx] = cell{pw: st.pw, ph: st.ph, kind: kindAscii, fg: col, ch: rune(ramp[bi])}
		}
	}
	return cells
}

// ---------------------------------------------------------------------------

type cell struct {
	ch     rune // ascii ramp glyph
	fg, bg color.NRGBA
	mask   uint32 // kindBlocks: bit i = subpixel i (row-major) is fg
	pw, ph int
	kind   kind
}

func main() {
	var (
		in   = flag.String("in", "", "input image (default tools/stylepreview/testdata/sample.jpg)")
		out  = flag.String("out", "", "output montage PNG (default <input dir>/preview.png)")
		cols = flag.Int("cols", 64, "output width in terminal columns")
		ansi = flag.String("ansi", "", "print this style as ANSI truecolor to stdout: ascii|asciiw|asciied|asciiwl|sextant|sextantx|sextantp|sextanted|sextantlr|all")
		rmse = flag.Bool("rmse", false, "print per-style reconstruction RMSE (lower = more accurate) and exit")
	)
	flag.Parse()

	inputs, err := loadInputs(*in)
	if err != nil {
		fail(err)
	}

	if *rmse {
		for _, in := range inputs {
			fmt.Fprintf(os.Stderr, "== %s ==\n", in.name)
			printRMSE(in.img, *cols)
		}
		return
	}
	if *ansi != "" {
		for _, in := range inputs {
			printANSI(os.Stdout, in.img, *ansi, *cols)
		}
		return
	}

	_, thisFile, _, _ := runtime.Caller(0)
	outDir := filepath.Dir(thisFile)
	for _, in := range inputs {
		dst := *out
		if dst == "" || len(inputs) > 1 {
			dst = filepath.Join(outDir, "preview-"+in.name+".png")
		}
		f, err := os.Create(dst)
		if err != nil {
			fail(err)
		}
		if err := png.Encode(f, buildMontage(in.img, *cols)); err != nil {
			f.Close()
			fail(err)
		}
		f.Close()
		fmt.Println("wrote", dst)
	}
	fmt.Println("  term check: go run ./tools/stylepreview -ansi sextanted | less -R")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "stylepreview:", err)
	os.Exit(1)
}

// input is one loaded example image.
type input struct {
	name string // file stem
	path string
	img  *image.NRGBA
}

// loadInputs takes -in (file or directory) or defaults to every testdata jpg.
func loadInputs(in string) ([]input, error) {
	var paths []string
	if in != "" {
		st, err := os.Stat(in)
		if err != nil {
			return nil, err
		}
		if st.IsDir() {
			if paths, err = filepath.Glob(filepath.Join(in, "*.jpg")); err != nil {
				return nil, err
			}
			pngs, _ := filepath.Glob(filepath.Join(in, "*.png"))
			paths = append(paths, pngs...)
			sort.Strings(paths)
		} else {
			paths = []string{in}
		}
	} else {
		_, thisFile, _, _ := runtime.Caller(0)
		dir := filepath.Join(filepath.Dir(thisFile), "testdata")
		var err error
		if paths, err = filepath.Glob(filepath.Join(dir, "*.jpg")); err != nil {
			return nil, err
		}
		sort.Strings(paths)
	}
	var out []input
	for _, p := range paths {
		img, err := decodeFile(p)
		if err != nil {
			return nil, err
		}
		out = append(out, input{strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)), p, img})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no images found (tried %q)", strings.Join(paths, ", "))
	}
	return out, nil
}

func decodeFile(path string) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	// Flatten onto the black "terminal" background.
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			a := uint32(c.A)
			p := y*out.Stride + x*4
			out.Pix[p] = uint8(uint32(c.R) * a / 255)
			out.Pix[p+1] = uint8(uint32(c.G) * a / 255)
			out.Pix[p+2] = uint8(uint32(c.B) * a / 255)
			out.Pix[p+3] = 255
		}
	}
	return out, nil
}

// boxResize area-averages src to dw x dh (plain sRGB average; the library
// will do linear light, the preview only needs to be representative).
func boxResize(src *image.NRGBA, dw, dh int) *image.NRGBA {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	out := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	for oy := 0; oy < dh; oy++ {
		y0 := oy * sh / dh
		y1 := (oy + 1) * sh / dh
		if y1 <= y0 {
			y1 = y0 + 1
		}
		if y1 > sh {
			y1 = sh
		}
		for ox := 0; ox < dw; ox++ {
			x0 := ox * sw / dw
			x1 := (ox + 1) * sw / dw
			if x1 <= x0 {
				x1 = x0 + 1
			}
			if x1 > sw {
				x1 = sw
			}
			var r, g, b, n uint64
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					c := src.NRGBAAt(sb.Min.X+x, sb.Min.Y+y)
					r += uint64(c.R)
					g += uint64(c.G)
					b += uint64(c.B)
					n++
				}
			}
			out.SetNRGBA(ox, oy, color.NRGBA{
				R: uint8((r + n/2) / n),
				G: uint8((g + n/2) / n),
				B: uint8((b + n/2) / n),
				A: 255,
			})
		}
	}
	return out
}

// linearResize area-averages in linear light then re-encodes to sRGB, the
// way the library plans to resize. sRGB-space averaging smears saturated
// colors and crushes dark detail; light-space averaging does not.
func linearResize(src *image.NRGBA, dw, dh int) *image.NRGBA {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	lin := make([]float64, sw*sh*3)
	for y := 0; y < sh; y++ {
		for x := 0; x < sw; x++ {
			c := src.NRGBAAt(sb.Min.X+x, sb.Min.Y+y)
			i := (y*sw + x) * 3
			lin[i], lin[i+1], lin[i+2] = toLinear[c.R], toLinear[c.G], toLinear[c.B]
		}
	}
	out := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	for oy := 0; oy < dh; oy++ {
		y0 := oy * sh / dh
		y1 := (oy + 1) * sh / dh
		if y1 <= y0 {
			y1 = y0 + 1
		}
		if y1 > sh {
			y1 = sh
		}
		for ox := 0; ox < dw; ox++ {
			x0 := ox * sw / dw
			x1 := (ox + 1) * sw / dw
			if x1 <= x0 {
				x1 = x0 + 1
			}
			if x1 > sw {
				x1 = sw
			}
			var r, g, b, n float64
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					i := (y*sw + x) * 3
					r += lin[i]
					g += lin[i+1]
					b += lin[i+2]
					n++
				}
			}
			out.SetNRGBA(ox, oy, srgbColor([3]float64{r / n, g / n, b / n}))
		}
	}
	return out
}

func rowsFor(cols, w, h int) int {
	r := int(math.Round(float64(cols) * float64(h) / float64(w) * 0.5))
	if r < 1 {
		r = 1
	}
	return r
}

// renderCells resizes img to cols*pw x rows*ph and maps every pw x ph block
// to one cell with the given style's algorithm.
func renderCells(img *image.NRGBA, st style, cols, rows int) []cell {
	var resized *image.NRGBA
	if st.algo == algoLinRes {
		resized = linearResize(img, cols*st.pw, rows*st.ph)
	} else {
		resized = boxResize(img, cols*st.pw, rows*st.ph)
	}
	if st.algo == algoLinExact || st.algo == algoLinED {
		return renderLinearCells(resized, st, cols, rows)
	}
	if st.algo == algoASCIIED {
		return renderASCIIED(resized, st, cols, rows)
	}
	cells := make([]cell, cols*rows)
	sub := make([]color.NRGBA, st.pw*st.ph)
	sbuf := newSplitBuf()
	for cy := 0; cy < rows; cy++ {
		for cx := 0; cx < cols; cx++ {
			for sy := 0; sy < st.ph; sy++ {
				for sx := 0; sx < st.pw; sx++ {
					sub[sy*st.pw+sx] = resized.NRGBAAt(cx*st.pw+sx, cy*st.ph+sy)
				}
			}
			c := cell{pw: st.pw, ph: st.ph, kind: st.kind}
			switch st.kind {
			case kindAscii:
				m := meanColor(sub)
				l := meanLum(sub)
				c.fg = m
				if st.algo == algo2means {
					c.ch = rune(ramp[int(l/255*(float64(len(ramp))-1)+0.5)])
				} else {
					c.ch = rune(ramp[pickRuneIdx(l, lum(m))])
				}
			case kindBlocks:
				if st.algo == algo2means {
					c.mask, c.fg, c.bg = cluster2(sub)
				} else {
					c.mask, c.fg, c.bg = exactSplit(sub, sbuf)
				}
				c.ch = sextantGlyph(int(c.mask))
			}
			cells[cy*cols+cx] = c
		}
	}
	return cells
}

// ----- cell math -----

func lum(c color.NRGBA) float64 {
	return 0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)
}

func meanColor(sub []color.NRGBA) color.NRGBA {
	var r, g, b, n uint64
	for _, c := range sub {
		r += uint64(c.R)
		g += uint64(c.G)
		b += uint64(c.B)
		n++
	}
	if n == 0 {
		return color.NRGBA{}
	}
	return color.NRGBA{R: uint8((r + n/2) / n), G: uint8((g + n/2) / n), B: uint8((b + n/2) / n), A: 255}
}

func meanLum(sub []color.NRGBA) float64 {
	var s float64
	for _, c := range sub {
		s += lum(c)
	}
	return s / float64(len(sub))
}

// cluster2 splits a block's subpixels into a bright and a dark cluster with
// 2-means seeded by the darkest and lightest pixels (the library's planned
// approach for quadrant/sextant). fg is the brighter cluster.
func cluster2(sub []color.NRGBA) (uint32, color.NRGBA, color.NRGBA) {
	n := len(sub)
	if n == 1 {
		return 1, sub[0], sub[0]
	}
	di, li := 0, 0
	for i, c := range sub {
		if lum(c) < lum(sub[di]) {
			di = i
		}
		if lum(c) > lum(sub[li]) {
			li = i
		}
	}
	darker := [3]float64{float64(sub[di].R), float64(sub[di].G), float64(sub[di].B)}
	brighter := [3]float64{float64(sub[li].R), float64(sub[li].G), float64(sub[li].B)}
	var mask uint32
	for it := 0; it < 8; it++ {
		var dm, bm uint32
		var dr, dg, db, dn, br, bg_, bb, bn uint64
		for i, c := range sub {
			p := [3]float64{float64(c.R), float64(c.G), float64(c.B)}
			if dist2(p, brighter) < dist2(p, darker) {
				bm |= 1 << uint(i)
				br += uint64(c.R)
				bg_ += uint64(c.G)
				bb += uint64(c.B)
				bn++
			} else {
				dm |= 1 << uint(i)
				dr += uint64(c.R)
				dg += uint64(c.G)
				db += uint64(c.B)
				dn++
			}
		}
		if bm == mask {
			break
		}
		mask = bm
		if bn > 0 && dn > 0 {
			brighter = [3]float64{float64(br) / float64(bn), float64(bg_) / float64(bn), float64(bb) / float64(bn)}
			darker = [3]float64{float64(dr) / float64(dn), float64(dg) / float64(dn), float64(db) / float64(dn)}
		} else {
			break
		}
	}
	fg := make([]color.NRGBA, 0, n)
	bg := make([]color.NRGBA, 0, n)
	for i, c := range sub {
		if mask>>uint(i)&1 == 1 {
			fg = append(fg, c)
		} else {
			bg = append(bg, c)
		}
	}
	if len(bg) == 0 {
		return mask, meanColor(fg), meanColor(fg)
	}
	if len(fg) == 0 {
		return 0, meanColor(bg), meanColor(bg)
	}
	return mask, meanColor(fg), meanColor(bg)
}

func dist2(a, b [3]float64) float64 {
	d := 0.0
	s := 0.0
	for i := range a {
		s = a[i] - b[i]
		d += s * s
	}
	return d
}

// ----- glyphs for the ANSI path -----

// sextantGlyph maps a bitmask (bit i = sextant cell i, row-major top down) to
// U+1FB00..U+1FB3B, with the Unicode gaps at masks 0, 21, 42, 63 covered by
// space and the older ▌ ▐ █ block elements.
func sextantGlyph(mask int) rune {
	switch mask {
	case 0:
		return ' '
	case 21:
		return '▌'
	case 42:
		return '▐'
	case 63:
		return '█'
	}
	off := 1 // masks are 1-based into the block
	if mask > 21 {
		off++
	}
	if mask > 42 {
		off++
	}
	return rune(0x1FB00 + mask - off)
}

// splitBuf holds reusable subset-sum arrays for exactSplit.
type splitBuf struct {
	sr, sg, sb, q []float64
	cnt           []uint8
}

func newSplitBuf() *splitBuf { return &splitBuf{} }

// exactSplit finds the two-color split of a block with the least total
// squared RGB error by testing every split. Subset-sum DP keeps each split
// O(1), so even a 2x8 block costs ~64k cheap iterations - the provably best
// fg/bg choice for one glyph, unlike 2-means which can stall at a local
// minimum. fg ends up the brighter cluster.
func exactSplit(sub []color.NRGBA, buf *splitBuf) (uint32, color.NRGBA, color.NRGBA) {
	if len(sub) > 16 {
		return cluster2(sub)
	}
	if len(sub) < 2 {
		return 1, sub[0], sub[0]
	}
	p := make([][3]float64, len(sub))
	for i, c := range sub {
		p[i] = [3]float64{float64(c.R), float64(c.G), float64(c.B)}
	}
	mask, f, b := exactSplitCore(p, buf)
	return mask, roundColor(f), roundColor(b)
}

// exactSplitCore is the space-agnostic DP: p holds the block's subpixels in
// whatever RGB space the metric should live in (sRGB or linear light).
func exactSplitCore(p [][3]float64, buf *splitBuf) (uint32, [3]float64, [3]float64) {
	n := len(p)
	lim := 1 << uint(n)
	if len(buf.sr) < lim {
		buf.sr = make([]float64, lim)
		buf.sg = make([]float64, lim)
		buf.sb = make([]float64, lim)
		buf.q = make([]float64, lim)
		buf.cnt = make([]uint8, lim)
	}
	sr, sg, sb, q, cnt := buf.sr[:lim], buf.sg[:lim], buf.sb[:lim], buf.q[:lim], buf.cnt[:lim]
	for m := 1; m < lim; m++ {
		lsb := m & -m
		prev := m ^ lsb
		i := bits.TrailingZeros(uint(lsb))
		r, g, b := p[i][0], p[i][1], p[i][2]
		sr[m] = sr[prev] + r
		sg[m] = sg[prev] + g
		sb[m] = sb[prev] + b
		q[m] = q[prev] + r*r + g*g + b*b
		cnt[m] = cnt[prev] + 1
	}
	tr, tg, tb, tq := sr[lim-1], sg[lim-1], sb[lim-1], q[lim-1]
	best := math.Inf(1)
	bm := 1
	for m := 1; m < lim-1; m++ {
		k := float64(cnt[m])
		nk := float64(n) - k
		dr, dg, db := tr-sr[m], tg-sg[m], tb-sb[m]
		sse := q[m] - (sr[m]*sr[m]+sg[m]*sg[m]+sb[m]*sb[m])/k +
			(tq - q[m]) - (dr*dr+dg*dg+db*db)/nk
		if sse < best {
			best, bm = sse, m
		}
	}
	mF := div3(sr[bm], sg[bm], sb[bm], float64(cnt[bm]))
	mC := div3(tr-sr[bm], tg-sg[bm], tb-sb[bm], float64(n)-float64(cnt[bm]))
	if lum3(mC) > lum3(mF) {
		return uint32((lim - 1) &^ bm), mC, mF
	}
	return uint32(bm), mF, mC
}

func div3(r, g, b, k float64) [3]float64 { return [3]float64{r / k, g / k, b / k} }

func lum3(v [3]float64) float64 { return 0.2126*v[0] + 0.7152*v[1] + 0.0722*v[2] }

func roundColor(v [3]float64) color.NRGBA {
	q := func(x float64) uint8 {
		switch {
		case x <= 0:
			return 0
		case x >= 255:
			return 255
		}
		return uint8(x + 0.5)
	}
	return color.NRGBA{q(v[0]), q(v[1]), q(v[2]), 255}
}

// sRGB transfer functions for the light-space styles.
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

func srgbColor(v [3]float64) color.NRGBA {
	f := func(x float64) uint8 {
		switch {
		case x <= 0:
			return 0
		case x >= 1:
			return 255
		}
		s := 12.92 * x
		if x > 0.0031308 {
			s = 1.055*math.Pow(x, 1.0/2.4) - 0.055
		}
		return uint8(s*255 + 0.5)
	}
	return color.NRGBA{f(v[0]), f(v[1]), f(v[2]), 255}
}

// renderLinearCells implements the light-space sextantx upgrades: the exact
// split measured in linear light (matches how light mixes, and how the
// library will resize), optionally spreading each cell's leftover error to
// not-yet-rendered neighbor cells (Floyd-Steinberg over cells, serpentine).
func renderLinearCells(resized *image.NRGBA, st style, cols, rows int) []cell {
	pw, ph := st.pw, st.ph
	W, H := cols*pw, rows*ph
	lin := make([]float64, W*H*3)
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			c := resized.NRGBAAt(x, y)
			i := (y*W + x) * 3
			lin[i], lin[i+1], lin[i+2] = toLinear[c.R], toLinear[c.G], toLinear[c.B]
		}
	}
	er := make([]float64, W*H*3) // read always; only written when ED is on
	pts := make([][3]float64, pw*ph)
	sbuf := newSplitBuf()
	cells := make([]cell, cols*rows)
	for cy := 0; cy < rows; cy++ {
		d := 1
		if cy%2 == 1 {
			d = -1
		}
		for i := 0; i < cols; i++ {
			cx := i
			if d < 0 {
				cx = cols - 1 - i
			}
			for sy := 0; sy < ph; sy++ {
				for sx := 0; sx < pw; sx++ {
					k := ((cy*ph+sy)*W + cx*pw + sx) * 3
					pts[sy*pw+sx] = [3]float64{lin[k] + er[k], lin[k+1] + er[k+1], lin[k+2] + er[k+2]}
				}
			}
			mask, f, b := exactSplitCore(pts, sbuf)
			c := cell{pw: pw, ph: ph, kind: st.kind, mask: mask,
				fg: srgbColor(f), bg: srgbColor(b), ch: sextantGlyph(int(mask))}
			if st.algo == algoLinED {
				// Per-subpixel residual, Floyd-Steinberg weights, sent to the
				// SAME subpixel position in neighboring cells (a cell-mean
				// version would be a no-op: residuals around cluster means
				// sum to zero).
				for sy := 0; sy < ph; sy++ {
					for sx := 0; sx < pw; sx++ {
						j := sy*pw + sx
						rec := b
						if mask>>uint(j)&1 == 1 {
							rec = f
						}
						pj := pts[j]
						r0 := pj[0] - rec[0]
						r1 := pj[1] - rec[1]
						r2 := pj[2] - rec[2]
						spreadTo := func(tcx, tcy int, w float64) {
							if tcx < 0 || tcx >= cols || tcy < 0 || tcy >= rows {
								return
							}
							k := ((tcy*ph+sy)*W + tcx*pw + sx) * 3
							er[k] += w * r0
							er[k+1] += w * r1
							er[k+2] += w * r2
						}
						spreadTo(cx+d, cy, 7.0/16)
						spreadTo(cx-d, cy+1, 3.0/16)
						spreadTo(cx, cy+1, 5.0/16)
						spreadTo(cx+d, cy+1, 1.0/16)
					}
				}
			}
			cells[cy*cols+cx] = c
		}
	}
	return cells
}

// ---------------------------------------------------------------------------
// PNG montage

type canvas struct {
	img *image.NRGBA
}

func (cv *canvas) fill(x0, y0, w, h int, c color.NRGBA) {
	b := cv.img.Bounds()
	for y := y0; y < y0+h && y < b.Max.Y; y++ {
		for x := x0; x < x0+w && x < b.Max.X; x++ {
			if x < b.Min.X || y < b.Min.Y {
				continue
			}
			cv.img.SetNRGBA(x, y, c)
		}
	}
}

func (cv *canvas) dot(cx, cy, r float64, c color.NRGBA) {
	for dy := -r - 1; dy <= r+1; dy++ {
		for dx := -r - 1; dx <= r+1; dx++ {
			if dx*dx+dy*dy <= r*r {
				cv.fill(int(math.Round(cx+dx)), int(math.Round(cy+dy)), 1, 1, c)
			}
		}
	}
}

var white = color.NRGBA{255, 255, 255, 255}
var gray = color.NRGBA{150, 150, 150, 255}
var black = color.NRGBA{}

func buildMontage(src *image.NRGBA, cols int) *image.NRGBA {
	sb := src.Bounds()
	w, h := sb.Dx(), sb.Dy()
	rows := rowsFor(cols, w, h)
	content := cols * cellW
	width := content + 2*pad

	refScale := float64(content) / float64(w)
	refH := int(float64(h) * refScale)
	if refH > refMaxH {
		refH = refMaxH
	}
	refW := int(float64(refH) * float64(w) / float64(h))

	height := pad + labelH + refH + 2*gap
	height += len(styles) * (labelH + rows*cellH + gap)
	cv := &canvas{img: image.NewNRGBA(image.Rect(0, 0, width, height))}
	cv.fill(0, 0, width, height, black)

	// reference strip
	y := pad
	drawText(cv, "0  REFERENCE", pad, y, 2, white)
	y += labelH
	for ry := 0; ry < refH; ry++ {
		for rx := 0; rx < refW; rx++ {
			sx := rx * w / refW
			sy := ry * h / refH
			cv.fill(pad+rx, y+ry, 1, 1, src.NRGBAAt(sx, sy))
		}
	}
	y += refH + gap

	for _, st := range styles {
		cv.fill(0, y-4, width, 1, gray)
		drawText(cv, st.name, pad, y, 2, white)
		drawText(cv, st.desc, pad, y+14, 1, gray)
		cy := y + labelH
		cells := renderCells(src, st, cols, rows)
		for r := 0; r < rows; r++ {
			for c := 0; c < cols; c++ {
				drawCell(cv, cells[r*cols+c], pad+c*cellW, cy+r*cellH)
			}
		}
		y = cy + rows*cellH + gap
	}
	return cv.img
}

func drawCell(cv *canvas, c cell, x, y int) {
	switch c.kind {
	case kindAscii:
		if c.ch != ' ' {
			drawGlyph(cv, c.ch, x, y, cellW, cellH, c.fg)
		}
	case kindBlocks:
		sw, sh := cellW/c.pw, cellH/c.ph
		for i := 0; i < c.pw*c.ph; i++ {
			col := c.bg
			if c.mask>>uint(i)&1 == 1 {
				col = c.fg
			}
			cv.fill(x+(i%c.pw)*sw, y+(i/c.pw)*sh, sw, sh, col)
		}
	}
}

// ---------------------------------------------------------------------------
// ANSI output (-ansi)

// printRMSE scores each style fairly: it builds the pixel model the
// terminal would show at a fixed cell of cellW x cellH (for ascii: the FG
// attenuated by glyph coverage) and compares against the source downscaled
// to the same canvas. Same terminal size for every style, so lower is more
// accurate.
func printRMSE(img *image.NRGBA, cols int) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	fmt.Println("style         RMSE(0-255)  PSNR   dE(Oklab)")
	for _, st := range styles {
		switch st.kind {
		case kindAscii, kindBlocks:
		default:
			continue
		}
		rows := rowsFor(cols, w, h)
		cells := renderCells(img, st, cols, rows)
		src := boxResize(img, cols*cellW, rows*cellH)
		var acc, accLab float64
		n := 0
		for cy := 0; cy < rows; cy++ {
			for cx := 0; cx < cols; cx++ {
				c := cells[cy*cols+cx]
				for sy := 0; sy < st.ph; sy++ {
					for sx := 0; sx < st.pw; sx++ {
						rec := c.bg
						if c.kind == kindAscii {
							rec = scaleColor(c.fg, rampCover(c.ch))
						} else if c.mask>>uint(sy*st.pw+sx)&1 == 1 {
							rec = c.fg
						}
						x0 := (cx*st.pw + sx) * cellW / st.pw
						x1 := (cx*st.pw + sx + 1) * cellW / st.pw
						y0 := (cy*st.ph + sy) * cellH / st.ph
						y1 := (cy*st.ph + sy + 1) * cellH / st.ph
						for py := y0; py < y1; py++ {
							for px := x0; px < x1; px++ {
								a := src.NRGBAAt(px, py)
								dr := float64(int(a.R) - int(rec.R))
								dg := float64(int(a.G) - int(rec.G))
								db := float64(int(a.B) - int(rec.B))
								acc += dr*dr + dg*dg + db*db
								accLab += oklabDist2(a, rec)
								n++
							}
						}
					}
				}
			}
		}
		rmse := math.Sqrt(acc / float64(3*n))
		labrmse := math.Sqrt(accLab/float64(n)) * 100
		fmt.Printf("%-13s %8.2f    %5.1f dB  %7.2f\n", st.id, rmse, 20*math.Log10(255/rmse), labrmse)
	}
}

// oklab maps sRGB bytes to CIE Oklab (perceptually near-uniform), so its
// RMSE is an honest "looks-like" score next to the raw sRGB number.
func oklab(c color.NRGBA) (l, a, b float64) {
	r, g, bl := toLinear[c.R], toLinear[c.G], toLinear[c.B]
	l_ := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*bl)
	m_ := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*bl)
	s_ := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*bl)
	return 0.2104542553*l_ + 0.7936177850*m_ - 0.0040720468*s_,
		1.9779984951*l_ - 2.4285922051*m_ + 0.4505937099*s_,
		0.0259040371*l_ + 0.7827717662*m_ - 0.8086757660*s_
}

func oklabDist2(a, b color.NRGBA) float64 {
	al, aa, ab := oklab(a)
	bl, ba, bb := oklab(b)
	d := al - bl
	x := aa - ba
	y := ab - bb
	return d*d + x*x + y*y
}

func printANSI(w io.Writer, src *image.NRGBA, id string, cols int) {
	for _, st := range styles {
		if id != "all" && st.id != id {
			continue
		}
		rows := rowsFor(cols, src.Bounds().Dx(), src.Bounds().Dy())
		cells := renderCells(src, st, cols, rows)
		fmt.Fprintf(os.Stderr, "== %s (%s) %dx%d ==\n", st.name, st.desc, cols, rows)
		for r := 0; r < rows; r++ {
			for c := 0; c < cols; c++ {
				writeANSICell(w, cells[r*cols+c])
			}
			fmt.Fprint(w, "\x1b[0m\n")
		}
	}
}

func writeANSICell(w io.Writer, c cell) {
	switch c.kind {
	case kindAscii:
		fmt.Fprintf(w, "\x1b[38;2;%d;%d;%dm%c", c.fg.R, c.fg.G, c.fg.B, c.ch)
	case kindBlocks:
		fmt.Fprintf(w, "\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm%c",
			c.fg.R, c.fg.G, c.fg.B, c.bg.R, c.bg.G, c.bg.B, c.ch)
	}
}

// ---------------------------------------------------------------------------
// tiny 3x5 font for labels and the ASCII-ramp preview

// font3x5 rows: 5 strings of 3 chars ('#' = ink).
var font3x5 = map[rune][]string{
	' ': {"   ", "   ", "   ", "   ", "   "},
	'.': {"   ", "   ", "   ", " # ", "   "},
	':': {"   ", " # ", "   ", " # ", "   "},
	'-': {"   ", "   ", "###", "   ", "   "},
	'=': {"   ", "###", "   ", "###", "   "},
	'+': {"   ", " # ", "###", " # ", "   "},
	'*': {"#.#", ".#.", "###", ".#.", "#.#"},
	'#': {".#.", "###", ".#.", "###", ".#."},
	'%': {"##.", "  #", " .#", "#..", ".##"},
	'@': {".##", "#.#", "#.#", ".#.", ".##"},
	'/': {"  #", "  #", " .#", ".# ", "#  "},
	'0': {"###", "#.#", "#.#", "#.#", "###"},
	'1': {" #.", "##.", " #.", " #.", "###"},
	'2': {"##.", "..#", " .#", "#..", "##."},
	'3': {"##.", "..#", " .#", "..#", "##."},
	'4': {"#.#", "#.#", "###", "..#", "..#"},
	'5': {"###", "#..", "##.", "..#", "##."},
	'6': {".##", "#..", "##.", "#.#", ".##"},
	'7': {"###", "..#", " .#", " .#", " .#"},
	'8': {"###", "#.#", " .#", "#.#", "###"},
	'9': {"##.", "#.#", ".##", "..#", "..#"},
	'A': {" .#", "#.#", "###", "#.#", "#.#"},
	'B': {"##.", "#.#", "##.", "#.#", "##."},
	'C': {".##", "#..", "#..", "#..", ".##"},
	'D': {"##.", "#.#", "#.#", "#.#", "##."},
	'E': {"###", "#..", "##.", "#..", "###"},
	'F': {"###", "#..", "##.", "#..", "#.."},
	'G': {".##", "#..", "#.#", "#.#", ".##"},
	'H': {"#.#", "#.#", "###", "#.#", "#.#"},
	'I': {"###", " #.", " #.", " #.", "###"},
	'J': {"###", "..#", "..#", "#.#", "##."},
	'K': {"#.#", "#.#", "##.", "#.#", "#.#"},
	'L': {"#..", "#..", "#..", "#..", "###"},
	'M': {"#.#", "###", "###", "#.#", "#.#"},
	'N': {"#.#", "###", "###", "###", "#.#"},
	'O': {"###", "#.#", "#.#", "#.#", "###"},
	'P': {"##.", "#.#", "##.", "#..", "#.."},
	'Q': {"###", "#.#", "#.#", "##.", "..#"},
	'R': {"##.", "#.#", "##.", "#.#", "#.#"},
	'S': {".##", "#..", " .#", "..#", "##."},
	'T': {"###", " #.", " #.", " #.", " #."},
	'U': {"#.#", "#.#", "#.#", "#.#", "###"},
	'V': {"#.#", "#.#", "#.#", "#.#", " .#"},
	'W': {"#.#", "#.#", "###", "###", "#.#"},
	'X': {"#.#", "#.#", " .#", "#.#", "#.#"},
	'Y': {"#.#", "#.#", " .#", " .#", " .#"},
	'Z': {"###", "..#", " .#", "#..", "###"},
}

func pattern(ch rune) []string {
	if p, ok := font3x5[unicodeUpper(ch)]; ok {
		return p
	}
	return nil
}

func unicodeUpper(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - 32
	}
	return r
}

func drawText(cv *canvas, s string, x, y, scale int, c color.NRGBA) {
	for _, r := range s {
		drawGlyphScaled(cv, r, x, y, 3*scale, 5*scale, c)
		x += 4 * scale
	}
}

// drawGlyph draws a font glyph stretched into the given cell box (ASCII ramp preview).
func drawGlyph(cv *canvas, r rune, x, y, w, h int, c color.NRGBA) {
	drawGlyphScaled(cv, r, x, y, w, h, c)
}

func drawGlyphScaled(cv *canvas, r rune, x, y, w, h int, c color.NRGBA) {
	p := pattern(r)
	if p == nil {
		return
	}
	for row := 0; row < 5; row++ {
		for col := 0; col < 3; col++ {
			if p[row][col] == '#' {
				cv.fill(x+col*w/3, y+row*h/5, (col+1)*w/3-col*w/3, (row+1)*h/5-row*h/5, c)
			}
		}
	}
}
