#!/usr/bin/env python3
"""A GTK4 frontend for the worker in ../worker.

    ./build.sh
    ./main.py

The same rows as the macOS and WPF examples: a Restart button, the job, the
phase it is in, and how far through it is.
"""

import os
import sys

import gi

gi.require_version("Gtk", "4.0")
from gi.repository import GLib, Gtk  # noqa: E402

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "../../bindings/python"))
from vero import NotRunning, Vero, run_in_thread  # noqa: E402

HERE = os.path.dirname(os.path.abspath(__file__))


class Window(Gtk.ApplicationWindow):
    def __init__(self, app: Gtk.Application, vero: Vero) -> None:
        super().__init__(application=app, title="vero")
        self.vero = vero
        self.rows: dict[int, dict] = {}
        self.set_default_size(380, 0)

        self.jobs = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=10)
        self.jobs.set_margin_top(16)
        self.jobs.set_margin_bottom(16)
        self.jobs.set_margin_start(16)
        self.jobs.set_margin_end(16)
        self.set_child(self.jobs)

        # latest draws a window that has just opened; the events after it keep
        # it current, and nothing polls.
        try:
            if (status := self.vero.latest()):
                self.apply(status)
        except NotRunning:
            pass
        run_in_thread(self.vero, lambda s: GLib.idle_add(self.apply, s))

    def apply(self, status: dict) -> bool:
        for job in status["jobs"]:
            if job["id"] not in self.rows:
                self.add_row(job)
            row = self.rows[job["id"]]
            row["name"].set_text(job["name"])
            row["phase"].set_text(job["phase"])
            row["bar"].set_fraction(job["progress"] / 100)
        return False  # GLib.idle_add: run once

    def restart(self, job_id: int) -> None:
        """Ask the worker to run this job again.

        call() names the handler on the worker - "restartJob" is the
        vero.UpdateWith in main.go - and the reply is the new status, so the
        window redraws without waiting for the next event.
        """
        try:
            self.apply(self.vero.call("restartJob", {"id": job_id}))
        except NotRunning:
            pass

    def add_row(self, job: dict) -> None:
        row = Gtk.Box(orientation=Gtk.Orientation.HORIZONTAL, spacing=10)

        button = Gtk.Button(label="Restart", valign=Gtk.Align.CENTER)
        button.connect("clicked", lambda *_: self.restart(job["id"]))
        row.append(button)

        name = Gtk.Label(halign=Gtk.Align.START)
        phase = Gtk.Label(halign=Gtk.Align.START)
        phase.add_css_class("dim-label")
        bar = Gtk.ProgressBar(hexpand=True, valign=Gtk.Align.CENTER)
        row.append(name)
        row.append(phase)
        row.append(bar)

        self.jobs.append(row)
        self.rows[job["id"]] = {"name": name, "phase": phase, "bar": bar}


class Application(Gtk.Application):
    def __init__(self) -> None:
        super().__init__(application_id="com.calmdocs.vero.example")
        self.vero: Vero | None = None

    def do_activate(self) -> None:
        if self.vero is None:
            self.vero = Vero(os.path.join(HERE, "worker"))
        Window(self, self.vero).present()

    def do_shutdown(self) -> None:
        # Not required - the worker's stdin closes when we exit and it goes
        # with us - but it stops the work a moment sooner.
        if self.vero:
            self.vero.stop()
        Gtk.Application.do_shutdown(self)


if __name__ == "__main__":
    sys.exit(Application().run(sys.argv))
