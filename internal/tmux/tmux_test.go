package tmux

import "testing"

func TestPickMostRecentClient(t *testing.T) {
	out := "/dev/ttys002\tterminal\t1784176097\n" +
		"/dev/ttys035\tterminal\t1784179999\n" + // clicked just now
		"/dev/ttys011\tsuite2\t1784180500\n" + // other session, ignore
		"bad\tline\n"

	if got := PickMostRecentClient(out, "terminal"); got != "/dev/ttys035" {
		t.Errorf("want most recent terminal client, got %q", got)
	}
	if got := PickMostRecentClient(out, "suite2"); got != "/dev/ttys011" {
		t.Errorf("want suite2 client, got %q", got)
	}
	if got := PickMostRecentClient(out, "nosuch"); got != "" {
		t.Errorf("want empty for unattached session, got %q", got)
	}
	if got := PickMostRecentClient("", "terminal"); got != "" {
		t.Errorf("want empty for no clients, got %q", got)
	}
}

func TestSidebarOnlySessions(t *testing.T) {
	panes := []Pane{
		{Session: "gone", ID: "%1", Sidebar: true},
		{Session: "alive", ID: "%2", Sidebar: true},
		{Session: "alive", ID: "%3", Command: "zsh"},
		{Session: "bare", ID: "%4", Command: "claude"},
		{Session: "also-gone", ID: "%5", Sidebar: true},
	}
	got := SidebarOnlySessions(panes)
	if len(got) != 2 || got[0] != "also-gone" || got[1] != "gone" {
		t.Errorf("want [also-gone gone] (sorted), got %v", got)
	}
	if got := SidebarOnlySessions(nil); len(got) != 0 {
		t.Errorf("no panes -> nothing to reap, got %v", got)
	}
}
