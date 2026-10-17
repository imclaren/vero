# The worker

The Go program behind every example. It pretends to sync three folders,
moving each through a few phases with a progress bar, so that the front
ends have something to show. The same binary drives the macOS, Windows,
Linux, Android, browser and Plan 9 examples.

```bash
go build -o worker .
./worker            # on its own, logging to standard error
./worker -json      # the state, as JSON, on standard output whenever it changes
./worker -version
```

It handles two requests:

| Name | Takes | Replies with |
|---|---|---|
| `status` | nothing | the current `Status` |
| `restartJob` | `{"id": 1}` | the new `Status` |

It pushes the state whenever it changes, so no front end polls.
[main.go](main.go) is short and commented, and is the shape of the
worker that `vero new` writes.
