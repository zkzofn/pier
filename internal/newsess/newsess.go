// Package newsess builds the candidate list and naming rules for creating a
// new Claude Code session: pick a directory, the session name follows it.
package newsess

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"pier/internal/discover"
	"pier/internal/tmux"
)

// Candidate is a directory a new session could start in.
type Candidate struct {
	Path       string
	Name       string // proposed session name (directory base name, sanitized)
	Manual     bool   // synthesized from the user's typed query
	NeedsMkdir bool   // the typed path doesn't exist yet — create it first
}

// Collect gathers candidate directories: ~/dev/*, the home dir, and every
// path Claude Code was previously used in (transcript history). Directories
// hosting a live (non-sidebar) pane are left out — the picker lists those as
// open sessions instead (see OpenSessions).
func Collect(home string, panes []tmux.Pane) []Candidate {
	// Both spellings of every live pane's directory: pane_current_path is
	// resolved by the kernel, while the paths pier scans keep whatever
	// symlinks the home dir was reached through (/tmp vs /private/tmp).
	open := map[string]bool{}
	for _, p := range panes {
		if p.Sidebar {
			continue
		}
		open[p.Path] = true
		open[resolve(p.Path)] = true
	}

	seen := map[string]bool{}
	var cands []Candidate
	add := func(path string) {
		if path == "" || seen[path] {
			return
		}
		if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
			return
		}
		seen[path] = true
		if open[path] || open[resolve(path)] {
			return // a running session already holds this directory
		}
		cands = append(cands, Candidate{Path: path, Name: Sanitize(filepath.Base(path))})
	}

	entries, _ := os.ReadDir(filepath.Join(home, "dev"))
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			add(filepath.Join(home, "dev", e.Name()))
		}
	}
	for _, p := range historyPaths(home) {
		add(p)
	}
	add(home)

	sort.Slice(cands, func(i, j int) bool { return cands[i].Path < cands[j].Path })
	return cands
}

// resolve follows symlinks, falling back to the path itself when it can't
// (a directory that no longer exists, a permission error).
func resolve(path string) string {
	if r, err := filepath.EvalSymlinks(path); err == nil {
		return r
	}
	return path
}

// Open is a running tmux session as the picker lists it.
type Open struct {
	Session string
	Path    string    // the representative pane's working directory
	Pane    tmux.Pane // representative pane — the sidebar's icon rules apply to it
	Here    bool      // the session the picker was opened from
}

// OpenSessions lists running sessions in sidebar order (discover.SessionOrder:
// worktree path, then session name), each with its representative pane.
// Sessions holding only a sidebar pane are skipped. `current` — the session
// the picker was opened from; "" standalone — moves to the front, flagged
// Here, so the picker boots with the cursor on it.
func OpenSessions(panes []tmux.Pane, current string) []Open {
	groups := discover.Group(panes, func(string) string { return "" })
	var out []Open
	for _, s := range discover.SessionOrder(groups) {
		rep, ok := discover.Representative(panes, s)
		if !ok {
			continue
		}
		o := Open{Session: s, Path: rep.Path, Pane: rep, Here: s == current}
		if o.Here {
			out = append([]Open{o}, out...)
		} else {
			out = append(out, o)
		}
	}
	return out
}

// FilterOpen returns the open sessions whose name or path contains the query
// (case-insensitive). An empty query returns everything.
func FilterOpen(opens []Open, query string) []Open {
	if query == "" {
		return opens
	}
	q := strings.ToLower(query)
	var out []Open
	for _, o := range opens {
		if strings.Contains(strings.ToLower(o.Session), q) || strings.Contains(strings.ToLower(o.Path), q) {
			out = append(out, o)
		}
	}
	return out
}

// historyPaths extracts working directories from Claude Code transcripts.
func historyPaths(home string) []string {
	root := filepath.Join(home, ".claude", "projects")
	dirs, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var paths []string
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(root, d.Name()))
		if err != nil {
			continue
		}
		// newest transcript speaks for the project dir
		var newest string
		var newestMod int64
		for _, f := range files {
			if !strings.HasSuffix(f.Name(), ".jsonl") {
				continue
			}
			if info, err := f.Info(); err == nil && info.ModTime().Unix() > newestMod {
				newestMod = info.ModTime().Unix()
				newest = filepath.Join(root, d.Name(), f.Name())
			}
		}
		if newest == "" {
			continue
		}
		if cwd := cwdFromTranscript(newest); cwd != "" {
			paths = append(paths, cwd)
		}
	}
	return paths
}

func cwdFromTranscript(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for i := 0; i < 50 && sc.Scan(); i++ {
		var rec struct {
			CWD string `json:"cwd"`
		}
		if json.Unmarshal(sc.Bytes(), &rec) == nil && rec.CWD != "" {
			return rec.CWD
		}
	}
	return ""
}

// Filter returns candidates whose path or name contains the query
// (case-insensitive). An empty query returns everything.
func Filter(cands []Candidate, query string) []Candidate {
	if query == "" {
		return cands
	}
	q := strings.ToLower(query)
	var out []Candidate
	for _, c := range cands {
		if strings.Contains(strings.ToLower(c.Path), q) || strings.Contains(strings.ToLower(c.Name), q) {
			out = append(out, c)
		}
	}
	return out
}

// Manual interprets the query itself as a directory path ("~" expanded).
// A path that doesn't exist yet is still a candidate, flagged NeedsMkdir so
// the picker creates the directory before the session. Existing non-directory
// paths and relative paths are rejected.
func Manual(home, query string) *Candidate {
	if query == "" {
		return nil
	}
	path := query
	if path == "~" || strings.HasPrefix(path, "~/") {
		path = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/"))
	}
	if !filepath.IsAbs(path) {
		return nil
	}
	path = filepath.Clean(path)
	c := &Candidate{Path: path, Name: Sanitize(filepath.Base(path)), Manual: true}
	fi, err := os.Stat(path)
	switch {
	case err == nil && fi.IsDir():
		return c
	case err == nil:
		return nil // exists but is a file
	case os.IsNotExist(err):
		c.NeedsMkdir = true
		return c
	default:
		return nil
	}
}

// Sanitize makes a name usable as a tmux session name ('.' and ':' are
// reserved in tmux targets).
func Sanitize(name string) string {
	name = strings.Map(func(r rune) rune {
		if r == '.' || r == ':' {
			return '-'
		}
		return r
	}, name)
	name = strings.Trim(name, "-")
	if name == "" {
		return "session"
	}
	return name
}

// Unique appends -2, -3, … until the name is free.
func Unique(name string, taken map[string]bool) string {
	if !taken[name] {
		return name
	}
	for i := 2; ; i++ {
		candidate := name + "-" + itoa(i)
		if !taken[candidate] {
			return candidate
		}
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
