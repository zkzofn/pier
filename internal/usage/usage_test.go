package usage

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseClaude(t *testing.T) {
	now := time.Unix(1700000000, 0)
	payload := []byte(`{
		"model": {"display_name": "Claude Opus"},
		"context_window": {"used_percentage": 23.5},
		"rate_limits": {
			"five_hour": {"used_percentage": 35.0, "resets_at": 1700007800},
			"seven_day": {"used_percentage": 17.0, "resets_at": "2023-11-16T22:13:20Z"}
		}
	}`)
	snap, ok := ParseClaude(payload, now)
	if !ok {
		t.Fatal("expected ParseClaude to succeed")
	}
	if snap.Agent != "claude" || snap.Model != "Claude Opus" {
		t.Errorf("unexpected metadata: %+v", snap)
	}
	if snap.ContextPct == nil || *snap.ContextPct != 23.5 {
		t.Errorf("unexpected context pct: %v", snap.ContextPct)
	}
	if len(snap.Buckets) != 2 {
		t.Fatalf("want 2 buckets, got %d", len(snap.Buckets))
	}
	if snap.Buckets[0].Label != "5h" || snap.Buckets[0].RemainingPct != 65 || snap.Buckets[0].ResetAt != 1700007800 {
		t.Errorf("5h bucket wrong: %+v", snap.Buckets[0])
	}
	if snap.Buckets[1].Label != "7d" || snap.Buckets[1].RemainingPct != 83 || snap.Buckets[1].ResetAt != 1700172800 {
		t.Errorf("7d bucket wrong: %+v", snap.Buckets[1])
	}

	// Payload without rate_limits still parses model/context.
	noLimits := []byte(`{"model":{"display_name":"Claude"},"context_window":{"used_percentage":10}}`)
	snap2, ok := ParseClaude(noLimits, now)
	if !ok || len(snap2.Buckets) != 0 || snap2.Model != "Claude" {
		t.Errorf("expected model-only snapshot, got ok=%v snap=%+v", ok, snap2)
	}
}

func TestParseAgy(t *testing.T) {
	now := time.Unix(1700000000, 0)
	payload := []byte(`{
		"model": {"id": "Gemini 4 Argon", "display_name": "Gemini 4 Argon"},
		"context_window": {"used_percentage": 14.2},
		"quota": {
			"gemini-5h": {"remaining_fraction": 0.84, "reset_in_seconds": 7200},
			"gemini-weekly": {"remaining_fraction": 0.9378, "reset_time": "2023-11-20T00:00:00Z"},
			"claude-weekly": {"remaining_fraction": 0.50, "reset_in_seconds": 3600},
			"disabled-bucket": {"remaining_fraction": 0.0, "disabled": true}
		}
	}`)
	snap, ok := ParseAgy(payload, now)
	if !ok {
		t.Fatal("expected ParseAgy to succeed")
	}
	// Active model is Gemini, so Gemini group buckets are selected.
	if len(snap.Buckets) != 2 {
		t.Fatalf("want 2 gemini buckets, got %+v", snap.Buckets)
	}
	if snap.Buckets[0].Label != "5h" || snap.Buckets[0].RemainingPct != 84 || snap.Buckets[0].ResetAt != 1700007200 {
		t.Errorf("5h bucket wrong: %+v", snap.Buckets[0])
	}
	if snap.Buckets[1].Label != "7d" || snap.Buckets[1].RemainingPct < 93.7 || snap.Buckets[1].RemainingPct > 93.8 {
		t.Errorf("7d bucket wrong: %+v", snap.Buckets[1])
	}

	// Only the three Gemini buckets are shown, in fixed order, whatever the
	// active model: the 3p-* third-party pools never appear.
	fullPayload := []byte(`{
		"model": {"id": "Orchestrator (Neon)", "display_name": "Orchestrator (Neon)"},
		"quota": {
			"3p-5h": {"remaining_fraction": 1},
			"3p-weekly": {"remaining_fraction": 1},
			"gemini-5h": {"remaining_fraction": 0.5},
			"gemini-weekly": {"remaining_fraction": 0.25},
			"gemini-argon-wtu-weekly": {"remaining_fraction": 0.75}
		}
	}`)
	snapFull, ok := ParseAgy(fullPayload, now)
	if !ok || len(snapFull.Buckets) != 3 {
		t.Fatalf("expected 3 whitelisted buckets, got ok=%v %+v", ok, snapFull.Buckets)
	}
	wantLabels := []string{"5h", "7d", "Argon"}
	wantPct := []float64{50, 25, 75}
	for i := range wantLabels {
		if snapFull.Buckets[i].Label != wantLabels[i] || snapFull.Buckets[i].RemainingPct != wantPct[i] {
			t.Errorf("bucket %d wrong: %+v", i, snapFull.Buckets[i])
		}
	}
}

func TestReadCodex(t *testing.T) {
	home := t.TempDir()
	sessDir := filepath.Join(home, ".codex", "sessions", "2026", "11", "19")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rollout := filepath.Join(sessDir, "rollout-1.jsonl")
	lines := strings.Join([]string{
		`{"timestamp":"2026-11-19T10:00:00Z","payload":{"type":"token_count","rate_limits":{"primary":{"used_percent":10,"resets_at":1700005000}}}}`,
		`{"timestamp":"2026-11-19T10:05:00Z","payload":{"type":"token_count","rate_limits":{"primary":{"used_percent":25,"resets_at":1700009000},"secondary":{"used_percent":40,"resets_at":1700500000}}}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(rollout, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	snap, ok := ReadCodex(home, time.Unix(1700000000, 0))
	if !ok {
		t.Fatal("expected ReadCodex to succeed")
	}
	if snap.Agent != "codex" || len(snap.Buckets) != 2 {
		t.Fatalf("unexpected codex snap: %+v", snap)
	}
	if snap.Buckets[0].Label != "5h" || snap.Buckets[0].RemainingPct != 75 {
		t.Errorf("codex primary wrong: %+v", snap.Buckets[0])
	}
	if snap.Buckets[1].Label != "7d" || snap.Buckets[1].RemainingPct != 60 {
		t.Errorf("codex secondary wrong: %+v", snap.Buckets[1])
	}
}

func TestSaveLoadAndPreserveBuckets(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(1700000000, 0)
	initial := []byte(`{"model":{"display_name":"Claude"},"rate_limits":{"five_hour":{"used_percentage":20}}}`)
	var out, errBuf bytes.Buffer
	if err := runStatusLine("claude", "", dir, bytes.NewReader(initial), &out, &errBuf, now); err != nil {
		t.Fatal(err)
	}
	// A subsequent tick without rate_limits preserves previous buckets and UpdatedAt.
	later := now.Add(5 * time.Minute)
	partial := []byte(`{"model":{"display_name":"Claude"},"context_window":{"used_percentage":50}}`)
	if err := runStatusLine("claude", "", dir, bytes.NewReader(partial), &out, &errBuf, later); err != nil {
		t.Fatal(err)
	}
	got, ok := LoadFile(dir, "claude")
	if !ok || len(got.Buckets) != 1 || got.Buckets[0].RemainingPct != 80 {
		t.Fatalf("expected preserved buckets, got ok=%v %+v", ok, got)
	}
	if got.UpdatedAt != now.Unix() {
		t.Errorf("UpdatedAt should stay at last bucket update (%d), got %d", now.Unix(), got.UpdatedAt)
	}
}

func TestRunStatusLineWrap(t *testing.T) {
	dir := t.TempDir()
	payload := []byte(`{"model":{"display_name":"Claude"},"context_window":{"used_percentage":40},"rate_limits":{"five_hour":{"used_percentage":15}}}`)
	var out, errBuf bytes.Buffer
	err := runStatusLine("claude", "cat", dir, bytes.NewReader(payload), &out, &errBuf, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), payload) {
		t.Errorf("wrapped command did not receive original stdin / preserve stdout: %q", out.String())
	}
	snap, ok := LoadFile(dir, "claude")
	if !ok || len(snap.Buckets) != 1 || snap.Buckets[0].RemainingPct != 85 {
		t.Errorf("cache not saved during wrap: ok=%v %+v", ok, snap)
	}
}

func TestFormatTmux(t *testing.T) {
	now := time.Unix(1700000000, 0)
	snap := Snapshot{
		Agent:     "claude",
		UpdatedAt: now.Add(-5 * time.Minute).Unix(),
		Buckets: []Bucket{
			{Label: "5h", RemainingPct: 65, ResetAt: now.Add(2*time.Hour + 10*time.Minute).Unix()},
			{Label: "7d", RemainingPct: 18, ResetAt: now.Add(3*24*time.Hour + 4*time.Hour).Unix()},
		},
	}
	full := FormatTmux(snap, now, false)
	for _, want := range []string{"#[fg=magenta]claude", "#[fg=green]5h:65%", "(2h10m)", "#[fg=red,bold]7d:18%#[nobold]", "(3d4h)", "#[default]"} {
		if !strings.Contains(full, want) {
			t.Errorf("full format missing %q in %q", want, full)
		}
	}

	compact := FormatTmux(snap, now, true)
	if strings.Contains(compact, "(2h10m)") || !strings.Contains(compact, "5h:65%") {
		t.Errorf("compact format unexpected: %q", compact)
	}

	// Stale (>30m) turns grey; yellow threshold 20..49.
	snapMid := Snapshot{
		Agent:     "agy",
		UpdatedAt: now.Add(-45 * time.Minute).Unix(),
		Buckets:   []Bucket{{Label: "5h", RemainingPct: 35}},
	}
	staleStr := FormatTmux(snapMid, now, false)
	if !strings.Contains(staleStr, "#[fg=colour244]agy #[fg=colour244]5h:35%") {
		t.Errorf("stale format wrong: %q", staleStr)
	}

	// Expired (>24h) hides completely.
	snapExpired := snapMid
	snapExpired.UpdatedAt = now.Add(-25 * time.Hour).Unix()
	if got := FormatTmux(snapExpired, now, false); got != "" {
		t.Errorf("expired snapshot must return empty string, got %q", got)
	}
}
