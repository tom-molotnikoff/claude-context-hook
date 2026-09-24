package budget_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/tom-molotnikoff/claude-context-hook/internal/budget"
	"github.com/tom-molotnikoff/claude-context-hook/internal/state"
)

const (
	warn75 = "[ctx] %d%% used (started at 4%%, stop at 75%%). Before each new task, average = (current - 4%%) / tasks done; start it only if current + average, adjusted for its size, stays at or under 75%%. Run ctx for the current figure."
	stop75 = "[ctx] %d%% of context used (stop at 75%%). Finish the current task. Start no new tasks."
)

func TestObserve(t *testing.T) {
	cases := []struct {
		name      string
		threshold int
		warnSent  bool
		stopSent  bool
		tokens    []int64
		want      []string
		wantWarn  bool
		wantStop  bool
	}{
		{
			name:   "below both points",
			tokens: []int64{60_000, 99_999},
			want:   []string{"", ""},
		},
		{
			name:     "crossing the warning",
			tokens:   []int64{100_000, 120_000},
			want:     []string{fmt.Sprintf(warn75, 50), ""},
			wantWarn: true,
		},
		{
			name:     "crossing the stop",
			tokens:   []int64{100_000, 149_999, 150_000, 170_000},
			want:     []string{fmt.Sprintf(warn75, 50), "", fmt.Sprintf(stop75, 75), ""},
			wantWarn: true,
			wantStop: true,
		},
		{
			name:     "jumping past both",
			tokens:   []int64{99_000, 180_000, 120_000},
			want:     []string{"", fmt.Sprintf(stop75, 90), ""},
			wantWarn: true,
			wantStop: true,
		},
		{
			name:     "after compaction",
			warnSent: true,
			stopSent: true,
			tokens:   []int64{160_000, 30_000, 110_000, 150_000},
			want:     []string{"", "", fmt.Sprintf(warn75, 55), fmt.Sprintf(stop75, 75)},
			wantWarn: true,
			wantStop: true,
		},
		{
			name:     "compaction clears both flags",
			warnSent: true,
			stopSent: true,
			tokens:   []int64{99_999},
			want:     []string{""},
		},
		{
			name:     "warning already sent",
			warnSent: true,
			tokens:   []int64{100_000, 140_000},
			want:     []string{"", ""},
			wantWarn: true,
		},
		{
			name:     "stop already sent",
			warnSent: true,
			stopSent: true,
			tokens:   []int64{100_000, 200_000},
			want:     []string{"", ""},
			wantWarn: true,
			wantStop: true,
		},
		{
			name:      "warning point is not rounded",
			threshold: 80,
			tokens:    []int64{106_666, 106_667},
			want:      []string{"", "[ctx] 53% used (started at 4%, stop at 80%). Before each new task, average = (current - 4%) / tasks done; start it only if current + average, adjusted for its size, stays at or under 80%. Run ctx for the current figure."},
			wantWarn:  true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			threshold := c.threshold
			if threshold == 0 {
				threshold = 75
			}
			start := int64(8_000)
			st := state.State{Threshold: threshold, Window: 200_000, StartTokens: &start, WarnSent: c.warnSent, StopSent: c.stopSent}
			for i, tokens := range c.tokens {
				if got := budget.Observe(&st, tokens, "/t.jsonl"); got != c.want[i] {
					t.Errorf("observation %d (%d tokens): got %q, want %q", i, tokens, got, c.want[i])
				}
			}
			if st.WarnSent != c.wantWarn || st.StopSent != c.wantStop {
				t.Errorf("flags warn=%v stop=%v, want warn=%v stop=%v", st.WarnSent, st.StopSent, c.wantWarn, c.wantStop)
			}
			if *st.StartTokens != 8_000 {
				t.Errorf("start changed to %d", *st.StartTokens)
			}
		})
	}
}

func TestStartIsShownAgainstTheCurrentWindow(t *testing.T) {
	start := int64(40_000)
	st := state.State{Threshold: 75, Window: 1_000_000, StartTokens: &start}
	got := budget.Observe(&st, 500_000, "/t.jsonl")
	if !strings.HasPrefix(got, "[ctx] 50% used (started at 4%, stop at 75%).") {
		t.Errorf("got %q", got)
	}
}

func TestMessagesNameNoWorkflowStep(t *testing.T) {
	st := state.State{Threshold: 75, Window: 200_000}
	messages := []string{
		budget.Observe(&st, 8_000, "/t.jsonl"),
		budget.Observe(&st, 100_000, "/t.jsonl"),
		budget.Observe(&st, 150_000, "/t.jsonl"),
	}
	workflow := regexp.MustCompile(`(?i)\b(merg\w*|pull requests?|prs?|ci|issues?|commits?|push\w*|reviews?)\b`)
	for _, m := range messages[1:] {
		if m == "" {
			t.Fatal("expected a message")
		}
		if w := workflow.FindString(m); w != "" {
			t.Errorf("%q mentions %q", m, w)
		}
	}
}
