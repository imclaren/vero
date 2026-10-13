# Ship an app from one Go file

## Before you start

You need:

- a Mac with Apple silicon;
- Homebrew, from [https://brew.sh](https://brew.sh);
- Go, with `brew install go`;
- Xcode, from the App Store;
- a Developer ID Application certificate in your keychain, from the Apple
  Developer Program, and a Developer ID Installer certificate as well if
  you want a `.pkg`;
- for Windows, Microsoft's Windows 11 ARM64 ISO in `~/vm/vero-windows/`.
  Microsoft will not serve it to a script, so download it yourself from
  [https://www.microsoft.com/en-us/software-download/windows11arm64](https://www.microsoft.com/en-us/software-download/windows11arm64).

Then install vero into Homebrew's folder of programs, which is already on
your `PATH`, and let it set up the rest:

```bash
GOBIN=$(brew --prefix)/bin go install github.com/imclaren/vero/cmd/vero@latest
vero setup
```

`vero setup` installs the tools that every system needs, such as colima,
Docker, qemu, the .NET SDK, makensis and ffmpeg. It makes the notarytool
profile named `vero`, for which it asks for your Apple ID, your team ID
and an app-specific password. Then it lists anything still missing.

## Write the worker

A worker is a Go program that keeps the app's state and answers its
requests. Every window, on every system, shows the state that the worker
pushes and sends the requests that it answers. This one counts:

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

A handler's context ends when the front end stops waiting for its reply,
as a window does when what asked has scrolled out of sight, and when the
front end goes away. A handler that does slow work watches `ctx.Done()` and
stops early. Swift cancels a request with its task, C# with a
`CancellationToken`, and Python and Kotlin with a `Cancel`.

## Ship it

```bash
vero ship counter.go
```

`vero ship` makes an app called `counter` from the file, with a window for
the Mac, for Linux, the BSDs and illumos, and for Windows. Before it does
anything else, it prints what it will build and test on this Mac, what it
will skip and why, and how long each virtual machine takes to make the
first time. Then it tests the app on every system it can, records each
one, and releases it. It stops at the first failure, and says where its
log is.

## What you get

| | |
|---|---|
| The app | `counter/`, a Go module with the worker in `cmd/worker`, a SwiftUI app in `macos/`, a GTK app in `gtk/`, a WPF app in `windows/`, an icon, and `vero-app.toml`, which describes it |
| Installers | a signed and notarised `.dmg` for the Mac, packages for Debian, Fedora, openSUSE, Arch, Alpine, Void, Chimera, FreeBSD, DragonFly, NetBSD, OpenBSD and illumos, and installers for Windows |
| A site | `dist/site`, plain files to put on any web server, with a page that offers each system its installer, and signed repositories through which every system updates the app |
| Recordings | each system's window as a GIF and a picture, in `~/.cache/vero/test-results/`, with one page of every result, which the site's page shows too |

## What it cannot do

- It cannot make an app for iOS, Android or the browser by itself, since
  each of those runs the worker inside the app. `vero add ios`, `vero add
  android` and `vero add web` write those front ends, and
  [PACKAGING.md](../PACKAGING.md) says how to release them.
- It does not choose where the site lives. Put its address in `site` in
  `vero-app.toml`, and `vero ship --upload user@host:/path` copies it to
  your server, or copy `dist/site` there yourself. Until then, the site is
  made for `http://localhost:8080`, to look at.
- It does not sign the Windows installers, which needs a certificate of
  your own. Until then, Windows asks people once whether to run them, as
  the site's page explains.

## After the first time

Change `counter/cmd/worker/main.go`, or the windows in `counter/macos`,
`counter/gtk` and `counter/windows`, and run `vero ship` in `counter/`.
Each release takes the next version. Add steps to `[test]` in
`vero-app.toml` so that each recording shows the app at work.
`vero ship --plan` says what it would do and stops, `--no-release` tests
without releasing, and `--only "macos debian"` tests on just those
systems.

## Bringing an existing app to vero

An app whose worker is already a Go program needs the same layout: the
worker in `cmd/worker` of the app's own module, and `vero add desktop` in
the module's folder for the front ends and `vero-app.toml`. An existing
SwiftUI app uses vero's Swift package with the worker bundled in its
Resources, as `macos/` does, and names its Xcode project in `[macos]`.
For updates, it adds the package's `VeroUpdates` product, whose
`VeroUpdater` is Sparkle set up for an app that people leave open: it
installs an update as soon as the app is not busy, rather than when the
app quits, and its `isBusy` holds the update back while the worker is in
the middle of something. Then `vero ship` works as above. [PACKAGING.md](../PACKAGING.md) covers
the settings in full.
