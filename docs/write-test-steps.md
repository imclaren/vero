# Write test steps

`[[test.step]]` entries in `vero-app.toml` say what a test does once the
window is open. vero's library in each front end plays them, so each
recording shows what a person would see.

```toml
[test]
files = ["testdata/sample.mp3"]     # copied onto the system, into {files}
env = { VERO_PROFILE = "test" }     # a copy apart from the person's own

[[test.step]]
call = "setLibrary"                 # a request, by name
with = { path = "{tmp}" }           # {tmp} is a new empty folder

[[test.step]]
wait = { path = "items.#", at_least = 1 }   # or is, not, contains
timeout = 300

[[test.step]]
pause = 3                           # so that the recording shows it
```

A `copy` step copies one of the files, as in
`copy = { from = "sample.mp3", to = "{tmp}/in/" }`. A wait's `path` walks the state by key and list index,
and `#` is a list's length, as in `jobs.0.phase`. The test stops at the
first step that fails. The Mac is recorded only when `[test] env` sets
`VERO_PROFILE`.
