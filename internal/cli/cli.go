package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strconv"
	"strings"
	"time"

	"github.com/tom-molotnikoff/claude-context-hook/internal/budget"
	"github.com/tom-molotnikoff/claude-context-hook/internal/state"
	"github.com/tom-molotnikoff/claude-context-hook/internal/transcript"
)

const armUsage = "usage: ctx arm <threshold> --model <model-id>"

type Env struct {
	Args   []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Getenv func(string) string
	Now    func() time.Time
}

func Run(env Env) int {
	store := state.Open(env.Getenv)
	if len(env.Args) == 0 {
		return status(env, store)
	}
	switch env.Args[0] {
	case "arm":
		return arm(env, store, env.Args[1:])
	case "hook":
		return hook(env, store)
	}
	fmt.Fprintln(env.Stderr, "usage: ctx | ctx arm <threshold> --model <model-id> | ctx hook")
	return 2
}

func arm(env Env, store state.Store, args []string) int {
	var threshold, model string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--model" && i+1 < len(args):
			model = args[i+1]
			i++
		case threshold == "" && !strings.HasPrefix(args[i], "--"):
			threshold = args[i]
		default:
			fmt.Fprintln(env.Stderr, armUsage)
			return 2
		}
	}
	if threshold == "" || model == "" {
		fmt.Fprintln(env.Stderr, armUsage)
		return 2
	}
	t, err := strconv.Atoi(threshold)
	if err != nil || t < budget.MinThreshold || t > budget.MaxThreshold {
		fmt.Fprintf(env.Stderr, "ctx: threshold must be an integer from %d to %d\n", budget.MinThreshold, budget.MaxThreshold)
		return 2
	}
	session := env.Getenv("CLAUDE_CODE_SESSION_ID")
	if session == "" {
		fmt.Fprintln(env.Stderr, "ctx: CLAUDE_CODE_SESSION_ID is not set, so there is no session ID to arm")
		return 1
	}
	st, err := store.Arm(session, t, budget.Window(model), env.Now())
	if err != nil {
		fmt.Fprintf(env.Stderr, "ctx: %v\n", err)
		return 1
	}
	fmt.Fprintln(env.Stdout, budget.ArmedMessage(st))
	return 0
}

func status(env Env, store state.Store) int {
	st, err := store.Load(env.Getenv("CLAUDE_CODE_SESSION_ID"))
	switch {
	case errors.Is(err, state.ErrNotArmed), errors.Is(err, state.ErrInvalidSession):
		fmt.Fprintln(env.Stdout, "[ctx] not on for this session.")
		return 1
	case errors.Is(err, state.ErrCorrupt):
		return noFigure(env, "state file corrupt")
	case err != nil:
		return noFigure(env, fmt.Sprintf("state file unreadable (%v)", err))
	case st.TranscriptPath == "":
		return noFigure(env, "no transcript path recorded yet")
	}
	tokens, err := transcript.ReadFile(st.TranscriptPath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return noFigure(env, "transcript missing")
	case errors.Is(err, transcript.ErrNoUsage):
		return noFigure(env, "no assistant usage in transcript")
	case err != nil:
		return noFigure(env, fmt.Sprintf("transcript unreadable (%v)", err))
	}
	fmt.Fprintln(env.Stdout, budget.Status(st, tokens))
	return 0
}

func noFigure(env Env, reason string) int {
	fmt.Fprintf(env.Stdout, "[ctx] no figure: %s\n", reason)
	return 1
}

type hookInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	AgentID        string `json:"agent_id"`
}

func hook(env Env, store state.Store) int {
	var in hookInput
	if err := json.NewDecoder(env.Stdin).Decode(&in); err != nil || in.AgentID != "" {
		return 0
	}
	store.Update(in.SessionID, func(st *state.State) error {
		tokens, err := transcript.ReadFile(in.TranscriptPath)
		if err != nil {
			return err
		}
		budget.Observe(st, tokens, in.TranscriptPath)
		return nil
	})
	return 0
}
