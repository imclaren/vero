# Write the worker

This page explains the worker, the Go half of a vero app, and what makes
it build for every system. The worker keeps the app's state and answers
its requests. Every window, on every system, shows the state that the
worker pushes and sends the requests that it answers.

## A worker in one file

This worker counts. It is a whole app's logic, and `vero ship counter.go`
makes an app from it, as [Start an app](start.md#an-app-from-one-go-file)
describes.

```go
package main

import (
	"flag"

	"github.com/imclaren/vero"
)

var version = "0.1.0"

// Status is what every window shows. The worker pushes it on each change.
type Status struct {
	Count int `json:"count"`
}

// By is what the add request takes.
type By struct {
	N int `json:"n"`
}

func main() {
	var opts vero.WorkerOptions
	opts.Version = version
	opts.RegisterFlags(flag.CommandLine)
	flag.Parse()
	opts.PrintVersionAndExit()

	w := vero.NewWorker(opts)
	state := vero.NewState(w, Status{})
	vero.Update(state, "status", func(*Status) error { return nil })
	vero.UpdateWith(state, "add", func(s *Status, req By) error {
		s.Count += req.N
		return nil
	})
	if err := w.Serve(); err != nil {
		w.Log("stopped: %v", err)
	}
}
```

The worker that `vero new` starts from is
[example/worker/main.go](../example/worker/main.go), which pretends to
sync three folders. It shows a list of items that requests find by their
ID, and work that changes the state in the background.

## The parts of a worker

- **The version.** `var version` has to increase with each release, so
  that a front end can tell which of two copies of the worker is newer.
  The release sets it when it builds the worker.
- **The flags.** `vero.WorkerOptions` registers `-json`, which puts the
  event stream on standard output, `-quiet`, which silences the log, and
  `-version`, which a front end asks before it replaces the copy of the
  worker that it has on disk.
- **The state.** `vero.NewState` holds the app's state. vero sends it to
  every front end whenever its JSON changes, so that no front end has to
  ask. `state.Do` changes it from the worker's own work, and `state.Every`
  changes it at an interval.
- **The requests.** `vero.Update` handles a request that carries no data,
  and `vero.UpdateWith` one that does. Each changes the state, and the
  reply is the new state. `vero.UpdateItem` changes one item of a list,
  found by its `Key`, and `vero.Edit` does the same inside a handler.
  `vero.Handle` answers a request with a reply of its own, rather than the
  state. A handler that returns an error sends it to the front end and
  changes nothing.
- **Serving.** `w.Serve` answers the front end until it goes away, and
  `w.Log` writes to the worker's log.

The [package documentation](https://pkg.go.dev/github.com/imclaren/vero)
describes each function.

A handler holds the state's lock while it runs, so slow work belongs in a
goroutine that the handler starts, which writes back through `state.Do`. A
handler made with `vero.Handle` takes a context, which ends when the front
end stops waiting for its reply, as a window does when what asked for it
has scrolled out of sight, and when the front end goes away. A handler
that does slow work watches `ctx.Done()` and stops early. Swift cancels a
request with its task, C# with a `CancellationToken`, and Python and
Kotlin with a `Cancel`.

## The front ends' types

`vero gen` reads the worker's types and writes a typed method for each
request beside each front end, in Swift, C# and Python. `vero add` runs it
for you. Run it again after you change a request or the state, and
`vero gen --check` fails when a file is out of date.

## A worker for every system

vero builds the worker without cgo, for every system and processor for
which the app is packaged, so that one Mac can build them all. `vero ship`
and `vero-repo plan` build it for each system before anything else, and
name each one that fails and the error that stops it. Three causes are
usual.

- **SQLite.** `mattn/go-sqlite3` needs cgo, and fails as soon as it opens
  a database without it. `modernc.org/sqlite` has no port for DragonFly,
  illumos, NetBSD on arm64 or OpenBSD on riscv64. `ncruces/go-sqlite3`
  builds everywhere, and runs on every Linux processor for which vero
  packages, which vero tested under emulation. Full-text search is an
  extension there, which `github.com/ncruces/go-sqlite3/ext/fts5`
  registers on each connection that `driver.Open` makes. A sandboxed Mac
  app's worker also needs `PRAGMA temp_store=MEMORY`, because SQLite
  cannot make its temporary files there, and a write that needs one, such
  as one that updates a search index, fails with "disk I/O error".
- **A package for one system**, such as a notifier for the Mac's sleep and
  wake that needs cgo. vero's [kit](../kit/README.md) has packages that
  work on every system, for the keychain, notifications, starting at
  sign-in, sleep and updates. Give the rest a build tag, so that other
  systems get a version that does nothing or does the work another way.
- **A system call that differs**, such as `syscall.Rlimit`, which Windows
  lacks and 32-bit systems type differently. `golang.org/x/sys` has each
  system's own.

`vero add` writes `PORTING.md`, which lists what in the worker needs
attention on the new systems, found by reading its code, with the files
and lines and the change to make in each. It finds cgo, programs that the
worker starts, which have to be shipped for each system and which iOS and
a browser forbid, a port on which it listens, which it cannot do inside an
app on iOS or in a browser, and secrets, sign-in items and notifications
done in one system's way, with the kit package to use instead.

A worker that needs cgo on the Mac, for the keychain, say, sets
`cgo = true` in `[worker]` in `vero-app.toml`. The Mac's worker is then
built with Xcode's clang, and every other system's without cgo, as before.

## A worker inside the app

On iOS and in a browser, an app may not start a program, so the worker
runs inside the app, on a goroutine. `vero add ios` and `vero add web`
arrange that by moving the worker's code into `internal/worker`, a package
that those two front ends call, and leaving the command as a few lines
that start it. That works for a worker shaped like the ones above, whose
`main` makes a `vero.WorkerOptions`, calls `vero.NewWorker` and ends with
`Serve`. For any other shape, `vero add` says so and leaves a placeholder,
with a note that says what it should call. `--keep-worker` asks for the
placeholder in any case.

Next, [the front ends](front-ends.md).
