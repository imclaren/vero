//go:build !wasip1

package vero

// A request each on its own goroutine, which is what every real operating
// system wants: a slow handler then holds up neither the events behind it nor
// the next request.  See dispatch_wasip1.go for the one exception.
const serialDispatch = false
