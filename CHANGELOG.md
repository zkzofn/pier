# Changelog

Every released version of pier, newest first. Downloads live on the
[Releases page](https://github.com/zkzofn/pier/releases); Homebrew users get
the latest with `pier upgrade` (or `brew upgrade zkzofn/tap/pier`).

Versions 0.5.0 – 0.8.0, 0.10.0 and 0.11.0 shipped as commits only — they were
never cut as GitHub releases, which is why the Releases page jumps from 0.9.0
to 1.0.0.

## 1.3.1 — 2026-09-02

- **`^T` checks before it promises.** Claude Code starts fine with
  `--channels plugin:telegram…` when the plugin is off or its bot token is
  missing — the session just never gets the channel (with no token the
  plugin's MCP server exits within a second; only `/mcp` shows it). The
  picker now keeps `^T` off in either case and says what's missing:
  `telegram plugin not enabled · /plugin` or `no telegram token ·
  /telegram:configure`. Enter and restore-all run the same check.
- The picker's hints sit on two lines: the first names what `Enter` and
  `^S` do on the `▸` row, the second the keys that don't depend on it —
  `^T: telegram`, `Tab: name` (only where there is a session to name),
  `Esc: close`. The header shows `telegram ✓` only while it's on; the
  `tg` abbreviation is gone.
- The new-session popup grew from 18 to 22 rows (17 list rows instead of
  13). `pier ensure` refreshes the tmux block on the next attach, as usual.
- Known, from the telegram plugin (0.0.7): one Claude Code session at a
  time holds the bot, and any new Claude Code session with the plugin
  enabled takes it over — the earlier session's channel goes quiet.

## 1.3.0 — 2026-08-25

- **The picker creates, it never switches.** Inside tmux the new-session
  picker now lists every running session — the one you are in on top, marked
  `▌`, with the cursor on it — and `Enter` on a running session starts
  another Claude Code session in that session's directory instead of jumping
  back into it (jumping is the sidebar's job). `^S` still opens a terminal
  there, and `Tab` now names the new session on running-session rows too.
  So `prefix+N` `Enter` is "one more Claude Code here".
- Names follow the directory on every row, `-2`, `-3`, … past taken ones: a
  third agent in `~/dev/suite1` is `suite1-3`, never `suite1-2-2`.
- Fixed: since 1.0.0 the picker dropped the session it was opened from, which
  left no row for its directory at all — no way to open a terminal or a
  second agent where you already are.
- Outside tmux (bare `pier`) `Enter` on a running session still attaches to
  it: without a sidebar, that is the way back into a session.

## 1.2.0 — 2026-08-25

- **`^S` on a running session opens a terminal there.** Since 1.0.0 the
  picker's cursor boots on the first running session, and `^S` on such a row
  silently jumped instead of creating anything — "new session → `^S`" hopped
  to a different session on every try. `^S` now means "terminal session at
  this row's directory" on every row; `Enter` still jumps back in.
- **The picker explains itself.** The bottom hint follows the cursor and says
  what `Enter`/`^S` will do to that row (jump · terminal there · resume). The
  `?` cheatsheet says keys act on the `▸` row and gains the missing `^T`
  line; both READMEs now spell out that picker keys are pressed inside the
  popup — no prefix.

## 1.1.1 — 2026-08-21

- `pier setup`'s reload line now names the tmux socket it applied to
  (`tmux: config reloaded on the running server (/private/tmp/tmux-501/default)`).
  A tmux client picks its server from `$TMUX`, `$TMUX_TMPDIR` or `-L`, so a
  reload can land on a different server than the one you were looking at.

## 1.1.0 — 2026-08-21

- **`pier upgrade`** — asks the Homebrew tap right now, without waiting for
  the daily check, and runs the upgrade. Source installs get the
  `git pull && make setup` instructions instead.
- **Sidebar update line** — while the tap carries a newer pier, every sidebar
  shows `↑ <version> · pier upgrade` at the bottom.
- The daily check used to fire only on a bare `pier`, so anyone whose habit is
  `tmux attach` + `prefix+N` never heard about a new version — and never got
  the new tmux settings either. `pier ensure`, which runs from the attach /
  session-change hook however you use pier, now quietly refreshes the managed
  `~/.tmux.conf` block (keeping the binary path it already records) and
  reloads tmux.
- Config writes are atomic, since that refresh can fire from several
  attaching clients at once.
- README gained an **Updating** section.

## 1.0.0 — 2026-08-21

`pier` itself became the way you start a session — the shell wrapper people
used to hand-roll is built in.

- **`pier`** — outside tmux it opens the picker and attaches to whatever you
  pick; inside tmux it runs `claude` in the current pane. `pier -r`,
  `pier --continue`, … pass straight through to `claude` (pier keeps only
  `-v` / `-h`), and an unknown bare word prints usage instead of starting a
  session.
- **First-run wizard** — offers to wire tmux and the Claude Code hooks (same
  as `pier setup`), then to write a shell alias (`cl` by default) as a marked
  block in your login shell's rc file (zsh / bash / fish). `pier alias <name>`
  changes it later.
- **Update check** — once a day a `pier` start compares your version against
  the Homebrew tap. `PIER_NO_UPDATE_CHECK=1` turns it off. After an upgrade
  the first run refreshes pier's `~/.tmux.conf` block, keeping whatever binary
  path it already points at, so a dev build never repoints your live config.
- **Leaving a session is clean** — `detach-on-destroy off` plus a
  `pane-exited` hook (`pier reap`) end a session the moment only its sidebar
  is left, and your client moves to the most recent session instead of being
  dropped out of tmux; plain `exit` now behaves like `prefix+x`. When the last
  session closes, the standalone attach erases tmux's `[exited]` line.
- **Picker** — running sessions are listed above the directories with the
  sidebar's own status icons; `Enter` on one jumps back into it, and the
  cursor starts there. Typing filters both groups.
- The plain-shell option (`^S`) is called a **terminal session** everywhere.
- Fixed: a directory reached through a symlink is recognised as already open,
  so it no longer appears twice in the picker.

## 0.11.0 — 2026-08-14

- Resuming a dead session became an explicit choice instead of something that
  happened to you: casualty directories carry a `↻ resume` button (`→` then
  `Enter`), plain `Enter` starts blank and keeps the offer, and an
  `↻ restore all` row rebuilds the whole pre-shutdown layout.
- The picker runs standalone outside tmux and attaches on create.
- `^T` starts the session with Claude Code's telegram channel attached.
- `pier resume-pick` is gone.

## 0.10.0 — 2026-08-10

- Sessions lost to a crash, power loss or OS shutdown can be resumed.
  `UserPromptSubmit` leaves a liveness marker (cwd + pid) in
  `~/.claude/live-sessions`; `SessionEnd` retires it into an end log. A crash
  leaves the marker with a dead pid; a clean shutdown is spotted by end-log
  entries falling within ±90 s of the sidebar heartbeat frozen before the
  current boot. Deliberate ends (`/clear`, logout, `exit`) are excluded.
- The sidebar's 2 s tick doubles as the machine heartbeat.

## 0.9.0 — 2026-08-03

- `? help` at the bottom of the sidebar (or `?`) pops up a keybinding
  cheatsheet.

## 0.8.0 — 2026-07-27

- Sessions with no Claude Code pane are listed and jumpable too, marked `$`.
- `prefix+N` opens the new-session picker from anywhere.
- `pier setup` upgrades its managed `~/.tmux.conf` block in place, so new
  bindings arrive with an upgrade.

## 0.7.0 — 2026-07-27

- `^S` in the picker starts a plain shell session instead of Claude Code.

## 0.6.1 — 2026-07-23

- `prefix+X` → `prefix+x` for `pier done`, overriding tmux's default
  kill-pane binding by explicit choice — panes close via `exit` / `Ctrl-D`.

## 0.6.0 — 2026-07-23

- `pier done` switches the client to the next session in sidebar order
  (wrapping, falling back to any live session) and then kills the old one;
  alone it just kills, leaving tmux. It refuses to kill when the switch fails,
  so a client is never stranded.

## 0.5.0 — 2026-07-23

- Sidebar rows show the prompt each session is working on, captured from the
  `UserPromptSubmit` hook.
- A fifth hook, `SessionStart`: startup and `/clear` reset the label and mark
  the pane fresh (`compact` is ignored). Fresh panes render blank on purpose —
  `/clear` carries the old ai-title into the new transcript, so a title lookup
  would resurrect the previous conversation's label.
- Label preference: last prompt → transcript title (unless fresh) → blank; the
  tmux session name only when no Claude state exists. The transcript title is
  the `ai-title` record, else the first real user message — which labels
  subagent panes with the task they were assigned.

## 0.4.1 — 2026-07-20

- Typing a path that doesn't exist offers `+ mkdir & create at …`, creating
  the directory before starting the session. A distinct label, so a typo'd
  path is noticed before a directory appears.

## 0.4.0 — 2026-07-20

- The new-session picker: `+ new session` in the sidebar (or `n`) opens a tmux
  popup listing candidate directories (`~/dev/*`, past Claude Code paths,
  home). Filter-as-you-type doubles as manual path entry, `Enter` creates a
  session named after the directory (`Tab` renames, `-2` suffix on collision),
  and picking an already-open path jumps to it.

## 0.3.2 — 2026-07-20

- Fixed ghost duplicate lines: frames drawn mid-reflow could wrap inside the
  30-column pane and leave artifacts the renderer couldn't diff away. The
  sidebar now renders against its fixed width and clears the screen on resize.
- The current session gets a full-row highlight with an edge bar.
- The keyboard cursor stays hidden until the first `j`/`k` — clicking is the
  everyday path.

## 0.3.1 — 2026-07-16

- `pier setup` checks for tmux before touching anything, and the Claude Code
  requirement is documented.

## 0.3.0 — 2026-07-16

- `pier setup` wires tmux and Claude Code in one shot, idempotently: a marked
  block in `~/.tmux.conf`, hook entries merged into `settings.json` preserving
  existing config, a `.bak-pier` backup, and a live reload.
- `make setup` = build + install + setup.

## 0.2.0 — 2026-07-16

- First public version: worktree-grouped session sidebar, click and keyboard
  jump, hook-driven status icons, path + branch in the status bar,
  auto-attach.
