package newsess

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"pier/internal/tmux"
)

func TestCollectLeavesOpenPathsToTheSessionList(t *testing.T) {
	home := t.TempDir()
	for _, d := range []string{"dev/suite1", "dev/wishi", "dev/.hidden"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	panes := []tmux.Pane{
		{Session: "suite1", ID: "%1", Command: "claude", Path: filepath.Join(home, "dev", "suite1")},
		{Session: "sb", ID: "%2", Command: "pier", Path: filepath.Join(home, "dev", "wishi"), Sidebar: true},
	}
	cands := Collect(home, panes)

	byPath := map[string]Candidate{}
	for _, c := range cands {
		byPath[c.Path] = c
	}
	if _, ok := byPath[filepath.Join(home, "dev", "suite1")]; ok {
		t.Error("a directory hosting a live pane is listed as an open session, not a directory")
	}
	if _, ok := byPath[filepath.Join(home, "dev", "wishi")]; !ok {
		t.Error("a sidebar pane's path does not count as open")
	}
	if _, ok := byPath[filepath.Join(home, "dev", ".hidden")]; ok {
		t.Error("hidden dirs should be skipped")
	}
	if _, ok := byPath[home]; !ok {
		t.Error("home dir should be a candidate")
	}
}

func TestOpenSessions(t *testing.T) {
	panes := []tmux.Pane{
		{Session: "zeta", ID: "%1", Command: "claude", Path: "/Users/j/dev/zeta", WindowActive: true, PaneActive: true},
		{Session: "alpha", ID: "%2", Command: "zsh", Path: "/Users/j/dev/alpha", WindowActive: true, PaneActive: true},
		{Session: "alpha", ID: "%3", Command: "pier", Path: "/Users/j", Sidebar: true},
		{Session: "lonely", ID: "%4", Command: "pier", Path: "/Users/j", Sidebar: true},
		{Session: "me", ID: "%5", Command: "claude", Path: "/Users/j/dev/me", WindowActive: true, PaneActive: true},
	}
	names := func(opens []Open) []string {
		var out []string
		for _, o := range opens {
			out = append(out, o.Session)
		}
		return out
	}

	// the session the picker was opened from leads, flagged Here; the rest
	// keep sidebar (path) order
	got := OpenSessions(panes, "me")
	if want := []string{"me", "alpha", "zeta"}; !slices.Equal(names(got), want) {
		t.Fatalf("want %v, got %+v", want, got)
	}
	if !got[0].Here || got[1].Here || got[2].Here {
		t.Errorf("only the current session is Here: %+v", got)
	}
	if got[1].Path != "/Users/j/dev/alpha" || got[1].Pane.ID != "%2" {
		t.Errorf("alpha should carry its representative pane: %+v", got[1])
	}

	// no current session (standalone): plain sidebar order, nothing marked
	got = OpenSessions(panes, "")
	if want := []string{"alpha", "me", "zeta"}; !slices.Equal(names(got), want) || got[0].Here || got[1].Here || got[2].Here {
		t.Errorf("without a current session: want %v, none Here, got %+v", want, got)
	}

	// a current session holding only a sidebar pane isn't a row at all
	if got := OpenSessions(panes, "lonely"); !slices.Equal(names(got), []string{"alpha", "me", "zeta"}) {
		t.Errorf("sidebar-only current session: want [alpha me zeta], got %+v", got)
	}
	if got := OpenSessions(nil, ""); len(got) != 0 {
		t.Errorf("no panes -> no sessions, got %+v", got)
	}
}

func TestFilterOpen(t *testing.T) {
	opens := []Open{{Session: "suite2", Path: "/Users/j/dev/suite2"}, {Session: "recoder", Path: "/Users/j/dev/recoder"}}
	if got := FilterOpen(opens, ""); len(got) != 2 {
		t.Errorf("empty query keeps all, got %d", len(got))
	}
	if got := FilterOpen(opens, "SUITE"); len(got) != 1 || got[0].Session != "suite2" {
		t.Errorf("case-insensitive name match failed: %+v", got)
	}
	if got := FilterOpen(opens, "dev/rec"); len(got) != 1 || got[0].Session != "recoder" {
		t.Errorf("path match failed: %+v", got)
	}
	if got := FilterOpen(opens, "nope"); len(got) != 0 {
		t.Errorf("no match should be empty, got %+v", got)
	}
}

func TestFilter(t *testing.T) {
	cands := []Candidate{
		{Path: "/Users/j/dev/suite1", Name: "suite1"},
		{Path: "/Users/j/dev/wishi", Name: "wishi"},
	}
	if got := Filter(cands, ""); len(got) != 2 {
		t.Errorf("empty query should keep all, got %d", len(got))
	}
	if got := Filter(cands, "SUITE"); len(got) != 1 || got[0].Name != "suite1" {
		t.Errorf("case-insensitive filter failed: %+v", got)
	}
	if got := Filter(cands, "nope"); len(got) != 0 {
		t.Errorf("no-match should be empty, got %+v", got)
	}
}

func TestManual(t *testing.T) {
	home := t.TempDir()
	sub := filepath.Join(home, "proj.v2")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	c := Manual(home, "~/proj.v2")
	if c == nil || c.Path != sub || c.Name != "proj-v2" || !c.Manual || c.NeedsMkdir {
		t.Errorf("tilde manual candidate wrong: %+v", c)
	}
	if c := Manual(home, "~/missing/nested"); c == nil || !c.NeedsMkdir || c.Name != "nested" {
		t.Errorf("nonexistent dir should be offered with NeedsMkdir: %+v", c)
	}
	file := filepath.Join(home, "afile")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if Manual(home, "~/afile") != nil {
		t.Error("existing file must not produce a candidate")
	}
	if Manual(home, "relative/path") != nil {
		t.Error("relative paths are not accepted")
	}
}

func TestSanitizeAndUnique(t *testing.T) {
	if got := Sanitize("proj.v2:x"); got != "proj-v2-x" {
		t.Errorf("Sanitize = %q", got)
	}
	if got := Sanitize("..."); got != "session" {
		t.Errorf("all-reserved name should fall back: %q", got)
	}
	taken := map[string]bool{"suite5": true, "suite5-2": true}
	if got := Unique("suite5", taken); got != "suite5-3" {
		t.Errorf("Unique = %q, want suite5-3", got)
	}
	if got := Unique("fresh", taken); got != "fresh" {
		t.Errorf("Unique = %q, want fresh", got)
	}
}

// A pane's cwd and the directory pier scans can name the same physical
// directory through different paths — the home dir itself may be reached
// through a symlink (/tmp vs /private/tmp on macOS), so the scanned string
// and pane_current_path differ. Such a directory is open, not a candidate.
func TestCollectResolvesSymlinkedOpenPaths(t *testing.T) {
	base := t.TempDir()
	realHome := filepath.Join(base, "realhome")
	if err := os.MkdirAll(filepath.Join(realHome, "dev", "proj"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(realHome, "dev", "other"), 0o755); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(base, "home") // reached through a symlink
	if err := os.Symlink(realHome, home); err != nil {
		t.Fatal(err)
	}

	paths := func(cands []Candidate) map[string]bool {
		out := map[string]bool{}
		for _, c := range cands {
			out[c.Path] = true
		}
		return out
	}

	// scanned through the link, pane reports the resolved path
	got := paths(Collect(home, []tmux.Pane{
		{Session: "s", ID: "%1", Command: "claude", Path: filepath.Join(realHome, "dev", "proj")},
	}))
	if got[filepath.Join(home, "dev", "proj")] {
		t.Error("a directory already hosting a pane must not be offered under its other name")
	}
	if !got[filepath.Join(home, "dev", "other")] {
		t.Error("unrelated directories must still be candidates")
	}

	// and the other way round: scanned directly, pane reports the linked path
	got = paths(Collect(realHome, []tmux.Pane{
		{Session: "s", ID: "%1", Command: "claude", Path: filepath.Join(home, "dev", "proj")},
	}))
	if got[filepath.Join(realHome, "dev", "proj")] {
		t.Error("the pane's directory must not be offered under its resolved name either")
	}
}
