// pier — a sidebar for AI coding sessions (claude, agy, codex, grok), inside tmux.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"pier/internal/agent"
	"pier/internal/discover"
	"pier/internal/launch"
	"pier/internal/onboard"
	"pier/internal/prompt"
	"pier/internal/resume"
	"pier/internal/setup"
	"pier/internal/state"
	"pier/internal/tmux"
	"pier/internal/ui"
	"pier/internal/usage"
)

const version = "1.4.1"

const usageText = `pier %s — a sidebar for AI coding sessions (claude, agy, codex, grok), inside tmux

usage:
  pier                      start: outside tmux, pick an open session or a
                              directory and attach; inside tmux, run the
                              default agent in this pane. The first run offers
                              setup and default agent selection
  pier <agent flags...>     pass straight through to the default agent
  pier setup                wire pier into ~/.tmux.conf and agent hooks
  pier agent [name]         show or set the default agent (claude, agy, codex, grok)
  pier alias [name]         show or set an optional shell alias for pier
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
  pier hook <event>         agent hook endpoint (reads stdin JSON)
                              events: user-prompt-submit | pre-tool-use |
                                      post-tool-use | stop | permission-request |
                                      session-start | session-end
  pier statusline <agent> [--wrap <cmd>]
                              cache agent statusLine quota JSON from stdin
                              (optionally wrapping an existing script)
  pier status <path> [session] [pane] [width]
                              print usage + "path ⎇ branch" for tmux status-right
  pier version              print version
`

func printUsage() {
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
		printUsage()
		return
	case launch.KindVersion:
		fmt.Println(version)
		return
	case launch.KindUnknown:
		printUsage()
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
	case "agent":
		env := onboard.DefaultEnv(self())
		if len(args) < 2 {
			onboard.ShowAgent(os.Stdout, env)
			return nil
		}
		return onboard.SetAgent(os.Stdout, env, args[1])
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
			printUsage()
			os.Exit(2)
		}
		payload, _ := io.ReadAll(os.Stdin)
		switch {
		case state.IsAgyPreToolUse(payload):
			// PreToolUse requires an explicit decision; never silently allow
			// arbitrary tools, and never emit {} which hard-denies all calls.
			fmt.Println(`{"decision":"ask"}`)
		case state.IsAgyPayload(payload):
			fmt.Println("{}")
		}
		// Liveness bookkeeping is best-effort: a failed write must never
		// surface as a hook error inside Claude Code.
		switch args[1] {
		case "user-prompt-submit":
			_ = resume.RecordPrompt(resume.Dir(), payload)
		case "session-end":
			_ = resume.RecordEnd(resume.Dir(), payload)
		}
		return state.Record(state.Dir(), args[1], payload)
	case "statusline":
		if len(args) < 2 {
			printUsage()
			os.Exit(2)
		}
		agentName := args[1]
		wrapCmd := ""
		for i := 2; i < len(args); i++ {
			if args[i] == "--wrap" && i+1 < len(args) {
				wrapCmd = args[i+1]
				i++
			}
		}
		return usage.RunStatusLine(agentName, wrapCmd, usage.Dir(), os.Stdin, os.Stdout, os.Stderr)
	case "status":
		if len(args) < 2 {
			printUsage()
			os.Exit(2)
		}
		var session, paneID string
		width := 120
		if len(args) >= 3 {
			session = args[2]
		}
		if len(args) >= 4 {
			paneID = args[3]
		}
		if len(args) >= 5 {
			if w, err := strconv.Atoi(args[4]); err == nil && w > 0 {
				width = w
			}
		}
		fmt.Print(statusLine(args[1], session, paneID, width))
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
	return setup.StableSelf(exe)
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

// statusLine renders active agent usage (when an agent runs in session) plus
// a colored "path ⎇ branch" fragment for tmux status-right.
func statusLine(path, session, paneID string, width int) string {
	now := time.Now()
	compact := width > 0 && width < 100
	var usageFrag string

	if session != "" {
		if panes, err := tmux.ListPanes(); err == nil {
			cfg, _ := onboard.Load(onboard.ConfigPath())
			ag := activeSessionAgent(panes, session, paneID, cfg.Agents)
			if ag != "" {
				home, _ := os.UserHomeDir()
				if snap, ok := usage.Load(usage.Dir(), home, ag, now); ok {
					usageFrag = usage.FormatTmux(snap, now, compact)
				}
			}
		}
	}

	short := discover.ShortPath(path)
	if usageFrag != "" && width > 0 && width < 110 {
		home, _ := os.UserHomeDir()
		if path != home {
			base := filepath.Base(path)
			if base != "" && base != "." {
				short = base
			}
		}
	}
	branch := discover.GitBranch(path)
	return formatStatusRight(usageFrag, short, branch, tmux.IsGreenBg(tmux.StatusBg()))
}

func formatStatusRight(usageFrag, short, branch string, darkPill bool) string {
	loc := fmt.Sprintf("#[fg=cyan]%s#[default]", short)
	if branch != "-" {
		loc = fmt.Sprintf("#[fg=cyan]%s #[fg=green]⎇ %s#[default]", short, branch)
	}
	out := loc
	if usageFrag != "" {
		out = usageFrag + " #[fg=colour244]│#[default] " + loc
	}
	if !darkPill {
		return out
	}
	inner := strings.TrimSuffix(out, "#[default]")
	inner = strings.ReplaceAll(inner, "#[default]", "#[fg=default,nobold]")
	return "#[bg=colour235] " + inner + " #[default]"
}

func activeSessionAgent(panes []tmux.Pane, session, paneID string, custom []agent.Spec) string {
	if paneID != "" {
		for _, p := range panes {
			if p.ID == paneID && !p.Sidebar {
				if ag := discover.AgentNameWithCustom(p.Command, custom); ag != "" {
					return ag
				}
				break
			}
		}
	}
	if rep, ok := discover.RepresentativeWithCustom(panes, session, custom); ok {
		return discover.AgentNameWithCustom(rep.Command, custom)
	}
	return ""
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
