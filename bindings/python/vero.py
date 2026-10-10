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
import shutil
import subprocess
import sys
import tempfile
import threading
import time
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

        # vero's tests set VERO_TEST to a file of steps, which are played
        # against the worker while the window shows what they do.
        if os.environ.get("VERO_TEST"):
            threading.Thread(target=play, args=(self, os.environ["VERO_TEST"]), daemon=True).start()

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


def play(vero: Vero, steps_file: str) -> bool:
    """Plays the steps vero's tests give an installed app, while its window
    is open: what vero-app.toml's [[test.step]]s say, as JSON. Each step is
    one of

        {"call": "name", "with": {...}}   a request to a named handler
        {"send": {...}}                    a request without a name
        {"wait": {"path": "jobs.0.phase", "is": "waiting"}, "timeout": 60}
        {"pause": 2}                       seconds, so a recording shows it
        {"copy": {"from": "sample.mp3", "to": "{tmp}/library/imports/"}}

    A wait's path goes through the state by key and list index, and "#" is a
    list's length; it can be "is", "not", "at_least" or "contains". In any
    string, {tmp} is a new empty folder and {files} the folder of the test's
    files, VERO_TEST_FILES. How it went is written as JSON to
    VERO_TEST_RESULT, and a line for each step to standard error. True if
    every step passed; it stops at the first that doesn't.
    """
    started = time.time()
    folders = {"tmp": tempfile.mkdtemp(prefix="vero-test-"),
               "files": os.environ.get("VERO_TEST_FILES", "")}
    results: list[dict] = []
    ok = True
    events: queue.Queue = queue.Queue()
    with vero._lock:
        vero._subscribers.append(events)
    try:
        with open(steps_file) as f:
            steps = json.load(f).get("steps", [])
        # The worker up, with a state to show, as the window waits for too.
        deadline = time.time() + 60
        while vero.state() != "running":
            if time.time() > deadline:
                raise VeroError(f"the worker wasn't running after 60s ({vero.state()})")
            time.sleep(0.05)
        vero.latest()
        for n, step in enumerate(steps, 1):
            step = _fill(step, folders)
            began = time.time()
            what = _describe(step)
            try:
                _step(vero, step, events)
                results.append({"step": what, "seconds": round(time.time() - began, 2)})
                print(f"vero test: step {n}/{len(steps)}, {what}: ok ({time.time() - began:.1f}s)",
                      file=sys.stderr, flush=True)
            except Exception as e:  # noqa: BLE001 - any failure is the step's
                ok = False
                results.append({"step": what, "seconds": round(time.time() - began, 2), "error": str(e)})
                print(f"vero test: step {n}/{len(steps)}, {what}: FAILED: {e}", file=sys.stderr, flush=True)
                break
    except Exception as e:  # noqa: BLE001 - the file, or the worker
        ok = False
        results.append({"step": "starting", "error": str(e)})
        print(f"vero test: {e}", file=sys.stderr, flush=True)
    finally:
        with vero._lock:
            if events in vero._subscribers:
                vero._subscribers.remove(events)
    out = os.environ.get("VERO_TEST_RESULT")
    if out:
        with open(out + ".part", "w") as f:
            json.dump({"ok": ok, "seconds": round(time.time() - started, 2), "steps": results}, f, indent=1)
        os.replace(out + ".part", out)
    return ok


def _fill(value: Any, folders: dict[str, str]) -> Any:
    """value with {tmp} and {files} replaced, in every string in it."""
    if isinstance(value, str):
        for name, folder in folders.items():
            value = value.replace("{" + name + "}", folder)
        return value
    if isinstance(value, list):
        return [_fill(v, folders) for v in value]
    if isinstance(value, dict):
        return {k: _fill(v, folders) for k, v in value.items()}
    return value


def _describe(step: dict) -> str:
    if "call" in step:
        return "call " + step["call"]
    if "send" in step:
        return "send " + json.dumps(step["send"])
    if "wait" in step:
        w = step["wait"]
        op = next((k for k in ("is", "not", "at_least", "contains") if k in w), "is")
        return f"wait until {w.get('path')} {op.replace('_', ' ')} {json.dumps(w.get(op))}"
    if "pause" in step:
        return f"pause {step['pause']}s"
    if "copy" in step:
        return f"copy {step['copy'].get('from')} to {step['copy'].get('to')}"
    return "unknown step " + json.dumps(step)


def _step(vero: Vero, step: dict, events: queue.Queue) -> None:
    if "call" in step:
        vero.call(step["call"], step.get("with"))
    elif "send" in step:
        vero.send(step["send"])
    elif "pause" in step:
        time.sleep(float(step["pause"]))
    elif "copy" in step:
        c = step["copy"]
        src = c["from"] if os.path.isabs(c["from"]) else os.path.join(os.environ.get("VERO_TEST_FILES", ""), c["from"])
        dest = c["to"]
        if dest.endswith("/"):
            os.makedirs(dest, exist_ok=True)
            dest = os.path.join(dest, os.path.basename(src))
        else:
            os.makedirs(os.path.dirname(dest) or ".", exist_ok=True)
        shutil.copy(src, dest)
    elif "wait" in step:
        w = step["wait"]
        deadline = time.time() + float(step.get("timeout", 60))
        state = vero.latest()
        while not _holds(state, w):
            left = deadline - time.time()
            if left <= 0:
                raise VeroError(f"after {step.get('timeout', 60)}s, {w.get('path')} is {json.dumps(_at(state, w.get('path', '')))}")
            try:
                state = events.get(timeout=min(left, 1))
            except queue.Empty:
                state = vero.latest()
            if state is _CLOSED:
                raise NotRunning("the worker host has stopped")
    else:
        raise VeroError("a step is call, send, wait, pause or copy")


def _at(state: Any, path: str) -> Any:
    """What path names in state: keys and list indexes, "#" for a length."""
    for part in [p for p in path.split(".") if p]:
        if part == "#" and isinstance(state, (list, dict, str)):
            state = len(state)
        elif isinstance(state, list) and part.lstrip("-").isdigit() and -len(state) <= int(part) < len(state):
            state = state[int(part)]
        elif isinstance(state, dict) and part in state:
            state = state[part]
        else:
            return None
    return state


def _holds(state: Any, wait: dict) -> bool:
    got = _at(state, wait.get("path", ""))
    if "is" in wait:
        return got == wait["is"]
    if "not" in wait:
        return got != wait["not"]
    if "at_least" in wait:
        return isinstance(got, (int, float)) and not isinstance(got, bool) and got >= wait["at_least"]
    if "contains" in wait:
        return got is not None and wait["contains"] in got
    raise VeroError("a wait is is, not, at_least or contains")
