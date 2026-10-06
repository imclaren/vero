//go:build linux || freebsd || openbsd || netbsd

package notify

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"
)

// The desktop's notification service: org.freedesktop.Notifications,
// which GNOME, KDE and the rest provide. (DragonFly, illumos and Solaris
// are left out: the D-Bus library does not build there.)
func send(ctx context.Context, n Notification) error {
	conn, err := dbus.ConnectSessionBus(dbus.WithContext(ctx))
	if err != nil {
		return err
	}
	defer conn.Close()
	timeout := int32(-1)
	if n.Timeout > 0 {
		timeout = int32(n.Timeout / time.Millisecond)
	}
	hints := map[string]dbus.Variant{}
	if n.ID != "" {
		hints["desktop-entry"] = dbus.MakeVariant(n.ID)
	}
	obj := conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")
	return obj.CallWithContext(ctx, "org.freedesktop.Notifications.Notify", 0,
		n.App, uint32(0), n.Icon, n.Title, n.Body, []string{}, hints, timeout).Err
}
