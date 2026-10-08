# vero, the tool

`vero add` writes a front end for each system, `vero release` builds the
installers and the site, and `vero credentials` is the checklist for
signing and the stores. [PACKAGING.md](../../PACKAGING.md) is the guide;
this is what `vero add` does in detail. `release`, `credentials`,
`publish`, `key`, `package`, `build` and `check` are handed on to
[vero-repo](../vero-repo/README.md), which `vero` installs the first time
it needs it.

```bash
go install github.com/imclaren/vero/cmd/vero@latest
```

## What vero add writes

In your app's folder, the one with its `go.mod`:

```bash
vero add                 # the desktop: macOS, GTK (Linux, the BSDs, illumos) and WPF (Windows)
vero add mobile          # Android and iOS
vero add all             # everything vero has a front end for
vero add all --except plan9,wasi
vero add windows android # any mix of groups and systems
```

It reads your worker - the state it pushes and the requests it handles -
and writes a starter for each system: a window that shows every field of
the state and a control for every request, so that it runs against your
worker as it is. Each starter's README says how to build and run it, with
vero's binding for that language copied beside it. Front ends you already
have are left alone, and each new one is added to `vero-app.toml` (made
if you have none), so that `vero release` builds them.

The starters are a first draft to make your own, not a design: the GTK one
is Python, the Windows one C#, the macOS and iOS ones SwiftUI, the Android
one Kotlin, the browser, WASI and Plan 9 ones Go. The shapes of your state
and requests are written out in each language, for the window you build
next.

It asks for a one-line summary, a description and the publisher, as
"Name <email>", when it first writes `vero-app.toml` at a terminal;
`--summary`, `--description` and `--publisher` answer from a script.

## PORTING.md

`vero add` also writes `PORTING.md`: what in your worker needs attention
on the new systems, found by reading its code. cgo, which stops the worker
being cross-compiled from one Mac; programs it starts, which have to be
shipped for each system and which iOS and a browser forbid; listening on a
port, which the worker cannot do inside an app on iOS or in a browser;
secrets, sign-in items and notifications done one system's way, with the
`kit` package to use instead. Each with the files and lines.

On iOS and in a browser the worker runs inside the app, since neither
lets an app start a program. `vero add` arranges that: it moves the
worker's code into `internal/worker`, a package those two front ends
call, and leaves the command as a few lines that start it. That works
for a worker shaped like vero's example - a `main` that makes a
`vero.WorkerOptions`, calls `vero.NewWorker` and ends with `Serve`. For
any other shape it says so and leaves a placeholder, with a note on what
to point it at; `--keep-worker` asks for that in any case.

## From SwiftUI to the others

For a front end of your own, the same ideas in each toolkit:

| SwiftUI | GTK 4 (Python) | WPF (C#) |
|---|---|---|
| `WindowGroup`, `MenuBarExtra` | `Gtk.ApplicationWindow`; a status icon needs an extension on GNOME, so a window is the usual choice | `Window`; a tray icon with `NotifyIcon` from Windows Forms |
| `List`, `ForEach` | `Gtk.ListBox` with a row per item, or `Gtk.ListView` for many | `ListBox` or `ItemsControl` with an `ItemTemplate` |
| `NavigationSplitView` | `Gtk.Paned` with a sidebar `Gtk.ListBox` | a `Grid` with a `GridSplitter` |
| `.sheet`, `.alert` | `Gtk.Dialog`, `Gtk.AlertDialog` | a `Window` shown with `ShowDialog`, `MessageBox` |
| `@StateObject` model fed by events | `run_in_thread` from `vero.py` with `GLib.idle_add` | `await foreach` over `VeroClient.Events()` and `Dispatcher.Invoke` |
| a `WKWebView` | `WebKit.WebView` from WebKitGTK 6 | `WebView2` |
