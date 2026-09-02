// Package setup wires pier into ~/.tmux.conf and Claude Code hooks.
// Everything is idempotent: re-running never duplicates configuration.
package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	if msg := ReloadTmux(tmuxConf); msg != "" {
		fmt.Println(msg)
	}
	fmt.Println("done — run `pier` (or your alias) to start")
	return nil
}

// ReloadTmux re-sources the config on a running server and returns the line
// to print — "" when no server is up or the reload failed. The message names
// the socket: `tmux` resolves one from $TMUX / $TMUX_TMPDIR / -L, so a
// reload can land on a different server than the one you meant, and saying
// which makes that visible instead of silent.
func ReloadTmux(tmuxConf string) string {
	if exec.Command("tmux", "has-session").Run() != nil {
		return ""
	}
	if exec.Command("tmux", "source-file", tmuxConf).Run() != nil {
		return ""
	}
	msg := "tmux: config reloaded on the running server"
	if out, err := exec.Command("tmux", "display-message", "-p", "#{socket_path}").Output(); err == nil {
		if sock := strings.TrimSpace(string(out)); sock != "" {
			msg += " (" + sock + ")"
		}
	}
	return msg
}

func tmuxBlock(self string) string {
	return strings.Join([]string{
		markerBegin,
		"set -g mouse on",
		// a session's end moves the client to the most recently active
		// session instead of dropping it out of tmux — `exit` in a pier
		// session behaves like prefix+x; only the last session closes the
		// client.
		"set -g detach-on-destroy off",
		"set -g status-interval 5",
		"set -g status-right-length 60",
		`set -g status-right '#(` + self + ` status "#{pane_current_path}") %H:%M '`,
		`set-hook -g client-attached 'run-shell "` + self + ` ensure"'`,
		`set-hook -g client-session-changed 'run-shell "` + self + ` ensure"'`,
		// the moment a pane dies, kill any session left holding only a
		// sidebar — no 2 s wait for the sidebar's own poll.
		`set-hook -g pane-exited 'run-shell "` + self + ` reap"'`,
		`bind-key g run-shell "` + self + ` toggle"`,
		// overrides tmux's default kill-pane binding — pier's session-level
		// "finish and move on" is the more frequent action here.
		`bind-key x run-shell "` + self + ` done"`,
		// bound directly (not via run-shell) so the popup lands on the
		// client that pressed the key. Keep the geometry and title in sync
		// with tmux.OpenNewPopup.
		`bind-key N display-popup -E -w 46 -h 22 -T " New session " "` + self + ` new"`,
		markerEnd,
		"",
	}, "\n")
}

// BlockStatus is what UpsertBlock did to the file.
type BlockStatus int

const (
	BlockUnchanged BlockStatus = iota
	BlockAdded
	BlockUpdated
)

// blockBounds locates pier's marker block in content: [start, end) covers
// the begin marker through the end marker inclusive.
func blockBounds(content string) (start, end int, ok bool) {
	i := strings.Index(content, markerBegin)
	if i < 0 {
		return 0, 0, false
	}
	j := strings.Index(content[i:], markerEnd)
	if j < 0 {
		return 0, 0, false
	}
	return i, i + j + len(markerEnd), true
}

// WithoutBlock returns content with pier's marker block cut out.
func WithoutBlock(content string) string {
	if i, j, ok := blockBounds(content); ok {
		return content[:i] + content[j:]
	}
	return content
}

// UpsertBlock writes block — marker lines included, newline-terminated —
// into the file at path: appended when the file has no marker block,
// replaced in place when it has a different one, untouched when identical.
// Content outside the markers is preserved; the file (and its directory) is
// created when missing.
func UpsertBlock(path, block string) (BlockStatus, error) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return BlockUnchanged, err
	}
	content := string(data)
	body := strings.TrimSuffix(block, "\n")
	if i, j, ok := blockBounds(content); ok {
		if content[i:j] == body {
			return BlockUnchanged, nil
		}
		content = content[:i] + body + content[j:]
		return BlockUpdated, writeAtomic(path, []byte(content))
	}
	if content != "" {
		content = strings.TrimRight(content, "\n") + "\n\n"
	}
	content += block
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return BlockUnchanged, err
	}
	return BlockAdded, writeAtomic(path, []byte(content))
}

// writeAtomic replaces a file through a temp file in the same directory, so
// a reader never sees a half-written config. `pier ensure` refreshes the
// tmux block from a hook that several attaching clients can fire at once.
func writeAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".pier-tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(0o644); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
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
	if _, _, managed := blockBounds(content); !managed && strings.Contains(content, "pier ensure") {
		return "tmux.conf: already configured (unmanaged) — skipped", nil
	}
	st, err := UpsertBlock(path, tmuxBlock(self))
	if err != nil {
		return "", err
	}
	switch st {
	case BlockAdded:
		return "tmux.conf: pier block added", nil
	case BlockUpdated:
		return "tmux.conf: pier block updated", nil
	}
	return "tmux.conf: already up to date — skipped", nil
}

// NeedsSetup reports whether pier still has to be wired: no pier tmux
// config at all (neither our marker block nor a hand-written `pier ensure`
// hook), or no pier hooks in Claude Code's settings. An outdated managed
// block is not "needs setup" — RefreshTmuxBlock handles that.
func NeedsSetup(tmuxConfPath, settingsPath string) bool {
	conf, _ := os.ReadFile(tmuxConfPath)
	_, _, managed := blockBounds(string(conf))
	tmuxOK := managed || strings.Contains(string(conf), "pier ensure")
	settings, _ := os.ReadFile(settingsPath)
	hooksOK := strings.Contains(string(settings), "pier hook")
	return !tmuxOK || !hooksOK
}

var selfRe = regexp.MustCompile(`run-shell "([^"]+) ensure"`)

// RefreshTmuxBlock brings an existing managed block up to date with this
// binary's tmuxBlock while keeping the binary path the block already uses —
// a dev build (`./bin/pier`, `go run .`) or a second install must not
// repoint the live config; that is `pier setup`'s job. Only when that path
// no longer exists does the running binary take over (on Linux the managed
// block holds a versioned Cellar path that disappears on upgrade). Unmanaged
// or missing blocks are left alone. Returns a one-line message when the file
// changed, "" otherwise.
func RefreshTmuxBlock(path, self string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	content := string(data)
	i, j, ok := blockBounds(content)
	if !ok {
		return "", nil
	}
	target := self
	if m := selfRe.FindStringSubmatch(content[i:j]); m != nil {
		if _, err := os.Stat(expandHome(m[1])); err == nil {
			target = m[1]
		}
	}
	st, err := UpsertBlock(path, tmuxBlock(target))
	if err != nil {
		return "", err
	}
	if st == BlockUpdated {
		return "tmux.conf: pier block updated", nil
	}
	return "", nil
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

func aliasBlock(name, target string) string {
	return strings.Join([]string{markerBegin, "alias " + name + "='" + target + "'", markerEnd, ""}, "\n")
}

// ShellAlias writes `alias name='target'` into the shell rc file at path as
// pier's marker block — added, or replaced when a previous alias is there.
func ShellAlias(path, name, target string) (BlockStatus, error) {
	return UpsertBlock(path, aliasBlock(name, target))
}

var aliasRe = regexp.MustCompile(`(?m)^alias ([^=\s]+)='`)

// AliasIn returns the alias name pier's block in the rc file defines, or "".
func AliasIn(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	i, j, ok := blockBounds(string(data))
	if !ok {
		return ""
	}
	if m := aliasRe.FindStringSubmatch(string(data)[i:j]); m != nil {
		return m[1]
	}
	return ""
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
