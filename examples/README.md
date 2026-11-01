# examples

Runnable programs that use gotuipic the way a real application would.
They live in the main module (stdlib-only, like the library), except
`examples/tcell`, which will be its own module when it lands (tcell is a
dependency the library must never grow).

| Program | Shows |
|---|---|
| `basic` | file/stdin → colored art: `-w -h -mode -color -fit -ramp -invert` |

Try them against the design tool's example photos:

```sh
go run ./examples/basic -w 100 tools/stylepreview/testdata/sample.jpg | less -R
go run ./examples/basic -mode ascii -color 256 tools/stylepreview/testdata/sample-fruit.jpg
go run ./examples/basic -mode ascii -color none -w 60 tools/stylepreview/testdata/sample-landscape.jpg
```

Planned (see PLAN.md): `modes` (one image, every mode), `gif` (animated),
`tcell` (live image viewer, separate module).
