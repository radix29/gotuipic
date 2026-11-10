// Command stream renders an image that arrives as a stream of bytes instead
// of a file you can name: an HTTP response body, a pipe from curl, a socket,
// a tar entry. It exists to show the io.Reader path - gotuipic sniffs the
// format from the header, so no file name, extension, seek or stat is needed.
//
//	go run ./examples/stream https://picsum.photos/800/600
//	curl -sL https://picsum.photos/800/600 | go run ./examples/stream -
//	go run ./examples/stream cat.jpg          # files take the same path
//
// The library still never touches the environment; this program reads $TERM
// and $COLORTERM through gotuipic.ProfileFromEnv, which is the caller's job.
package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/radix29/gotuipic"
	_ "github.com/radix29/gotuipic/formats" // bmp, tiff, webp
)

func main() {
	width := flag.Int("w", 80, "output width in columns")
	mode := flag.String("mode", "sextant", "sextant | ascii")
	colorFlag := flag.String("color", "auto", "auto | truecolor | 256 | 16 | none")
	maxPixels := flag.Int64("maxpixels", 64<<20, "refuse images with more pixels than this")
	timeout := flag.Duration("timeout", 20*time.Second, "network timeout")
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: stream [-w N] [-mode M] [-color C] URL | - | FILE")
		os.Exit(2)
	}

	var opts []gotuipic.Option
	opts = append(opts, gotuipic.WithWidth(*width), gotuipic.WithMaxPixels(*maxPixels))
	switch *mode {
	case "sextant":
		opts = append(opts, gotuipic.WithMode(gotuipic.ModeSextant))
	case "ascii":
		opts = append(opts, gotuipic.WithMode(gotuipic.ModeASCII))
	default:
		fmt.Fprintln(os.Stderr, "bad -mode")
		os.Exit(2)
	}
	if *colorFlag == "auto" {
		// Environment detection belongs here, in the caller: the library
		// itself has no idea it is drawing into a terminal.
		opts = append(opts, gotuipic.WithColor(gotuipic.ProfileFromEnv(os.Getenv)))
	} else {
		prof, ok := map[string]gotuipic.ColorProfile{
			"truecolor": gotuipic.ColorTrueColor,
			"256":       gotuipic.ColorANSI256,
			"16":        gotuipic.ColorANSI16,
			"none":      gotuipic.ColorNone,
		}[*colorFlag]
		if !ok {
			fmt.Fprintln(os.Stderr, "bad -color")
			os.Exit(2)
		}
		opts = append(opts, gotuipic.WithColor(prof))
	}

	src, closeFn, err := open(flag.Arg(0), *timeout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stream:", err)
		os.Exit(1)
	}
	defer closeFn()

	// One call, whatever the source. Go's http transport already inflates
	// gzip responses it requested, so this reader yields real PNG/JPEG bytes
	// for a URL, a pipe or a file alike.
	art, err := gotuipic.ConvertReader(src, opts...)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stream:", err)
		os.Exit(1)
	}
	if _, err := art.WriteTo(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "stream:", err)
		os.Exit(1)
	}
}

// open turns a URL, "-" (stdin) or a path into an io.Reader plus a func that
// releases it. Nothing downstream cares which it was.
func open(arg string, timeout time.Duration) (io.Reader, func(), error) {
	switch {
	case arg == "-":
		return os.Stdin, func() {}, nil
	case strings.HasPrefix(arg, "http://"), strings.HasPrefix(arg, "https://"):
		client := &http.Client{Timeout: timeout}
		resp, err := client.Get(arg)
		if err != nil {
			return nil, func() {}, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, func() {}, fmt.Errorf("%s: %s", arg, resp.Status)
		}
		return resp.Body, func() { resp.Body.Close() }, nil
	default:
		f, err := os.Open(arg)
		if err != nil {
			return nil, func() {}, err
		}
		return f, func() { f.Close() }, nil
	}
}
