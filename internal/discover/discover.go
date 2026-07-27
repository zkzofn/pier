// Package discover finds running Claude Code panes (plus plain-shell
// sessions) and groups them by worktree.
package discover

import (
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"

	"pier/internal/tmux"
)

// Claude Code sets its process title to the version binary name (e.g. "2.1.206"),
// observed live on this machine; older/plain installs show "claude".
var versionRe = regexp.MustCompile(`^\d+(\.\d+)+[a-z]?$`)

// IsClaudeCommand reports whether a pane_current_command looks like Claude Code.
func IsClaudeCommand(cmd string) bool {
	return cmd == "claude" || versionRe.MatchString(cmd)
}

// Worktree is a group of Claude Code panes sharing a working directory.
type Worktree struct {
	Path      string
	Branch    string
	Instances []tmux.Pane
}

// BranchFunc resolves a directory to a branch label. Injectable for tests.
type BranchFunc func(path string) string

// paneRank orders a session's panes by how well they represent it: the
// focused pane first, then any pane in the session's current window. The
// sidebar can hold focus itself, so "current window" matters as a fallback.
func paneRank(p tmux.Pane) int {
	switch {
	case p.WindowActive && p.PaneActive:
		return 2
	case p.WindowActive:
		return 1
	}
	return 0
}

// Group filters Claude Code panes and groups them by pane_current_path.
// A session with no Claude Code pane at all still gets one entry — its most
// prominent regular pane (focused > current window > first) represents it as
// a shell instance, so plain-shell sessions stay visible and jumpable.
func Group(panes []tmux.Pane, branch BranchFunc) []Worktree {
	hasCC := map[string]bool{}
	for _, p := range panes {
		if !p.Sidebar && IsClaudeCommand(p.Command) {
			hasCC[p.Session] = true
		}
	}
	shellRep := map[string]tmux.Pane{}
	for _, p := range panes {
		if p.Sidebar || hasCC[p.Session] {
			continue
		}
		if cur, ok := shellRep[p.Session]; !ok || paneRank(p) > paneRank(cur) {
			shellRep[p.Session] = p
		}
	}
	byPath := map[string][]tmux.Pane{}
	for _, p := range panes {
		if p.Sidebar || !IsClaudeCommand(p.Command) {
			continue
		}
		byPath[p.Path] = append(byPath[p.Path], p)
	}
	for _, p := range shellRep {
		byPath[p.Path] = append(byPath[p.Path], p)
	}
	wts := make([]Worktree, 0, len(byPath))
	for path, ps := range byPath {
		sort.Slice(ps, func(i, j int) bool {
			if ps[i].Session != ps[j].Session {
				return ps[i].Session < ps[j].Session
			}
			return ps[i].ID < ps[j].ID
		})
		wts = append(wts, Worktree{Path: path, Branch: branch(path), Instances: ps})
	}
	sort.Slice(wts, func(i, j int) bool { return wts[i].Path < wts[j].Path })
	return wts
}

// SessionOrder returns unique session names in sidebar display order:
// worktrees sorted by path, instances within a worktree by session name.
func SessionOrder(wts []Worktree) []string {
	seen := map[string]bool{}
	var order []string
	for _, wt := range wts {
		for _, p := range wt.Instances {
			if !seen[p.Session] {
				seen[p.Session] = true
				order = append(order, p.Session)
			}
		}
	}
	return order
}

// NextSession picks the session to land on after closing cur: the next one
// in sidebar order (wrapping), else any other live session, else "".
func NextSession(order, all []string, cur string) string {
	for i, s := range order {
		if s == cur {
			if next := order[(i+1)%len(order)]; next != cur {
				return next
			}
			break // cur is the only sidebar session
		}
	}
	// cur not in the sidebar (or alone there): prefer the sidebar's first
	// entry, then any other live session.
	for _, s := range order {
		if s != cur {
			return s
		}
	}
	for _, s := range all {
		if s != cur {
			return s
		}
	}
	return ""
}

// GitBranch returns the current branch, "detached@<sha>" for detached HEAD,
// or "-" outside a git repository.
func GitBranch(path string) string {
	out, err := exec.Command("git", "-C", path, "branch", "--show-current").Output()
	if err != nil {
		return "-"
	}
	if b := strings.TrimSpace(string(out)); b != "" {
		return b
	}
	out, err = exec.Command("git", "-C", path, "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "-"
	}
	return "detached@" + strings.TrimSpace(string(out))
}

// ShortPath abbreviates the home directory to ~.
func ShortPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+"/") {
		return "~" + path[len(home):]
	}
	return path
}
