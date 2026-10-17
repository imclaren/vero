# Build the worker for every system

The worker is built without cgo, for every system and processor, from the
Mac. `vero ship --plan` builds it for each and names each one that fails
and why. `PORTING.md` lists what in the worker needs attention, with files
and lines. These are the usual causes.

- **SQLite.** Use `github.com/ncruces/go-sqlite3`, which builds everywhere.
  `mattn/go-sqlite3` needs cgo, and `modernc.org/sqlite` has no port for
  DragonFly, illumos, NetBSD on arm64 or OpenBSD on riscv64. Register full
  text search with `ext/fts5` on each connection. A sandboxed Mac app also
  needs `PRAGMA temp_store=MEMORY`, or a write that needs a temporary file
  fails with "disk I/O error".
- **Code for one system**, such as a keychain or a sleep notifier. Use the
  [kit](../kit/README.md), or give the code a build tag and a version for the
  other systems that does nothing.
- **System calls that differ**, such as `syscall.Rlimit`. Use
  `golang.org/x/sys`.
- **Programs that the worker runs.** Ship a build for each system, list it
  in `resources`, and find it with `kit/tools`.

A worker that needs cgo on the Mac sets `cgo = true` in `[worker]`, and is
built with Xcode's clang there and without cgo everywhere else.
