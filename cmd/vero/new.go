package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// newApp makes a vero app called name in a folder of that name: a Go
// module that requires vero, with vero's example worker as cmd/worker to
// start from. vero add then gives it its front ends and vero-app.toml.
// module is its module path, name when empty; local, when set, is a vero
// checkout to use in place of the published module.
func newApp(name, module, local string) error {
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`).MatchString(name) {
		return fmt.Errorf("%q: an app's name is lower case letters, digits and hyphens, such as my-app", name)
	}
	if _, err := os.Stat(name); err == nil {
		return fmt.Errorf("%s is there already: vero add adds to an app that exists", name)
	}
	if module == "" {
		module = name
	}
	if err := os.MkdirAll(filepath.Join(name, "cmd", "worker"), 0o755); err != nil {
		return err
	}
	mod := "module " + module + "\n\ngo 1.22\n"
	if local != "" {
		abs, err := filepath.Abs(local)
		if err != nil {
			return err
		}
		mod += "\nrequire github.com/imclaren/vero v0.0.0\n\nreplace github.com/imclaren/vero => " + abs + "\n"
	}
	if err := os.WriteFile(filepath.Join(name, "go.mod"), []byte(mod), 0o644); err != nil {
		return err
	}
	if local == "" {
		if err := run(name, "go", "get", "github.com/imclaren/vero@latest"); err != nil {
			return err
		}
	}
	out, err := output(name, "go", "list", "-m", "-f", "{{.Dir}}", "github.com/imclaren/vero")
	if err != nil {
		return err
	}
	src, err := os.ReadFile(filepath.Join(strings.TrimSpace(out), "example", "worker", "main.go"))
	if err != nil {
		return err
	}
	worker := regexp.MustCompile(`var version = "[^"]*"`).ReplaceAll(src, []byte(`var version = "0.1.0"`))
	if err := os.WriteFile(filepath.Join(name, "cmd", "worker", "main.go"), worker, 0o644); err != nil {
		return err
	}
	if err := run(name, "go", "mod", "tidy"); err != nil {
		return err
	}
	readme := "# " + name + "\n\nA vero app: a Go worker in `cmd/worker`, which vero's example started,\n" +
		"and a front end for each system in a folder of its own. `PORTING.md`\nsays what to do next.\n"
	return os.WriteFile(filepath.Join(name, "README.md"), []byte(readme), 0o644)
}

func run(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return nil
}

func output(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %v", name, strings.Join(args, " "), err)
	}
	return string(out), nil
}
