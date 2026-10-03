//go:build js && wasm

// Command web-app is the browser example: the same jobs as ../worker, the
// same rows as the macOS, WPF and GTK frontends, in a page.
//
// A browser may not start a program, so there is no worker beside this one to
// spawn.  There does not need to be: SupervisorOptions.Serve runs the worker
// in this process, on a goroutine, joined to the supervisor by a pipe in
// memory.  Above that pipe nothing changes - the same requests, the same
// replies, the same state arriving as it changes - so the half of this file
// below main is the worker you would otherwise ship as a binary, and the half
// above it is a frontend like any other.
//
//	./build.sh && go run serve.go

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"slices"
	"syscall/js"
	"time"

	"github.com/imclaren/vero"
)

var document = js.Global().Get("document")

func main() {
	ui := newUI()

	sup := vero.Supervise(vero.SupervisorOptions{
		Serve:   serve,
		OnEvent: ui.apply,
		OnLog:   func(line string) { js.Global().Get("console").Call("log", "worker: "+line) },
	})
	defer sup.Stop()
	ui.sup = sup

	// The state a page that has just loaded draws, before any event.
	if status := sup.Latest(); status != nil {
		ui.apply(status)
	}

	<-make(chan struct{}) // main returning would end the program
}

// --- the frontend ---------------------------------------------------------

type row struct{ name, phase, fill js.Value }

type ui struct {
	jobs *js.Value
	rows map[int]row
	sup  *vero.Supervisor
}

func newUI() *ui {
	jobs := document.Call("getElementById", "jobs")
	return &ui{jobs: &jobs, rows: map[int]row{}}
}

// apply draws a status, whether it arrived as a reply or as an event.
func (u *ui) apply(status json.RawMessage) {
	var s Status
	if err := json.Unmarshal(status, &s); err != nil {
		return
	}
	for _, job := range s.Jobs {
		r, ok := u.rows[job.ID]
		if !ok {
			r = u.addRow(job)
			u.rows[job.ID] = r
		}
		r.name.Set("textContent", job.Name)
		r.phase.Set("textContent", job.Phase)
		r.fill.Get("style").Set("width", fmt.Sprintf("%d%%", job.Progress))
	}
}

func (u *ui) addRow(job Job) row {
	el := func(tag, class string) js.Value {
		e := document.Call("createElement", tag)
		e.Set("className", class)
		return e
	}

	div := el("div", "row")
	button := el("button", "")
	button.Set("textContent", "Restart")
	// A click handler has to return before the browser does anything else,
	// so the request goes on a goroutine of its own.  The reply redraws the
	// row without waiting for the next event.
	button.Call("addEventListener", "click", js.FuncOf(func(js.Value, []js.Value) any {
		go func() {
			status, err := u.sup.Call(context.Background(), "restartJob", vero.ID[int]{ID: job.ID})
			if err == nil {
				u.apply(status)
			}
		}()
		return nil
	}))

	name, phase := el("span", "name"), el("span", "phase")
	bar, fill := el("div", "bar"), el("div", "fill")
	bar.Call("appendChild", fill)
	for _, child := range []js.Value{button, name, phase, bar} {
		div.Call("appendChild", child)
	}
	u.jobs.Call("appendChild", div)
	return row{name: name, phase: phase, fill: fill}
}

// --- the worker -----------------------------------------------------------

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

// serve is ../worker/main.go, reading and writing the pipes it is given
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

// work is the business logic, with random state changes to show the events
// arriving: the same as ../worker/main.go.
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
