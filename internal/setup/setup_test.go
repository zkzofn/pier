package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const self = "/opt/homebrew/bin/pier"

func TestTmuxConfCreatesAndSkips(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".tmux.conf")

	msg, err := TmuxConf(path, self)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "added") {
		t.Errorf("want added, got %q", msg)
	}
	data, _ := os.ReadFile(path)
	for _, want := range []string{markerBegin, markerEnd, self + " ensure", self + ` status "#{pane_current_path}"`, "bind-key N display-popup"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("config missing %q", want)
		}
	}

	// Second run must not duplicate.
	msg, err = TmuxConf(path, self)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "skipped") {
		t.Errorf("want skipped, got %q", msg)
	}
	again, _ := os.ReadFile(path)
	if strings.Count(string(again), markerBegin) != 1 {
		t.Error("pier block duplicated")
	}
}

func TestTmuxConfPreservesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".tmux.conf")
	if err := os.WriteFile(path, []byte("set -g prefix C-a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := TmuxConf(path, self); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(data), "set -g prefix C-a\n") {
		t.Error("existing config not preserved at top")
	}
	if !strings.Contains(string(data), markerBegin) {
		t.Error("pier block not appended")
	}
}

func TestTmuxConfUpgradesManagedBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".tmux.conf")
	old := "set -g prefix C-a\n\n" +
		markerBegin + "\n" +
		`set-hook -g client-attached 'run-shell "` + self + ` ensure"'` + "\n" +
		markerEnd + "\n\n" +
		"set -g history-limit 9000\n"
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	msg, err := TmuxConf(path, self)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "updated") {
		t.Errorf("outdated block should be rewritten, got %q", msg)
	}
	data, _ := os.ReadFile(path)
	conf := string(data)
	if !strings.HasPrefix(conf, "set -g prefix C-a\n") || !strings.Contains(conf, "set -g history-limit 9000\n") {
		t.Error("config around the pier block not preserved")
	}
	if !strings.Contains(conf, "bind-key N display-popup") {
		t.Error("upgraded block missing the new binding")
	}
	if strings.Count(conf, markerBegin) != 1 || strings.Count(conf, markerEnd) != 1 {
		t.Error("markers duplicated on upgrade")
	}

	// Now current: a further run must not rewrite.
	msg, err = TmuxConf(path, self)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "skipped") {
		t.Errorf("up-to-date block should be skipped, got %q", msg)
	}
}

func TestTmuxConfSkipsManualSetup(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".tmux.conf")
	manual := `set-hook -g client-attached 'run-shell "~/.local/bin/pier ensure"'` + "\n"
	if err := os.WriteFile(path, []byte(manual), 0o644); err != nil {
		t.Fatal(err)
	}
	msg, err := TmuxConf(path, self)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "skipped") {
		t.Errorf("manual setup should be detected, got %q", msg)
	}
}

func TestClaudeHooksFromScratch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	msg, err := ClaudeHooks(path, self)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "5 event(s)") {
		t.Errorf("want 5 events wired, got %q", msg)
	}
	var s map[string]any
	data, _ := os.ReadFile(path)
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	hooks := s["hooks"].(map[string]any)
	for _, ev := range []string{"UserPromptSubmit", "PreToolUse", "Stop", "PermissionRequest", "SessionStart"} {
		raw, _ := json.Marshal(hooks[ev])
		if !strings.Contains(string(raw), "pier hook") {
			t.Errorf("event %s not wired: %s", ev, raw)
		}
	}
	// matcher events carry "*"
	raw, _ := json.Marshal(hooks["PreToolUse"])
	if !strings.Contains(string(raw), `"matcher":"*"`) {
		t.Errorf("PreToolUse missing matcher: %s", raw)
	}
}

func TestClaudeHooksMergesAndBacksUp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	existing := `{
  "model": "claude-fable-5",
  "hooks": {
    "Stop": [{"hooks": [{"type": "command", "command": "/bin/echo bye", "timeout": 3}]}]
  }
}`
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ClaudeHooks(path, self); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	var s map[string]any
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	if s["model"] != "claude-fable-5" {
		t.Error("unrelated settings key lost")
	}
	stops, _ := json.Marshal(s["hooks"].(map[string]any)["Stop"])
	if !strings.Contains(string(stops), "/bin/echo bye") {
		t.Error("existing Stop hook lost")
	}
	if !strings.Contains(string(stops), "pier hook stop") {
		t.Error("pier Stop hook not merged")
	}
	if _, err := os.Stat(path + ".bak-pier"); err != nil {
		t.Error("backup not written")
	}

	// Idempotent on re-run.
	msg, err := ClaudeHooks(path, self)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "skipped") {
		t.Errorf("want skipped on re-run, got %q", msg)
	}
}
