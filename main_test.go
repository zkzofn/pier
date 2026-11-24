package main

import (
	"strings"
	"testing"
)

func TestFormatStatusRight(t *testing.T) {
	usageFrag := "#[fg=magenta]agy #[fg=green]5h:73%#[fg=colour244](2h34m)#[default]"

	// Non-green tmux theme: keep exact original output without dark pill.
	gotPlain := formatStatusRight(usageFrag, "~/dev/pier", "main", false)
	wantPlain := usageFrag + " #[fg=colour244]│#[default] #[fg=cyan]~/dev/pier #[fg=green]⎇ main#[default]"
	if gotPlain != wantPlain {
		t.Errorf("darkPill=false:\ngot:  %q\nwant: %q", gotPlain, wantPlain)
	}
	if strings.Contains(gotPlain, "#[bg=colour235]") {
		t.Errorf("darkPill=false must not wrap in bg pill: %q", gotPlain)
	}

	// Green tmux theme with usage + branch: dark pill, no inner #[default].
	gotPill := formatStatusRight(usageFrag, "~/dev/pier", "main", true)
	if !strings.HasPrefix(gotPill, "#[bg=colour235] ") || !strings.HasSuffix(gotPill, " #[default]") {
		t.Errorf("darkPill=true missing pill wrapper: %q", gotPill)
	}
	for _, want := range []string{"5h:73%", "⎇ main", "#[fg=default,nobold]"} {
		if !strings.Contains(gotPill, want) {
			t.Errorf("darkPill=true missing %q in %q", want, gotPill)
		}
	}
	if count := strings.Count(gotPill, "#[default]"); count != 1 {
		t.Errorf("darkPill=true has %d #[default] tags, want exactly 1 at end: %q", count, gotPill)
	}

	// Green tmux theme without usage (shell pane): with and without branch.
	gotNoBranch := formatStatusRight("", "~/dev/pier", "-", true)
	wantNoBranch := "#[bg=colour235] #[fg=cyan]~/dev/pier #[default]"
	if gotNoBranch != wantNoBranch {
		t.Errorf("darkPill=true no usage no branch:\ngot:  %q\nwant: %q", gotNoBranch, wantNoBranch)
	}

	gotWithBranch := formatStatusRight("", "~/dev/pier", "main", true)
	wantWithBranch := "#[bg=colour235] #[fg=cyan]~/dev/pier #[fg=green]⎇ main #[default]"
	if gotWithBranch != wantWithBranch {
		t.Errorf("darkPill=true no usage with branch:\ngot:  %q\nwant: %q", gotWithBranch, wantWithBranch)
	}
}
