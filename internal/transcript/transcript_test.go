package transcript_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tom-molotnikoff/claude-context-hook/internal/transcript"
)

func TestUsageFromFixtures(t *testing.T) {
	cases := []struct {
		fixture string
		want    int64
	}{
		{"trailing-non-assistant.jsonl", 2 + 1200 + 15000},
		{"synthetic.jsonl", 4 + 2000 + 30000},
		{"repeated-id.jsonl", 2 + 6648 + 45214},
		{"single-id.jsonl", 2 + 6648 + 45214},
	}
	for _, c := range cases {
		t.Run(c.fixture, func(t *testing.T) {
			got, err := transcript.ReadFile(filepath.Join("testdata", c.fixture))
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got %d, want %d", got, c.want)
			}
		})
	}
}

func TestRepeatedMessageIDMatchesSingleLine(t *testing.T) {
	repeated, err := transcript.ReadFile("testdata/repeated-id.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	single, err := transcript.ReadFile("testdata/single-id.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if repeated != single {
		t.Errorf("repeated %d, single %d", repeated, single)
	}
}

func TestNoUsage(t *testing.T) {
	_, err := transcript.ReadFile("testdata/no-usage.jsonl")
	if !errors.Is(err, transcript.ErrNoUsage) {
		t.Errorf("got %v, want ErrNoUsage", err)
	}
}

func TestMissingFile(t *testing.T) {
	_, err := transcript.ReadFile(filepath.Join(t.TempDir(), "gone.jsonl"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("got %v, want not exist", err)
	}
}

func TestPartialLastLineIsPassedOver(t *testing.T) {
	data := assistantLine("msg_1", 7, strings.Repeat("a", 10)) + `{"type":"assistant","message":{"id":"msg_2","model":"m","usage":{"input_`
	got, err := transcript.Read(strings.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if got != 7 {
		t.Errorf("got %d, want 7", got)
	}
}

func TestReadsOnlyFromTheEnd(t *testing.T) {
	cases := []struct {
		name      string
		lineSize  int
		afterSize int
	}{
		{"small line, nothing after", 100, 0},
		{"small line, much after", 100, 300_000},
		{"line larger than a chunk", 500_000, 10_000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var b strings.Builder
			for i := 0; b.Len() < 2_000_000; i++ {
				b.WriteString(assistantLine(fmt.Sprintf("old_%d", i), 1, strings.Repeat("x", 1000)))
			}
			target := assistantLine("target", 424242, strings.Repeat("y", c.lineSize))
			b.WriteString(target)
			for after := 0; after < c.afterSize; {
				l := toolResultLine(strings.Repeat("z", 5000))
				b.WriteString(l)
				after += len(l)
			}
			data := []byte(b.String())
			fromEnd := int64(len(data) - strings.LastIndex(b.String(), target))

			r := &countingReader{r: bytes.NewReader(data), lowest: int64(len(data))}
			got, err := transcript.Read(r, int64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			if got != 424242 {
				t.Errorf("got %d, want 424242", got)
			}
			if r.read > fromEnd+transcript.ChunkSize {
				t.Errorf("read %d bytes, line is %d from the end", r.read, fromEnd)
			}
			if r.lowest == 0 {
				t.Error("read from the start of the file")
			}
		})
	}
}

type countingReader struct {
	r      *bytes.Reader
	read   int64
	lowest int64
}

func (c *countingReader) ReadAt(p []byte, off int64) (int, error) {
	n, err := c.r.ReadAt(p, off)
	c.read += int64(n)
	c.lowest = min(c.lowest, off)
	return n, err
}

func assistantLine(id string, cacheRead int64, text string) string {
	return fmt.Sprintf(`{"type":"assistant","message":{"id":%q,"model":"claude-opus-5-5","content":[{"type":"text","text":%q}],"usage":{"input_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":%d}}}`+"\n", id, text, cacheRead)
}

func toolResultLine(content string) string {
	return fmt.Sprintf(`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":%q}]}}`+"\n", content)
}
