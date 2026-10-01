package vero_test

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/imclaren/vero"
)

// A worker running inside the application rather than beside it, which is the
// only shape iOS allows: it forbids starting a program, and permits compiling
// Go in.  Everything above the pipe is unchanged, so these are the same
// requests and the same replies as the process tests make.

type inProcessJob struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Runs int    `json:"runs"`
}

func (j inProcessJob) Key() int { return j.ID }

type inProcessStatus struct {
	Jobs []inProcessJob `json:"jobs"`
}

// serveInProcess is what an application would hand to Supervise: its own
// handlers, reading and writing the pipes it is given.
func serveInProcess(in io.Reader, out io.Writer) error {
	w := vero.NewWorker(vero.WorkerOptions{In: in, Out: out})
	state := vero.NewState(w, inProcessStatus{Jobs: []inProcessJob{{ID: 1, Name: "Photos"}}})

	vero.Update(state, "status", func(*inProcessStatus) error { return nil })
	vero.UpdateItem(state, "runJob", func(j *inProcessJob) error {
		j.Runs++
		return nil
	})
	return w.Serve()
}

func startInProcess(t *testing.T, onEvent func(json.RawMessage)) *vero.Supervisor {
	t.Helper()
	sup := vero.Supervise(vero.SupervisorOptions{
		Serve:   serveInProcess,
		OnEvent: onEvent,
	})
	t.Cleanup(func() { sup.Stop() })

	if err := sup.Err(); err != nil {
		t.Fatalf("starting in process: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for sup.State() != vero.Running {
		if time.Now().After(deadline) {
			t.Fatalf("the worker never ran: state is %s", sup.State())
		}
		time.Sleep(5 * time.Millisecond)
	}
	return sup
}

func TestAWorkerInThisProcessAnswers(t *testing.T) {
	sup := startInProcess(t, nil)

	reply, err := sup.Call(context.Background(), "status", struct{}{})
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	var status inProcessStatus
	if err := json.Unmarshal(reply, &status); err != nil {
		t.Fatalf("decoding the reply: %v", err)
	}
	if len(status.Jobs) != 1 || status.Jobs[0].Name != "Photos" {
		t.Fatalf("got %+v, want one job called Photos", status.Jobs)
	}
}

func TestAWorkerInThisProcessChangesItsState(t *testing.T) {
	sup := startInProcess(t, nil)

	reply, err := sup.Call(context.Background(), "runJob", vero.ID[int]{ID: 1})
	if err != nil {
		t.Fatalf("runJob: %v", err)
	}

	var status inProcessStatus
	if err := json.Unmarshal(reply, &status); err != nil {
		t.Fatalf("decoding the reply: %v", err)
	}
	if status.Jobs[0].Runs != 1 {
		t.Fatalf("runs is %d, want 1", status.Jobs[0].Runs)
	}
}

func TestAWorkerInThisProcessEmits(t *testing.T) {
	events := make(chan json.RawMessage, 8)
	sup := startInProcess(t, func(e json.RawMessage) {
		select {
		case events <- e:
		default:
		}
	})

	// The state is pushed when it changes, so cause a change.
	if _, err := sup.Call(context.Background(), "runJob", vero.ID[int]{ID: 1}); err != nil {
		t.Fatalf("runJob: %v", err)
	}

	select {
	case e := <-events:
		var status inProcessStatus
		if err := json.Unmarshal(e, &status); err != nil {
			t.Fatalf("decoding the event: %v", err)
		}
		if len(status.Jobs) == 0 {
			t.Fatal("an event arrived with no jobs in it")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no event arrived")
	}
}

func TestAWorkerInThisProcessRefusesAnUnknownRequest(t *testing.T) {
	sup := startInProcess(t, nil)

	if _, err := sup.Call(context.Background(), "nosuch", struct{}{}); err == nil {
		t.Fatal("an unknown request should be refused, not answered")
	}
}

func TestStoppingAWorkerInThisProcess(t *testing.T) {
	sup := startInProcess(t, nil)

	if err := sup.Stop(); err != nil {
		t.Fatalf("stopping: %v", err)
	}
	if state := sup.State(); state != vero.Stopped {
		t.Fatalf("state is %s after Stop, want stopped", state)
	}
	if _, err := sup.Call(context.Background(), "status", struct{}{}); err == nil {
		t.Fatal("a request after Stop should fail")
	}
}

// Two of them at once: in process there is no lock to take, because the
// worker's life is the application's and one goroutine is one worker.
func TestTwoWorkersInThisProcessDoNotCollide(t *testing.T) {
	first := startInProcess(t, nil)
	second := startInProcess(t, nil)

	for i, sup := range []*vero.Supervisor{first, second} {
		if _, err := sup.Call(context.Background(), "status", struct{}{}); err != nil {
			t.Fatalf("supervisor %d: %v", i, err)
		}
	}
}
