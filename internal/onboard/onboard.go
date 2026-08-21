// Package onboard runs pier's first-run wizard — wire tmux/hooks, offer a
// shell alias — and owns the config file whose existence means it ran.
package onboard

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"pier/internal/prompt"
	"pier/internal/setup"
)

// Config is what the wizard recorded.
type Config struct {
	Alias string `json:"alias"` // shell alias pointing at pier ("" when skipped)
}

// ConfigPath is $XDG_CONFIG_HOME/pier/config.json (default
// ~/.config/pier/config.json). Its existence means onboarding happened.
func ConfigPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "pier", "config.json")
}

// Load reads the config; ok is false when it is missing or unreadable.
func Load(path string) (Config, bool) {
	var c Config
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &c) != nil {
		return Config{}, false
	}
	return c, true
}

// Save writes the config, creating its directory.
func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// RCFile returns the startup file an alias belongs in for the user's login
// shell (the basename of $SHELL), or "" for shells pier doesn't know. macOS
// terminals start bash as a login shell, so .bash_profile there.
func RCFile(shell, home, goos string, getenv func(string) string) string {
	switch shell {
	case "zsh":
		dir := getenv("ZDOTDIR")
		if dir == "" {
			dir = home
		}
		return filepath.Join(dir, ".zshrc")
	case "bash":
		if goos == "darwin" {
			return filepath.Join(home, ".bash_profile")
		}
		return filepath.Join(home, ".bashrc")
	case "fish":
		base := getenv("XDG_CONFIG_HOME")
		if base == "" {
			base = filepath.Join(home, ".config")
		}
		return filepath.Join(base, "fish", "config.fish")
	}
	return ""
}

var nameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

// ValidName accepts conservative alias names: letters, digits, _ and -, not
// starting with a digit or -.
func ValidName(name string) bool { return nameRe.MatchString(name) }

// Conflicts lists why `name` may already be taken — an existing PATH
// command, or a definition outside pier's block in the rc file. Functions
// sourced from other files are invisible here (a documented limitation).
func Conflicts(name, rcPath string, lookPath func(string) (string, error)) []string {
	var out []string
	if p, err := lookPath(name); err == nil {
		out = append(out, fmt.Sprintf("`%s` is an existing command (%s)", name, p))
	}
	if rcPath != "" {
		if data, err := os.ReadFile(rcPath); err == nil && definesName(setup.WithoutBlock(string(data)), name) {
			out = append(out, fmt.Sprintf("`%s` is already defined in %s", name, rcPath))
		}
	}
	return out
}

func definesName(content, name string) bool {
	q := regexp.QuoteMeta(name)
	re := regexp.MustCompile(`(?m)^\s*(alias\s+` + q + `=|` + q + `\s*\(\)|function\s+` + q + `\b)`)
	return re.MatchString(content)
}

// AliasTarget is what the alias expands to: plain `pier` when it is on PATH,
// else this binary's absolute path (source installs without ~/.local/bin on
// PATH).
func AliasTarget(self string, lookPath func(string) (string, error)) string {
	if _, err := lookPath("pier"); err == nil {
		return "pier"
	}
	return self
}

// Env is everything the wizard touches, injectable for tests.
type Env struct {
	Home       string
	Self       string
	TmuxConf   string
	Settings   string
	RCPath     string // "" = unknown shell
	ConfigPath string
	LookPath   func(string) (string, error)
	RunSetup   func() error
}

// DefaultEnv wires Env to the real home, $SHELL and setup.Run.
func DefaultEnv(self string) Env {
	home, _ := os.UserHomeDir()
	shell := ""
	if s := os.Getenv("SHELL"); s != "" {
		shell = filepath.Base(s)
	}
	return Env{
		Home:       home,
		Self:       self,
		TmuxConf:   filepath.Join(home, ".tmux.conf"),
		Settings:   filepath.Join(home, ".claude", "settings.json"),
		RCPath:     RCFile(shell, home, runtime.GOOS, os.Getenv),
		ConfigPath: ConfigPath(),
		LookPath:   exec.LookPath,
		RunSetup:   func() error { return setup.Run(self) },
	}
}

// Wizard is the first run: offer setup when tmux/hooks aren't wired, offer
// a shell alias, and record the outcome so it never asks again.
func Wizard(p *prompt.Prompter, env Env) error {
	fmt.Fprintln(p.Out, "pier — first run")
	if setup.NeedsSetup(env.TmuxConf, env.Settings) {
		if p.YesNo("pier needs tmux and Claude Code hooks wired (~/.tmux.conf, ~/.claude/settings.json — settings.json is backed up first). Do it now?", true) {
			if err := env.RunSetup(); err != nil {
				fmt.Fprintln(p.Out, "setup:", err)
			}
		} else {
			fmt.Fprintln(p.Out, "skipped — run `pier setup` later to wire tmux and hooks")
		}
	}
	alias := ""
	if p.YesNo("Add a shell alias for pier?", true) {
		alias = askAlias(p, env)
	}
	return Save(env.ConfigPath, Config{Alias: alias})
}

// askAlias prompts for a name (default cl), validates it, warns on
// conflicts, and installs it; returns the alias actually installed.
func askAlias(p *prompt.Prompter, env Env) string {
	for try := 0; try < 3; try++ {
		name := p.Line("alias name", "cl")
		if !ValidName(name) {
			fmt.Fprintf(p.Out, "invalid name %q — letters, digits, _ and - only, not starting with a digit or -\n", name)
			continue
		}
		if cs := Conflicts(name, env.RCPath, env.LookPath); len(cs) > 0 {
			fmt.Fprintln(p.Out, strings.Join(cs, "\n"))
			if !p.YesNo("Use it anyway?", false) {
				continue
			}
		}
		return Install(p.Out, env, name)
	}
	fmt.Fprintln(p.Out, "no alias set — `pier alias <name>` sets one later")
	return ""
}

// Install writes `alias name='…'` into the rc file, or prints the line when
// the shell is unknown, and reports back. Returns the name when it was
// written, "" otherwise.
func Install(out io.Writer, env Env, name string) string {
	target := AliasTarget(env.Self, env.LookPath)
	line := "alias " + name + "='" + target + "'"
	if env.RCPath == "" {
		fmt.Fprintf(out, "unknown shell — add this to your shell's startup file yourself:\n  %s\n", line)
		return ""
	}
	if _, err := setup.ShellAlias(env.RCPath, name, target); err != nil {
		fmt.Fprintf(out, "%s: %v\n", env.RCPath, err)
		return ""
	}
	rc := short(env.RCPath, env.Home)
	fmt.Fprintf(out, "%s: %s added — open a new shell or run: source %s\n", rc, line, rc)
	return name
}

// SetAlias is `pier alias <name>`: validate, warn on conflicts, install,
// record.
func SetAlias(p *prompt.Prompter, env Env, name string) error {
	if !ValidName(name) {
		return fmt.Errorf("invalid alias name %q — letters, digits, _ and - only, not starting with a digit or -", name)
	}
	if cs := Conflicts(name, env.RCPath, env.LookPath); len(cs) > 0 {
		fmt.Fprintln(p.Out, strings.Join(cs, "\n"))
		if !p.YesNo("Use it anyway?", false) {
			return nil
		}
	}
	installed := Install(p.Out, env, name)
	c, _ := Load(env.ConfigPath)
	c.Alias = installed
	return Save(env.ConfigPath, c)
}

// ShowAlias is `pier alias` with no argument: what the rc file says now.
func ShowAlias(out io.Writer, env Env) {
	name := ""
	if env.RCPath != "" {
		name = setup.AliasIn(env.RCPath)
	}
	if name == "" {
		fmt.Fprintln(out, "alias: none — `pier alias <name>` adds one")
		return
	}
	fmt.Fprintf(out, "alias: %s (%s)\n", name, short(env.RCPath, env.Home))
}

// short abbreviates home to ~ for messages.
func short(path, home string) string {
	if home != "" && strings.HasPrefix(path, home+"/") {
		return "~" + path[len(home):]
	}
	return path
}
