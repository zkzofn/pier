// Package launch is the bare `pier` entry point — what the shell alias
// (e.g. `cl`) runs: first-run wizard, daily update check, tmux block
// refresh, then the picker (outside tmux) or claude in place (inside).
package launch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"pier/internal/claude"
	"pier/internal/onboard"
	"pier/internal/prompt"
	"pier/internal/setup"
	"pier/internal/tmux"
	"pier/internal/ui"
	"pier/internal/update"
)

// Kind is how main should treat the command line.
type Kind int

const (
	KindLaunch      Kind = iota // no args: the launcher
	KindPassthrough             // leading -flag: straight to claude
	KindHelp
	KindVersion
	KindSubcommand // a known pier subcommand
	KindUnknown    // a bare word pier doesn't know — usage, not claude
)

// subcommands are pier's own verbs; keep in sync with main.go's switch.
var subcommands = map[string]bool{
	"setup": true, "alias": true, "run": true, "new": true, "keys": true,
	"ensure": true, "toggle": true, "done": true, "reap": true, "hook": true, "status": true,
}

// Classify decides what the arguments mean. Flags go to claude so `pier -r`
// works like `claude -r` (the alias's whole point), except pier's own
// -v/-h; unknown bare words are a usage error rather than a claude launch,
// so typos don't start a session.
func Classify(args []string) Kind {
	if len(args) == 0 {
		return KindLaunch
	}
	switch args[0] {
	case "-h", "--help", "help":
		return KindHelp
	case "-v", "--version", "version":
		return KindVersion
	}
	if strings.HasPrefix(args[0], "-") {
		return KindPassthrough
	}
	if subcommands[args[0]] {
		return KindSubcommand
	}
	return KindUnknown
}

// Passthrough replaces this process with `claude args...`.
func Passthrough(args []string) error {
	path, err := exec.LookPath(claude.Bin)
	if err != nil {
		return fmt.Errorf("%s not found in PATH", claude.Bin)
	}
	return syscall.Exec(path, append([]string{claude.Bin}, args...), os.Environ())
}

// Run is bare `pier`: wizard (once) → update check (daily) → tmux block
// refresh → outside tmux, the picker and an attach; inside, claude in this
// pane. version is pier's own, self its executable path.
func Run(version, self string) error {
	if _, err := exec.LookPath("tmux"); err != nil {
		return fmt.Errorf("tmux not found in PATH — install it first (e.g. `brew install tmux`), then re-run `pier`")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if prompt.IsTerminal() {
		p := prompt.Std()
		if _, ok := onboard.Load(onboard.ConfigPath()); !ok {
			if err := onboard.Wizard(p, onboard.DefaultEnv(self)); err != nil {
				fmt.Fprintln(os.Stderr, "pier: first-run wizard:", err)
			}
		}
		if checkUpdate(p, version, self) {
			return reexec()
		}
	}
	tmuxConf := filepath.Join(home, ".tmux.conf")
	if msg, err := setup.RefreshTmuxBlock(tmuxConf, self); err == nil && msg != "" {
		fmt.Println(msg)
		if setup.ReloadTmux(tmuxConf) {
			fmt.Println("tmux: config reloaded on the running server")
		}
	}
	if os.Getenv("TMUX") != "" {
		return Passthrough(nil)
	}
	attach, err := ui.RunNew()
	if err != nil || attach == "" {
		return err
	}
	return tmux.Attach(attach)
}

// checkUpdate runs the daily tap check and, for brew installs, offers the
// upgrade. Returns true when the binary was replaced and must be re-exec'd.
func checkUpdate(p *prompt.Prompter, version, self string) bool {
	if os.Getenv("PIER_NO_UPDATE_CHECK") != "" {
		return false
	}
	url := update.FormulaURL
	if u := os.Getenv("PIER_UPDATE_URL"); u != "" {
		url = u
	}
	now := time.Now()
	latest, notify := update.Check(update.CachePath(), url, version, now, func(u string) (string, error) {
		return update.Fetch(u, 2*time.Second)
	})
	if !notify {
		return false
	}
	update.MarkNotified(update.CachePath(), now)
	if !update.IsBrewInstall(self) {
		fmt.Fprintf(p.Out, "pier %s is available (you have %s) — https://github.com/zkzofn/pier#install\n", latest, version)
		return false
	}
	if !p.YesNo(fmt.Sprintf("pier %s is available (you have %s). Upgrade with Homebrew now?", latest, version), false) {
		return false
	}
	if err := update.Upgrade(p.Out); err != nil {
		fmt.Fprintln(p.Out, "upgrade failed:", err)
		return false
	}
	fmt.Fprintf(p.Out, "upgraded to %s — restarting\n", latest)
	return true
}

// reexec restarts pier through PATH so the freshly installed binary runs.
func reexec() error {
	exe, err := exec.LookPath("pier")
	if err != nil {
		exe = os.Args[0]
	}
	return syscall.Exec(exe, os.Args, os.Environ())
}
