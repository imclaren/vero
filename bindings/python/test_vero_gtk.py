"""Tests for vero_gtk.py, against the example worker, in a real display.

    cd bindings/python && python3 -m unittest -v test_vero_gtk

Each scenario runs in its own process, since the library chooses
libadwaita or plain GTK when it is imported. They need a display (xvfb-run
gives one) and, for the tray and notifications, a session bus
(dbus-run-session gives one); without either, the scenarios that need it
are skipped. VERO_GTK_PLAIN=1 runs them all on plain GTK 4.

vero's scripts/test-gtk-libs.sh runs them in containers: Debian 13 with
libadwaita and a web view, the same plain, Debian 12, whose libadwaita is
too old, and each without a session bus.
"""

from __future__ import annotations

import json
import os
import re
import subprocess
import sys
import tempfile
import time
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.abspath(os.path.join(HERE, "../.."))
NO_BUS = "unix:path=/nonexistent/vero-test-bus"


def worker_path(tmp: str) -> str:
    """The example worker: VERO_TEST_WORKER where it was built elsewhere,
    as for a container, or built here."""
    if os.environ.get("VERO_TEST_WORKER"):
        return os.environ["VERO_TEST_WORKER"]
    path = os.path.join(tmp, "worker")
    subprocess.run(["go", "build", "-o", path, "./example/worker"], cwd=REPO, check=True)
    return path


def run(scenario: str, worker: str, *args: str, env: dict | None = None, timeout: int = 60,
        wait: bool = True) -> subprocess.CompletedProcess | subprocess.Popen:
    cmd = [sys.executable, os.path.abspath(__file__), "--scenario", scenario, worker, *args]
    environment = dict(os.environ, **(env or {}))
    if not wait:
        return subprocess.Popen(cmd, env=environment, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    return subprocess.run(cmd, env=environment, capture_output=True, text=True, timeout=timeout)


def display() -> bool:
    return bool(os.environ.get("DISPLAY") or os.environ.get("WAYLAND_DISPLAY"))


def bus() -> bool:
    address = os.environ.get("DBUS_SESSION_BUS_ADDRESS", "")
    return bool(address) and address != NO_BUS


@unittest.skipUnless(display(), "no display")
class GtkLibraryTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.tmp = tempfile.mkdtemp()
        cls.worker = worker_path(cls.tmp)

    def check(self, result: subprocess.CompletedProcess) -> str:
        if result.returncode != 0 or "SCENARIO OK" not in result.stdout:
            self.fail(f"exit {result.returncode}\n{result.stdout}\n{result.stderr}")
        return result.stdout

    def test_window_and_pieces(self) -> None:
        self.check(run("window", self.worker))

    def test_window_and_pieces_without_a_bus(self) -> None:
        self.check(run("window", self.worker, env={"DBUS_SESSION_BUS_ADDRESS": NO_BUS}))

    def test_single_copy_without_a_bus_passes_on_links_and_files(self) -> None:
        env = {"DBUS_SESSION_BUS_ADDRESS": NO_BUS, "XDG_RUNTIME_DIR": tempfile.mkdtemp()}
        first = run("single", self.worker, env=env, wait=False)
        time.sleep(3)
        second = run("single", self.worker, "mailto:someone@example.com", "notes.txt", env=env, timeout=20)
        self.assertEqual(second.returncode, 0, second.stdout + second.stderr)
        self.assertNotIn("SCENARIO STARTED", second.stdout, "the second copy opened a window of its own")
        out, _ = first.communicate(timeout=30)
        self.assertEqual(first.returncode, 0, out)
        self.assertIn('opened ["mailto:someone@example.com", "', out)
        self.assertIn('/notes.txt"]', out)

    @unittest.skipUnless(bus(), "no session bus")
    def test_single_copy_with_a_bus_passes_on_links(self) -> None:
        first = run("single", self.worker, wait=False)
        time.sleep(3)
        second = run("single", self.worker, "calmtasks://task/1", timeout=20)
        self.assertEqual(second.returncode, 0, second.stdout + second.stderr)
        out, _ = first.communicate(timeout=30)
        self.assertEqual(first.returncode, 0, out)
        self.assertIn('opened ["calmtasks://task/1"]', out)

    def with_watcher(self, scenario: str, name: str, *args: str) -> str:
        """Runs a scenario with a stand-in for the desktop's tray, which
        presses the icon named name and then chooses Quit, and returns what
        the app printed."""
        watcher = subprocess.Popen([sys.executable, os.path.abspath(__file__), "--watcher", name],
                                   stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        time.sleep(1)
        app = run(scenario, self.worker, *args, wait=False)
        try:
            w_out, _ = watcher.communicate(timeout=60)
            a_out, _ = app.communicate(timeout=30)
        finally:
            for p in (watcher, app):
                if p.poll() is None:
                    p.kill()
        self.assertEqual(watcher.returncode, 0, w_out)
        self.assertIn("WATCHER OK", w_out)
        self.assertEqual(app.returncode, 0, a_out)
        return a_out

    @unittest.skipUnless(bus(), "no session bus")
    def test_tray_menu_activate_and_quit(self) -> None:
        """A tray app: no window at sign-in, the small window from its icon,
        the main window from the small one, and Quit from its menu."""
        out = self.with_watcher("tray", "vero tray", "--background")
        for line in ("no window at start", "small window shown", "main window shown", "main window hidden, still running"):
            self.assertIn(line, out)

    @unittest.skipUnless(bus(), "no session bus")
    def test_window_app_with_a_tray_icon(self) -> None:
        """A window app with a tray icon: closing its window leaves it in the
        tray, and the icon brings the window back."""
        out = self.with_watcher("windowtray", "vero windowtray")
        for line in ("window at start", "window hidden, still running", "window back from the tray"):
            self.assertIn(line, out)

    @unittest.skipUnless(os.environ.get("VERO_TEST_KEYCHAIN_WORKER"), "no keychain worker")
    def test_sign_ins_with_a_password(self) -> None:
        """Where there is no keychain: a password chosen the first time,
        then asked for, a wrong one refused, and the file encrypted."""
        worker = os.environ["VERO_TEST_KEYCHAIN_WORKER"]
        env = {"VERO_TEST_KEYCHAIN_DIR": tempfile.mkdtemp()}
        out = self.check(run("keychain", worker, "correct horse", env=env))
        self.assertIn("loaded: secret", out)
        with open(os.path.join(env["VERO_TEST_KEYCHAIN_DIR"], "token"), "rb") as f:
            self.assertNotIn(b"secret", f.read())
        out = self.check(run("keychain", worker, "wrong horse", "correct horse", env=env))
        self.assertIn("asked 2 times", out)
        self.assertIn("loaded: secret", out)

    def test_tray_app_without_a_tray_is_a_window(self) -> None:
        out = self.check(run("tray-alone", self.worker, env={"DBUS_SESSION_BUS_ADDRESS": NO_BUS}))
        self.assertIn("closing asked first", out)


# The scenarios, each in its own process.

def scenario(name: str, worker: str, args: list[str]) -> int:
    sys.path.insert(0, HERE)
    import vero_gtk as vg
    from gi.repository import GLib, Gtk

    print(f"libadwaita: {vg.has_adwaita()}, bus: {vg.has_session_bus()}, web view: {vg.has_webview()}, "
          f"GTK {vg.GTK_VERSION}", flush=True)
    failures: list[str] = []

    def expect(ok: bool, what: str) -> None:
        if not ok:
            failures.append(what)
            print(f"FAILED: {what}", flush=True)

    def later(seconds: float, do) -> None:
        GLib.timeout_add(int(seconds * 1000), lambda: do() and False)

    def finish(app) -> None:
        print("SCENARIO OK" if not failures else f"SCENARIO FAILED: {failures}", flush=True)
        app.quit_app()

    if name == "window":
        config = tempfile.mkdtemp()
        os.environ["XDG_CONFIG_HOME"] = config
        GLib.reload_user_special_dirs_cache()
        seen: dict = {}

        class Main(vg.Window):
            def __init__(self, app):
                super().__init__(app, "vero test")
                self.rows = vg.label("waiting")
                self.set_content(self.rows)
                app.on_state(lambda s: seen.setdefault("state", s))

        # The test script says what each container has, so that a library
        # that fell back to plain GTK, or lost its web view, is caught.
        for var, has in (("VERO_EXPECT_ADWAITA", vg.has_adwaita()), ("VERO_EXPECT_WEBVIEW", vg.has_webview())):
            if os.environ.get(var) in ("0", "1"):
                expect(has == (os.environ[var] == "1"), f"{var}={os.environ[var]}, but it is {has}")
        app = vg.App("com.calmdocs.vero.test.window", "vero test", worker, window=Main)

        def checks() -> None:
            w = app.window
            expect(isinstance(seen.get("state"), dict) and "jobs" in seen["state"], "the state arrived")
            app.ask("status", {}, lambda r: seen.setdefault("reply", r))
            app.ask("restartJob", {"id": 999}, None, lambda e: seen.setdefault("refused", e))
            app.ask(lambda: app.vero.call("status", {}), None, done=lambda r: seen.setdefault("callable", r))
            w.problem("Something is wrong.")
            dismiss = w.toast("Sent.", "Undo", lambda: None)
            dismiss()
            w.show_update("vero test 2 is out.", "https://example.com")
            w.problem("")
            for widget in (vg.status_page("folder-symbolic", "Nothing here", "Add something.",
                                          Gtk.Button(label="Add")),
                           vg.avatar("Ada Lovelace", 40), vg.split_view(Gtk.Label(), Gtk.Label())):
                expect(isinstance(widget, Gtk.Widget), f"{type(widget).__name__} is a widget")
            s = vg.Settings(w)
            g = s.page("General", "preferences-system-symbolic").group("Syncing", "How often.")
            g.switch("Sync automatically", "Every few minutes", True, lambda on: None)
            g.row("Account", "you@example.com", Gtk.Button(label="Sign Out"))
            g.button("Data", "", "Delete", lambda: None, destructive=True)
            s.present()
            later(0.5, s.close)
            vg.question(w, "Delete it?", "It cannot be undone.", ["Cancel", "Delete"], destructive=1)
            vg.ask_text(w, "Password", "For the keyring file.", lambda t: None, password=True)
            links: list = []
            view = vg.HtmlView(on_link=lambda uri: uri.startswith("vero-test:") and not links.append(uri))
            view.load("<p>Hello <b>there</b></p><script>alert(1)</script>")
            if view.view is not None:
                # A link in the app's own scheme reaches on_link.
                later(1, lambda: view.view.load_uri("vero-test:attachment/1"))
                later(2.5, lambda: expect(links == ["vero-test:attachment/1"], f"the app's own link: {links}"))
            # HTML to a PDF file, offscreen, twice in turn.
            pdfs = tempfile.mkdtemp()
            writer = vg.PdfWriter(paper="a4", margin=10)
            long = "<p>A paragraph of a receipt, long enough to fill a line or two of the page.</p>" * 150
            for n in (1, 2):
                path = os.path.join(pdfs, f"{n}.pdf")
                writer.write(f"<h1>Message {n}</h1>" + (long if n == 2 else "<p>Hello.</p>"), path,
                             lambda error, path=path: seen.setdefault(path, error or open(path, "rb").read(5)))
            seen["pdfs"] = pdfs
            if view.text is not None:
                buf = view.text.get_buffer()
                text = buf.get_text(buf.get_start_iter(), buf.get_end_iter(), False)
                expect(text == "Hello there", f"HTML shown as text: {text!r}")
            # Undo and redo.
            done: list = []
            app.undo.push("Deleted a task", lambda: (done.append("undo"), lambda: done.append("redo"))[1])
            app.undo.undo()
            app.undo.redo()
            expect(done == ["undo", "redo"], f"undo then redo: {done}")
            # Notifications' buttons reach the app.
            pressed: list = []
            app.notify("n1", "New mail", "From Ada", [("Archive", lambda: pressed.append("archive"))])
            app.activate_action("notification", GLib.Variant("s", "n1\n0"))
            expect(pressed == ["archive"], f"the notification's button: {pressed}")
            # Opening at sign-in.
            expect(app.set_opens_at_login(True) == "" and app.opens_at_login(), "opens at login")
            with open(os.path.join(config, "autostart", "com.calmdocs.vero.test.window.desktop")) as f:
                expect("Exec=" in f.read(), "the autostart file has a command")
            expect(app.set_opens_at_login(False) == "" and not app.opens_at_login(), "no longer opens at login")
            later(3, replies)

        def replies() -> None:
            expect(isinstance(seen.get("reply"), dict), f"status replied: {seen.get('reply')}")
            expect(bool(seen.get("refused")), "the refusal reached failed")
            expect(isinstance(seen.get("callable"), dict), "a function as the request")
            for n in (1, 2):
                got = seen.get(os.path.join(seen["pdfs"], f"{n}.pdf"))
                if vg.has_webview():
                    expect(got == b"%PDF-", f"PDF {n}: {got!r}")
                    with open(os.path.join(seen["pdfs"], f"{n}.pdf"), "rb") as f:
                        data = f.read()
                    # Pages of A4, 595 by 842 points (841.9), and several for the long one.
                    pages = len(re.findall(rb"/Type\s*/Page[^s]", data))
                    expect(b"595" in data and (b"841" in data or b"842" in data), f"PDF {n} is on A4")
                    expect(pages == 1 if n == 1 else pages > 1, f"PDF {n} has {pages} pages")
                else:
                    expect(isinstance(got, str) and "web view" in got, f"no PDF without a web view: {got!r}")
            for win in list(app.get_windows()):
                if win is not app.window:
                    win.destroy()
            finish(app)

        later(3, checks)
        return app.run(["vero-test"])

    if name == "single":
        def got(things: list[str]) -> None:
            print("opened " + json.dumps(things), flush=True)
            later(0.5, lambda: finish(app))

        app = vg.App("com.calmdocs.vero.test.single", "vero single", worker,
                     window=lambda a: vg.Window(a, "single"), opened=got)
        later(0.1, lambda: print("SCENARIO STARTED", flush=True))
        later(40, lambda: (failures.append("nothing was passed on"), finish(app)))
        return app.run(["vero-test", *args])

    if name == "tray":
        state = {"shown": False}

        def look() -> bool:
            visible = [w for w in app.get_windows() if w.get_visible()]
            if not state["shown"] and not visible:
                print("no window at start", flush=True)
                state["shown"] = None
            if visible and app.small in visible and state["shown"] is None:
                print("small window shown", flush=True)
                state["shown"] = True
                later(0.3, open_main)
            return True

        def open_main() -> None:
            app.show_window()
            later(0.3, close_main)

        def close_main() -> None:
            if app.window is not None and app.window.get_visible():
                print("main window shown", flush=True)
                app.window.close()
                later(0.3, lambda: app.window.get_visible() or print("main window hidden, still running", flush=True))

        app = vg.App("com.calmdocs.vero.test.tray", "vero tray", worker,
                     small_window=lambda a: vg.SmallWindow(a, "vero tray"),
                     window=lambda a: vg.Window(a, "vero tray main"))
        later(1.5, look)
        GLib.timeout_add(250, look)
        return app.run(["vero-test", *args])

    if name == "windowtray":
        state = {"hidden": False}

        def first() -> None:
            if app.window is not None and app.window.get_visible():
                print("window at start", flush=True)
                app.window.close()
                later(0.3, hidden)

        def hidden() -> None:
            if not app.window.get_visible():
                print("window hidden, still running", flush=True)
                state["hidden"] = True

        def look() -> bool:
            if state["hidden"] and app.window.get_visible():
                print("window back from the tray", flush=True)
                state["hidden"] = False
            return True

        app = vg.App("com.calmdocs.vero.test.windowtray", "vero windowtray", worker,
                     window=lambda a: vg.Window(a, "vero windowtray"), tray=True)
        later(1, first)
        GLib.timeout_add(250, look)
        return app.run(["vero-test", *args])

    if name == "keychain":
        # The person's answers, in turn, in place of typing them.
        answers = list(args)
        asked = [0]

        def answer(_parent, _heading, _body, done, **_kw) -> None:
            asked[0] += 1
            GLib.idle_add(lambda: done(answers.pop(0)) and False)

        vg.ask_text = answer
        app = vg.App("com.calmdocs.vero.test.keychain", "vero keychain", worker,
                     window=lambda a: vg.Window(a, "vero keychain"))

        def unlocked() -> None:
            print(f"asked {asked[0]} times", flush=True)
            app.ask("save", {"text": "secret"}, lambda _r: app.ask("load", {}, loaded),
                    lambda e: (failures.append(e), finish(app)))

        def loaded(text: str) -> None:
            print(f"loaded: {text}", flush=True)
            finish(app)

        later(1, lambda: app.unlock_sign_ins(unlocked))
        return app.run(["vero-test"])

    if name == "tray-alone":
        app = vg.App("com.calmdocs.vero.test.alone", "vero alone", worker,
                     small_window=lambda a: vg.SmallWindow(a, "vero alone"))

        def checks() -> None:
            expect(app.small is not None and app.small.get_visible(), "the small window is an ordinary window")
            app.small.close()
            later(0.5, after)

        def after() -> None:
            expect(app.small.get_visible(), "the window stayed open")
            # The question is a dialog in the window, or, where the window is
            # too small for it or there is no libadwaita, a window of its own.
            tops = [w for w in Gtk.Window.list_toplevels() if w is not app.small and w.get_visible()]
            dialog = app.small.get_visible_dialog() if vg.has_adwaita() else None
            asking = dialog is not None or bool(tops)
            expect(asking, "closing asked first")
            if not failures:
                print("closing asked first", flush=True)
            finish(app)

        later(2, checks)
        return app.run(["vero-test"])

    raise SystemExit(f"no scenario {name}")


def watcher(name: str) -> int:
    """A stand-in for the desktop's tray: it takes the watcher's name,
    waits for the app's icon, reads its menu, presses the icon, then
    chooses Quit."""
    from gi.repository import Gio, GLib

    bus = Gio.bus_get_sync(Gio.BusType.SESSION, None)
    xml = """<node><interface name="org.kde.StatusNotifierWatcher">
      <method name="RegisterStatusNotifierItem"><arg type="s" direction="in"/></method>
      <property name="IsStatusNotifierHostRegistered" type="b" access="read"/>
      <property name="RegisteredStatusNotifierItems" type="as" access="read"/>
      <property name="ProtocolVersion" type="i" access="read"/>
    </interface></node>"""
    info = Gio.DBusNodeInfo.new_for_xml(xml).interfaces[0]
    loop = GLib.MainLoop()
    result = {"ok": False}

    def check(service: str) -> bool:
        try:
            layout = bus.call_sync(service, "/MenuBar", "com.canonical.dbusmenu", "GetLayout",
                                   GLib.Variant("(iias)", (0, -1, [])), None, Gio.DBusCallFlags.NONE, 3000, None).unpack()
            items = {child[1].get("label", ""): child[0] for child in layout[1][2]}
            print(f"menu: {list(items)}", flush=True)
            assert f"Open {name}" in items and f"Quit {name}" in items, items
            title = bus.call_sync(service, "/StatusNotifierItem", "org.freedesktop.DBus.Properties", "Get",
                                  GLib.Variant("(ss)", ("org.kde.StatusNotifierItem", "Title")), None,
                                  Gio.DBusCallFlags.NONE, 3000, None).unpack()[0]
            assert title == name, title
            bus.call_sync(service, "/StatusNotifierItem", "org.kde.StatusNotifierItem", "Activate",
                          GLib.Variant("(ii)", (0, 0)), None, Gio.DBusCallFlags.NONE, 3000, None)
            time.sleep(2)
            bus.call_sync(service, "/MenuBar", "com.canonical.dbusmenu", "Event",
                          GLib.Variant("(isvu)", (items[f"Quit {name}"], "clicked", GLib.Variant("s", ""), 0)),
                          None, Gio.DBusCallFlags.NONE, 3000, None)
            result["ok"] = True
        except Exception as e:  # noqa: BLE001
            print(f"watcher: {e}", flush=True)
        loop.quit()
        return False

    def call(_c, _s, _p, _i, method, params, invocation) -> None:
        invocation.return_value(None)
        if method == "RegisterStatusNotifierItem":
            print(f"registered {params.unpack()[0]}", flush=True)
            GLib.timeout_add(2500, check, params.unpack()[0])

    def get(_c, _s, _p, _i, prop):
        return {"IsStatusNotifierHostRegistered": GLib.Variant("b", True),
                "RegisteredStatusNotifierItems": GLib.Variant("as", []),
                "ProtocolVersion": GLib.Variant("i", 0)}.get(prop)

    bus.register_object("/StatusNotifierWatcher", info, call, get, None)
    Gio.bus_own_name_on_connection(bus, "org.kde.StatusNotifierWatcher", Gio.BusNameOwnerFlags.NONE, None, None)
    GLib.timeout_add_seconds(45, loop.quit)
    loop.run()
    print("WATCHER OK" if result["ok"] else "WATCHER FAILED", flush=True)
    return 0 if result["ok"] else 1


if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] == "--scenario":
        sys.exit(scenario(sys.argv[2], sys.argv[3], sys.argv[4:]))
    if len(sys.argv) > 2 and sys.argv[1] == "--watcher":
        sys.exit(watcher(sys.argv[2]))
    unittest.main()
