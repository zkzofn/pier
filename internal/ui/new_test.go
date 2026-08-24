package ui

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"pier/internal/newsess"
	"pier/internal/resume"
	"pier/internal/state"
	"pier/internal/tmux"
)

var (
	testOpens = []newsess.Open{
		{Session: "suite2", Path: "/Users/j/dev/suite2", Pane: tmux.Pane{ID: "%1", Command: "claude"}},
		{Session: "recoder", Path: "/Users/j/dev/recoder", Pane: tmux.Pane{ID: "%2", Command: "zsh"}},
	}
	testDirs = []newsess.Candidate{
		{Path: "/Users/j/dev/pier", Name: "pier"},
		{Path: "/Users/j/dev/suite3", Name: "suite3"},
	}
	testCasualties = map[string]resume.Casualty{
		"/Users/j/dev/suite3": {SID: "sid-3", CWD: "/Users/j/dev/suite3", LastActive: time.Now()},
	}
)

func testPicker(opens []newsess.Open, dirs []newsess.Candidate, cas map[string]resume.Casualty) pickerModel {
	m := pickerModel{
		home:       "/Users/j",
		all:        dirs,
		opens:      opens,
		states:     map[string]state.PaneState{"%1": {Pane: "%1", State: state.Working}},
		taken:      map[string]bool{},
		casualties: cas,
		width:      46,
		height:     18,
	}
	m.refilter()
	return m
}

func kinds(m pickerModel) []pickerRowKind {
	ks := make([]pickerRowKind, 0, len(m.rows))
	for _, r := range m.rows {
		ks = append(ks, r.kind)
	}
	return ks
}

func TestPickerRowsOpenSessionsFirst(t *testing.T) {
	m := testPicker(testOpens, testDirs, testCasualties)
	want := []pickerRowKind{prRestoreAll, prOpen, prOpen, prSep, prDir, prDir}
	if got := kinds(m); !slices.Equal(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	if m.cursor != 1 {
		t.Errorf("cursor = %d, want the first open session (1), never restore-all", m.cursor)
	}
	if m.rows[1].open.Session != "suite2" || m.rows[4].cand.Name != "pier" {
		t.Error("rows carry the wrong payloads")
	}
}

func TestPickerRowsWithoutOpenSessions(t *testing.T) {
	m := testPicker(nil, testDirs, nil)
	if got := kinds(m); !slices.Equal(got, []pickerRowKind{prDir, prDir}) || m.cursor != 0 {
		t.Errorf("rows = %v cursor = %d, want two dirs, cursor 0", got, m.cursor)
	}
	m = testPicker(nil, testDirs, testCasualties)
	if got := kinds(m); !slices.Equal(got, []pickerRowKind{prRestoreAll, prDir, prDir}) || m.cursor != 1 {
		t.Errorf("rows = %v cursor = %d, want restore-all + dirs, cursor on the first dir", got, m.cursor)
	}
}

func TestPickerFilterAppliesToBothGroups(t *testing.T) {
	m := testPicker(testOpens, testDirs, testCasualties)
	m.query = "suite"
	m.refilter()
	if got := kinds(m); !slices.Equal(got, []pickerRowKind{prOpen, prSep, prDir}) {
		t.Fatalf("rows = %v, want open suite2 · sep · dir suite3 (restore-all hidden while filtering)", got)
	}
	m.query = "recoder"
	m.refilter()
	if got := kinds(m); !slices.Equal(got, []pickerRowKind{prOpen}) {
		t.Errorf("rows = %v, want only the open session, no separator", got)
	}
	m.query = "zzz"
	m.refilter()
	if len(m.rows) != 0 || m.manual != nil {
		t.Errorf("no match and not a path: rows = %v manual = %v", kinds(m), m.manual)
	}
	m.query = "~/brand-new"
	m.refilter()
	if got := kinds(m); !slices.Equal(got, []pickerRowKind{prDir}) || !m.rows[0].cand.Manual || !m.rows[0].cand.NeedsMkdir {
		t.Errorf("a path query with no match offers the manual candidate: %v", got)
	}
}

// ^S must keep its promise on every row: on a running session it opens a
// fresh terminal session in that session's directory — never a jump. The
// cursor boots on an open row whenever one exists, so a jump here sent
// "new session → ^S" ping-ponging between running sessions (v1.0.0).
func TestPickerShellOnOpenRowOpensTerminalThere(t *testing.T) {
	var gotName, gotPath, gotCmd string
	created := false
	tmuxNewDetached = func(name, path, command string) error {
		created, gotName, gotPath, gotCmd = true, name, path, command
		return nil
	}
	defer func() { tmuxNewDetached = tmux.NewSessionDetached }()

	m := testPicker(testOpens, testDirs, nil)
	m.taken = map[string]bool{"suite2": true, "recoder": true}
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	fm := model.(pickerModel)
	if !created {
		t.Fatalf("^S on an open-session row must create a terminal session, not jump (attach=%q)", fm.attach)
	}
	if gotName != "suite2-2" || gotPath != "/Users/j/dev/suite2" || gotCmd != cmdShell {
		t.Errorf("created %q at %q running %q, want suite2-2 at /Users/j/dev/suite2 running the default shell",
			gotName, gotPath, gotCmd)
	}
	if fm.attach != "suite2-2" {
		t.Errorf("standalone picker should attach to the new terminal, got %q", fm.attach)
	}
}

func TestPickerEnterOnOpenRowJumps(t *testing.T) {
	m := testPicker(testOpens, testDirs, nil)
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if fm := model.(pickerModel); fm.attach != "suite2" {
		t.Errorf("Enter on an open row should jump back into it, attach = %q, want suite2", fm.attach)
	}
}

func TestPickerMoveSkipsSeparator(t *testing.T) {
	m := testPicker(testOpens, testDirs, nil) // [open open sep dir dir]
	m.cursor = 1
	m.move(1)
	if m.cursor != 3 {
		t.Errorf("down from the last open row should land on the first dir (3), got %d", m.cursor)
	}
	m.move(-1)
	if m.cursor != 1 {
		t.Errorf("up from the first dir should land on the last open row (1), got %d", m.cursor)
	}
	m.move(-5)
	if m.cursor != 1 {
		t.Errorf("moving past the top is a no-op, got %d", m.cursor)
	}
}

func TestPickerViewFitsAndShowsOpenRows(t *testing.T) {
	m := testPicker(testOpens, testDirs, nil)
	v := m.View()
	for _, line := range strings.Split(v, "\n") {
		if w := lipgloss.Width(line); w > 45 {
			t.Errorf("line %d cols wide, over 45: %q", w, line)
		}
	}
	if !strings.Contains(v, "● ") || !strings.Contains(v, "suite2") || !strings.Contains(v, "dev/suite2") {
		t.Errorf("open row should show icon, session name and path:\n%s", v)
	}
	if !strings.Contains(v, "$ ") || !strings.Contains(v, "recoder") {
		t.Errorf("terminal session should show the $ icon:\n%s", v)
	}
	if !strings.Contains(v, "^S: terminal") {
		t.Errorf("hint should say terminal:\n%s", v)
	}
}
