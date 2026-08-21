// Package update tells the user when the Homebrew tap carries a newer pier
// and runs the upgrade when asked. The tap formula is the source of truth:
// it is exactly what `brew upgrade` would install.
package update

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"pier/internal/state"
)

// FormulaURL is the raw tap formula; PIER_UPDATE_URL overrides it (e2e).
const FormulaURL = "https://raw.githubusercontent.com/zkzofn/homebrew-tap/main/Formula/pier.rb"

// CheckInterval bounds both the network check and the nagging.
const CheckInterval = 24 * time.Hour

// Cache is what update.json remembers between runs.
type Cache struct {
	CheckedAt  time.Time `json:"checked_at"`
	Latest     string    `json:"latest"`
	NotifiedAt time.Time `json:"notified_at"`
}

// CachePath lives in state.Root — NOT state.Dir, whose *.json files the
// sidebar prunes against live panes every two seconds.
func CachePath() string { return filepath.Join(state.Root(), "update.json") }

// LoadCache reads the cache; a missing or broken file is an empty cache.
func LoadCache(path string) Cache {
	var c Cache
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &c)
	}
	return c
}

// SaveCache writes the cache, creating its directory.
func SaveCache(path string, c Cache) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

var versionRe = regexp.MustCompile(`version "(\d+\.\d+\.\d+)"`)

// ParseFormulaVersion pulls the `version "x.y.z"` line out of a formula.
func ParseFormulaVersion(rb string) string {
	if m := versionRe.FindStringSubmatch(rb); m != nil {
		return m[1]
	}
	return ""
}

func parseSemver(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// Newer reports whether latest is a strictly higher x.y.z than current;
// anything unparseable is never newer.
func Newer(latest, current string) bool {
	l, ok1 := parseSemver(latest)
	c, ok2 := parseSemver(current)
	if !ok1 || !ok2 {
		return false
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

// Fetch downloads the formula and returns its version.
func Fetch(url string, timeout time.Duration) (string, error) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", err
	}
	v := ParseFormulaVersion(string(body))
	if v == "" {
		return "", fmt.Errorf("no version in formula")
	}
	return v, nil
}

// Check returns the latest known version — from the cache, or from fetch
// when the cache is older than CheckInterval — and whether the user should
// be told now: latest is newer than current and no notice went out within
// the interval. A failed fetch keeps the last known version but still stamps
// checked_at, so an offline machine waits a day before trying again rather
// than hanging every start. The cache file is updated in place.
func Check(cachePath, url, current string, now time.Time, fetch func(string) (string, error)) (latest string, notify bool) {
	c := LoadCache(cachePath)
	if now.Sub(c.CheckedAt) >= CheckInterval {
		if v, err := fetch(url); err == nil {
			c.Latest = v
		}
		c.CheckedAt = now
		_ = SaveCache(cachePath, c)
	}
	if !Newer(c.Latest, current) {
		return c.Latest, false
	}
	return c.Latest, now.Sub(c.NotifiedAt) >= CheckInterval
}

// MarkNotified records that the user saw the notice (prompt or one-liner).
func MarkNotified(cachePath string, now time.Time) {
	c := LoadCache(cachePath)
	c.NotifiedAt = now
	_ = SaveCache(cachePath, c)
}

// IsBrewInstall reports whether the binary lives in a Homebrew Cellar.
func IsBrewInstall(exe string) bool {
	real, err := filepath.EvalSymlinks(exe)
	if err != nil {
		real = exe
	}
	return strings.Contains(real, "/Cellar/pier/")
}

// Upgrade runs the Homebrew upgrade, streaming brew's output to out. The
// explicit `brew update` matters: the tap checkout is what brew upgrades
// from, and auto-update may not have refreshed it.
func Upgrade(out io.Writer) error {
	for _, args := range [][]string{{"update", "--quiet"}, {"upgrade", "zkzofn/tap/pier"}} {
		cmd := exec.Command("brew", args...)
		cmd.Stdout, cmd.Stderr = out, out
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("brew %s: %w", args[0], err)
		}
	}
	return nil
}
