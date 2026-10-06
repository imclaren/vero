//go:build !darwin && !windows && !plan9 && !wasip1 && !js && !android

package autostart

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// A .desktop file in the autostart folder, which every desktop that follows
// freedesktop.org's Desktop Application Autostart Specification reads.
func path(a App) (string, error) {
	home, err := configHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "autostart", a.ID+".desktop"), nil
}

func enable(a App) error {
	p, err := path(a)
	if err != nil {
		return err
	}
	name := a.Name
	if name == "" {
		name = a.ID
	}
	icon := a.Icon
	if icon == "" {
		icon = a.ID
	}
	entry := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=%s\nExec=%s\nIcon=%s\nTerminal=false\nX-GNOME-Autostart-enabled=true\n", name, quote(a.Exec), icon)
	return writeFile(p, []byte(entry), 0o644)
}

func disable(a App) error {
	p, err := path(a)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func enabled(a App) (bool, error) {
	p, err := path(a)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(p)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
