package vero_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/imclaren/vero"
)

// The tests run this same binary as the worker, so there is nothing to build
// and the worker really is a separate process.
const envTestWorker = "VERO_TEST_WORKER"

func TestMain(m *testing.M) {
	if os.Getenv(envTestWorker) != "" {
		testWorker()
		return
	}
	if os.Getenv(envRouterWorker) != "" {
		routerWorker()
		return
	}
	os.Exit(m.Run())
}

type request struct {
	Type string `json:"type"`
	N    int    `json:"n"`
}

type reply struct {
	Message string `json:"message"`
}

func testWorker() {
	w := vero.NewWorker(vero.WorkerOptions{})

	var frozen atomic.Bool
	var counter int64
	go w.EmitOnChange(context.Background(), 20*time.Millisecond, func() any {
		if frozen.Load() {
			return map[string]int64{"tick": atomic.LoadInt64(&counter)}
		}
		return map[string]int64{"tick": atomic.AddInt64(&counter, 1)}
	})

	// No named handlers: everything arrives at the fallback, which is what an
	// frontend that predates named requests sends.
	w.Fallback(func(ctx context.Context, req json.RawMessage) (any, error) {
		var r request
		if err := json.Unmarshal(req, &r); err != nil {
			return nil, err
		}
		switch r.Type {
		case "echo":
			return reply{Message: fmt.Sprintf("echo %d", r.N)}, nil
		case "slow":
			time.Sleep(300 * time.Millisecond)
			return reply{Message: "slow done"}, nil
		case "freeze":
			frozen.Store(true)
			return reply{Message: "frozen"}, nil
		case "forever":
			// A long poll with nothing to report. It must notice the context,
			// or the worker cannot exit when the frontend does.
			<-ctx.Done()
			return nil, ctx.Err()
		case "refuse":
			return nil, errors.New("i will not")
		case "quiet":
			return nil, nil
		case "panic":
			panic("handler exploded")
		case "noisy":
			// Exactly the mistake the design is vulnerable to: a print
			// straight to stdout, where the protocol lives.
			fmt.Println("a stray Println that would corrupt a frame")
			print("and a builtin print\n")
			os.Stdout.WriteString("and a direct write to os.Stdout\n")
			return reply{Message: "survived"}, nil
		case "die":
			// An unrecoverable failure, of the kind recover cannot catch.
			go func() { var p *int; _ = *p }()
			time.Sleep(time.Second)
			return nil, nil
		default:
			return nil, fmt.Errorf("unknown request type: %q", r.Type)
		}
	})
	w.Serve()
	os.Exit(0)
}

func newSupervisor(t *testing.T, opts *vero.SupervisorOptions) *vero.Supervisor {
	t.Helper()
	o := vero.SupervisorOptions{}
	if opts != nil {
		o = *opts
	}
	o.Path = os.Args[0]
	o.Env = append(os.Environ(), envTestWorker+"=1")
	s := vero.Supervise(o)
	t.Cleanup(func() { s.Stop() })

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s.State() == vero.Running {
			return s
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the worker never came up")
	return nil
}

func TestRequestAndReply(t *testing.T) {
	s := newSupervisor(t, nil)
	raw, err := s.Request(context.Background(), request{Type: "echo", N: 7})
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	var got reply
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Message != "echo 7" {
		t.Fatalf("got %q, want %q", got.Message, "echo 7")
	}
}

func TestHandlerErrorReachesTheCaller(t *testing.T) {
	s := newSupervisor(t, nil)
	_, err := s.Request(context.Background(), request{Type: "refuse"})
	var remote *vero.RemoteError
	if !errors.As(err, &remote) {
		t.Fatalf("got %v, want a *RemoteError", err)
	}
	if remote.Message != "i will not" {
		t.Fatalf("got %q, want %q", remote.Message, "i will not")
	}
}

func TestUnknownRequestIsRefusedNotIgnored(t *testing.T) {
	s := newSupervisor(t, nil)
	_, err := s.Request(context.Background(), request{Type: "nonsense"})
	if err == nil || !strings.Contains(err.Error(), `unknown request type: "nonsense"`) {
		t.Fatalf("got %v, want the worker's own message", err)
	}
}

func TestAHandlerMayAnswerNothing(t *testing.T) {
	s := newSupervisor(t, nil)
	raw, err := s.Request(context.Background(), request{Type: "quiet"})
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if len(raw) != 0 {
		t.Fatalf("got %q, want nothing", raw)
	}
}

func TestAPanickingHandlerCostsOneRequest(t *testing.T) {
	s := newSupervisor(t, nil)
	if _, err := s.Request(context.Background(), request{Type: "panic"}); err == nil {
		t.Fatal("want an error from a panicking handler")
	}
	// The worker must still be the same process, still answering.
	if n := s.Restarts(); n != 0 {
		t.Fatalf("the worker restarted %d times; a handler panic should not kill it", n)
	}
	if _, err := s.Request(context.Background(), request{Type: "echo", N: 1}); err != nil {
		t.Fatalf("the worker stopped answering after a handler panic: %v", err)
	}
}

func TestEventsArriveWithoutBeingAsked(t *testing.T) {
	var mu sync.Mutex
	seen := 0
	s := newSupervisor(t, &vero.SupervisorOptions{
		OnEvent: func(json.RawMessage) { mu.Lock(); seen++; mu.Unlock() },
	})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := seen
		mu.Unlock()
		if n >= 3 {
			if s.Latest() == nil {
				t.Fatal("events arrived but Latest is empty")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no events arrived")
}

func TestASlowRequestDoesNotBlockTheOnesBehindIt(t *testing.T) {
	s := newSupervisor(t, nil)
	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.Request(context.Background(), request{Type: "slow"})
	}()
	time.Sleep(50 * time.Millisecond)

	if _, err := s.Request(context.Background(), request{Type: "echo", N: 2}); err != nil {
		t.Fatalf("Request: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Fatalf("the quick request waited %v for the slow one", elapsed)
	}
	wg.Wait()
}

func TestManyConcurrentRequestsEachGetTheirOwnReply(t *testing.T) {
	s := newSupervisor(t, nil)
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			raw, err := s.Request(context.Background(), request{Type: "echo", N: n})
			if err != nil {
				t.Errorf("Request %d: %v", n, err)
				return
			}
			var got reply
			json.Unmarshal(raw, &got)
			if want := fmt.Sprintf("echo %d", n); got.Message != want {
				t.Errorf("got %q, want %q - replies are crossed", got.Message, want)
			}
		}(i)
	}
	wg.Wait()
}

func TestAStrayPrintlnCannotCorruptTheStream(t *testing.T) {
	var mu sync.Mutex
	var logs []string
	s := newSupervisor(t, &vero.SupervisorOptions{
		OnLog: func(line string) { mu.Lock(); logs = append(logs, line); mu.Unlock() },
	})

	// The reply must arrive intact despite the handler printing to stdout.
	raw, err := s.Request(context.Background(), request{Type: "noisy"})
	if err != nil {
		t.Fatalf("a stray print broke the request: %v", err)
	}
	var got reply
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("a stray print corrupted the reply frame: %v", err)
	}
	if got.Message != "survived" {
		t.Fatalf("got %q, want %q", got.Message, "survived")
	}

	// And the request after it must still work: a corrupted stream would
	// desynchronise everything behind it, not just the noisy one.
	if _, err := s.Request(context.Background(), request{Type: "echo", N: 5}); err != nil {
		t.Fatalf("the stream was left desynchronised: %v", err)
	}

	// The text is not lost - it is on stderr, where logs belong.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		joined := strings.Join(logs, "\n")
		mu.Unlock()
		if strings.Contains(joined, "a stray Println") &&
			strings.Contains(joined, "a builtin print") &&
			strings.Contains(joined, "a direct write to os.Stdout") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	t.Fatalf("the printed text did not reach stderr; got: %q", strings.Join(logs, "\n"))
}

func TestAWorkerCrashDoesNotTakeUsWithIt(t *testing.T) {
	s := newSupervisor(t, &vero.SupervisorOptions{Backoff: 50 * time.Millisecond})
	s.Request(context.Background(), request{Type: "die"})

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if s.Restarts() > 0 && s.State() == vero.Running {
			if _, err := s.Request(context.Background(), request{Type: "echo", N: 3}); err == nil {
				return // a new worker is up and answering
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("the worker never came back (restarts=%d state=%v)", s.Restarts(), s.State())
}

// Workers exit on purpose: to pick up a changed configuration, or to run a
// version they have just downloaded.  Counting those with the crashes means
// the backoff climbs to MaxBackoff over a day of ordinary use, and a frontend
// watching Restarts eventually decides the worker is broken while it is doing
// what it was asked to.
//
// Two deliberate exits, each after a healthy run, must therefore leave the
// count at one rather than two.
func TestAWorkerThatRanIsNotAWorkerThatFailed(t *testing.T) {
	s := newSupervisor(t, &vero.SupervisorOptions{
		Backoff: 20 * time.Millisecond,
		Healthy: 100 * time.Millisecond,
	})

	serveThenExit := func(round int) {
		if err := waitForWorker(s); err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		time.Sleep(150 * time.Millisecond) // longer than Healthy
		s.Request(context.Background(), request{Type: "die"})
	}

	serveThenExit(1)
	serveThenExit(2)

	if err := waitForWorker(s); err != nil {
		t.Fatal(err)
	}
	if got := s.Restarts(); got != 1 {
		t.Fatalf("Restarts is %d, want 1: each exit followed a run that lasted", got)
	}
}

// waitForWorker returns once a worker is up and answering.
func waitForWorker(s *vero.Supervisor) error {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if s.State() == vero.Running {
			if _, err := s.Request(context.Background(), request{Type: "echo", N: 1}); err == nil {
				return nil
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("no worker answered (restarts=%d state=%v)", s.Restarts(), s.State())
}

func TestRequestBeforeTheWorkerIsUp(t *testing.T) {
	s := vero.Supervise(vero.SupervisorOptions{Path: "/nonexistent/worker"})
	t.Cleanup(func() { s.Stop() })
	if _, err := s.Request(context.Background(), request{Type: "echo"}); !errors.Is(err, vero.ErrWorkerNotRunning) {
		t.Fatalf("got %v, want ErrWorkerNotRunning", err)
	}
}

// The frontend going away has to end the worker even mid-request. Otherwise
// quitting while a long poll is outstanding - which is almost always, and
// certainly whenever nothing is changing - leaves it running.
// Emitting on a timer rather than on change floods the frontend and loses the
// quiet-when-idle property that is the whole reason events are pushed.
func TestEmitOnChangeIsQuietWhileNothingChanges(t *testing.T) {
	var mu sync.Mutex
	var events int
	s := newSupervisor(t, &vero.SupervisorOptions{
		OnEvent: func(json.RawMessage) { mu.Lock(); events++; mu.Unlock() },
	})

	time.Sleep(600 * time.Millisecond)
	mu.Lock()
	moving := events
	mu.Unlock()
	if moving < 5 {
		t.Fatalf("only %d events while the value was changing", moving)
	}

	// Stop the value changing. The snapshot still runs 50 times a second.
	if _, err := s.Request(context.Background(), request{Type: "freeze"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond) // let any in-flight emit land
	mu.Lock()
	settled := events
	mu.Unlock()

	time.Sleep(1 * time.Second) // 50 ticks with nothing to say
	mu.Lock()
	after := events
	mu.Unlock()

	t.Logf("%d events while changing, %d in the second after it settled", moving, after-settled)
	if after-settled > 1 {
		t.Fatalf("%d events in a second with nothing changing; it is emitting on the timer", after-settled)
	}
}

func TestAWorkerWithABlockedHandlerStillExits(t *testing.T) {
	s := newSupervisor(t, nil)

	go func() { s.Request(context.Background(), request{Type: "forever"}) }()
	time.Sleep(300 * time.Millisecond)

	start := time.Now()
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if took := time.Since(start); took > 4*time.Second {
		t.Fatalf("took %v to stop with a handler still running", took)
	}
	if st := s.State(); st != vero.Stopped {
		t.Fatalf("state is %v, want stopped", st)
	}
}

func TestStopEndsTheWorker(t *testing.T) {
	s := newSupervisor(t, nil)
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if st := s.State(); st != vero.Stopped {
		t.Fatalf("state is %v after Stop, want stopped", st)
	}
	if _, err := s.Request(context.Background(), request{Type: "echo"}); err == nil {
		t.Fatal("a request after Stop should fail")
	}
}
