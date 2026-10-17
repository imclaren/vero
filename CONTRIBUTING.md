# Contributing

These are the tests of vero itself. Each runs on a Mac with Go and Xcode.

```bash
go test -race ./...                             # the worker, the router and the lock
(cd cmd/vero && go test ./...)                  # the vero command
(cd cmd/vero-repo && go test ./...)             # the packaging
(cd bindings/python && python3 -m unittest)     # the Python binding, against a real worker
swift build                                     # the Swift package compiles
scripts/test-starters.sh                        # vero new from this checkout, with each front end built
scripts/test-bindings.sh                        # cancellation in the Swift and C# bindings
scripts/run-ios.sh --test                       # the Go tests in the iOS Simulator
go test -run Wasm .                             # under wasmtime, when it is installed
GOOS=js GOARCH=wasm go test -run InThisProcess \
    -exec="$(go env GOROOT)/lib/wasm/go_js_wasm_exec" .   # in node
scripts/run-plan9.sh --test                     # the Go tests on 9front, in an emulated VM
```

The Plan 9 run takes a few minutes and is worth it, since its lock is the
exclusive-use bit rather than `flock`, which no other system exercises.
The Swift generator's test in `cmd/vero` builds against a checkout whose
folder is named `vero`, since SwiftPM names a package after its folder.
One test fails when Go adds a target that the table in
[the lab](docs/lab.md) lacks.

Before sending a change, `gofmt -l .` prints nothing and `go vet ./...` is
quiet. The bindings are in [bindings](bindings) and [Sources](Sources), and
each one's header comment documents it. `scripts/build-all.sh` builds the
worker for every target in bold into `dist/`, and
[the Windows VM's notes](.windows/unattend/README.md) explain how that VM
is installed.
