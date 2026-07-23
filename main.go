// pier — a tmux sidebar dashboard for running Claude Code sessions.
package main

import (
	"fmt"
	"io"
	"os"

	"pier/internal/discover"
	"pier/internal/setup"
	"pier/internal/state"
	"pier/internal/tmux"
	"pier/internal/ui"
)

const version = "0.6.0"

const usageText = `pier %s — tmux sidebar for Claude Code sessions

usage:
  pier setup                wire pier into ~/.tmux.conf and Claude Code hooks
  pier run                  run the sidebar TUI (inside a tmux pane)
  pier new                  new-session picker (usually via the sidebar's "+")
  pier ensure [-t session]  create the sidebar in a session if missing
  pier toggle [-t session]  toggle the sidebar pane
  pier done                 kill the current session and jump to the next
                              one in sidebar order (or just exit if alone)
  pier hook <event>         Claude Code hook endpoint (reads stdin JSON)
                              events: user-prompt-submit | pre-tool-use |
                                      stop | permission-request | session-start
  pier status <path>        print "path ⎇ branch" for tmux status-right
  pier version              print version
`

func usage() {
	fmt.Fprintf(os.Stderr, usageText, version)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "setup":
		err = setup.Run(self())
	case "run":
		err = ui.Run()
	case "new":
		err = ui.RunNew()
	case "ensure":
		err = tmux.EnsureSidebar(targetSession(), self())
	case "toggle":
		err = tmux.ToggleSidebar(targetSession(), self())
	case "done":
		err = doneCmd()
	case "hook":
		if len(os.Args) < 3 {
			usage()
			os.Exit(2)
		}
		var payload []byte
		payload, _ = io.ReadAll(os.Stdin)
		err = state.Record(state.Dir(), os.Args[2], payload)
	case "status":
		if len(os.Args) < 3 {
			usage()
			os.Exit(2)
		}
		fmt.Print(statusLine(os.Args[2]))
	case "version", "--version", "-v":
		fmt.Println(version)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "pier:", err)
		os.Exit(1)
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
