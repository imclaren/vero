package main

import (
	"fmt"
	"os"
	"os/exec"
)

// veroRepo runs vero-repo with the arguments given: one that matches this
// tool, built from VERO_DIR's checkout when that is set, or installed at
// this tool's version. It returns the exit status.
func veroRepo(args []string) int {
	path, err := veroRepoPath(os.Getenv("VERO_DIR"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	cmd := exec.Command(path, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode()
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
