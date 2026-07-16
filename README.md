# Pier

[한국어](README.ko.md)

**A tmux sidebar dashboard for Claude Code sessions.** A pier where your sessions dock.
See every running Claude Code session at a glance — which worktree and branch each one is on, whether it's working or waiting for you — and jump to any of them with a single click. Your tmux workflow stays exactly as it is, at **8 MB** per sidebar.

```
┌──────────────────────┬─────────────────────────────────────┐
│ Pier — CC sessions   │                                     │
│                      │                                     │
│ suite2 ⎇ feat/nmt    │                                     │
│  ● Refactor payments │        Claude Code (main pane)      │
│ suite3 ⎇ dev         │                                     │
│  ○ suite3            │                                     │
│ terminal ⎇ -         │                                     │
│  ● Build tmux dash ◀ │                                     │
└──────────────────────┴─────────────────────────────────────┘
 [session]                      [~/dev/suite2 ⎇ feat/nmt 13:53]
```

- **Sidebar**: every running Claude Code instance across all tmux sessions, grouped by worktree (path + branch). Items show the conversation's AI-generated title, falling back to the tmux session name
- **Status icons**: `●` working / `○` waiting for your input / `!` waiting for permission approval / `·` unknown
- **Jump**: click an item to focus it — even across tmux sessions
- **Status bar**: the current pane's path + git branch, always visible (`detached@sha` on a detached HEAD)
- **Auto-attach**: the sidebar appears on its own when you attach to or switch into a session

## Requirements

- tmux 3.2+ (developed and verified on 3.6a)
- Go 1.24+ (build only)
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

`pier setup` appends a marked block to `~/.tmux.conf` and merges four hook
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
    "PermissionRequest": [{ "matcher": "*", "hooks": [{ "type": "command", "command": "/Users/<you>/.local/bin/pier hook permission-request", "timeout": 5 }] }]
  }
}
```

Everything except the status icons — sidebar, jump, status bar — works without the hooks; icons just stay at `·` (unknown).

</details>

## Usage

| To do this | Do this |
|---|---|
| Jump to another session | **Click** the item in the sidebar |
| Jump with the keyboard | Focus the sidebar pane (`prefix+←`), then `j`/`k` + `Enter` |
| Show/hide the sidebar | `prefix + g`, or `pier toggle` |
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
- **Conversation titles** are read from the `ai-title` record in `~/.claude/projects/*/<session-id>.jsonl`, resolved from the hook payload's session id.

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
