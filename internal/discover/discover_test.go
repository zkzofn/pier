package discover

import (
	"testing"

	"pier/internal/tmux"
)

func TestIsClaudeCommand(t *testing.T) {
	cases := map[string]bool{
		"2.1.206":  true, // observed live: CC process title is its version
		"2.1.211":  true,
		"claude":   true,
		"1.0":      true,
		"3.6a":     true, // tmux-style version suffix
		"zsh":      false,
		"node":     false,
		"nvim":     false,
		"pier":     false,
		"2.1.206.": false,
		"v2.1.206": false,
		"":         false,
		"go1.26.5": false,
	}
	for cmd, want := range cases {
		if got := IsClaudeCommand(cmd); got != want {
			t.Errorf("IsClaudeCommand(%q) = %v, want %v", cmd, got, want)
		}
	}
}

func fakeBranch(path string) string { return "br:" + path }

func TestGroup(t *testing.T) {
	panes := []tmux.Pane{
		{Session: "suite2", ID: "%1", Command: "2.1.202", Path: "/dev/suite2"},
		{Session: "suite1", ID: "%2", Command: "2.1.206", Path: "/dev/suite1"},
		{Session: "suite2", ID: "%3", Command: "zsh", Path: "/dev/suite2"},               // not CC
		{Session: "suite2", ID: "%4", Command: "pier", Path: "/dev/suite2"},              // not CC
		{Session: "suite2", ID: "%5", Command: "claude", Path: "/dev/suite2"},            // 2nd CC in suite2
		{Session: "x", ID: "%6", Command: "2.1.211", Path: "/dev/suite2", Sidebar: true}, // marked sidebar
	}
	got := Group(panes, fakeBranch)
	if len(got) != 2 {
		t.Fatalf("want 2 worktrees, got %d: %+v", len(got), got)
	}
	if got[0].Path != "/dev/suite1" || got[1].Path != "/dev/suite2" {
		t.Fatalf("wrong path order: %+v", got)
	}
	if got[1].Branch != "br:/dev/suite2" {
		t.Errorf("branch func not applied: %+v", got[1])
	}
	if len(got[1].Instances) != 2 {
		t.Fatalf("suite2 should have 2 CC panes, got %+v", got[1].Instances)
	}
	if got[1].Instances[0].ID != "%1" || got[1].Instances[1].ID != "%5" {
		t.Errorf("instances not sorted by session/id: %+v", got[1].Instances)
	}
}

func TestGroupShellSessions(t *testing.T) {
	panes := []tmux.Pane{
		// CC session: its extra zsh pane must NOT produce a shell entry
		{Session: "suite1", ID: "%1", Command: "claude", Path: "/dev/suite1", WindowActive: true, PaneActive: true},
		{Session: "suite1", ID: "%2", Command: "zsh", Path: "/dev/suite1", WindowActive: true},
		// shell session: the focused pane represents it, not another window's
		{Session: "term", ID: "%3", Command: "zsh", Path: "/home", PaneActive: true},
		{Session: "term", ID: "%4", Command: "nvim", Path: "/dev/notes", WindowActive: true, PaneActive: true},
		{Session: "term", ID: "%5", Command: "pier", Path: "/home", Sidebar: true, WindowActive: true},
		// shell session with focus on its sidebar: current-window pane wins
		{Session: "logs", ID: "%6", Command: "tail", Path: "/var/log", WindowActive: true},
		{Session: "logs", ID: "%7", Command: "pier", Path: "/var/log", Sidebar: true, WindowActive: true, PaneActive: true},
	}
	got := Group(panes, fakeBranch)
	if len(got) != 3 {
		t.Fatalf("want 3 worktrees, got %d: %+v", len(got), got)
	}
	if got[0].Path != "/dev/notes" || got[1].Path != "/dev/suite1" || got[2].Path != "/var/log" {
		t.Fatalf("wrong paths: %+v", got)
	}
	if len(got[0].Instances) != 1 || got[0].Instances[0].ID != "%4" {
		t.Errorf("term should be represented by its focused pane %%4: %+v", got[0].Instances)
	}
	if len(got[1].Instances) != 1 || got[1].Instances[0].ID != "%1" {
		t.Errorf("suite1 must keep only its CC pane: %+v", got[1].Instances)
	}
	if len(got[2].Instances) != 1 || got[2].Instances[0].ID != "%6" {
		t.Errorf("logs should fall back to its current-window pane: %+v", got[2].Instances)
	}

	order := SessionOrder(got)
	want := []string{"term", "suite1", "logs"}
	if len(order) != 3 || order[0] != want[0] || order[1] != want[1] || order[2] != want[2] {
		t.Errorf("shell sessions missing from session order: %v, want %v", order, want)
	}
}

func TestParsePanes(t *testing.T) {
	out := "suite2\t0\t0\t%1\t2.1.202\t/Users/j/dev/suite2\t1\t1\t\n" +
		"term\t0\t1\t%9\tpier\t/Users/j\t1\t0\t1\n" +
		"short\tline\n" +
		// Last line with its trailing tab stripped (observed live: TrimSpace
		// ate the empty @pier field and pane %23 vanished from the list).
		"terminal\t0\t1\t%23\t2.1.211\t/Users/j\t0\t1"
	panes := tmux.ParsePanes(out)
	if len(panes) != 3 {
		t.Fatalf("want 3 panes, got %d: %+v", len(panes), panes)
	}
	if panes[0].ID != "%1" || panes[0].Sidebar || !panes[0].WindowActive || !panes[0].PaneActive {
		t.Errorf("pane 0 wrong: %+v", panes[0])
	}
	if !panes[1].Sidebar || !panes[1].WindowActive || panes[1].PaneActive {
		t.Errorf("pane 1 should be inactive sidebar in active window: %+v", panes[1])
	}
	if panes[2].ID != "%23" || panes[2].Sidebar || panes[2].WindowActive || !panes[2].PaneActive {
		t.Errorf("last pane with stripped trailing field wrong: %+v", panes[2])
	}
}

func TestSessionOrderAndNext(t *testing.T) {
	panes := []tmux.Pane{
		{Session: "beta", ID: "%2", Command: "claude", Path: "/dev/b"},
		{Session: "alpha", ID: "%1", Command: "claude", Path: "/dev/a"},
		{Session: "alpha", ID: "%3", Command: "claude", Path: "/dev/a"}, // dup pane, same session
		{Session: "gamma", ID: "%4", Command: "claude", Path: "/dev/c"},
	}
	order := SessionOrder(Group(panes, func(string) string { return "" }))
	want := []string{"alpha", "beta", "gamma"}
	if len(order) != 3 || order[0] != want[0] || order[1] != want[1] || order[2] != want[2] {
		t.Fatalf("order = %v, want %v", order, want)
	}

	if got := NextSession(order, nil, "beta"); got != "gamma" {
		t.Errorf("next after beta = %q, want gamma", got)
	}
	if got := NextSession(order, nil, "gamma"); got != "alpha" {
		t.Errorf("next should wrap to alpha, got %q", got)
	}
	// Current session not in the sidebar: land on the sidebar's first.
	if got := NextSession(order, []string{"zeta", "alpha"}, "zeta"); got != "alpha" {
		t.Errorf("non-sidebar current should go to first sidebar session, got %q", got)
	}
	// No sidebar sessions: any other live session.
	if got := NextSession(nil, []string{"only", "other"}, "only"); got != "other" {
		t.Errorf("fallback to other live session, got %q", got)
	}
	// Alone: nothing to switch to.
	if got := NextSession([]string{"solo"}, []string{"solo"}, "solo"); got != "" {
		t.Errorf("alone should return empty, got %q", got)
	}
}

func TestRepresentative(t *testing.T) {
	panes := []tmux.Pane{
		// a focused shell split must not outrank the claude pane
		{Session: "s", ID: "%2", Command: "zsh", WindowActive: true, PaneActive: true},
		{Session: "s", ID: "%1", Command: "2.1.238", WindowActive: true},
		{Session: "s", ID: "%9", Command: "pier", Sidebar: true, WindowActive: true, PaneActive: true},
		// shell-only session: the focused pane wins
		{Session: "term", ID: "%4", Command: "zsh", WindowActive: true},
		{Session: "term", ID: "%5", Command: "zsh", WindowActive: true, PaneActive: true},
		// equal rank: the lower pane id wins
		{Session: "tie", ID: "%8", Command: "claude"},
		{Session: "tie", ID: "%7", Command: "claude"},
		// nothing but a sidebar
		{Session: "empty", ID: "%6", Command: "pier", Sidebar: true},
	}
	cases := map[string]string{"s": "%1", "term": "%5", "tie": "%7"}
	for session, want := range cases {
		rep, ok := Representative(panes, session)
		if !ok || rep.ID != want {
			t.Errorf("Representative(%s) = %v ok=%v, want %s", session, rep.ID, ok, want)
		}
	}
	if _, ok := Representative(panes, "empty"); ok {
		t.Error("a sidebar-only session has no representative")
	}
	if _, ok := Representative(panes, "nosuch"); ok {
		t.Error("unknown session has no representative")
	}
}
