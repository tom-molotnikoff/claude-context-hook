package budget

import (
	"fmt"

	"github.com/tom-molotnikoff/claude-context-hook/internal/state"
)

const (
	MinThreshold = 10
	MaxThreshold = 90
	Window       = 1_000_000
)

func ArmedMessage(st state.State) string {
	return fmt.Sprintf("[ctx] on: stop at %d%%, early warning at %d%%, window %s.", st.Threshold, warnPercent(st), windowLabel(st.Window))
}

func Observe(st *state.State, tokens int64, transcriptPath string) string {
	if st.StartTokens == nil {
		st.StartTokens = &tokens
	}
	st.LastTokens = &tokens
	st.TranscriptPath = transcriptPath
	switch {
	case belowWarning(*st, tokens):
		st.WarnSent, st.StopSent = false, false
	case reachedStop(*st, tokens) && !st.StopSent:
		st.WarnSent, st.StopSent = true, true
		return fmt.Sprintf("[ctx] %d%% of context used (stop at %d%%). Finish the current task. Start no new tasks.",
			percent(tokens, st.Window), st.Threshold)
	case !reachedStop(*st, tokens) && !st.WarnSent:
		st.WarnSent = true
		start := percent(*st.StartTokens, st.Window)
		return fmt.Sprintf("[ctx] %d%% used (started at %d%%, stop at %d%%). Before each new task, average = (current - %d%%) / tasks done; start it only if current + average, adjusted for its size, stays at or under %d%%. Run ctx for the current figure.",
			percent(tokens, st.Window), start, st.Threshold, start, st.Threshold)
	}
	return ""
}

func Status(st state.State, tokens int64) string {
	return fmt.Sprintf("[ctx] %d%% used (%d of %d tokens), started at %d%%, early warning at %d%%, stop at %d%%.",
		percent(tokens, st.Window), tokens, st.Window, percent(*st.StartTokens, st.Window), warnPercent(st), st.Threshold)
}

func percent(tokens, window int64) int64 {
	return tokens * 100 / window
}

func warnPercent(st state.State) int {
	return st.Threshold * 2 / 3
}

func belowWarning(st state.State, tokens int64) bool {
	return tokens*100*3 < int64(st.Threshold)*2*st.Window
}

func reachedStop(st state.State, tokens int64) bool {
	return tokens*100 >= int64(st.Threshold)*st.Window
}

func windowLabel(window int64) string {
	if window%1_000_000 == 0 {
		return fmt.Sprintf("%dM", window/1_000_000)
	}
	return fmt.Sprintf("%dk", window/1_000)
}
