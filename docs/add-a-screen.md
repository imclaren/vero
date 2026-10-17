# Add a screen

The starter that `vero new` writes shows every field of the state and a
button for every request. A real app replaces that with screens of its
own, one set per front end. This page is about doing that without losing
the thread between the three toolkits.

## The order of work

1. Write the screen on the Mac first, in SwiftUI. It is the quickest to
   build and run, and it settles what the screen needs from the state.
2. If the state or the requests need to change, change the worker, run
   `vero gen`, and the typed binding for every front end updates.
3. Write the same screen in GTK and in WPF, reading the same typed state
   from their generated files.

Each front end reads the state through its generated binding and sends
requests through it. The toolkits differ, but the ideas map across. The
table below is the crib sheet.

| Idea | SwiftUI | GTK 4, in Python | WPF, in C# |
|---|---|---|---|
| A window, or a menu bar / tray app | `WindowGroup`, `MenuBarExtra` | `vero_gtk.Window`, and `App(..., tray=True)` | `VeroWindow`, and `Tray = true` in `VeroAppOptions` |
| Asking the person something, or a toast | an alert or a toast | `vero_gtk.question`, `Window.toast` | `VeroDialogs.Question`, `VeroWindow.Toast` |
| Settings | `Settings` scene | `vero_gtk.Settings` | a window of your own, in XAML |
| A list | `List`, `ForEach` | `Gtk.ListBox`, or `Gtk.ListView` for many rows | `ListBox`, or `ItemsControl` with an `ItemTemplate` |
| Sidebar and detail | `NavigationSplitView` | `vero_gtk.split_view` | a `Grid` with a `GridSplitter` |
| A sheet | `.sheet` | `Gtk.Window` with `transient_for`, or `Adw.Dialog` | a `Window` shown with `ShowDialog` |
| Reacting to a new state | a model fed by `VeroModel` | `App.on_state`, called on the main loop | `VeroApp.OnState`, called on the window's thread |
| Embedded web content | `WKWebView` | `vero_gtk.HtmlView` | `VeroHtmlView`, in `VeroWebView.cs` |

## One thing to know about GTK

`vero_gtk.py` uses libadwaita where the system has it and plain GTK 4
where it does not. Some BSDs do not have libadwaita. A screen therefore
has to work without it, so use the `vero_gtk` helpers rather than `Adw`
widgets directly where you can.

## Related

- The starters in `macos/`, `gtk/` and `windows/` are the place to begin.
  They already read the state and send every request.
- [Add a request](add-a-request.md).
