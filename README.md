# Pier

[한국어](README.ko.md)

**A sidebar for your AI coding sessions (`claude`, `agy`, `codex`, `grok`), inside tmux.** A pier where your sessions dock.
See every running agent session at a glance — which worktree and branch each one is on, whether it's working or waiting for you — and jump to any of them with a single click. Your tmux workflow stays exactly as it is, at **15 MB** per sidebar.

```
+----------------------+-------------------------------------+
| Pier - sessions      |                                     |
|                      |                                     |
| web (feat/search)    |                                     |
|  * Refactor payments |       AI agent (main pane)          |
| api (dev)            |                                     |
|  o api               |                                     |
| terminal (-)         |                                     |
|  $ terminal        < |                                     |
+----------------------+-------------------------------------+
 [session]    [claude 5h:82% | ~/dev/web (feat/search) 13:53]
```

- **Sidebar**: every running AI CLI instance (`claude`, `agy`, `codex`, `grok`, plus custom `agents` in `~/.config/pier/config.json`) across all tmux sessions, grouped by worktree (path + branch). Items show the current prompt or conversation title, falling back to the tmux session name. When multiple agent types run at once, rows display a compact agent badge (`cl:`, `agy:`, `cdx:`, `grk:`). Long lists scroll via mouse wheel or `j`/`k`. Sessions with no agent pane at all are listed too, marked `$` — terminal sessions stay visible and jumpable
- **Status icons**: `●` working / `○` waiting for your input / `!` waiting for permission approval / `·` unknown / `$` terminal session (a plain shell)
- **Jump**: click an item to focus it — even across tmux sessions
- **Status bar**: shows the current session's active agent remaining quota (`claude` `5h`/`7d`, `agy` `5h`/`7d`/`Argon`, `codex` `5h`/`7d`) alongside the current pane's path + git branch (`detached@sha` on a detached HEAD)
- **Auto-attach**: the sidebar appears on its own when you attach to or switch into a session
- **`pier` is the front door**: outside tmux it opens the picker and attaches to whatever you pick; inside tmux it runs your configured default agent (`pier agent`) in the current pane; `pier -r` and friends pass straight through to the default agent. The first run offers to wire tmux, agent hooks (`~/.claude/settings.json`, `~/.gemini/config/hooks.json`), and `statusLine` quota caching, and lets you choose a default agent (`claude`, `agy`, `codex`, `grok`); existing configs with no `agent` set yet prompt once on the next `pier` start. Once a day it checks the Homebrew tap for a newer pier and offers to upgrade
- **New session**: click `+ new session`, press `n` in the sidebar, or `prefix+N` from anywhere — the picker lists your running sessions first, the one you are in on top and marked `▌`, then directories (`~/dev/*`, past Claude Code paths, and Antigravity history). It creates, it never switches (that is the sidebar's job): `Enter` on a running session starts another agent session in that session's directory, `Enter` on a directory starts one there — named after the directory, with a `-2`, `-3`, … suffix when the name is taken. Press `^A` to cycle between installed agents (`claude`, `agy`, `codex`, `grok`, or custom `agents`), or `^S` instead of `Enter` to start a terminal session (a plain shell) at the row. Picker keys are pressed inside the popup itself, no prefix, and every key acts on the highlighted `▸` row; the cursor starts on your current session, so `prefix+N` `Enter` is "one more agent here". Outside tmux, plain `pier` opens the same picker in your terminal, where `Enter` on a running session attaches to it instead
- **Resume**: a Claude Code session lost to a crash, power loss, or OS shutdown isn't gone — its directory grows a `↻ resume` button in the picker. `→` then `Enter` continues that conversation under its old session name; plain `Enter` starts blank and keeps the offer around. With any casualties present, an `↻ restore all` row rebuilds the whole pre-shutdown layout in one stroke
- **Telegram**: `^T` in the picker (Claude Code only; a `telegram ✓` badge shows it's on) starts the session with Claude Code's telegram channel attached. `^T` first checks that the plugin is enabled and a bot token is configured and otherwise says what's missing (`/plugin`, `/telegram:configure`) — Claude Code itself would start fine and just never attach the channel. Plugin caveat (0.0.7): one Claude Code session at a time holds the bot, and any new Claude Code session with the plugin enabled takes it over
- **Update notice**: while the Homebrew tap carries a newer pier, every sidebar shows an `↑ <version> · pier upgrade` line at the bottom
- **Help**: click `? help` at the bottom of the sidebar (or press `?`) — a popup with every key above, so none of this needs memorizing

## Requirements

- At least one supported AI coding CLI ([Claude Code](https://claude.com/claude-code), [Antigravity CLI](https://antigravity.google) `agy`, `codex`, or `grok`)
- tmux 3.2+ (developed and verified on 3.6a; installed automatically with Homebrew)
- Go 1.24+ only when building from source — the Homebrew install ships a prebuilt binary
- Developed and verified on macOS. Linux should work but is unverified.

## Install

### Homebrew

```sh
brew tap zkzofn/tap
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

It offers to wire tmux, agent hooks, and status-bar quota caching — the same thing as `pier setup`:
- a marked block appended to `~/.tmux.conf`
- hook entries merged into `~/.claude/settings.json` and (when `agy` or `~/.gemini` is present) `~/.gemini/config/hooks.json`
- `statusLine` configured in `~/.claude/settings.json` (wrapping any existing `statusLine` script transparently) and `~/.gemini/antigravity-cli/settings.json` so the tmux status bar displays remaining 5-hour / weekly quotas for the active session's agent

Existing settings are preserved, a `.bak-pier` backup is written before the first modification, re-running never duplicates anything, and a running tmux server is reloaded on the spot. Note that `claude` and `agy` read `statusLine` config only on startup, so restart any already-running sessions to see their usage in the status bar.

When multiple agents are installed on `PATH`, the wizard also lets you pick the default agent (`pier agent <name>` changes it later). If you are upgrading from a version before 1.4.0 and already have `~/.config/pier/config.json` without an `agent` field, `pier` prompts once for your default agent on the next launch while preserving any existing alias or custom agents.

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
set -g status-right-length 140
set -g status-right '#(~/.local/bin/pier status "#{pane_current_path}" "#{session_name}" "#{pane_id}" "#{client_width}") %H:%M '

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
bind-key N display-popup -E -w 46 -h 22 -T " New session " "~/.local/bin/pier new"
```

Apply with `tmux source-file ~/.tmux.conf`.

### 2. Claude Code hooks (for status icons)

Add to `hooks` in `~/.claude/settings.json` (use `pier` if on `PATH`, or full path):

```json
{
  "hooks": {
    "UserPromptSubmit": [{ "hooks": [{ "type": "command", "command": "pier hook user-prompt-submit", "timeout": 5 }] }],
    "PreToolUse": [{ "matcher": "*", "hooks": [{ "type": "command", "command": "pier hook pre-tool-use", "timeout": 5 }] }],
    "Stop": [{ "hooks": [{ "type": "command", "command": "pier hook stop", "timeout": 5 }] }],
    "PermissionRequest": [{ "matcher": "*", "hooks": [{ "type": "command", "command": "pier hook permission-request", "timeout": 5 }] }],
    "SessionStart": [{ "hooks": [{ "type": "command", "command": "pier hook session-start", "timeout": 5 }] }],
    "SessionEnd": [{ "hooks": [{ "type": "command", "command": "pier hook session-end", "timeout": 5 }] }]
  }
}
```

### 3. Antigravity (`agy`) hooks (optional, for `agy` status icons)

Add under `"pier"` in `~/.gemini/config/hooks.json`:

```json
{
  "pier": {
    "SessionStart": [{ "type": "command", "command": "pier hook session-start", "timeout": 5 }],
    "PreInvocation": [{ "type": "command", "command": "pier hook user-prompt-submit", "timeout": 5 }],
    "PreToolUse": [{ "matcher": "*", "hooks": [{ "type": "command", "command": "pier hook pre-tool-use", "timeout": 5 }] }],
    "PostToolUse": [{ "matcher": "*", "hooks": [{ "type": "command", "command": "pier hook pre-tool-use", "timeout": 5 }] }],
    "Stop": [{ "type": "command", "command": "pier hook stop", "timeout": 5 }]
  }
}
```

### 4. Status-bar quota caching (optional)

- In `~/.claude/settings.json`, set `"statusLine": { "type": "command", "command": "pier statusline claude" }` (add `--wrap '<your-existing-cmd>'` to preserve an existing status line script).
- In `~/.gemini/antigravity-cli/settings.json`, set `"statusLine": { "type": "command", "command": "pier statusline agy", "stack_with_default": true }`.

Everything except status icons, prompt labels, status-bar quota, and crash recovery — sidebar, jump, path/branch status bar — works without hooks or `statusLine`; icons just stay at `·` (unknown).

</details>

## Usage

Type `pier` (or an optional alias if you set one via `pier alias`):

- **Outside tmux** — the picker opens: running sessions on top (`Enter` attaches to one — the one place the picker joins a session instead of creating), then directories to start a new session in. Whatever you pick, pier attaches to it. When the last session ends you are back at your prompt, with no `[exited]` line left behind.
- **Inside tmux** — your configured default agent (`pier agent`) starts in the current pane.
- **`pier -r`, `pier --continue`, …** — anything starting with `-` passes straight through to your default agent (except pier's own `-v` / `-h`). A bare word pier doesn't know is a usage error, so a typo never starts a session.

Once a day, on a `pier` start, pier checks the Homebrew tap for a newer version and offers to upgrade — `PIER_NO_UPDATE_CHECK=1` turns it off, and source installs get a one-line notice instead of the prompt.

| To do this | Do this |
|---|---|
| Jump to another session | **Click** the item in the sidebar |
| Create a new session | Click `+ new session` (`n` with the sidebar focused, or `prefix+N` from anywhere; outside tmux just run `pier`). Type to filter — the query matches both running sessions and directories (`~/dev/*` + past Claude Code & Antigravity paths). `Enter` to create, `^A` to cycle between installed agents, `^S` to create a terminal session (a plain shell), `^T` to attach the telegram channel (Claude Code only), `Tab` to edit the proposed name. Typing a path that doesn't exist offers `mkdir & create`. Running sessions are listed on top, your current one first (`▌`) with the cursor on it — `Enter` on a running session starts another agent session in its directory (`-2`, `-3`, … suffix), `^S` a terminal there; inside tmux the picker never switches sessions (click the sidebar for that), outside tmux `Enter` on a running session attaches to it. Every key acts on the highlighted `▸` row, pressed inside the popup itself (no prefix) |
| Resume a dead session | In the picker, directories with a crashed / shutdown-lost Claude Code conversation show `↻ resume`: `→` then `Enter` continues it; plain `Enter` starts blank and keeps the offer. `↻ restore all` (top row, one `↑` away) brings every casualty back as its own session |
| Jump or scroll with the keyboard | Focus the sidebar pane (`prefix+←`), then `j`/`k` (auto-scrolls long lists) + `Enter`; mouse wheel also scrolls the sidebar |
| Change default agent | `pier agent <name>` (`claude`, `agy`, `codex`, `grok`, or a custom name); `pier agent` alone shows current and available agents |
| Add custom agents | Add `"agents": [{"name": "mycli", "cmd": "mycli --fast"}]` in `~/.config/pier/config.json` (can also override a built-in `cmd`) |
| Change or add a shell alias | `pier alias <name>`; `pier alias` alone shows the current one |
| Update pier | `pier upgrade` (or `brew upgrade zkzofn/tap/pier`) |
| Key cheatsheet | Click `? help` at the bottom of the sidebar, or `?` with it focused — any key closes the popup |
| Show/hide the sidebar | `prefix + g`, or `pier toggle` |
| Finish the current session | `prefix + x` (or `pier done`) — kills the session and jumps to the next one in sidebar order; with no other session it just exits. No confirmation — the kill is immediate. Overrides tmux's default kill-pane binding. Plain `exit` in a session's last pane does the same: the session closes at once and your client moves to the most recently used one |
| Force refresh | `r` in the sidebar |
| Quit the sidebar | `q` in the sidebar |

tmux delivers keystrokes to the focused pane, so `j`/`k` only work while the sidebar has focus. Clicking and wheel scrolling work without focusing.

## How it works

```
┌─ tmux session (one sidebar per session) ──────┐
│ ┌────────┐  ┌──────────────────────────────┐  │
│ │  pier  │  │  AI CLI (claude/agy/…)       │  │
│ │  run   │  │                              │  │
│ └───┬────┘  └──────────────┬───────────────┘  │
└─────┼──────────────────────┼──────────────────┘
      │ poll every 2s:       │ hooks / statusLine
      │ tmux list-panes -a   ▼
      │◀─ fsnotify ── ~/.local/state/pier/panes/<pane>.json
```

- **tmux is the source of truth.** Every 2 seconds the sidebar polls `list-panes -a` and detects AI CLI panes (`claude` or a version string like `2.1.206`, `agy`, `codex`, `grok`, plus any custom `agents` in `~/.config/pier/config.json`). State files are decoration only — even if the hooks die, the list stays correct.
- **Status is recorded by hooks.** `pier hook <event>` runs as a child of Claude Code (`~/.claude/settings.json`) or Antigravity (`~/.gemini/config/hooks.json`), inheriting `$TMUX_PANE` so it maps to the exact pane. The TUI picks changes up instantly via fsnotify.
- **Item labels show the current prompt.** Each instance row shows the prompt you last submitted in that pane. `/clear` starts a new Claude Code session (`SessionStart` hook), which blanks the label until the next prompt. When no prompt has been recorded yet (fresh install, resume), the label falls back to Claude Code's `ai-title` record (or first user prompt) in `~/.claude/projects/*/<session-id>.jsonl`, Antigravity's `~/.gemini/antigravity-cli/cache/conversation_metadata.json`, and finally the tmux session name.
- **Status-bar quota caching.** `pier statusline claude` and `pier statusline agy` cache quota JSON payloads locally under `~/.local/state/pier/usage/<agent>.json` (preserving existing `statusLine` output when `--wrap` is used). `codex` reads recent primary/secondary rate limits from `~/.codex/sessions/**/rollout-*.jsonl`. `grok` omits quota display. `pier status` formats the active session agent's remaining quota alongside the current path and git branch.
- **Crash & shutdown recovery (Claude Code).** Every `UserPromptSubmit` also writes a liveness marker (`~/.claude/live-sessions/<session-id>.json` with the session's cwd, pid, and tmux session name); `SessionEnd` retires it into an end log. The picker flags a directory as holding a casualty when either:
  - *Crash* (SIGKILL, kernel panic, power loss): hooks never ran, so the marker is still there with a dead pid.
  - *Clean OS shutdown*: hooks did run and the marker is gone — instead, a session whose end log entry falls within ±90 s of the sidebar's frozen heartbeat (a file the sidebar touches once a minute) from before the current boot counts as a shutdown casualty. Ends the user asked for (`/clear`, logout, exit at the prompt) are excluded.

  Resuming is always an explicit choice — the `↻ resume` button or `↻ restore all`, never automatic. Only an actual resume consumes the record; starting blank leaves it for later. Records expire after 7 days, or as soon as their conversation transcript is gone. Caveats: shutdown detection requires a sidebar to have been running at shutdown time (it owns the heartbeat), and a session you closed by hand in the last ~90 s before a shutdown can show up as a false positive — an offer that's harmless to ignore.

## Memory usage (measured)

Measured on an iMac Pro (Intel Xeon W-2140B, 8 cores / 16 threads), macOS 15.7.9,
Go 1.26.5, with eight tmux sessions live. RSS, the number `ps` and Activity
Monitor report. tmux's model means one sidebar per session.

| Item | Measured |
|---|---|
| One sidebar | 14 – 16 MB |
| Six sidebars | 91.6 MB |
| `pier hook` / `pier status` | No resident process (a few ms per event) |
| Binary size | 10.6 MB |

A sidebar reaches that level within seconds of starting and stays there — the six
above had been up for an hour and measured 14.6 – 15.6 MB each. Through 1.3.1 it
instead climbed to 40 – 90 MB over the first few minutes, scaling with the
machine's core count; see the 1.4.0 entry in the [changelog](CHANGELOG.md).

## Troubleshooting

- **Sidebar pane is dead** → `prefix + g` twice (kill, then recreate)
- **See what discovery sees** → `go run ./cmd/dbg`
- **Mouse text selection stopped working** → the `mouse on` trade-off; in iTerm2 use `Option+drag` for native selection
- **An alias runs something else** → `pier alias <other-name>` picks a different one; functions sourced from other files can shadow aliases depending on load order
- **All status icons are `·`** → hooks not configured (run `pier setup`), or an already-running session hasn't received a new prompt since hooks were added — recording starts with the next prompt
- **Status bar doesn't show agent usage** → run `pier setup` and restart any already-running `claude` / `agy` sessions (`statusLine` config is read on startup). For `codex`, usage appears once a session rollout log records rate limits

## License

MIT
