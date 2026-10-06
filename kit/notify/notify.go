// Package notify shows a desktop notification from a worker, where the
// system lets a program that is not the app do that: on Linux and the
// BSDs, through the desktop's notification service over D-Bus, and
// on Windows through a toast.
//
// On macOS and iOS, notifications come from the app, through UserNotifications,
// and need its permission: the worker's Send returns ErrFrontEnd there, and
// the usual pattern is for the worker to put what is new in its state - a
// list of arrivals, say - and for the front end to notify. That pattern
// works everywhere, so an app that wants one path for every system can
// use it alone.
package notify

import (
	"context"
	"errors"
	"time"
)

// ErrFrontEnd says the front end has to show notifications on this
// system.
var ErrFrontEnd = errors.New("notify: on this system notifications come from the app, not the worker")

// Notification is one notification.
type Notification struct {
	// App is the app's name, shown as who the notification is from where
	// the system shows that.
	App string
	// ID is the app's reverse-domain id, which Windows and some desktops
	// match to its installed icon.
	ID    string
	Title string
	Body  string
	// Icon is an icon name or file path, for desktops that take one.
	Icon string
	// Timeout is how long it stays, where the desktop honours that; zero
	// for the desktop's default.
	Timeout time.Duration
}

// Send shows the notification.
func Send(ctx context.Context, n Notification) error {
	if n.Title == "" && n.Body == "" {
		return errors.New("notify: nothing to say")
	}
	return send(ctx, n)
}
