//go:build linux || freebsd || openbsd || netbsd

package awake

import (
	"os"

	"github.com/godbus/dbus/v5"
)

// Linux and the BSDs that run it: logind's sleep inhibitor, which holds
// while the file it gives is open. Without logind, as on most BSDs and on a
// Linux without systemd or elogind, the computer cannot be held awake.
func hold(reason string) (func(), error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, ErrUnsupported
	}
	var fd dbus.UnixFD
	err = conn.Object("org.freedesktop.login1", "/org/freedesktop/login1").
		Call("org.freedesktop.login1.Manager.Inhibit", 0, "sleep", "vero", reason, "block").Store(&fd)
	if err != nil {
		conn.Close()
		return nil, ErrUnsupported
	}
	f := os.NewFile(uintptr(fd), "logind-inhibit")
	return func() {
		f.Close()
		conn.Close()
	}, nil
}
