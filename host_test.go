package vero_test

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"testing"
	"time"
)

// A host is the same binary as the worker, run with VERO_HOST set, so these
// drive the test worker through one: write a request on its standard input,
// read events and replies back.  That is the whole protocol a binding needs,
// and the reason the Python and C# ones do not have to load a C library.

type hosted struct {
	t    *testing.T
	cmd  *exec.Cmd
	in   *json.Encoder
	out  *bufio.Scanner
	next uint64
}

func startHost(t *testing.T) *hosted {
	t.Helper()

	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "VERO_HOST=1", envTestWorker+"=1")
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting the host: %v", err)
	}

	h := &hosted{t: t, cmd: cmd, in: json.NewEncoder(stdin), out: bufio.NewScanner(stdout)}
	h.out.Buffer(make([]byte, 0, 64<<10), 1<<20)
	t.Cleanup(func() {
		stdin.Close()
		_ = cmd.Wait()
	})
	return h
}

// send writes a request and returns the id it was given.
func (h *hosted) send(kind, name string, payload any) uint64 {
	h.t.Helper()
	h.next++

	var raw json.RawMessage
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			h.t.Fatalf("encoding the request: %v", err)
		}
		raw = encoded
	}
	if err := h.in.Encode(map[string]any{"t": kind, "id": h.next, "n": name, "p": raw}); err != nil {
		h.t.Fatalf("writing the request: %v", err)
	}
	return h.next
}

// reply reads lines until the answer to id arrives, ignoring the events and
// state lines that arrive alongside it.
func (h *hosted) reply(id uint64) (json.RawMessage, string) {
	h.t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for h.out.Scan() {
		if time.Now().After(deadline) {
			h.t.Fatal("no reply within ten seconds")
		}
		var e struct {
			Kind    string          `json:"t"`
			ID      uint64          `json:"id"`
			Payload json.RawMessage `json:"p"`
			Error   string          `json:"e"`
		}
		if err := json.Unmarshal(h.out.Bytes(), &e); err != nil {
			continue
		}
		if e.Kind == "reply" && e.ID == id {
			return e.Payload, e.Error
		}
	}
	h.t.Fatalf("the host closed before replying: %v", h.out.Err())
	return nil, ""
}

// event reads lines until an event arrives.
func (h *hosted) event() json.RawMessage {
	h.t.Helper()
	for h.out.Scan() {
		var e struct {
			Kind    string          `json:"t"`
			Payload json.RawMessage `json:"p"`
		}
		if err := json.Unmarshal(h.out.Bytes(), &e); err != nil {
			continue
		}
		if e.Kind == "event" {
			return e.Payload
		}
	}
	h.t.Fatalf("the host closed before emitting: %v", h.out.Err())
	return nil
}

// waitRunning blocks until the host says the worker is up.  A request before
// then is refused, exactly as it is through the C shim: starting is
// asynchronous, and a frontend is expected to notice.
func (h *hosted) waitRunning() {
	h.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		payload, errText := h.reply(h.send("ctl", "state", nil))
		if errText == "" {
			var st struct {
				State string `json:"state"`
			}
			if json.Unmarshal(payload, &st) == nil && st.State == "running" {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	h.t.Fatal("the worker never came up")
}

func TestAHostAnswersARequest(t *testing.T) {
	h := startHost(t)
	h.waitRunning()

	payload, errText := h.reply(h.send("", "", request{Type: "echo", N: 7}))
	if errText != "" {
		t.Fatalf("the worker refused it: %s", errText)
	}

	var r reply
	if err := json.Unmarshal(payload, &r); err != nil {
		t.Fatalf("decoding the reply: %v", err)
	}
	if r.Message != "echo 7" {
		t.Fatalf("got %q, want %q", r.Message, "echo 7")
	}
}

func TestAHostCarriesHandlerErrors(t *testing.T) {
	h := startHost(t)
	h.waitRunning()

	_, errText := h.reply(h.send("", "", request{Type: "nonsense"}))
	if errText == "" {
		t.Fatal("an unknown request should be refused, not answered")
	}
}

func TestAHostForwardsEvents(t *testing.T) {
	h := startHost(t)

	// The test worker ticks on a timer, so an event arrives without asking.
	if payload := h.event(); len(payload) == 0 {
		t.Fatal("an event arrived with no payload")
	}
}

func TestAHostAnswersLifecycleQuestionsItself(t *testing.T) {
	h := startHost(t)
	h.waitRunning()

	payload, errText := h.reply(h.send("ctl", "state", nil))
	if errText != "" {
		t.Fatalf("asking for the state: %s", errText)
	}

	var st struct {
		State    string `json:"state"`
		Restarts int    `json:"restarts"`
	}
	if err := json.Unmarshal(payload, &st); err != nil {
		t.Fatalf("decoding the state: %v", err)
	}
	if st.State != "running" {
		t.Fatalf("state is %q, want running", st.State)
	}
	if st.Restarts != 0 {
		t.Fatalf("restarts is %d, want 0: nothing has crashed", st.Restarts)
	}
}

func TestAHostRefusesAnUnknownControlRequest(t *testing.T) {
	h := startHost(t)

	if _, errText := h.reply(h.send("ctl", "nosuch", nil)); errText == "" {
		t.Fatal("an unknown control request should be refused")
	}
}

func TestAHostKeepsTheLatestState(t *testing.T) {
	h := startHost(t)

	h.event() // wait until the worker has published something

	payload, errText := h.reply(h.send("ctl", "latest", nil))
	if errText != "" {
		t.Fatalf("asking for the latest state: %s", errText)
	}
	if len(payload) == 0 {
		t.Fatal("latest is empty after an event has arrived")
	}
}
