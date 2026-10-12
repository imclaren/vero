package vero_test

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/imclaren/vero"
)

// envLockWorker runs this test binary as a worker that takes the
// single-worker lock under the name it gives.
const envLockWorker = "VERO_TEST_LOCK_WORKER"

func lockWorker(key string) {
	w := vero.NewWorker(vero.WorkerOptions{Lock: key})
	vero.Handle(w, "ping", func(context.Context, struct{}) (string, error) { return "pong", nil })
	os.Stderr.WriteString("holding the lock\n")
	// Work of its own, as a daemon has, so that one on its own runs on.
	go func() {
		for {
			time.Sleep(time.Hour)
		}
	}()
	w.Serve()
}

// TestWorkerLock checks WorkerOptions.Lock: while one worker holds it, a
// second on its own says so and exits, and one started by an app answers
// with already_running and runs nothing of the program's own.
func TestWorkerLock(t *testing.T) {
	key := "dev.vero.test.lock." + time.Now().Format("150405.000000")
	first := exec.Command(os.Args[0])
	first.Env = append(os.Environ(), envLockWorker+"="+key)
	stderr, _ := first.StderrPipe()
	if err := first.Start(); err != nil {
		t.Fatal(err)
	}
	defer first.Process.Kill()
	if line, _ := bufio.NewReader(stderr).ReadString('\n'); !strings.Contains(line, "holding the lock") {
		t.Fatalf("the first worker said %q", line)
	}

	second := exec.Command(os.Args[0])
	second.Env = append(os.Environ(), envLockWorker+"="+key)
	out, err := second.CombinedOutput()
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 3 || !strings.Contains(string(out), "another copy of this worker is running") {
		t.Errorf("the second worker on its own: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "holding the lock") {
		t.Error("the second worker ran the program's own code")
	}

	third := exec.Command(os.Args[0])
	third.Env = append(os.Environ(), envLockWorker+"="+key, "VERO_SERVE=1")
	third.Stdin = strings.NewReader(`{"id":1,"n":"ping"}` + "\n")
	out, err = third.Output()
	if err != nil || !strings.Contains(string(out), `"c":"already_running"`) || strings.Contains(string(out), "pong") {
		t.Errorf("the worker an app started: %v\n%s", err, out)
	}

	// Once the first has gone, the lock is free.
	first.Process.Kill()
	first.Wait()
	fourth := exec.Command(os.Args[0])
	fourth.Env = append(os.Environ(), envLockWorker+"="+key, "VERO_SERVE=1")
	fourth.Stdin = strings.NewReader(`{"id":1,"n":"ping"}` + "\n")
	if out, err := fourth.Output(); err != nil || !strings.Contains(string(out), "pong") {
		t.Errorf("after the first went: %v\n%s", err, out)
	}
}
