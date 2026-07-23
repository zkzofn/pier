package discover

import (
	"testing"

	"pier/internal/tmux"
)

func TestIsClaudeCommand(t *testing.T) {
	cases := map[string]bool{
		"2.1.206":    true, // observed live: CC process title is its version
		"2.1.211":    true,
		"claude":     true,
		"1.0":        true,
		"3.6a":       true, // tmux-style version suffix
		"zsh":        false,
		"node":       false,
		"nvim":       false,
		"pier":     false,
		"2.1.206.":   false,
		"v2.1.206":   false,
		"":           false,
		"go1.26.5":   false,
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
		{Session: "suite2", ID: "%3", Command: "zsh", Path: "/dev/suite2"},     // not CC
		{Session: "suite2", ID: "%4", Command: "pier", Path: "/dev/suite2"},  // not CC
		{Session: "suite2", ID: "%5", Command: "claude", Path: "/dev/suite2"},  // 2nd CC in suite2
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

func TestParsePanes(t *testing.T) {
	out := "suite2\t0\t0\t%1\t2.1.202\t/Users/j/dev/suite2\t\n" +
		"term\t0\t1\t%9\tpier\t/Users/j\t1\n" +
		"short\tline\n" +
		// Last line with its trailing tab stripped (observed live: TrimSpace
		// ate the empty @pier field and pane %23 vanished from the list).
		"terminal\t0\t1\t%23\t2.1.211\t/Users/j"
	panes := tmux.ParsePanes(out)
	if len(panes) != 3 {
		t.Fatalf("want 3 panes, got %d: %+v", len(panes), panes)
	}
	if panes[0].ID != "%1" || panes[0].Sidebar {
		t.Errorf("pane 0 wrong: %+v", panes[0])
	}
	if !panes[1].Sidebar {
		t.Errorf("pane 1 should be sidebar: %+v", panes[1])
	}
	if panes[2].ID != "%23" || panes[2].Sidebar {
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
