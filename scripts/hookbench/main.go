package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const session = "00000000-0000-4000-8000-000000000000"

func main() {
	runs := flag.Int("runs", 200, "hook runs per case")
	size := flag.Int64("transcript-mb", 100, "size of the generated transcript in MB")
	flag.Parse()
	if err := run(*runs, *size<<20); err != nil {
		fmt.Fprintln(os.Stderr, "hookbench:", err)
		os.Exit(1)
	}
}

func run(runs int, size int64) error {
	dir, err := os.MkdirTemp("", "hookbench-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	bin := filepath.Join(dir, "ctx")
	build := exec.Command("go", "build", "-o", bin, "./cmd/ctx")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("build: %w", err)
	}

	transcript := filepath.Join(dir, "transcript.jsonl")
	if err := writeTranscript(transcript, size); err != nil {
		return err
	}
	payload, err := hookPayload(transcript)
	if err != nil {
		return err
	}
	env := append(os.Environ(), "XDG_STATE_HOME="+filepath.Join(dir, "state"), "CLAUDE_CODE_SESSION_ID="+session)

	ok := report(fmt.Sprintf("not on (%d runs)", runs), time.Duration(10*time.Millisecond), measure(bin, env, payload, runs))

	arm := exec.Command(bin, "arm", "75", "--model", "claude-opus-5-5[1m]")
	arm.Env = env
	if out, err := arm.CombinedOutput(); err != nil {
		return fmt.Errorf("arm: %v: %s", err, out)
	}
	ok = report(fmt.Sprintf("armed, %d MB transcript (%d runs)", size>>20, runs), 50*time.Millisecond, measure(bin, env, payload, runs)) && ok

	if !ok {
		return fmt.Errorf("over budget")
	}
	return nil
}

func measure(bin string, env []string, payload []byte, runs int) []time.Duration {
	times := make([]time.Duration, 0, runs)
	for i := -5; i < runs; i++ {
		cmd := exec.Command(bin, "hook")
		cmd.Env = env
		cmd.Stdin = bytes.NewReader(payload)
		var out bytes.Buffer
		cmd.Stdout = &out
		start := time.Now()
		err := cmd.Run()
		elapsed := time.Since(start)
		if err != nil || out.Len() != 0 {
			fmt.Fprintf(os.Stderr, "hookbench: run %d: err %v, output %q\n", i, err, out.String())
			os.Exit(1)
		}
		if i >= 0 {
			times = append(times, elapsed)
		}
	}
	slices.Sort(times)
	return times
}

func report(name string, budget time.Duration, times []time.Duration) bool {
	p95 := times[(len(times)*95+99)/100-1]
	verdict := "within budget"
	if p95 > budget {
		verdict = "OVER BUDGET"
	}
	fmt.Printf("%-40s p50 %6.2fms  p95 %6.2fms  max %6.2fms  budget %4.0fms  %s\n",
		name, ms(times[len(times)/2]), ms(p95), ms(times[len(times)-1]), ms(budget), verdict)
	return p95 <= budget
}

func ms(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

func hookPayload(transcript string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"session_id":      session,
		"transcript_path": transcript,
		"cwd":             "/home/user/project",
		"permission_mode": "default",
		"hook_event_name": "PostToolUse",
		"tool_name":       "Bash",
		"tool_input":      map[string]any{"command": "go test ./...", "description": "Run the tests"},
		"tool_response":   map[string]any{"stdout": strings.Repeat("ok  \tgithub.com/example/project/internal/pkg\t0.012s\n", 80), "stderr": "", "interrupted": false},
		"tool_use_id":     "toolu_01ABCDEFGHIJKLMNOPQRSTUV",
	})
}

func writeTranscript(path string, size int64) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := bufio.NewWriterSize(f, 1<<20)
	var written int64
	for turn := 0; written < size; turn++ {
		for _, line := range turnLines(turn) {
			n, err := w.Write(line)
			if err != nil {
				f.Close()
				return err
			}
			written += int64(n)
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func turnLines(turn int) [][]byte {
	id := fmt.Sprintf("msg_%024d", turn)
	usage := map[string]any{
		"input_tokens":                2,
		"cache_creation_input_tokens": 1_000 + turn%5_000,
		"cache_read_input_tokens":     100_000 + turn%200_000,
		"output_tokens":               300,
	}
	assistant := func(content map[string]any) map[string]any {
		return map[string]any{
			"type":      "assistant",
			"sessionId": session,
			"uuid":      fmt.Sprintf("a-%d-%s", turn, content["type"]),
			"message": map[string]any{
				"id": id, "type": "message", "role": "assistant", "model": "claude-opus-5-5",
				"content": []any{content}, "usage": usage,
			},
		}
	}
	lines := []map[string]any{
		assistant(map[string]any{"type": "thinking", "thinking": strings.Repeat("Considering the next step. ", 20)}),
		assistant(map[string]any{"type": "text", "text": "Running the tests."}),
		assistant(map[string]any{"type": "tool_use", "id": "toolu_" + id, "name": "Bash", "input": map[string]any{"command": "go test ./..."}}),
		{
			"type":      "user",
			"sessionId": session,
			"uuid":      fmt.Sprintf("u-%d", turn),
			"message": map[string]any{"role": "user", "content": []any{map[string]any{
				"type": "tool_result", "tool_use_id": "toolu_" + id,
				"content": strings.Repeat("output line from a tool call\n", 50+turn%600),
			}}},
		},
		{"type": "system", "subtype": "turn_duration", "durationMs": 1200 + turn%900, "sessionId": session},
	}
	out := make([][]byte, len(lines))
	for i, l := range lines {
		raw, _ := json.Marshal(l)
		out[i] = append(raw, '\n')
	}
	return out
}
