# Pier

[한국어](README.ko.md)

**A tmux sidebar dashboard for Claude Code sessions.** A pier where your sessions dock.
See every running Claude Code session at a glance — which worktree and branch each one is on, whether it's working or waiting for you — and jump to any of them with a single click. Your tmux workflow stays exactly as it is, at **8 MB** per sidebar.

```
+----------------------+-------------------------------------+
| Pier - sessions      |                                     |
|                      |                                     |
| suite2 (feat/nmt)    |                                     |
|  * Refactor payments |       Claude Code (main pane)       |
| suite3 (dev)         |                                     |
|  o suite3            |                                     |
| terminal (-)         |                                     |
|  $ terminal        < |                                     |
+----------------------+-------------------------------------+
 [session]                     [~/dev/suite2 (feat/nmt) 13:53]
```

- **Sidebar**: every running Claude Code instance across all tmux sessions, grouped by worktree (path + branch). Items show the conversation's AI-generated title, falling back to the tmux session name. Sessions with no Claude Code pane at all are listed too, marked `$` — terminal sessions stay visible and jumpable
- **Status icons**: `●` working / `○` waiting for your input / `!` waiting for permission approval / `·` unknown / `$` terminal session (a plain shell)
- **Jump**: click an item to focus it — even across tmux sessions
- **Status bar**: the current pane's path + git branch, always visible (`detached@sha` on a detached HEAD)
- **Auto-attach**: the sidebar appears on its own when you attach to or switch into a session
- **`pier` is the front door**: outside tmux it opens the picker and attaches to whatever you pick; inside tmux it runs `claude` in the current pane; `pier -r` and friends pass straight through to `claude`. The first run offers to wire tmux and the hooks and to add a shell alias (`cl` by default). Once a day it checks the Homebrew tap for a newer pier and offers to upgrade
- **New session**: click `+ new session`, press `n` in the sidebar, or `prefix+N` from anywhere — the picker lists your running sessions first (`Enter` jumps back into one), then directories: pick one and a Claude Code session starts there, named after the directory. `^S` instead of `Enter` starts a terminal session (a plain shell, no Claude Code) — for when you want a terminal without splitting a pane or opening a new window. Picker keys are pressed inside the popup itself, no prefix, and every key acts on the highlighted `▸` row: on a running session, `Enter` jumps back in while `^S` opens a terminal in that session's directory. Outside tmux, plain `pier` opens the same picker in your terminal
- **Resume**: a Claude Code session lost to a crash, power loss, or OS shutdown isn't gone — its directory grows a `↻ resume` button in the picker. `→` then `Enter` continues that conversation under its old session name; plain `Enter` starts blank and keeps the offer around. With any casualties present, an `↻ restore all` row rebuilds the whole pre-shutdown layout in one stroke
- **Telegram**: `^T` in the picker (a `tg✓` badge shows it's on) starts the session with Claude Code's telegram channel attached
- **Update notice**: while the Homebrew tap carries a newer pier, every sidebar shows an `↑ <version> · pier upgrade` line at the bottom
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
pier
```

### From source

```sh
git clone https://github.com/zkzofn/pier.git
cd pier
make setup          # build + install to ~/.local/bin + pier setup
pier
```

The first `pier` run is a short wizard.

It offers to wire tmux and Claude Code — the same thing as `pier setup`: a
marked block appended to `~/.tmux.conf` and six hook entries merged into
`~/.claude/settings.json`. Existing settings are preserved, a `.bak-pier`
backup of `settings.json` is written first, re-running never duplicates
anything, and a running tmux server is reloaded on the spot.

Then it offers a shell alias — `cl` by default — written as a marked block
into your login shell's startup file (`~/.zshrc`, `~/.bash_profile` on macOS /
`~/.bashrc` on Linux, or fish's `config.fish`). Say `n` to either and run
`pier setup` / `pier alias <name>` whenever you like.

After an upgrade, the first `pier` run refreshes pier's block in
`~/.tmux.conf` and reloads tmux (`tmux.conf: pier block updated`). The block
keeps whichever binary path it already points at, so a second copy of pier
never repoints your live config.

## Updating

```sh
brew update && brew upgrade zkzofn/tap/pier
```

or, from inside pier, `pier upgrade` — it asks the tap right now (no waiting
for the daily check) and runs the Homebrew upgrade for you. Source installs:
`git pull && make setup` in your checkout.

You normally don't have to remember any of this. A `pier` start checks the tap
once a day and says so, and every sidebar carries an `↑ <version> · pier
upgrade` line while a newer one is out. `PIER_NO_UPDATE_CHECK=1` silences both.

Upgrading never leaves you on a stale config: the next time pier's tmux hook
runs — which is every attach or session switch — it brings its `~/.tmux.conf`
block up to date and reloads tmux, keeping whichever binary path the block
already points at.

What changed in each version: [CHANGELOG.md](CHANGELOG.md).

<details>
<summary>Manual setup — exactly what <code>pier setup</code> does</summary>

### 1. tmux config

Add to `~/.tmux.conf`:

```tmux
set -g mouse on

# A finished session moves the client to the most recent one, instead of
# dropping it out of tmux (`exit` then behaves like prefix+x)
set -g detach-on-destroy off

set -g status-interval 5
set -g status-right-length 60
set -g status-right '#(~/.local/bin/pier status "#{pane_current_path}") %H:%M '

# Auto-create the sidebar on attach / session switch (no-op if it exists)
set-hook -g client-attached 'run-shell "~/.local/bin/pier ensure"'
set-hook -g client-session-changed 'run-shell "~/.local/bin/pier ensure"'

# Kill a session the moment only its sidebar is left
set-hook -g pane-exited 'run-shell "~/.local/bin/pier reap"'

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

Everything except the status icons, prompt labels, and crash recovery — sidebar, jump, status bar — works without the hooks; icons just stay at `·` (unknown).

</details>

## Usage

Type `pier` (or your alias, e.g. `cl`):

- **Outside tmux** — the picker opens: running sessions on top (`Enter` jumps back into one), then directories to start a new Claude Code session in. Whatever you pick, pier attaches to it. When the last session ends you are back at your prompt, with no `[exited]` line left behind.
- **Inside tmux** — `claude` starts in the current pane, exactly like typing `claude`.
- **`pier -r`, `pier --continue`, …** — anything starting with `-` goes straight to `claude` (except pier's own `-v` / `-h`). A bare word pier doesn't know is a usage error, so a typo never starts a session.

Once a day, on a `pier` start, pier checks the Homebrew tap for a newer version and offers to upgrade — `PIER_NO_UPDATE_CHECK=1` turns it off, and source installs get a one-line notice instead of the prompt.

| To do this | Do this |
|---|---|
| Jump to another session | **Click** the item in the sidebar |
| Create a new session | Click `+ new session` (`n` with the sidebar focused, or `prefix+N` from anywhere; outside tmux just run `pier`). Type to filter — the query matches both running sessions and directories (`~/dev/*` + past Claude Code paths). `Enter` to create, `^S` to create a terminal session (a plain shell) instead of Claude Code, `^T` to attach the telegram channel, `Tab` to edit the proposed name. Typing a path that doesn't exist offers `mkdir & create`. Running sessions are listed on top — `Enter` on one jumps to it, `^S` opens a terminal in its directory. Every key acts on the highlighted `▸` row, pressed inside the popup itself (no prefix) |
| Resume a dead session | In the picker, directories with a crashed / shutdown-lost conversation show `↻ resume`: `→` then `Enter` continues it; plain `Enter` starts blank and keeps the offer. `↻ restore all` (top row, one `↑` away) brings every casualty back as its own session |
| Jump with the keyboard | Focus the sidebar pane (`prefix+←`), then `j`/`k` + `Enter` |
| Change or add the shell alias | `pier alias <name>`; `pier alias` alone shows the current one |
| Update pier | `pier upgrade` (or `brew upgrade zkzofn/tap/pier`) |
| Key cheatsheet | Click `? help` at the bottom of the sidebar, or `?` with it focused — any key closes the popup |
| Show/hide the sidebar | `prefix + g`, or `pier toggle` |
| Finish the current session | `prefix + x` (or `pier done`) — kills the session and jumps to the next one in sidebar order; with no other session it just exits. No confirmation — the kill is immediate. Overrides tmux's default kill-pane binding. Plain `exit` in a session's last pane does the same: the session closes at once and your client moves to the most recently used one |
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
- **Crash & shutdown recovery.** Every `UserPromptSubmit` also writes a liveness marker (`~/.claude/live-sessions/<session-id>.json` with the session's cwd, pid, and tmux session name); `SessionEnd` retires it into an end log. The picker flags a directory as holding a casualty when either:
  - *Crash* (SIGKILL, kernel panic, power loss): hooks never ran, so the marker is still there with a dead pid.
  - *Clean OS shutdown*: hooks did run and the marker is gone — instead, a session whose end log entry falls within ±90 s of the sidebar's frozen heartbeat (a file the sidebar touches once a minute) from before the current boot counts as a shutdown casualty. Ends the user asked for (`/clear`, logout, exit at the prompt) are excluded.

  Resuming is always an explicit choice — the `↻ resume` button or `↻ restore all`, never automatic. Only an actual resume consumes the record; starting blank leaves it for later. Records expire after 7 days, or as soon as their conversation transcript is gone. Caveats: shutdown detection requires a sidebar to have been running at shutdown time (it owns the heartbeat), and a session you closed by hand in the last ~90 s before a shutdown can show up as a false positive — an offer that's harmless to ignore.

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
- **`cl` (or your alias) runs something else** → the wizard only checks your rc file and `PATH`; a function of the same name sourced from another file can win or lose depending on order. `pier alias <other-name>` picks a different one
- **All status icons are `·`** → hooks not configured, or a long-running CC session hasn't received a new prompt since the hooks were added — recording starts with the next prompt

## License

MIT
