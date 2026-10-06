//go:build darwin || ios

package notify

import "context"

func send(context.Context, Notification) error { return ErrFrontEnd }
