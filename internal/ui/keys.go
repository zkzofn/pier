// The keybinding cheatsheet: runs inside a tmux popup (`pier keys`),
// opened from the sidebar's "? help" row. Any key or click closes it.
package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"pier/internal/state"
)

type keysModel struct{}

// RunKeys shows the static cheatsheet until any key or mouse press.
func RunKeys() error {
	p := tea.NewProgram(keysModel{}, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}

func (keysModel) Init() tea.Cmd { return nil }

func (m keysModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m, tea.Quit
	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (keysModel) View() string { return keysContent() }

// keysContent renders the cheatsheet. Keep every line within the popup
// width (46, see tmux.OpenKeysPopup) — there is no truncation here.
func keysContent() string {
	section := stylePrompt.Render
	key := stylePick.Render
	dim := styleHint.Render
	icon := func(st, glyph string) string {
		if s, ok := iconStyles[st]; ok {
			return s.Render(glyph)
		}
		return styleDim.Render(glyph)
	}

	lines := []string{
		" " + section("tmux") + dim("  (prefix: ctrl+b)"),
		"  " + key("prefix N") + "    new-session picker",
		"  " + key("prefix g") + "    show / hide the sidebar",
		"  " + key("prefix x") + "    kill session, jump to next",
		"  " + key("prefix ←") + "    focus the sidebar pane",
		"",
		" " + section("sidebar"),
		"  " + key("click") + "       jump to that session",
		"  " + key("j / k") + "       move · " + key("Enter") + " jump",
		"  " + key("n") + "           new-session picker",
		"  " + key("r") + "           refresh now",
		"  " + key("q") + "           hide the sidebar",
		"  " + key("?") + "           this help",
		"",
		" " + section("picker"),
		"  " + key("type") + "        filter dirs, or a ~/ path",
		"  " + key("Enter") + "       create Claude Code session",
		"  " + key("^S") + "          create terminal session",
		"  " + key("Tab") + "         edit the session name",
		"",
		" " + section("icons"),
		"  " + icon(state.Working, "●") + " working      " + icon(state.Waiting, "○") + " waiting for you",
		"  " + icon(state.Permission, "!") + " permission   " + styleDim.Render("·") + " unknown",
		"  " + styleDim.Render("$") + " terminal session",
		"",
		" " + dim("any key to close"),
	}
	return strings.Join(lines, "\n")
}
