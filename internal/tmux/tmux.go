// Package tmux wraps the tmux CLI commands pier needs.
package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Pane is one tmux pane as reported by list-panes.
type Pane struct {
	Session   string
	WindowIdx string
	PaneIdx   string
	ID        string // e.g. "%23"
	Command   string // pane_current_command
	Path      string // pane_current_path
	Sidebar   bool   // @pier user option set
}

// SidebarWidth is the fixed sidebar pane width in columns.
const SidebarWidth = 30

const paneFormat = "#{session_name}\t#{window_index}\t#{pane_index}\t#{pane_id}\t#{pane_current_command}\t#{pane_current_path}\t#{@pier}"

func run(args ...string) (string, error) {
	out, err := exec.Command("tmux", args...).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("tmux %s: %s", args[0], strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("tmux %s: %w", args[0], err)
	}
	// Trim newlines ONLY: a trailing empty tab-separated field (e.g. unset
	// @pier on the last pane line) must survive, or that pane vanishes.
	return strings.Trim(string(out), "\n"), nil
}

// ParsePanes parses list-panes output lines in paneFormat.
func ParsePanes(out string) []Pane {
	var panes []Pane
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		// The trailing @pier field is empty for normal panes and may get
		// stripped along the way; require only the 6 guaranteed fields.
		if len(f) < 6 {
			continue
		}
		panes = append(panes, Pane{
			Session:   f[0],
			WindowIdx: f[1],
			PaneIdx:   f[2],
			ID:        f[3],
			Command:   f[4],
			Path:      f[5],
			Sidebar:   len(f) >= 7 && f[6] == "1",
		})
	}
	return panes
}

// ListPanes returns every pane on the tmux server.
func ListPanes() ([]Pane, error) {
	out, err := run("list-panes", "-a", "-F", paneFormat)
	if err != nil {
		return nil, err
	}
	return ParsePanes(out), nil
}

// CurrentSession returns the session this process belongs to (via $TMUX_PANE),
// falling back to the current client's session.
func CurrentSession() string {
	if pane := os.Getenv("TMUX_PANE"); pane != "" {
		if out, err := run("display-message", "-t", pane, "-p", "#{session_name}"); err == nil {
			return out
		}
	}
	out, _ := run("display-message", "-p", "#{client_session}")
	return out
}

// Jump focuses the given pane: selects it inside its window/session and
// switches every client viewing our session over to the target session.
func Jump(paneID string) error {
	if _, err := run("select-pane", "-t", paneID); err != nil {
		return err
	}
	if _, err := run("select-window", "-t", paneID); err != nil {
		return err
	}
	target, err := run("display-message", "-t", paneID, "-p", "#{session_name}")
	if err != nil {
		return err
	}
	cur := CurrentSession()
	if cur == target {
		return nil
	}
	// A pane-bound TUI cannot tell which client a click came from, but the
	// click itself just bumped that client's activity — so among clients
	// viewing OUR session, the most recently active one is the clicker.
	// Switch only that one; the others keep their screens. No fallback to
	// arbitrary clients: that would yank someone else's screen.
	out, _ := run("list-clients", "-F", "#{client_name}\t#{client_session}\t#{client_activity}")
	if best := PickMostRecentClient(out, cur); best != "" {
		_, err := run("switch-client", "-c", best, "-t", target)
		return err
	}
	return nil
}

// PickMostRecentClient returns the most recently active client attached to
// the given session, parsing "name\tsession\tactivity" lines.
func PickMostRecentClient(listClientsOut, session string) string {
	var best string
	var bestAct int64 = -1
	for _, line := range strings.Split(listClientsOut, "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 3 || f[1] != session {
			continue
		}
		act, err := strconv.ParseInt(f[2], 10, 64)
		if err != nil {
			continue
		}
		if act > bestAct {
			bestAct, best = act, f[0]
		}
	}
	return best
}

// SessionNames returns the set of existing tmux session names.
func SessionNames() map[string]bool {
	names := map[string]bool{}
	out, _ := run("list-sessions", "-F", "#{session_name}")
	for _, s := range strings.Split(out, "\n") {
		if s != "" {
			names[s] = true
		}
	}
	return names
}

// NewSessionAndSwitch creates a detached Claude Code session at path and
// switches the current client to it. Inside a popup the current client is
// the one hosting the popup — exactly who asked. A failed switch (headless
// contexts have no client) is not an error: the session exists either way.
func NewSessionAndSwitch(name, path string) error {
	if _, err := run("new-session", "-d", "-s", name, "-c", path, "claude"); err != nil {
		return err
	}
	_, _ = run("switch-client", "-t", name)
	return nil
}

// SwitchTo focuses an existing session from the current client.
func SwitchTo(session string) error {
	_, err := run("switch-client", "-t", session)
	return err
}

// OpenNewPopup opens the new-session picker as a centered popup on the
// client that just interacted with our session.
func OpenNewPopup(self, session string) error {
	args := []string{"display-popup", "-E", "-w", "46", "-h", "18", "-T", " New CC session "}
	out, _ := run("list-clients", "-F", "#{client_name}\t#{client_session}\t#{client_activity}")
	if client := PickMostRecentClient(out, session); client != "" {
		args = append(args, "-c", client)
	}
	args = append(args, self+" new")
	_, err := run(args...)
	return err
}

// sidebarPaneIn returns the pane id of an existing sidebar in the session.
func sidebarPaneIn(session string) string {
	out, _ := run("list-panes", "-s", "-t", session, "-F", "#{pane_id}\t#{@pier}")
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\t")
		if len(f) == 2 && f[1] == "1" {
			return f[0]
		}
	}
	return ""
}

// EnsureSidebar creates a left sidebar pane running `self run` in the
// session's active window unless the session already has one. Idempotent.
func EnsureSidebar(session, self string) error {
	if session == "" {
		return fmt.Errorf("no target session (not inside tmux?)")
	}
	if sidebarPaneIn(session) != "" {
		return nil
	}
	paneID, err := run("split-window", "-hb", "-l", fmt.Sprint(SidebarWidth), "-d", "-P", "-F", "#{pane_id}",
		"-t", session+":.", "exec "+self+" run")
	if err != nil {
		return err
	}
	_, err = run("set-option", "-p", "-t", paneID, "@pier", "1")
	return err
}

// RestoreWidth resizes a pane back to the fixed sidebar width. Multi-client
// layout reflows can squeeze the sidebar; the TUI calls this to self-heal.
func RestoreWidth(paneID string) {
	_, _ = run("resize-pane", "-t", paneID, "-x", fmt.Sprint(SidebarWidth))
}

// ToggleSidebar kills the session's sidebar pane if present, else creates one.
func ToggleSidebar(session, self string) error {
	if id := sidebarPaneIn(session); id != "" {
		_, err := run("kill-pane", "-t", id)
		return err
	}
	return EnsureSidebar(session, self)
}
