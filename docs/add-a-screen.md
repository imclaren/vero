# Add a screen

Write the screen on the Mac first, then the same screen in GTK and WPF.
Each front end reads the typed state from its generated file and sends
requests through it. These are the same ideas in each toolkit.

| SwiftUI | GTK 4, in Python | WPF, in C# |
|---|---|---|
| `WindowGroup`, `MenuBarExtra` | `vero_gtk.Window`, and `App(..., tray=True)` for a tray icon | `VeroWindow`, and `Tray = true` in `VeroAppOptions` |
| An alert or a toast | `vero_gtk.question`, `Window.toast` | `VeroDialogs.Question`, `VeroWindow.Toast` |
| Settings | `vero_gtk.Settings` | A window of the app's own, in XAML |
| `List`, `ForEach` | `Gtk.ListBox`, or `Gtk.ListView` for many rows | `ListBox` or `ItemsControl` with an `ItemTemplate` |
| `NavigationSplitView` | `vero_gtk.split_view` | A `Grid` with a `GridSplitter` |
| `.sheet` | `Gtk.Window` with `transient_for`, or `Adw.Dialog` | A `Window` shown with `ShowDialog` |
| A model fed by the state | `App.on_state`, called on the main loop | `VeroApp.OnState`, called on the window's thread |
| A `WKWebView` | `vero_gtk.HtmlView` | `VeroHtmlView`, in `VeroWebView.cs` |

`vero_gtk.py` uses libadwaita where the system has it and plain GTK 4 where
it does not, so a screen must work without it. The starters that `vero add`
wrote show every field of the state, and are the place to begin.
