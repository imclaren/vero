//go:build wasip1

package vero

// WASI has no threads, and a read on standard input is a call into the host
// that parks the whole instance: while the worker waits for the next request,
// no other goroutine of its runs.  A handler on a goroutine of its own would
// therefore compute its reply and then sit on it until the next request
// arrived, which from the frontend looks like a worker that never answers.
//
// So a WASI worker answers one request at a time, in the goroutine that read
// it, and the reply is written before the next read parks anything.  The cost
// is the concurrency: a slow handler does hold up the requests behind it here.
// A worker given an in-memory pipe instead of standard input - the mode iOS
// uses - keeps its goroutine per request, because nothing is parked.
const serialDispatch = true
