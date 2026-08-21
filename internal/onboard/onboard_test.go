package onboard

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pier/internal/prompt"
	"pier/internal/setup"
)

func TestRCFile(t *testing.T) {
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}
	cases := []struct {
		shell, goos string
		vars        map[string]string
		want        string
	}{
		{"zsh", "darwin", nil, "/h/.zshrc"},
		{"zsh", "darwin", map[string]string{"ZDOTDIR": "/h/.config/zsh"}, "/h/.config/zsh/.zshrc"},
		{"bash", "darwin", nil, "/h/.bash_profile"},
		{"bash", "linux", nil, "/h/.bashrc"},
		{"fish", "darwin", nil, "/h/.config/fish/config.fish"},
		{"fish", "linux", map[string]string{"XDG_CONFIG_HOME": "/xdg"}, "/xdg/fish/config.fish"},
		{"tcsh", "darwin", nil, ""},
		{"", "darwin", nil, ""},
	}
	for _, c := range cases {
		if got := RCFile(c.shell, "/h", c.goos, env(c.vars)); got != c.want {
			t.Errorf("RCFile(%q,%q,%v) = %q, want %q", c.shell, c.goos, c.vars, got, c.want)
		}
	}
}

func TestValidName(t *testing.T) {
	for _, ok := range []string{"cl", "pier2", "my-pier", "_x"} {
		if !ValidName(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "2cl", "-x", "a b", "c;l", "한글"} {
		if ValidName(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

func TestConflicts(t *testing.T) {
	rc := filepath.Join(t.TempDir(), ".zshrc")
	os.WriteFile(rc, []byte("alias ll='ls -l'\ncl() { claude \"$@\"; }\nfunction gs { git status; }\n"), 0o644)
	setup.ShellAlias(rc, "pp", "pier") // pier's own block must not count
	look := func(name string) (string, error) {
		if name == "cc" {
			return "/usr/bin/cc", nil
		}
		return "", errors.New("not found")
	}
	if got := Conflicts("fresh", rc, look); len(got) != 0 {
		t.Errorf("fresh name has no conflicts: %v", got)
	}
	if got := Conflicts("pp", rc, look); len(got) != 0 {
		t.Errorf("pier's own block is not a conflict: %v", got)
	}
	for _, name := range []string{"ll", "cl", "gs"} {
		if got := Conflicts(name, rc, look); len(got) != 1 || !strings.Contains(got[0], "already defined") {
			t.Errorf("%s defined in rc should conflict: %v", name, got)
		}
	}
	if got := Conflicts("cc", rc, look); len(got) != 1 || !strings.Contains(got[0], "existing command") {
		t.Errorf("PATH command should conflict: %v", got)
	}
	if got := Conflicts("cc", "", look); len(got) != 1 {
		t.Errorf("no rc file: only the PATH check applies: %v", got)
	}
}

func TestAliasTarget(t *testing.T) {
	found := func(string) (string, error) { return "/opt/homebrew/bin/pier", nil }
	missing := func(string) (string, error) { return "", errors.New("nope") }
	if got := AliasTarget("/Users/me/.local/bin/pier", found); got != "pier" {
		t.Errorf("on PATH -> plain pier, got %q", got)
	}
	if got := AliasTarget("/Users/me/.local/bin/pier", missing); got != "/Users/me/.local/bin/pier" {
		t.Errorf("off PATH -> absolute path, got %q", got)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfg", "config.json")
	if _, ok := Load(path); ok {
		t.Error("missing config must report !ok")
	}
	if err := Save(path, Config{Alias: "cl"}); err != nil {
		t.Fatal(err)
	}
	c, ok := Load(path)
	if !ok || c.Alias != "cl" {
		t.Errorf("round trip: %+v %v", c, ok)
	}
}

func testEnv(t *testing.T, wired bool) (Env, *int) {
	t.Helper()
	dir := t.TempDir()
	env := Env{
		Home:       dir,
		Self:       filepath.Join(dir, "bin", "pier"),
		TmuxConf:   filepath.Join(dir, ".tmux.conf"),
		Settings:   filepath.Join(dir, ".claude", "settings.json"),
		RCPath:     filepath.Join(dir, ".zshrc"),
		ConfigPath: filepath.Join(dir, ".config", "pier", "config.json"),
		LookPath: func(name string) (string, error) {
			if name == "pier" {
				return "/opt/homebrew/bin/pier", nil
			}
			return "", errors.New("not found")
		},
	}
	setups := 0
	env.RunSetup = func() error { setups++; return nil }
	if wired {
		setup.TmuxConf(env.TmuxConf, env.Self)
		setup.ClaudeHooks(env.Settings, env.Self)
	}
	return env, &setups
}

func TestWizardFreshMachine(t *testing.T) {
	env, setups := testEnv(t, false)
	var out bytes.Buffer
	// setup? y · alias? y · name: Enter (=cl)
	if err := Wizard(prompt.New(strings.NewReader("y\ny\n\n"), &out), env); err != nil {
		t.Fatal(err)
	}
	if *setups != 1 {
		t.Errorf("setup should run once, ran %d", *setups)
	}
	rc, _ := os.ReadFile(env.RCPath)
	if !strings.Contains(string(rc), "alias cl='pier'") {
		t.Errorf("alias not written:\n%s", rc)
	}
	if c, ok := Load(env.ConfigPath); !ok || c.Alias != "cl" {
		t.Errorf("config should record alias cl: %+v %v", c, ok)
	}
	if !strings.Contains(out.String(), "source") {
		t.Errorf("should tell the user to reload the shell: %q", out.String())
	}
}

func TestWizardAlreadyWiredSkipsSetupQuestion(t *testing.T) {
	env, setups := testEnv(t, true)
	var out bytes.Buffer
	// alias? n  (no setup question is asked, so this answer is the alias one)
	if err := Wizard(prompt.New(strings.NewReader("n\n"), &out), env); err != nil {
		t.Fatal(err)
	}
	if *setups != 0 {
		t.Error("setup must not be offered when already wired")
	}
	if strings.Contains(out.String(), "hooks wired") {
		t.Errorf("setup question should not appear: %q", out.String())
	}
	if _, err := os.Stat(env.RCPath); err == nil {
		t.Error("declining the alias must not touch the rc file")
	}
	if c, ok := Load(env.ConfigPath); !ok || c.Alias != "" {
		t.Errorf("config should exist with an empty alias: %+v %v", c, ok)
	}
}

func TestWizardConflictThenOtherName(t *testing.T) {
	env, _ := testEnv(t, true)
	os.WriteFile(env.RCPath, []byte("cl() { claude \"$@\"; }\n"), 0o644)
	var out bytes.Buffer
	// alias? y · name: cl (conflict) · use anyway? n · name: pp
	if err := Wizard(prompt.New(strings.NewReader("y\ncl\nn\npp\n"), &out), env); err != nil {
		t.Fatal(err)
	}
	if c, _ := Load(env.ConfigPath); c.Alias != "pp" {
		t.Errorf("alias should be pp after the retry, got %q", c.Alias)
	}
	rc, _ := os.ReadFile(env.RCPath)
	if !strings.HasPrefix(string(rc), "cl() {") || !strings.Contains(string(rc), "alias pp='pier'") {
		t.Errorf("rc should keep the function and gain the alias block:\n%s", rc)
	}
}

func TestWizardUnknownShellPrintsTheLine(t *testing.T) {
	env, _ := testEnv(t, true)
	env.RCPath = ""
	var out bytes.Buffer
	Wizard(prompt.New(strings.NewReader("y\n\n"), &out), env)
	if !strings.Contains(out.String(), "alias cl='pier'") {
		t.Errorf("unknown shell: the line to add must be printed: %q", out.String())
	}
	if c, _ := Load(env.ConfigPath); c.Alias != "" {
		t.Errorf("nothing was installed, config alias should be empty, got %q", c.Alias)
	}
}

func TestSetAndShowAlias(t *testing.T) {
	env, _ := testEnv(t, true)
	var out bytes.Buffer
	if err := SetAlias(prompt.New(strings.NewReader(""), &out), env, "bad name"); err == nil {
		t.Error("invalid name must error")
	}
	if err := SetAlias(prompt.New(strings.NewReader(""), &out), env, "cl"); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	ShowAlias(&out, env)
	if !strings.Contains(out.String(), "alias: cl") {
		t.Errorf("ShowAlias = %q", out.String())
	}
	env.RCPath = filepath.Join(env.Home, "other")
	out.Reset()
	ShowAlias(&out, env)
	if !strings.Contains(out.String(), "none") {
		t.Errorf("no block -> none, got %q", out.String())
	}
}
