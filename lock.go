package vero

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// ErrAlreadyRunning is returned when another process is already supervising
// this worker.
//
// Two copies of an application - the one in /Applications and the one still in
// ~/Downloads after an update - are two processes, and without this they would
// each start a worker and both would work on the same state.  One window too
// many is a nuisance; two workers is corruption.
var ErrAlreadyRunning = errors.New("a worker is already running for this application")

// lockPath is where the lock for key lives: one file per key, in this user's
// cache directory, named by a hash so the key can be any path at all.
func lockPath(key string) string {
	sum := sha256.Sum256([]byte(key))
	name := "vero-" + hex.EncodeToString(sum[:8]) + ".lock"

	dir, err := os.UserCacheDir()
	if err == nil && dir != "" {
		dir = filepath.Join(dir, "vero")
		if err := os.MkdirAll(dir, 0o700); err == nil {
			return filepath.Join(dir, name)
		}
	}
	// A per-user directory is better - another user cannot squat the name -
	// but a lock in the shared one beats no lock.
	return filepath.Join(os.TempDir(), name)
}

// acquireLock takes the single-worker lock, or reports who has it.
//
// The lock is held by the open file, not by anything written in it, so it goes
// when this process does - including when it is killed, which is the case that
// matters.  A lock that outlived a crash would need clearing by hand.
func (s *Supervisor) acquireLock() error {
	key := s.opts.Lock
	if key == "" {
		key = s.opts.Path
		if abs, err := filepath.Abs(key); err == nil {
			key = abs
		}
	}

	path := lockPath(key)
	f, err := openLockFile(path)
	if err != nil {
		// On Plan 9 the open is the lock, so this is where a second copy of
		// an application finds out, rather than at lockFile below.
		if errors.Is(err, ErrAlreadyRunning) {
			return err
		}
		return fmt.Errorf("cannot open the worker lock: %w", err)
	}
	if err := lockFile(f); err != nil {
		held, _ := os.ReadFile(path)
		f.Close()
		if pid := string(held); pid != "" {
			return fmt.Errorf("%w (process %s)", ErrAlreadyRunning, pid)
		}
		return ErrAlreadyRunning
	}

	// Only for the message above: whoever comes second can say who has it.
	if err := f.Truncate(0); err == nil {
		f.WriteAt([]byte(strconv.Itoa(os.Getpid())), 0)
		f.Sync()
	}
	s.lock = f
	return nil
}

// releaseLock lets the next process have it.  The file stays: removing it
// races with whoever is opening it next.
func (s *Supervisor) releaseLock() {
	if s.lock == nil {
		return
	}
	unlockFile(s.lock)
	s.lock.Close()
	s.lock = nil
}
