# The WASI example

A worker that is not a native program: one `worker.wasm`, run by a WASI
runtime, driven from a terminal.

```bash
scripts/run-wasi.sh
```

```
  1  Photos       uploading            [######################  ]
  2  Documents    waiting              [                        ]
  3  Team share   scanning             [############            ]
  advance 1
```

Type `1`, `2` or `3` to move that job on, `r1` to restart one, and `q`
to leave.

## Why you might want this

A WASI worker is a single `worker.wasm` that a runtime such as wasmtime
starts, instead of an executable built for each machine. Two reasons to
want that:

- **One file for every CPU.** The same `worker.wasm` runs on amd64, arm64
  and riscv64, on macOS, Linux and Windows. No build matrix.
- **The worker is boxed in.** A WASI guest sees only what the runtime is
  told to give it, and it cannot start anything. If the worker runs code
  you did not write, such as a customer's rules or a plugin, that
  boundary is the point.

vero makes the swap cheap, because the worker was already a separate
process speaking JSON over a pipe. One line changes:

```go
sup := vero.Supervise(vero.SupervisorOptions{
    Path: "wasmtime",
    Args: []string{"run", "--env", "VERO_SERVE=1", "worker.wasm"},
    Lock: "worker.wasm",   // Path is the runtime here, so key the lock on the worker
})
```

`--env` is there because a WASI runtime passes the guest nothing it is
not told to, and `VERO_SERVE` is how a worker knows it is supervised.

## What WASI takes away

- **The worker only runs while it is answering.** WASI gives the
  instance one thread. While the worker waits for its next request,
  nothing else inside it runs: no timers, no goroutines, no state pushed
  out on its own. That is why this example has a worker of its own
  rather than sharing [../worker](../worker), whose jobs advance in a
  goroutine. [worker](worker) does its work inside the handlers instead.
  Ask it to advance a job and it advances. Leave it alone and nothing
  happens.
- **One request at a time.** For the same reason, a WASI worker answers
  in the goroutine that read the request, so a slow handler holds up the
  one behind it.
- **No window.** WASI has no screen. The front end is whatever program
  starts the runtime, which here is a terminal.
- **Go front ends only.** The Python, C# and Kotlin bindings start the
  worker in host mode, where it supervises a second copy of itself, and
  a WASI guest cannot start anything.

## The files

| | |
|---|---|
| [main.go](main.go) | the front end: three rows in a terminal, redrawn in place |
| [worker/main.go](worker/main.go) | the worker, doing its work in the handlers |
| [../../scripts/run-wasi.sh](../../scripts/run-wasi.sh) | builds both and runs them |
| [../../wasi_test.go](../../wasi_test.go) | the same thing as a test, skipped without `wasmtime` |
