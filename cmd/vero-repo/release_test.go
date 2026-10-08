package main

import (
	"strings"
	"testing"
)

func TestNextVersion(t *testing.T) {
	for _, c := range []struct{ v, part, want string }{
		{"1.2.3", "patch", "1.2.4"},
		{"1.2.3", "minor", "1.3.0"},
		{"1.2.3", "major", "2.0.0"},
		{"2", "patch", "2.0.1"},
	} {
		got, err := nextVersion(c.v, c.part)
		if err != nil || got != c.want {
			t.Errorf("nextVersion(%q, %q) = %q, %v; want %q", c.v, c.part, got, err, c.want)
		}
	}
	if _, err := nextVersion("1.2.3", "huge"); err == nil {
		t.Error("an unknown part was accepted")
	}
	if _, err := nextVersion("v1.2", "patch"); err == nil {
		t.Error("a version that is not numbers was accepted")
	}
}

// TestPossibleTargets checks that release offers each front end the file
// has, in package's order, and nothing the file lacks.
func TestPossibleTargets(t *testing.T) {
	a := &App{GTK: &GTK{FreeBSD: &BSDPkg{}, OpenBSD: &OpenBSD{}}, WPF: &WPF{}, MacOS: &MacOS{}, Web: &Web{}}
	var names []string
	for _, p := range possibleTargets(a) {
		names = append(names, p.name)
	}
	if got := strings.Join(names, ","); got != "deb,rpm,freebsd,openbsd,web,macos,windows" {
		t.Errorf("possibleTargets = %s", got)
	}
	a.GTK.Flatpak.Build = true
	a.WPF.MSIX.Build = true
	names = nil
	for _, p := range possibleTargets(a) {
		names = append(names, p.name)
	}
	if got := strings.Join(names, ","); !strings.Contains(got, "flatpak") || !strings.Contains(got, "msix") {
		t.Errorf("flatpak and msix, when asked for: %s", got)
	}
	// Only Go is needed for most; the ones that need more say what.
	for _, name := range []string{"deb", "rpm", "freebsd", "web", "wasi", "plan9"} {
		if why := missingFor(name); why != "" {
			t.Errorf("%s needs %q, but should need only Go", name, why)
		}
	}
}

// TestSigningSummary checks the release's last word names each installer
// built and what signed it, and nothing that was not built.
func TestSigningSummary(t *testing.T) {
	t.Setenv("VERO_MAC_IDENTITY", "")
	t.Setenv("VERO_ANDROID_KEYSTORE", "")
	s := signingSummary(&App{}, "deb,macos,windows,android")
	for _, want := range []string{"Linux", "macOS", "ad hoc", "Windows", "SmartScreen", "Android", "keystore beside your key"} {
		if !strings.Contains(s, want) {
			t.Errorf("summary lacks %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "iOS") {
		t.Errorf("summary names iOS, which was not built:\n%s", s)
	}
	t.Setenv("VERO_MAC_IDENTITY", "Developer ID Application: Someone (TEAM)")
	t.Setenv("VERO_NOTARY_PROFILE", "vero")
	if s := signingSummary(&App{}, "macos"); !strings.Contains(s, "notarised") || strings.Contains(s, "ad hoc") {
		t.Errorf("with an identity and a profile:\n%s", s)
	}
}
