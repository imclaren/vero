//go:build !(linux || freebsd || openbsd || netbsd || windows || darwin || ios)

package notify

import (
	"context"
	"errors"
)

func send(context.Context, Notification) error {
	return errors.New("notify: no notifications on this system")
}
