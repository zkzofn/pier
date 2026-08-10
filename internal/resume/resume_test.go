package resume

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func writeMarker(t *testing.T, dir, sid, cwd string, pid int, mtime time.Time) {
	t.Helper()
	data, _ := json.Marshal(marker{CWD: cwd, PID: pid})
	path := filepath.Join(dir, sid+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func writeTranscript(t *testing.T, projectsDir, sid string) {
	t.Helper()
	dir := filepath.Join(projectsDir, "some-project")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, sid+".jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeEnded(t *testing.T, dir string, recs ...endedRec) {
	t.Helper()
	var b []byte
	for _, r := range recs {
		line, _ := json.Marshal(r)
		b = append(b, line...)
		b = append(b, '\n')
	}
	if err := os.WriteFile(filepath.Join(dir, endedLog), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readEnded(t *testing.T, dir string) []endedRec {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, endedLog))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var recs []endedRec
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		var r endedRec
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("bad line %q: %v", line, err)
		}
		recs = append(recs, r)
	}
	return recs
}

func writeHeartbeat(t *testing.T, dir string, epoch int64, mtime time.Time) {
	t.Helper()
	path := filepath.Join(dir, ".heartbeat-"+strconv.FormatInt(epoch, 10))
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func TestRecordPromptWritesMarker(t *testing.T) {
	dir := t.TempDir()
	payload := []byte(`{"session_id":"aaaa-1111","cwd":"/work/x","prompt":"hi"}`)
	if err := RecordPrompt(dir, payload); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "aaaa-1111.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m marker
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m.CWD != "/work/x" {
		t.Errorf("cwd = %q", m.CWD)
	}
	if m.PID <= 0 {
		t.Errorf("pid = %d, want > 0", m.PID)
	}
}

func TestRecordPromptRejectsBadSID(t *testing.T) {
	dir := t.TempDir()
	for _, payload := range []string{
		`{"cwd":"/x"}`,
		`{"session_id":"../evil","cwd":"/x"}`,
		`not json`,
	} {
		if err := RecordPrompt(dir, []byte(payload)); err != nil {
			t.Fatal(err)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("markers written for invalid payloads: %v", entries)
	}
}

func TestRecordEnd(t *testing.T) {
	dir := t.TempDir()
	writeMarker(t, dir, "bbbb-2222", "/work/y", 1, time.Now())

	payload := []byte(`{"session_id":"bbbb-2222","cwd":"/work/y","reason":"clear"}`)
	if err := RecordEnd(dir, payload); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "bbbb-2222.json")); !os.IsNotExist(err) {
		t.Error("marker not removed")
	}
	recs := readEnded(t, dir)
	if len(recs) != 1 {
		t.Fatalf("ended entries = %d, want 1", len(recs))
	}
	r := recs[0]
	if r.SID != "bbbb-2222" || r.CWD != "/work/y" || r.Reason != "clear" {
		t.Errorf("bad record: %+v", r)
	}
	if d := time.Now().Unix() - r.EndedAt; d < 0 || d > 5 {
		t.Errorf("ended_at drift: %d", d)
	}
}

func TestRecordEndWithoutMarkerIsSilent(t *testing.T) {
	dir := t.TempDir()
	payload := []byte(`{"session_id":"cccc-3333","cwd":"/w","reason":"other"}`)
	if err := RecordEnd(dir, payload); err != nil {
		t.Fatal(err)
	}
	if recs := readEnded(t, dir); recs != nil {
		t.Errorf("logged a session that had no marker: %+v", recs)
	}
}

func TestRecordEndDefaultsReason(t *testing.T) {
	dir := t.TempDir()
	writeMarker(t, dir, "dddd-4444", "/w", 1, time.Now())
	if err := RecordEnd(dir, []byte(`{"session_id":"dddd-4444","cwd":"/w"}`)); err != nil {
		t.Fatal(err)
	}
	if recs := readEnded(t, dir); len(recs) != 1 || recs[0].Reason != "?" {
		t.Errorf("recs = %+v, want one with reason %q", recs, "?")
	}
}

func TestPickCrashedPicksNewestAndConsumes(t *testing.T) {
	dir, projects := t.TempDir(), t.TempDir()
	now := time.Now()
	writeMarker(t, dir, "old-1", "/w", 0, now.Add(-2*time.Hour))
	writeMarker(t, dir, "new-1", "/w", 0, now.Add(-time.Minute))
	writeTranscript(t, projects, "old-1")
	writeTranscript(t, projects, "new-1")

	dead := func(int) bool { return false }
	if got := pickCrashed(dir, projects, "/w", dead); got != "new-1" {
		t.Errorf("picked %q, want new-1", got)
	}
	for _, sid := range []string{"old-1", "new-1"} {
		if _, err := os.Stat(filepath.Join(dir, sid+".json")); !os.IsNotExist(err) {
			t.Errorf("marker %s not consumed", sid)
		}
	}
	if got := pickCrashed(dir, projects, "/w", dead); got != "" {
		t.Errorf("second pick = %q, want empty", got)
	}
}

func TestPickCrashedKeepsLiveAndOtherCWD(t *testing.T) {
	dir, projects := t.TempDir(), t.TempDir()
	now := time.Now()
	writeMarker(t, dir, "live-1", "/w", 4242, now)
	writeMarker(t, dir, "other-1", "/elsewhere", 0, now)
	writeTranscript(t, projects, "live-1")
	writeTranscript(t, projects, "other-1")

	alive := func(pid int) bool { return pid == 4242 }
	if got := pickCrashed(dir, projects, "/w", alive); got != "" {
		t.Errorf("picked %q, want empty", got)
	}
	for _, sid := range []string{"live-1", "other-1"} {
		if _, err := os.Stat(filepath.Join(dir, sid+".json")); err != nil {
			t.Errorf("marker %s should be untouched: %v", sid, err)
		}
	}
}

func TestPickCrashedSkipsMissingTranscript(t *testing.T) {
	dir, projects := t.TempDir(), t.TempDir()
	now := time.Now()
	writeMarker(t, dir, "gone-1", "/w", 0, now)                   // no transcript
	writeMarker(t, dir, "kept-1", "/w", 0, now.Add(-time.Minute)) // older, has one
	writeTranscript(t, projects, "kept-1")

	if got := pickCrashed(dir, projects, "/w", func(int) bool { return false }); got != "kept-1" {
		t.Errorf("picked %q, want kept-1", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "gone-1.json")); !os.IsNotExist(err) {
		t.Error("transcript-less marker not consumed")
	}
}

func TestPickShutdown(t *testing.T) {
	dir, projects := t.TempDir(), t.TempDir()
	const boot, frozen = int64(2_000_000_000), int64(1_999_990_000)
	writeHeartbeat(t, dir, boot-100_000, time.Unix(frozen, 0))
	writeEnded(t, dir,
		endedRec{SID: "hit-old", CWD: "/w", Reason: "other", EndedAt: frozen - 60},
		endedRec{SID: "hit-new", CWD: "/w", Reason: "?", EndedAt: frozen + 30},
		endedRec{SID: "far", CWD: "/w", Reason: "other", EndedAt: frozen - 500},
		endedRec{SID: "cleared", CWD: "/w", Reason: "clear", EndedAt: frozen + 10},
		endedRec{SID: "elsewhere", CWD: "/other", Reason: "other", EndedAt: frozen + 20},
	)
	writeTranscript(t, projects, "hit-old")
	writeTranscript(t, projects, "hit-new")

	if got := pickShutdown(dir, projects, "/w", boot); got != "hit-new" {
		t.Errorf("picked %q, want hit-new", got)
	}
	// both window hits consumed; the rest stay
	var sids []string
	for _, r := range readEnded(t, dir) {
		sids = append(sids, r.SID)
	}
	want := []string{"far", "cleared", "elsewhere"}
	if strings.Join(sids, ",") != strings.Join(want, ",") {
		t.Errorf("remaining = %v, want %v", sids, want)
	}
	if got := pickShutdown(dir, projects, "/w", boot); got != "" {
		t.Errorf("second pick = %q, want empty", got)
	}
}

func TestPickShutdownNeedsHeartbeatAndBoot(t *testing.T) {
	dir, projects := t.TempDir(), t.TempDir()
	writeEnded(t, dir, endedRec{SID: "x-1", CWD: "/w", Reason: "other", EndedAt: 100})
	writeTranscript(t, projects, "x-1")

	if got := pickShutdown(dir, projects, "/w", 0); got != "" {
		t.Errorf("boot=0 picked %q", got)
	}
	if got := pickShutdown(dir, projects, "/w", 1000); got != "" {
		t.Errorf("no heartbeat but picked %q", got)
	}
	if len(readEnded(t, dir)) != 1 {
		t.Error("entries consumed without a pick basis")
	}
}

func TestPickShutdownIgnoresCurrentBootEntries(t *testing.T) {
	dir, projects := t.TempDir(), t.TempDir()
	const boot = int64(1000)
	// a heartbeat from the *current* epoch must not qualify entries
	writeHeartbeat(t, dir, boot, time.Unix(boot+500, 0))
	writeEnded(t, dir, endedRec{SID: "y-1", CWD: "/w", Reason: "other", EndedAt: boot + 490})
	writeTranscript(t, projects, "y-1")

	if got := pickShutdown(dir, projects, "/w", boot); got != "" {
		t.Errorf("picked %q from the current epoch", got)
	}
}

func TestPickShutdownMatchesAnyPreviousBoot(t *testing.T) {
	dir, projects := t.TempDir(), t.TempDir()
	const boot = int64(3_000_000)
	writeHeartbeat(t, dir, 1_000_000, time.Unix(1_500_000, 0)) // two boots ago
	writeHeartbeat(t, dir, 2_000_000, time.Unix(2_500_000, 0)) // previous boot
	writeEnded(t, dir, endedRec{SID: "z-1", CWD: "/w", Reason: "other", EndedAt: 1_500_040})
	writeTranscript(t, projects, "z-1")

	if got := pickShutdown(dir, projects, "/w", boot); got != "z-1" {
		t.Errorf("picked %q, want z-1", got)
	}
}

func TestHeartbeatTick(t *testing.T) {
	dir := t.TempDir()
	const boot = int64(1_700_000_000)
	path := filepath.Join(dir, ".heartbeat-1700000000")

	heartbeatTick(dir, boot)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	created := info.ModTime()

	// within the throttle window: untouched
	heartbeatTick(dir, boot)
	if info, _ := os.Stat(path); !info.ModTime().Equal(created) {
		t.Error("throttled tick still touched the file")
	}

	// stale: touched
	old := time.Now().Add(-2 * heartbeatEvery)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	heartbeatTick(dir, boot)
	if info, _ := os.Stat(path); !info.ModTime().After(old.Add(time.Second)) {
		t.Error("stale heartbeat not refreshed")
	}

	heartbeatTick(dir, 0) // no boot time: no files, no panic
}

func TestHeartbeatTickReapsAncientEpochs(t *testing.T) {
	dir := t.TempDir()
	const boot = int64(1_700_000_000)
	writeHeartbeat(t, dir, 1, time.Now().Add(-40*24*time.Hour)) // ancient
	writeHeartbeat(t, dir, 2, time.Now().Add(-2*24*time.Hour))  // recent prev boot
	heartbeatTick(dir, boot)                                    // creates current, reaps
	if _, err := os.Stat(filepath.Join(dir, ".heartbeat-1")); !os.IsNotExist(err) {
		t.Error("ancient heartbeat not reaped")
	}
	if _, err := os.Stat(filepath.Join(dir, ".heartbeat-2")); err != nil {
		t.Error("recent previous-boot heartbeat must survive")
	}
}

func TestTrimEnded(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().Unix()
	writeEnded(t, dir,
		endedRec{SID: "old-2", CWD: "/w", Reason: "clear", EndedAt: now - 100},
		endedRec{SID: "new-2", CWD: "/w", Reason: "clear", EndedAt: now},
	)
	path := filepath.Join(dir, endedLog)

	trimEnded(path, 1024*1024, now-50) // under maxSize: untouched
	if len(readEnded(t, dir)) != 2 {
		t.Fatal("trim ran below the size threshold")
	}
	trimEnded(path, 1, now-50) // over: old entry dropped
	recs := readEnded(t, dir)
	if len(recs) != 1 || recs[0].SID != "new-2" {
		t.Errorf("after trim: %+v", recs)
	}
}

func TestParseBootTime(t *testing.T) {
	got := parseBootTime("{ sec = 1753054334, usec = 673069 } Mon Jul 21 08:32:14 2025\n")
	if got != 1753054334 {
		t.Errorf("parseBootTime = %d", got)
	}
	if parseBootTime("garbage") != 0 {
		t.Error("garbage should parse to 0")
	}
}

func TestIsClaudeComm(t *testing.T) {
	for comm, want := range map[string]bool{
		"claude":                true,
		"/usr/local/bin/claude": true,
		"2.1.226":               true,
		"zsh":                   false,
		"":                      false,
		".":                     false,
		"12":                    false, // digits but no dot: not a version
		"2.1.2a":                false,
	} {
		if got := isClaudeComm(comm); got != want {
			t.Errorf("isClaudeComm(%q) = %v, want %v", comm, got, want)
		}
	}
}

func TestValidSID(t *testing.T) {
	for sid, want := range map[string]bool{
		"9cb8d10c-ce01-4e53-8c2b-e129b38bec87": true,
		"":                                     false, "../x": false, "a b": false, "a/b": false,
	} {
		if got := validSID(sid); got != want {
			t.Errorf("validSID(%q) = %v, want %v", sid, got, want)
		}
	}
}
