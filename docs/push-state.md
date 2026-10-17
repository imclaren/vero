# Push state

The state is one struct, held by `vero.NewState`. vero sends it to every
window whenever its JSON changes, so a front end never polls.

```go
state := vero.NewState(w, Status{})

go func() {                           // work of the worker's own
	for item := range results {
		state.Do(func(s *Status) { s.Items = append(s.Items, item) })
	}
}()

state.Every(time.Minute, func(s *Status) { s.Now = time.Now() })
```

- Change the state only inside a handler, `state.Do` or `state.Every`,
  which hold its lock. Slow work runs outside them and writes back with
  `state.Do`.
- Every field needs a `json` tag, and the front ends see only what is
  exported.
- After changing the struct, run `vero gen`.
