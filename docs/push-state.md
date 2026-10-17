# Push state

*Step 4 of 15 in [the vero guide](README.md).*

The worker keeps everything the window shows in one Go struct, the
state. vero sends the whole struct to every window each time its JSON
changes. The window never asks for it and never polls. This page shows
how to change the state from the worker's own work.

## The state struct

```go
type Status struct {
	Items []Item `json:"items"`
	Now   string `json:"now"`
}

state := vero.NewState(w, Status{})
```

Every field needs a `json` tag, and only exported fields reach the
window. A worker has one state. After you change the struct, run
`vero gen` so that the front ends' bindings match it.

## Changing it

The state is shared between your code and vero's encoder, so it sits
behind a lock. Change it only inside a function that holds the lock,
which is any of these:

- A handler registered with `vero.Update`, `vero.UpdateWith` or
  `vero.UpdateItem`.
- `state.Do`, from anywhere in the worker.
- `state.Every`, for work on a timer.

```go
// Work of the worker's own, in a goroutine. Each result is written back
// with state.Do, which takes the lock for the moment it takes.
go func() {
	for item := range results {
		state.Do(func(s *Status) { s.Items = append(s.Items, item) })
	}
}()

// Once a minute, under the lock.
state.Every(time.Minute, func(s *Status) { s.Now = time.Now().Format(time.Kitchen) })
```

## The rule

Do not do slow work while holding the lock. A network call inside
`state.Do` or inside a handler freezes every window until it returns,
because vero cannot encode the state meanwhile. Do the slow work in a
goroutine, then write the result back with `state.Do`.

Also do not keep the pointer that `state.Do` hands you. It is only valid
inside the function.

## Related

- [Add a request](add-a-request.md), for changing the state in answer to
  the window.
- [How vero works](how-vero-works.md), for why the state is one struct.

---

Next: [Add a request](add-a-request.md).
