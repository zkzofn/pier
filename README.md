# Pier

[한국어](README.ko.md)

**A tmux sidebar dashboard for Claude Code sessions.** A pier where your sessions dock.
See every running Claude Code session at a glance — which worktree and branch each one is on, whether it's working or waiting for you — and jump to any of them with a single click. Your tmux workflow stays exactly as it is, at **8 MB** per sidebar.

```
┌──────────────────────┬─────────────────────────────────────┐
│ Pier — sessions      │                                     │
│                      │                                     │
│ suite2 ⎇ feat/nmt    │                                     │
│  ● Refactor payments │        Claude Code (main pane)      │
│ suite3 ⎇ dev         │                                     │
│  ○ suite3            │                                     │
│ terminal ⎇ -         │                                     │
│  $ terminal        ◀ │                                     │
└──────────────────────┴─────────────────────────────────────┘
 [session]                      [~/dev/suite2 ⎇ feat/nmt 13:53]
```

- **Sidebar**: every running Claude Code instance across all tmux sessions, grouped by worktree (path + branch). Items show the conversation's AI-generated title, falling back to the tmux session name. Sessions with no Claude Code pane at all are listed too, marked `$` — plain shells stay visible and jumpable
- **Status icons**: `●` working / `○` waiting for your input / `!` waiting for permission approval / `·` unknown / `$` plain shell session
- **Jump**: click an item to focus it — even across tmux sessions
- **Status bar**: the current pane's path + git branch, always visible (`detached@sha` on a detached HEAD)
- **Auto-attach**: the sidebar appears on its own when you attach to or switch into a session
- **New session**: click `+ new session`, press `n` in the sidebar, or `prefix+N` from anywhere — pick a directory in a centered popup and a Claude Code session starts there, named after the directory. `^S` instead of `Enter` starts a plain shell session — for when you want a terminal without splitting a pane or opening a new window
- **Auto-resume**: a Claude Code session lost to a crash, power loss, or OS shutdown isn't gone — the next session created in that directory starts with `claude --resume <that conversation>` instead of blank
- **Help**: click `? help` at the bottom of the sidebar (or press `?`) — a popup with every key above, so none of this needs memorizing

## Requirements

- [Claude Code](https://claude.com/claude-code) — the thing being dashboarded
- tmux 3.2+ (developed and verified on 3.6a; installed automatically with Homebrew)
- Go 1.24+ only when building from source — the Homebrew install ships a prebuilt binary
- Developed and verified on macOS. Linux should work but is unverified.

## Install

### Homebrew

```sh
brew install zkzofn/tap/pier
pier setup
```

### From source

```sh
git clone https://github.com/zkzofn/pier.git
cd pier
make setup          # build + install to ~/.local/bin + pier setup
```

`pier setup` appends a marked block to `~/.tmux.conf` and merges six hook
entries into `~/.claude/settings.json` — existing settings are preserved and a
`.bak-pier` backup is written first. Re-running it never duplicates anything.
If a tmux server is running, the config is reloaded on the spot.

<details>
<summary>Manual setup — exactly what <code>pier setup</code> does</summary>

### 1. tmux config

Add to `~/.tmux.conf`:

```tmux
set -g mouse on
set -g status-interval 5
set -g status-right-length 60
set -g status-right '#(~/.local/bin/pier status "#{pane_current_path}") %H:%M '

# Auto-create the sidebar on attach / session switch (no-op if it exists)
set-hook -g client-attached 'run-shell "~/.local/bin/pier ensure"'
set-hook -g client-session-changed 'run-shell "~/.local/bin/pier ensure"'

# prefix + g : toggle the sidebar
bind-key g run-shell "~/.local/bin/pier toggle"

# prefix + x : kill the current session, jump to the next one (sidebar order)
# (overrides the default kill-pane binding; close panes with exit/Ctrl-D)
bind-key x run-shell "~/.local/bin/pier done"

# prefix + N : open the new-session picker
bind-key N display-popup -E -w 46 -h 18 -T " New session " "~/.local/bin/pier new"
```

Apply with `tmux source-file ~/.tmux.conf`.

### 2. Claude Code hooks (for status icons)

Add to `hooks` in `~/.claude/settings.json` (replace `<you>` in the paths):

```json
{
  "hooks": {
    "UserPromptSubmit": [{ "hooks": [{ "type": "command", "command": "/Users/<you>/.local/bin/pier hook user-prompt-submit", "timeout": 5 }] }],
    "PreToolUse": [{ "matcher": "*", "hooks": [{ "type": "command", "command": "/Users/<you>/.local/bin/pier hook pre-tool-use", "timeout": 5 }] }],
    "Stop": [{ "hooks": [{ "type": "command", "command": "/Users/<you>/.local/bin/pier hook stop", "timeout": 5 }] }],
    "PermissionRequest": [{ "matcher": "*", "hooks": [{ "type": "command", "command": "/Users/<you>/.local/bin/pier hook permission-request", "timeout": 5 }] }],
    "SessionStart": [{ "hooks": [{ "type": "command", "command": "/Users/<you>/.local/bin/pier hook session-start", "timeout": 5 }] }],
    "SessionEnd": [{ "hooks": [{ "type": "command", "command": "/Users/<you>/.local/bin/pier hook session-end", "timeout": 5 }] }]
  }
}
```

Everything except the status icons, prompt labels, and auto-resume — sidebar, jump, status bar — works without the hooks; icons just stay at `·` (unknown).

</details>

## Usage

| To do this | Do this |
|---|---|
| Jump to another session | **Click** the item in the sidebar |
| Create a new session | Click `+ new session` (`n` with the sidebar focused, or `prefix+N` from anywhere). Type to filter directories (`~/dev/*` + past Claude Code paths), `Enter` to create, `^S` to create with a plain shell instead of Claude Code, `Tab` to edit the proposed name. Typing a path that doesn't exist offers `mkdir & create`; picking an already-open path jumps to it instead |
| Jump with the keyboard | Focus the sidebar pane (`prefix+←`), then `j`/`k` + `Enter` |
| Key cheatsheet | Click `? help` at the bottom of the sidebar, or `?` with it focused — any key closes the popup |
| Show/hide the sidebar | `prefix + g`, or `pier toggle` |
| Finish the current session | `prefix + x` (or `pier done`) — kills the session and jumps to the next one in sidebar order; with no other session it just exits. No confirmation — the kill is immediate. Overrides tmux's default kill-pane binding |
| Force refresh | `r` in the sidebar |
| Quit the sidebar | `q` in the sidebar |

tmux delivers keystrokes to the focused pane, so `j`/`k` only work while the sidebar has focus. Clicking is the everyday path.

## How it works

```
┌─ tmux session (one sidebar per session) ──────┐
│ ┌────────┐  ┌──────────────────────────────┐  │
│ │  pier  │  │  Claude Code                 │  │
│ │  run   │  │                              │  │
│ └───┬────┘  └──────────────┬───────────────┘  │
└─────┼──────────────────────┼──────────────────┘
      │ poll every 2s:       │ hooks
      │ tmux list-panes -a   ▼
      │◀─ fsnotify ── ~/.local/state/pier/panes/<pane>.json
```

- **tmux is the source of truth.** Every 2 seconds the sidebar polls `list-panes -a` and detects Claude Code panes (process name `claude`, or a version string like `2.1.206`). State files are decoration only — even if the hooks die, the list stays correct.
- **Status is recorded by Claude Code hooks.** `pier hook <event>` runs as a child of Claude Code, so it inherits `$TMUX_PANE` and maps to the exact pane. The TUI picks changes up instantly via fsnotify.
- **Item labels show the current prompt.** Each instance row shows the prompt you last submitted in that pane (captured from the `UserPromptSubmit` hook payload). `/clear` starts a new session (`SessionStart` hook), which blanks the label until the next prompt. When no prompt has been recorded yet (fresh install, resume), the label falls back to the session's `ai-title` record in `~/.claude/projects/*/<session-id>.jsonl`, then to the tmux session name.
- **Auto-resume.** Every `UserPromptSubmit` also writes a liveness marker (`~/.claude/live-sessions/<session-id>.json` with the session's cwd and pid); `SessionEnd` retires it into an end log. When the picker starts a Claude Code session, pier looks for a casualty in that directory and appends `--resume <id>`:
  - *Crash* (SIGKILL, kernel panic, power loss): hooks never ran, so the marker is still there with a dead pid.
  - *Clean OS shutdown*: hooks did run and the marker is gone — instead, a session whose end log entry falls within ±90 s of the sidebar's frozen heartbeat (a file the sidebar touches once a minute) from before the current boot counts as a shutdown casualty. Ends the user asked for (`/clear`, logout, exit at the prompt) are excluded.

  `pier resume-pick <dir>` exposes the same decision to shell scripts and wrappers: it prints the session id to resume and consumes the record (prints nothing when there is none). Caveats: shutdown detection requires a sidebar to have been running at shutdown time (it owns the heartbeat), and a session you closed by hand in the last ~90 s before a shutdown can be picked up as a false positive — resuming it is harmless but unasked-for.

## Memory usage (measured)

macOS (Apple Silicon), RSS. tmux's model means one sidebar per session.

| Item | Measured |
|---|---|
| One sidebar | 7.1 – 9.7 MB (avg 8.1 MB) |
| Total with 6 sessions | 48.8 MB |
| `pier hook` / `pier status` | No resident process (a few ms per event) |
| Binary size | 5 MB |

## Troubleshooting

- **Sidebar pane is dead** → `prefix + g` twice (kill, then recreate)
- **See what discovery sees** → `go run ./cmd/dbg`
- **Mouse text selection stopped working** → the `mouse on` trade-off; in iTerm2 use `Option+drag` for native selection
- **All status icons are `·`** → hooks not configured, or a long-running CC session hasn't received a new prompt since the hooks were added — recording starts with the next prompt

## License

MIT
