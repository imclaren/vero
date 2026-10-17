# Add a request

A request is a named handler on the worker, which a front end calls.

```go
type Rename struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

vero.UpdateWith(state, "rename", func(s *Status, req Rename) error {
	return vero.Edit(s.Jobs, req.ID, func(j *Job) { j.Name = req.Name })
})
```

- `vero.Update` takes no data, `vero.UpdateWith` takes a struct, and
  `vero.UpdateItem` takes a `vero.ID` and finds the item by its `Key()`.
  Each replies with the new state, and an error that it returns reaches the
  front end and changes nothing.
- `vero.Handle(w, name, func(ctx, req) (Reply, error))` replies with a value
  of its own, such as a page of search results. It stops when `ctx.Done()`
  closes, which happens when the front end no longer wants the reply.

Then run `vero gen`, which writes the typed call into `WorkerAPI.swift`,
`WorkerApi.cs` and `worker_api.py`, and call it from each front end.
