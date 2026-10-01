"""Drive a Go worker from a Python frontend - GTK, Qt, or anything else.

The worker supervises itself.  Run with VERO_HOST set it becomes a host: it
launches a second copy of itself to do the work, restarts that copy if it
dies, and speaks JSON on its own standard input and output.  So this module
spawns one process and writes lines to it; the supervision, the backoff, the
single-worker lock and the cleanup are all Go, on the other side of the pipe.

    go build -o worker ./your/worker

Then:

    from vero import Vero

    v = Vero("./worker")
    for status in v.events():
        print(status["jobs"])

There is no shared library to build, ship or match to an architecture, which
is why this works anywhere Go produces an executable - every BSD, illumos, and
Linux on thirteen architectures - rather than only where it can produce a C
library.
"""

from __future__ import annotations

import json
import os
import queue
import subprocess
import threading
from typing import Any, Iterator


class VeroError(Exception):
    """Something went wrong talking to the worker."""


class Refused(VeroError):
    """The worker received the request and refused it.

    It is still running, so this is a problem with the request.  Show the
    message and carry on.
    """


class AlreadyRunning(VeroError):
    """Another process is already running a worker for this application.

    Offer to switch to the copy that is running: retrying will not help, and
    nothing is broken.
    """


class NotRunning(VeroError):
    """The worker is starting, restarting after a crash, or stopped.

    Nothing you did was wrong: wait, and say so in the frontend.
    """


_ERRORS = {
    "refused": Refused,
    "already_running": AlreadyRunning,
    "not_running": NotRunning,
}


class Vero:
    """Runs a Go worker and talks to it."""

    def __init__(self, worker_path: str, arguments: list[str] | None = None) -> None:
        self._next_id = 0
        self._pending: dict[int, queue.Queue] = {}
        self._subscribers: list[queue.Queue] = []
        self._latest: Any = None
        self._state = "starting"
        self._restarts = 0
        self._stopped = False
        self._lock = threading.Lock()

        environment = dict(os.environ, VERO_HOST="1")
        self._process = subprocess.Popen(
            [worker_path, *(arguments or [])],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            env=environment,
        )

        # A daemon thread, so an application that forgets to stop() still
        # exits.  The worker goes with it either way: its standard input
        # closes when this process does.
        self._reader = threading.Thread(target=self._read, daemon=True)
        self._reader.start()

    def _read(self) -> None:
        """Sort what the host says into replies, events and state changes."""
        for line in self._process.stdout:
            try:
                envelope = json.loads(line)
            except ValueError:
                continue  # not ours; a worker's own prints go to stderr

            kind = envelope.get("t")
            if kind == "reply":
                with self._lock:
                    waiting = self._pending.pop(envelope.get("id", 0), None)
                if waiting is not None:
                    waiting.put(envelope)
                elif envelope.get("e"):
                    # Unasked for: the host could not start at all, and said
                    # so before anyone had sent it anything.
                    self._startup_error = envelope

            elif kind == "event":
                payload = envelope.get("p")
                with self._lock:
                    self._latest = payload
                    subscribers = list(self._subscribers)
                for subscriber in subscribers:
                    subscriber.put(payload)

            elif kind == "state":
                payload = envelope.get("p") or {}
                with self._lock:
                    self._state = payload.get("state", self._state)
                    self._restarts = payload.get("restarts", self._restarts)

        # The host has gone: wake everything waiting on it rather than
        # leaving a frontend blocked for good.
        with self._lock:
            self._stopped = True
            waiting = list(self._pending.values())
            subscribers = list(self._subscribers)
            self._pending.clear()
        for slot in waiting:
            slot.put({"e": "the worker host has stopped", "c": "not_running"})
        for subscriber in subscribers:
            subscriber.put(_CLOSED)

    def _ask(self, kind: str, name: str, payload: Any) -> Any:
        """Write one request and wait for its reply."""
        if self._stopped:
            raise NotRunning("the worker host has stopped")

        slot: queue.Queue = queue.Queue(maxsize=1)
        with self._lock:
            self._next_id += 1
            request_id = self._next_id
            self._pending[request_id] = slot

            request = {"id": request_id}
            if kind:
                request["t"] = kind
            if name:
                request["n"] = name
            if payload is not None:
                request["p"] = payload
            try:
                self._process.stdin.write((json.dumps(request) + "\n").encode())
                self._process.stdin.flush()
            except (BrokenPipeError, ValueError) as broken:
                self._pending.pop(request_id, None)
                raise NotRunning("the worker host has stopped") from broken

        envelope = slot.get()
        message = envelope.get("e")
        if message is not None:
            raise _ERRORS.get(envelope.get("c"), VeroError)(message)
        return envelope.get("p")

    def send(self, request: Any) -> Any:
        """Send a request and wait for the reply.

        Returns None when the worker chose not to answer, which is normal for
        a request that only causes an action.  There is no timeout: a worker
        may hold a request for as long as the work takes, so call this off
        whatever thread draws your frontend.
        """
        return self._ask("", "", request)

    def call(self, name: str, request: Any) -> Any:
        """Send a request to one named handler, matching vero.Handle on the worker.

        The same as send(), except the worker routes on the name rather than on
        something inside the request, so neither side has to agree on a "type"
        field.
        """
        return self._ask("", name, request)

    def latest(self) -> Any:
        """The most recent event, without waiting for the next one.

        Use it to draw a window that has just opened; events() keeps it up to
        date afterwards.
        """
        with self._lock:
            if self._latest is not None:
                return self._latest
        return self._ask("ctl", "latest", None)

    def state(self) -> str:
        """"starting", "running", "restarting" or "stopped"."""
        with self._lock:
            return self._state

    def restarts(self) -> int:
        """How many times the worker has been restarted after dying."""
        with self._lock:
            return self._restarts

    def events(self) -> Iterator[Any]:
        """Yield every state change the worker reports, as it happens.

        This blocks between events, so run it on its own thread and hand each
        one to your frontend's main thread - GLib.idle_add under GTK.  There
        is no polling and no interval to choose: the worker sends one when
        something changes and nothing while it is quiet.
        """
        subscriber: queue.Queue = queue.Queue()
        with self._lock:
            if self._stopped:
                return
            self._subscribers.append(subscriber)
        try:
            while True:
                event = subscriber.get()
                if event is _CLOSED:
                    return
                yield event
        finally:
            with self._lock:
                if subscriber in self._subscribers:
                    self._subscribers.remove(subscriber)

    def stop(self) -> None:
        """Stop the worker.

        Not required - the worker's standard input closes when this process
        exits and it stops with it, crash included - but it ends the work a
        moment sooner.
        """
        if self._stopped:
            return
        try:
            self._ask("ctl", "stop", None)
        except VeroError:
            pass  # it is going away; how it went is not interesting

        self._stopped = True
        try:
            self._process.stdin.close()
        except (BrokenPipeError, ValueError):
            pass
        try:
            self._process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            self._process.kill()
            self._process.wait()

        # The reader has reached the end of this by now, or will never read
        # from it again.
        try:
            self._process.stdout.close()
        except (OSError, ValueError):
            pass

    def __enter__(self) -> "Vero":
        return self

    def __exit__(self, *exc: object) -> None:
        self.stop()


# A sentinel rather than None, which is a payload a worker may legitimately
# send.
_CLOSED = object()


def run_in_thread(vero: Vero, on_event) -> threading.Thread:
    """Convenience: read events on a daemon thread, calling on_event for each.

    on_event runs on that thread, not your frontend's, so hop across before
    touching any widget.
    """
    def loop() -> None:
        for event in vero.events():
            on_event(event)

    thread = threading.Thread(target=loop, daemon=True)
    thread.start()
    return thread
