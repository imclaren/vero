"""Tests for the Python binding, against a real worker process.

    cd bindings/python && python3 -m unittest -v

Builds the example worker first, so what is exercised is the whole stack: the
host a worker becomes when VERO_HOST is set, the supervisor inside it, and a
worker that is genuinely a separate process.
"""

import os
import subprocess
import sys
import tempfile
import threading
import time
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import json  # noqa: E402

from vero import NotRunning, Refused, Vero, play  # noqa: E402

REPO = os.path.abspath(os.path.join(os.path.dirname(__file__), "../.."))


def build(tmp: str) -> tuple[str, str]:
    worker = os.path.join(tmp, "worker")
    subprocess.run(["go", "build", "-o", worker, "./bindings/python/testdata/jobsworker"],
                   cwd=REPO, check=True)
    router = os.path.join(tmp, "routerworker")
    subprocess.run(
        ["go", "build", "-o", router, "./bindings/python/testdata/routerworker"],
        cwd=REPO, check=True,
    )
    return worker, router


class VeroTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.tmp = tempfile.mkdtemp()
        cls.worker, cls.router = build(cls.tmp)

    def setUp(self) -> None:
        self.vero = Vero(self.worker)
        self.addCleanup(self.vero.stop)
        deadline = time.time() + 5
        while time.time() < deadline:
            if self.vero.state() == "running":
                return
            time.sleep(0.01)
        self.fail("the worker never came up")

    def test_request_and_reply(self) -> None:
        status = self.vero.send({"type": "status"})
        self.assertEqual(len(status["jobs"]), 3)

    def test_a_refusal_carries_the_workers_own_message(self) -> None:
        with self.assertRaises(Refused) as caught:
            self.vero.send({"type": "restart", "id": 99})
        self.assertEqual(str(caught.exception), "no job with id 99")

    def test_an_unknown_request_is_refused_not_ignored(self) -> None:
        with self.assertRaises(Refused) as caught:
            self.vero.send({"type": "nonsense"})
        self.assertIn('unknown request type: "nonsense"', str(caught.exception))

    def test_events_arrive_without_being_asked(self) -> None:
        seen: list = []
        threading.Thread(
            target=lambda: [seen.append(e) for e in self.vero.events()],
            daemon=True,
        ).start()
        deadline = time.time() + 5
        while time.time() < deadline:
            if len(seen) >= 3:
                self.assertIn("jobs", seen[0])
                return
            time.sleep(0.02)
        self.fail(f"only {len(seen)} events arrived")

    def test_latest_is_available_before_any_event_is_awaited(self) -> None:
        deadline = time.time() + 5
        while time.time() < deadline:
            if self.vero.latest() is not None:
                return
            time.sleep(0.02)
        self.fail("latest stayed empty")

    def test_restarting_a_real_job_succeeds(self) -> None:
        status = self.vero.send({"type": "restart", "id": 1})
        job = next(j for j in status["jobs"] if j["id"] == 1)
        self.assertEqual(job["progress"], 0)

    def test_a_slow_request_does_not_block_the_ones_behind_it(self) -> None:
        """A long poll must not stop every other request from starting.

        The worker holds a poll open until something changes, which can be
        minutes. If the binding serialises requests, the first poll stops the
        frontend answering any button until it happens to return - and the
        symptom is an app that looks like it ignores clicks, a long way from
        the cause. The Swift binding had exactly this bug.
        """
        results: list = []

        def slow() -> None:
            # "status" returns at once, so use enough of them to keep the
            # transport busy while the timed request goes out.
            for _ in range(40):
                self.vero.send({"type": "status"})
            results.append("slow done")

        thread = threading.Thread(target=slow, daemon=True)
        thread.start()
        time.sleep(0.05)

        start = time.time()
        self.vero.send({"type": "status"})
        waited = time.time() - start
        thread.join(timeout=10)

        self.assertLess(waited, 1.0,
                        f"a request waited {waited:.2f}s behind others; requests are serialised")

    def test_requests_after_stop_fail(self) -> None:
        self.vero.stop()
        with self.assertRaises((NotRunning, Refused)):
            self.vero.send({"type": "status"})


class NoWorkerTests(unittest.TestCase):
    def test_a_missing_worker_reports_not_running(self) -> None:
        with self.assertRaises((OSError, NotRunning)):
            # Nothing to host and nothing to supervise: the spawn itself
            # fails, which is a clearer answer than a wait that never ends.
            Vero("/nonexistent/worker").send({"type": "status"})


class NamedRouteTests(unittest.TestCase):
    """call() reaches a handler registered with vero.Handle, by name."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.tmp = tempfile.mkdtemp()
        _, cls.router = build(cls.tmp)

    def setUp(self) -> None:
        self.vero = Vero(self.router)
        self.addCleanup(self.vero.stop)
        deadline = time.time() + 5
        while time.time() < deadline:
            if self.vero.state() == "running":
                return
            time.sleep(0.01)
        self.fail("the worker never came up")

    def test_call_routes_on_the_name(self) -> None:
        self.assertEqual(
            self.vero.call("status", {}),
            {"jobs": ["Photos", "Documents"], "working": True},
        )

    def test_call_carries_the_request_body(self) -> None:
        self.assertEqual(self.vero.call("restartJob", {"id": 7}), {"restarted": 7})

    def test_an_unrouted_name_is_refused(self) -> None:
        with self.assertRaises(Refused):
            self.vero.call("nosuch", {})

    def test_a_handler_error_is_a_refusal(self) -> None:
        with self.assertRaises(Refused):
            self.vero.call("restartJob", {"id": 0})


class PlayTests(unittest.TestCase):
    """The steps vero's tests play against an installed app."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.tmp = tempfile.mkdtemp()
        cls.worker, _ = build(cls.tmp)

    def play(self, steps: list, files: str = "") -> tuple[bool, dict]:
        stepfile = os.path.join(self.tmp, "steps.json")
        result = os.path.join(self.tmp, "result.json")
        with open(stepfile, "w") as f:
            json.dump({"steps": steps}, f)
        os.environ["VERO_TEST_RESULT"] = result
        os.environ["VERO_TEST_FILES"] = files
        self.addCleanup(os.environ.pop, "VERO_TEST_RESULT", None)
        self.addCleanup(os.environ.pop, "VERO_TEST_FILES", None)
        with Vero(self.worker) as v:
            ok = play(v, stepfile)
        with open(result) as f:
            return ok, json.load(f)

    def test_steps_that_hold_pass(self) -> None:
        ok, result = self.play([
            {"wait": {"path": "jobs.#", "is": 3}},
            {"send": {"type": "restart", "id": 1}},
            {"wait": {"path": "jobs.0.id", "at_least": 1}},
            {"wait": {"path": "jobs.0.name", "not": ""}},
            {"pause": 0.1},
        ])
        self.assertTrue(ok, result)
        self.assertEqual(len(result["steps"]), 5)
        self.assertNotIn("error", json.dumps(result))

    def test_a_wait_that_never_holds_fails_with_what_it_saw(self) -> None:
        ok, result = self.play([
            {"wait": {"path": "jobs.#", "is": 4}, "timeout": 1},
            {"pause": 0.1},
        ])
        self.assertFalse(ok)
        self.assertEqual(len(result["steps"]), 1, "it stops at the first failure")
        self.assertIn("jobs.# is 3", result["steps"][0]["error"])

    def test_a_refused_request_fails_the_step(self) -> None:
        ok, result = self.play([{"send": {"type": "restart", "id": 99}}])
        self.assertFalse(ok)
        self.assertIn("no job with id 99", result["steps"][0]["error"])

    def test_copy_fills_in_the_folders(self) -> None:
        files = tempfile.mkdtemp()
        with open(os.path.join(files, "sample.txt"), "w") as f:
            f.write("hello")
        marker = os.path.join(self.tmp, "where")
        ok, result = self.play([
            {"copy": {"from": "sample.txt", "to": "{tmp}/a/b/"}},
            {"copy": {"from": "{tmp}/a/b/sample.txt", "to": marker}},
        ], files)
        self.assertTrue(ok, result)
        with open(marker) as f:
            self.assertEqual(f.read(), "hello")

    def test_vero_test_plays_the_steps_by_itself(self) -> None:
        stepfile = os.path.join(self.tmp, "auto.json")
        result = os.path.join(self.tmp, "auto-result.json")
        with open(stepfile, "w") as f:
            json.dump({"steps": [{"wait": {"path": "jobs.#", "is": 3}}]}, f)
        os.environ.update(VERO_TEST=stepfile, VERO_TEST_RESULT=result)
        self.addCleanup(os.environ.pop, "VERO_TEST", None)
        self.addCleanup(os.environ.pop, "VERO_TEST_RESULT", None)
        with Vero(self.worker):
            deadline = time.time() + 10
            while not os.path.exists(result) and time.time() < deadline:
                time.sleep(0.05)
        with open(result) as f:
            self.assertTrue(json.load(f)["ok"])


if __name__ == "__main__":
    unittest.main()
