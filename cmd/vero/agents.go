package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// The app's copy of vero's AGENTS.md, the guide for coding agents, which
// vero new writes and vero upgrade replaces. It is vero's alone, as the
// bindings are: the app's own rules go in a file of the app's own, which
// vero never touches, so that an upgrade has nothing to merge.
const agentsName = "AGENTS.md"

// agentsHeader begins every copy that vero writes, which is how vero tells
// its copy from an AGENTS.md of the app's own.
const agentsHeader = "<!-- vero writes this file, and vero upgrade replaces it. Do not edit it. -->\n"

const agentsNote = "> vero writes this file from its own `AGENTS.md`, and `vero upgrade`\n" +
	"> replaces it with each new vero's. Do not edit it. This app's own rules\n" +
	"> belong in a file of the app's own, such as `CLAUDE.md`, which vero never\n" +
	"> changes. Read that file as well, since its rules about this app come first.\n\n"

// veroModule is the folder of the vero module that the app's go.mod
// requires: a checkout, when go.mod replaces vero with one.
func veroModule(dir string) (string, error) {
	out, err := output(dir, "go", "list", "-m", "-f", "{{.Dir}}", "github.com/imclaren/vero")
	if err != nil {
		return "", err
	}
	if out = strings.TrimSpace(out); out == "" {
		return "", fmt.Errorf("go.mod does not require github.com/imclaren/vero")
	}
	return out, nil
}

// agentsCopy is the app's copy of vero's guide, from the vero module in
// src, or nil for a vero that has none. Links within vero's repository
// become links to it on GitHub, since the app has no copy of vero beside it.
func agentsCopy(src string) ([]byte, error) {
	guide, err := os.ReadFile(filepath.Join(src, agentsName))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	link := regexp.MustCompile(`\]\(([^)#:]+)((?:#[^)]*)?)\)`)
	guide = link.ReplaceAll(guide, []byte("](https://github.com/imclaren/vero/blob/main/$1$2)"))
	return append([]byte(agentsHeader+agentsNote), guide...), nil
}

// writeAgents writes the guide into a new app, and records it, so that
// vero upgrade can tell an edit from vero's copy.
func writeAgents(dir, src string) error {
	fresh, err := agentsCopy(src)
	if err != nil || fresh == nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, agentsName), fresh, 0o644); err != nil {
		return err
	}
	rec := readRecord(dir)
	rec[agentsName] = hashOf(fresh)
	return writeRecord(dir, rec)
}

// agents brings the app's copy of the guide up to this vero's. An
// AGENTS.md of the app's own, without vero's header, is left alone, and
// so is vero's copy once it has been edited, unless --force is given.
func (u *upgrader) agents() error {
	src, err := veroModule(u.dir)
	if err != nil {
		return err
	}
	fresh, err := agentsCopy(src)
	if err != nil || fresh == nil {
		return err
	}
	path := filepath.Join(u.dir, agentsName)
	rec := u.record()
	old, err := os.ReadFile(path)
	existed := err == nil
	switch {
	case existed && bytes.Equal(old, fresh):
		rec[agentsName] = hashOf(fresh)
		return u.saveRecord(rec)
	case existed && !bytes.HasPrefix(old, []byte(agentsHeader)):
		u.must("AGENTS.md is the app's own, so vero's guide for agents was not written. Move its rules into a file of the app's own, such as CLAUDE.md, delete AGENTS.md, and run vero upgrade again")
		return nil
	case existed && rec[agentsName] != "" && rec[agentsName] != hashOf(old) && !u.force:
		u.edited = append(u.edited, agentsName)
		return nil
	}
	if err := os.WriteFile(path, fresh, 0o644); err != nil {
		return err
	}
	rec[agentsName] = hashOf(fresh)
	if existed {
		u.did("AGENTS.md: updated")
	} else {
		u.did("AGENTS.md: added, the guide for coding agents")
	}
	return u.saveRecord(rec)
}
