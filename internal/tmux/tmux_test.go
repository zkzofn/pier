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
