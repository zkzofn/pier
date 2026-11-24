// Package usage caches and formats per-agent subscription rate-limit usage
// (e.g. 5-hour and weekly remaining quota) for the tmux status bar.
package usage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"pier/internal/state"
)

const (
	staleAfter  = 30 * time.Minute
	expireAfter = 24 * time.Hour
	codexTail   = 128 * 1024
)

// Bucket is one quota window (e.g. 5h or 7d) with remaining percentage.
type Bucket struct {
	ID           string  `json:"id"`
	Label        string  `json:"label"`
	RemainingPct float64 `json:"remaining_pct"`
	ResetAt      int64   `json:"reset_at,omitempty"`
	Disabled     bool    `json:"disabled,omitempty"`
}

// Snapshot is the cached usage state for one AI CLI agent.
type Snapshot struct {
	Agent      string   `json:"agent"`
	Model      string   `json:"model,omitempty"`
	ContextPct *float64 `json:"context_pct,omitempty"`
	Buckets    []Bucket `json:"buckets,omitempty"`
	UpdatedAt  int64    `json:"updated_at"`
}

// Dir returns the usage cache directory under state.Root (outside panes/ so
// sidebar pane cleanup never deletes usage snapshots).
func Dir() string {
	return filepath.Join(state.Root(), "usage")
}

// Save writes snap atomically to <dir>/<agent>.json.
func Save(dir string, snap Snapshot) error {
	if snap.Agent == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, snap.Agent+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadFile reads a cached snapshot for agent from dir.
func LoadFile(dir, agent string) (Snapshot, bool) {
	if agent == "" {
		return Snapshot{}, false
	}
	data, err := os.ReadFile(filepath.Join(dir, agent+".json"))
	if err != nil {
		return Snapshot{}, false
	}
	var snap Snapshot
	if json.Unmarshal(data, &snap) != nil {
		return Snapshot{}, false
	}
	if snap.Agent == "" {
		snap.Agent = agent
	}
	return snap, true
}

// Load returns the latest usage snapshot for agent, consulting the local cache
// for push-based agents (claude, agy) and session rollout logs for codex.
func Load(dir, home, agent string, now time.Time) (Snapshot, bool) {
	switch agent {
	case "claude", "agy":
		return LoadFile(dir, agent)
	case "codex":
		if snap, ok := LoadFile(dir, agent); ok && len(snap.Buckets) > 0 &&
			now.Unix()-snap.UpdatedAt <= int64(staleAfter.Seconds()) {
			return snap, true
		}
		if snap, ok := ReadCodex(home, now); ok {
			return snap, true
		}
		return LoadFile(dir, agent)
	default:
		return Snapshot{}, false
	}
}

type rawClaudePayload struct {
	Model struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"model"`
	ContextWindow struct {
		UsedPercentage *float64 `json:"used_percentage"`
	} `json:"context_window"`
	RateLimits map[string]rawClaudeLimit `json:"rate_limits"`
}

type rawClaudeLimit struct {
	UsedPercentage      *float64        `json:"used_percentage"`
	RemainingPercentage *float64        `json:"remaining_percentage"`
	ResetsAt            json.RawMessage `json:"resets_at"`
}

// ParseClaude parses a Claude Code statusLine JSON payload into a Snapshot.
func ParseClaude(payload []byte, now time.Time) (Snapshot, bool) {
	var raw rawClaudePayload
	if json.Unmarshal(payload, &raw) != nil {
		return Snapshot{}, false
	}
	model := raw.Model.DisplayName
	if model == "" {
		model = raw.Model.ID
	}
	type rankedBucket struct {
		rank   int
		bucket Bucket
	}
	var ranked []rankedBucket
	for id, lim := range raw.RateLimits {
		var rem float64
		switch {
		case lim.RemainingPercentage != nil:
			rem = *lim.RemainingPercentage
		case lim.UsedPercentage != nil:
			rem = 100 - *lim.UsedPercentage
		default:
			continue
		}
		label, rank := claudeBucketLabel(id)
		ranked = append(ranked, rankedBucket{
			rank: rank,
			bucket: Bucket{
				ID:           id,
				Label:        label,
				RemainingPct: clampPct(rem),
				ResetAt:      parseTimestamp(lim.ResetsAt),
			},
		})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].rank != ranked[j].rank {
			return ranked[i].rank < ranked[j].rank
		}
		return ranked[i].bucket.ID < ranked[j].bucket.ID
	})
	buckets := make([]Bucket, len(ranked))
	for i, r := range ranked {
		buckets[i] = r.bucket
	}
	if len(buckets) == 0 && model == "" && raw.ContextWindow.UsedPercentage == nil {
		return Snapshot{}, false
	}
	return Snapshot{
		Agent:      "claude",
		Model:      model,
		ContextPct: raw.ContextWindow.UsedPercentage,
		Buckets:    buckets,
		UpdatedAt:  now.Unix(),
	}, true
}

func claudeBucketLabel(id string) (string, int) {
	switch id {
	case "five_hour":
		return "5h", 0
	case "seven_day":
		return "7d", 1
	case "seven_day_opus":
		return "opus", 2
	case "seven_day_sonnet":
		return "sonnet", 3
	case "seven_day_oauth_apps":
		return "oauth", 4
	default:
		return strings.ReplaceAll(id, "_", "-"), 10
	}
}

type rawAgyPayload struct {
	Model struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"model"`
	ContextWindow struct {
		UsedPercentage *float64 `json:"used_percentage"`
	} `json:"context_window"`
	Quota map[string]rawAgyQuota `json:"quota"`
}

type rawAgyQuota struct {
	RemainingFraction *float64 `json:"remaining_fraction"`
	RemainingAmount   *float64 `json:"remaining_amount"`
	ResetTime         string   `json:"reset_time"`
	ResetInSeconds    *int64   `json:"reset_in_seconds"`
	Disabled          bool     `json:"disabled"`
}

// agyBuckets is the fixed set of Antigravity quota buckets pier displays,
// in display order: the Gemini 5-hour window, the Gemini weekly window, and
// the Argon weekly pool. Antigravity reports other pools too (e.g. the 3p-*
// third-party model quotas); pier intentionally shows only these three.
var agyBuckets = []struct{ id, label string }{
	{"gemini-5h", "5h"},
	{"gemini-weekly", "7d"},
	{"gemini-argon-wtu-weekly", "Argon"},
}

// ParseAgy parses an Antigravity CLI statusLine JSON payload into a Snapshot.
func ParseAgy(payload []byte, now time.Time) (Snapshot, bool) {
	var raw rawAgyPayload
	if json.Unmarshal(payload, &raw) != nil {
		return Snapshot{}, false
	}
	model := raw.Model.DisplayName
	if model == "" {
		model = raw.Model.ID
	}
	var buckets []Bucket
	for _, want := range agyBuckets {
		q, ok := raw.Quota[want.id]
		if !ok || q.Disabled || q.RemainingFraction == nil {
			continue
		}
		var resetAt int64
		if q.ResetTime != "" {
			resetAt = parseRFC3339(q.ResetTime)
		}
		if resetAt == 0 && q.ResetInSeconds != nil && *q.ResetInSeconds > 0 {
			resetAt = now.Unix() + *q.ResetInSeconds
		}
		buckets = append(buckets, Bucket{
			ID:           want.id,
			Label:        want.label,
			RemainingPct: clampPct(*q.RemainingFraction * 100),
			ResetAt:      resetAt,
		})
	}
	if len(buckets) == 0 && model == "" && raw.ContextWindow.UsedPercentage == nil {
		return Snapshot{}, false
	}
	return Snapshot{
		Agent:      "agy",
		Model:      model,
		ContextPct: raw.ContextWindow.UsedPercentage,
		Buckets:    buckets,
		UpdatedAt:  now.Unix(),
	}, true
}

type codexRateLimitWindow struct {
	UsedPercent    *float64        `json:"used_percent"`
	UsedPercentage *float64        `json:"used_percentage"`
	ResetsAt       json.RawMessage `json:"resets_at"`
}

func (w codexRateLimitWindow) used() (float64, bool) {
	if w.UsedPercent != nil {
		return *w.UsedPercent, true
	}
	if w.UsedPercentage != nil {
		return *w.UsedPercentage, true
	}
	return 0, false
}

type codexRateLimits struct {
	Primary   *codexRateLimitWindow `json:"primary"`
	Secondary *codexRateLimitWindow `json:"secondary"`
}

type codexLine struct {
	Timestamp  string           `json:"timestamp"`
	RateLimits *codexRateLimits `json:"rate_limits"`
	Payload    *struct {
		RateLimits *codexRateLimits `json:"rate_limits"`
		Info       *struct {
			RateLimits *codexRateLimits `json:"rate_limits"`
		} `json:"info"`
	} `json:"payload"`
	Info *struct {
		RateLimits *codexRateLimits `json:"rate_limits"`
	} `json:"info"`
}

func (l codexLine) limits() *codexRateLimits {
	if l.RateLimits != nil {
		return l.RateLimits
	}
	if l.Payload != nil {
		if l.Payload.RateLimits != nil {
			return l.Payload.RateLimits
		}
		if l.Payload.Info != nil && l.Payload.Info.RateLimits != nil {
			return l.Payload.Info.RateLimits
		}
	}
	if l.Info != nil {
		return l.Info.RateLimits
	}
	return nil
}

// ReadCodex reads the newest ~/.codex/sessions/**/rollout-*.jsonl file and
// extracts its most recent primary (5h) / secondary (7d) rate limits.
func ReadCodex(home string, now time.Time) (Snapshot, bool) {
	if home == "" {
		return Snapshot{}, false
	}
	root := filepath.Join(home, ".codex", "sessions")
	var newestPath string
	var newestMod time.Time
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasPrefix(name, "rollout-") || !strings.HasSuffix(name, ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if newestPath == "" || info.ModTime().After(newestMod) {
			newestPath = path
			newestMod = info.ModTime()
		}
		return nil
	})
	if newestPath == "" {
		return Snapshot{}, false
	}
	f, err := os.Open(newestPath)
	if err != nil {
		return Snapshot{}, false
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return Snapshot{}, false
	}
	seeked := false
	if info.Size() > codexTail {
		if _, err := f.Seek(-codexTail, io.SeekEnd); err == nil {
			seeked = true
		}
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return Snapshot{}, false
	}
	if seeked {
		if nl := bytes.IndexByte(data, '\n'); nl >= 0 {
			data = data[nl+1:]
		}
	}

	var lastLimits *codexRateLimits
	var lastTS int64
	for _, line := range bytes.Split(data, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var rec codexLine
		if json.Unmarshal(line, &rec) != nil {
			continue
		}
		if lim := rec.limits(); lim != nil && (lim.Primary != nil || lim.Secondary != nil) {
			lastLimits = lim
			if ts := parseRFC3339(rec.Timestamp); ts > 0 {
				lastTS = ts
			}
		}
	}
	if lastLimits == nil {
		return Snapshot{}, false
	}
	var buckets []Bucket
	if lastLimits.Primary != nil {
		if used, ok := lastLimits.Primary.used(); ok {
			buckets = append(buckets, Bucket{
				ID:           "primary",
				Label:        "5h",
				RemainingPct: clampPct(100 - used),
				ResetAt:      parseTimestamp(lastLimits.Primary.ResetsAt),
			})
		}
	}
	if lastLimits.Secondary != nil {
		if used, ok := lastLimits.Secondary.used(); ok {
			buckets = append(buckets, Bucket{
				ID:           "secondary",
				Label:        "7d",
				RemainingPct: clampPct(100 - used),
				ResetAt:      parseTimestamp(lastLimits.Secondary.ResetsAt),
			})
		}
	}
	if len(buckets) == 0 {
		return Snapshot{}, false
	}
	updatedAt := lastTS
	if updatedAt == 0 {
		updatedAt = newestMod.Unix()
	}
	return Snapshot{
		Agent:     "codex",
		Buckets:   buckets,
		UpdatedAt: updatedAt,
	}, true
}

// FormatTmux renders snap as a tmux-colored status-right fragment. Returns ""
// when snap has no buckets or is older than 24 hours.
func FormatTmux(snap Snapshot, now time.Time, compact bool) string {
	if len(snap.Buckets) == 0 || snap.UpdatedAt <= 0 {
		return ""
	}
	age := now.Unix() - snap.UpdatedAt
	if age > int64(expireAfter.Seconds()) {
		return ""
	}
	stale := age > int64(staleAfter.Seconds())

	var parts []string
	agentColor := "#[fg=magenta]"
	if stale {
		agentColor = "#[fg=colour244]"
	}
	parts = append(parts, agentColor+snap.Agent)

	for _, b := range snap.Buckets {
		pct := int(math.Round(clampPct(b.RemainingPct)))
		s := colorFor(b.RemainingPct, stale) + fmt.Sprintf("%s:%d%%", b.Label, pct)
		if !compact && b.ResetAt > now.Unix() && (b.ResetAt-now.Unix() < 24*3600 || len(snap.Buckets) <= 2) {
			rem := time.Duration(b.ResetAt-now.Unix()) * time.Second
			s += "#[fg=colour244](" + formatDuration(rem) + ")"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " ") + "#[default]"
}

func colorFor(pct float64, stale bool) string {
	if stale {
		return "#[fg=colour244]"
	}
	switch {
	case pct >= 50:
		return "#[fg=green]"
	case pct >= 20:
		return "#[fg=yellow]"
	default:
		return "#[fg=red,bold]"
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "<1m"
	}
	totalMin := int(d.Minutes())
	days := totalMin / (24 * 60)
	hours := (totalMin % (24 * 60)) / 60
	mins := totalMin % 60
	switch {
	case days > 0:
		if hours > 0 {
			return fmt.Sprintf("%dd%dh", days, hours)
		}
		return fmt.Sprintf("%dd", days)
	case hours > 0:
		if mins > 0 {
			return fmt.Sprintf("%dh%dm", hours, mins)
		}
		return fmt.Sprintf("%dh", hours)
	default:
		return fmt.Sprintf("%dm", mins)
	}
}

func clampPct(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func parseTimestamp(raw json.RawMessage) int64 {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return 0
	}
	var num float64
	if json.Unmarshal(raw, &num) == nil && num > 0 {
		if num > 1e12 {
			num /= 1000
		}
		return int64(num)
	}
	var s string
	if json.Unmarshal(raw, &s) == nil && s != "" {
		if ts := parseRFC3339(s); ts > 0 {
			return ts
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil && f > 0 {
			if f > 1e12 {
				f /= 1000
			}
			return int64(f)
		}
	}
	return 0
}

func parseRFC3339(s string) int64 {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Unix()
		}
	}
	return 0
}
