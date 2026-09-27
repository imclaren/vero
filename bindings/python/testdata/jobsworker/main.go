// A worker that answers unnamed requests - a "type" inside the message rather
// than a handler name - so the binding's send() has something stateful to talk
// to.  The example worker routes on handler names instead.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/imclaren/vero"
)

type Job struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Phase    string `json:"phase"`
	Progress int    `json:"progress"`
}

// Key is how an id in a request finds one job.
func (j Job) Key() int { return j.ID }

func (j *Job) Restart() { j.Phase, j.Progress = "waiting", 0 }

type Status struct {
	Jobs    []Job  `json:"jobs"`
	Working bool   `json:"working"`
	Since   string `json:"since"`
}

type Request struct {
	Type string `json:"type"`
	ID   int    `json:"id"`
}

func main() {
	w := vero.NewWorker(vero.WorkerOptions{})

	state := vero.NewState(w, Status{
		Jobs: []Job{
			{ID: 1, Name: "Photos", Phase: "waiting"},
			{ID: 2, Name: "Documents", Phase: "waiting"},
			{ID: 3, Name: "Team share", Phase: "waiting"},
		},
		Since: time.Now().Format("15:04:05"),
	})

	// Moves a job along, so events arrive unasked.
	go func() {
		for {
			time.Sleep(100 * time.Millisecond)
			state.Do(func(s *Status) {
				j := &s.Jobs[0]
				j.Phase, j.Progress = "uploading", (j.Progress+10)%100
				s.Working = true
			})
		}
	}()

	w.Fallback(func(_ context.Context, request json.RawMessage) (any, error) {
		var r Request
		if err := json.Unmarshal(request, &r); err != nil {
			return nil, err
		}
		switch r.Type {
		case "status":
			return state.JSON()

		case "restart":
			var err error
			state.Do(func(s *Status) {
				if err = vero.Edit(s.Jobs, r.ID, (*Job).Restart); err != nil {
					err = fmt.Errorf("no job with id %d", r.ID)
				}
			})
			if err != nil {
				return nil, err
			}
			return state.JSON()

		default:
			return nil, fmt.Errorf("unknown request type: %q", r.Type)
		}
	})

	w.Serve()
}
