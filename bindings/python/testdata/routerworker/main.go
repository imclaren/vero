// A worker that routes on the handler's name rather than on something inside
// the request, so the binding's call() has something to talk to.
package main

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

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

	// slow waits until its request is cancelled, and stopped counts the
	// slow requests that saw it.
	var stopped atomic.Int64
	vero.Handle(w, "slow", func(ctx context.Context, _ struct{}) (map[string]string, error) {
		select {
		case <-ctx.Done():
			stopped.Add(1)
			return nil, ctx.Err()
		case <-time.After(20 * time.Second):
			return map[string]string{"slow": "finished"}, nil
		}
	})
	vero.Handle(w, "stopped", func(context.Context, struct{}) (int64, error) { return stopped.Load(), nil })

	w.Serve()
}
