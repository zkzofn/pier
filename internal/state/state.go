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
	// LastPrompt is the most recent prompt submitted in this pane, condensed
	// to one line. Cleared on session-start (startup, /clear, resume).
	LastPrompt string `json:"lastPrompt,omitempty"`
	// Fresh marks a conversation with no content yet (startup or /clear):
	// the sidebar shows it blank instead of a transcript-derived title —
	// /clear carries the old ai-title into the new transcript, so falling
	// back would resurrect the previous conversation's label.
	Fresh bool  `json:"fresh,omitempty"`
	TS    int64 `json:"ts"`
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
	case "session-start":
		return Waiting // fresh conversation, idle at the input box
	}
	return ""
}

type hookPayload struct {
	SessionID string `json:"session_id"`
	CWD       string `json:"cwd"`
	Prompt    string `json:"prompt"` // UserPromptSubmit only
	Source    string `json:"source"` // SessionStart only: startup|resume|clear|compact
}

func fileFor(dir, paneID string) string {
	return filepath.Join(dir, strings.TrimPrefix(paneID, "%")+".json")
}

// Label condenses prompt-like text to a sidebar-sized label: the first line
// that survives tag stripping, capped at 120 runes. Wrapper tags such as
// <teammate-message ...> or <command-name>…</command-name> (as seen in hook
// payloads and transcript user records) are peeled off the line's edges.
func Label(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = cleanLine(line)
		if line == "" {
			continue
		}
		if r := []rune(line); len(r) > 120 {
			line = string(r[:120])
		}
		return line
	}
	return ""
}

// cleanLine strips leading <...> and trailing </...> wrapper tags.
func cleanLine(line string) string {
	line = strings.TrimSpace(line)
	for strings.HasPrefix(line, "<") {
		i := strings.Index(line, ">")
		if i < 0 {
			break
		}
		line = strings.TrimSpace(line[i+1:])
	}
	for strings.HasSuffix(line, ">") {
		i := strings.LastIndex(line, "</")
		if i < 0 {
			break
		}
		line = strings.TrimSpace(line[:i])
	}
	return line
}

// Record writes the state for $TMUX_PANE derived from a Claude Code hook
// event. Outside tmux it is a silent no-op. Skips the write when nothing
// changed so watchers aren't spammed on every tool call.
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
	// Compaction rewrites the transcript mid-conversation; it is not a
	// session boundary, so leave the recorded state and prompt alone.
	if event == "session-start" && p.Source == "compact" {
		return nil
	}

	path := fileFor(dir, pane)
	var prev PaneState
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &prev)
	}
	prompt, fresh := prev.LastPrompt, prev.Fresh
	switch event {
	case "user-prompt-submit":
		// Injected task-completion notifications also fire this hook; they
		// are not user instructions, so they don't replace the label.
		if !strings.HasPrefix(strings.TrimSpace(p.Prompt), "<task-notification") {
			prompt, fresh = Label(p.Prompt), false
		}
	case "session-start":
		prompt = "" // no current question yet
		// resume/fork continue an existing conversation, so a transcript
		// title still applies; startup and /clear start with nothing.
		fresh = p.Source != "resume" && p.Source != "fork"
	}
	if prev.State == st && prev.SessionID == p.SessionID &&
		prev.LastPrompt == prompt && prev.Fresh == fresh {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(PaneState{
		Pane: pane, State: st, SessionID: p.SessionID, CWD: p.CWD,
		LastPrompt: prompt, Fresh: fresh, TS: time.Now().Unix(),
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
