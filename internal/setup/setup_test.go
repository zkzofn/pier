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
	for _, want := range []string{markerBegin, markerEnd, self + " ensure", self + ` status "#{pane_current_path}"`, "bind-key N display-popup", "set -g detach-on-destroy off", self + ` reap"'`} {
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
	if !strings.Contains(msg, "6 event(s)") {
		t.Errorf("want 6 events wired, got %q", msg)
	}
	var s map[string]any
	data, _ := os.ReadFile(path)
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	hooks := s["hooks"].(map[string]any)
	for _, ev := range []string{"UserPromptSubmit", "PreToolUse", "Stop", "PermissionRequest", "SessionStart", "SessionEnd"} {
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

func TestUpsertBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fish", "config.fish") // parent dir missing
	block := markerBegin + "\nalias cl='pier'\n" + markerEnd + "\n"

	st, err := UpsertBlock(path, block)
	if err != nil || st != BlockAdded {
		t.Fatalf("first write: %v %v", st, err)
	}
	if st, _ := UpsertBlock(path, block); st != BlockUnchanged {
		t.Errorf("same block again should be unchanged, got %v", st)
	}
	// content around the block survives an update
	data, _ := os.ReadFile(path)
	os.WriteFile(path, []byte("set -x FOO 1\n\n"+string(data)+"\nfunction later\nend\n"), 0o644)
	st, err = UpsertBlock(path, markerBegin+"\nalias zz='pier'\n"+markerEnd+"\n")
	if err != nil || st != BlockUpdated {
		t.Fatalf("update: %v %v", st, err)
	}
	got, _ := os.ReadFile(path)
	conf := string(got)
	if !strings.HasPrefix(conf, "set -x FOO 1\n") || !strings.HasSuffix(conf, "function later\nend\n") {
		t.Errorf("content outside the block not preserved:\n%s", conf)
	}
	if strings.Contains(conf, "alias cl=") || !strings.Contains(conf, "alias zz='pier'") || strings.Count(conf, markerBegin) != 1 {
		t.Errorf("block not replaced in place:\n%s", conf)
	}
}

func TestShellAliasAndAliasIn(t *testing.T) {
	rc := filepath.Join(t.TempDir(), ".zshrc")
	os.WriteFile(rc, []byte("export EDITOR=vim\n"), 0o644)
	if _, err := ShellAlias(rc, "cl", "pier"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(rc)
	if !strings.Contains(string(data), "\n"+markerBegin+"\nalias cl='pier'\n"+markerEnd+"\n") {
		t.Errorf("alias block wrong:\n%s", data)
	}
	if got := AliasIn(rc); got != "cl" {
		t.Errorf("AliasIn = %q, want cl", got)
	}
	ShellAlias(rc, "pp", "/Users/me/.local/bin/pier")
	if got := AliasIn(rc); got != "pp" {
		t.Errorf("AliasIn after replace = %q, want pp", got)
	}
	if got := AliasIn(filepath.Join(t.TempDir(), "none")); got != "" {
		t.Errorf("missing file -> no alias, got %q", got)
	}
}

func TestNeedsSetup(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, ".tmux.conf")
	settings := filepath.Join(dir, "settings.json")
	if !NeedsSetup(conf, settings) {
		t.Error("nothing written yet -> needs setup")
	}
	TmuxConf(conf, self)
	if !NeedsSetup(conf, settings) {
		t.Error("tmux only -> still needs hooks")
	}
	ClaudeHooks(settings, self)
	if NeedsSetup(conf, settings) {
		t.Error("both wired -> no setup needed")
	}
	// a hand-written (unmanaged) tmux config counts as configured
	os.WriteFile(conf, []byte(`set-hook -g client-attached 'run-shell "~/.local/bin/pier ensure"'`+"\n"), 0o644)
	if NeedsSetup(conf, settings) {
		t.Error("unmanaged pier config + hooks -> no setup needed")
	}
	os.Remove(settings)
	if !NeedsSetup(conf, settings) {
		t.Error("hooks only missing -> needs setup")
	}
}

func TestRefreshTmuxBlock(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, ".tmux.conf")
	oldBin := filepath.Join(dir, "old", "pier")
	os.MkdirAll(filepath.Dir(oldBin), 0o755)
	os.WriteFile(oldBin, []byte("#!/bin/sh\n"), 0o755)
	devBin := filepath.Join(dir, "dev", "pier")
	outdated := "set -g prefix C-a\n\n" + markerBegin + "\n" +
		`set-hook -g client-attached 'run-shell "` + oldBin + ` ensure"'` + "\n" + markerEnd + "\n"

	// outdated block, its binary still exists -> refreshed, path kept
	os.WriteFile(conf, []byte(outdated), 0o644)
	msg, err := RefreshTmuxBlock(conf, devBin)
	if err != nil || msg == "" {
		t.Fatalf("outdated block should be refreshed: %q %v", msg, err)
	}
	data, _ := os.ReadFile(conf)
	if !strings.Contains(string(data), oldBin+" reap") || strings.Contains(string(data), devBin) {
		t.Errorf("refresh must keep the existing binary path:\n%s", data)
	}
	// up to date now, different binary running -> untouched
	before, _ := os.ReadFile(conf)
	if msg, _ := RefreshTmuxBlock(conf, devBin); msg != "" {
		t.Errorf("current block must not be rewritten for a path change, got %q", msg)
	}
	if after, _ := os.ReadFile(conf); string(after) != string(before) {
		t.Error("file changed although the block was current")
	}
	// the recorded binary is gone -> the running one takes over
	os.Remove(oldBin)
	os.WriteFile(conf, []byte(outdated), 0o644)
	if msg, _ := RefreshTmuxBlock(conf, devBin); msg == "" {
		t.Fatal("outdated block with a dead path should be refreshed")
	}
	data, _ = os.ReadFile(conf)
	if !strings.Contains(string(data), devBin+" ensure") {
		t.Errorf("dead path should be replaced by self:\n%s", data)
	}
	// unmanaged or missing -> no-op
	manual := `set-hook -g client-attached 'run-shell "~/.local/bin/pier ensure"'` + "\n"
	os.WriteFile(conf, []byte(manual), 0o644)
	if msg, _ := RefreshTmuxBlock(conf, devBin); msg != "" {
		t.Errorf("unmanaged config must be left alone, got %q", msg)
	}
	if got, _ := os.ReadFile(conf); string(got) != manual {
		t.Error("unmanaged config was modified")
	}
	if msg, err := RefreshTmuxBlock(filepath.Join(dir, "nope"), devBin); msg != "" || err != nil {
		t.Errorf("missing file -> nothing: %q %v", msg, err)
	}
}
