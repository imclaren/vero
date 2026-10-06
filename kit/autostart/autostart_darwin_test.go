//go:build darwin && !ios

package autostart

import (
	"strings"
	"testing"
)

// TestAgent: the LaunchAgent for an app bundle opens it as the app.
func TestAgent(t *testing.T) {
	data, err := agent(App{ID: "dev.vero.test", Exec: []string{"/Applications/vero test.app"}})
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{"<key>Label</key>", "<string>dev.vero.test</string>", "<string>/usr/bin/open</string>", "<string>/Applications/vero test.app</string>", "<true></true>"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s in:\n%s", want, s)
		}
	}
}
