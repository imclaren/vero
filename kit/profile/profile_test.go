package profile

import (
	"path/filepath"
	"testing"
)

func TestProfile(t *testing.T) {
	t.Setenv("VERO_PROFILE", "")
	if Name() != "" || Service("com.example.app") != "com.example.app" || Folder("app") != "app" {
		t.Error("without a profile, names changed")
	}
	t.Setenv("VERO_PROFILE", "demo")
	if Name() != "demo" || Service("com.example.app") != "com.example.app.demo" || Folder("app") != "app-demo" {
		t.Errorf("with demo: %q %q %q", Name(), Service("com.example.app"), Folder("app"))
	}
	if dir, err := Dir("app"); err != nil || filepath.Base(dir) != "app-demo" {
		t.Errorf("Dir: %s %v", dir, err)
	}
	for _, bad := range []string{"../x", "a b", "a/b", "é"} {
		t.Setenv("VERO_PROFILE", bad)
		if Name() != "" {
			t.Errorf("%q was taken as a profile", bad)
		}
	}
}
