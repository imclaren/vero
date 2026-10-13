# Contributing

This page explains how to test vero itself. Everything here runs on a Mac
with Go and Xcode installed.

## The tests

Three suites need no Windows or Linux machine.

```bash
go test -race ./...                             # the worker, the router, the lock
cd bindings/python && python3 -m unittest       # the Python binding, end to end
swift build                                     # the Swift package compiles
```

The same Go suite runs on four other platforms, each with a runtime to
install first.

```bash
scripts/run-ios.sh --test                                   # in the iOS Simulator
GOOS=js GOARCH=wasm go test -exec="$(go env GOROOT)/lib/wasm/go_js_wasm_exec" \
    -run InThisProcess .                                    # in node
go test -run Wasm .                                         # under wasmtime, if installed
scripts/run-plan9.sh --test                                 # on 9front, in a VM
```

The Plan 9 one takes a few minutes, since Plan 9 is x86 and an Apple
silicon Mac emulates it. It is worth the wait, because the lock there is the
exclusive-use bit rather than flock, and that is the only part of vero that
no other platform exercises.

The Go tests take about half a minute, because several of them start a real
worker process and wait for it.

The Python tests build the example worker first and drive it through a
host, which is a worker run with `VERO_HOST=1` that supervises a second
copy of itself. So they exercise the whole stack rather than the binding
on its own, with no cgo and no shared library.

There is no Swift test target. `swift build` is there to catch a package
that no longer compiles.

The suite includes one test that is about the README rather than the
code. It fails when `go tool dist list` names a target that the table at
the top of the README does not, and it reads only the table, which must
therefore come before the README's first `## ` heading. Go adds ports,
and that table is the only place where the list is written down.

Before sending a change, `gofmt -l .` should print nothing and `go vet ./...`
should be quiet.

## Building and running everywhere

`scripts/build-all.sh` builds a worker for eighteen targets into `dist/`,
plus the C archive that the Swift package links. Everything but that
archive is a plain cross-compile. The worker supervises itself, so a front
end starts it rather than loading a library, and no foreign C toolchain is
involved. `scripts/build-cvero.sh` builds that archive from the checkout,
for Swift code that needs C functions which the last release's archive
lacks.

[Build, run and test on every system](docs/test.md) describes the scripts
that run the examples and test the packaging on each system, and
[The bindings and the examples](docs/bindings.md) lists the examples.
`scripts/test-starters.sh` makes a new app of each shape with `vero new`
from this checkout and builds each front end, and
`scripts/test-bindings.sh` checks the cancellation that the Swift and C#
bindings do.
