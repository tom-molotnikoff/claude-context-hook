package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

type State struct {
	Threshold      int       `json:"threshold"`
	Window         int64     `json:"window"`
	StartTokens    *int64    `json:"start_tokens"`
	LastTokens     *int64    `json:"last_tokens"`
	WarnSent       bool      `json:"warn_sent"`
	StopSent       bool      `json:"stop_sent"`
	TranscriptPath string    `json:"transcript_path"`
	ArmedAt        time.Time `json:"armed_at"`
}

var (
	ErrNotArmed       = errors.New("not armed")
	ErrCorrupt        = errors.New("state file corrupt")
	ErrInvalidSession = errors.New("invalid session id")
)

type Store struct {
	dir string
}

func Open(getenv func(string) string) Store {
	base := getenv("XDG_STATE_HOME")
	if base == "" {
		base = filepath.Join(getenv("HOME"), ".local", "state")
	}
	return Store{dir: filepath.Join(base, "ctx")}
}

func (s Store) Load(session string) (State, error) {
	path, err := s.path(session)
	if err != nil {
		return State{}, err
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return State{}, ErrNotArmed
	}
	if err != nil {
		return State{}, err
	}
	return decode(raw)
}

func (s Store) Update(session string, change func(*State) error) error {
	_, err := s.modify(session, false, change)
	return err
}

const staleAfter = 7 * 24 * time.Hour

func (s Store) Arm(session string, threshold int, window int64, now time.Time) (State, error) {
	st, err := s.modify(session, true, func(st *State) error {
		if st.Window == 0 {
			*st = State{Window: window, ArmedAt: now}
		}
		st.Threshold = threshold
		return nil
	})
	if err == nil {
		s.removeStale(now.Add(-staleAfter))
	}
	return st, err
}

func (s Store) removeStale(cutoff time.Time) {
	paths, _ := filepath.Glob(filepath.Join(s.dir, "*.json"))
	for _, path := range paths {
		if info, err := os.Stat(path); err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		f, err := lock(path)
		if err != nil {
			continue
		}
		if info, err := f.Stat(); err == nil && info.ModTime().Before(cutoff) {
			os.Remove(path)
		}
		f.Close()
	}
}

func (s Store) LogError(session string, cause error, now time.Time) error {
	f, err := os.OpenFile(filepath.Join(s.dir, "error.log"), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s %s %v\n", now.UTC().Format(time.RFC3339), session, cause)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

func (s Store) path(session string) (string, error) {
	if session == "" || session == "." || session == ".." || filepath.Base(session) != session {
		return "", ErrInvalidSession
	}
	return filepath.Join(s.dir, session+".json"), nil
}

func (s Store) modify(session string, create bool, change func(*State) error) (State, error) {
	path, err := s.path(session)
	if err != nil {
		return State{}, err
	}
	if create {
		if err := os.MkdirAll(s.dir, 0o700); err != nil {
			return State{}, err
		}
	}
	for {
		f, err := lock(path)
		if errors.Is(err, fs.ErrNotExist) {
			if !create {
				return State{}, ErrNotArmed
			}
			var st State
			if err := change(&st); err != nil {
				return State{}, err
			}
			created, err := s.create(path, st)
			if err != nil || created {
				return st, err
			}
			continue
		}
		if err != nil {
			return State{}, err
		}
		st, err := s.rewrite(f, path, create, change)
		f.Close()
		return st, err
	}
}

func (s Store) rewrite(f *os.File, path string, create bool, change func(*State) error) (State, error) {
	raw, err := io.ReadAll(f)
	if err != nil {
		return State{}, err
	}
	st, err := decode(raw)
	if err != nil && !create {
		return State{}, err
	}
	if err := change(&st); err != nil {
		return State{}, err
	}
	tmp, err := s.writeTemp(st)
	if err != nil {
		return State{}, err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return State{}, err
	}
	return st, nil
}

func (s Store) create(path string, st State) (bool, error) {
	tmp, err := s.writeTemp(st)
	if err != nil {
		return false, err
	}
	defer os.Remove(tmp)
	err = os.Link(tmp, path)
	if errors.Is(err, fs.ErrExist) {
		return false, nil
	}
	return err == nil, err
}

func (s Store) writeTemp(st State) (string, error) {
	raw, err := json.Marshal(st)
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp(s.dir, ".tmp-*")
	if err != nil {
		return "", err
	}
	_, err = f.Write(raw)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

func lock(path string) (*os.File, error) {
	for {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
			f.Close()
			return nil, err
		}
		held, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, err
		}
		current, err := os.Stat(path)
		if err == nil && os.SameFile(held, current) {
			return f, nil
		}
		f.Close()
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
}

func decode(raw []byte) (State, error) {
	var st State
	if err := json.Unmarshal(raw, &st); err != nil || st.Window <= 0 || st.Threshold <= 0 || (st.TranscriptPath != "" && st.StartTokens == nil) {
		return State{}, ErrCorrupt
	}
	return st, nil
}
