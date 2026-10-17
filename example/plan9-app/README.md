# The Plan 9 example

The same three jobs as every other example, in a rio window on 9front.

```bash
scripts/run-plan9.sh
```

![the Plan 9 example](../../docs/screenshots/plan9.gif)

`scripts/run-plan9.sh --record out.gif` records it, and
`scripts/run-plan9.sh --test` runs vero's own test suite on Plan 9
instead.

## What is different here

Plan 9 has no toolkit. It has libdraw: a window is an image, and a
program draws on it. Every other front end in this repository binds to a
toolkit. This one is a hundred lines that draw rectangles and text, and
a click is a point inside a rectangle.

The front end is written in Go, which no other example does. It is the
natural choice here: `9fans.net/go/draw` is a port of libdraw that talks
to `/dev/draw` directly, so there is nothing to bind to, and the front
end can use `vero.Supervise` itself. The worker is
[../worker](../worker), built for `plan9/amd64` and started beside the
app as a process, exactly as on the desktop. The single-copy lock uses
Plan 9's exclusive-use bit rather than `flock`, and this is the only
example that exercises it.

## The library

`go.mod` points `9fans.net/go` at a fork,
[imclaren/9fans-go](https://github.com/imclaren/9fans-go), because
upstream does not compile for `GOOS=plan9`. The fork is upstream plus one
ten-line commit on the `plan9-build` branch. This is also why the example
is a module of its own: vero itself has no dependencies, and nothing else
needs this one.

## Driving it from a Mac

`run-plan9.sh` does everything by typing into the VM. 9front boots from
its ISO into rio, and there is no SSH and no install. Three things about
that are not obvious:

- rio sends the keyboard to the window that was **clicked** last, not the
  one under the pointer. Typing into a window that has only been pointed
  at goes nowhere, silently.
- The VM has a PS/2 mouse, so the pointer moves by relative events, and
  9front moves it two pixels per unit.
- The binaries arrive over `hget` from a web server on the Mac, which
  qemu's user networking places at 10.0.2.2.

## The files

| | |
|---|---|
| [main.go](main.go) | the front end: rows, buttons and bars, drawn through `/dev/draw` |
| [go.mod](go.mod) | its own module, pinned to the fork of `9fans.net/go` |
| [../../scripts/run-plan9.sh](../../scripts/run-plan9.sh) | boots 9front, fetches both binaries, runs the app |
