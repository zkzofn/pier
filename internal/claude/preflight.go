package claude

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const tokenVar = "TELEGRAM_BOT_TOKEN"

// The picker's one-line messages: 45 columns minus the leading space.
var (
	errPluginOff = errors.New("telegram plugin not enabled · /plugin")
	errNoToken   = errors.New("no telegram token · /telegram:configure")
)

// TelegramPreflight is the picker's ^T check. Claude Code refuses nothing
// here: with the plugin off, --channels starts a session that lists the
// channel as "plugin not installed" in a startup notice; with the plugin
// on but no bot token, the plugin's MCP server exits within a second and
// the session runs on without telegram, visible only in /mcp. So the
// picker asks first and keeps ^T off rather than launch a session that
// silently lacks what was asked for. nil means ready; the error text is
// the picker's message.
//
// Paths follow the plugin: CLAUDE_CONFIG_DIR (default ~/.claude) holds
// settings.json, whose enabledPlugins is where Claude Code turns the plugin
// on; TELEGRAM_STATE_DIR (default <config>/channels/telegram) holds .env
// with TELEGRAM_BOT_TOKEN, and the same variable in the environment wins.
// Project-level settings are not consulted.
func TelegramPreflight(home string) error {
	cfg := os.Getenv("CLAUDE_CONFIG_DIR")
	if cfg == "" {
		cfg = filepath.Join(home, ".claude")
	}
	if !pluginEnabled(filepath.Join(cfg, "settings.json")) {
		return errPluginOff
	}
	state := os.Getenv("TELEGRAM_STATE_DIR")
	if state == "" {
		state = filepath.Join(cfg, "channels", "telegram")
	}
	if os.Getenv(tokenVar) == "" && !tokenInEnvFile(filepath.Join(state, ".env")) {
		return errNoToken
	}
	return nil
}

// pluginEnabled reads enabledPlugins[telegramPlugin] == true out of a
// Claude Code settings file; unreadable or malformed counts as off.
func pluginEnabled(settingsPath string) bool {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return false
	}
	var s struct {
		EnabledPlugins map[string]any `json:"enabledPlugins"`
	}
	if json.Unmarshal(data, &s) != nil {
		return false
	}
	return s.EnabledPlugins[telegramPlugin] == true
}

// tokenInEnvFile mirrors the plugin's own .env reader — one KEY=value per
// line, no quoting — and only asks whether the token line carries a value.
func tokenInEnvFile(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, tokenVar+"="); ok && strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}
