package vero

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// SupervisorOptions describes the worker to run and who to tell about it.
//
// The callbacks are called from the supervisor's own goroutines, never on the
// frontend's main thread.  Whatever you do in them, hop to the main thread
// before touching the frontend.
type SupervisorOptions struct {
	// Path is the worker binary.  Args are passed to it verbatim, so the
	// application keeps its own command line.
	Path string
	Args []string

	// Dir and Env, if set, are the worker's working directory and
	// environment.  VERO_SERVE is added to Env either way.
	Dir string
	Env []string

	// Serve runs a worker in this process, reading requests from in and
	// writing events and replies to out, instead of Path being started as a
	// program.  Everything else is unchanged: the same requests, the same
	// events, the same replies, over an io.Pipe rather than a pipe to a
	// child.
	//
	// iOS is why this exists.  An application there may not start another
	// program, but it may compile Go in, so the worker runs on a goroutine
	// of its own.  Restarts lose their meaning when it does: a goroutine
	// dies with the process it is in, and there is nothing left to restart
	// it into.
	Serve func(in io.Reader, out io.Writer) error

	// OnEvent receives every event the worker emits, in order.
	OnEvent func(event json.RawMessage)

	// OnLog receives the worker's standard error, a line at a time.  Workers
	// log there precisely so this does not have to guess at framing.
	OnLog func(line string)

	// OnStateChange is told when the worker starts, dies, or is given up on.
	OnStateChange func(RunState)

	// Backoff is the wait before the first restart, doubling up to
	// MaxBackoff.  A worker that crashes on startup would otherwise be
	// relaunched as fast as the machine can fork.  Zero means 200ms and 30s.
	Backoff    time.Duration
	MaxBackoff time.Duration

	// Healthy is how long a worker has to run before the restarts behind it
	// are forgotten: the backoff returns to Backoff and Restarts returns to
	// zero.  Zero means 30 seconds.
	//
	// Workers exit on purpose - to pick up a change to their configuration,
	// or to run a version they have just downloaded - and one that served for
	// an hour before doing so has told us nothing except that it is working.
	// Without this the backoff climbs to MaxBackoff over a day of ordinary
	// use, so an intentional exit costs half a minute of nothing, and any
	// frontend watching the count concludes the worker is broken.
	Healthy time.Duration

	// Lock keys the single-worker lock: one process at a time supervises a
	// worker under a given key.  Empty means the worker's own path, which is
	// what you want unless two applications share a binary and should still
	// be allowed to run at once - then give them a key each.
	Lock string

	// NoLock turns the lock off, for a program that genuinely wants several
	// workers at once.
	NoLock bool
}

// RunState is what the supervisor last saw the worker doing.  It is the
// worker's lifecycle, not the state it publishes: that is Worker.NewState on
// the other side of the pipe.
type RunState int

const (
	// Starting means a worker is being launched, including a restart.
	Starting RunState = iota
	// Running means a worker is up and answering.
	Running
	// Restarting means one died and another is on the way.
	Restarting
	// Stopped means Stop was called; nothing further will be launched.
	Stopped
)

// String is what the bindings report: "starting", "running",
// "restarting" or "stopped".
func (s RunState) String() string {
	switch s {
	case Starting:
		return "starting"
	case Running:
		return "running"
	case Restarting:
		return "restarting"
	case Stopped:
		return "stopped"
	}
	return "unknown"
}

// Supervisor runs a worker, keeps it running, and talks to it.
type Supervisor struct {
	opts SupervisorOptions

	mu      sync.Mutex
	state   RunState
	stdin   io.WriteCloser
	enc     *json.Encoder
	nextID  uint64
	pending map[uint64]chan Envelope
	latest  json.RawMessage
	cmd     *exec.Cmd

	stop     chan struct{}
	stopOnce sync.Once
	done     chan struct{}
	restarts int

	lock     *os.File // the single-worker lock, held until Stop or exit
	startErr error    // why there is no worker at all, if there is not
}

// Supervise launches the worker and keeps it alive until Stop.
//
// It returns immediately; the worker comes up on its own goroutine.  Requests
// made before it is ready return ErrWorkerNotRunning rather than blocking, so
// a frontend can draw itself at once and show that it is waiting.
func Supervise(opts SupervisorOptions) *Supervisor {
	if opts.Backoff <= 0 {
		opts.Backoff = 200 * time.Millisecond
	}
	if opts.MaxBackoff <= 0 {
		opts.MaxBackoff = 30 * time.Second
	}
	if opts.Healthy <= 0 {
		opts.Healthy = 30 * time.Second
	}
	s := &Supervisor{
		opts:    opts,
		pending: map[uint64]chan Envelope{},
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	// A worker running in this process with no key of its own has nothing to
	// take a lock against: the key would be this directory, which says
	// nothing about which application is asking.  The lock is there to stop
	// two workers for one application, and in process there is one worker
	// because there is one goroutine.
	if opts.Serve != nil && opts.Lock == "" && opts.Path == "" {
		opts.NoLock = true
		s.opts = opts
	}

	// Before the goroutine: a caller that asks Err() straight away should get
	// a straight answer, not a race.
	if !opts.NoLock {
		if err := s.acquireLock(); err != nil {
			s.startErr = err
			s.state = Stopped
			close(s.done)
			return s
		}
	}

	go s.supervise()
	return s
}

// Err reports why the worker never started, or nil.
//
// The one that matters is ErrAlreadyRunning: another process holds the lock,
// so this Supervisor will not run a worker at all.
func (s *Supervisor) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startErr
}

// State reports what the worker is doing now.
func (s *Supervisor) State() RunState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Restarts counts how many times the worker has been relaunched without one
// of them lasting: a worker that runs for Healthy and then exits, whatever its
// reason, puts this back to zero.
//
// So it answers "is this worker failing to start", which is the question worth
// asking, and not "how many times has it been started", which counts every
// deliberate exit as evidence of a fault.
func (s *Supervisor) Restarts() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.restarts
}

// Latest returns the most recent event, or nil before the first one.
//
// A frontend that has just drawn a window needs the current state without
// waiting for the next change, and this is that.  Events keep it up to date
// afterwards.
func (s *Supervisor) Latest() json.RawMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.latest
}

// Request sends a request and waits for its reply.
//
// A handler's error comes back as *RemoteError, which is worth telling apart
// from ErrWorkerNotRunning: one means the worker refused what you asked, the
// other that it is not there to ask.
func (s *Supervisor) Request(ctx context.Context, request any) (json.RawMessage, error) {
	return s.Call(ctx, "", request)
}

// Call sends a request to the handler registered under name, for a worker
// registered by name.
//
// The name travels beside the payload rather than inside it, so an
// application's own message types stay exactly as it defined them.
func (s *Supervisor) Call(ctx context.Context, name string, request any) (json.RawMessage, error) {
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("vero: cannot encode request: %w", err)
	}

	s.mu.Lock()
	if s.startErr != nil {
		err := s.startErr
		s.mu.Unlock()
		// Say which of the two it is.  "Not running" invites waiting for a
		// worker that is never going to arrive.
		return nil, err
	}
	if s.state != Running || s.enc == nil {
		s.mu.Unlock()
		return nil, ErrWorkerNotRunning
	}
	s.nextID++
	id := s.nextID
	ch := make(chan Envelope, 1)
	s.pending[id] = ch
	err = s.enc.Encode(Envelope{ID: id, Name: name, Payload: payload})
	s.mu.Unlock()

	if err != nil {
		s.forget(id)
		return nil, ErrWorkerNotRunning
	}

	select {
	case reply := <-ch:
		if reply.Error != "" {
			return nil, &RemoteError{Message: reply.Error}
		}
		return reply.Payload, nil
	case <-ctx.Done():
		s.forget(id)
		return nil, ctx.Err()
	case <-s.stop:
		s.forget(id)
		return nil, ErrWorkerNotRunning
	}
}

func (s *Supervisor) forget(id uint64) {
	s.mu.Lock()
	delete(s.pending, id)
	s.mu.Unlock()
}

// Stop shuts the worker down and waits for it to go.
//
// Closing its standard input is the whole shutdown protocol: Run returns, the
// process exits, and anything it was holding is released the way it would be
// on any other exit.  A worker that ignores that is killed after a grace
// period.
func (s *Supervisor) Stop() error {
	s.stopOnce.Do(func() { close(s.stop) })
	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
	}
	s.releaseLock()
	return nil
}

func (s *Supervisor) setState(st RunState) {
	s.mu.Lock()
	s.state = st
	s.mu.Unlock()
	if s.opts.OnStateChange != nil {
		s.opts.OnStateChange(st)
	}
}

func (s *Supervisor) supervise() {
	defer close(s.done)
	backoff := s.opts.Backoff

	for {
		select {
		case <-s.stop:
			s.setState(Stopped)
			return
		default:
		}

		s.setState(Starting)
		started := time.Now()
		err := s.runOnce()
		lasted := time.Since(started)

		select {
		case <-s.stop:
			s.setState(Stopped)
			return
		default:
		}

		// A worker that ran is not a worker that cannot start.  Clearing the
		// history here is what keeps an exit on purpose - a configuration
		// change, a version it has just downloaded - from reading like the
		// latest in a run of failures.
		if lasted >= s.opts.Healthy {
			backoff = s.opts.Backoff
			s.mu.Lock()
			s.restarts = 0
			s.mu.Unlock()
		}

		s.mu.Lock()
		s.restarts++
		s.mu.Unlock()
		if s.opts.OnLog != nil && err != nil {
			s.opts.OnLog(fmt.Sprintf("vero: worker exited: %v", err))
		}
		s.setState(Restarting)

		select {
		case <-time.After(backoff):
		case <-s.stop:
			s.setState(Stopped)
			return
		}
		if backoff *= 2; backoff > s.opts.MaxBackoff {
			backoff = s.opts.MaxBackoff
		}
	}
}

// runOnce launches one worker and returns when it exits.
func (s *Supervisor) runOnce() error {
	if s.opts.Serve != nil {
		return s.runInProcess()
	}

	cmd := exec.Command(s.opts.Path, s.opts.Args...)
	hideConsole(cmd)
	cmd.Dir = s.opts.Dir
	env := s.opts.Env
	if env == nil {
		env = os.Environ()
	}
	cmd.Env = append(append([]string{}, env...), envServe+"=1")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	s.mu.Lock()
	s.stdin, s.enc, s.cmd = stdin, json.NewEncoder(stdin), cmd
	s.mu.Unlock()
	s.setState(Running)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); s.readEvents(stdout) }()
	go func() { defer wg.Done(); s.readLogs(stderr) }()

	// Stop closes stdin, which is how a worker is asked to leave.
	shutdown := make(chan struct{})
	go func() {
		select {
		case <-s.stop:
			stdin.Close()
			select {
			case <-shutdown:
			case <-time.After(3 * time.Second):
				cmd.Process.Kill() // it ignored us
			}
		case <-shutdown:
		}
	}()

	waitErr := cmd.Wait()
	close(shutdown)
	wg.Wait()

	// Nothing is coming back for anything still outstanding.
	s.mu.Lock()
	s.enc, s.stdin = nil, nil
	for id, ch := range s.pending {
		close(ch)
		delete(s.pending, id)
	}
	s.mu.Unlock()
	return waitErr
}

// runInProcess runs a worker on a goroutine and returns when it stops.
//
// The shape is runOnce's: pipes in place of the child's, the same reader for
// events, and the same clearing of whatever was outstanding when it ends.
// What it does not have is a process to kill, so Stop closes the request pipe
// and waits.
func (s *Supervisor) runInProcess() error {
	requests, toWorker := io.Pipe() // supervisor writes, worker reads
	fromWorker, events := io.Pipe() // worker writes, supervisor reads

	s.mu.Lock()
	s.stdin, s.enc, s.cmd = toWorker, json.NewEncoder(toWorker), nil
	s.mu.Unlock()
	s.setState(Running)

	served := make(chan error, 1)
	go func() {
		err := s.opts.Serve(requests, events)
		// Closing this is what ends readEvents, as a child exiting does.
		events.Close()
		served <- err
	}()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); s.readEvents(fromWorker) }()

	shutdown := make(chan struct{})
	go func() {
		select {
		case <-s.stop:
			toWorker.Close() // the worker sees its requests end
		case <-shutdown:
		}
	}()

	err := <-served
	close(shutdown)
	toWorker.Close()
	wg.Wait()

	s.mu.Lock()
	s.enc, s.stdin = nil, nil
	for id, ch := range s.pending {
		close(ch)
		delete(s.pending, id)
	}
	s.mu.Unlock()
	return err
}

func (s *Supervisor) readEvents(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), MaxLineSize)
	for sc.Scan() {
		var e Envelope
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue // a half written line from a dying worker
		}
		switch e.Kind {
		case kindEvent:
			s.mu.Lock()
			s.latest = e.Payload
			s.mu.Unlock()
			if s.opts.OnEvent != nil {
				s.opts.OnEvent(e.Payload)
			}
		case kindReply:
			s.mu.Lock()
			ch, ok := s.pending[e.ID]
			delete(s.pending, e.ID)
			s.mu.Unlock()
			if ok {
				ch <- e
			}
		}
	}
}

func (s *Supervisor) readLogs(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 8<<10), MaxLineSize)
	for sc.Scan() {
		if s.opts.OnLog != nil {
			s.opts.OnLog(sc.Text())
		}
	}
}
