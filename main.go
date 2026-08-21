// pier — a tmux sidebar dashboard for running Claude Code sessions.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"pier/internal/discover"
	"pier/internal/launch"
	"pier/internal/onboard"
	"pier/internal/prompt"
	"pier/internal/resume"
	"pier/internal/setup"
	"pier/internal/state"
	"pier/internal/tmux"
	"pier/internal/ui"
)

const version = "1.1.1"

const usageText = `pier %s — tmux sidebar for Claude Code sessions

usage:
  pier                      start: outside tmux, pick an open session or a
                              directory and attach; inside tmux, run claude
                              in this pane. The first run offers setup and
                              a shell alias; upgrades are announced here
  pier <claude flags...>    pass straight through to claude (e.g. pier -r)
  pier setup                wire pier into ~/.tmux.conf and Claude Code hooks
  pier alias [name]         show or set the shell alias for pier
  pier upgrade              check the Homebrew tap now and upgrade
  pier run                  run the sidebar TUI (inside a tmux pane)
  pier new                  new-session picker (sidebar "+" / prefix+N /
                              standalone outside tmux — attaches on create)
  pier keys                 keybinding cheatsheet (sidebar "?")
  pier ensure [-t session]  create the sidebar in a session if missing
  pier toggle [-t session]  toggle the sidebar pane
  pier done                 kill the current session and jump to the next
                              one in sidebar order (or just exit if alone)
  pier reap                 kill sessions left with only a sidebar pane
                              (tmux pane-exited hook)
  pier hook <event>         Claude Code hook endpoint (reads stdin JSON)
                              events: user-prompt-submit | pre-tool-use |
                                      stop | permission-request |
                                      session-start | session-end
  pier status <path>        print "path ⎇ branch" for tmux status-right
  pier version              print version
`

func usage() {
	fmt.Fprintf(os.Stderr, usageText, version)
}

func main() {
	args := os.Args[1:]
	var err error
	switch launch.Classify(args) {
	case launch.KindLaunch:
		err = launch.Run(version, self())
	case launch.KindPassthrough:
		err = launch.Passthrough(args)
	case launch.KindHelp:
		usage()
		return
	case launch.KindVersion:
		fmt.Println(version)
		return
	case launch.KindUnknown:
		usage()
		os.Exit(2)
	case launch.KindSubcommand:
		err = subcommand(args)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "pier:", err)
		os.Exit(1)
	}
}

func subcommand(args []string) error {
	switch args[0] {
	case "setup":
		return setup.Run(self())
	case "alias":
		env := onboard.DefaultEnv(self())
		if len(args) < 2 {
			onboard.ShowAlias(os.Stdout, env)
			return nil
		}
		return onboard.SetAlias(prompt.Std(), env, args[1])
	case "upgrade":
		return launch.Upgrade(version, self())
	case "run":
		ui.Version = version
		return ui.Run()
	case "new":
		attach, err := ui.RunNew()
		if err != nil || attach == "" {
			return err
		}
		return tmux.Attach(attach)
	case "keys":
		return ui.RunKeys()
	case "ensure":
		// The attach / session-change hook is the one thing that runs no
		// matter how someone uses pier, so it is where an upgraded binary
		// gets its tmux block refreshed — people who never type bare `pier`
		// would otherwise keep the old block forever. Silent: run-shell
		// shows any output in view mode.
		refreshTmuxBlockQuietly(self())
		return tmux.EnsureSidebar(targetSession(), self())
	case "toggle":
		return tmux.ToggleSidebar(targetSession(), self())
	case "done":
		return doneCmd()
	case "reap":
		reapCmd()
		return nil
	case "hook":
		if len(args) < 2 {
			usage()
			os.Exit(2)
		}
		payload, _ := io.ReadAll(os.Stdin)
		// Liveness bookkeeping is best-effort: a failed write must never
		// surface as a hook error inside Claude Code.
		switch args[1] {
		case "user-prompt-submit":
			_ = resume.RecordPrompt(resume.Dir(), payload)
		case "session-end":
			_ = resume.RecordEnd(resume.Dir(), payload)
		}
		return state.Record(state.Dir(), args[1], payload)
	case "status":
		if len(args) < 2 {
			usage()
			os.Exit(2)
		}
		fmt.Print(statusLine(args[1]))
	}
	return nil
}

// refreshTmuxBlockQuietly brings pier's managed ~/.tmux.conf block up to
// date (keeping the binary path it already records) and reloads the server
// when it changed. Every failure is ignored: this runs from a hook.
func refreshTmuxBlockQuietly(self string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	conf := filepath.Join(home, ".tmux.conf")
	if msg, err := setup.RefreshTmuxBlock(conf, self); err == nil && msg != "" {
		setup.ReloadTmux(conf)
	}
}

func targetSession() string {
	if len(os.Args) >= 4 && os.Args[2] == "-t" {
		return os.Args[3]
	}
	return tmux.CurrentSession()
}

func self() string {
	exe, err := os.Executable()
	if err != nil {
		return "pier"
	}
	return exe
}

// doneCmd kills the current session after switching the client to the next
// session in sidebar order. With no other session it just kills — the client
// leaves tmux, matching "close the last one".
func doneCmd() error {
	cur := tmux.CurrentSession()
	if cur == "" {
		return fmt.Errorf("not inside tmux")
	}
	panes, err := tmux.ListPanes()
	if err != nil {
		return err
	}
	// Ordering only — skip branch resolution (one git call per worktree).
	groups := discover.Group(panes, func(string) string { return "" })
	next := discover.NextSession(discover.SessionOrder(groups), tmux.AllSessions(), cur)
	if next != "" {
		// Don't kill if the switch failed: that would strand the client.
		if err := tmux.SwitchTo(next); err != nil {
			return err
		}
	}
	return tmux.KillSession(cur)
}

// statusLine renders a colored "path ⎇ branch" fragment for tmux status-right.
func statusLine(path string) string {
	short := discover.ShortPath(path)
	branch := discover.GitBranch(path)
	if branch == "-" {
		return fmt.Sprintf("#[fg=cyan]%s#[default]", short)
	}
	return fmt.Sprintf("#[fg=cyan]%s #[fg=green]⎇ %s#[default]", short, branch)
}

// reapCmd kills every session left with only a sidebar pane. It runs from
// the pane-exited hook, so it is context-free — the hook's formats don't
// reliably name the dead pane's session — and never fails: fire-and-forget.
func reapCmd() {
	panes, err := tmux.ListPanes()
	if err != nil {
		return
	}
	for _, s := range tmux.SidebarOnlySessions(panes) {
		_ = tmux.KillSession(s)
	}
}
