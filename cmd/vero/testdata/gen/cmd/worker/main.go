// The worker vero gen's test reads: one of each kind of request and type.
package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/imclaren/vero"
)

// Speed is how fast to go.
type Speed string

// Job is one row.
type Job struct {
	ID     int    `json:"id"`
	Title  string `json:"title"`
	Paused bool   `json:"paused,omitempty"`
}

func (j Job) Key() int { return j.ID }

// Common is embedded, and its fields are brought up.
type Common struct {
	// When it changed.
	Changed time.Time `json:"changed"`
}

// Status is what the app draws.
type Status struct {
	Common
	Jobs   []Job           `json:"jobs"`
	Speed  Speed           `json:"speed"`
	Labels map[string]int  `json:"labels"`
	Extra  json.RawMessage `json:"extra"`
	Owner  *Owner          `json:"owner,omitempty"`
	Secret string          `json:"-"`
	hidden int
	Score  float64    `json:"score,string"`
	Tags   [][]string `json:"tags"`
	Data   []byte     `json:"data"`
	Any    any        `json:"any"`
	Task   Task       `json:"task"`
}

// Owner is who owns it.
type Owner struct {
	Name string `json:"name"`
}

// Task clashes with Swift's Task and C#'s.
type Task struct {
	Due string `json:"due"`
}

func main() {
	w := vero.NewWorker(vero.WorkerOptions{})
	state := vero.NewState(w, Status{})

	// rename renames a job.
	vero.Handle(w, "rename", func(_ context.Context, req struct {
		ID    int    `json:"id"`
		Title string `json:"title"`
		// Note is anonymous and nested.
		Note struct {
			Text string `json:"text"`
		} `json:"note"`
	}) (Job, error) {
		return Job{}, nil
	})
	vero.Handle(w, "ping", func(context.Context, struct{}) (struct{}, error) { return struct{}{}, nil })
	vero.Handle(w, "raw", func(context.Context, []string) (json.RawMessage, error) { return nil, nil })
	vero.Handle(w, "owner", func(context.Context, struct{}) (*Owner, error) { return nil, nil })
	vero.Handle(w, "count", func(context.Context, struct{}) (int, error) { return 0, nil })
	vero.Update(state, "add", func(s *Status) error { return nil })
	vero.UpdateWith(state, "setSpeed", func(s *Status, req struct {
		Speed Speed `json:"speed"`
	}) error {
		return nil
	})
	vero.UpdateItem(state, "pause", func(j *Job) error { return nil })
	w.Serve()
}
