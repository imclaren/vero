# Add a request

*Step 5 of 15 in [the vero guide](README.md).*

A request is a message that the window sends to the worker, such as
"rename job 3 to Photos". The worker answers it with a handler, a Go
function registered under the request's name. This page adds one.

## 1. Write the handler in the worker

In `cmd/worker/main.go`, define a struct for what the request carries, and
register a handler that changes the state:

```go
type Rename struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

vero.UpdateWith(state, "rename", func(s *Status, req Rename) error {
	return vero.Edit(s.Jobs, req.ID, func(j *Job) { j.Name = req.Name })
})
```

Every field needs a `json` tag. That tag is the name the front ends use.

There are four ways to register a handler. Pick by what the request
carries and what it should reply with.

| Function | The request carries | The reply is |
|---|---|---|
| `vero.Update` | nothing | the new state |
| `vero.UpdateWith` | a struct of your own | the new state |
| `vero.UpdateItem` | the key of one item in a list | the new state |
| `vero.Handle` | a struct of your own | a value of your own |

The first three run while holding the state's lock and reply with the
state as it is after the handler ran. That is almost always what you
want: the window gets the result of its own action straight back.

`vero.Handle` is for a request whose answer is not the state, such as a
page of search results or the contents of a file. It gets a context:

```go
vero.Handle(w, "search", func(ctx context.Context, req Search) (Results, error) {
	...
})
```

The context is cancelled when the front end stops waiting, for example
when the view that asked has been closed. A slow handler should check
`ctx.Done()` and give up.

If a handler returns an error, the error's message goes back to the front
end, which shows it. Nothing else happens. The worker keeps running.

## 2. Regenerate the bindings

```bash
vero gen
```

This reads the worker's Go types and rewrites `WorkerAPI.swift`,
`worker_api.py` and `WorkerApi.cs`. Each now has a typed `Rename` request
whose reply is typed as the state. `vero gen --check` fails if any of the
three is out of date, which is useful before a commit.

## 3. Call it from each front end

In SwiftUI, from a button:

```swift
vero.call(RenameRequest(id: 1, name: "Photos"))
```

or, where you can await, `let status = try await vero.call(RenameRequest(...))`.

In the GTK app, through the generated `Worker` class:

```python
status = Worker(app.vero).rename(id=1, name="Photos")
```

In WPF, as an extension method on the client:

```csharp
var status = await vero.RenameAsync(new RenameRequest { Id = 1, Name = "Photos" });
```

You usually do not need the returned state. The worker also pushes it as
an event, and the window redraws from that.

## Related

- [Push state](push-state.md), for how the state reaches the window.
- [How vero works](how-vero-works.md), for the protocol underneath.

---

Next: [Add a screen](add-a-screen.md).
