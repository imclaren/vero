// Command cshim builds vero as a C archive, so a native frontend can embed
// it and drive a worker without knowing anything about pipes, JSON framing or
// process supervision.
//
// Nothing here is specific to any application.  The worker's path arrives at
// runtime and every message is JSON, so this is built once and reused:
//
//	go build -buildmode=c-archive -o libvero.a ./cshim
//
// For a universal macOS build, build each architecture and join them:
//
//	export MACOSX_DEPLOYMENT_TARGET=11.0   # match the app, or the linker warns
//	CGO_ENABLED=1 GOARCH=arm64 go build -a -buildmode=c-archive \
//	    -o libvero-arm64.a ./cshim
//	CGO_ENABLED=1 GOARCH=amd64 CC="clang -arch x86_64 -mmacosx-version-min=11.0" \
//	    go build -a -buildmode=c-archive -o libvero-amd64.a ./cshim
//	lipo -create libvero-arm64.a libvero-amd64.a -output libvero.a
//
// Every function that returns a string returns an envelope, {"p":...} or
// {"e":"..."}, and the caller must hand it back to VeroFree.  Go allocated it
// with malloc and no garbage collector on either side will claim it.
package main

/*
#include <stdlib.h>
*/
import "C"
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"unsafe"

	"github.com/imclaren/vero"
)

var (
	mu      sync.Mutex
	sup     *vero.Supervisor
	latest  json.RawMessage
	seq     uint64 // bumped on every event
	seen    uint64 // how far VeroWaitForEvent has delivered
	changed = make(chan struct{})

	// startMu serialises VeroStart without holding mu while the supervisor
	// comes up.  The supervisor's first event can arrive before Supervise has
	// even returned, and that callback takes mu.
	startMu sync.Mutex

	// logLines carries the worker's log to whatever standard error the host
	// has, one line at a time.  It is buffered and lossy on purpose: a GUI
	// process on Windows has no standard error at all, and a host that reads
	// it slowly must not be able to stall the worker's log reader.
	logLines = make(chan string, 256)
	logOnce  sync.Once
)

func main() {}

// inProcess is the worker an application has compiled in, if it has.  An
// application registers one with vero.ServeInProcess before the frontend
// calls VeroStart, and VeroStart then runs it here rather than starting a
// program - which is the only arrangement iOS permits.
func inProcess() func(in io.Reader, out io.Writer) error { return vero.InProcessWorker() }

// VeroStart launches the worker and begins supervising it.
//
// argsJSON is a JSON array of arguments for the worker, or empty for none.
// It returns immediately: the worker comes up on its own, and until it does
// requests report that it is not running.
//
//export VeroStart
func VeroStart(workerPath *C.char, argsJSON *C.char) *C.char {
	var args []string
	if raw := C.GoString(argsJSON); raw != "" {
		if err := json.Unmarshal([]byte(raw), &args); err != nil {
			return cstring(errEnvelope(fmt.Errorf("cannot read the argument list: %w", err)))
		}
	}

	startMu.Lock()
	defer startMu.Unlock()

	mu.Lock()
	started := sup != nil
	mu.Unlock()
	if started {
		return cstring(errEnvelope(fmt.Errorf("already started")))
	}

	logOnce.Do(func() {
		go func() {
			for line := range logLines {
				fmt.Fprintln(os.Stderr, "worker:", line)
			}
		}()
	})

	s := vero.Supervise(vero.SupervisorOptions{
		Path: C.GoString(workerPath),
		Args: args,

		// Set when the application compiled its worker in rather than
		// shipping one beside the executable.  Supervise starts a program
		// when this is nil, and a goroutine when it is not.
		Serve: inProcess(),
		OnEvent: func(event json.RawMessage) {
			mu.Lock()
			latest = event
			seq++
			old := changed
			changed = make(chan struct{})
			mu.Unlock()
			close(old) // wake every waiter; none can miss it, seq only grows
		},
		// The worker logs to its standard error, so send it to ours. A GUI
		// application's stderr is the system log, which is where this belongs.
		// Dropped rather than blocked: losing a log line costs nothing, and
		// stalling the reader that produced it stops the worker.
		OnLog: func(line string) {
			select {
			case logLines <- line:
			default:
			}
		},
	})

	// Another process already has this worker.  Say so now rather than
	// leaving a frontend waiting for something that will never start.
	if err := s.Err(); err != nil {
		return cstring(errEnvelope(err))
	}

	mu.Lock()
	sup = s
	mu.Unlock()
	return cstring(okEnvelope(nil))
}

// VeroRequest sends a request and waits for its reply.
//
// There is no timeout: a worker may hold a request for as long as the work
// takes. Call it off the frontend's main thread.
//
//export VeroCall
func VeroCall(name *C.char, requestJSON *C.char) *C.char {
	return request(C.GoString(name), C.GoString(requestJSON))
}

//export VeroRequest
func VeroRequest(requestJSON *C.char) *C.char {
	return request("", C.GoString(requestJSON))
}

func request(name, requestJSON string) *C.char {
	mu.Lock()
	s := sup
	mu.Unlock()
	if s == nil {
		// Either VeroStart was never called or VeroStop has been. Both mean
		// the worker is not running, which a frontend should show as
		// waiting rather than as something having gone wrong.
		return cstring(errEnvelope(fmt.Errorf("%w: VeroStart has not been called, or VeroStop has", vero.ErrWorkerNotRunning)))
	}

	var payload json.RawMessage
	if err := json.Unmarshal([]byte(requestJSON), &payload); err != nil {
		return cstring(errEnvelope(err))
	}
	reply, err := s.Call(context.Background(), name, payload)
	if err != nil {
		return cstring(errEnvelope(err))
	}
	return cstring(okEnvelope(reply))
}

// VeroLatest returns the most recent event without waiting, so a window that
// has just opened can draw the current state rather than an empty one.
//
//export VeroLatest
func VeroLatest() *C.char {
	mu.Lock()
	defer mu.Unlock()
	return cstring(okEnvelope(latest))
}

// VeroWaitForEvent blocks until the worker's state changes, then returns it.
//
// This is what replaces polling: call it in a loop on a background thread and
// update the frontend each time it returns.  It expects a single caller.
//
//export VeroWaitForEvent
func VeroWaitForEvent() *C.char {
	for {
		mu.Lock()
		if seq > seen {
			seen = seq
			payload := latest
			mu.Unlock()
			return cstring(okEnvelope(payload))
		}
		wait := changed
		mu.Unlock()
		<-wait
	}
}

// VeroState reports "starting", "running", "restarting" or "stopped", so an
// frontend can say what is happening rather than showing stale numbers.
//
//export VeroState
func VeroState() *C.char {
	mu.Lock()
	s := sup
	mu.Unlock()
	if s == nil {
		return cstring(okEnvelope(json.RawMessage(`"stopped"`)))
	}
	payload, _ := json.Marshal(s.State().String())
	return cstring(okEnvelope(payload))
}

// VeroRestarts reports how many times the worker has been relaunched after
// dying.
//
// A frontend needs this to tell "it crashed once and recovered" from "it is
// crashing over and over", which call for different responses: the first is
// worth a line in a log, the second means the binary itself is the problem.
//
//export VeroRestarts
func VeroRestarts() *C.char {
	mu.Lock()
	s := sup
	mu.Unlock()
	if s == nil {
		return cstring(okEnvelope(json.RawMessage("0")))
	}
	payload, _ := json.Marshal(s.Restarts())
	return cstring(okEnvelope(payload))
}

// VeroStop shuts the worker down.  Not strictly needed - the worker's standard
// input closes when this process exits, and it goes with it - but it lets an
// application stop the work before its window has finished closing.
//
//export VeroStop
func VeroStop() {
	mu.Lock()
	s := sup
	sup = nil
	mu.Unlock()
	if s != nil {
		s.Stop()
	}
}

// VeroFree releases a string returned by any of the above.
//
//export VeroFree
func VeroFree(s *C.char) { C.free(unsafe.Pointer(s)) }

func cstring(s string) *C.char { return C.CString(s) }

func okEnvelope(payload json.RawMessage) string {
	if len(payload) == 0 {
		return `{}`
	}
	out, err := json.Marshal(map[string]json.RawMessage{"p": payload})
	if err != nil {
		return errEnvelope(err)
	}
	return string(out)
}

// errEnvelope carries a code beside the message so the other side can tell
// the two cases apart without matching on English: "the worker refused this"
// and "the worker is not there" call for different responses in a frontend.
//
// A refusal carries the worker's own words and nothing else. The Go side's
// wrapping is for Go's benefit, and would only be noise in a menu.
func errEnvelope(err error) string {
	e := map[string]string{"e": err.Error(), "code": "failed"}

	var remote *vero.RemoteError
	switch {
	case errors.As(err, &remote):
		e["e"], e["code"] = remote.Message, "refused"
	case errors.Is(err, vero.ErrAlreadyRunning):
		// Its own code: a frontend should offer to switch to the copy that
		// is running, not retry or report a fault.
		e["code"] = "already_running"
	case errors.Is(err, vero.ErrWorkerNotRunning):
		e["code"] = "not_running"
	}
	out, _ := json.Marshal(e)
	return string(out)
}
