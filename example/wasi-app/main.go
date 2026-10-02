// Command wasi-app drives a worker that is not a program.
//
//	scripts/run-wasi.sh
//
// Everywhere else in this repository the frontend starts the worker: a native
// executable, built for the machine it will run on.  Here the worker is a
// single `worker.wasm`, and a WASI runtime starts it instead.  Two things come
// of that, and they are the reasons to do it:
//
//   - One file runs anywhere.  The same worker.wasm runs on amd64, arm64 and
//     riscv64, on macOS, Linux and Windows, with no build for any of them.
//   - The worker is boxed in.  A WASI guest sees only what the runtime grants
//     it: no files here, because none are named on the command line, and no
//     way to start anything.
//
// The protocol is the one every other example uses, and the only line that
// differs from a native worker is Path.  What does differ is what a worker
// can be: WASI gives the instance one thread, and while it waits for its next
// request nothing else inside it runs.  So ./worker does its work in its
// handlers rather than in a goroutine - press a key and the job moves, leave
// it alone and nothing happens.
//
// There is no window because WASI has no screen, so this frontend is a
// terminal.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/imclaren/vero"
)

// The shape the worker sends: the same Status and Job as ../worker.
type Job struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Phase    string `json:"phase"`
	Progress int    `json:"progress"`
}

type Status struct {
	Jobs    []Job  `json:"jobs"`
	Working bool   `json:"working"`
	Since   string `json:"since"`
}

func main() {
	worker := flag.String("worker", "worker.wasm", "the worker, as a wasm file")
	runtime := flag.String("runtime", "wasmtime", "the WASI runtime to run it with")
	flag.Parse()

	screen := &screen{}

	sup := vero.Supervise(vero.SupervisorOptions{
		// The whole difference.  Elsewhere Path is the worker; here it is the
		// runtime, and the worker is an argument to it.  wasmtime passes the
		// guest nothing it is not told to, so VERO_SERVE - which is how a
		// worker knows it is supervised - is named explicitly.
		Path: *runtime,
		Args: []string{"run", "--env", "VERO_SERVE=1", *worker},

		// The lock keeps one frontend to one worker, and it is keyed on Path
		// unless told otherwise.  Path is the runtime here, and two different
		// wasm workers would collide on it.
		Lock: *worker,

		OnEvent: func(event json.RawMessage) {
			var status Status
			if json.Unmarshal(event, &status) == nil {
				screen.show(&status, "")
			}
		},
		OnStateChange: func(state vero.RunState) {
			screen.show(nil, fmt.Sprintf("the worker is %s", state))
		},
	})
	defer sup.Stop()

	if err := sup.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "vero: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("%s, running %s\n", *runtime, *worker)
	fmt.Println("1, 2 or 3 and enter moves that job on.  r1, r2, r3 restart one.  q leaves.")
	fmt.Println()

	// The worker takes a moment to come up, and the first thing it sends is
	// the state it starts with.  Nothing is drawn until it arrives.
	deadline := time.Now().Add(10 * time.Second)
	for sup.Latest() == nil && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if status := sup.Latest(); status != nil {
		var s Status
		if json.Unmarshal(status, &s) == nil {
			screen.show(&s, "")
		}
	}

	in := bufio.NewScanner(os.Stdin)
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "q" || line == "" {
			break
		}
		request := "advanceJob"
		if strings.HasPrefix(line, "r") {
			request, line = "restartJob", strings.TrimPrefix(line, "r")
		}
		id, err := strconv.Atoi(line)
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		reply, err := sup.Call(ctx, request, vero.ID[int]{ID: id})
		cancel()
		if err != nil {
			screen.show(nil, err.Error())
			continue
		}
		var s Status
		if json.Unmarshal(reply, &s) == nil {
			screen.show(&s, fmt.Sprintf("%s %d", strings.TrimSuffix(request, "Job"), id))
		}
	}
}

// screen keeps the rows in one place on the terminal rather than letting them
// scroll past, which is the whole of its job.  One block, redrawn: the jobs,
// and a line underneath for whatever last happened.
type screen struct {
	mu    sync.Mutex
	jobs  []Job
	note  string
	drawn int
}

func (s *screen) show(status *Status, note string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if status != nil {
		s.jobs = status.Jobs
	}
	if note != "" {
		s.note = note
	}

	// Back to the top of the block, so this replaces it rather than adding
	// to it.
	for i := 0; i < s.drawn; i++ {
		fmt.Print("\033[F")
	}
	for _, job := range s.jobs {
		fmt.Printf("\r\033[K  %d  %-12s %-20s %s\n",
			job.ID, job.Name, job.Phase, bar(job.Progress))
	}
	fmt.Printf("\r\033[K  %s\n", s.note)
	s.drawn = len(s.jobs) + 1
}

func bar(percent int) string {
	const width = 24
	filled := percent * width / 100
	return "[" + strings.Repeat("#", filled) + strings.Repeat(" ", width-filled) + "]"
}
