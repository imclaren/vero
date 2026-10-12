package main

import (
	"os"
	"slices"
	"strings"
	"testing"

	tomlpkg "github.com/BurntSushi/toml"
)

// TestUpgradeToml checks, on audiobooks' vero-app.toml from before vero
// 0.18, that the rewritten file asks each system for everything the old
// one did, keeps what vero does not work out, and is rewritten once.
func TestUpgradeToml(t *testing.T) {
	data, err := os.ReadFile("testdata/upgrade/audiobooks.toml")
	if err != nil {
		t.Fatal(err)
	}
	load := func(text string) *GTK {
		var a App
		md, err := tomlpkg.Decode(text, &a)
		if err != nil {
			t.Fatal(err)
		}
		for _, k := range md.Undecoded() {
			if !(len(k) > 3 && k[0] == "test" && k[1] == "step") {
				t.Fatalf("unknown setting %s", k)
			}
		}
		if err := a.GTK.resolveDeps(); err != nil {
			t.Fatal(err)
		}
		return a.GTK
	}
	before := load(string(data))
	text, changes := upgradeToml(string(data))
	if len(changes) == 0 || changes[0] != "[gtk] uses adwaita, webview, keyring, and extra ffmpeg" {
		t.Fatalf("changes: %q", changes)
	}
	after := load(text)
	// Every package the old file asked for is asked for still.
	for what, pair := range map[string][2][]string{
		"deb depends":     {splitList(before.Deb.Depends), splitList(after.Deb.Depends)},
		"deb recommends":  {splitList(before.Deb.Recommends), splitList(after.Deb.Recommends)},
		"rpm requires":    {splitList(before.RPM.Requires), splitList(after.RPM.Requires)},
		"arch depends":    {splitList(before.Arch.Depends), splitList(after.Arch.Depends)},
		"alpine depends":  {splitList(before.Alpine.Depends), splitList(after.Alpine.Depends)},
		"void depends":    {splitList(before.Void.Depends), splitList(after.Void.Depends)},
		"netbsd depends":  {before.NetBSD.Depends, after.NetBSD.Depends},
		"openbsd depends": {before.OpenBSD.Depends, after.OpenBSD.Depends},
	} {
		for _, d := range pair[0] {
			if !covered(d, pair[1]) {
				t.Errorf("%s: %q is no longer asked for: %v", what, d, pair[1])
			}
		}
	}
	for k, v := range before.FreeBSD.ByArch["armv7"].Deps {
		if after.FreeBSD.ByArch["armv7"].Deps[k] != v {
			t.Errorf("freebsd armv7: %s is no longer %s", k, v)
		}
	}
	if after.FreeBSD.ByArch["armv7"].Python != "python3.11" || after.DragonFly.Python != "python3.11" || after.NetBSD.Python != "python3.12" {
		t.Error("a system's Python changed")
	}
	// illumos: OpenIndiana's own packages, in place of pkgsrc's.
	if !strings.Contains(strings.Join(after.Illumos.Depends, " "), "library/desktop/gtk4") || strings.Contains(strings.Join(after.Illumos.Depends, " "), "[0-9]*") || after.Illumos.Python != "/usr/bin/python3.9" {
		t.Errorf("illumos: %+v", after.Illumos)
	}
	// What vero does not work out stays: the processors, the section, the
	// Flatpak's own ffmpeg.
	for _, keep := range []string{`section = "sound"`, `arches = ["amd64", "arm64", "riscv64"`, `prepare = "scripts/build-ffmpeg-linux.sh"`, "[gtk.freebsd.by_arch.armv7]"} {
		if !strings.Contains(text, keep) {
			t.Errorf("lost %q", keep)
		}
	}
	if again, more := upgradeToml(text); again != text || len(more) != 0 {
		t.Errorf("a second upgrade changed it again: %q", more)
	}
	if !slices.Contains(after.Uses, "adwaita") {
		t.Error("no adwaita")
	}
}
