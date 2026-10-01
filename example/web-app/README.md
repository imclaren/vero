# The browser example

The same three jobs as every other example, in a page:

```bash
./build.sh
go run serve.go     # http://localhost:8080
```

![the browser example](../../docs/screenshots/web.gif)

`scripts/run-web.sh` does both and opens a browser at it.

## What is different here

Everywhere else, the frontend starts the worker as a separate program and
talks to it over a pipe. A page cannot start a program, so this example does
not: `SupervisorOptions.Serve` runs the worker on a goroutine of this same
wasm module, joined to the supervisor by a pipe in memory.

```go
sup := vero.Supervise(vero.SupervisorOptions{
    Serve:   serve,       // the worker, in this process
    OnEvent: ui.apply,    // state, as it changes
})
```

Above that pipe nothing changes. The frontend still calls `restartJob` and
gets the new status back, still redraws from events it did not ask for, and
`serve` is the worker from [../worker](../worker) reading the streams it is
given instead of standard input and output. It is the mode iOS needs, for the
same reason: there, too, an application may not start a program.

## The files

| | |
|---|---|
| [main.go](main.go) | both halves: the DOM frontend, then the worker it supervises |
| [index.html](index.html) | the page, the styles, and the three lines that load the module |
| [build.sh](build.sh) | builds `main.wasm` and copies Go's `wasm_exec.js` beside it |
| [serve.go](serve.go) | a static server, because `.wasm` has to arrive as `application/wasm` |

`main.wasm` is about 5MB, which is what a Go runtime costs in a browser. It
compresses to roughly a third of that, so serve it with gzip or brotli.
