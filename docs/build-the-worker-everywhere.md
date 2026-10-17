# Build the worker for every system

*Step 8 of 15 in [the vero guide](README.md).*

The worker is built from the Mac for every system and processor that the
app ships to, with cgo turned off. No cgo means no C compiler for each
target and no C library to find on each system. It is the reason one Mac
can build for all of them. The cost is that a worker cannot use any Go
package that needs C.

## Find out what fails

```bash
vero ship --plan
```

This builds the worker for every target and names each one that fails,
with the reason. `PORTING.md`, which `vero new` writes, also lists the
places in the worker that need attention, with files and lines.

## The usual causes, and what to do

- **SQLite.** `mattn/go-sqlite3` needs cgo, and `modernc.org/sqlite`
  lacks ports for DragonFly, illumos, NetBSD on ARM64 and OpenBSD on
  RISC-V. Use `github.com/ncruces/go-sqlite3`, which builds everywhere.
  Two notes on it: register full text search with `ext/fts5` on each
  connection, and in a sandboxed Mac app set `PRAGMA temp_store=MEMORY`,
  or a write that needs a temporary file fails with "disk I/O error".
- **Code that only makes sense on one system**, such as reading the
  keychain or stopping the machine sleeping. Use the
  [kit](../kit/README.md), which has one API for every system. If the kit
  has nothing for it, give your code a build tag and write a version for
  the other systems that does nothing.
- **System calls whose shape differs**, such as `syscall.Rlimit`. Use
  `golang.org/x/sys` instead of the `syscall` package.
- **Helper programs the worker runs**, such as ffmpeg. Ship a build for
  each system, list it in `resources` in `vero-app.toml`, and find it at
  run time with `kit/tools`.

## If the Mac really needs cgo

Set `cgo = true` under `[worker]` in `vero-app.toml`. The Mac's worker is
then built with cgo and Xcode's clang, and every other system's without
it. The code that needs cgo must be behind a build tag so that the other
builds still compile.

---

Next: [Write test steps](write-test-steps.md).
