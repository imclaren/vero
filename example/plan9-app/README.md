# The Plan 9 example

The same three jobs as every other example, in a rio window:

```bash
scripts/run-plan9.sh
```

![the Plan 9 example](../../docs/screenshots/plan9.gif)

`scripts/run-plan9.sh --record out.gif` records it, and
`scripts/run-plan9.sh --test` runs vero's own test suite on Plan 9 instead.

## What is different here

Plan 9 has no toolkit. It has libdraw: a window is an image, and a program
draws on it. Every other frontend in this repository is a binding to a
toolkit - SwiftUI, WPF, GTK - and this one is a hundred lines of drawing
rectangles and text, with a click being a point inside a rectangle.

The frontend is Go, which is unusual among the examples and is the natural
choice here: `9fans.net/go/draw` is a port of libdraw that speaks to
`/dev/draw` directly, so there is nothing to bind to and the frontend can use
`vero.Supervise` itself rather than one of the bindings. The worker is
[../worker](../worker), built for `plan9/amd64` and started beside the app
as a process, exactly as on macOS, Windows and the BSDs. The single-worker
lock is Plan 9's exclusive-use bit rather than `flock`, which this is the
only example to exercise.

## The library

`go.mod` points `9fans.net/go` at a fork,
[imclaren/9fans-go](https://github.com/imclaren/9fans-go), because the
current upstream does not compile for `GOOS=plan9`: a few lines of
`mux_plan9.go` went stale against the rest of the package, and an older
revision that does compile turns out to exec a Unix helper rather than open
`/dev/draw`. The fork is upstream plus one ten-line commit on the
`plan9-build` branch. This is also why the example is a module of its own,
with its own `go.mod`: vero itself has no dependencies, and the one this needs
is wanted by nothing else.

## Driving it from a Mac

Everything `run-plan9.sh` does, it does by typing: 9front boots from its ISO
into rio, and there is no ssh and no install. Three things about that are
not obvious:

* rio sends the keyboard to the window that was **clicked** last, not the one
  under the pointer. Typing into a window that has only been pointed at goes
  nowhere, silently.
* The VM has a PS/2 mouse, so the pointer is moved with relative events, and
  9front moves it two pixels per unit.
* The binaries arrive over `hget` from a web server on the Mac, which qemu's
  user networking places at 10.0.2.2.

## The files

| | |
|---|---|
| [main.go](main.go) | the frontend: rows, buttons and bars, drawn through `/dev/draw` |
| [go.mod](go.mod) | its own module, pinned to the fork of `9fans.net/go` |
| [../../scripts/run-plan9.sh](../../scripts/run-plan9.sh) | boots 9front, fetches both binaries, runs the app |
