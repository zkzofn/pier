package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestForEvent(t *testing.T) {
	cases := map[string]string{
		"user-prompt-submit": Working,
		"pre-tool-use":       Working,
		"stop":               Waiting,
		"permission-request": Permission,
		"unknown":            "",
	}
	for ev, want := range cases {
		if got := ForEvent(ev); got != want {
			t.Errorf("ForEvent(%q) = %q, want %q", ev, got, want)
		}
	}
}

func TestRecordAndReadAll(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMUX_PANE", "%42")

	payload := []byte(`{"session_id":"abc-123","cwd":"/dev/suite2","hook_event_name":"UserPromptSubmit"}`)
	if err := Record(dir, "user-prompt-submit", payload); err != nil {
		t.Fatal(err)
	}
	states := ReadAll(dir)
	st, ok := states["%42"]
	if !ok {
		t.Fatalf("no state for %%42: %v", states)
	}
	if st.State != Working || st.SessionID != "abc-123" || st.CWD != "/dev/suite2" {
		t.Errorf("wrong state: %+v", st)
	}

	// Same state again: file should not be rewritten (mtime noise guard).
	path := filepath.Join(dir, "42.json")
	before, _ := os.Stat(path)
	if err := Record(dir, "pre-tool-use", payload); err != nil { // also maps to working
		t.Fatal(err)
	}
	after, _ := os.Stat(path)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("unchanged state should skip the write")
	}

	// Transition to waiting.
	if err := Record(dir, "stop", payload); err != nil {
		t.Fatal(err)
	}
	if got := ReadAll(dir)["%42"].State; got != Waiting {
		t.Errorf("state = %q, want waiting", got)
	}
}

func TestRecordOutsideTmux(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMUX_PANE", "")
	if err := Record(dir, "stop", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Error("outside tmux should write nothing")
	}
}

func TestCleanup(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMUX_PANE", "%7")
	if err := Record(dir, "stop", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	Cleanup(dir, map[string]bool{"%7": true})
	if len(ReadAll(dir)) != 1 {
		t.Error("live pane state should survive cleanup")
	}
	Cleanup(dir, map[string]bool{})
	if len(ReadAll(dir)) != 0 {
		t.Error("dead pane state should be removed")
	}
}
