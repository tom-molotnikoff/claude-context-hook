package budget

import (
	"fmt"
	"strings"

	"github.com/tom-molotnikoff/claude-context-hook/internal/state"
)

const (
	MinThreshold = 10
	MaxThreshold = 90
)

func Window(model string) int64 {
	if strings.HasSuffix(model, "[1m]") {
		return 1_000_000
	}
	return 200_000
}

func ArmedMessage(st state.State) string {
	return fmt.Sprintf("[ctx] on: stop at %d%%, early warning at %d%%, window %s.", st.Threshold, warnPercent(st), windowLabel(st.Window))
}

func Observe(st *state.State, tokens int64, transcriptPath string) {
	if st.StartTokens == nil {
		st.StartTokens = &tokens
	}
	st.LastTokens = &tokens
	st.TranscriptPath = transcriptPath
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

func windowLabel(window int64) string {
	if window%1_000_000 == 0 {
		return fmt.Sprintf("%dM", window/1_000_000)
	}
	return fmt.Sprintf("%dk", window/1_000)
}
