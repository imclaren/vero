# The browser example

The same three jobs as every other example, in a web page.

```bash
./build.sh
go run serve.go     # http://localhost:8080
```

![the browser example](../../docs/screenshots/web.gif)

`scripts/run-web.sh`, from the top of the checkout, does both and opens
a browser at the page.

## What is different here

Everywhere else, the front end starts the worker as a separate program
and talks to it over a pipe. A web page cannot start a program. So in
this example the worker is compiled into the same wasm module as the
front end, and runs on a goroutine. The pipe between them is in memory:

```go
sup := vero.Supervise(vero.SupervisorOptions{
    Serve:   serve,       // the worker, in this process
    OnEvent: ui.apply,    // the state, as it changes
})
```

Above that pipe nothing changes. The front end still calls `restartJob`
and gets the new state back, and still redraws when a state event
arrives. `serve` is the worker from [../worker](../worker), reading the
streams it is given instead of standard input and output. iOS works the
same way, for the same reason.

## The files

| | |
|---|---|
| [main.go](main.go) | both halves: the DOM front end, then the worker it supervises |
| [index.html](index.html) | the page, the styles, and the three lines that load the module |
| [build.sh](build.sh) | builds `main.wasm` and copies Go's `wasm_exec.js` beside it |
| [serve.go](serve.go) | a static server, because `.wasm` has to arrive as `application/wasm` |

`main.wasm` is about 5 MB, which is what a Go runtime costs in a browser.
It compresses to about a third of that, so serve it with gzip or brotli.
