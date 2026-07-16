// Package state records and reads per-pane Claude Code activity, written by
// `pier hook` (wired into Claude Code hooks) and read by the sidebar TUI.
// tmux pane listing is the source of truth; this state only decorates it.
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Pane activity states.
const (
	Working    = "working"    // agent is running a turn
	Waiting    = "waiting"    // turn finished, waiting for user input
	Permission = "permission" // waiting for a permission approval
)

// PaneState is one pane's recorded activity.
type PaneState struct {
	Pane      string `json:"pane"`
	State     string `json:"state"`
	SessionID string `json:"sessionId,omitempty"`
	CWD       string `json:"cwd,omitempty"`
	TS        int64  `json:"ts"`
}

// Dir returns the state directory, honoring XDG_STATE_HOME.
func Dir() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "pier", "panes")
}

// ForEvent maps a pier hook event name to a pane state ("" = ignore).
func ForEvent(event string) string {
	switch event {
	case "user-prompt-submit", "pre-tool-use":
		return Working
	case "stop":
		return Waiting
	case "permission-request":
		return Permission
	}
	return ""
}

type hookPayload struct {
	SessionID string `json:"session_id"`
	CWD       string `json:"cwd"`
}

func fileFor(dir, paneID string) string {
	return filepath.Join(dir, strings.TrimPrefix(paneID, "%")+".json")
}

// Record writes the state for $TMUX_PANE derived from a Claude Code hook
// event. Outside tmux it is a silent no-op. Skips the write when the state
// is unchanged so watchers aren't spammed on every tool call.
func Record(dir, event string, payload []byte) error {
	pane := os.Getenv("TMUX_PANE")
	if pane == "" {
		return nil
	}
	st := ForEvent(event)
	if st == "" {
		return nil
	}
	var p hookPayload
	_ = json.Unmarshal(payload, &p)

	path := fileFor(dir, pane)
	if prev, err := os.ReadFile(path); err == nil {
		var prevState PaneState
		if json.Unmarshal(prev, &prevState) == nil &&
			prevState.State == st && prevState.SessionID == p.SessionID {
			return nil
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(PaneState{
		Pane: pane, State: st, SessionID: p.SessionID, CWD: p.CWD,
		TS: time.Now().Unix(),
	})
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ReadAll loads every recorded pane state, keyed by pane id (e.g. "%23").
func ReadAll(dir string) map[string]PaneState {
	states := map[string]PaneState{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return states
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var ps PaneState
		if json.Unmarshal(data, &ps) != nil || ps.Pane == "" {
			continue
		}
		states[ps.Pane] = ps
	}
	return states
}

// Cleanup removes state files for panes not in the live set.
func Cleanup(dir string, livePanes map[string]bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".json")
		if name == e.Name() {
			continue
		}
		if !livePanes["%"+name] {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
