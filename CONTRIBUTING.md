# Contributing

Everything here runs on a Mac with Go and Xcode installed.

## The tests

Three suites, none of which needs a Windows or Linux machine:

```bash
go test -race ./...                             # the worker, the router, the lock
cd bindings/python && python3 -m unittest       # the Python binding, end to end
swift build                                     # the Swift package compiles
```

The same Go suite runs on four other platforms, each with a runtime to
install first:

```bash
scripts/run-ios.sh --test                                   # in the iOS Simulator
GOOS=js GOARCH=wasm go test -exec="$(go env GOROOT)/lib/wasm/go_js_wasm_exec" \
    -run InThisProcess .                                    # in node
go test -run Wasm .                                         # under wasmtime, if installed
scripts/run-plan9.sh                                        # on 9front, in a VM
```

The Plan 9 one takes a few minutes: it is x86, so an Apple Silicon Mac
emulates it. It is worth the wait - the lock there is the exclusive-use bit
rather than flock, and that is the only part of vero no other platform
exercises.

The Go tests take about half a minute, because several of them start a real
worker process and wait on it.

The Python tests build the example worker first and drive it through a host -
a worker run with `VERO_HOST=1`, supervising a second copy of itself - so what
they exercise is the whole stack rather than the binding on its own. No cgo,
and no shared library.

There is no Swift test target: `swift build` is there to catch a package that
no longer compiles.

Before sending a change, `gofmt -l .` should print nothing and `go vet ./...`
should be quiet.

## Building for every platform

`scripts/build-all.sh` builds a worker for seventeen targets into `dist/`,
plus the C archive the Swift package links.
Everything but that archive is a plain cross-compile: the worker supervises
itself, so a frontend spawns it rather than loading a library, and no foreign
C toolchain is involved.

`scripts/setup.sh` installs Docker and qemu - needed to *run* the examples,
not to build them - and `scripts/run.sh` opens three of them at once: macOS
natively, Linux in a container over VNC, and Windows in a VM.
`scripts/run-freebsd.sh` does the same for FreeBSD, in a VM of its own.

The example apps all drive the same worker in
[example/worker](example/worker):

| | |
|---|---|
| [example/menubar-app](example/menubar-app) | macOS, SwiftUI |
| [example/wpf-app](example/wpf-app) | Windows, WPF |
| [example/gtk-app](example/gtk-app) | Linux and FreeBSD, GTK4 - the same app, unchanged |
| [example/android-app](example/android-app) | Android, Kotlin |
| [example/ios-app](example/ios-app) | iOS, SwiftUI, with the worker compiled in |
| [example/web-app](example/web-app) | a browser, in wasm, with the worker compiled in |

The GTK one runs unchanged on FreeBSD, because nothing in it is
Linux-specific once the shared library is gone. The last two compile the
worker into the application rather than starting it, because neither iOS nor
a browser may start a program; `scripts/run-ios.sh`, `scripts/run-android.sh`
and `scripts/run-web.sh` build and run them.
