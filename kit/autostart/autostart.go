// Package autostart opens an app when the person signs in: a LaunchAgent
// on macOS, a Run registry entry on Windows, and an autostart .desktop
// file on Linux, the BSDs and illumos.
//
// The worker can do all of this itself, so a front end's "Open at login"
// switch can be one request to the worker. On macOS, though, an app in a
// bundle is better off asking the system from Swift - SMAppService.mainApp
// since macOS 13 - which shows the app under Login Items in System
// Settings; what this package writes there appears as a background item
// instead. Use this on macOS for a worker that is not inside an app.
package autostart

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// App is what to open at sign-in.
type App struct {
	// ID is the app's reverse-domain id, such as "com.example.myapp",
	// which names the entry.
	ID string
	// Name is what people see, where the system shows it.
	Name string
	// Exec is the program to run: the app's executable, or the app bundle
	// on macOS, with any arguments after it.
	Exec []string
	// Icon is the name of the icon to show, where there is a place for it
	// (the .desktop file's Icon=); the app's id does for an installed app.
	Icon string
}

// Enable opens the app at every sign-in from now on.
func Enable(a App) error {
	if a.ID == "" || len(a.Exec) == 0 {
		return errors.New("autostart: an id and a program are needed")
	}
	return enable(a)
}

// Disable stops opening the app at sign-in.
func Disable(a App) error {
	if a.ID == "" {
		return errors.New("autostart: an id is needed")
	}
	return disable(a)
}

// Enabled says whether the app opens at sign-in.
func Enabled(a App) (bool, error) {
	if a.ID == "" {
		return false, errors.New("autostart: an id is needed")
	}
	return enabled(a)
}

// quote joins a command line the way a .desktop file's Exec= and a shell
// read it: each argument in double quotes when it needs them.
func quote(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \t\"'\\$`") {
			a = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`, "`", "\\`").Replace(a) + `"`
		}
		out[i] = a
	}
	return strings.Join(out, " ")
}

// writeFile writes a file whole, making its folder.
func writeFile(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func configHome() (string, error) {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return x, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("autostart: %w", err)
	}
	return filepath.Join(home, ".config"), nil
}
