package vero

import (
	"io"
	"sync"
)

var (
	inProcessMu     sync.Mutex
	inProcessWorker func(in io.Reader, out io.Writer) error
)

// ServeInProcess hands vero a worker to run inside this process, for the
// frontends that cannot start one beside it.
//
// iOS is the case that needs it: an application there may not start another
// program, but it may compile Go in.  The application calls this before its
// frontend asks for a worker, with the same function it would have given
// SupervisorOptions.Serve:
//
//	vero.ServeInProcess(func(in io.Reader, out io.Writer) error {
//	    w := vero.NewWorker(vero.WorkerOptions{In: in, Out: out})
//	    jobs := vero.NewState(w, Status{})
//	    vero.Update(jobs, "addJob", func(s *Status) error { ... })
//	    return w.Serve()
//	})
//
// Everywhere else, leave it alone: a worker in a process of its own survives
// a panic that would take the frontend with it, which is the whole reason
// the two are separate when they can be.
func ServeInProcess(serve func(in io.Reader, out io.Writer) error) {
	inProcessMu.Lock()
	defer inProcessMu.Unlock()
	inProcessWorker = serve
}

// InProcessWorker returns what ServeInProcess was given, or nil.  The C shim
// uses it; an application has no reason to.
func InProcessWorker() func(in io.Reader, out io.Writer) error {
	inProcessMu.Lock()
	defer inProcessMu.Unlock()
	return inProcessWorker
}
