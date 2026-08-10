// Package setup wires pier into ~/.tmux.conf and Claude Code hooks.
// Everything is idempotent: re-running never duplicates configuration.
package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	markerBegin = "# >>> pier >>>"
	markerEnd   = "# <<< pier <<<"
)

// Run configures tmux and Claude Code for the pier binary at `self`,
// printing a one-line result per target.
func Run(self string) error {
	if _, err := exec.LookPath("tmux"); err != nil {
		return fmt.Errorf("tmux not found in PATH — install it first (e.g. `brew install tmux`), then re-run `pier setup`")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	tmuxConf := filepath.Join(home, ".tmux.conf")
	msg, err := TmuxConf(tmuxConf, self)
	if err != nil {
		return fmt.Errorf("tmux.conf: %w", err)
	}
	fmt.Println(msg)

	msg, err = ClaudeHooks(filepath.Join(home, ".claude", "settings.json"), self)
	if err != nil {
		return fmt.Errorf("claude hooks: %w", err)
	}
	fmt.Println(msg)

	// Apply live if a tmux server is already running.
	if exec.Command("tmux", "has-session").Run() == nil {
		if exec.Command("tmux", "source-file", tmuxConf).Run() == nil {
			fmt.Println("tmux: config reloaded on the running server")
		}
	}
	fmt.Println("done — attach to (or switch) a tmux session and the sidebar appears")
	return nil
}

func tmuxBlock(self string) string {
	return strings.Join([]string{
		markerBegin,
		"set -g mouse on",
		"set -g status-interval 5",
		"set -g status-right-length 60",
		`set -g status-right '#(` + self + ` status "#{pane_current_path}") %H:%M '`,
		`set-hook -g client-attached 'run-shell "` + self + ` ensure"'`,
		`set-hook -g client-session-changed 'run-shell "` + self + ` ensure"'`,
		`bind-key g run-shell "` + self + ` toggle"`,
		// overrides tmux's default kill-pane binding — pier's session-level
		// "finish and move on" is the more frequent action here.
		`bind-key x run-shell "` + self + ` done"`,
		// bound directly (not via run-shell) so the popup lands on the
		// client that pressed the key. Keep the geometry and title in sync
		// with tmux.OpenNewPopup.
		`bind-key N display-popup -E -w 46 -h 18 -T " New session " "` + self + ` new"`,
		markerEnd,
		"",
	}, "\n")
}

// TmuxConf writes the pier block into the tmux config: appended when absent,
// rewritten in place when the marker block exists but is outdated (so
// upgrades pick up new bindings). A pier reference outside our markers means
// a manual setup — left untouched. The file is created when missing.
func TmuxConf(path, self string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	content := string(data)
	if i := strings.Index(content, markerBegin); i >= 0 {
		if j := strings.Index(content, markerEnd); j > i {
			block := strings.TrimSuffix(tmuxBlock(self), "\n")
			if content[i:j+len(markerEnd)] == block {
				return "tmux.conf: already up to date — skipped", nil
			}
			content = content[:i] + block + content[j+len(markerEnd):]
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				return "", err
			}
			return "tmux.conf: pier block updated", nil
		}
	}
	if strings.Contains(content, "pier ensure") {
		return "tmux.conf: already configured (unmanaged) — skipped", nil
	}
	if content != "" {
		content = strings.TrimRight(content, "\n") + "\n\n"
	}
	content += tmuxBlock(self)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return "tmux.conf: pier block added", nil
}

var hookEvents = []struct {
	event   string // settings.json hooks key
	arg     string // pier hook <arg>
	matcher bool   // event type requires a matcher field
}{
	{"UserPromptSubmit", "user-prompt-submit", false},
	{"PreToolUse", "pre-tool-use", true},
	{"Stop", "stop", false},
	{"PermissionRequest", "permission-request", true},
	// no matcher: fire on every source (startup|resume|clear|compact);
	// pier itself ignores compact.
	{"SessionStart", "session-start", false},
	// retires the live-session marker (crash/shutdown auto-resume).
	{"SessionEnd", "session-end", false},
}

// ClaudeHooks merges pier's hook entries into Claude Code's settings.json,
// preserving all existing settings and hooks. A .bak-pier backup of the
// previous file is written before the first modification.
func ClaudeHooks(path, self string) (string, error) {
	settings := map[string]any{}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &settings); err != nil {
			return "", fmt.Errorf("parse %s: %w", path, err)
		}
	case os.IsNotExist(err):
		// start from an empty settings object
	default:
		return "", err
	}

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	added := 0
	for _, ev := range hookEvents {
		arr, _ := hooks[ev.event].([]any)
		if raw, _ := json.Marshal(arr); strings.Contains(string(raw), "pier hook") {
			continue
		}
		entry := map[string]any{
			"hooks": []any{map[string]any{
				"type":    "command",
				"command": self + " hook " + ev.arg,
				"timeout": 5,
			}},
		}
		if ev.matcher {
			entry["matcher"] = "*"
		}
		hooks[ev.event] = append(arr, entry)
		added++
	}
	if added == 0 {
		return "claude hooks: already configured — skipped", nil
	}
	settings["hooks"] = hooks

	if data != nil {
		if err := os.WriteFile(path+".bak-pier", data, 0o600); err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("claude hooks: %d event(s) wired (backup: %s.bak-pier)", added, filepath.Base(path)), nil
}
