package claude

import (
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"
)

func TestTelegramPreflight(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("TELEGRAM_STATE_DIR", "")
	t.Setenv(tokenVar, "")
	settings := filepath.Join(home, ".claude", "settings.json")
	env := filepath.Join(home, ".claude", "channels", "telegram", ".env")
	write := func(path, s string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	const enabled = `{"enabledPlugins":{"figma@claude-plugins-official":true,"telegram@claude-plugins-official":true}}`

	// The plugin is off until settings.json says otherwise.
	if err := TelegramPreflight(home); err != errPluginOff {
		t.Errorf("no settings: %v, want %v", err, errPluginOff)
	}
	write(settings, `{"enabledPlugins":{"telegram@claude-plugins-official":false}}`)
	if err := TelegramPreflight(home); err != errPluginOff {
		t.Errorf("plugin false: %v, want %v", err, errPluginOff)
	}
	write(settings, `{"enabledPlugins":{"figma@claude-plugins-official":true}}`)
	if err := TelegramPreflight(home); err != errPluginOff {
		t.Errorf("other plugins only: %v, want %v", err, errPluginOff)
	}
	write(settings, `{not json`)
	if err := TelegramPreflight(home); err != errPluginOff {
		t.Errorf("malformed settings: %v, want %v", err, errPluginOff)
	}

	// Enabled: the token has to be somewhere the plugin would find it.
	write(settings, enabled)
	if err := TelegramPreflight(home); err != errNoToken {
		t.Errorf("no .env: %v, want %v", err, errNoToken)
	}
	write(env, "TELEGRAM_BOT_TOKEN=\n")
	if err := TelegramPreflight(home); err != errNoToken {
		t.Errorf("empty token line: %v, want %v", err, errNoToken)
	}
	write(env, "# bot\nTELEGRAM_ACCESS_MODE=static\nTELEGRAM_BOT_TOKEN=123456:AAH-test\n")
	if err := TelegramPreflight(home); err != nil {
		t.Errorf("token in .env: %v, want ready", err)
	}
	if err := os.Remove(env); err != nil {
		t.Fatal(err)
	}
	t.Setenv(tokenVar, "999:env-wins")
	if err := TelegramPreflight(home); err != nil {
		t.Errorf("token in the environment, no file: %v, want ready", err)
	}
	t.Setenv(tokenVar, "")

	// Relocated dirs, the way the plugin honours them.
	other := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", other)
	if err := TelegramPreflight(home); err != errPluginOff {
		t.Errorf("CLAUDE_CONFIG_DIR elsewhere, no settings there: %v, want %v", err, errPluginOff)
	}
	write(filepath.Join(other, "settings.json"), enabled)
	t.Setenv("TELEGRAM_STATE_DIR", filepath.Join(other, "state"))
	if err := TelegramPreflight(home); err != errNoToken {
		t.Errorf("TELEGRAM_STATE_DIR elsewhere, empty: %v, want %v", err, errNoToken)
	}
	write(filepath.Join(other, "state", ".env"), "TELEGRAM_BOT_TOKEN=1:a\n")
	if err := TelegramPreflight(home); err != nil {
		t.Errorf("token under TELEGRAM_STATE_DIR: %v, want ready", err)
	}
}

// Both messages sit on the picker's 45-column error line behind one space.
func TestTelegramPreflightMessagesFit(t *testing.T) {
	for _, err := range []error{errPluginOff, errNoToken} {
		if n := utf8.RuneCountInString(err.Error()); n > 44 {
			t.Errorf("%q is %d cells, over 44", err, n)
		}
	}
}
