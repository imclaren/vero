# The bindings and the examples

This page lists vero's library for each language, which a front end uses
to start the worker and talk to it, and vero's example app on each
system. Each library's own header comment documents it in full.

## The bindings

| Language | Files | What it is |
|---|---|---|
| Go | [The `vero` package](https://pkg.go.dev/github.com/imclaren/vero) | The worker's side, and a supervisor for a front end written in Go, as on Plan 9, in a browser and with WASI. |
| Swift | The Swift package in [Sources](../Sources), with the products `Vero` and `VeroUpdates` | `VeroModel` and `VeroClient` start the bundled worker, or the one compiled into an iOS app, and publish its state to SwiftUI. `VeroSingleCopy`, `VeroDock` and the test step player are here too, and `VeroUpdates` is Sparkle for an app that people leave open. |
| Python | [`vero.py`](../bindings/python/vero.py) | Starts the worker and holds the conversation with it, for GTK or any other Python front end. |
| Python, for GTK | [`vero_gtk.py`](../bindings/python/vero_gtk.py) | The plumbing of a GTK 4 window on Linux, the BSDs and illumos, with libadwaita where it is installed and plain GTK 4 where it is not. |
| C# | [`Vero.cs`](../bindings/csharp/Vero.cs) | Starts the worker and holds the conversation with it, for any .NET front end. |
| C#, for WPF | [`VeroWpf.cs`](../bindings/csharp/VeroWpf.cs) and [`VeroWebView.cs`](../bindings/csharp/VeroWebView.cs) | The plumbing of a WPF window on Windows, and a sign-in page, safe HTML and a bridge to a page's scripts with WebView2. |
| Kotlin | [`Vero.kt`](../bindings/kotlin/Vero.kt) | Starts the worker and holds the conversation with it, for Android or anything else on the JVM. |

`vero add` copies the right files beside each front end, and
`vero upgrade` replaces them with each new vero, as
[The front ends](front-ends.md#the-starters) explains.

## The examples

Every example drives the same worker,
[example/worker](../example/worker/README.md), except the WASI one, whose
worker has to do its work inside its handlers.
[example/vero-app.toml](../example/vero-app.toml) describes all of them to
the packaging.

| Example | System | What it shows |
|---|---|---|
| [example/menubar-app](../example/menubar-app/README.md) | macOS | SwiftUI, in the menu bar. |
| [example/gtk-app](../example/gtk-app/README.md) | Linux, the BSDs and illumos | GTK 4 in Python, the same app unchanged on each. |
| [example/wpf-app](../example/wpf-app/README.md) | Windows | WPF in C#. |
| [example/android-app](../example/android-app/README.md) | Android | Kotlin, with the worker shipped as a native library and no Gradle. |
| [example/ios-app](../example/ios-app/README.md) | iOS | SwiftUI, with the worker compiled into the app. |
| [example/web-app](../example/web-app/README.md) | A browser | The front end and the worker in one wasm module. |
| [example/wasi-app](../example/wasi-app/README.md) | A terminal | A worker that is a `.wasm` file, run by wasmtime. |
| [example/plan9-app](../example/plan9-app/README.md) | Plan 9 | libdraw in a rio window, with Go on both sides. |

[Build, run and test on every system](test.md) runs each of them.
