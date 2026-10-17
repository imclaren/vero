# Contributing

This page is for working on vero itself, not on an app built with it.
Everything runs on a Mac with Go and Xcode.

## Before sending a change

```bash
gofmt -l .          # prints nothing
go vet ./...        # says nothing
go test -race ./... # passes
```

## The tests, and what each covers

The quick ones, which every change should pass:

```bash
go test -race ./...                             # the worker, the router and the lock
(cd cmd/vero && go test ./...)                  # the vero command
(cd cmd/vero-repo && go test ./...)             # the packaging
(cd bindings/python && python3 -m unittest)     # the Python binding, against a real worker
swift build                                     # the Swift package compiles
```

The slower ones, for a change to a binding, a starter, or the protocol:

```bash
scripts/test-starters.sh        # vero new from this checkout, with each front end built
scripts/test-bindings.sh        # cancellation in the Swift and C# bindings
scripts/run-ios.sh --test       # the Go tests in the iOS Simulator
go test -run Wasm .             # the WASI worker, under wasmtime, when it is installed
scripts/run-plan9.sh --test     # the Go tests on 9front, in an emulated VM
GOOS=js GOARCH=wasm go test -run InThisProcess \
    -exec="$(go env GOROOT)/lib/wasm/go_js_wasm_exec" .   # the in-process worker, in node
```

The Plan 9 run takes a few minutes and is worth it: its file lock is the
exclusive-use bit rather than `flock`, and no other system exercises that
code.

Two tests have conditions worth knowing:

- The Swift generator's test in `cmd/vero` builds against a checkout whose
  folder is named `vero`, because SwiftPM names a package after its folder.
- A test fails when Go adds a target that the table in
  [the lab](docs/lab.md) lacks. Add the target to the table.

## Where things are

- The bindings are in [bindings](bindings) (Python, C#, Kotlin) and
  [Sources](Sources) (Swift). Each one's header comment documents it.
- `scripts/build-all.sh` builds the worker for every target in bold in
  the lab's table, into `dist/`.
- [The Windows VM's notes](.windows/unattend/README.md) explain how that
  VM is installed.
