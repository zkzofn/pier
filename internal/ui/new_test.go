package ui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"pier/internal/claude"
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
	// inside tmux the picker was opened from session "me": it leads the open
	// rows, flagged Here
	testOpensHere = append([]newsess.Open{
		{Session: "me", Path: "/Users/j/dev/me", Pane: tmux.Pane{ID: "%3", Command: "claude"}, Here: true},
	}, testOpens...)
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

// Standalone (outside tmux) the picker is also how you get back into a
// session, so Enter on an open row attaches instead of creating.
func TestPickerEnterOnOpenRowStandaloneAttaches(t *testing.T) {
	m := testPicker(testOpens, testDirs, nil)
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if fm := model.(pickerModel); fm.attach != "suite2" {
		t.Errorf("Enter on an open row should attach to it, attach = %q, want suite2", fm.attach)
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

// The bottom hint teaches the picker's one non-obvious rule: keys act on
// the ▸ row. It must say what Enter/^S do to THAT row — a fixed line
// claimed "Enter: create" while the cursor sat on a running session.
func TestPickerHintFollowsCursorRow(t *testing.T) {
	m := testPicker(testOpens, testDirs, testCasualties)
	// rows: [restore-all, open suite2, open recoder, sep, dir pier, dir suite3]
	if v := m.View(); !strings.Contains(v, "Enter: jump · ^S: terminal there") {
		t.Errorf("open row: hint must explain jump/terminal-there:\n%s", v)
	}
	m.cursor = 4
	if v := m.View(); !strings.Contains(v, "Enter: claude · ^S: terminal") {
		t.Errorf("dir row: hint must explain claude/terminal:\n%s", v)
	}
	m.cursor = 5 // suite3 carries a casualty
	if v := m.View(); !strings.Contains(v, "→ resume") {
		t.Errorf("casualty row: hint must offer the resume arrow:\n%s", v)
	}
	m.onResume = true
	if v := m.View(); !strings.Contains(v, "Enter: resume") {
		t.Errorf("focused ↻ button: hint must say Enter resumes:\n%s", v)
	}
	m.onResume = false
	m.cursor = 0
	if v := m.View(); !strings.Contains(v, "Enter: restore all") {
		t.Errorf("restore-all row: hint must say what Enter does:\n%s", v)
	}
	m.editing = true
	if v := m.View(); !strings.Contains(v, "Enter: claude") {
		t.Errorf("name editor: hint should match the dir-row wording:\n%s", v)
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

// Inside tmux the picker is a new-session tool through and through: the
// session it was opened from leads the open rows (marked like the sidebar's
// current row), the other running sessions follow without a gap, and the
// cursor boots on the current one — prefix+N, Enter is "one more claude here".
func TestPickerCurrentSessionLeadsOpenRowsAndBootsTheCursor(t *testing.T) {
	m := testPicker(testOpensHere, testDirs, testCasualties)
	want := []pickerRowKind{prRestoreAll, prOpen, prOpen, prOpen, prSep, prDir, prDir}
	if got := kinds(m); !slices.Equal(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	if !m.rows[1].open.Here || m.rows[1].open.Session != "me" || m.rows[2].open.Session != "suite2" {
		t.Errorf("the current session leads, the others follow without a gap: %+v", m.rows[1:4])
	}
	if m.cursor != 1 {
		t.Errorf("cursor = %d, want the current session (1)", m.cursor)
	}
}

// Enter on an open row inside tmux starts another Claude Code session in
// that session's directory — never a switch (the sidebar does that). Names
// follow the directory, with the usual -2/-3 suffix past taken ones, so a
// third agent is "-3", not "-2-2".
func TestPickerEnterOnOpenRowInsideTmuxStartsAnotherClaudeThere(t *testing.T) {
	var gotName, gotPath, gotCmd string
	tmuxNewSwitch = func(name, path, command string) error {
		gotName, gotPath, gotCmd = name, path, command
		return nil
	}
	tmuxSwitchTo = func(string) error {
		t.Fatal("the picker must not switch sessions inside tmux")
		return nil
	}
	defer func() { tmuxNewSwitch, tmuxSwitchTo = tmux.NewSessionAndSwitch, tmux.SwitchTo }()

	m := testPicker(testOpensHere, testDirs, nil)
	m.insideTmux = true
	m.taken = map[string]bool{"me": true, "suite2": true, "recoder": true}
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // cursor boots on "me"
	if gotName != "me-2" || gotPath != "/Users/j/dev/me" || gotCmd != claude.Cmd(false) {
		t.Errorf("created %q at %q running %q, want me-2 at /Users/j/dev/me running claude", gotName, gotPath, gotCmd)
	}
	if fm := model.(pickerModel); fm.attach != "" {
		t.Errorf("inside tmux nothing is handed back to attach, got %q", fm.attach)
	}

	m.cursor = 1 // suite2
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if gotName != "suite2-2" || gotPath != "/Users/j/dev/suite2" {
		t.Errorf("another open row: created %q at %q, want suite2-2 at /Users/j/dev/suite2", gotName, gotPath)
	}

	// on a second agent's row ("suite1-2" at ~/dev/suite1) the third is suite1-3
	m = testPicker([]newsess.Open{{Session: "suite1-2", Path: "/Users/j/dev/suite1", Here: true}}, testDirs, nil)
	m.insideTmux = true
	m.taken = map[string]bool{"suite1": true, "suite1-2": true}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if gotName != "suite1-3" {
		t.Errorf("names follow the directory: got %q, want suite1-3", gotName)
	}
}

// Tab names the session Enter/^S will create — on open rows too, now that
// they create. Standalone, where Enter attaches, Tab stays a no-op there.
func TestPickerTabOnOpenRowInsideTmuxEditsTheName(t *testing.T) {
	var gotName, gotPath string
	tmuxNewSwitch = func(name, path, command string) error {
		gotName, gotPath = name, path
		return nil
	}
	defer func() { tmuxNewSwitch = tmux.NewSessionAndSwitch }()

	m := testPicker(testOpensHere, testDirs, nil)
	m.insideTmux = true
	m.taken = map[string]bool{"me": true}
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = model.(pickerModel)
	if !m.editing || m.name != "me-2" {
		t.Fatalf("Tab on the current row should propose me-2, got editing=%v name=%q", m.editing, m.name)
	}
	if v := m.View(); !strings.Contains(v, "path: /Users/j/dev/me") {
		t.Errorf("the editor shows the open row's directory:\n%s", v)
	}
	m.name = "me-review"
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if gotName != "me-review" || gotPath != "/Users/j/dev/me" {
		t.Errorf("created %q at %q, want me-review at /Users/j/dev/me", gotName, gotPath)
	}

	s := testPicker(testOpens, testDirs, nil) // standalone
	if model, _ := s.Update(tea.KeyMsg{Type: tea.KeyTab}); model.(pickerModel).editing {
		t.Error("standalone, Enter attaches — there is no name to edit on an open row")
	}
}

func TestPickerCurrentOpenRowIsMarkedLikeTheSidebar(t *testing.T) {
	m := testPicker(testOpensHere, testDirs, nil) // cursor on "me"
	v := m.View()
	if !strings.Contains(v, "▸ · me  /Users/j/dev/me") {
		t.Errorf("under the cursor the current row reads like any ▸ row:\n%s", v)
	}
	m.cursor = 1 // suite2
	v = m.View()
	if !strings.Contains(v, "▌ · me  /Users/j/dev/me") {
		t.Errorf("off the cursor the current row carries the sidebar's ▌ marker:\n%s", v)
	}
	if strings.Contains(v, "▌ ● suite2") || strings.Contains(v, "▌ $ recoder") {
		t.Errorf("only the current session is marked:\n%s", v)
	}
	for _, line := range strings.Split(v, "\n") {
		if w := lipgloss.Width(line); w > 45 {
			t.Errorf("line %d cols wide, over 45: %q", w, line)
		}
	}
}

// The ^T state shows only while it is on — a badge that is always there
// is noise — and spelled out: "tg" told nobody anything. The key itself is
// taught on the keys line, from the first frame.
func TestPickerTelegramBadgeOnlyWhenOn(t *testing.T) {
	telegramPreflight = func(string) error { return nil }
	defer func() { telegramPreflight = claude.TelegramPreflight }()
	m := testPicker(testOpensHere, testDirs, testCasualties)
	m.insideTmux = true
	head := func(v string) string { return strings.SplitN(v, "\n", 2)[0] }
	v := m.View()
	if strings.Contains(head(v), "telegram") {
		t.Errorf("fresh picker: no badge while off:\n%s", v)
	}
	if !strings.Contains(v, "^T: telegram") {
		t.Errorf("fresh picker: the keys line names ^T:\n%s", v)
	}
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	m = mm.(pickerModel)
	v = m.View()
	if !m.telegram || !strings.Contains(head(v), "telegram ✓") {
		t.Errorf("after ^T: the header carries the on-state:\n%s", v)
	}
	if strings.Contains(v, "tg✓") {
		t.Errorf("the abbreviation is gone:\n%s", v)
	}
	for _, line := range strings.Split(v, "\n") {
		if w := lipgloss.Width(line); w > 45 {
			t.Errorf("line %d cols wide, over 45: %q", w, line)
		}
	}
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	m = mm.(pickerModel)
	if v := m.View(); m.telegram || strings.Contains(v, "telegram ✓") {
		t.Errorf("^T again: badge gone:\n%s", v)
	}
}

// The second hint line carries the keys that mean the same on every row;
// Tab appears only where there is a session to name, and only there.
func TestPickerKeysLine(t *testing.T) {
	m := testPicker(testOpensHere, testDirs, testCasualties)
	m.insideTmux = true
	last := func(v string) string {
		lines := strings.Split(v, "\n")
		return lines[len(lines)-1]
	}
	// rows: [restore-all, open me (here), open suite2, open recoder, sep, dir pier, dir suite3]
	if l := last(m.View()); !strings.Contains(l, "^T: telegram") || !strings.Contains(l, "Tab: name") || !strings.Contains(l, "Esc: close") {
		t.Errorf("open row inside tmux creates, so Tab names: %q", l)
	}
	if v := m.View(); strings.Count(v, "Tab: name") != 1 {
		t.Errorf("Tab is named once, on the keys line:\n%s", v)
	}
	m.cursor = 0
	if l := last(m.View()); strings.Contains(l, "Tab: name") || !strings.Contains(l, "^T: telegram") {
		t.Errorf("restore-all row: nothing to name, ^T still applies: %q", l)
	}
	m.cursor = 5
	if l := last(m.View()); !strings.Contains(l, "Tab: name") {
		t.Errorf("dir row: Tab names: %q", l)
	}
	m.insideTmux = false
	m.cursor = 1
	if l := last(m.View()); strings.Contains(l, "Tab: name") {
		t.Errorf("standalone open row jumps, nothing to name: %q", l)
	}
	// A list longer than the popup gives up rows, never the hint lines: the
	// view is exactly the popup's height and still ends on the keys line.
	many := make([]newsess.Candidate, 0, 30)
	for i := 0; i < 30; i++ {
		many = append(many, newsess.Candidate{Path: fmt.Sprintf("/Users/j/dev/d%02d", i), Name: fmt.Sprintf("d%02d", i)})
	}
	tall := testPicker(nil, many, nil)
	if v := tall.View(); strings.Count(v, "\n")+1 != tall.height || !strings.Contains(last(v), "^T: telegram") {
		t.Errorf("%d lines for height %d, keys line last:\n%s", strings.Count(v, "\n")+1, tall.height, v)
	}
}

func TestPickerHintOnOpenRowsInsideTmux(t *testing.T) {
	m := testPicker(testOpensHere, testDirs, nil)
	m.insideTmux = true
	if v := m.View(); !strings.Contains(v, "Enter: new claude here · ^S: terminal here") {
		t.Errorf("current row: hint says Enter creates here:\n%s", v)
	}
	m.cursor = 1
	if v := m.View(); !strings.Contains(v, "Enter: new claude there · ^S: terminal there") {
		t.Errorf("another open row: hint says Enter creates there, not jumps:\n%s", v)
	}
	m.insideTmux = false
	if v := m.View(); !strings.Contains(v, "Enter: jump · ^S: terminal there") {
		t.Errorf("standalone: Enter still jumps (attaches):\n%s", v)
	}
}

// ^T asks before it promises: with the plugin off or no bot token, Claude
// Code would start a session that silently lacks telegram, so the toggle
// stays off and the error line names the fix. Enter and restore-all run
// the same check — nothing launches into such a session either. ^S opens
// a plain shell and never needed telegram.
func TestPickerTelegramPreflight(t *testing.T) {
	noToken := errors.New("no telegram token · /telegram:configure")
	telegramPreflight = func(string) error { return noToken }
	launched := 0
	tmuxNewSwitch = func(name, path, command string) error { launched++; return nil }
	tmuxNewDetached = func(name, path, command string) error { launched++; return nil }
	tmuxSwitchTo = func(string) error { return nil }
	defer func() {
		telegramPreflight = claude.TelegramPreflight
		tmuxNewSwitch, tmuxNewDetached, tmuxSwitchTo = tmux.NewSessionAndSwitch, tmux.NewSessionDetached, tmux.SwitchTo
	}()

	m := testPicker(testOpensHere, testDirs, testCasualties)
	m.insideTmux = true
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	m = mm.(pickerModel)
	v := m.View()
	if m.telegram || !strings.Contains(v, noToken.Error()) || strings.Contains(v, "telegram ✓") {
		t.Errorf("not ready: ^T stays off and the error line says why:\n%s", v)
	}
	for _, line := range strings.Split(v, "\n") {
		if w := lipgloss.Width(line); w > 45 {
			t.Errorf("line %d cols wide, over 45: %q", w, line)
		}
	}

	// ready now: ^T goes on and the error clears
	telegramPreflight = func(string) error { return nil }
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	m = mm.(pickerModel)
	if !m.telegram || m.errMsg != "" {
		t.Fatalf("ready: ^T turns on with no error; got on=%v err=%q", m.telegram, m.errMsg)
	}

	// the token vanished between the toggle and Enter: nothing launches
	telegramPreflight = func(string) error { return noToken }
	m.cursor = 5 // dir row: pier
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm.(pickerModel)
	if launched != 0 || m.errMsg != noToken.Error() {
		t.Errorf("Enter with a failing preflight: launched=%d err=%q", launched, m.errMsg)
	}
	m.cursor = 0 // restore all
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm.(pickerModel)
	if launched != 0 || m.errMsg != noToken.Error() {
		t.Errorf("restore-all with a failing preflight: launched=%d err=%q", launched, m.errMsg)
	}
	m.cursor = 5
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if launched != 1 {
		t.Errorf("^S is a plain shell, the preflight must not block it: launched=%d", launched)
	}

	// ^T off again: flag off, error gone
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	m = mm.(pickerModel)
	if m.telegram || m.errMsg != "" {
		t.Errorf("^T off: flag off and error cleared; got on=%v err=%q", m.telegram, m.errMsg)
	}
}
