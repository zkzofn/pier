// Package resume tracks live Claude Code sessions so that ones killed
// without warning — SIGKILL, kernel panic, power loss, an OS shutdown
// tearing tmux down — can be offered for resumption in the new-session
// picker.
//
// Everything lives under ~/.claude/live-sessions:
//
//   - <session-id>.json marker {cwd,pid,session}: written on every user
//     prompt (UserPromptSubmit), removed on SessionEnd. A leftover marker
//     whose pid is gone means the session died with no chance to run its
//     hooks — a crash.
//   - ended.jsonl {sid,cwd,session,reason,ended_at}: appended when a
//     session that had a marker ends normally. A clean OS shutdown *does*
//     give hooks a chance to run, so its casualties look "ended" — they
//     are told apart by time instead: the sidebar touches
//     .heartbeat-<boot-epoch> every minute, the file's mtime freezes when
//     the system stops, and entries that ended within that final window
//     of a previous boot are reclassified as shutdown casualties.
//
// List surfaces casualties without consuming them: declining an offer (by
// starting a fresh conversation) keeps the record around, and only an
// actual resume (Consume) or expiry retires it.
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

	"pier/internal/tmux"
)

const (
	endedLog       = "ended.jsonl"
	heartbeatEvery = 60 * time.Second
	// shutdownWindow is how close (seconds) to the frozen heartbeat a session
	// must have ended to count as a shutdown casualty: one heartbeat period
	// plus slack for the shutdown itself.
	shutdownWindow = 90
	// casualtyKeep is how long a casualty stays on offer. The picker never
	// consumes records the user didn't act on, so expiry is what keeps
	// declined offers from lingering forever.
	casualtyKeep = 7 * 24 * time.Hour
	// ended.jsonl is trimmed to entries younger than endedKeepDays once it
	// outgrows endedMaxSize.
	endedMaxSize  = 64 * 1024
	endedKeepDays = 14
	heartbeatKeep = 30 * 24 * time.Hour
)

type marker struct {
	CWD     string `json:"cwd"`
	PID     int    `json:"pid"`
	Session string `json:"session,omitempty"` // tmux session name, for restore
}

type endedRec struct {
	SID     string `json:"sid"`
	CWD     string `json:"cwd"`
	Session string `json:"session,omitempty"`
	Reason  string `json:"reason"`
	EndedAt int64  `json:"ended_at"`
}

type hookPayload struct {
	SessionID string `json:"session_id"`
	CWD       string `json:"cwd"`
	Reason    string `json:"reason"` // SessionEnd only
}

// Casualty is a Claude Code session that died without the user asking it to.
type Casualty struct {
	SID        string
	CWD        string
	Session    string // tmux session name at its last prompt ("" on old records)
	LastActive time.Time
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

// sessionName resolves the tmux session the hook's Claude Code runs in, ""
// outside tmux. Swappable for tests.
var sessionName = func() string {
	if os.Getenv("TMUX_PANE") == "" {
		return ""
	}
	return tmux.CurrentSession()
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
	data, err := json.Marshal(marker{CWD: p.CWD, PID: claudePID(), Session: sessionName()})
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
	data, err := os.ReadFile(mpath)
	if err != nil {
		return nil
	}
	var m marker
	_ = json.Unmarshal(data, &m)
	reason := p.Reason
	if reason == "" {
		reason = "?"
	}
	if rec, err := json.Marshal(endedRec{
		SID: p.SessionID, CWD: p.CWD, Session: m.Session,
		Reason: reason, EndedAt: time.Now().Unix(),
	}); err == nil {
		appendEnded(dir, rec)
	}
	return os.Remove(mpath)
}

// List returns every current casualty, newest first, one per directory —
// without consuming anything. Crashes (stale markers) and shutdown
// casualties (ended.jsonl near a frozen heartbeat) both count.
func List(dir string) []Casualty {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	projects := filepath.Join(home, ".claude", "projects")
	return list(dir, projects, claudeAlive, bootUnix(), time.Now())
}

func list(dir, projectsDir string, alive func(pid int) bool, boot int64, now time.Time) []Casualty {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	cutoff := now.Add(-casualtyKeep)
	var all []Casualty

	// Crashes: leftover markers whose process is gone.
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".json") {
			continue
		}
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var m marker
		if json.Unmarshal(data, &m) != nil || alive(m.PID) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		sid := strings.TrimSuffix(name, ".json")
		// A dead marker is an offer or garbage; expired and transcript-less
		// ones are reaped here, since declined offers are never consumed.
		if info.ModTime().Before(cutoff) || !hasTranscript(projectsDir, sid) {
			_ = os.Remove(path)
			continue
		}
		all = append(all, Casualty{
			SID: sid, CWD: filepath.Clean(m.CWD),
			Session: m.Session, LastActive: info.ModTime(),
		})
	}

	// Shutdown casualties: cleanly-ended sessions from a previous boot whose
	// end hugs that boot's frozen heartbeat.
	if boot > 0 {
		if beats := prevHeartbeats(dir, boot); len(beats) > 0 {
			data, _ := os.ReadFile(filepath.Join(dir, endedLog))
			for _, line := range bytes.Split(data, []byte("\n")) {
				if len(line) == 0 {
					continue
				}
				var r endedRec
				if json.Unmarshal(line, &r) != nil || !validSID(r.SID) {
					continue
				}
				at := time.Unix(r.EndedAt, 0)
				if r.EndedAt >= boot || deliberateEnd[r.Reason] || !nearAny(beats, r.EndedAt) ||
					at.Before(cutoff) || !hasTranscript(projectsDir, r.SID) {
					continue
				}
				all = append(all, Casualty{
					SID: r.SID, CWD: filepath.Clean(r.CWD),
					Session: r.Session, LastActive: at,
				})
			}
		}
	}

	// One comeback offer per directory: the most recent. Older ones stay
	// reachable via `claude -r` until they expire.
	sort.Slice(all, func(i, j int) bool { return all[i].LastActive.After(all[j].LastActive) })
	seen := map[string]bool{}
	out := all[:0]
	for _, c := range all {
		if seen[c.CWD] {
			continue
		}
		seen[c.CWD] = true
		out = append(out, c)
	}
	return out
}

// Consume erases every record of sid. Called right after a resume actually
// launches — declining an offer keeps the record, expiry retires it.
func Consume(dir, sid string) {
	if !validSID(sid) {
		return
	}
	_ = os.Remove(filepath.Join(dir, sid+".json"))
	path := filepath.Join(dir, endedLog)
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var keep []byte
	removed := false
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var r endedRec
		if json.Unmarshal(line, &r) == nil && r.SID == sid {
			removed = true
			continue
		}
		keep = append(keep, line...)
		keep = append(keep, '\n')
	}
	if removed {
		tmp := path + ".tmp"
		if os.WriteFile(tmp, keep, 0o644) == nil {
			_ = os.Rename(tmp, path)
		}
	}
}

// HeartbeatTick keeps .heartbeat-<boot-epoch> fresh; the sidebar calls it on
// its poll tick. Throttled to one touch a minute — the point is only that
// the mtime freezes near the moment the system stops running. Files from
// previous boots are left alone (List reads them) until reaped.
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
