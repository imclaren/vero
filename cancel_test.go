package vero_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/imclaren/vero"
)

// waitStopped asks the worker until n "stoppable" handlers have seen their
// context end, which they do only when their request is cancelled.
func waitStopped(t *testing.T, ask func() string, n string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if ask() == n {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("the handler did not stop: %s stopped, want %s", ask(), n)
}

// TestACancelledCallStopsItsHandler: a caller whose context ends gets
// ErrCancelled at once, and the worker's handler sees its own context end,
// rather than doing work whose reply nobody will read.
func TestACancelledCallStopsItsHandler(t *testing.T) {
	s := newSupervisor(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)
	start := time.Now()
	_, err := s.Request(ctx, request{Type: "stoppable"})
	if !errors.Is(err, vero.ErrCancelled) || !errors.Is(err, context.Canceled) {
		t.Fatalf("the cancelled call: %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("the cancelled call took %s", time.Since(start))
	}
	ask := func() string {
		raw, err := s.Request(context.Background(), request{Type: "stopped"})
		if err != nil {
			return err.Error()
		}
		var r reply
		json.Unmarshal(raw, &r)
		return r.Message
	}
	waitStopped(t, ask, "1")

	// The worker still answers, and other requests were not touched.
	if raw, err := s.Request(context.Background(), request{Type: "echo", N: 7}); err != nil || string(raw) != `{"message":"echo 7"}` {
		t.Errorf("after the cancel: %s %v", raw, err)
	}
}

// TestAHostPassesOnACancel: a frontend that speaks to a host, as Python and
// C# do, cancels with {"t":"cancel","id":N}. Its reply comes back at once,
// coded "cancelled", and the worker's handler stops. A cancel for a request
// that has finished, or that never was, does nothing.
func TestAHostPassesOnACancel(t *testing.T) {
	h := startHost(t)
	h.waitRunning()

	done := h.send("", "", request{Type: "echo", N: 1})
	if _, errText := h.reply(done); errText != "" {
		t.Fatal(errText)
	}
	if err := h.in.Encode(map[string]any{"t": "cancel", "id": done}); err != nil {
		t.Fatal(err)
	}
	if err := h.in.Encode(map[string]any{"t": "cancel", "id": 999999}); err != nil {
		t.Fatal(err)
	}

	slow := h.send("", "", request{Type: "stoppable"})
	time.Sleep(100 * time.Millisecond)
	start := time.Now()
	if err := h.in.Encode(map[string]any{"t": "cancel", "id": slow}); err != nil {
		t.Fatal(err)
	}
	if _, errText := h.reply(slow); errText == "" {
		t.Error("the cancelled request had no error")
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("the cancelled request took %s", time.Since(start))
	}
	ask := func() string {
		raw, errText := h.reply(h.send("", "", request{Type: "stopped"}))
		if errText != "" {
			return errText
		}
		var r reply
		json.Unmarshal(raw, &r)
		return r.Message
	}
	waitStopped(t, ask, "1")
}
