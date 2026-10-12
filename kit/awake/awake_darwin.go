package awake

import (
	"os"
	"os/exec"
	"strconv"
)

// macOS: caffeinate, which Apple ships, holds an assertion that stops
// idle sleep until it is stopped, or until this process ends.
func hold(string) (func(), error) {
	cmd := exec.Command("/usr/bin/caffeinate", "-i", "-w", strconv.Itoa(os.Getpid()))
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return func() {
		cmd.Process.Kill()
		cmd.Wait()
	}, nil
}
