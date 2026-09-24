package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tom-molotnikoff/claude-context-hook/internal/cli"
)

const session = "11111111-2222-3333-4444-555555555555"

type harness struct {
	t        *testing.T
	stateDir string
	env      map[string]string
}

type result struct {
	code   int
	stdout string
	stderr string
}

func newHarness(t *testing.T) *harness {
	base := t.TempDir()
	return &harness{
		t:        t,
		stateDir: filepath.Join(base, "ctx"),
		env:      map[string]string{"XDG_STATE_HOME": base, "CLAUDE_CODE_SESSION_ID": session},
	}
}

func (h *harness) run(stdin string, args ...string) result {
	var stdout, stderr bytes.Buffer
	code := cli.Run(cli.Env{
		Args:   args,
		Stdin:  strings.NewReader(stdin),
		Stdout: &stdout,
		Stderr: &stderr,
		Getenv: func(k string) string { return h.env[k] },
		Now:    func() time.Time { return time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC) },
	})
	return result{code, stdout.String(), stderr.String()}
}

func (h *harness) hook(transcriptPath string) result {
	return h.hookInput(map[string]string{"session_id": session, "transcript_path": transcriptPath, "hook_event_name": "PostToolUse", "tool_name": "Bash"})
}

func (h *harness) hookInput(in map[string]string) result {
	raw, _ := json.Marshal(in)
	return h.run(string(raw), "hook")
}

func (h *harness) statePath() string {
	return filepath.Join(h.stateDir, session+".json")
}

func (h *harness) state() map[string]any {
	raw, err := os.ReadFile(h.statePath())
	if err != nil {
		h.t.Fatal(err)
	}
	var st map[string]any
	if err := json.Unmarshal(raw, &st); err != nil {
		h.t.Fatal(err)
	}
	return st
}

func (h *harness) arm(threshold, model string) {
	if r := h.run("", "arm", threshold, "--model", model); r.code != 0 {
		h.t.Fatalf("arm failed: %+v", r)
	}
}

func (h *harness) transcript(tokens ...int64) string {
	path := filepath.Join(h.t.TempDir(), "transcript.jsonl")
	writeTranscript(h.t, path, tokens...)
	return path
}

func writeTranscript(t *testing.T, path string, tokens ...int64) {
	var b strings.Builder
	for i, n := range tokens {
		fmt.Fprintf(&b, `{"type":"user","message":{"role":"user","content":"task %d"}}`+"\n", i)
		fmt.Fprintf(&b, `{"type":"assistant","message":{"id":"msg_%d","model":"claude-opus-5-5","usage":{"input_tokens":1,"cache_creation_input_tokens":%d,"cache_read_input_tokens":%d,"output_tokens":50}}}`+"\n", i, n/2, n-n/2-1)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestArmWritesState(t *testing.T) {
	h := newHarness(t)
	r := h.run("", "arm", "75", "--model", "claude-opus-5-5[1m]")
	if r.code != 0 || r.stdout != "[ctx] on: stop at 75%, early warning at 50%, window 1M.\n" {
		t.Fatalf("got %+v", r)
	}
	st := h.state()
	if st["threshold"] != 75.0 || st["window"] != 1_000_000.0 || st["start_tokens"] != nil || st["warn_sent"] != false || st["stop_sent"] != false {
		t.Errorf("state %v", st)
	}
}

func TestArmSmallWindow(t *testing.T) {
	h := newHarness(t)
	r := h.run("", "arm", "80", "--model", "claude-sonnet-5")
	if r.code != 0 || r.stdout != "[ctx] on: stop at 80%, early warning at 53%, window 200k.\n" {
		t.Fatalf("got %+v", r)
	}
	if st := h.state(); st["window"] != 200_000.0 {
		t.Errorf("window %v", st["window"])
	}
}

func TestArmRejectsBadThreshold(t *testing.T) {
	for _, threshold := range []string{"9", "91", "75.5", "abc", "-50"} {
		t.Run(threshold, func(t *testing.T) {
			h := newHarness(t)
			r := h.run("", "arm", threshold, "--model", "claude-sonnet-5")
			if r.code == 0 || !strings.Contains(r.stderr, "10 to 90") {
				t.Errorf("got %+v", r)
			}
			assertNoState(t, h)
		})
	}
}

func TestArmRequiresModel(t *testing.T) {
	h := newHarness(t)
	r := h.run("", "arm", "75")
	if r.code == 0 || !strings.Contains(r.stderr, "usage: ctx arm <threshold> --model <model-id>") {
		t.Errorf("got %+v", r)
	}
	assertNoState(t, h)
}

func TestArmRequiresSessionID(t *testing.T) {
	h := newHarness(t)
	delete(h.env, "CLAUDE_CODE_SESSION_ID")
	r := h.run("", "arm", "75", "--model", "claude-sonnet-5")
	if r.code == 0 || !strings.Contains(r.stderr, "CLAUDE_CODE_SESSION_ID") {
		t.Errorf("got %+v", r)
	}
	if _, err := os.Stat(h.stateDir); !os.IsNotExist(err) {
		t.Errorf("state dir exists: %v", err)
	}
}

func TestStatePermissions(t *testing.T) {
	h := newHarness(t)
	h.arm("75", "claude-sonnet-5")
	h.hook(h.transcript(30_000))
	assertMode(t, h.stateDir, 0o700)
	assertMode(t, h.statePath(), 0o600)
}

func TestHookSilentWhenNotArmed(t *testing.T) {
	h := newHarness(t)
	r := h.hook(h.transcript(30_000))
	if r.code != 0 || r.stdout != "" || r.stderr != "" {
		t.Errorf("got %+v", r)
	}
	if _, err := os.Stat(h.stateDir); !os.IsNotExist(err) {
		t.Errorf("state dir exists: %v", err)
	}
}

func TestHookIgnoresSubagents(t *testing.T) {
	h := newHarness(t)
	h.arm("75", "claude-sonnet-5")
	before, _ := os.ReadFile(h.statePath())
	r := h.hookInput(map[string]string{"session_id": session, "transcript_path": h.transcript(30_000), "agent_id": "agent-1"})
	after, _ := os.ReadFile(h.statePath())
	if r.code != 0 || r.stdout != "" || !bytes.Equal(before, after) {
		t.Errorf("got %+v, state %s -> %s", r, before, after)
	}
}

func TestHookRecordsUsage(t *testing.T) {
	h := newHarness(t)
	h.arm("75", "claude-sonnet-5")
	path := h.transcript(10_000, 30_000)
	r := h.hook(path)
	if r.code != 0 || r.stdout != "" {
		t.Fatalf("got %+v", r)
	}
	st := h.state()
	if st["start_tokens"] != 30_000.0 || st["last_tokens"] != 30_000.0 || st["transcript_path"] != path {
		t.Errorf("state %v", st)
	}
}

func TestHookKeepsStartingUsage(t *testing.T) {
	h := newHarness(t)
	h.arm("75", "claude-sonnet-5")
	path := h.transcript(20_000)
	h.hook(path)
	writeTranscript(t, path, 20_000, 60_000)
	h.hook(path)
	st := h.state()
	if st["start_tokens"] != 20_000.0 || st["last_tokens"] != 60_000.0 {
		t.Errorf("state %v", st)
	}
}

func TestStatus(t *testing.T) {
	h := newHarness(t)
	h.arm("75", "claude-opus-5-5[1m]")
	path := h.transcript(40_000)
	h.hook(path)
	writeTranscript(t, path, 40_000, 129_999)
	before, _ := os.ReadFile(h.statePath())
	r := h.run("")
	want := "[ctx] 12% used (129999 of 1000000 tokens), started at 4%, early warning at 50%, stop at 75%.\n"
	if r.code != 0 || r.stdout != want {
		t.Errorf("got %+v, want %q", r, want)
	}
	after, _ := os.ReadFile(h.statePath())
	if !bytes.Equal(before, after) {
		t.Errorf("state changed: %s -> %s", before, after)
	}
}

func TestStatusNotArmed(t *testing.T) {
	h := newHarness(t)
	r := h.run("")
	if r.code != 1 || r.stdout != "[ctx] not on for this session.\n" {
		t.Errorf("got %+v", r)
	}
}

func TestStatusNoFigure(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(h *harness)
		reason string
	}{
		{"no transcript path", func(h *harness) {}, "no transcript path recorded yet"},
		{"transcript missing", func(h *harness) {
			path := h.transcript(30_000)
			h.hook(path)
			os.Remove(path)
		}, "transcript missing"},
		{"transcript unreadable", func(h *harness) {
			path := h.transcript(30_000)
			h.hook(path)
			os.Remove(path)
			os.Mkdir(path, 0o700)
		}, "transcript unreadable"},
		{"no usage", func(h *harness) {
			path := h.transcript(30_000)
			h.hook(path)
			os.WriteFile(path, []byte(`{"type":"user","message":{"content":"hi"}}`+"\n"), 0o600)
		}, "no assistant usage in transcript"},
		{"corrupt state", func(h *harness) {
			os.WriteFile(h.statePath(), []byte("{not json"), 0o600)
		}, "state file corrupt"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			h.arm("75", "claude-sonnet-5")
			c.setup(h)
			r := h.run("")
			if r.code != 1 || !strings.HasPrefix(r.stdout, "[ctx] no figure: "+c.reason) {
				t.Errorf("got %+v", r)
			}
		})
	}
}

func assertNoState(t *testing.T, h *harness) {
	t.Helper()
	if _, err := os.Stat(h.statePath()); !os.IsNotExist(err) {
		t.Errorf("state file exists: %v", err)
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s mode %o, want %o", path, got, want)
	}
}

func TestHookEmitsMessageAsAdditionalContext(t *testing.T) {
	h := newHarness(t)
	h.arm("75", "claude-sonnet-5")
	path := h.transcript(8_000)
	h.hook(path)
	writeTranscript(t, path, 8_000, 150_000)
	r := h.hook(path)
	want := `{"hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"[ctx] 75% of context used (stop at 75%). Finish the current task. Start no new tasks."}}` + "\n"
	if r.code != 0 || r.stdout != want {
		t.Errorf("got %+v, want %q", r, want)
	}
}

func TestHookSilentBelowEarlyWarning(t *testing.T) {
	h := newHarness(t)
	h.arm("75", "claude-sonnet-5")
	path := h.transcript(8_000)
	var out strings.Builder
	for tokens := int64(8_000); tokens < 100_000; tokens += 7_000 {
		writeTranscript(t, path, tokens)
		r := h.hook(path)
		out.WriteString(r.stdout)
		out.WriteString(r.stderr)
	}
	if out.Len() != 0 {
		t.Errorf("got %d bytes of output: %q", out.Len(), out.String())
	}
}

func TestSessionsAreSeparate(t *testing.T) {
	h := newHarness(t)
	h.arm("75", "claude-sonnet-5")
	h.env["CLAUDE_CODE_SESSION_ID"] = "other"
	h.arm("75", "claude-sonnet-5")
	ours, theirs := h.transcript(8_000), h.transcript(8_000)
	h.hook(ours)
	h.hookInput(map[string]string{"session_id": "other", "transcript_path": theirs})
	writeTranscript(t, ours, 8_000, 160_000)
	if r := h.hook(ours); !strings.Contains(r.stdout, "Start no new tasks.") {
		t.Fatalf("got %+v", r)
	}
	writeTranscript(t, theirs, 8_000, 160_000)
	r := h.hookInput(map[string]string{"session_id": "other", "transcript_path": theirs})
	if !strings.Contains(r.stdout, "Start no new tasks.") {
		t.Errorf("other session got %+v", r)
	}
}

func TestConcurrentHooksSendOneStop(t *testing.T) {
	h := newHarness(t)
	h.arm("75", "claude-sonnet-5")
	path := h.transcript(8_000)
	h.hook(path)
	writeTranscript(t, path, 8_000, 170_000)
	input, _ := json.Marshal(map[string]string{"session_id": session, "transcript_path": path})

	const processes = 10
	cmds := make([]*exec.Cmd, processes)
	outs := make([]*bytes.Buffer, processes)
	for i := range cmds {
		cmds[i] = exec.Command(os.Args[0], "hook")
		cmds[i].Env = append(os.Environ(), "CTX_TEST_AS_CTX=1", "XDG_STATE_HOME="+h.env["XDG_STATE_HOME"])
		cmds[i].Stdin = bytes.NewReader(input)
		outs[i] = &bytes.Buffer{}
		cmds[i].Stdout = outs[i]
	}
	for _, c := range cmds {
		if err := c.Start(); err != nil {
			t.Fatal(err)
		}
	}
	stops := 0
	for i, c := range cmds {
		if err := c.Wait(); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(outs[i].String(), "Start no new tasks.") {
			stops++
		}
	}
	if stops != 1 {
		t.Errorf("%d stop messages, want 1", stops)
	}
}

func TestMain(m *testing.M) {
	if os.Getenv("CTX_TEST_AS_CTX") == "1" {
		os.Exit(cli.Run(cli.Env{
			Args:   os.Args[1:],
			Stdin:  os.Stdin,
			Stdout: os.Stdout,
			Stderr: os.Stderr,
			Getenv: os.Getenv,
			Now:    time.Now,
		}))
	}
	os.Exit(m.Run())
}

func TestHookLogsFailures(t *testing.T) {
	const secret = "TRANSCRIPT-CONTENT-MUST-NOT-LEAK"
	cases := []struct {
		name  string
		setup func(h *harness, path string)
		want  string
	}{
		{"transcript missing", func(h *harness, path string) { os.Remove(path) }, "no such file or directory"},
		{"transcript unreadable", func(h *harness, path string) { os.Remove(path); os.Mkdir(path, 0o700) }, "is a directory"},
		{"state file corrupt", func(h *harness, path string) {
			os.WriteFile(h.statePath(), []byte(`{"threshold":`+secret), 0o600)
		}, "state file corrupt"},
		{"no assistant usage", func(h *harness, path string) {
			os.WriteFile(path, []byte(`{"type":"user","message":{"content":"`+secret+`"}}`+"\n"), 0o600)
		}, "no assistant usage in transcript"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			h.arm("75", "claude-sonnet-5")
			path := h.transcript(8_000)
			h.hook(path)
			before, _ := os.ReadFile(h.statePath())
			c.setup(h, path)
			r := h.hook(path)
			if r.code != 0 || r.stdout != "" {
				t.Errorf("got %+v", r)
			}
			log, err := os.ReadFile(filepath.Join(h.stateDir, "error.log"))
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.TrimSuffix(string(log), "\n"), "\n")
			if len(lines) != 1 || !strings.HasPrefix(lines[0], "2026-09-24T12:00:00Z "+session+" ") || !strings.Contains(lines[0], c.want) {
				t.Errorf("error.log %q", log)
			}
			if strings.Contains(string(log), secret) {
				t.Errorf("error.log holds transcript content: %q", log)
			}
			assertMode(t, filepath.Join(h.stateDir, "error.log"), 0o600)
			if after, _ := os.ReadFile(h.statePath()); !bytes.Equal(before, after) && c.name != "state file corrupt" {
				t.Errorf("state changed: %s -> %s", before, after)
			}
		})
	}
}

func TestHookAppendsOneLinePerFailure(t *testing.T) {
	h := newHarness(t)
	h.arm("75", "claude-sonnet-5")
	missing := filepath.Join(t.TempDir(), "gone.jsonl")
	h.hook(missing)
	h.hook(missing)
	log, _ := os.ReadFile(filepath.Join(h.stateDir, "error.log"))
	if n := strings.Count(string(log), "\n"); n != 2 {
		t.Errorf("%d lines in error.log: %q", n, log)
	}
}

func TestUnarmedHookLogsNothing(t *testing.T) {
	h := newHarness(t)
	h.arm("75", "claude-sonnet-5")
	r := h.hookInput(map[string]string{"session_id": "not-armed", "transcript_path": filepath.Join(t.TempDir(), "gone.jsonl")})
	if r.code != 0 || r.stdout != "" {
		t.Errorf("got %+v", r)
	}
	if _, err := os.Stat(filepath.Join(h.stateDir, "error.log")); !os.IsNotExist(err) {
		t.Errorf("error.log exists: %v", err)
	}
}
