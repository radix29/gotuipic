# examples

Runnable programs that use gotuipic the way a real application would.
They live in the main module (stdlib-only, like the library), except
`examples/tcell`, which will be its own module when it lands (tcell is a
dependency the library must never grow).

| Program | Shows |
|---|---|
| `basic` | file/stdin → colored art: `-w -h -mode -color -fit -ramp -invert` |
| `stream` | URL, pipe or file → art over one `io.Reader` path: HTTP status handling, timeouts, `-maxpixels` |

Try them against the design tool's example photos:

```sh
go run ./examples/basic -w 100 tools/stylepreview/testdata/sample.jpg | less -R
go run ./examples/basic -mode ascii -color 256 tools/stylepreview/testdata/sample-fruit.jpg
go run ./examples/basic -mode ascii -color none -w 60 tools/stylepreview/testdata/sample-landscape.jpg

# streams: the library sniffs the format from the bytes, so a URL, a pipe
# and a file are the same code path (see examples/stream/main.go)
curl -sL https://picsum.photos/800/600 | go run ./examples/stream -w 90 -mode ascii -
go run ./examples/stream -w 90 https://picsum.photos/800/600 | less -R
go run ./examples/stream -maxpixels 100000 https://picsum.photos/800/600 # rejected from the header, before decoding
```

Runnable `// Output:`-checked examples for godoc live in `example_test.go` at
the repo root (`Convert`, `ConvertReader`, `ConvertBytes`, `ConvertFile`,
`Art.WriteTo`, `Art.Cells`).

Planned (see PLAN.md): `modes` (one image, every mode), `gif` (animated),
`tcell` (live image viewer, separate module).
