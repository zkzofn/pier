// Package discover finds running Claude Code panes and groups them by worktree.
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

// Group filters Claude Code panes and groups them by pane_current_path.
func Group(panes []tmux.Pane, branch BranchFunc) []Worktree {
	byPath := map[string][]tmux.Pane{}
	for _, p := range panes {
		if p.Sidebar || !IsClaudeCommand(p.Command) {
			continue
		}
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
