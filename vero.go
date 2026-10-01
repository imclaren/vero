// Package vero is a Go backend with native macOS, Windows and Linux
// frontends, all built on macOS.  See the README for how to build one.
package vero

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Envelope is one line of the protocol.  Everything is optional except what
// the message kind requires, so the same struct decodes in both directions.
type Envelope struct {
	// Kind is "event" or "reply" on the way out of a worker, and empty on the
	// way in, where every line is a request.
	Kind string `json:"t,omitempty"`

	// ID matches a reply to the request that caused it.  Events have none.
	ID uint64 `json:"id,omitempty"`

	// Name routes a request to a handler registered with Handle or Update.
	// Empty when
	// the worker has a single handler that dispatches for itself.
	//
	// It lives here rather than inside the payload so that vero can route
	// without knowing anything about the message, and so an application is
	// not obliged to carry a type field in every request it defines.
	Name string `json:"n,omitempty"`

	// Payload is your own message, carried through untouched.
	Payload json.RawMessage `json:"p,omitempty"`

	// Error is set instead of Payload when a handler returned an error.
	Error string `json:"e,omitempty"`

	// Code classifies an error for a frontend that cannot match on English:
	// "refused", "not_running", "already_running" or "failed".  A host sets
	// it; a worker does not, because the supervisor on the other side of it
	// has the Go errors themselves.
	Code string `json:"c,omitempty"`
}

const (
	kindEvent = "event"
	kindReply = "reply"
)

// Handler answers one request.
//
// ctx is cancelled when the frontend goes away, so a handler that waits -
// a long poll, most obviously - must select on ctx.Done() as well. Without
// that the worker cannot exit when its parent does: it has stopped reading
// requests, but it is still waiting for the answer to one nobody will read.  Returning an error sends the message to the
// caller rather than closing anything: the frontend can then tell "the worker
// refused this" from "the worker is not there", which want opposite responses.
//
// Returning a nil reply and a nil error answers nothing at all, which suits a
// request that only causes an action.
type Handler func(ctx context.Context, request json.RawMessage) (reply any, err error)

// ErrWorkerNotRunning is returned by Request when the worker has died and has
// not been restarted yet.  It is worth distinguishing in a frontend: it
// means wait, not that anything you did was wrong.
var ErrWorkerNotRunning = errors.New("vero: the worker is not running")

// RemoteError is what a Handler's error becomes on the other side of the pipe.
type RemoteError struct{ Message string }

// Error is the worker's own message, not vero's.
func (e *RemoteError) Error() string { return "vero: worker refused the request: " + e.Message }

// DefaultStateInterval is how often WorkerOptions.State is sampled for a
// change when StateInterval is not set.  Fast enough that a progress bar moves
// smoothly, slow enough that nothing is sent while nothing is happening.
const DefaultStateInterval = 100 * time.Millisecond

// MaxLineSize caps one protocol line, so a corrupt or hostile stream cannot
// ask for an unbounded allocation.  Payloads larger than this belong in a file
// with the path sent through the pipe.
const MaxLineSize = 16 << 20 // 16 MiB

func replyEnvelope(id uint64, reply any, err error) Envelope {
	if err != nil {
		return Envelope{Kind: kindReply, ID: id, Error: err.Error()}
	}
	if reply == nil {
		return Envelope{Kind: kindReply, ID: id}
	}
	payload, marshalErr := json.Marshal(reply)
	if marshalErr != nil {
		return Envelope{Kind: kindReply, ID: id, Error: fmt.Sprintf("cannot encode reply: %v", marshalErr)}
	}
	return Envelope{Kind: kindReply, ID: id, Payload: payload}
}
