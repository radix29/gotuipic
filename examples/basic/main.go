// Command basic renders an image file (or stdin with "-") as terminal art
// with the gotuipic library - the smallest complete picture of the API.
//
//	go run ./examples/basic -w 100 ~/Pictures/cat.jpg | less -R
//	go run ./examples/basic -mode ascii -color 256 cat.png
//
// The library never inspects the environment; this program does, which is
// exactly the split the design calls for.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/radix29/gotuipic"
	_ "github.com/radix29/gotuipic/formats" // bmp, tiff, webp
)

func main() {
	width := flag.Int("w", 0, "output width in columns (default: auto-detect terminal, else 80)")
	mode := flag.String("mode", "sextant", "sextant | ascii")
	colorFlag := flag.String("color", "auto", "auto | truecolor | 256 | 16 | none")
	fit := flag.String("fit", "contain", "contain | stretch | crop (needs -h)")
	height := flag.Int("h", 0, "height in rows (with -fit)")
	invert := flag.Bool("invert", false, "reverse the ASCII ramp")
	ramp := flag.String("ramp", "", "ASCII ramp, dark to light (default \" .:-=+*#%@\")")
	stdin := flag.Bool("stdin", false, "read the image from stdin instead of a file argument")
	flag.Parse()

	var opts []gotuipic.Option
	w := *width
	if w == 0 {
		w = terminalWidth()
	}
	opts = append(opts, gotuipic.WithWidth(w))
	if *height > 0 {
		opts = append(opts, gotuipic.WithHeight(*height))
		switch *fit {
		case "contain":
			opts = append(opts, gotuipic.WithFit(gotuipic.FitContain))
		case "stretch":
			opts = append(opts, gotuipic.WithFit(gotuipic.FitStretch))
		case "crop":
			opts = append(opts, gotuipic.WithFit(gotuipic.FitCrop))
		default:
			fmt.Fprintln(os.Stderr, "bad -fit")
			os.Exit(2)
		}
	}
	switch *mode {
	case "sextant":
		opts = append(opts, gotuipic.WithMode(gotuipic.ModeSextant))
	case "ascii":
		opts = append(opts, gotuipic.WithMode(gotuipic.ModeASCII))
	default:
		fmt.Fprintln(os.Stderr, "bad -mode")
		os.Exit(2)
	}
	switch *colorFlag {
	case "auto":
		// The one place environment detection belongs: the caller, never
		// the library.
		opts = append(opts, gotuipic.WithColor(gotuipic.ProfileFromEnv(os.Getenv)))
	case "truecolor":
		opts = append(opts, gotuipic.WithColor(gotuipic.ColorTrueColor))
	case "256":
		opts = append(opts, gotuipic.WithColor(gotuipic.ColorANSI256))
	case "16":
		opts = append(opts, gotuipic.WithColor(gotuipic.ColorANSI16))
	case "none":
		opts = append(opts, gotuipic.WithColor(gotuipic.ColorNone))
	default:
		fmt.Fprintln(os.Stderr, "bad -color")
		os.Exit(2)
	}
	if *ramp != "" {
		opts = append(opts, gotuipic.WithRamp(*ramp))
	}
	opts = append(opts, gotuipic.WithInvert(*invert))

	var (
		art *gotuipic.Art
		err error
	)
	if *stdin || flag.NArg() == 0 || flag.Arg(0) == "-" {
		art, err = gotuipic.ConvertReader(os.Stdin, opts...)
	} else {
		art, err = gotuipic.ConvertFile(flag.Arg(0), opts...)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, strings.TrimRight(err.Error(), "\n"))
		os.Exit(1)
	}
	if _, err := art.WriteTo(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// terminalWidth asks the terminal, falling back to $COLUMNS then 80. It
// deliberately avoids golang.org/x/term for now so this example stays in
// the main module; a planned cmd/gotuipic will do the proper ioctl.
func terminalWidth() int {
	if c := os.Getenv("COLUMNS"); c != "" {
		var n int
		if _, err := fmt.Sscanf(c, "%d", &n); err == nil && n > 0 {
			return n
		}
	}
	return 80
}
