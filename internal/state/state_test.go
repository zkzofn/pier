package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestForEvent(t *testing.T) {
	cases := map[string]string{
		"user-prompt-submit": Working,
		"pre-tool-use":       Working,
		"stop":               Waiting,
		"permission-request": Permission,
		"session-start":      Waiting,
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

	payload := []byte(`{"session_id":"abc-123","cwd":"/dev/suite2","hook_event_name":"UserPromptSubmit","prompt":"fix the login bug"}`)
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
	if st.LastPrompt != "fix the login bug" {
		t.Errorf("LastPrompt = %q, want the submitted prompt", st.LastPrompt)
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

	// Tool use must not clobber the recorded prompt.
	if got := ReadAll(dir)["%42"].LastPrompt; got != "fix the login bug" {
		t.Errorf("LastPrompt after pre-tool-use = %q, want preserved", got)
	}

	// Transition to waiting.
	if err := Record(dir, "stop", payload); err != nil {
		t.Fatal(err)
	}
	if got := ReadAll(dir)["%42"].State; got != Waiting {
		t.Errorf("state = %q, want waiting", got)
	}
}

func TestRecordPromptUpdatesWithoutStateChange(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMUX_PANE", "%42")

	first := []byte(`{"session_id":"abc","prompt":"first question"}`)
	if err := Record(dir, "user-prompt-submit", first); err != nil {
		t.Fatal(err)
	}
	// A queued prompt submitted mid-turn: state stays working, prompt differs.
	second := []byte(`{"session_id":"abc","prompt":"second question"}`)
	if err := Record(dir, "user-prompt-submit", second); err != nil {
		t.Fatal(err)
	}
	if got := ReadAll(dir)["%42"].LastPrompt; got != "second question" {
		t.Errorf("LastPrompt = %q, want the newer prompt", got)
	}
}

func TestRecordIgnoresTaskNotifications(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMUX_PANE", "%42")

	if err := Record(dir, "user-prompt-submit", []byte(`{"session_id":"abc","prompt":"실제 지시"}`)); err != nil {
		t.Fatal(err)
	}
	notif := `{"session_id":"abc","prompt":"<task-notification>\nBackground task done\n</task-notification>"}`
	if err := Record(dir, "user-prompt-submit", []byte(notif)); err != nil {
		t.Fatal(err)
	}
	if got := ReadAll(dir)["%42"].LastPrompt; got != "실제 지시" {
		t.Errorf("LastPrompt = %q, want the user instruction kept", got)
	}
}

func TestRecordPromptCondensed(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMUX_PANE", "%42")

	long := strings.Repeat("한", 200)
	payload := []byte(`{"session_id":"abc","prompt":"\n\n  fix this:  \nsecond line\n` + long + `"}`)
	if err := Record(dir, "user-prompt-submit", payload); err != nil {
		t.Fatal(err)
	}
	if got := ReadAll(dir)["%42"].LastPrompt; got != "fix this:" {
		t.Errorf("LastPrompt = %q, want first non-empty line, trimmed", got)
	}

	payload = []byte(`{"session_id":"abc","prompt":"` + long + `"}`)
	if err := Record(dir, "user-prompt-submit", payload); err != nil {
		t.Fatal(err)
	}
	if got := []rune(ReadAll(dir)["%42"].LastPrompt); len(got) != 120 {
		t.Errorf("LastPrompt length = %d runes, want capped at 120", len(got))
	}
}

func TestLabel(t *testing.T) {
	cases := map[string]string{
		"fix the login bug":                    "fix the login bug",
		"\n\n  spaced  \nsecond":               "spaced",
		"<teammate-message id=\"x\">\ndo the research task\n</teammate-message>": "do the research task",
		"<teammate-message id=\"x\"> inline task":                               "inline task",
		"<command-name>/clear</command-name>":                                   "/clear",
		"<local-command-stdout></local-command-stdout>":                         "",
		"a < b and c > d":                                                       "a < b and c > d",
		"":                                                                      "",
	}
	for in, want := range cases {
		if got := Label(in); got != want {
			t.Errorf("Label(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRecordSessionStart(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMUX_PANE", "%42")

	prompt := []byte(`{"session_id":"old-session","prompt":"refactor the auth flow"}`)
	if err := Record(dir, "user-prompt-submit", prompt); err != nil {
		t.Fatal(err)
	}

	// Compaction is not a session boundary: nothing changes.
	compact := []byte(`{"session_id":"old-session","source":"compact"}`)
	if err := Record(dir, "session-start", compact); err != nil {
		t.Fatal(err)
	}
	st := ReadAll(dir)["%42"]
	if st.State != Working || st.LastPrompt != "refactor the auth flow" {
		t.Errorf("compact should be ignored, got %+v", st)
	}

	// /clear starts a new session: prompt blanks, fresh set — the sidebar
	// must not resurrect a title carried into the new transcript.
	clear := []byte(`{"session_id":"new-session","source":"clear"}`)
	if err := Record(dir, "session-start", clear); err != nil {
		t.Fatal(err)
	}
	st = ReadAll(dir)["%42"]
	if st.State != Waiting || st.SessionID != "new-session" || st.LastPrompt != "" || !st.Fresh {
		t.Errorf("clear should blank the prompt and mark fresh, got %+v", st)
	}

	// A resumed conversation keeps its transcript title (not fresh).
	resume := []byte(`{"session_id":"resumed","source":"resume"}`)
	if err := Record(dir, "session-start", resume); err != nil {
		t.Fatal(err)
	}
	if st = ReadAll(dir)["%42"]; st.Fresh {
		t.Errorf("resume should not be fresh, got %+v", st)
	}

	// The next real prompt takes over from fresh.
	if err := Record(dir, "user-prompt-submit", []byte(`{"session_id":"resumed","prompt":"다음 질문"}`)); err != nil {
		t.Fatal(err)
	}
	if st = ReadAll(dir)["%42"]; st.Fresh || st.LastPrompt != "다음 질문" {
		t.Errorf("prompt should clear fresh and set the label, got %+v", st)
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
