// The worker, compiled into the application.
//
// On every other platform this would be a program of its own, started by the
// frontend - ../../worker is exactly that.  iOS forbids an application
// starting a program, so instead it is registered here, and vero runs it on a
// goroutine when the frontend asks for a worker.
//
// This directory is the shim and the worker together, because a C archive is
// built from one package: shim.go is a symbolic link to ../../../cshim/main.go,
// which is the same file every other frontend links, unchanged.
package main

import (
	"fmt"
	"io"
	"math/rand"
	"slices"
	"time"

	"github.com/imclaren/vero"
)

func init() { vero.ServeInProcess(serve) }

type Job struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Phase    string `json:"phase"`
	Progress int    `json:"progress"`
}

func (j Job) Key() int { return j.ID }

func (j *Job) Restart() { j.Phase, j.Progress = "waiting", 0 }

type Status struct {
	Jobs    []Job  `json:"jobs"`
	Working bool   `json:"working"`
	Since   string `json:"since"`
}

func working(s *Status) {
	s.Working = slices.ContainsFunc(s.Jobs, func(j Job) bool {
		return j.Phase != "waiting" && j.Phase != "done"
	})
}

// serve is ../../worker/main.go, reading and writing the streams it is given
// instead of standard input and output.
func serve(in io.Reader, out io.Writer) error {
	w := vero.NewWorker(vero.WorkerOptions{In: in, Out: out, Version: "0.12.0"})

	state := vero.NewState(w, Status{
		Jobs: []Job{
			{ID: 1, Name: "Photos", Phase: "waiting"},
			{ID: 2, Name: "Documents", Phase: "waiting"},
			{ID: 3, Name: "Team share", Phase: "waiting"},
		},
		Since: time.Now().Format("15:04:05"),
	})

	go work(w, state)

	vero.Update(state, "status", func(*Status) error { return nil })
	vero.UpdateWith(state, "restartJob", func(s *Status, req vero.ID[int]) error {
		if err := vero.Edit(s.Jobs, req.ID, (*Job).Restart); err != nil {
			return fmt.Errorf("no job with id %d", req.ID)
		}
		working(s)
		return nil
	})
	return w.Serve()
}

// work is the business logic, with the random state changes the other
// examples use to show events arriving.
func work(w *vero.Worker, state *vero.State[Status]) {
	phases := []string{"looking for changes", "scanning", "uploading", "done"}
	for {
		time.Sleep(time.Duration(200+rand.Intn(400)) * time.Millisecond)

		var name, phase, before string
		state.Do(func(s *Status) {
			j := &s.Jobs[rand.Intn(len(s.Jobs))]
			before = j.Phase
			switch i := slices.Index(phases, j.Phase); {
			case j.Phase == "waiting" || j.Phase == "done":
				j.Phase, j.Progress = phases[0], 0
			case j.Progress < 100:
				j.Progress = min(j.Progress+20+rand.Intn(30), 100)
			case i+1 < len(phases):
				j.Phase, j.Progress = phases[i+1], 0
			}
			name, phase = j.Name, j.Phase
			working(s)
		})

		if phase != before {
			w.Log("%s: %s", name, phase)
		}
	}
}
