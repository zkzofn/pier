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

// Run configures tmux, Claude Code, and Antigravity CLI for the pier binary
// at `self`, printing a one-line result per target.
func Run(self string) error {
	if _, err := exec.LookPath("tmux"); err != nil {
		return fmt.Errorf("tmux not found in PATH — install it first (e.g. `brew install tmux`), then re-run `pier setup`")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	tmuxBin := StableSelf(self)
	hookBin := PortableBin(self, exec.LookPath)

	tmuxConf := filepath.Join(home, ".tmux.conf")
	msg, err := TmuxConf(tmuxConf, tmuxBin)
	if err != nil {
		return fmt.Errorf("tmux.conf: %w", err)
	}
	fmt.Println(msg)

	claudeSettings := filepath.Join(home, ".claude", "settings.json")
	msg, err = ClaudeHooks(claudeSettings, hookBin)
	if err != nil {
		return fmt.Errorf("claude hooks: %w", err)
	}
	fmt.Println(msg)

	msg, err = ClaudeStatusLine(claudeSettings, hookBin)
	if err != nil {
		return fmt.Errorf("claude statusLine: %w", err)
	}
	fmt.Println(msg)
	statusWired := strings.Contains(msg, "wired") || strings.Contains(msg, "wrapped")

	if hasAgy(home) {
		msg, err = AgyHooks(filepath.Join(home, ".gemini", "config", "hooks.json"), hookBin)
		if err != nil {
			return fmt.Errorf("agy hooks: %w", err)
		}
		fmt.Println(msg)

		msg, err = AgyStatusLine(filepath.Join(home, ".gemini", "antigravity-cli", "settings.json"), hookBin)
		if err != nil {
			return fmt.Errorf("agy statusLine: %w", err)
		}
		fmt.Println(msg)
		if strings.Contains(msg, "wired") || strings.Contains(msg, "wrapped") {
			statusWired = true
		}
	}

	// Apply live if a tmux server is already running.
	if msg := ReloadTmux(tmuxConf); msg != "" {
		fmt.Println(msg)
	}
	if statusWired {
		fmt.Println("note: agy/claude read the statusLine config only at startup — restart any sessions already running so they start reporting usage")
	}
	fmt.Println("done — run `pier` to start")
	return nil
}

func hasAgy(home string) bool {
	if _, err := exec.LookPath("agy"); err == nil {
		return true
	}
	_, err := os.Stat(filepath.Join(home, ".gemini"))
	return err == nil
}

// StableSelf normalizes a Homebrew versioned Cellar binary path
// (e.g. /opt/homebrew/Cellar/pier/1.3.1/bin/pier) to its linked bin path
// (/opt/homebrew/bin/pier) when both resolve to the same executable.
func StableSelf(self string) string {
	const marker = "/Cellar/pier/"
	i := strings.Index(self, marker)
	if i < 0 || !strings.HasSuffix(self, "/bin/pier") {
		return self
	}
	cand := self[:i] + "/bin/pier"
	realCand, err1 := filepath.EvalSymlinks(cand)
	realSelf, err2 := filepath.EvalSymlinks(self)
	if err1 == nil && err2 == nil && realCand == realSelf {
		return cand
	}
	return self
}

// PortableBin returns "pier" when PATH resolves to the same binary as self,
// avoiding machine-specific absolute paths in synced hook configs.
func PortableBin(self string, lookPath func(string) (string, error)) string {
	stable := StableSelf(self)
	if lookPath == nil {
		return stable
	}
	p, err := lookPath("pier")
	if err != nil {
		return stable
	}
	realP, err1 := filepath.EvalSymlinks(p)
	realSelf, err2 := filepath.EvalSymlinks(self)
	if err1 == nil && err2 == nil && realP == realSelf {
		return "pier"
	}
	return stable
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
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
		"set -g status-right-length 140",
		`set -g status-right '#(` + self + ` status "#{pane_current_path}" "#{session_name}" "#{pane_id}" "#{client_width}") %H:%M '`,
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
	return writeAtomicMode(path, data, 0o644)
}

func writeAtomicMode(path string, data []byte, mode os.FileMode) error {
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
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func writeBackupOnce(path string, data []byte) error {
	if data == nil {
		return nil
	}
	bak := path + ".bak-pier"
	if _, err := os.Stat(bak); err == nil {
		return nil
	}
	return os.WriteFile(bak, data, 0o600)
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
	target := StableSelf(self)
	if m := selfRe.FindStringSubmatch(content[i:j]); m != nil {
		if _, err := os.Stat(expandHome(m[1])); err == nil {
			target = StableSelf(m[1])
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

	if err := writeBackupOnce(path, data); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", err
	}
	if err := writeAtomicMode(path, append(out, '\n'), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("claude hooks: %d event(s) wired (backup: %s.bak-pier)", added, filepath.Base(path)), nil
}

// ClaudeStatusLine configures Claude Code's statusLine to pass JSON payloads
// through `pier statusline claude`, wrapping any existing statusLine command
// transparently so the user's original status line output stays intact.
func ClaudeStatusLine(path, self string) (string, error) {
	settings := map[string]any{}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &settings); err != nil {
			return "", fmt.Errorf("parse %s: %w", path, err)
		}
	case os.IsNotExist(err):
	default:
		return "", err
	}

	sl, _ := settings["statusLine"].(map[string]any)
	existingCmd, _ := sl["command"].(string)
	if strings.Contains(existingCmd, "pier statusline") {
		return "claude statusLine: already configured — skipped", nil
	}
	cmd := self + " statusline claude"
	msg := "claude statusLine: wired"
	if strings.TrimSpace(existingCmd) != "" {
		cmd += " --wrap " + shellQuote(existingCmd)
		msg = "claude statusLine: wrapped existing command"
	}
	settings["statusLine"] = map[string]any{
		"type":    "command",
		"command": cmd,
	}

	if err := writeBackupOnce(path, data); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", err
	}
	if err := writeAtomicMode(path, append(out, '\n'), 0o644); err != nil {
		return "", err
	}
	return msg, nil
}

// AgyHooks merges pier's named hook block ("pier") into Antigravity CLI's
// global hooks.json (~/.gemini/config/hooks.json), preserving any other named
// hooks. A .bak-pier backup is written before the first modification.
func AgyHooks(path, self string) (string, error) {
	root := map[string]any{}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &root); err != nil {
			return "", fmt.Errorf("parse %s: %w", path, err)
		}
	case os.IsNotExist(err):
		// start from an empty hooks object
	default:
		return "", err
	}

	if existing, ok := root["pier"]; ok {
		if raw, _ := json.Marshal(existing); strings.Contains(string(raw), "pier hook") {
			return "agy hooks: already configured — skipped", nil
		}
	}

	handler := func(arg string) map[string]any {
		return map[string]any{
			"type":    "command",
			"command": self + " hook " + arg,
			"timeout": 5,
		}
	}
	root["pier"] = map[string]any{
		"SessionStart":  []any{handler("session-start")},
		"PreInvocation": []any{handler("user-prompt-submit")},
		"PreToolUse": []any{map[string]any{
			"matcher": "*",
			"hooks":   []any{handler("pre-tool-use")},
		}},
		"PostToolUse": []any{map[string]any{
			"matcher": "*",
			"hooks":   []any{handler("pre-tool-use")},
		}},
		"Stop": []any{handler("stop")},
	}

	if err := writeBackupOnce(path, data); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", err
	}
	if err := writeAtomicMode(path, append(out, '\n'), 0o600); err != nil {
		return "", err
	}
	return fmt.Sprintf("agy hooks: 5 event(s) wired (backup: %s.bak-pier)", filepath.Base(path)), nil
}

// AgyStatusLine configures Antigravity CLI's statusLine in
// ~/.gemini/antigravity-cli/settings.json, preserving stack_with_default and
// wrapping any existing user statusLine command. Written with 0600 permissions.
func AgyStatusLine(path, self string) (string, error) {
	root := map[string]any{}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &root); err != nil {
			return "", fmt.Errorf("parse %s: %w", path, err)
		}
	case os.IsNotExist(err):
	default:
		return "", err
	}

	sl, ok := root["statusLine"].(map[string]any)
	existingCmd, _ := sl["command"].(string)
	if strings.Contains(existingCmd, "pier statusline") {
		return "agy statusLine: already configured — skipped", nil
	}
	msg := "agy statusLine: wired"
	if ok && strings.TrimSpace(existingCmd) != "" {
		next := make(map[string]any, len(sl)+2)
		for k, v := range sl {
			next[k] = v
		}
		if _, hasType := next["type"]; !hasType {
			next["type"] = "command"
		}
		next["command"] = self + " statusline agy --wrap " + shellQuote(existingCmd)
		sl = next
		msg = "agy statusLine: wrapped existing command"
	} else {
		sl = map[string]any{
			"type":               "command",
			"command":            self + " statusline agy",
			"stack_with_default": true,
		}
	}
	root["statusLine"] = sl

	if err := writeBackupOnce(path, data); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", err
	}
	if err := writeAtomicMode(path, append(out, '\n'), 0o600); err != nil {
		return "", err
	}
	return msg, nil
}
