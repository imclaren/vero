# The iOS example

The same three jobs as every other example, on a phone:

```bash
./build.sh          # builds, installs and launches in the Simulator
```

![the iOS example](../../docs/screenshots/ios.gif)

`scripts/run-ios.sh` does the same from the top of the checkout, and
`scripts/run-ios.sh --test` runs vero's own tests inside the Simulator.

## What is different here

Everywhere else, the frontend starts the worker as a separate program and
talks to it over a pipe. iOS forbids an application starting a program, so
this example does not: the worker is compiled into the application, and vero
runs it on a goroutine joined to the frontend by a pipe in memory.

That is the whole difference, and in Swift it is one line:

```swift
@StateObject private var vero = VeroModel<Status>(compiledInWorker: [])
```

against the macOS example's `VeroModel<Status>(bundledWorker: "worker", ...)`,
which names a program to copy out of the bundle and start. Above the pipe
nothing changes: the same `restartJob` request, the same reply, the same state
arriving as it changes.

Restarting goes away with the process boundary. A worker on a goroutine dies
when the application does, and there is nothing left to restart it into. So
does the single-worker lock: there is no second process to keep out.

## The files

| | |
|---|---|
| [Sources/IOSExample/ExampleApp.swift](Sources/IOSExample/ExampleApp.swift) | the frontend: a list, three rows, a Restart button each |
| [archive/worker.go](archive/worker.go) | the worker, registered with `vero.ServeInProcess` |
| [archive/shim.go](archive/shim.go) | a symbolic link to [../../cshim/main.go](../../cshim/main.go), unchanged |
| [build.sh](build.sh) | the archive for the Simulator, then the app, then `simctl` |

A C archive is built from one Go package, which is why `archive/` holds both:
the shim every frontend links, and this application's worker beside it. In
your own project that directory is your worker plus a copy of the shim.

## Running it on a phone

`build.sh` targets the Simulator, which needs no signing - that is why it runs
from a script here. For a device, build the archive with the device SDK:

```bash
CGO_ENABLED=1 GOOS=ios GOARCH=arm64 go build -buildmode=c-archive \
    -o libvero.a ./archive
```

and link it from an Xcode project with your own signing identity. Nothing else
changes; the Swift above is the same.

## What has been run, and where

`scripts/run-ios.sh --test` builds vero's test suite for the Simulator and runs
it there. All of it passes, including the tests that start a worker as a
separate program - because the Simulator is macOS underneath, and can. A phone
cannot, which is the reason the in-process mode exists, and those tests pass
here too. Nothing in this example has run on a physical device.
