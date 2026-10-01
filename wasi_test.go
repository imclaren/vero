package vero_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/imclaren/vero"
)

// A worker compiled to WebAssembly and supervised through a WASI runtime, so
// that the one thing WASI changes stays honest: a read on standard input parks
// the whole instance, so a worker there answers in the goroutine that read the
// request rather than on one of its own.  Without that, every reply waits for
// the next request, and this test times out.
//
// Skipped unless wasmtime is installed, since nothing else here needs it.
func TestAWorkerCompiledToWasmAnswersUnderWASI(t *testing.T) {
	runtime, err := exec.LookPath("wasmtime")
	if err != nil {
		t.Skip("wasmtime is not installed")
	}

	worker := filepath.Join(t.TempDir(), "worker.wasm")
	build := exec.Command("go", "build", "-o", worker, "./example/worker")
	build.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the worker for wasip1: %v\n%s", err, out)
	}

	sup := vero.Supervise(vero.SupervisorOptions{
		Path: runtime,
		// wasmtime gives the guest nothing it is not told to, so the one
		// variable that matters is passed through by hand.
		Args:   []string{"run", "--env", "VERO_SERVE=1", worker},
		NoLock: true,
	})
	defer sup.Stop()

	deadline := time.Now().Add(30 * time.Second)
	for sup.State() != vero.Running {
		if time.Now().After(deadline) {
			t.Fatalf("the worker never ran: state is %s (%v)", sup.State(), sup.Err())
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Several requests, because the failure this guards against answers the
	// first one late rather than never.
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		reply, err := sup.Call(ctx, "status", struct{}{})
		cancel()
		if err != nil {
			t.Fatalf("status %d: %v", i, err)
		}
		var status struct {
			Jobs []struct {
				Name string `json:"name"`
			} `json:"jobs"`
		}
		if err := json.Unmarshal(reply, &status); err != nil {
			t.Fatalf("decoding reply %d: %v", i, err)
		}
		if len(status.Jobs) == 0 {
			t.Fatalf("reply %d has no jobs in it", i)
		}
	}
	if n := sup.Restarts(); n != 0 {
		t.Fatalf("the worker restarted %d times", n)
	}
}
