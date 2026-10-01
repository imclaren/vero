package vero

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
)

// envHost asks a worker to supervise a copy of itself rather than serve.
//
// The supervisor is Go, and so is the worker, but a frontend written in
// anything else has to reach it somehow.  Loading it as a C library is one
// way - cshim - and it is the right one where the frontend links Go into its
// own process, as the Swift package does.  Where it cannot, this is the
// other: the same binary, run twice, with the supervision in the parent.
//
// It is what lets the Python and C# bindings work on every platform Go
// compiles for rather than the handful where -buildmode=c-shared does.
const envHost = "VERO_HOST"

// kindState is a host telling its frontend that the worker's lifecycle moved.
// Events and replies keep the names they have coming out of a worker, so a
// frontend reading a host reads the same three kinds either way.
const kindState = "state"

// kindControl marks a request the host answers itself - the lifecycle
// questions a frontend would otherwise call a C function for.  Requests
// without it are forwarded to the worker untouched, so an application's own
// handler names can be anything at all without colliding with these.
const kindControl = "ctl"

// Hosting reports whether this process was started to supervise a worker
// rather than be one.  NewWorker calls it, so an application only needs this
// if it builds its worker without NewWorker.
func Hosting() bool { return os.Getenv(envHost) != "" }

// host supervises a copy of this executable and speaks the envelope protocol
// on standard input and output.  It does not return: the process exists to
// supervise, and when its frontend closes the pipe there is nothing else for
// it to do.
//
// The frontend writes requests, exactly as it would to a worker, and reads
// events and replies back.  The difference a host makes is what happens when
// the worker dies: it is restarted, the requests waiting on it are told, and
// the frontend is sent a state line rather than an end of file.
func host() {
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "vero: cannot find this executable:", err)
		os.Exit(1)
	}

	out := json.NewEncoder(os.Stdout)
	var mu sync.Mutex // one writer at a time: events arrive on their own goroutine
	write := func(e Envelope) {
		mu.Lock()
		defer mu.Unlock()
		_ = out.Encode(e)
	}

	// Declared before Supervise so OnStateChange can report the restart
	// count, which lives on the supervisor it is being handed to.
	var sup *Supervisor
	sup = Supervise(SupervisorOptions{
		Path: self,
		Args: os.Args[1:],

		// Without this the child inherits VERO_HOST, decides it is a host
		// too, and supervises a copy of itself - which does the same.
		Env:     without(os.Environ(), envHost),
		OnEvent: func(event json.RawMessage) { write(Envelope{Kind: kindEvent, Payload: event}) },
		OnLog:   func(line string) { fmt.Fprintln(os.Stderr, "worker:", line) },
		OnStateChange: func(st RunState) {
			restarts := 0
			if sup != nil {
				restarts = sup.Restarts()
			}
			payload, _ := json.Marshal(map[string]any{"state": st.String(), "restarts": restarts})
			write(Envelope{Kind: kindState, Payload: payload})
		},
	})
	defer sup.Stop()

	// Another process already holds this worker.  Say so now, rather than
	// leaving a frontend waiting for something that will never start.
	if err := sup.Err(); err != nil {
		write(failed(0, err))
	}

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64<<10), MaxLineSize)

	var wg sync.WaitGroup
	for sc.Scan() {
		line := make([]byte, len(sc.Bytes()))
		copy(line, sc.Bytes())

		var req Envelope
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			write(answer(sup, req))
		}()
	}

	// The frontend has gone.  Stop the worker before waiting on the requests
	// in flight: they are answers nobody will read.
	sup.Stop()
	wg.Wait()
	os.Exit(0)
}

// answer handles one request, either here or on the other side of the worker.
func answer(sup *Supervisor, req Envelope) Envelope {
	if req.Kind == kindControl {
		return control(sup, req)
	}

	var reply json.RawMessage
	var err error
	if req.Name != "" {
		reply, err = sup.Call(context.Background(), req.Name, req.Payload)
	} else {
		reply, err = sup.Request(context.Background(), req.Payload)
	}
	if err != nil {
		return failed(req.ID, err)
	}
	return Envelope{Kind: kindReply, ID: req.ID, Payload: reply}
}

// control answers the questions about the worker rather than to it.
func control(sup *Supervisor, req Envelope) Envelope {
	switch req.Name {
	case "latest":
		return Envelope{Kind: kindReply, ID: req.ID, Payload: sup.Latest()}

	case "state":
		// Built here rather than handed to replyEnvelope, which would encode
		// the bytes a second time and hand the frontend a base64 string.
		payload, err := json.Marshal(map[string]any{
			"state": sup.State().String(), "restarts": sup.Restarts(),
		})
		if err != nil {
			return failed(req.ID, err)
		}
		return Envelope{Kind: kindReply, ID: req.ID, Payload: payload}

	case "stop":
		if err := sup.Stop(); err != nil {
			return failed(req.ID, err)
		}
		return Envelope{Kind: kindReply, ID: req.ID}

	default:
		return failed(req.ID, fmt.Errorf("vero: no control request named %q", req.Name))
	}
}

// failed answers with an error and the code that tells a frontend what to do
// about it: show a refusal, wait out a restart, or offer to switch to the
// copy of the application that is already running.
//
// A refusal carries the worker's own words.  The wrapping Go adds on the way
// here is for Go's benefit and would only be noise in a menu.
func failed(id uint64, err error) Envelope {
	e := Envelope{Kind: kindReply, ID: id, Error: err.Error(), Code: "failed"}

	var remote *RemoteError
	switch {
	case errors.As(err, &remote):
		e.Error, e.Code = remote.Message, "refused"
	case errors.Is(err, ErrAlreadyRunning):
		e.Code = "already_running"
	case errors.Is(err, ErrWorkerNotRunning):
		e.Code = "not_running"
	}
	return e
}

// without returns env with every setting of name removed.
func without(env []string, name string) []string {
	prefix := name + "="
	out := make([]string, 0, len(env))
	for _, entry := range env {
		if !strings.HasPrefix(entry, prefix) {
			out = append(out, entry)
		}
	}
	return out
}
