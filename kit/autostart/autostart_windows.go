package autostart

import (
	"errors"

	"golang.org/x/sys/windows/registry"
)

// A value under HKCU\...\Run, which Windows runs at sign-in. Windows shows
// it under Startup apps in Settings, where it can be switched off.
const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

func enable(a App) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(a.ID, quote(a.Exec))
}

func disable(a App) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(a.ID); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

func enabled(a App) (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false, err
	}
	defer k.Close()
	_, _, err = k.GetStringValue(a.ID)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
