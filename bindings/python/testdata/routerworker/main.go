// A worker that routes on the handler's name rather than on something inside
// the request, so the binding's call() has something to talk to.
package main

import (
	"context"
	"fmt"

	"github.com/imclaren/vero"
)

type restart struct {
	ID int `json:"id"`
}

func main() {
	w := vero.NewWorker(vero.WorkerOptions{})

	vero.Handle(w, "status", func(context.Context, struct{}) (map[string]any, error) {
		return map[string]any{"jobs": []string{"Photos", "Documents"}, "working": true}, nil
	})
	vero.Handle(w, "restartJob", func(_ context.Context, req restart) (map[string]int, error) {
		if req.ID == 0 {
			return nil, fmt.Errorf("no such job")
		}
		return map[string]int{"restarted": req.ID}, nil
	})

	w.Serve()
}
