# Start an app

This page makes a new vero app, or brings an app that you already have to
vero. Every app has the same layout, which the later guides assume.

## A new app

`vero new` makes a Go module that requires vero, with vero's example
worker in `cmd/worker` as a starting point. Then it does what `vero add` does.
It reads the worker and writes a starter front end for each system, which
shows the worker's state and has a control for each request.

```bash
vero new hello
cd hello
```

It asks for the three things that the packaging needs and the worker
cannot tell it, which are a one-line summary, a description, and your name
and email as the publisher. It writes them into `vero-app.toml`.
`--summary`, `--description` and `--publisher` give them from a script,
and `--module` gives the module a path such as `example.com/hello`.

Without the names of any systems, `vero new` writes the desktop front ends
for macOS, for Linux, the BSDs and illumos, and for Windows. `vero new
hello all` writes every front end that vero has, and
[`vero add`](../cmd/vero/README.md#vero-add) adds more later.

To run it on the Mac, run `./build.sh` in `macos/`, then the command that
it prints. The README in `gtk/` and the one in `windows/` say how to run
those front ends in a container and in a virtual machine.

## What is in the app's folder

| Path | What it is |
|---|---|
| `go.mod` | The app's Go module, which requires vero. |
| `cmd/worker/` | The worker. [Write the worker](worker.md) explains it. |
| `macos/` | The SwiftUI app for macOS. |
| `gtk/` | The GTK 4 app, in Python, for Linux, the BSDs and illumos. |
| `windows/` | The WPF app, in C#, for Windows. |
| `android/`, `ios/`, `web/`, `wasi/` and `plan9/` | The other front ends, when they are added. |
| `vero-app.toml` | The app's description, which the packaging and the tests read. [`vero-app.toml`](../PACKAGING.md) lists its settings. |
| `icon.png` | An icon to replace with your own. |
| `testdata/` | Files for the test's steps, which `[test]` in `vero-app.toml` lists. |
| `PORTING.md` | A checklist of what to do next, and of what in the worker needs attention on other systems. |
| `.vero-files.json` | The hash of each file that vero wrote, so that `vero upgrade` can tell which ones you have changed. It belongs in git. |

[The front ends](front-ends.md) explains each front end's folder.

## A window app, a tray app, or both

An app is a window app, a window app with a tray icon, or a tray app.

```bash
vero new hello              # a window app
vero new hello --tray       # a window app with an icon in the menu bar and the tray,
                            # which keeps running when its window is closed
vero new hello --menubar    # a tray app, which lives in the menu bar and the tray, and
                            # whose icon opens a small window, from which its main window opens
```

`vero add` takes the same flags. Each writes its setting, `tray = true` or
`menubar = true`, at the top of `vero-app.toml`, so that front ends added
later take the same shape. A tray app's Mac app has no Dock icon, except
while its main window is open, which `VeroDock` in vero's Swift package
arranges. Where a desktop shows no tray icons, such as GNOME without its
extension, a tray app opens as an ordinary window, and a window app that
would keep running says so before its window closes.

## An app from one Go file

`vero ship` goes from a single Go file to a released app in one command.

```bash
vero ship counter.go
```

It makes an app called `counter` from the file, in the layout above, with
front ends for the Mac, for Linux, the BSDs and illumos, and for Windows.
Then it tests the app on every system it can, records each one, and
releases it. [Write the worker](worker.md#a-worker-in-one-file) has a
worker with which to try it, and [Package and release](release.md#vero-ship)
says what `vero ship` does at each step.

## An app that you already have

An app whose worker is already a Go program takes the same layout. The
worker goes in `cmd/worker` of the app's own module, and `vero add` in the
module's folder writes the front ends and `vero-app.toml`.

An existing SwiftUI app keeps its own Xcode project. It uses vero's Swift
package, with the worker bundled in its Resources as `macos/` does, and
names its Xcode project in `[macos]` in `vero-app.toml`.
[The front ends](front-ends.md#a-mac-app-by-hand-in-xcode) shows how to
add vero to an Xcode project by hand.

`vero add` also writes `PORTING.md`, which says what in the worker needs
attention on the new systems. [Write the worker](worker.md#a-worker-for-every-system)
explains the usual causes.

Next, [write the worker](worker.md).
