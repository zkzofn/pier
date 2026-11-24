package tmux

import "testing"

func TestPickMostRecentClient(t *testing.T) {
	out := "/dev/ttys002\tterminal\t1784176097\n" +
		"/dev/ttys035\tterminal\t1784179999\n" + // clicked just now
		"/dev/ttys011\tweb\t1784180500\n" + // other session, ignore
		"bad\tline\n"

	if got := PickMostRecentClient(out, "terminal"); got != "/dev/ttys035" {
		t.Errorf("want most recent terminal client, got %q", got)
	}
	if got := PickMostRecentClient(out, "web"); got != "/dev/ttys011" {
		t.Errorf("want web client, got %q", got)
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

func TestParseStatusBg(t *testing.T) {
	cases := []struct {
		statusStyle      string
		statusRightStyle string
		want             string
	}{
		{"bg=green,fg=black", "default", "green"},
		{"bg=colour235,fg=white", "default", "colour235"},
		{"bg=green,fg=black", "bg=black", "black"},
		{"", "", ""},
	}
	for _, tc := range cases {
		if got := ParseStatusBg(tc.statusStyle, tc.statusRightStyle); got != tc.want {
			t.Errorf("ParseStatusBg(%q, %q) = %q, want %q", tc.statusStyle, tc.statusRightStyle, got, tc.want)
		}
	}
}

func TestIsGreenBg(t *testing.T) {
	for _, bg := range []string{"green", "colour2", "color2", "brightgreen", "colour10", "color10", " GREEN "} {
		if !IsGreenBg(bg) {
			t.Errorf("IsGreenBg(%q) = false, want true", bg)
		}
	}
	for _, bg := range []string{"black", "colour235", "default", "", "#1e1e2e"} {
		if IsGreenBg(bg) {
			t.Errorf("IsGreenBg(%q) = true, want false", bg)
		}
	}
}
