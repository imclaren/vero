package vero

import "testing"

// TestSnapshot checks that a snapshot is the state as it was, and that
// changing the state afterwards leaves it alone.
func TestSnapshot(t *testing.T) {
	type st struct {
		Names []string `json:"names"`
	}
	w := NewWorker(WorkerOptions{})
	s := NewState(w, st{Names: []string{"a"}})
	snap, err := s.Snapshot()
	if err != nil || len(snap.Names) != 1 || snap.Names[0] != "a" {
		t.Fatalf("snapshot %+v, %v", snap, err)
	}
	s.Do(func(v *st) { v.Names[0] = "changed" })
	if snap.Names[0] != "a" {
		t.Error("the snapshot shares the state's slice")
	}
}
