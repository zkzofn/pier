// Package claude is the single place that knows how to launch Claude Code.
// It imports nothing from pier so every other package (the picker, the
// launcher) can depend on it without cycles. Process-name detection stays in
// discover/resume; this is only about the command line pier runs.
package claude

// Bin is the Claude Code executable name looked up on PATH.
const Bin = "claude"

// telegramPlugin is the channel plugin's id in Claude Code: enabledPlugins
// key in settings.json, --channels target.
const telegramPlugin = "telegram@claude-plugins-official"

const telegramArgs = " --channels plugin:" + telegramPlugin

// Cmd is the shell command a new Claude Code pane runs; telegram attaches
// the telegram channel plugin (the picker's ^T).
func Cmd(telegram bool) string {
	if telegram {
		return Bin + telegramArgs
	}
	return Bin
}

// ResumeCmd continues the conversation sid instead of starting blank.
func ResumeCmd(telegram bool, sid string) string {
	return Cmd(telegram) + " --resume " + sid
}
