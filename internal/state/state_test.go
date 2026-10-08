package state_test

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/tom-molotnikoff/claude-context-hook/internal/state"
)

func TestConcurrentUpdatesAreSerialised(t *testing.T) {
	base := t.TempDir()
	store := state.Open(func(k string) string {
		if k == "XDG_STATE_HOME" {
			return base
		}
		return ""
	})
	if _, err := store.Arm("s", 75, 200_000, time.Now()); err != nil {
		t.Fatal(err)
	}
	const writers = 50
	var wg sync.WaitGroup
	for range writers {
		wg.Go(func() {
			err := store.Update("s", func(st *state.State) error {
				n := int64(1)
				if st.LastTokens != nil {
					n = *st.LastTokens + 1
				}
				st.LastTokens = &n
				return nil
			})
			if err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	st, err := store.Load("s")
	if err != nil {
		t.Fatal(err)
	}
	if st.LastTokens == nil || *st.LastTokens != writers {
		t.Errorf("got %v updates, want %d", st.LastTokens, writers)
	}
}

func TestUpdateOfUnarmedSession(t *testing.T) {
	store := state.Open(func(k string) string {
		if k == "XDG_STATE_HOME" {
			return t.TempDir()
		}
		return ""
	})
	called := false
	err := store.Update("s", func(*state.State) error {
		called = true
		return nil
	})
	if err != state.ErrNotArmed || called {
		t.Errorf("got %v, called %v", err, called)
	}
}

func TestDirFollowsPlatformConvention(t *testing.T) {
	cases := []struct {
		name string
		goos string
		env  map[string]string
		want string
	}{
		{"linux", "linux", map[string]string{"HOME": "/home/u"}, "/home/u/.local/state/ctx"},
		{"darwin", "darwin", map[string]string{"HOME": "/Users/u"}, "/Users/u/Library/Application Support/ctx"},
		{"linux with XDG_STATE_HOME", "linux", map[string]string{"HOME": "/home/u", "XDG_STATE_HOME": "/x"}, "/x/ctx"},
		{"darwin with XDG_STATE_HOME", "darwin", map[string]string{"HOME": "/Users/u", "XDG_STATE_HOME": "/x"}, "/x/ctx"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := state.Dir(c.goos, func(k string) string { return c.env[k] })
			if got != filepath.FromSlash(c.want) {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
