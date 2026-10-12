package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// workerTargets are the systems and processors, as Go names them, for
// which the app's packages need its worker: each Linux package's
// processors, each BSD's and illumos', Windows' and the Mac's.
func workerTargets(a *App) [][2]string {
	var out [][2]string
	add := func(goos, goarch string) {
		t := [2]string{goos, goarch}
		if !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	if a.MacOS != nil {
		add("darwin", "arm64")
		add("darwin", "amd64")
	}
	if a.WPF != nil {
		arches, _ := a.WPF.arches()
		for _, w := range arches {
			add("windows", map[string]string{"x64": "amd64", "arm64": "arm64", "x86": "386"}[w])
		}
	}
	if a.GTK != nil {
		sections := []string{"deb", "rpm", "arch"}
		for name, p := range map[string]*LinuxPkg{"alpine": a.GTK.Alpine, "void": a.GTK.Void, "chimera": a.GTK.Chimera} {
			if p != nil {
				sections = append(sections, name)
			}
		}
		slices.Sort(sections)
		for _, arch := range a.linuxGoarches(sections...) {
			add("linux", arch)
		}
		present := map[string]bool{"freebsd": a.GTK.FreeBSD != nil, "dragonfly": a.GTK.DragonFly != nil,
			"netbsd": a.GTK.NetBSD != nil, "illumos": a.GTK.Illumos != nil, "openbsd": a.GTK.OpenBSD != nil}
		for i := range unixSystems {
			sys := &unixSystems[i]
			if present[sys.name] {
				for _, arch := range a.archesFor(sys) {
					add(sys.goos, arch.goarch)
				}
			}
		}
	}
	return out
}

// workerChecks builds the worker for each target, as the release would,
// and says which do not build, and the first error for each, which names
// the package that stops it. A new app finds such a target at once, and
// not at its release.
func workerChecks(a *App) []PlanStep {
	targets := workerTargets(a)
	steps := make([]PlanStep, len(targets))
	var wg sync.WaitGroup
	limit := make(chan struct{}, 6)
	for i, t := range targets {
		wg.Add(1)
		go func(i int, goos, goarch string) {
			defer wg.Done()
			limit <- struct{}{}
			defer func() { <-limit }()
			name := goos + "/" + goarch
			cmd := exec.Command("go", "build", "-o", os.DevNull, "./"+filepath.ToSlash(a.Worker.Package))
			cmd.Dir = a.dir
			cmd.Env = append(append(os.Environ(), "GOOS="+goos, "GOARCH="+goBuildArch(goarch), "CGO_ENABLED=0"), goarchEnv(goarch)...)
			if goos == "darwin" && a.Worker.CGO {
				cmd.Env = append(cmd.Env, "CGO_ENABLED=1")
			}
			out, err := cmd.CombinedOutput()
			if err == nil {
				steps[i] = PlanStep{Name: name, OK: true}
				return
			}
			why := err.Error()
			for _, line := range strings.Split(string(out), "\n") {
				if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
					why = line
					break
				}
			}
			steps[i] = PlanStep{Name: name, Why: why}
		}(i, t[0], t[1])
	}
	wg.Wait()
	return steps
}

// workerText is what the plan says of the worker's builds.
func workerText(steps []PlanStep) string {
	var b strings.Builder
	failed := 0
	for _, s := range steps {
		if !s.OK {
			failed++
			fmt.Fprintf(&b, "  %-18s %s: %s\n", "worker fails on", s.Name, s.Why)
		}
	}
	if failed == 0 && len(steps) > 0 {
		fmt.Fprintf(&b, "  %-18s every one of the %d systems and processors it is packaged for\n", "the worker builds", len(steps))
	}
	return b.String()
}
