# Write test steps

*Step 9 of 15 in [the vero guide](README.md).*

A vero test installs the app on a system, opens its window, and then
does what a person would: sends requests, waits for the state to change,
and lets the recording show the result. The steps that do this are
`[[test.step]]` entries in `vero-app.toml`. vero's library inside each
front end plays them, so the window shows each step as it happens.

## An example

```toml
[test]
files = ["testdata/sample.mp3"]     # copied onto the system, into {files}
env = { VERO_PROFILE = "test" }     # a copy of the app apart from the person's own
record = true                       # the full run records each system

[[test.step]]
call = "setLibrary"                 # a request, by name
with = { path = "{tmp}" }           # what it carries; {tmp} is a new empty folder

[[test.step]]
copy = { from = "sample.mp3", to = "{tmp}/in/" }

[[test.step]]
wait = { path = "items.#", at_least = 1 }   # until the state shows an item
timeout = 300                               # seconds; the default is shorter

[[test.step]]
pause = 3                           # so that the recording shows the result
```

## The kinds of step

| Step | What it does |
|---|---|
| `call = "name"`, `with = {...}` | Sends the request `name` with the data in `with`, and waits for its reply. |
| `wait = { path = "...", ... }` | Waits until a value in the state matches. `timeout` is in seconds. |
| `copy = { from = "...", to = "..." }` | Copies one of the files in `[test] files` to a path on the system. A `to` ending in `/` is a folder. |
| `pause = N` | Does nothing for N seconds, so that the recording shows a moment. |
| `send = {...}` | Sends a raw message without a request name, for a worker that routes its own messages. Rare. |

A `wait` has a `path` and one condition: `is`, `not`, `at_least` or
`contains`. The path walks the state by field name and list index, and
`#` is a list's length. `jobs.0.phase` is the phase of the first job, and
`jobs.#` is how many jobs there are.

## Placeholders

- `{tmp}` is a new empty folder, made for this test run.
- `{files}` is the folder that holds the files listed in `[test] files`.

## What stops a test

The test stops at the first step that fails: a request that the worker
refuses, or a wait that runs out of time. The step's name and the error
are in the log, and the recording shows the window up to that point.

## The Mac

The Mac is recorded only when `[test] env` sets `VERO_PROFILE`. That is
a safety check: a recording on the Mac runs the app as you, and the
profile is what stops it touching the settings and secrets of the copy
you use.

---

Next: [Test on Linux and the BSDs](test-on-linux-and-the-bsds.md).
