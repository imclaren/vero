// Command worker is the WASI example's worker: the half that would be a
// program anywhere else, and is a single worker.wasm here.
//
// It is deliberately not ../../worker.  That one does its work in a goroutine
// of its own, moving jobs along on a timer and letting vero push the new
// state out as it changes.  None of that happens under WASI: the instance has
// one thread, and while the worker waits for its next request on standard
// input nothing else inside it runs - no timers, no goroutines, nothing.  The
// jobs would sit at "waiting" for ever.
//
// So this worker does its work where WASI lets it: in the handler, while it
// is answering.  Ask it to advance a job and it advances, and replies with
// the new state.  Ask it nothing and it does nothing, which is the honest
// shape of a worker in a sandbox with no threads.
package main

import (
	"flag"
	"fmt"
	"slices"

	"github.com/imclaren/vero"
)

var version = "0.13.0"

type Job struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Phase    string `json:"phase"`
	Progress int    `json:"progress"`
}

func (j Job) Key() int { return j.ID }

type Status struct {
	Jobs    []Job  `json:"jobs"`
	Working bool   `json:"working"`
	Since   string `json:"since"`
}

var phases = []string{"waiting", "looking for changes", "scanning", "uploading", "done"}

// step is the work: one call moves a job on a little, which is what the
// frontend asks for.
func (j *Job) step() {
	switch i := slices.Index(phases, j.Phase); {
	case j.Phase == "done":
		j.Phase, j.Progress = phases[0], 0
	case j.Progress < 100:
		j.Progress = min(j.Progress+25, 100)
		if j.Phase == "waiting" {
			j.Phase = phases[1]
		}
	case i+1 < len(phases):
		j.Phase, j.Progress = phases[i+1], 0
	}
}

func (j *Job) restart() { j.Phase, j.Progress = "waiting", 0 }

func working(s *Status) {
	s.Working = slices.ContainsFunc(s.Jobs, func(j Job) bool {
		return j.Phase != "waiting" && j.Phase != "done"
	})
}

func main() {
	var opts vero.WorkerOptions
	opts.Version = version
	opts.RegisterFlags(flag.CommandLine)
	flag.Parse()
	opts.PrintVersionAndExit()

	w := vero.NewWorker(opts)

	state := vero.NewState(w, Status{
		Jobs: []Job{
			{ID: 1, Name: "Photos", Phase: "waiting"},
			{ID: 2, Name: "Documents", Phase: "waiting"},
			{ID: 3, Name: "Team share", Phase: "waiting"},
		},
	})

	vero.Update(state, "status", func(*Status) error { return nil })

	vero.UpdateWith(state, "advanceJob", func(s *Status, req vero.ID[int]) error {
		if err := vero.Edit(s.Jobs, req.ID, (*Job).step); err != nil {
			return fmt.Errorf("no job with id %d", req.ID)
		}
		working(s)
		return nil
	})

	vero.UpdateWith(state, "restartJob", func(s *Status, req vero.ID[int]) error {
		if err := vero.Edit(s.Jobs, req.ID, (*Job).restart); err != nil {
			return fmt.Errorf("no job with id %d", req.ID)
		}
		working(s)
		return nil
	})

	if err := w.Serve(); err != nil {
		w.Log("stopped: %v", err)
	}
}
