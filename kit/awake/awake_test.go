package awake

import (
	"os/exec"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestOnWake checks that a gap between the wall clock and the monotonic
// clock is taken for a sleep, and a short one is not.
func TestOnWake(t *testing.T) {
	var asleep atomic.Int64 // how far the wall clock has moved past the monotonic one
	start := time.Now()
	clock := func() (time.Time, time.Duration) {
		mono := time.Since(start)
		return start.Round(0).Add(mono + time.Duration(asleep.Load())), mono
	}
	woke := make(chan struct{}, 4)
	stop := watch(func() { woke <- struct{}{} }, 20*time.Millisecond, time.Minute, clock)
	defer stop()
	time.Sleep(100 * time.Millisecond)
	if len(woke) != 0 {
		t.Fatal("woke without sleeping")
	}
	asleep.Store(int64(30 * time.Second)) // shorter than the shortest sleep
	time.Sleep(100 * time.Millisecond)
	if len(woke) != 0 {
		t.Fatal("a short gap was taken for a sleep")
	}
	asleep.Add(int64(2 * time.Hour))
	select {
	case <-woke:
	case <-time.After(2 * time.Second):
		t.Fatal("two hours asleep were not noticed")
	}
	time.Sleep(100 * time.Millisecond)
	if len(woke) != 0 {
		t.Fatal("one sleep was taken for two")
	}
}

// TestHold checks, on the Mac, that caffeinate runs while held and stops
// when released; elsewhere, that Hold returns.
func TestHold(t *testing.T) {
	release, err := Hold("vero's test")
	if runtime.GOOS != "darwin" {
		release()
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	running := func() bool {
		out, _ := exec.Command("pgrep", "-f", "caffeinate -i -w").Output()
		return strings.TrimSpace(string(out)) != ""
	}
	if !running() {
		t.Error("caffeinate is not running")
	}
	release()
	release()
	time.Sleep(200 * time.Millisecond)
	if running() {
		t.Error("caffeinate still runs after release")
	}
}
