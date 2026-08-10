// Package resume tracks live Claude Code sessions so that ones killed
// without warning — SIGKILL, kernel panic, power loss, an OS shutdown
// tearing tmux down — can be picked up automatically the next time a
// session starts in the same directory.
//
// Everything lives under ~/.claude/live-sessions:
//
//   - <session-id>.json marker {cwd,pid}: written on every user prompt
//     (UserPromptSubmit), removed on SessionEnd. A leftover marker whose
//     pid is gone means the session died with no chance to run its hooks —
//     a crash.
//   - ended.jsonl {sid,cwd,reason,ended_at}: appended when a session that
//     had a marker ends normally. A clean OS shutdown *does* give hooks a
//     chance to run, so its casualties look "ended" — they are told apart
//     by time instead: the sidebar touches .heartbeat-<boot-epoch> every
//     minute, the file's mtime freezes when the system stops, and entries
//     that ended within that final window of a previous boot are
//     reclassified as shutdown casualties.
//
// Prompt-driven (not SessionStart) on purpose: in-process teammate and
// subagent sessions never receive user prompts, so only real user sessions
// are ever recorded.
package resume

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	endedLog       = "ended.jsonl"
	heartbeatEvery = 60 * time.Second
	// shutdownWindow is how close (seconds) to the frozen heartbeat a session
	// must have ended to count as a shutdown casualty: one heartbeat period
	// plus slack for the shutdown itself.
	shutdownWindow = 90
	// ended.jsonl is trimmed to entries younger than endedKeepDays once it
	// outgrows endedMaxSize.
	endedMaxSize  = 64 * 1024
	endedKeepDays = 14
	heartbeatKeep = 30 * 24 * time.Hour
)

type marker struct {
	CWD string `json:"cwd"`
	PID int    `json:"pid"`
}

type endedRec struct {
	SID     string `json:"sid"`
	CWD     string `json:"cwd"`
	Reason  string `json:"reason"`
	EndedAt int64  `json:"ended_at"`
}

type hookPayload struct {
	SessionID string `json:"session_id"`
	CWD       string `json:"cwd"`
	Reason    string `json:"reason"` // SessionEnd only
}

// deliberateEnd lists SessionEnd reasons where the user closed or replaced
// the conversation on purpose — a shutdown moments later doesn't make those
// worth resurrecting. A SIGHUP/SIGTERM teardown reports "other", which stays
// eligible; the time window does the real filtering.
var deliberateEnd = map[string]bool{
	"clear":                       true,
	"logout":                      true,
	"prompt_input_exit":           true,
	"resume":                      true, // replaced via /resume picker
	"bypass_permissions_disabled": true,
}

// Dir returns the live-session tracking directory.
func Dir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "live-sessions")
}

// RecordPrompt (UserPromptSubmit hook) marks the session live: it writes the
// marker, whose mtime doubles as the last-activity time.
func RecordPrompt(dir string, payload []byte) error {
	var p hookPayload
	if json.Unmarshal(payload, &p) != nil || !validSID(p.SessionID) {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(marker{CWD: p.CWD, PID: claudePID()})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, p.SessionID+".json"), data, 0o644)
}

// RecordEnd (SessionEnd hook) retires the marker. Only sessions that had one
// (real user sessions past their first prompt) get an ended.jsonl entry —
// that log is what shutdown recovery mines.
func RecordEnd(dir string, payload []byte) error {
	var p hookPayload
	if json.Unmarshal(payload, &p) != nil || !validSID(p.SessionID) {
		return nil
	}
	mpath := filepath.Join(dir, p.SessionID+".json")
	if _, err := os.Stat(mpath); err != nil {
		return nil
	}
	reason := p.Reason
	if reason == "" {
		reason = "?"
	}
	if rec, err := json.Marshal(endedRec{
		SID: p.SessionID, CWD: p.CWD, Reason: reason, EndedAt: time.Now().Unix(),
	}); err == nil {
		appendEnded(dir, rec)
	}
	return os.Remove(mpath)
}

// Pick returns the session id to resume for a new session starting in cwd,
// or "" when there is nothing to recover. Crashed sessions (stale marker)
// win over shutdown casualties (ended.jsonl near a frozen heartbeat).
// Whatever it returns — and every record that matched — is consumed, so a
// session is only ever offered once.
func Pick(dir, cwd string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	projects := filepath.Join(home, ".claude", "projects")
	if sid := pickCrashed(dir, projects, cwd, claudeAlive); sid != "" {
		return sid
	}
	return pickShutdown(dir, projects, cwd, bootUnix())
}

// pickCrashed scans markers recorded in cwd whose process is gone and picks
// the one with the most recent activity that still has a transcript. All of
// the cwd's stale markers are consumed — one resurrection per crash; any
// others remain reachable via `claude -r`.
func pickCrashed(dir, projectsDir, cwd string, alive func(pid int) bool) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	cwd = filepath.Clean(cwd)
	type cand struct {
		sid   string
		mtime time.Time
	}
	var stale []cand
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		var m marker
		if json.Unmarshal(data, &m) != nil || filepath.Clean(m.CWD) != cwd {
			continue
		}
		if alive(m.PID) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		stale = append(stale, cand{sid: strings.TrimSuffix(name, ".json"), mtime: info.ModTime()})
	}
	if len(stale) == 0 {
		return ""
	}
	sort.Slice(stale, func(i, j int) bool { return stale[i].mtime.After(stale[j].mtime) })
	best := ""
	for _, c := range stale {
		_ = os.Remove(filepath.Join(dir, c.sid+".json"))
		if best == "" && hasTranscript(projectsDir, c.sid) {
			best = c.sid
		}
	}
	return best
}

// pickShutdown reclassifies "cleanly ended" sessions that were actually torn
// down by an OS shutdown/reboot: entries from a previous boot that ended
// within shutdownWindow of a frozen heartbeat, minus deliberate exits.
// Every matching entry for the cwd is consumed.
func pickShutdown(dir, projectsDir, cwd string, boot int64) string {
	if boot <= 0 {
		return ""
	}
	beats := prevHeartbeats(dir, boot)
	if len(beats) == 0 {
		return ""
	}
	path := filepath.Join(dir, endedLog)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	cwd = filepath.Clean(cwd)
	best, bestAt := "", int64(0)
	var keep []byte
	consumed := false
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var r endedRec
		if json.Unmarshal(line, &r) == nil &&
			filepath.Clean(r.CWD) == cwd && r.EndedAt < boot &&
			!deliberateEnd[r.Reason] && nearAny(beats, r.EndedAt) {
			consumed = true
			if validSID(r.SID) && r.EndedAt > bestAt && hasTranscript(projectsDir, r.SID) {
				best, bestAt = r.SID, r.EndedAt
			}
			continue
		}
		keep = append(keep, line...)
		keep = append(keep, '\n')
	}
	if consumed {
		tmp := path + ".tmp"
		if os.WriteFile(tmp, keep, 0o644) == nil {
			_ = os.Rename(tmp, path)
		}
	}
	return best
}

// HeartbeatTick keeps .heartbeat-<boot-epoch> fresh; the sidebar calls it on
// its poll tick. Throttled to one touch a minute — the point is only that
// the mtime freezes near the moment the system stops running. Files from
// previous boots are left alone (pickShutdown reads them) until reaped.
func HeartbeatTick(dir string) {
	heartbeatTick(dir, bootUnix())
}

func heartbeatTick(dir string, boot int64) {
	if boot <= 0 {
		return
	}
	path := filepath.Join(dir, fmt.Sprintf(".heartbeat-%d", boot))
	if info, err := os.Stat(path); err == nil {
		if time.Since(info.ModTime()) >= heartbeatEvery {
			now := time.Now()
			_ = os.Chtimes(path, now, now)
		}
		return
	}
	// first tick this boot: create the epoch file, reap ancient ones
	if os.MkdirAll(dir, 0o755) != nil || os.WriteFile(path, nil, 0o644) != nil {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		epoch, ok := heartbeatEpoch(e.Name())
		if !ok || epoch == boot {
			continue
		}
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > heartbeatKeep {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// prevHeartbeats lists frozen heartbeat mtimes from earlier boots — each one
// marks the last minute a previous uptime was alive.
func prevHeartbeats(dir string, boot int64) []int64 {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var beats []int64
	for _, e := range entries {
		epoch, ok := heartbeatEpoch(e.Name())
		if !ok || epoch >= boot {
			continue
		}
		if info, err := e.Info(); err == nil {
			beats = append(beats, info.ModTime().Unix())
		}
	}
	return beats
}

func heartbeatEpoch(name string) (int64, bool) {
	s, ok := strings.CutPrefix(name, ".heartbeat-")
	if !ok {
		return 0, false
	}
	epoch, err := strconv.ParseInt(s, 10, 64)
	if err != nil || epoch <= 0 {
		return 0, false
	}
	return epoch, true
}

func nearAny(beats []int64, t int64) bool {
	for _, b := range beats {
		d := t - b
		if d < 0 {
			d = -d
		}
		if d <= shutdownWindow {
			return true
		}
	}
	return false
}

func appendEnded(dir string, line []byte) {
	path := filepath.Join(dir, endedLog)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	_, _ = f.Write(append(line, '\n'))
	_ = f.Close()
	trimEnded(path, endedMaxSize, time.Now().Unix()-endedKeepDays*24*3600)
}

// trimEnded drops entries older than cutoff (and unparseable lines) once the
// log outgrows maxSize.
func trimEnded(path string, maxSize, cutoff int64) {
	info, err := os.Stat(path)
	if err != nil || info.Size() <= maxSize {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var keep []byte
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var r endedRec
		if json.Unmarshal(line, &r) != nil || r.EndedAt < cutoff {
			continue
		}
		keep = append(keep, line...)
		keep = append(keep, '\n')
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, keep, 0o644) == nil {
		_ = os.Rename(tmp, path)
	}
}

// claudePID resolves the Claude Code process a hook is running under. Hook
// commands are spawned as direct children (or via a `sh -c` that execs), so
// the parent normally is claude itself; when a shell wrapper didn't exec,
// hop one level up.
func claudePID() int {
	pid := os.Getppid()
	out, err := exec.Command("ps", "-o", "ppid=,comm=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return pid
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return pid
	}
	switch filepath.Base(strings.Join(fields[1:], " ")) {
	case "sh", "bash", "zsh", "dash":
		if pp, err := strconv.Atoi(fields[0]); err == nil && pp > 1 {
			return pp
		}
	}
	return pid
}

// claudeAlive reports whether pid is a live Claude Code process. `ps` alone
// answers both liveness and identity; a recycled pid on some other program
// counts as dead, which is what resuming wants.
func claudeAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	out, err := exec.Command("ps", "-o", "comm=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return false
	}
	return isClaudeComm(strings.TrimSpace(string(out)))
}

// isClaudeComm matches "claude" or a bare version string like "2.1.226" —
// the CLI re-execs its versioned binary, which is how a running claude's
// comm often reads.
func isClaudeComm(comm string) bool {
	comm = filepath.Base(comm)
	if comm == "claude" {
		return true
	}
	if comm == "" || comm[0] < '0' || comm[0] > '9' {
		return false
	}
	dot := false
	for _, r := range comm {
		switch {
		case r == '.':
			dot = true
		case r < '0' || r > '9':
			return false
		}
	}
	return dot
}

// hasTranscript reports whether Claude Code still has a transcript for sid —
// without one `claude --resume` has nothing to reopen.
func hasTranscript(projectsDir, sid string) bool {
	matches, _ := filepath.Glob(filepath.Join(projectsDir, "*", sid+".jsonl"))
	return len(matches) > 0
}

// validSID accepts uuid-shaped ids; anything else never reaches the
// filesystem as a path fragment.
func validSID(sid string) bool {
	if sid == "" {
		return false
	}
	for _, r := range sid {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
		default:
			return false
		}
	}
	return true
}

var bootOnce struct {
	sync.Once
	unix int64
}

// bootUnix returns the current boot's epoch second (macOS: sysctl
// kern.boottime), 0 when unknown — heartbeat and shutdown recovery then
// degrade to no-ops.
func bootUnix() int64 {
	bootOnce.Do(func() {
		out, err := exec.Command("sysctl", "-n", "kern.boottime").Output()
		if err == nil {
			bootOnce.unix = parseBootTime(string(out))
		}
	})
	return bootOnce.unix
}

// parseBootTime extracts sec from `{ sec = 1753054334, usec = 673069 } ...`.
func parseBootTime(s string) int64 {
	i := strings.Index(s, "sec =")
	if i < 0 {
		return 0
	}
	s = strings.TrimLeft(s[i+len("sec ="):], " ")
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	sec, err := strconv.ParseInt(s[:end], 10, 64)
	if err != nil {
		return 0
	}
	return sec
}
