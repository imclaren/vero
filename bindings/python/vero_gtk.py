"""vero's GTK library: what every vero app's window on Linux, the BSDs and
illumos needs, so that each app writes only how it looks.

Written by vero. Do not edit it: vero upgrade replaces it with each
version of vero, and stops if it has been changed. An app that needs
something different asks for it through the arguments here, or subclasses.

It uses libadwaita 1.5 or later where it is installed, and plain GTK 4
where it is not, as on Debian 12, OpenBSD on ARM and POWER, and illumos
on SmartOS's packages. Every piece works both ways, and works without a
D-Bus session bus, which some BSD and illumos desktops start without, and
without a web view. It needs GTK 4.8 and PyGObject 3.42, as Debian 12 has,
and Python 3.9, as OpenIndiana's PyGObject is for. VERO_GTK_PLAIN=1 makes it use plain GTK 4 where
libadwaita is installed, for testing.

    from vero_gtk import App, Window

    class Main(Window):
        def __init__(self, app):
            super().__init__(app, "counter")
            self.set_content(...)
            app.on_state(self.apply)

    App("com.example.counter", "counter", "counter-worker", window=Main).run()

Everything the worker is asked goes through App.ask, which asks on a thread
and answers on the main loop, so that a slow request never holds up the
window.
"""

from __future__ import annotations

import html
import json
import os
import re
import socket
import sys
import threading
from typing import Any, Callable

import gi

gi.require_version("Gtk", "4.0")
from gi.repository import Gio, GLib, GObject, Gtk, Pango  # noqa: E402

from vero import Cancel, Cancelled, NotRunning, Refused, Vero, VeroError  # noqa: E402

Adw: Any = None
if not os.environ.get("VERO_GTK_PLAIN"):
    try:
        gi.require_version("Adw", "1")
        from gi.repository import Adw as _Adw

        if (_Adw.get_major_version(), _Adw.get_minor_version()) >= (1, 5):
            Adw = _Adw
    except (ValueError, ImportError):
        pass

GTK_VERSION = (Gtk.get_major_version(), Gtk.get_minor_version())

__all__ = [
    "Adw", "App", "Cancel", "Window", "SmallWindow", "Settings", "UndoStack", "HtmlView", "HTML_POLICY", "PdfWriter",
    "SignInPage",
    "has_adwaita", "has_session_bus", "has_webview", "webkit", "web_bridge",
    "choose_files", "save_file", "choose_folder", "question", "ask_text",
    "open_uri", "show_in_folder", "status_page", "avatar", "split_view", "label",
]


def has_adwaita() -> bool:
    """Whether libadwaita 1.5 or later is in use. An app that uses a widget
    only libadwaita has asks this first."""
    return Adw is not None


_bus: Any = False


def has_session_bus() -> bool:
    """Whether there is a D-Bus session bus, which the tray, notifications
    and GTK's own single copy need."""
    global _bus
    if _bus is False:
        try:
            _bus = Gio.bus_get_sync(Gio.BusType.SESSION, None)
        except GLib.Error:
            _bus = None
    return _bus is not None


_webkit: Any = False


def webkit() -> Any:
    """WebKitGTK's GTK 4 module (WebKit 6.0), or None where it is not
    installed, as on NetBSD, OpenBSD i386 and arm, and SmartOS."""
    global _webkit
    if _webkit is False:
        try:
            gi.require_version("WebKit", "6.0")
            from gi.repository import WebKit

            _webkit = WebKit
        except (ValueError, ImportError):
            _webkit = None
    return _webkit


def has_webview() -> bool:
    return webkit() is not None


def label(text: str = "", css: str = "", wrap: bool = False, select: bool = False, xalign: float = 0) -> Gtk.Label:
    """A label, left-aligned unless told otherwise, with GTK's style classes
    in css, such as "dim-label", "heading" or "title-2"."""
    lbl = Gtk.Label(label=text, xalign=xalign, wrap=wrap, selectable=select)
    if wrap:
        lbl.set_wrap_mode(Pango.WrapMode.WORD_CHAR)
    for c in css.split():
        lbl.add_css_class(c)
    return lbl


# Plain GTK's stand-ins for libadwaita's banners, toasts, avatars and
# status pages, drawn to look much like them.
_CSS = """
.vero-banner { background: alpha(@accent_bg_color, 0.15); padding: 6px 12px; }
.vero-banner.warning { background: alpha(#e5a50a, 0.2); }
.vero-toast { background: alpha(#303030, 0.95); color: white; border-radius: 99px; padding: 6px 6px 6px 18px; margin: 12px; }
.vero-toast button { color: white; }
.vero-avatar { border-radius: 9999px; background: alpha(@accent_bg_color, 0.6); color: white; font-weight: bold; }
.vero-status-icon { opacity: 0.5; }
"""
_css_loaded = False


def _load_css() -> None:
    global _css_loaded
    if _css_loaded or Gdk_display() is None:
        return
    _css_loaded = True
    provider = Gtk.CssProvider()
    if hasattr(provider, "load_from_string"):
        provider.load_from_string(_CSS)
    else:
        provider.load_from_data(_CSS.encode())
    Gtk.StyleContext.add_provider_for_display(Gdk_display(), provider, Gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)


def Gdk_display() -> Any:
    from gi.repository import Gdk

    return Gdk.Display.get_default()


# Opening things.

def open_uri(uri: str, parent: Gtk.Window | None = None) -> None:
    """Opens a web page, a file or a folder in what the desktop uses for it.
    Under Flatpak, GIO asks the portal."""
    if not uri:
        return
    if "://" not in uri and not uri.startswith(("mailto:", "file:")):
        uri = GLib.filename_to_uri(os.path.abspath(uri), None)
    try:
        Gio.AppInfo.launch_default_for_uri_async(uri, None, None, None)
    except GLib.Error as e:
        print(f"vero_gtk: could not open {uri}: {e.message}", file=sys.stderr)


def show_in_folder(path: str) -> None:
    """Shows a file in its folder, chosen, through the file manager's
    ShowItems over D-Bus; without it, opens the folder."""
    uri = GLib.filename_to_uri(os.path.abspath(path), None)
    if has_session_bus():
        try:
            _bus.call_sync("org.freedesktop.FileManager1", "/org/freedesktop/FileManager1",
                           "org.freedesktop.FileManager1", "ShowItems", GLib.Variant("(ass)", ([uri], "")),
                           None, Gio.DBusCallFlags.NONE, 3000, None)
            return
        except GLib.Error:
            pass
    open_uri(os.path.dirname(os.path.abspath(path)))


# Dialogs. Each takes a done callback, which is not called when the person
# cancels.

def _filters(filters: list[tuple[str, list[str]]] | None) -> list[Gtk.FileFilter]:
    out = []
    for name, patterns in filters or []:
        f = Gtk.FileFilter(name=name)
        for p in patterns:
            if "/" in p:
                f.add_mime_type(p)
            else:
                f.add_pattern(p)
        out.append(f)
    return out


def _native(parent: Gtk.Window | None, title: str, action: Gtk.FileChooserAction, accept: str,
            done: Callable[[Gtk.FileChooserNative], None], filters: list | None = None,
            multiple: bool = False, name: str = "") -> None:
    chooser = Gtk.FileChooserNative(title=title, transient_for=parent, action=action, accept_label=accept,
                                    modal=True, select_multiple=multiple)
    for f in _filters(filters):
        chooser.add_filter(f)
    if name:
        chooser.set_current_name(name)

    def answered(d: Gtk.FileChooserNative, response: int) -> None:
        if response == Gtk.ResponseType.ACCEPT:
            done(d)
        d.destroy()

    chooser.connect("response", answered)
    chooser.show()
    # Kept alive until answered: GTK holds no reference to it.
    _keep.add(chooser)
    chooser.connect("response", lambda d, _r: _keep.discard(d))


_keep: set = set()


def _dialog(title: str, filters: list | None, name: str = "") -> Gtk.FileDialog:
    d = Gtk.FileDialog(title=title, modal=True)
    fs = _filters(filters)
    if fs:
        store = Gio.ListStore(item_type=Gtk.FileFilter)
        for f in fs:
            store.append(f)
        d.set_filters(store)
    if name:
        d.set_initial_name(name)
    return d


def choose_files(parent: Gtk.Window | None, title: str, done: Callable[[list[str]], Any],
                 multiple: bool = False, filters: list[tuple[str, list[str]]] | None = None) -> None:
    """Asks for one file, or several, and gives done their paths. filters
    are (name, patterns), where a pattern is "*.zip" or a type such as
    "image/png"."""
    if GTK_VERSION >= (4, 10):
        d = _dialog(title, filters)

        def got(d: Gtk.FileDialog, result: Gio.AsyncResult) -> None:
            try:
                if multiple:
                    files = d.open_multiple_finish(result)
                    done([files.get_item(i).get_path() for i in range(files.get_n_items())])
                else:
                    done([d.open_finish(result).get_path()])
            except GLib.Error:
                pass

        (d.open_multiple if multiple else d.open)(parent, None, got)
        return

    def chosen(c: Gtk.FileChooserNative) -> None:
        files = c.get_files()
        done([files.get_item(i).get_path() for i in range(files.get_n_items())])

    _native(parent, title, Gtk.FileChooserAction.OPEN, "Open", chosen, filters, multiple)


def save_file(parent: Gtk.Window | None, title: str, name: str, done: Callable[[str], Any],
              filters: list[tuple[str, list[str]]] | None = None) -> None:
    """Asks where to save a file, suggesting name, and gives done the path."""
    if GTK_VERSION >= (4, 10):
        d = _dialog(title, filters, name)

        def got(d: Gtk.FileDialog, result: Gio.AsyncResult) -> None:
            try:
                done(d.save_finish(result).get_path())
            except GLib.Error:
                pass

        d.save(parent, None, got)
        return
    _native(parent, title, Gtk.FileChooserAction.SAVE, "Save", lambda c: done(c.get_file().get_path()),
            filters, name=name)


def choose_folder(parent: Gtk.Window | None, title: str, done: Callable[[str], Any]) -> None:
    """Asks for a folder, and gives done its path."""
    if GTK_VERSION >= (4, 10):
        d = _dialog(title, None)

        def got(d: Gtk.FileDialog, result: Gio.AsyncResult) -> None:
            try:
                done(d.select_folder_finish(result).get_path())
            except GLib.Error:
                pass

        d.select_folder(parent, None, got)
        return
    _native(parent, title, Gtk.FileChooserAction.SELECT_FOLDER, "Choose", lambda c: done(c.get_file().get_path()))


def question(parent: Gtk.Window | None, heading: str, body: str, buttons: list[str],
             done: Callable[[int], Any] | None = None, default: int = -1, destructive: int = -1) -> None:
    """Asks a question with buttons, the first being Cancel or its like,
    and gives done the index of the button pressed. Closing the dialog is
    the first button."""
    done = done or (lambda _i: None)
    if Adw is not None:
        d = Adw.AlertDialog(heading=heading, body=body)
        for i, b in enumerate(buttons):
            d.add_response(str(i), b)
        if default >= 0:
            d.set_default_response(str(default))
            d.set_response_appearance(str(default), Adw.ResponseAppearance.SUGGESTED)
        if destructive >= 0:
            d.set_response_appearance(str(destructive), Adw.ResponseAppearance.DESTRUCTIVE)
        d.set_close_response("0")
        d.connect("response", lambda _d, r: done(int(r)))
        d.present(parent)
        return
    d = Gtk.MessageDialog(transient_for=parent, modal=True, message_type=Gtk.MessageType.QUESTION,
                          buttons=Gtk.ButtonsType.NONE, text=heading, secondary_text=body or None)
    for i, b in enumerate(buttons):
        button = d.add_button(b, i)
        if i == destructive:
            button.add_css_class("destructive-action")
        elif i == default:
            button.add_css_class("suggested-action")
    if default >= 0:
        d.set_default_response(default)

    def answered(d: Gtk.MessageDialog, response: int) -> None:
        d.destroy()
        done(response if response >= 0 else 0)

    d.connect("response", answered)
    d.present()


def ask_text(parent: Gtk.Window | None, heading: str, body: str, done: Callable[[str], Any],
             ok: str = "OK", initial: str = "", password: bool = False, placeholder: str = "") -> None:
    """Asks for some text, or a password, and gives done what was typed."""
    entry: Gtk.Widget
    if password:
        entry = Gtk.PasswordEntry(show_peek_icon=True, activates_default=True, placeholder_text=placeholder)
    else:
        entry = Gtk.Entry(text=initial, activates_default=True, placeholder_text=placeholder)
    entry.set_hexpand(True)
    if Adw is not None:
        d = Adw.AlertDialog(heading=heading, body=body)
        d.set_extra_child(entry)
        d.add_response("cancel", "Cancel")
        d.add_response("ok", ok)
        d.set_response_appearance("ok", Adw.ResponseAppearance.SUGGESTED)
        d.set_default_response("ok")
        d.set_close_response("cancel")
        d.connect("response", lambda _d, r: r == "ok" and done(entry.get_text()))
        d.present(parent)
        return
    d = Gtk.Dialog(title=heading, transient_for=parent, modal=True)
    d.add_button("Cancel", Gtk.ResponseType.CANCEL)
    d.add_button(ok, Gtk.ResponseType.OK).add_css_class("suggested-action")
    d.set_default_response(Gtk.ResponseType.OK)
    box = d.get_content_area()
    box.set_spacing(10)
    for side in ("top", "bottom", "start", "end"):
        getattr(box, f"set_margin_{side}")(18)
    if body:
        box.append(label(body, wrap=True))
    box.append(entry)

    def answered(d: Gtk.Dialog, response: int) -> None:
        text = entry.get_text()
        d.destroy()
        if response == Gtk.ResponseType.OK:
            done(text)

    d.connect("response", answered)
    d.present()


# Widgets that libadwaita has, with a plain GTK form.

def status_page(icon: str, title: str, description: str = "", child: Gtk.Widget | None = None) -> Gtk.Widget:
    """An empty state: a large icon, a title, a line or two, and perhaps a
    button."""
    if Adw is not None:
        return Adw.StatusPage(icon_name=icon, title=title, description=description, child=child, vexpand=True)
    _load_css()
    box = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=12, valign=Gtk.Align.CENTER,
                  halign=Gtk.Align.CENTER, vexpand=True, margin_start=24, margin_end=24)
    image = Gtk.Image(icon_name=icon, pixel_size=96)
    image.add_css_class("vero-status-icon")
    box.append(image)
    box.append(label(title, "title-1", wrap=True, xalign=0.5))
    if description:
        box.append(label(description, "dim-label", wrap=True, xalign=0.5))
    if child:
        box.append(child)
    return box


def avatar(name: str, size: int = 32, picture: str = "") -> Gtk.Widget:
    """A round picture, or the initials of name."""
    if Adw is not None:
        a = Adw.Avatar(size=size, text=name, show_initials=True)
        if picture and os.path.exists(picture):
            from gi.repository import Gdk

            a.set_custom_image(Gdk.Texture.new_from_filename(picture))
        return a
    _load_css()
    if picture and os.path.exists(picture):
        p = Gtk.Picture.new_for_filename(picture)
        p.set_size_request(size, size)
        p.add_css_class("vero-avatar")
        p.set_overflow(Gtk.Overflow.HIDDEN)
        return p
    initials = "".join(w[0] for w in name.split()[:2]).upper() or "?"
    lbl = label(initials, xalign=0.5)
    lbl.set_size_request(size, size)
    lbl.add_css_class("vero-avatar")
    lbl.set_valign(Gtk.Align.CENTER)
    return lbl


def split_view(sidebar: Gtk.Widget, content: Gtk.Widget, sidebar_width: int = 260) -> Gtk.Widget:
    """A sidebar beside the content: libadwaita's, which folds on a narrow
    window, or a pane."""
    if Adw is not None:
        view = Adw.NavigationSplitView(min_sidebar_width=sidebar_width)
        view.set_sidebar(Adw.NavigationPage(child=sidebar, title="Sidebar"))
        view.set_content(Adw.NavigationPage(child=content, title="Content"))
        return view
    return Gtk.Paned(start_child=sidebar, end_child=content, position=sidebar_width,
                     shrink_start_child=False, resize_start_child=False)


class Settings:
    """A settings window of pages, each of groups of rows: libadwaita's
    preferences dialog, or a plain window with a sidebar.

        s = Settings(window, "Settings")
        g = s.page("General", "preferences-system-symbolic").group("Syncing")
        g.switch("Sync automatically", "Every few minutes", on, lambda on: ...)
        g.row("Account", "you@example.com", Gtk.Button(label="Sign Out"))
        s.present()
    """

    def __init__(self, parent: Gtk.Window, title: str = "Settings") -> None:
        self.parent = parent
        if Adw is not None:
            self.dialog = Adw.PreferencesDialog(title=title, search_enabled=False)
            return
        self.dialog = Gtk.Window(title=title, transient_for=parent, modal=True, default_width=640,
                                 default_height=520)
        self.stack = Gtk.Stack()
        sidebar = Gtk.StackSidebar(stack=self.stack)
        sidebar.set_size_request(160, -1)
        box = Gtk.Box()
        box.append(sidebar)
        box.append(self.stack)
        self.stack.set_hexpand(True)
        self.dialog.set_child(box)

    def page(self, title: str, icon: str = "") -> "Settings.Page":
        return Settings.Page(self, title, icon)

    def present(self) -> None:
        if Adw is not None:
            self.dialog.present(self.parent)
        else:
            self.dialog.present()

    def close(self) -> None:
        self.dialog.close()

    class Page:
        def __init__(self, settings: "Settings", title: str, icon: str) -> None:
            if Adw is not None:
                self.widget = Adw.PreferencesPage(title=title, icon_name=icon or None)
                settings.dialog.add(self.widget)
                return
            self.box = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=18, margin_top=18,
                               margin_bottom=18, margin_start=18, margin_end=18)
            settings.stack.add_titled(Gtk.ScrolledWindow(child=self.box, hscrollbar_policy=Gtk.PolicyType.NEVER),
                                      title, title)

        def group(self, title: str = "", description: str = "") -> "Settings.Group":
            return Settings.Group(self, title, description)

    class Group:
        def __init__(self, page: "Settings.Page", title: str, description: str) -> None:
            if Adw is not None:
                self.widget = Adw.PreferencesGroup(title=title, description=description)
                page.widget.add(self.widget)
                return
            box = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=6)
            if title:
                box.append(label(title, "heading"))
            if description:
                box.append(label(description, "dim-label", wrap=True))
            self.list = Gtk.ListBox(selection_mode=Gtk.SelectionMode.NONE)
            self.list.add_css_class("boxed-list")
            self.list.add_css_class("frame")
            box.append(self.list)
            page.box.append(box)

        def add(self, widget: Gtk.Widget) -> None:
            """Adds any widget as a row of its own."""
            if Adw is not None:
                self.widget.add(widget)
            else:
                self.list.append(widget)

        def row(self, title: str, subtitle: str = "", suffix: Gtk.Widget | None = None) -> Gtk.Widget:
            """A row with a title, a line under it, and a widget at its end."""
            if Adw is not None:
                r = Adw.ActionRow(title=GLib.markup_escape_text(title), subtitle=GLib.markup_escape_text(subtitle))
                if suffix is not None:
                    suffix.set_valign(Gtk.Align.CENTER)
                    r.add_suffix(suffix)
                self.widget.add(r)
                return r
            box = Gtk.Box(spacing=12, margin_top=8, margin_bottom=8, margin_start=12, margin_end=12)
            text = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, hexpand=True, valign=Gtk.Align.CENTER)
            text.append(label(title, wrap=True))
            if subtitle:
                text.append(label(subtitle, "dim-label caption", wrap=True))
            box.append(text)
            if suffix is not None:
                suffix.set_valign(Gtk.Align.CENTER)
                box.append(suffix)
            self.list.append(box)
            return box

        def switch(self, title: str, subtitle: str, on: bool, changed: Callable[[bool], Any]) -> Gtk.Widget:
            """A row with a switch, which tells changed when it is turned."""
            if Adw is not None:
                r = Adw.SwitchRow(title=GLib.markup_escape_text(title), subtitle=GLib.markup_escape_text(subtitle),
                                  active=on)
                r.connect("notify::active", lambda r, _p: changed(r.get_active()))
                self.widget.add(r)
                return r
            s = Gtk.Switch(active=on)
            s.connect("notify::active", lambda s, _p: changed(s.get_active()))
            self.row(title, subtitle, s)
            return s

        def button(self, title: str, subtitle: str, text: str, pressed: Callable[[], Any],
                   destructive: bool = False) -> Gtk.Button:
            b = Gtk.Button(label=text)
            if destructive:
                b.add_css_class("destructive-action")
            b.connect("clicked", lambda *_: pressed())
            self.row(title, subtitle, b)
            return b


# The shell.

class Window((Adw.ApplicationWindow if Adw is not None else Gtk.ApplicationWindow)):  # type: ignore[misc]
    """The app's window: a header bar, a banner for problems and one for a
    newer version, toasts, and the app's content under them.

    With libadwaita it is an Adw.ApplicationWindow with an Adw.ToolbarView;
    without, a Gtk.ApplicationWindow with a Gtk.HeaderBar as its title bar.
    Either way, put buttons in self.header and the content in
    set_content().
    """

    def __init__(self, app: "App", title: str, width: int = 900, height: int = 640) -> None:
        super().__init__(application=app, title=title)
        _load_css()
        self.app = app
        self.set_default_size(width, height)
        self.header = Adw.HeaderBar() if Adw is not None else Gtk.HeaderBar()
        self._update_link = ""
        self._problem = self._banner("", "", "warning")
        self._update = self._banner("", "Download", "")
        self._update_handler = None

        column = Gtk.Box(orientation=Gtk.Orientation.VERTICAL)
        column.append(self._problem["widget"])
        column.append(self._update["widget"])
        self._content = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, vexpand=True)
        column.append(self._content)
        if Adw is not None:
            self._toasts = Adw.ToastOverlay(child=column)
            view = Adw.ToolbarView(content=self._toasts)
            view.add_top_bar(self.header)
            super().set_content(view)
        else:
            self.set_titlebar(self.header)
            self._toasts = Gtk.Overlay(child=column)
            self._toast_box = Gtk.Revealer(transition_type=Gtk.RevealerTransitionType.SLIDE_UP,
                                           valign=Gtk.Align.END, halign=Gtk.Align.CENTER)
            self._toasts.add_overlay(self._toast_box)
            self._toast_timer = 0
            self.set_child(self._toasts)
        self._update["button"](lambda: open_uri(self._update_link))
        self.connect("close-request", self._closing)

    def set_content(self, widget: Gtk.Widget) -> None:  # type: ignore[override]
        """The window's own content, under the header bar and banners."""
        while child := self._content.get_first_child():
            self._content.remove(child)
        widget.set_vexpand(True)
        self._content.append(widget)

    def _banner(self, text: str, button: str, kind: str) -> dict:
        if Adw is not None:
            b = Adw.Banner(title=text, revealed=False)
            if button:
                b.set_button_label(button)
            return {"widget": b,
                    "show": lambda t: (b.set_title(GLib.markup_escape_text(t)), b.set_revealed(bool(t))),
                    "button": lambda do: b.connect("button-clicked", lambda *_: do())}
        revealer = Gtk.Revealer(transition_type=Gtk.RevealerTransitionType.SLIDE_DOWN)
        row = Gtk.Box(spacing=12)
        row.add_css_class("vero-banner")
        if kind:
            row.add_css_class(kind)
        text_label = label(text, wrap=True, xalign=0.5)
        text_label.set_hexpand(True)
        row.append(text_label)
        btn = Gtk.Button(label=button, visible=bool(button), valign=Gtk.Align.CENTER)
        row.append(btn)
        revealer.set_child(row)
        return {"widget": revealer,
                "show": lambda t: (text_label.set_text(t), revealer.set_reveal_child(bool(t))),
                "button": lambda do: btn.connect("clicked", lambda *_: do())}

    def problem(self, text: str) -> bool:
        """Shows text in the banner at the top, or hides it when text is
        empty. Returns False, so that it can end a GLib.idle_add."""
        self._problem["show"](text)
        return False

    def show_update(self, text: str, link: str) -> None:
        """Shows a newer version, with a Download button that opens link."""
        self._update_link = link
        self._update["show"](text)

    def toast(self, text: str, button: str = "", pressed: Callable[[], Any] | None = None,
              seconds: int = 5) -> Callable[[], None]:
        """Shows a short message at the bottom of the window, with a button
        such as Undo, for seconds (0 keeps it until it is dismissed).
        Returns a function that dismisses it."""
        if Adw is not None:
            t = Adw.Toast(title=GLib.markup_escape_text(text), timeout=seconds)
            if button:
                t.set_button_label(button)
                t.connect("button-clicked", lambda *_: pressed and pressed())
            self._toasts.add_toast(t)
            return t.dismiss
        row = Gtk.Box(spacing=12)
        row.add_css_class("vero-toast")
        row.append(label(text, wrap=True))
        if button:
            b = Gtk.Button(label=button)
            b.add_css_class("flat")
            b.connect("clicked", lambda *_: (self._hide_toast(), pressed and pressed()))
            row.append(b)
        close = Gtk.Button(icon_name="window-close-symbolic")
        close.add_css_class("flat")
        close.add_css_class("circular")
        close.connect("clicked", lambda *_: self._hide_toast())
        row.append(close)
        self._toast_box.set_child(row)
        self._toast_box.set_reveal_child(True)
        if self._toast_timer:
            GLib.source_remove(self._toast_timer)
            self._toast_timer = 0
        if seconds:
            self._toast_timer = GLib.timeout_add_seconds(seconds, self._toast_over)
        mine = row
        return lambda: self._toast_box.get_child() is mine and self._hide_toast()

    def _toast_over(self) -> bool:
        self._toast_timer = 0
        self._hide_toast()
        return False

    def _hide_toast(self) -> None:
        if self._toast_timer:
            GLib.source_remove(self._toast_timer)
            self._toast_timer = 0
        self._toast_box.set_reveal_child(False)

    def _closing(self, *_: Any) -> bool:
        """Closing the window of an app that keeps running hides it. Where
        nothing could bring it back, closing it says that the app stops."""
        return self.app._window_closing(self)


class SmallWindow(Window):
    """The small window that a tray app's icon opens, as the Mac's menu bar
    window: of the size the app asks for, with no header bar buttons but
    its own. Where there is no tray it is the app's ordinary window."""

    def __init__(self, app: "App", title: str, width: int = 360, height: int = 480) -> None:
        super().__init__(app, title, width, height)
        self.set_resizable(False)


# The tray: a StatusNotifierItem and its menu, over D-Bus, written here so
# that no app needs libappindicator.

_SNI_XML = """
<node>
  <interface name="org.kde.StatusNotifierItem">
    <property name="Category" type="s" access="read"/>
    <property name="Id" type="s" access="read"/>
    <property name="Title" type="s" access="read"/>
    <property name="Status" type="s" access="read"/>
    <property name="IconName" type="s" access="read"/>
    <property name="IconThemePath" type="s" access="read"/>
    <property name="ToolTip" type="(sa(iiay)ss)" access="read"/>
    <property name="ItemIsMenu" type="b" access="read"/>
    <property name="Menu" type="o" access="read"/>
    <method name="Activate"><arg type="i" direction="in"/><arg type="i" direction="in"/></method>
    <method name="SecondaryActivate"><arg type="i" direction="in"/><arg type="i" direction="in"/></method>
    <method name="ContextMenu"><arg type="i" direction="in"/><arg type="i" direction="in"/></method>
    <method name="Scroll"><arg type="i" direction="in"/><arg type="s" direction="in"/></method>
    <signal name="NewIcon"/>
    <signal name="NewToolTip"/>
    <signal name="NewTitle"/>
    <signal name="NewStatus"><arg type="s"/></signal>
  </interface>
</node>
"""

_MENU_XML = """
<node>
  <interface name="com.canonical.dbusmenu">
    <property name="Version" type="u" access="read"/>
    <property name="TextDirection" type="s" access="read"/>
    <property name="Status" type="s" access="read"/>
    <property name="IconThemePath" type="as" access="read"/>
    <method name="GetLayout">
      <arg type="i" direction="in"/><arg type="i" direction="in"/><arg type="as" direction="in"/>
      <arg type="u" direction="out"/><arg type="(ia{sv}av)" direction="out"/>
    </method>
    <method name="GetGroupProperties">
      <arg type="ai" direction="in"/><arg type="as" direction="in"/><arg type="a(ia{sv})" direction="out"/>
    </method>
    <method name="GetProperty">
      <arg type="i" direction="in"/><arg type="s" direction="in"/><arg type="v" direction="out"/>
    </method>
    <method name="Event">
      <arg type="i" direction="in"/><arg type="s" direction="in"/><arg type="v" direction="in"/><arg type="u" direction="in"/>
    </method>
    <method name="EventGroup">
      <arg type="a(isvu)" direction="in"/><arg type="ai" direction="out"/>
    </method>
    <method name="AboutToShow"><arg type="i" direction="in"/><arg type="b" direction="out"/></method>
    <method name="AboutToShowGroup">
      <arg type="ai" direction="in"/><arg type="ai" direction="out"/><arg type="ai" direction="out"/>
    </method>
    <signal name="LayoutUpdated"><arg type="u"/><arg type="i"/></signal>
    <signal name="ItemsPropertiesUpdated"><arg type="a(ia{sv})"/><arg type="a(ias)"/></signal>
  </interface>
</node>
"""

_WATCHER = "org.kde.StatusNotifierWatcher"


class _Tray:
    """The app's tray icon. It shows only where a tray is: KDE, Xfce and
    most other desktops, and GNOME with its AppIndicator extension, which
    Ubuntu installs. shown says whether it is, now."""

    def __init__(self, app: "App", icon: str, title: str, activate: Callable[[], None]) -> None:
        self.app = app
        self.title = title
        self.tooltip = title
        self.activate = activate
        self.items: list[tuple[str, Callable[[], Any] | None]] = []
        self.revision = 1
        self.shown = False
        self.changed: Callable[[bool], None] = lambda _shown: None
        self.icon_name, self.icon_path = icon, ""
        if os.path.isabs(icon):
            self.icon_path = os.path.dirname(icon)
            self.icon_name = os.path.splitext(os.path.basename(icon))[0]
        self.bus = _bus
        self.name = f"org.kde.StatusNotifierItem-{os.getpid()}-1"
        sni = Gio.DBusNodeInfo.new_for_xml(_SNI_XML).interfaces[0]
        menu = Gio.DBusNodeInfo.new_for_xml(_MENU_XML).interfaces[0]
        self.bus.register_object("/StatusNotifierItem", sni, self._sni_call, self._sni_get, None)
        self.bus.register_object("/MenuBar", menu, self._menu_call, self._menu_get, None)
        Gio.bus_own_name_on_connection(self.bus, self.name, Gio.BusNameOwnerFlags.NONE, None, None)
        Gio.bus_watch_name_on_connection(self.bus, _WATCHER, Gio.BusNameWatcherFlags.NONE,
                                         self._watcher_up, self._watcher_down)

    def _watcher_up(self, *_: Any) -> None:
        try:
            self.bus.call_sync(_WATCHER, "/StatusNotifierWatcher", _WATCHER, "RegisterStatusNotifierItem",
                               GLib.Variant("(s)", (self.name,)), None, Gio.DBusCallFlags.NONE, 3000, None)
            self._set_shown(True)
        except GLib.Error as e:
            print(f"vero_gtk: the tray refused the icon: {e.message}", file=sys.stderr)
            self._set_shown(False)

    def _watcher_down(self, *_: Any) -> None:
        self._set_shown(False)

    def _set_shown(self, shown: bool) -> None:
        if shown != self.shown:
            self.shown = shown
            self.changed(shown)

    def _emit(self, path: str, iface: str, signal: str, args: GLib.Variant | None = None) -> None:
        try:
            self.bus.emit_signal(None, path, iface, signal, args)
        except GLib.Error:
            pass

    def set_icon(self, icon: str) -> None:
        if os.path.isabs(icon):
            self.icon_path = os.path.dirname(icon)
            icon = os.path.splitext(os.path.basename(icon))[0]
        self.icon_name = icon
        self._emit("/StatusNotifierItem", "org.kde.StatusNotifierItem", "NewIcon")

    def set_tooltip(self, text: str) -> None:
        self.tooltip = text
        self._emit("/StatusNotifierItem", "org.kde.StatusNotifierItem", "NewToolTip")

    def set_menu(self, items: list[tuple[str, Callable[[], Any] | None]]) -> None:
        self.items = items
        self.revision += 1
        self._emit("/MenuBar", "com.canonical.dbusmenu", "LayoutUpdated", GLib.Variant("(ui)", (self.revision, 0)))

    def _sni_get(self, _c: Any, _s: str, _p: str, _i: str, prop: str) -> GLib.Variant | None:
        values = {
            "Category": GLib.Variant("s", "ApplicationStatus"),
            "Id": GLib.Variant("s", self.app.get_application_id() or self.title),
            "Title": GLib.Variant("s", self.title),
            "Status": GLib.Variant("s", "Active"),
            "IconName": GLib.Variant("s", self.icon_name),
            "IconThemePath": GLib.Variant("s", self.icon_path),
            "ToolTip": GLib.Variant("(sa(iiay)ss)", ("", [], self.title, self.tooltip)),
            "ItemIsMenu": GLib.Variant("b", False),
            "Menu": GLib.Variant("o", "/MenuBar"),
        }
        return values.get(prop)

    def _sni_call(self, _c: Any, _s: str, _p: str, _i: str, method: str, _params: GLib.Variant,
                  invocation: Gio.DBusMethodInvocation) -> None:
        if method in ("Activate", "SecondaryActivate"):
            GLib.idle_add(lambda: self.activate() and False)
        invocation.return_value(None)

    def _menu_items(self) -> list[tuple[int, str, Callable[[], Any] | None]]:
        return [(i + 1, text, do) for i, (text, do) in enumerate(self.items)]

    def _props(self, ident: int) -> dict:
        if ident == 0:
            return {"children-display": GLib.Variant("s", "submenu")}
        for i, text, do in self._menu_items():
            if i == ident:
                if text == "-":
                    return {"type": GLib.Variant("s", "separator")}
                return {"label": GLib.Variant("s", text.replace("_", "__")), "enabled": GLib.Variant("b", do is not None)}
        return {}

    def _menu_get(self, _c: Any, _s: str, _p: str, _i: str, prop: str) -> GLib.Variant | None:
        return {"Version": GLib.Variant("u", 3), "TextDirection": GLib.Variant("s", "ltr"),
                "Status": GLib.Variant("s", "normal"), "IconThemePath": GLib.Variant("as", [])}.get(prop)

    def _menu_call(self, _c: Any, _s: str, _p: str, _i: str, method: str, params: GLib.Variant,
                   invocation: Gio.DBusMethodInvocation) -> None:
        args = params.unpack()
        if method == "GetLayout":
            children = [GLib.Variant("(ia{sv}av)", (i, self._props(i), [])) for i, _t, _d in self._menu_items()]
            invocation.return_value(GLib.Variant("(u(ia{sv}av))", (self.revision, (0, self._props(0), children))))
        elif method == "GetGroupProperties":
            ids = args[0] or [i for i, _t, _d in self._menu_items()]
            invocation.return_value(GLib.Variant("(a(ia{sv}))", ([(i, self._props(i)) for i in ids],)))
        elif method == "GetProperty":
            value = self._props(args[0]).get(args[1])
            if value is None:
                invocation.return_dbus_error("com.canonical.dbusmenu.Error", "No such property.")
            else:
                invocation.return_value(GLib.Variant("(v)", (value,)))
        elif method == "Event":
            self._event(args[0], args[1])
            invocation.return_value(None)
        elif method == "EventGroup":
            for ident, event, _data, _time in args[0]:
                self._event(ident, event)
            invocation.return_value(GLib.Variant("(ai)", ([],)))
        elif method == "AboutToShow":
            invocation.return_value(GLib.Variant("(b)", (False,)))
        elif method == "AboutToShowGroup":
            invocation.return_value(GLib.Variant("(aiai)", ([], [])))
        else:
            invocation.return_dbus_error("org.freedesktop.DBus.Error.UnknownMethod", method)

    def _event(self, ident: int, event: str) -> None:
        if event != "clicked":
            return
        for i, _text, do in self._menu_items():
            if i == ident and do is not None:
                GLib.idle_add(lambda d=do: d() and False)


# Single copy without a session bus: a lock and a socket in the runtime
# folder. With a bus, GApplication does this itself.

def _runtime_dir() -> str:
    folder = os.environ.get("XDG_RUNTIME_DIR") or os.path.join(GLib.get_tmp_dir(), f"vero-{os.getuid()}")
    os.makedirs(folder, mode=0o700, exist_ok=True)
    return folder


class _LocalCopy:
    """The first copy listens; a second sends it its command line and
    stops."""

    def __init__(self, app_id: str) -> None:
        self.path = os.path.join(_runtime_dir(), app_id + ".sock")
        self.listener: socket.socket | None = None

    def forward(self, argv: list[str]) -> bool:
        """Sends argv to a copy already running, and says whether one was."""
        try:
            with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as s:
                s.settimeout(3)
                s.connect(self.path)
                s.sendall((json.dumps({"argv": argv, "cwd": os.getcwd()}) + "\n").encode())
                return True
        except OSError:
            return False

    def listen(self, received: Callable[[list[str], str], None]) -> None:
        try:
            os.unlink(self.path)
        except FileNotFoundError:
            pass
        self.listener = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.listener.bind(self.path)
        os.chmod(self.path, 0o600)
        self.listener.listen(4)

        def serve() -> None:
            while True:
                try:
                    conn, _ = self.listener.accept()
                except OSError:
                    return
                with conn:
                    data = b""
                    while not data.endswith(b"\n"):
                        chunk = conn.recv(65536)
                        if not chunk:
                            break
                        data += chunk
                try:
                    msg = json.loads(data)
                except ValueError:
                    continue
                GLib.idle_add(lambda m=msg: received(m.get("argv", []), m.get("cwd", "")) and False)

        threading.Thread(target=serve, daemon=True).start()

    def close(self) -> None:
        if self.listener:
            self.listener.close()
            try:
                os.unlink(self.path)
            except OSError:
                pass


# The app.

_Base = Adw.Application if Adw is not None else Gtk.Application


class App(_Base):  # type: ignore[misc, valid-type]
    """The app: the worker, a single copy, the window, and, for an app that
    keeps running, the tray, starting at sign-in and running in the
    background.

    app_id is the reverse-domain id, name what people see, and worker the
    worker's program, beside the app's own files unless it is a path.

    window is a function, or a Window subclass, that makes the main window
    from the app. A tray app (menubar=True) gives small_window instead, the
    small window its tray icon opens, and may give window too, for a main
    window opened from the small one. tray=True gives a window app a tray
    icon and keeps it running when its window is closed, as treehouse does
    while it syncs.

    opened is told what the app was asked to open: links, such as mailto:
    or the app's own scheme, and files, as given on the command line or
    from the file manager, whether to this copy or to a second one, which
    passes them on and stops.
    """

    def __init__(self, app_id: str, name: str, worker: str, arguments: list[str] | None = None, *,
                 window: Callable[["App"], Gtk.Window] | None = None,
                 small_window: Callable[["App"], Gtk.Window] | None = None,
                 tray: bool = False, icon: str = "", opened: Callable[[list[str]], Any] | None = None) -> None:
        flags = Gio.ApplicationFlags.HANDLES_COMMAND_LINE
        self._local: _LocalCopy | None = None
        if not has_session_bus():
            flags |= Gio.ApplicationFlags.NON_UNIQUE
            self._local = _LocalCopy(app_id)
        super().__init__(application_id=app_id, flags=flags)
        GLib.set_application_name(name)
        self.name = name
        here = os.path.dirname(os.path.abspath(sys.argv[0] or __file__))
        self.worker_path = worker if os.path.isabs(worker) else os.path.join(here, worker)
        self.arguments = arguments or []
        self.make_window = window
        self.make_small_window = small_window
        self.tray_app = small_window is not None
        self.keeps_running = tray or self.tray_app
        self.icon = icon or app_id
        self.opened = opened
        self.vero: Vero | None = None
        self.window: Gtk.Window | None = None
        self.small: Gtk.Window | None = None
        self.tray: _Tray | None = None
        self.state: Any = None
        self._watchers: list[Callable[[Any], Any]] = []
        self._dark_watchers: list[Callable[[bool], Any]] = []
        self._actions: dict[str, Callable[[], Any]] = {}
        self._held = False
        self._quitting = False
        self._background = False
        self.undo = UndoStack(self)

    # Starting.

    def run(self, argv: list[str] | None = None) -> int:  # type: ignore[override]
        argv = list(sys.argv if argv is None else argv)
        if self._local is not None:
            if self._local.forward(argv[1:]):
                return 0
            self._local.listen(lambda args, cwd: self._command(args, cwd, remote=True))
        try:
            return super().run(argv)
        finally:
            if self._local is not None:
                self._local.close()

    def do_startup(self) -> None:
        _Base.do_startup(self)
        _load_css()
        self.vero = Vero(self.worker_path, self.arguments)
        threading.Thread(target=self._read_events, daemon=True).start()
        self._watch_dark()
        self._add_action("notification", self._notification_pressed, "s")
        self._add_action("undo", lambda: self._undo_key("undo"))
        self._add_action("redo", lambda: self._undo_key("redo"))
        self._add_action("quit", self.quit_app)
        self.set_accels_for_action("app.undo", ["<Control>z"])
        self.set_accels_for_action("app.redo", ["<Control><Shift>z", "<Control>y"])
        if self.keeps_running:
            self.hold()
            self._held = True
            if has_session_bus():
                self.tray = _Tray(self, self.icon, self.name, self._tray_pressed)
                self.tray.set_menu(self._tray_items([]))
                self.tray.changed = lambda shown: self._tray_changed(shown)
            self._ask_background()

    def do_command_line(self, cmdline: Gio.ApplicationCommandLine) -> int:
        args = cmdline.get_arguments()[1:]
        self._command(args, cmdline.get_cwd() or "", remote=cmdline.get_is_remote())
        return 0

    def _command(self, args: list[str], cwd: str, remote: bool) -> None:
        background = "--background" in args
        things = []
        for a in args:
            if a.startswith("--"):
                continue
            if re.match(r"^[a-zA-Z][a-zA-Z0-9+.-]*:", a) and not os.path.exists(a):
                things.append(a)
            else:
                things.append(os.path.join(cwd, a) if cwd and not os.path.isabs(a) else a)
        # Started at sign-in: the tray only, and no window, unless no tray
        # appears to open it from. The tray registers a moment after start.
        if background and not remote and self.keeps_running:
            GLib.timeout_add_seconds(3, lambda: (not self._tray_possible() and self.activate()) and False)
        else:
            self.activate()
        if things and self.opened:
            self.opened(things)

    def do_activate(self) -> None:
        # A tray app's small window is also its ordinary window where there
        # is no tray.
        if self.tray_app:
            self.show_small_window()
        else:
            self.show_window()

    def show_window(self) -> Gtk.Window:
        """Shows the main window, making it the first time."""
        if self.window is None:
            self.window = self.make_window(self)  # type: ignore[misc]
        self.window.present()
        return self.window

    def show_small_window(self) -> Gtk.Window:
        """Shows a tray app's small window."""
        if self.small is None:
            self.small = (self.make_small_window or self.make_window)(self)  # type: ignore[misc]
        self.small.present()
        return self.small

    def active(self) -> Gtk.Window | None:
        """The window to show a problem or a dialog in."""
        return self.get_active_window() or self.window or self.small

    def _undo_key(self, which: str) -> None:
        """Ctrl+Z: in a text field, the field's own undo; elsewhere, the
        worker's."""
        window = self.active()
        focus = window.get_focus() if window is not None else None
        if isinstance(focus, (Gtk.Text, Gtk.TextView)):
            focus.activate_action(f"text.{which}", None)
        else:
            getattr(self.undo, which)()

    # Keeping running.

    def _tray_possible(self) -> bool:
        """Whether anything could bring a hidden window back: a tray icon,
        or, under Flatpak, the background portal's list of apps."""
        return self._tray_shown() or (has_session_bus() and os.path.exists("/.flatpak-info"))

    def _tray_shown(self) -> bool:
        return self.tray is not None and self.tray.shown

    def _tray_changed(self, shown: bool) -> None:
        # A tray that went away while only it showed the app: show a window.
        if not shown and not self._quitting and not any(w.get_visible() for w in self.get_windows()):
            self.activate()

    def _tray_pressed(self) -> None:
        if self.tray_app:
            if self.small is not None and self.small.get_visible() and self.small.is_active():
                self.small.set_visible(False)
            else:
                self.show_small_window()
        else:
            self.show_window()

    def _tray_items(self, items: list[tuple[str, Callable[[], Any] | None]]) -> list:
        opener = self.show_small_window if self.tray_app else self.show_window
        own = list(items) + (["-"] if items else [])
        return [i if i != "-" else ("-", None) for i in own] + [
            (f"Open {self.name}", opener), ("-", None), (f"Quit {self.name}", self.quit_app)]

    def set_tray(self, icon: str | None = None, tooltip: str | None = None,
                 items: list[tuple[str, Callable[[], Any] | None]] | None = None) -> None:
        """Changes the tray icon's picture (an icon name, or a path to a
        PNG or SVG), its tooltip, or its menu. items are (text, function)
        pairs, with a function of None for a line that cannot be chosen
        and "-" for a separator. Open and Quit are added after them."""
        if self.tray is None:
            return
        if icon is not None:
            self.tray.set_icon(icon)
        if tooltip is not None:
            self.tray.set_tooltip(tooltip)
        if items is not None:
            self.tray.set_menu(self._tray_items(items))

    def _window_closing(self, window: Gtk.Window) -> bool:
        if self._quitting or not self.keeps_running:
            return False
        if window is self.small and self.tray_app and self._tray_shown():
            window.set_visible(False)
            return True
        if window is self.window and self._tray_possible():
            window.set_visible(False)
            return True
        if window is self.window and self.tray_app and self.small is not None and self.small.get_visible():
            window.set_visible(False)
            return True
        # Nothing could bring the app back: say that closing stops it.
        question(window, f"Quit {self.name}?",
                 f"There is no tray icon on this desktop, so {self.name} stops when its window closes.",
                 ["Cancel", "Quit"], lambda i: i == 1 and self.quit_app(), default=1)
        return True

    def quit_app(self) -> None:
        """Quits, whatever is open."""
        self._quitting = True
        for w in list(self.get_windows()):
            w.destroy()
        if self._held:
            self.release()
            self._held = False
        self.quit()

    def do_shutdown(self) -> None:
        if self.vero is not None:
            self.vero.stop()
        _Base.do_shutdown(self)

    def _ask_background(self) -> None:
        """Under Flatpak, asks the background portal to let the app keep
        running with no window."""
        if not (has_session_bus() and os.path.exists("/.flatpak-info")):
            return
        options = {"reason": GLib.Variant("s", f"{self.name} keeps working with its window closed."),
                   "autostart": GLib.Variant("b", False)}
        _bus.call("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop",
                  "org.freedesktop.portal.Background", "RequestBackground", GLib.Variant("(sa{sv})", ("", options)),
                  None, Gio.DBusCallFlags.NONE, -1, None, None, None)

    # Starting at sign-in.

    def _autostart_file(self) -> str:
        return os.path.join(GLib.get_user_config_dir(), "autostart", self.get_application_id() + ".desktop")

    def opens_at_login(self) -> bool:
        return os.path.exists(self._autostart_file())

    def set_opens_at_login(self, on: bool) -> str:
        """Opens the app at sign-in, in the background for an app that keeps
        running. Returns what went wrong, or an empty string."""
        if os.path.exists("/.flatpak-info") and has_session_bus():
            options = {"reason": GLib.Variant("s", f"{self.name} opens when you sign in."),
                       "autostart": GLib.Variant("b", on),
                       "commandline": GLib.Variant("as", [self._command_name(), "--background"])}
            try:
                _bus.call_sync("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop",
                               "org.freedesktop.portal.Background", "RequestBackground",
                               GLib.Variant("(sa{sv})", ("", options)), None, Gio.DBusCallFlags.NONE, -1, None)
            except GLib.Error as e:
                return e.message
            # The portal keeps the autostart entry; ours records the choice.
        path = self._autostart_file()
        try:
            if on:
                os.makedirs(os.path.dirname(path), exist_ok=True)
                command = f"{GLib.shell_quote(sys.executable)} {GLib.shell_quote(os.path.abspath(sys.argv[0]))}"
                if self.keeps_running:
                    command += " --background"
                with open(path, "w") as f:
                    f.write(f"[Desktop Entry]\nType=Application\nName={self.name}\nExec={command}\n"
                            f"Icon={self.get_application_id()}\nX-GNOME-Autostart-enabled=true\n")
            elif os.path.exists(path):
                os.remove(path)
        except OSError as e:
            return str(e)
        return ""

    def _command_name(self) -> str:
        return os.path.basename(sys.argv[0]).removesuffix(".py")

    # The worker.

    def _read_events(self) -> None:
        assert self.vero is not None
        try:
            latest = self.vero.latest()
            if latest is not None:
                GLib.idle_add(self._got_state, latest)
        except VeroError:
            pass
        for state in self.vero.events():
            GLib.idle_add(self._got_state, state)

    def _got_state(self, state: Any) -> bool:
        self.state = state
        for watch in list(self._watchers):
            watch(state)
        return False

    def on_state(self, watch: Callable[[Any], Any]) -> None:
        """Calls watch on the main loop with each state the worker pushes,
        starting with the latest, if there is one yet."""
        self._watchers.append(watch)
        if self.state is not None:
            watch(self.state)

    def ask(self, request: str | Callable[[], Any], payload: Any = None,
            done: Callable[[Any], Any] | None = None, failed: Callable[[str], Any] | None = None,
            cancel: Cancel | None = None) -> None:
        """Asks the worker on a thread, and calls done with the reply on the
        main loop. request is a handler's name, with payload, or a function
        that asks, such as lambda: api.rename(id=3, title="new") with
        vero gen's Worker. A refusal goes to failed, or to the window's
        banner. cancel, a vero.Cancel, cancels the request when its cancel()
        is called, as for a picture that has scrolled out of sight: then
        neither done nor failed is called, and the worker stops the work."""
        assert self.vero is not None
        vero = self.vero

        def run() -> None:
            try:
                if cancel is not None:
                    with cancel:
                        reply = request() if callable(request) else vero.call(request, payload or {})
                else:
                    reply = request() if callable(request) else vero.call(request, payload or {})
            except Cancelled:
                return
            except (Refused, NotRunning, VeroError) as e:
                words = str(e)
                GLib.idle_add(lambda: (failed or self.problem)(words) and False)
                return
            if done is not None:
                GLib.idle_add(lambda: done(reply) and False)

        threading.Thread(target=run, daemon=True).start()

    def problem(self, text: str) -> bool:
        """Shows text in the active window's banner."""
        window = self.active()
        if isinstance(window, Window):
            window.problem(text)
        else:
            print(f"{self.name}: {text}", file=sys.stderr)
        return False

    def check_update(self, done: Callable[[str], Any] | None = None) -> None:
        """Asks the worker, through kit/update's standard request, whether a
        newer version is out, and shows it in the main window's banner.
        done, when given, is told the answer in words."""

        def got(u: Any) -> None:
            u = u or {}
            if u.get("newer"):
                text = f"{self.name} {u.get('latest', '')} is out."
                if sys.platform.startswith("linux") and not os.path.exists("/.flatpak-info"):
                    text += " Your system's updates bring it if you added its repository."
                for w in (self.window, self.small):
                    if isinstance(w, Window):
                        w.show_update(text, u.get("url", ""))
                done and done(f"Version {u.get('latest', '')} is out.")
            else:
                done and done("This is the newest version.")

        self.ask("vero.update", {}, got, lambda e: done and done(f"Could not check for updates. {e}"))

    # Sign-ins where there is no keychain.

    def unlock_sign_ins(self, done: Callable[[], Any] | None = None) -> None:
        """Where the system has no keychain, the worker's kit/keychain keeps
        sign-ins in files encrypted with a password: this asks the person to
        choose one, or to give it, as the worker says is needed, then calls
        done. Where there is a keychain it calls done at once. Call it at
        start, before anything signs in."""
        finish = done or (lambda: None)

        def ask(heading: str, body: str, ok: str, request: str, payload: Callable[[str], dict]) -> None:
            def given(password: str) -> None:
                self.ask(request, payload(password), lambda _r: finish(),
                         lambda e: (self.problem(e), self.unlock_sign_ins(done)))
            ask_text(self.active(), heading, body, given, ok=ok, password=True)

        def got(status: Any) -> None:
            status = status or {}
            if status.get("needsPassword"):
                ask("Choose a Password for Your Sign-Ins",
                    f"This system has no keychain, so {self.name} keeps your sign-ins in a file encrypted with "
                    "a password. Choose one of at least eight characters. You will be asked for it when "
                    f"{self.name} opens.", "Choose", "vero.setPassword", lambda p: {"old": "", "password": p})
            elif status.get("locked"):
                ask("Unlock Your Sign-Ins", f"Give the password with which {self.name} keeps your sign-ins.",
                    "Unlock", "vero.unlock", lambda p: {"password": p})
            else:
                finish()

        self.ask("vero.keychain", {}, got, lambda _e: finish())

    # Notifications.

    def notify(self, ident: str, title: str, body: str = "",
               buttons: list[tuple[str, Callable[[], Any]]] | None = None,
               pressed: Callable[[], Any] | None = None) -> None:
        """Shows a notification with buttons, each of which calls its
        function in this app, as does pressing the notification itself
        (pressed, or showing the window). Without a session bus it is a
        toast in the window instead."""
        self._notified = getattr(self, "_notified", {})
        self._notified[ident] = (pressed, buttons or [])
        if not has_session_bus():
            window = self.active()
            if isinstance(window, Window):
                first = (buttons or [None])[0]
                window.toast(f"{title}. {body}" if body else title, first[0] if first else "",
                             first[1] if first else None, 8)
            return
        n = Gio.Notification.new(title)
        if body:
            n.set_body(body)
        n.set_default_action_and_target("app.notification", GLib.Variant("s", f"{ident}\n-1"))
        for i, (text, _do) in enumerate(buttons or []):
            n.add_button_with_target(text, "app.notification", GLib.Variant("s", f"{ident}\n{i}"))
        self.send_notification(ident, n)

    def _notification_pressed(self, target: str) -> None:
        ident, _, which = target.partition("\n")
        pressed, buttons = getattr(self, "_notified", {}).get(ident, (None, []))
        i = int(which or -1)
        if 0 <= i < len(buttons):
            buttons[i][1]()
        elif pressed is not None:
            pressed()
        else:
            self.activate()

    def _add_action(self, name: str, do: Callable[..., Any], parameter: str = "") -> None:
        action = Gio.SimpleAction.new(name, GLib.VariantType.new(parameter) if parameter else None)
        if parameter:
            action.connect("activate", lambda _a, v: do(v.unpack()))
        else:
            action.connect("activate", lambda *_: do())
        self.add_action(action)

    # Light and dark.

    def _watch_dark(self) -> None:
        if Adw is not None:
            Adw.StyleManager.get_default().connect("notify::dark", lambda *_: self._dark_changed())
            return
        # Plain GTK follows the portal's colour scheme, where there is one,
        # through GTK's own dark setting.
        if not has_session_bus():
            return

        def apply(scheme: int) -> None:
            settings = Gtk.Settings.get_default()
            if settings is not None:
                settings.set_property("gtk-application-prefer-dark-theme", scheme == 1)
            self._dark_changed()

        def read() -> None:
            try:
                v = _bus.call_sync("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop",
                                   "org.freedesktop.portal.Settings", "Read",
                                   GLib.Variant("(ss)", ("org.freedesktop.appearance", "color-scheme")),
                                   None, Gio.DBusCallFlags.NONE, 2000, None)
                value = v.unpack()[0]
                while isinstance(value, GLib.Variant):
                    value = value.unpack()
                apply(int(value))
            except (GLib.Error, TypeError, ValueError):
                pass

        read()
        _bus.signal_subscribe("org.freedesktop.portal.Desktop", "org.freedesktop.portal.Settings", "SettingChanged",
                              "/org/freedesktop/portal/desktop", None, Gio.DBusSignalFlags.NONE,
                              lambda *a: a[5].unpack()[:2] == ("org.freedesktop.appearance", "color-scheme")
                              and apply(int(a[5].unpack()[2])), None)

    def dark(self) -> bool:
        """Whether the app is drawn dark, following the system."""
        if Adw is not None:
            return Adw.StyleManager.get_default().get_dark()
        settings = Gtk.Settings.get_default()
        return bool(settings and settings.get_property("gtk-application-prefer-dark-theme"))

    def on_dark(self, watch: Callable[[bool], Any]) -> None:
        """Calls watch whenever the app turns light or dark."""
        self._dark_watchers.append(watch)

    def _dark_changed(self) -> None:
        dark = self.dark()
        for watch in list(self._dark_watchers):
            watch(dark)

    # The network.

    def on_network(self, back: Callable[[], Any]) -> None:
        """Calls back whenever the network comes back, so that the app can
        ask the worker to sync at once."""
        monitor = Gio.NetworkMonitor.get_default()
        was = [monitor.get_network_available()]

        def changed(m: Gio.NetworkMonitor, available: bool) -> None:
            if available and not was[0]:
                back()
            was[0] = available

        monitor.connect("network-changed", changed)


class UndoStack:
    """Undo and redo of what the worker did, on Ctrl+Z and Ctrl+Shift+Z.

    After an action, push its label and a function that undoes it (asking
    the worker, such as with the action id the worker replied with). That
    function may return a function that redoes it, or None. The window's
    toast offers Undo for the newest. clear() forgets everything, as when
    the account changes.
    """

    def __init__(self, app: App) -> None:
        self.app = app
        self.undos: list[tuple[str, Callable[[], Any]]] = []
        self.redos: list[tuple[str, Callable[[], Any]]] = []

    def push(self, text: str, undo: Callable[[], Any], toast: bool = True) -> None:
        self.undos.append((text, undo))
        del self.undos[:-100]
        self.redos.clear()
        window = self.app.active()
        if toast and isinstance(window, Window):
            window.toast(text, "Undo", self.undo)

    def undo(self) -> None:
        if not self.undos:
            return
        text, do = self.undos.pop()
        redo = do()
        if callable(redo):
            self.redos.append((text, redo))

    def redo(self) -> None:
        if not self.redos:
            return
        text, do = self.redos.pop()
        undo = do()
        if callable(undo):
            self.undos.append((text, undo))

    def clear(self) -> None:
        self.undos.clear()
        self.redos.clear()


# Web content.

def _plain_text(markup: str) -> str:
    text = re.sub(r"(?is)<(script|style)\b.*?</\1>", "", markup)
    text = re.sub(r"(?i)</p>|<br\s*/?>|</div>|</li>|</tr>|</h[1-6]>", "\n", text)
    text = re.sub(r"<[^>]+>", "", text)
    return re.sub(r"\n{3,}", "\n\n", html.unescape(text)).strip()


# What a page may load: its own styles, and pictures that come with it.
HTML_POLICY = "default-src 'none'; style-src 'unsafe-inline'; img-src data: cid:"


def _with_policy(markup: str, policy: str, remote: bool) -> str:
    if remote:
        policy = policy.replace("img-src", "img-src https: http:", 1) if "img-src" in policy else policy + "; img-src https: http: data:"
    return f'<meta http-equiv="Content-Security-Policy" content="{html.escape(policy)}">' + markup


class HtmlView(Gtk.Box):
    """Shows HTML, such as a message, with scripts off and nothing loaded
    from the network unless remote is set, which can change before each
    load. policy is the Content Security Policy, HTML_POLICY unless the app
    sets its own. Links to the web and mailto: open in the browser. A link
    in one of the app's own schemes, such as myapp-attachment:, goes to
    on_link, which returns True when it has dealt with it. Where there is
    no web view it shows the text of the HTML instead, never the page in
    the browser, since what it shows may be private."""

    def __init__(self, remote: bool = False, policy: str = HTML_POLICY,
                 on_link: Callable[[str], bool] | None = None) -> None:
        super().__init__(orientation=Gtk.Orientation.VERTICAL, vexpand=True, hexpand=True)
        self.remote = remote
        self.policy = policy
        self.on_link = on_link
        WebKit = webkit()
        if WebKit is not None:
            self.view = WebKit.WebView(vexpand=True, hexpand=True)
            settings = self.view.get_settings()
            settings.set_enable_javascript(False)
            self.view.connect("decide-policy", self._decide)
            self.append(self.view)
            self.text = None
        else:
            self.view = None
            self.text = Gtk.TextView(editable=False, cursor_visible=False, wrap_mode=Gtk.WrapMode.WORD_CHAR,
                                     left_margin=16, right_margin=16, top_margin=12, bottom_margin=12)
            self.append(Gtk.ScrolledWindow(child=self.text, vexpand=True))

    def load(self, markup: str) -> None:
        if self.view is not None:
            self.view.load_html(_with_policy(markup, self.policy, self.remote), None)
        else:
            self.text.get_buffer().set_text(_plain_text(markup))

    def _decide(self, _view: Any, decision: Any, kind: Any) -> bool:
        WebKit = webkit()
        if kind not in (WebKit.PolicyDecisionType.NAVIGATION_ACTION, WebKit.PolicyDecisionType.NEW_WINDOW_ACTION):
            return False
        action = decision.get_navigation_action()
        uri = action.get_request().get_uri() or ""
        # The page itself, as load() gives it.
        if uri.startswith(("about:", "data:")) and action.get_navigation_type() == WebKit.NavigationType.OTHER:
            return False
        decision.ignore()
        if self.on_link is not None and self.on_link(uri):
            return True
        if uri.startswith(("http:", "https:", "mailto:")):
            open_uri(uri)
        return True


class PdfWriter:
    """Turns HTML into PDF files without showing it, one after another
    through one hidden web view, so that thousands can be written in turn:

        pdf = PdfWriter()
        pdf.write(markup, "/path/to/message.pdf", lambda error: ...)

    done is called on the main loop with "" once the file is written, or
    with what went wrong. Scripts are off and nothing is loaded from the
    network unless remote is set. Where there is no web view, every write
    fails, saying so; has_webview() tells an app beforehand."""

    def __init__(self, remote: bool = False, policy: str = HTML_POLICY) -> None:
        self.remote, self.policy = remote, policy
        self._queue: list[tuple[str, str, Callable[[str], Any]]] = []
        self._busy = False
        self._view: Any = None
        self._loaded: Any = None

    def write(self, markup: str, path: str, done: Callable[[str], Any]) -> None:
        self._queue.append((markup, os.path.abspath(path), done))
        if not self._busy:
            self._next()

    def _next(self) -> None:
        if not self._queue:
            self._busy = False
            return
        self._busy = True
        markup, path, done = self._queue.pop(0)
        WebKit = webkit()
        if WebKit is None:
            GLib.idle_add(lambda: (done("This system has no web view, which writing a PDF needs."), self._next()) and False)
            return
        if self._view is None:
            self._view = WebKit.WebView()
            self._view.get_settings().set_enable_javascript(False)
            # Nothing but the page: a link in it goes nowhere.
            self._view.connect("decide-policy", self._decide)
            self._view.connect("load-changed", self._load_changed)
        self._job = (path, done)
        self._view.load_html(_with_policy(markup, self.policy, self.remote), None)

    def _decide(self, _view: Any, decision: Any, kind: Any) -> bool:
        WebKit = webkit()
        if kind == WebKit.PolicyDecisionType.NAVIGATION_ACTION:
            uri = decision.get_navigation_action().get_request().get_uri() or ""
            if not uri.startswith(("about:", "data:")):
                decision.ignore()
                return True
        return False

    def _load_changed(self, view: Any, event: Any) -> None:
        WebKit = webkit()
        if event != WebKit.LoadEvent.FINISHED:
            return
        path, done = self._job
        settings = Gtk.PrintSettings()
        settings.set_printer("Print to File")
        settings.set(Gtk.PRINT_SETTINGS_OUTPUT_FILE_FORMAT, "pdf")
        settings.set(Gtk.PRINT_SETTINGS_OUTPUT_URI, GLib.filename_to_uri(path, None))
        op = WebKit.PrintOperation.new(view)
        op.set_print_settings(settings)
        op.set_page_setup(Gtk.PageSetup())
        result: dict = {"error": ""}
        op.connect("failed", lambda _o, e: result.update(error=e.message))

        def finished(_o: Any) -> None:
            error = result["error"] or ("" if os.path.exists(path) else "The PDF was not written.")
            done(error)
            self._next()

        op.connect("finished", finished)
        op.print_()


def web_bridge(view: Any, name: str, received: Callable[[Any, Callable[[Any], None]], Any]) -> Callable[[str], None]:
    """Lets a page's scripts and the app talk, for an editor or add-ins
    in a web view (from webkit().WebView, which exists only where
    has_webview() is true). The page sends with
    window.webkit.messageHandlers[name].postMessage(value); received gets
    the value and a function that replies to it. Returns a function that
    runs a script in the page."""
    gi.require_version("JavaScriptCore", "6.0")
    from gi.repository import JavaScriptCore

    manager = view.get_user_content_manager()

    def got(_m: Any, value: Any, reply: Any) -> bool:
        try:
            data = json.loads(value.to_json(0))
        except (AttributeError, ValueError, TypeError):
            data = None
        context = value.get_context()
        received(data, lambda v: reply.return_value(JavaScriptCore.Value.new_from_json(context, json.dumps(v))))
        return True

    manager.connect(f"script-message-with-reply-received::{name}", got)
    manager.register_script_message_handler_with_reply(name, None)

    def run(script: str) -> None:
        view.evaluate_javascript(script, -1, None, None, None, None)

    return run


class SignInPage((Adw.Window if Adw is not None else Gtk.Window)):  # type: ignore[misc]
    """A sign-in page in a web view, which stops when the page goes to an
    address that stop_at accepts and gives that address to done, as an
    OAuth sign-in lands on its redirect. Nothing the page sets outlives it.

    Where there is no web view, the page opens in the browser, and the
    person pastes back the address it lands on, which steps explains."""

    def __init__(self, parent: Gtk.Window, title: str, url: str, stop_at: Callable[[str], bool],
                 done: Callable[[str], Any], steps: str = "") -> None:
        super().__init__(title=title, transient_for=parent, modal=True, default_width=560, default_height=680)
        self.stop_at, self.done, self.finished = stop_at, done, False
        box = Gtk.Box(orientation=Gtk.Orientation.VERTICAL)
        if Adw is not None:
            view = Adw.ToolbarView(content=box)
            view.add_top_bar(Adw.HeaderBar())
            self.set_content(view)
        else:
            self.set_child(box)
        self.problem = label(wrap=True, css="error")
        WebKit = webkit()
        if WebKit is None:
            self._in_browser(box, url, steps)
            return
        web = WebKit.WebView(network_session=WebKit.NetworkSession.new_ephemeral(), vexpand=True)
        web.connect("decide-policy", self._decide)
        box.append(web)
        box.append(self.problem)
        web.load_uri(url)

    def _decide(self, _web: Any, decision: Any, kind: Any) -> bool:
        WebKit = webkit()
        if kind != WebKit.PolicyDecisionType.NAVIGATION_ACTION:
            return False
        uri = decision.get_navigation_action().get_request().get_uri() or ""
        if self.stop_at(uri):
            decision.ignore()
            self._finish(uri)
            return True
        return False

    def _in_browser(self, box: Gtk.Box, url: str, steps: str) -> None:
        self.set_default_size(520, -1)
        for side in ("top", "bottom", "start", "end"):
            getattr(box, f"set_margin_{side}")(18)
        box.set_spacing(12)
        box.append(label("This system has no web view, so the sign-in page opens in your web browser.", wrap=True))
        if steps:
            box.append(label(steps, wrap=True))
        open_button = Gtk.Button(label="Open the Sign-In Page", halign=Gtk.Align.START)
        open_button.connect("clicked", lambda *_: open_uri(url))
        address = Gtk.Entry(placeholder_text="The address the page went to")
        finish = Gtk.Button(label="Finish Signing In", halign=Gtk.Align.START)

        def pasted(*_: Any) -> None:
            uri = address.get_text().strip()
            if self.stop_at(uri):
                self._finish(uri)
            else:
                self.problem.set_text("That is not the address the page goes to after signing in. "
                                      "Copy all of it from the browser's address bar.")

        address.connect("activate", pasted)
        finish.connect("clicked", pasted)
        for w in (open_button, address, finish, self.problem):
            box.append(w)

    def _finish(self, uri: str) -> None:
        if not self.finished:
            self.finished = True
            self.close()
            self.done(uri)


def main(app: App) -> None:
    """Runs the app and exits with its status."""
    sys.exit(app.run(sys.argv))
