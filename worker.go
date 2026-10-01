package vero

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"reflect"
	"sync"
	"time"
)

// envServe is set by a Supervisor in the worker's environment.  It is not a
// command line flag because the worker's own flags belong to the application,
// and a library has no business claiming names in that space.
const envServe = "VERO_SERVE"

// WorkerOptions controls what a worker writes, and where.
//
// The two settings are deliberately separate.  Format and volume are different
// questions: a frontend wants every state change as JSON, and a person
// reading a terminal or a journal wants a handful of lines saying what
// actually happened.  Emitting the first to satisfy the second is how logs
// become unreadable.
type WorkerOptions struct {
	// JSON emits the event stream on standard output.  A Supervisor turns
	// this on for you; set it yourself when a script is watching.
	JSON bool

	// Quiet suppresses the human readable log on standard error.
	Quiet bool

	// Version is what -version reports.  A frontend that ships this worker
	// in its bundle asks for it to decide whether the copy it has on disk is
	// older than the one it was built with, so the two halves have to agree
	// on the flag: vero registers it here so they cannot drift.
	Version string

	// StateInterval is how often the state is sampled for a change, for a
	// worker that has one.  Zero means DefaultStateInterval.
	StateInterval time.Duration

	// ShowVersion is set by RegisterFlags when -version was passed. Answer it
	// and exit before anything else starts - it runs as its own short-lived
	// process, several times per launch, and must not open a port, touch a
	// configuration directory, or print anything else.
	ShowVersion bool
}

// RegisterFlags adds -json and -quiet to fs.  Call it before flag.Parse.
//
// A worker does not need a flag for serve mode: a Supervisor sets it through
// the environment, so nothing on the command line has to describe an
// arrangement the user did not ask for.
func (o *WorkerOptions) RegisterFlags(fs *flag.FlagSet) {
	fs.BoolVar(&o.JSON, "json", false, "emit the event stream as JSON on stdout")
	fs.BoolVar(&o.Quiet, "quiet", false, "suppress the log on stderr")
	fs.BoolVar(&o.ShowVersion, "version", false, "print the version and exit")
}

// PrintVersionAndExit answers -version if it was asked for, and does not
// return in that case.
//
// Call it immediately after flag.Parse, before anything else starts.
func (o *WorkerOptions) PrintVersionAndExit() {
	if !o.ShowVersion {
		return
	}
	v := o.Version
	if v == "" {
		v = "0.0.0"
	}
	fmt.Println(v)
	os.Exit(0)
}

// Worker is the logic half of a vero application.
type Worker struct {
	opts   WorkerOptions
	serve  bool // a supervisor launched us, so stdin carries requests
	router *router
	state  state // set by NewState; nil for a worker that emits by hand

	// Cancelled when the frontend goes away, so everything this worker
	// started - the state emitter, an Every loop - stops with it.
	ctx    context.Context
	cancel context.CancelFunc

	in  io.Reader
	out io.Writer
	err io.Writer

	mu  sync.Mutex // serialises writes to out; one line must not interleave
	enc *json.Encoder
}

// NewWorker prepares a worker.  Whether a supervisor launched it is read from
// the environment, so the same binary and the same code run both ways.
//
// # Call this before starting any goroutine
//
// In serve mode standard output belongs to the protocol, so a stray Println
// anywhere in the program would corrupt a frame.  Rather than ask every author
// of every line in the program to remember that, this takes the real standard
// output for itself and points the package-level os.Stdout at standard error.
//
// fmt.Println and friends read os.Stdout at the moment they are called, so
// every existing print in the program - and every one anybody adds later, in
// any package, including ones you depend on and cannot edit - lands harmlessly
// on stderr from here on.  There is nothing to audit and nothing to keep
// auditing.
//
// It does mean reassigning a package-level variable, so call NewWorker before
// anything else could be printing: another goroutine writing while this
// reassigns is a data race, and one the race detector will find.
func NewWorker(opts WorkerOptions) *Worker {
	// A host supervises a copy of this executable instead of serving, and
	// never comes back: nothing an application writes after NewWorker runs
	// twice, because the parent never reaches it.
	if Hosting() {
		host()
	}

	ctx, cancel := context.WithCancel(context.Background())
	w := &Worker{
		opts:   opts,
		router: newRouter(),
		ctx:    ctx,
		cancel: cancel,
		serve:  os.Getenv(envServe) != "",
		in:     os.Stdin,
		out:    os.Stdout,
		err:    os.Stderr,
	}
	if w.serve {
		// Take the real stdout for the protocol, and send everyone else's
		// prints to stderr.
		os.Stdout = os.Stderr
	}
	if w.serve || w.opts.JSON {
		w.enc = json.NewEncoder(w.out)
	}
	return w
}

// Supervised reports whether a Supervisor launched this worker.  Useful when
// the standalone form wants to behave differently - printing a summary and
// exiting, say, rather than running forever.
func (w *Worker) Supervised() bool { return w.serve }

// emit sends an event to whoever is listening.  When nothing is - a daemon
// running with no frontend and no -json - it costs a comparison and returns.
func (w *Worker) emit(event any) {
	if w.enc == nil {
		return
	}
	payload, err := json.Marshal(event)
	if err != nil {
		w.Log("cannot encode event: %v", err)
		return
	}
	w.write(Envelope{Kind: kindEvent, Payload: payload})
}

// EmitOnChange emits what snapshot returns, whenever it differs from what was
// emitted last. It runs until ctx is done, so start it with go.
//
//	go w.EmitOnChange(ctx, 250*time.Millisecond, func() any { return state() })
//
// Most workers do not call this: set WorkerOptions.State instead and Serve
// runs it for you, on a context that ends when the frontend does.  This is
// for the ones whose events are not one state snapshot.
//
// Emitting on a timer instead is the obvious thing to write and it defeats the
// point: a frontend is sent the same thing several times a second, and the
// quiet-when-nothing-happens property - the reason events are pushed rather
// than polled - is gone.
//
// Comparison is by reflect.DeepEqual, so snapshot should return a value rather
// than something holding a pointer to state that keeps changing underneath it.
func (w *Worker) EmitOnChange(ctx context.Context, interval time.Duration, snapshot func() any) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var last any
	first := true
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		current := snapshot()
		if !first && reflect.DeepEqual(current, last) {
			continue
		}
		first = false
		last = current
		w.emit(current)
	}
}

// emitState sends the state whenever its encoding differs from the last one
// sent.  Comparing the encoding rather than the value is what lets the state
// be read under its own lock: there is no copy handed out to go stale, and a
// change made while this is encoding waits for the next tick rather than
// producing half of one.
func (w *Worker) emitState(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var last []byte
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		current, err := w.state.encode()
		if err != nil {
			w.Log("cannot encode the state: %v", err)
			continue
		}
		if bytes.Equal(current, last) {
			continue
		}
		last = current
		if w.enc != nil {
			w.write(Envelope{Kind: kindEvent, Payload: current})
		}
	}
}

// Log writes a line to standard error, where logs belong.  Standard output is
// reserved for the protocol, and a stray write there corrupts the stream.
func (w *Worker) Log(format string, a ...any) {
	if w.opts.Quiet {
		return
	}
	fmt.Fprintf(w.err, format+"\n", a...)
}

func (w *Worker) write(e Envelope) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.enc != nil {
		w.enc.Encode(e) // Encode writes the newline for us
	}
}

// Serve answers the requests registered with Handle and Update.
//
// Otherwise identical to Run: it blocks, it returns when the frontend goes
// away, and on its own it never returns.
func (w *Worker) Serve() error {
	return w.serveEnvelopes(func(ctx context.Context, e Envelope) (any, error) {
		return w.router.route(ctx, e.Name, e.Payload)
	})
}

// Fallback answers requests whose name matches nothing registered, including
// requests that carry no name at all.
func (w *Worker) Fallback(h Handler) { w.router.Fallback(h) }

// FallbackCalls counts the requests that have reached the fallback, so an
// application can tell when the old shape has stopped being used.
func (w *Worker) FallbackCalls() uint64 { return w.router.FallbackCalls() }

func (w *Worker) serveEnvelopes(dispatch func(context.Context, Envelope) (any, error)) error {
	// w.ctx is cancelled when standard input closes, which is how the
	// frontend says it has gone. Handlers that wait have to notice, or this
	// process cannot leave with it.
	ctx, cancel := w.ctx, w.cancel
	defer cancel()

	// Before the standalone check, so `worker -json` on its own emits too.
	if w.state != nil {
		interval := w.opts.StateInterval
		if interval <= 0 {
			interval = DefaultStateInterval
		}
		go w.emitState(ctx, interval)
	}

	if !w.serve {
		select {} // nothing will ever send us a request
	}

	sc := bufio.NewScanner(w.in)
	sc.Buffer(make([]byte, 0, 64<<10), MaxLineSize)

	var wg sync.WaitGroup
	for sc.Scan() {
		line := make([]byte, len(sc.Bytes()))
		copy(line, sc.Bytes()) // the scanner reuses its buffer

		var req Envelope
		if err := json.Unmarshal(line, &req); err != nil {
			w.Log("ignoring an unreadable request: %v", err)
			continue
		}

		// One goroutine per request, so a slow handler holds up neither the
		// events behind it nor the next request.
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				// A panic in a handler should cost that one request, not the
				// worker.  Go's unrecoverable failures still take the process
				// down, which is what the supervisor is for.
				if r := recover(); r != nil {
					w.Log("handler panicked: %v", r)
					w.write(Envelope{Kind: kindReply, ID: req.ID, Error: fmt.Sprintf("handler panicked: %v", r)})
				}
			}()
			reply, err := dispatch(ctx, req)
			w.write(replyEnvelope(req.ID, reply, err))
		}()
	}
	// Standard input has closed, so nobody is going to read another reply.
	// Tell the handlers, then leave whether or not they listened: an orphaned
	// worker holding a folder open is worse than one that skipped its
	// cleanup, and the alternative is not exiting at all.
	cancel()

	finished := make(chan struct{})
	go func() { wg.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(ShutdownGrace):
		w.Log("a handler did not stop within %s; exiting anyway", ShutdownGrace)
	}

	if err := sc.Err(); err != nil {
		return fmt.Errorf("vero: reading requests: %w", err)
	}
	return nil
}

// ShutdownGrace is how long Run waits for handlers to notice that the
// frontend has gone before exiting regardless.
var ShutdownGrace = 5 * time.Second
