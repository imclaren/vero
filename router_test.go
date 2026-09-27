package vero_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/imclaren/vero"
)

const envRouterWorker = "VERO_ROUTER_WORKER"

type Group struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Folder struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

type AddGroup struct {
	Name string `json:"name"`
}

func routerWorker() {
	w := vero.NewWorker(vero.WorkerOptions{})

	// Both return empty lists. Decoded by guessing, an empty JSON array
	// satisfies either type, so whichever is tried first wins - which is the
	// bug this exists to remove.
	vero.Handle(w, "getGroups", func(ctx context.Context, _ struct{}) ([]Group, error) {
		return []Group{}, nil
	})
	vero.Handle(w, "getFolders", func(ctx context.Context, _ struct{}) ([]Folder, error) {
		return []Folder{}, nil
	})
	vero.Handle(w, "addGroup", func(ctx context.Context, req AddGroup) (Group, error) {
		return Group{ID: len(req.Name), Name: req.Name}, nil
	})
	vero.Handle(w, "slow", func(ctx context.Context, _ struct{}) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Minute):
			return "never", nil
		}
	})
	w.Serve()
	os.Exit(0)
}

func newRouterSupervisor(t *testing.T) *vero.Supervisor {
	t.Helper()
	s := vero.Supervise(vero.SupervisorOptions{
		Path: os.Args[0],
		Env:  append(os.Environ(), envRouterWorker+"=1"),
	})
	t.Cleanup(func() { s.Stop() })
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s.State() == vero.Running {
			return s
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the worker never came up")
	return nil
}

func TestTheReplyTypeIsNotGuessed(t *testing.T) {
	s := newRouterSupervisor(t)

	// Two different requests, both answering with an empty list. The caller
	// knows which is which because it asked by name, not because it managed
	// to decode one.
	groups, err := s.Call(context.Background(), "getGroups", struct{}{})
	if err != nil {
		t.Fatalf("getGroups: %v", err)
	}
	folders, err := s.Call(context.Background(), "getFolders", struct{}{})
	if err != nil {
		t.Fatalf("getFolders: %v", err)
	}
	if string(groups) != "[]" || string(folders) != "[]" {
		t.Fatalf("got %s and %s", groups, folders)
	}
	// Both are "[]" on the wire and would decode as either type. Routing by
	// name is what tells them apart.
}

func TestATypedRequestReachesItsHandler(t *testing.T) {
	s := newRouterSupervisor(t)
	raw, err := s.Call(context.Background(), "addGroup", AddGroup{Name: "photos"})
	if err != nil {
		t.Fatalf("addGroup: %v", err)
	}
	var g Group
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	if g.Name != "photos" || g.ID != len("photos") {
		t.Fatalf("got %+v", g)
	}
}

func TestAnUnknownNameIsRefused(t *testing.T) {
	s := newRouterSupervisor(t)
	_, err := s.Call(context.Background(), "nonsense", struct{}{})
	if err == nil || !strings.Contains(err.Error(), `unsupported request type: "nonsense"`) {
		t.Fatalf("got %v", err)
	}
}

func TestARouterWithoutAFallbackRefusesUnnamedRequests(t *testing.T) {
	// The default. It is what makes typed routing the only way in: a request
	// with no name has nowhere to go.
	s := newRouterSupervisor(t)
	_, err := s.Request(context.Background(), map[string]string{"anything": "at all"})
	if err == nil || !strings.Contains(err.Error(), "unsupported request type") {
		t.Fatalf("got %v, want a refusal", err)
	}
}

func TestARoutedHandlerStillGetsTheCancellation(t *testing.T) {
	s := newRouterSupervisor(t)
	go s.Call(context.Background(), "slow", struct{}{})
	time.Sleep(300 * time.Millisecond)

	start := time.Now()
	s.Stop()
	if took := time.Since(start); took > 4*time.Second {
		t.Fatalf("took %v to stop with a routed handler still running", took)
	}
}
