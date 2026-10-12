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

## Window apps and tray apps

An app is a window app, a window app with a tray icon, or a tray app.

```bash
vero add                 # a window app
vero add --tray          # a window app with an icon in the menu bar and the tray,
                         # which keeps running when its window is closed
vero add --menubar       # a tray app: it lives in the menu bar and the tray, and its
                         # icon opens a small window, from which its main window opens
```

`vero new NAME` takes the same flags. Each writes its setting, `tray = true`
or `menubar = true`, at the top of `vero-app.toml`, so that front ends added
later take the same shape. A tray app's Mac app has no Dock icon, except
while its main window is open, which `VeroDock` in vero's Swift package
arranges. Where a desktop shows no tray icons, such as GNOME without its
extension, a tray app opens as an ordinary window, and a window app that
would keep running says so before its window closes.

The Windows and GTK starters are built on vero's libraries, `VeroWpf.cs`
and `vero_gtk.py`, copied beside them. These do the plumbing: the worker, a
single copy, the tray, banners and toasts, dialogs, notifications, light and
dark mode, and, on Linux, the BSDs and illumos, libadwaita where the system
has it and plain GTK 4 where it does not. A starter is then only how the app
looks. `vero gen` writes a typed method for each request beside each
starter, and `vero add` runs it.

It asks for a one-line summary, a description and the publisher, as
"Name <email>", when it first writes `vero-app.toml` at a terminal;
`--summary`, `--description` and `--publisher` answer from a script.

## vero upgrade

An app made with vero 0.13 or later is brought up to the vero you have with
`vero upgrade`, in its folder. It moves `go.mod` and the kit, the Mac app's
pin on vero's Swift package (Up to Next Major) and the Windows app (.NET 10,
with Windows Forms for the tray icon). It copies vero's files into each front
end afresh, writes `vero-app.toml`'s dependencies as `uses` and `extra`, runs
`vero gen`, and adds `WorkerAPI.swift` to an Xcode project that needs it.
Then it lists what it changed, and what to change in the app's own code,
such as a type of the app's own that `WorkerAPI.swift` now defines.

vero's files say at their top that vero wrote them, and `.vero-files.json`,
which belongs in git, records each one's hash. A file that has been changed
since is left alone, and `vero upgrade` names it: an app changes what vero's
libraries do through their settings or by subclassing. `--force` replaces
it all the same.

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
| `WindowGroup`, `MenuBarExtra` | `vero_gtk.Window`; `App(..., tray=True)` for a tray icon, `small_window=...` for a tray app | `VeroWindow`; `Tray = true` or `MenuBar = true` in `VeroAppOptions` |
| an alert or a toast | `vero_gtk.question`, `Window.toast` | `VeroDialogs.Question`, `VeroWindow.Toast` |
| settings | `vero_gtk.Settings`, libadwaita's preferences dialog where it is installed | a window of the app's own, in XAML |
| `List`, `ForEach` | `Gtk.ListBox` with a row per item, or `Gtk.ListView` for many | `ListBox` or `ItemsControl` with an `ItemTemplate` |
| `NavigationSplitView` | `vero_gtk.split_view`, libadwaita's where it is installed and a pane where not | a `Grid` with a `GridSplitter` |
| `.sheet` | `Gtk.Window` with `transient_for`, or `Adw.Dialog` where `vero_gtk.has_adwaita()` | a `Window` shown with `ShowDialog` |
| `@StateObject` model fed by events | `App.on_state`, which calls back on the main loop | `VeroApp.OnState`, which calls back on the window's thread |
| a `WKWebView` | `vero_gtk.HtmlView` and `SignInPage`, which fall back where there is no web view | `VeroHtmlView` and `VeroSignInPage`, in `VeroWebView.cs` |
