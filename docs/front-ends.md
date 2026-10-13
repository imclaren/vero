# The front ends

This page explains the native half of a vero app, the front end, on each
system. It covers what `vero add` writes, the libraries on which those
front ends are built, and how to add vero to a Mac app by hand.

## One front end for each toolkit

| System | Toolkit and language | Folder that `vero add` writes | vero's library | vero's example |
|---|---|---|---|---|
| macOS | SwiftUI | `macos/` | The Swift package, `Vero` | [example/menubar-app](../example/menubar-app) |
| Linux, the BSDs and illumos | GTK 4, in Python | `gtk/` | [`vero.py`](../bindings/python/vero.py) and [`vero_gtk.py`](../bindings/python/vero_gtk.py) | [example/gtk-app](../example/gtk-app) |
| Windows | WPF, in C# | `windows/` | [`Vero.cs`](../bindings/csharp/Vero.cs), [`VeroWpf.cs`](../bindings/csharp/VeroWpf.cs) and [`VeroWebView.cs`](../bindings/csharp/VeroWebView.cs) | [example/wpf-app](../example/wpf-app) |
| Android | Kotlin | `android/` | [`Vero.kt`](../bindings/kotlin/Vero.kt) | [example/android-app](../example/android-app) |
| iOS | SwiftUI | `ios/` | The Swift package, `Vero` | [example/ios-app](../example/ios-app) |
| A browser | Go, compiled to wasm | `web/` | The Go package | [example/web-app](../example/web-app) |
| A terminal, with WASI | Go | `wasi/` | The Go package | [example/wasi-app](../example/wasi-app) |
| Plan 9 | Go, with libdraw | `plan9/` | The Go package | [example/plan9-app](../example/plan9-app) |

The same GTK app runs unchanged on Linux, the BSDs and illumos, since
nothing in it is specific to one of them. [The bindings and the
examples](bindings.md) describes each library and example.

## How a front end runs the worker

A front end starts the worker as a separate program and talks to it in
JSON over a pipe. The Python, C# and Kotlin bindings start it with
`VERO_HOST=1`, which makes it supervise a second copy of itself and answer
on its standard input and output. Restarts, backoff and the lock that
keeps to one worker at a time are all in the worker, so each binding is
only the conversation. There is no shared library and no cgo, so the
worker cross-compiles from the Mac for every system.

On iOS and in a browser, an app may not start a program, so the worker is
compiled into the app and runs on a goroutine, joined to the front end by
a pipe in memory. [Write the worker](worker.md#a-worker-inside-the-app)
explains how `vero add` arranges that.

## The starters

`vero add` reads the worker, meaning the state that it pushes and the
requests that it handles, and writes a starter for each system. Each
starter is a window that shows every field of the state and has a control
for every request, so that it runs against the worker as it is. Each
starter's README says how to build and run it, and vero's library for its
language is copied beside it. Front ends that you already have are left
alone, and each new one is added to `vero-app.toml`, so that a release
builds it.

The starters are a first draft to make your own, not a design. The shapes
of the state and the requests are written out in each language, for the
window that you build next.

The Windows and GTK starters are built on vero's libraries, `VeroWpf.cs`
and `vero_gtk.py`. These do the plumbing, which is the worker, a single
copy of the app, the tray, banners and toasts, dialogs, notifications, and
light and dark mode. On Linux, the BSDs and illumos, `vero_gtk.py` uses
libadwaita where the system has it and plain GTK 4 where it does not. A
starter is then only how the app looks. The Mac and iOS starters use
vero's Swift package in the same way.

vero's files say at their top that vero wrote them. An app changes what
vero's libraries do through their settings or by subclassing, rather than
by editing them, so that [`vero upgrade`](updates.md#moving-an-app-to-a-newer-vero)
can replace them with each new vero.

## From SwiftUI to the others

For a window of your own, these are the same ideas in each toolkit.

| SwiftUI | GTK 4 (Python) | WPF (C#) |
|---|---|---|
| `WindowGroup`, `MenuBarExtra` | `vero_gtk.Window`; `App(..., tray=True)` for a tray icon, `small_window=...` for a tray app | `VeroWindow`; `Tray = true` or `MenuBar = true` in `VeroAppOptions` |
| An alert or a toast | `vero_gtk.question`, `Window.toast` | `VeroDialogs.Question`, `VeroWindow.Toast` |
| Settings | `vero_gtk.Settings`, libadwaita's preferences dialog where it is installed | A window of the app's own, in XAML |
| `List`, `ForEach` | `Gtk.ListBox` with a row for each item, or `Gtk.ListView` for many | `ListBox` or `ItemsControl` with an `ItemTemplate` |
| `NavigationSplitView` | `vero_gtk.split_view`, libadwaita's where it is installed and a pane where it is not | A `Grid` with a `GridSplitter` |
| `.sheet` | `Gtk.Window` with `transient_for`, or `Adw.Dialog` where `vero_gtk.has_adwaita()` | A `Window` shown with `ShowDialog` |
| A `@StateObject` model fed by events | `App.on_state`, which calls back on the main loop | `VeroApp.OnState`, which calls back on the window's thread |
| A `WKWebView` | `vero_gtk.HtmlView` and `SignInPage`, which fall back where there is no web view | `VeroHtmlView` and `VeroSignInPage`, in `VeroWebView.cs` |

## A Mac app by hand, in Xcode

`vero new` writes the Mac app for you. This section adds vero to an Xcode
project of your own instead, with the example's worker and window, so
that you can see each part.

**1. Build the worker.** Make a Go module, and copy the example's worker
into it.

```bash
mkdir -p ~/vero-example/macos-app && cd ~/vero-example/macos-app
go mod init macos-app
go get github.com/imclaren/vero
curl -O https://raw.githubusercontent.com/imclaren/vero/main/example/worker/main.go
```

Build it for Intel and for Apple silicon, and join the two into one
program, so that the app runs on either. `lipo` comes with Xcode.

```bash
GOOS=darwin GOARCH=amd64 go build -o worker-amd64 . && \
GOOS=darwin GOARCH=arm64 go build -o worker-arm64 . && \
lipo -create worker-amd64 worker-arm64 -output worker
```

**2. Make the Xcode project.** In Xcode, choose File, New, Project, macOS,
App, with the interface set to SwiftUI. Then do two things.

- Choose File, Add Package Dependencies, and add
  `https://github.com/imclaren/vero`, with the dependency rule set to **Up
  to Next Major Version**. Before 1.0, Xcode's other rule, Up to Next Minor
  Version, would keep an app on 0.13 when 0.15 is out, so that it never
  gets what came after.
- Drag `~/vero-example/macos-app/worker` into the project, and tick your
  app under **Add to targets**.

**3. Write the app.** Replace the `ContentView.swift` and the
`<YourApp>App.swift` that Xcode made with the example's
[ExampleApp.swift](../example/menubar-app/Sources/MenuBarExample/ExampleApp.swift),
which is the `@main` entry point. Its main parts are these.

```swift
// What the worker pushes, in the same shape as Status and Job in main.go.
struct Job: Decodable, Identifiable {
    let id: Int
    let name: String
    let phase: String
    let progress: Int
}

struct Status: Decodable {
    let jobs: [Job]
    let working: Bool
    let since: String
}

// One of these for each handler on the worker. The name routes the
// request, and Reply says what comes back.
struct RestartJob: NamedRequest {
    static let name = "restartJob"
    typealias Reply = Status
    let id: Int
}

@main
struct ExampleApp: App {
    // One copy at a time. A second copy brings the first forward and quits.
    init() { VeroSingleCopy.yieldToRunningCopy() }

    // The worker, the last state that it pushed, and the reason it could
    // not start, if it did not.
    @StateObject private var vero = VeroModel<Status>(
        bundledWorker: "worker", directoryName: "Example/bin")

    var body: some Scene {
        MenuBarExtra {
            ForEach(vero.state?.jobs ?? []) { job in
                // The worker pushes the new state, which redraws the row.
                Button("Restart") { vero.call(RestartJob(id: job.id)) }
            }
        } label: {
            Image(systemName: vero.state?.working == true
                  ? "arrow.triangle.2.circlepath" : "checkmark.circle")
        }
        .menuBarExtraStyle(.window)
    }
}
```

**4. Run it.** Press Cmd-R in Xcode. The icon appears in the menu bar, and
spins while the worker has jobs in flight. Click it to see each job's
progress, and click a row's button to restart that job.

[example/menubar-app](../example/menubar-app) is the same app as a Swift
package, which `./build.sh` builds without Xcode's editor.

## The other systems by hand

The examples' READMEs build each front end outside vero's repository, in
the same way.

- [example/gtk-app](../example/gtk-app/README.md) builds the GTK app and
  runs it in a container on your Mac.
- [example/wpf-app](../example/wpf-app/README.md) builds the WPF app and
  runs it in a Windows virtual machine on your Mac.
- [example/android-app](../example/android-app/README.md),
  [example/ios-app](../example/ios-app/README.md),
  [example/web-app](../example/web-app/README.md),
  [example/wasi-app](../example/wasi-app/README.md) and
  [example/plan9-app](../example/plan9-app/README.md) explain what is
  different on each of those systems.

Next, [build, run and test on every system](test.md).
