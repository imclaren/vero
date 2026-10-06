package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// toolsImage is vero's tools container: what makes Flatpaks and the rpm
// and Flatpak repositories, which have no Go equivalent: flatpak and
// ostree, createrepo_c, rpmsign, and gpg, which rpmsign and ostree sign
// with. vero-repo builds it the first time it's needed.
const toolsImage = "vero-tools:1"

const toolsDockerfile = `FROM debian:trixie-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
        flatpak ostree gnupg createrepo-c rpm ca-certificates \
    && rm -rf /var/lib/apt/lists/*
`

// tools runs scripts in vero's tools container, with the folders they use
// mounted at the same paths, and the signing key, if there is one,
// imported for gpg: importKey is the shell that does that, which a script
// starts with.
type tools struct {
	env       []string
	keyDir    string
	importKey string
}

// newTools checks that Docker is running, and builds the tools container
// if it isn't there yet. keyDir is the signing key's folder, or "".
func newTools(keyDir string) (*tools, error) {
	t := &tools{env: dockerEnv(), keyDir: keyDir}
	if err := t.docker(nil, "info"); err != nil {
		return nil, errors.New("docker isn't running - try: colima start")
	}
	if t.docker(nil, "image", "inspect", toolsImage) != nil {
		fmt.Println("making vero's tools container, once")
		if err := t.docker(strings.NewReader(toolsDockerfile), "build", "-q", "-t", toolsImage, "-"); err != nil {
			return nil, fmt.Errorf("making vero's tools container: %w", err)
		}
	}
	if keyDir != "" {
		t.importKey = fmt.Sprintf("export GNUPGHOME=$(mktemp -d)\ngpg --batch --quiet --import %s 2>/dev/null\n",
			shellQuote(filepath.Join(keyDir, privateFile)))
	}
	return t, nil
}

// run runs script in the tools container, with folders mounted.
func (t *tools) run(script string, folders ...string) error {
	return t.runIn(false, script, folders...)
}

// runFlatpak runs script as run does, in a privileged container: flatpak
// checks each icon in a sandbox of its own, which needs it.
func (t *tools) runFlatpak(script string, folders ...string) error {
	return t.runIn(true, script, folders...)
}

func (t *tools) runIn(privileged bool, script string, folders ...string) error {
	args := []string{"run", "--rm"}
	if privileged {
		args = append(args, "--privileged")
	}
	if t.keyDir != "" {
		folders = append(folders, t.keyDir)
	}
	home, _ := os.UserHomeDir()
	for _, f := range folders {
		abs, err := filepath.Abs(f)
		if err != nil {
			return err
		}
		// colima, like Docker Desktop, shares only the home folder.
		if !strings.HasPrefix(abs, home+string(filepath.Separator)) {
			return fmt.Errorf("%s is outside your home folder, which is all Docker can see", abs)
		}
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return err
		}
		args = append(args, "-v", abs+":"+abs)
	}
	args = append(args, toolsImage, "sh", "-ec", script)
	return t.docker(nil, args...)
}

func (t *tools) docker(stdin *strings.Reader, args ...string) error {
	cmd := exec.Command("docker", args...)
	cmd.Env = t.env
	if stdin != nil {
		cmd.Stdin = stdin
	}
	switch {
	case len(args) > 0 && args[0] == "run":
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	case len(args) > 0 && args[0] == "build":
		// Docker's own chatter only when it fails.
		out, err := cmd.CombinedOutput()
		if err != nil {
			os.Stderr.Write(out)
		}
		return err
	}
	return cmd.Run()
}

// dockerEnv is the environment Docker runs in. A Mac that once had Docker
// Desktop keeps "credsStore": "desktop" in ~/.docker/config.json after
// it's gone, and every pull then fails looking for its helper; as
// scripts/lib/docker.sh does, such a config is copied without it, with its
// contexts, and used instead.
func dockerEnv() []string {
	env := os.Environ()
	cfg := os.Getenv("DOCKER_CONFIG")
	home, _ := os.UserHomeDir()
	if cfg == "" {
		cfg = filepath.Join(home, ".docker")
	}
	data, err := os.ReadFile(filepath.Join(cfg, "config.json"))
	if err != nil {
		return env
	}
	var c map[string]any
	if json.Unmarshal(data, &c) != nil {
		return env
	}
	store, _ := c["credsStore"].(string)
	if store == "" {
		return env
	}
	if _, err := exec.LookPath("docker-credential-" + store); err == nil {
		return env
	}
	clean := filepath.Join(home, ".cache", "vero-docker")
	delete(c, "credsStore")
	delete(c, "credHelpers")
	out, _ := json.Marshal(c)
	if os.MkdirAll(clean, 0o755) != nil || os.WriteFile(filepath.Join(clean, "config.json"), out, 0o644) != nil {
		return env
	}
	exec.Command("cp", "-R", filepath.Join(cfg, "contexts"), clean+"/").Run()
	return append(env, "DOCKER_CONFIG="+clean)
}

// shellQuote is each of args quoted for sh, joined by spaces.
func shellQuote(args ...string) string {
	q := make([]string, len(args))
	for i, a := range args {
		q[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(q, " ")
}

// fingerprint is the signing key's fingerprint, which gpg, rpmsign and
// ostree name it by.
func (s *signer) fingerprint() string {
	return strings.ToUpper(hex.EncodeToString(s.entity.PrimaryKey.Fingerprint))
}
