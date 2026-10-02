# The WASI example

A worker that is not a program:

```bash
scripts/run-wasi.sh
```

```
  1  Photos       uploading            [######################  ]
  2  Documents    waiting              [                        ]
  3  Team share   scanning             [############            ]
  advance 1
```

`1`, `2` or `3` moves that job on. `r1` restarts one. `q` leaves.

## When you would use this

The worker is a single `worker.wasm` that a WASI runtime starts, rather than
an executable built for the machine it runs on. Two reasons to want that:

* **One file for every CPU.** The same `worker.wasm` runs on amd64, arm64 and
  riscv64, on macOS, Linux and Windows. No build matrix, no per-architecture
  release.
* **The worker is boxed in.** A WASI guest sees only what the runtime is told
  to give it - no files here, because none are named on the command line -
  and it cannot start anything. If the worker runs logic you did not write, a
  customer's rules or somebody's plugin, that boundary is the point.

vero makes the swap cheap, because the worker was already a separate process
speaking JSON over a pipe. One line changes:

```go
sup := vero.Supervise(vero.SupervisorOptions{
    Path: "wasmtime",
    Args: []string{"run", "--env", "VERO_SERVE=1", "worker.wasm"},
    Lock: "worker.wasm",   // Path is the runtime here, so key the lock on the worker
})
```

`--env` is there because a WASI runtime passes the guest nothing it is not
told to, and `VERO_SERVE` is how a worker knows it is supervised.

## What WASI takes away

**The worker only runs while it is answering.** WASI gives the instance one
thread, and while the worker waits for its next request on standard input
nothing else inside it runs: no timers, no goroutines, no state pushed out on
its own. That is why this example has a worker of its own rather than sharing
[../worker](../worker) with the other five - that one moves its jobs along in
a goroutine, and here that goroutine would never run at all. [worker](worker)
does its work in the handlers instead. Ask it to advance a job and it
advances; leave it alone and nothing happens.

**One request at a time.** For the same reason, vero cannot hand each request
to a goroutine here, as it does everywhere else: the reply would be computed
and then sit unsent until the next request arrived. A WASI worker answers in
the goroutine that read the request, so a slow handler holds up the one
behind it.

**No window.** WASI has no screen. The frontend is whatever program starts the
runtime, which is why this one is a terminal.

**Go only.** The Python, C# and Kotlin bindings cannot drive a WASI worker.
They start it in host mode, where the worker supervises a second copy of
itself, and a WASI guest cannot start anything:

```
vero: cannot find this executable: Executable not implemented for wasip1
```

## The files

| | |
|---|---|
| [main.go](main.go) | the frontend: three rows in a terminal, redrawn in place |
| [worker/main.go](worker/main.go) | the worker, doing its work in the handlers |
| [../../scripts/run-wasi.sh](../../scripts/run-wasi.sh) | builds both and runs them |
| [../../wasi_test.go](../../wasi_test.go) | the same thing as a test, skipped without `wasmtime` |
