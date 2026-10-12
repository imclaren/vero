// Package awake keeps the computer from going to sleep while a worker has
// work that must finish, such as a sync, and tells it when the computer has
// woken from sleep, so that it can look again at what changed meanwhile.
//
//	release, err := awake.Hold("Syncing your folders")
//	defer release()
//
//	stop := awake.OnWake(func() { resync() })
//
// Hold uses caffeinate on macOS, SetThreadExecutionState on Windows, and
// logind's sleep inhibitor on Linux and the BSDs that run it. Where there
// is none, as on illumos, it returns ErrUnsupported and a release that does
// nothing, and the computer sleeps as it would have.
//
// OnWake works on every system, and needs nothing of it: the wall clock
// moves on while the computer sleeps, and a clock that counts only time
// awake does not, so a gap between them is a sleep. That clock is Go's
// monotonic one, which stops in sleep on macOS, Linux and the BSDs, and on
// Windows the unbiased interrupt time, which Windows documents as leaving
// out sleep and hibernation.
package awake

import (
	"errors"
	"sync"
	"time"
)

// ErrUnsupported is a system on which nothing can keep the computer awake.
var ErrUnsupported = errors.New("awake: this system cannot be kept awake")

// Hold keeps the computer from sleeping until release is called. reason
// is what the system shows, where it shows one. release may be called
// more than once.
func Hold(reason string) (release func(), err error) {
	stop, err := hold(reason)
	if err != nil {
		return func() {}, err
	}
	var once sync.Once
	return func() { once.Do(stop) }, nil
}

// OnWake calls woke, on a goroutine of its own, each time the computer
// wakes from a sleep longer than half a minute, within about ten seconds of
// waking. stop ends it.
func OnWake(woke func()) (stop func()) {
	start := awakeTime()
	return watch(woke, 10*time.Second, 30*time.Second, func() (time.Time, time.Duration) {
		return time.Now().Round(0), awakeTime() - start
	})
}

// awakeTime is a clock that stops while the computer sleeps: Go's own
// monotonic clock, except on Windows (awake_windows.go).
var start0 = time.Now()

func goAwakeTime() time.Duration { return time.Since(start0) }

// watch is OnWake with its interval, the shortest sleep, and the clock,
// which a test replaces: now gives the wall time and the monotonic time
// since a start, which stops while the computer sleeps.
func watch(woke func(), every, longest time.Duration, now func() (time.Time, time.Duration)) func() {
	done := make(chan struct{})
	go func() {
		lastWall, lastMono := now()
		tick := time.NewTicker(every)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				wall, mono := now()
				// What the wall clock moved, less what the monotonic one did:
				// the time the computer slept.
				if slept := wall.Sub(lastWall) - (mono - lastMono); slept > longest {
					woke()
				}
				lastWall, lastMono = wall, mono
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(done) }) }
}
