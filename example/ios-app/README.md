# The iOS example

The same three jobs as every other example, on a phone.

```bash
./build.sh          # builds, installs and launches in the Simulator
```

![the iOS example](../../docs/screenshots/ios.gif)

`scripts/run-ios.sh`, from the top of the checkout, does the same, and
`scripts/run-ios.sh --test` runs vero's own tests inside the Simulator.

## What is different here

Everywhere else, the front end starts the worker as a separate program
and talks to it over a pipe. iOS forbids an app from starting a program.
So the worker is compiled into the app, and vero runs it on a goroutine
joined to the front end by a pipe in memory.

In Swift that is one line:

```swift
@StateObject private var vero = VeroModel<Status>(compiledInWorker: [])
```

The macOS example says `VeroModel<Status>(bundledWorker: "worker", ...)`
instead, naming a program to copy out of the bundle and start. Above the
pipe nothing changes: the same `restartJob` request, the same reply, the
same state arriving as it changes.

Two things disappear with the process boundary. Restarting: a worker on
a goroutine dies when the app does, and there is nothing to restart it
into. And the single-copy lock: there is no second process to keep out.

## The files

| | |
|---|---|
| [Sources/IOSExample/ExampleApp.swift](Sources/IOSExample/ExampleApp.swift) | the front end: a list, three rows, a Restart button each |
| [archive/worker.go](archive/worker.go) | the worker, registered with `vero.ServeInProcess` |
| [archive/shim.go](archive/shim.go) | a symbolic link to [../../cshim/main.go](../../cshim/main.go), unchanged |
| [build.sh](build.sh) | the archive for the Simulator, then the app, then `simctl` |

A C archive is built from one Go package, which is why `archive/` holds
both the shim that every Swift front end links and this app's worker. In
a project of your own, that folder is your worker plus a copy of the
shim.

## Running it on a phone

`build.sh` targets the Simulator, which needs no signing. For a real
device, build the archive with the device SDK:

```bash
CGO_ENABLED=1 GOOS=ios GOARCH=arm64 go build -buildmode=c-archive \
    -o libvero.a ./archive
```

and link it from an Xcode project with your own signing identity. The
Swift above does not change.

## What has been run, and where

`scripts/run-ios.sh --test` builds vero's test suite for the Simulator
and runs it there. All of it passes, including the tests that start a
worker as a separate program, because the Simulator is macOS underneath
and can. Nothing in this example has run on a physical device.
