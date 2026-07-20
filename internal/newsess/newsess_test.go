package newsess

import (
	"os"
	"path/filepath"
	"testing"

	"pier/internal/tmux"
)

func TestCollect(t *testing.T) {
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
	if c, ok := byPath[filepath.Join(home, "dev", "suite1")]; !ok || c.Open != "suite1" {
		t.Errorf("suite1 should be collected and marked open: %+v", c)
	}
	if c, ok := byPath[filepath.Join(home, "dev", "wishi")]; !ok || c.Open != "" {
		t.Errorf("wishi should be collected, sidebar pane must not mark it open: %+v", c)
	}
	if _, ok := byPath[filepath.Join(home, "dev", ".hidden")]; ok {
		t.Error("hidden dirs should be skipped")
	}
	if _, ok := byPath[home]; !ok {
		t.Error("home dir should be a candidate")
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
