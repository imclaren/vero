package vero_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/imclaren/vero"
)

// The same trick the other tests use: this binary is the worker.
func lockOpts() vero.SupervisorOptions {
	return vero.SupervisorOptions{
		Path: os.Args[0],
		Env:  append(os.Environ(), envTestWorker+"=1"),
	}
}

// Two supervisors, one worker: the second must not start a second worker.
func TestSecondSupervisorIsRefused(t *testing.T) {
	first := vero.Supervise(lockOpts())
	defer first.Stop()
	if err := first.Err(); err != nil {
		t.Fatalf("the first supervisor should hold the lock: %v", err)
	}

	second := vero.Supervise(lockOpts())
	defer second.Stop()
	if !errors.Is(second.Err(), vero.ErrAlreadyRunning) {
		t.Fatalf("second supervisor: got %v, want ErrAlreadyRunning", second.Err())
	}

	// And it says so when asked to do something, rather than looking like a
	// worker that has not come up yet.
	_, err := second.Call(context.Background(), "anything", struct{}{})
	if !errors.Is(err, vero.ErrAlreadyRunning) {
		t.Fatalf("request on the locked-out supervisor: got %v, want ErrAlreadyRunning", err)
	}
}

// Stopping the first hands the lock over.
func TestTheLockIsReleasedOnStop(t *testing.T) {
	first := vero.Supervise(lockOpts())
	if err := first.Err(); err != nil {
		t.Fatalf("first: %v", err)
	}
	first.Stop()

	second := vero.Supervise(lockOpts())
	defer second.Stop()
	if err := second.Err(); err != nil {
		t.Fatalf("after the first stopped, the second should get the lock: %v", err)
	}
}

// Different keys are different locks, for two applications sharing a binary.
func TestSeparateKeysDoNotCollide(t *testing.T) {
	oa := lockOpts()
	oa.Lock = "vero-test-key-a"
	a := vero.Supervise(oa)
	defer a.Stop()

	ob := lockOpts()
	ob.Lock = "vero-test-key-b"
	b := vero.Supervise(ob)
	defer b.Stop()

	if err := a.Err(); err != nil {
		t.Fatalf("a: %v", err)
	}
	if err := b.Err(); err != nil {
		t.Fatalf("b: %v", err)
	}
}

// NoLock is the way out for a program that wants several.
func TestNoLockAllowsTwo(t *testing.T) {
	oa := lockOpts()
	oa.NoLock = true
	a := vero.Supervise(oa)
	defer a.Stop()

	ob := lockOpts()
	ob.NoLock = true
	b := vero.Supervise(ob)
	defer b.Stop()

	if err := a.Err(); err != nil {
		t.Fatalf("a: %v", err)
	}
	if err := b.Err(); err != nil {
		t.Fatalf("b: %v", err)
	}
}
