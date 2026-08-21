package update

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const formula = `class Pier < Formula
  desc "Tmux sidebar dashboard for Claude Code sessions"
  version "0.12.0"
  url "https://github.com/zkzofn/pier/releases/download/v0.12.0/pier_v0.12.0_darwin_arm64.tar.gz"
end
`

func TestParseFormulaVersion(t *testing.T) {
	if got := ParseFormulaVersion(formula); got != "0.12.0" {
		t.Errorf("got %q", got)
	}
	if got := ParseFormulaVersion("no version here"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"0.12.0", "0.11.0", true}, {"0.11.0", "0.11.0", false}, {"0.11.0", "0.12.0", false},
		{"1.0.0", "0.99.9", true}, {"0.11.10", "0.11.9", true}, {"", "0.11.0", false}, {"x.y", "0.11.0", false},
	}
	for _, c := range cases {
		if got := Newer(c.latest, c.current); got != c.want {
			t.Errorf("Newer(%q, %q) = %v", c.latest, c.current, got)
		}
	}
}

func TestCheck(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "update.json")
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	calls := 0
	fetch := func(string) (string, error) { calls++; return "0.12.0", nil }

	latest, notify := Check(cache, "u", "0.11.0", now, fetch)
	if latest != "0.12.0" || !notify || calls != 1 {
		t.Fatalf("cold cache: latest=%q notify=%v calls=%d", latest, notify, calls)
	}
	// within the interval: no fetch; not yet marked notified -> still notify
	if _, notify := Check(cache, "u", "0.11.0", now.Add(time.Hour), fetch); !notify || calls != 1 {
		t.Errorf("warm cache should not fetch (calls=%d) and should notify (%v)", calls, notify)
	}
	MarkNotified(cache, now.Add(time.Hour))
	if _, notify := Check(cache, "u", "0.11.0", now.Add(2*time.Hour), fetch); notify {
		t.Error("notified an hour ago -> quiet")
	}
	if _, notify := Check(cache, "u", "0.11.0", now.Add(26*time.Hour), fetch); !notify || calls != 2 {
		t.Errorf("a day later: re-fetch (calls=%d) and notify again (%v)", calls, notify)
	}
	// fetch failure keeps the known latest and still stamps checked_at
	failing := func(string) (string, error) { return "", errors.New("offline") }
	if latest, _ := Check(cache, "u", "0.11.0", now.Add(60*time.Hour), failing); latest != "0.12.0" {
		t.Errorf("failure should keep the last known version, got %q", latest)
	}
	if c := LoadCache(cache); !c.CheckedAt.Equal(now.Add(60 * time.Hour)) {
		t.Errorf("checked_at should move even on failure: %v", c.CheckedAt)
	}
	// dev build ahead of the tap -> never notify
	if _, notify := Check(cache, "u", "0.13.0", now.Add(100*time.Hour), fetch); notify {
		t.Error("current ahead of latest must not notify")
	}
}

func TestIsBrewInstall(t *testing.T) {
	dir := t.TempDir()
	cellar := filepath.Join(dir, "Cellar", "pier", "0.11.0", "bin", "pier")
	os.MkdirAll(filepath.Dir(cellar), 0o755)
	os.WriteFile(cellar, []byte("x"), 0o755)
	link := filepath.Join(dir, "bin", "pier")
	os.MkdirAll(filepath.Dir(link), 0o755)
	os.Symlink(cellar, link)
	if !IsBrewInstall(link) || !IsBrewInstall(cellar) {
		t.Error("Cellar path (direct or via the bin symlink) is a brew install")
	}
	if IsBrewInstall(filepath.Join(dir, "local", "bin", "pier")) {
		t.Error("a path outside Cellar is not a brew install")
	}
}
