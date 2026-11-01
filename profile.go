package gotuipic

import "strings"

// ProfileFromEnv infers a sensible color profile from terminal environment
// variables. It never reads the environment itself - the caller passes a
// getter (os.Getenv, or a stub in tests) because the library does not touch
// the environment.
//
// The mapping: NO_COLOR (any non-empty value) means ColorNone;
// COLORTERM=truecolor/24bit means ColorTrueColor; a 256color-capable TERM
// means ColorANSI256; TERM=dumb or TERM=off means ColorNone; everything
// else falls back to ColorANSI16.
func ProfileFromEnv(getenv func(string) string) ColorProfile {
	if getenv == nil {
		return ColorTrueColor
	}
	if getenv("NO_COLOR") != "" {
		return ColorNone
	}
	switch getenv("COLORTERM") {
	case "truecolor", "24bit":
		return ColorTrueColor
	}
	term := getenv("TERM")
	switch {
	case term == "dumb" || term == "off" || term == "":
		return ColorNone
	case strings.Contains(term, "256color"),
		strings.Contains(term, "truecolor"),
		strings.Contains(term, "direct"):
		return ColorANSI256
	default:
		return ColorANSI16
	}
}
