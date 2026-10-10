// Package profile keeps one copy of an app apart from another on the same
// computer: a test or a demo beside the copy somebody uses. With
// VERO_PROFILE set, to "demo" say, the app's settings folder is
// myapp-demo rather than myapp, and its keychain items are under
// com.example.myapp.demo, so that the copy under test can neither read
// nor change the other's. Without it, nothing changes.
//
// vero's tests set it through vero-app.toml's [test] env:
//
//	[test]
//	env = { VERO_PROFILE = "test" }
//
// kit/keychain applies it to every Item's service by itself, and vero's
// Swift package to the folder its copy of the worker runs from.
package profile

import (
	"os"
	"path/filepath"
)

// Name is VERO_PROFILE, when it is letters, digits and hyphens, and ""
// otherwise, which is the app's own copy.
func Name() string {
	name := os.Getenv("VERO_PROFILE")
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
			return ""
		}
	}
	return name
}

// Service is a keychain service for this profile: service itself, or
// service.NAME.
func Service(service string) string {
	if name := Name(); name != "" {
		return service + "." + name
	}
	return service
}

// Folder is a folder's name for this profile: app itself, or app-NAME.
func Folder(app string) string {
	if name := Name(); name != "" {
		return app + "-" + name
	}
	return app
}

// Dir is the app's folder in the user's configuration folder, for this
// profile: ~/Library/Application Support/myapp on a Mac,
// %AppData%\myapp on Windows and ~/.config/myapp elsewhere, or
// myapp-NAME in each. It is not made.
func Dir(app string) (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, Folder(app)), nil
}
