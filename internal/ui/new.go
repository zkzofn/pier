// The new-session picker: runs inside a tmux popup (`pier new`).
// Pick a directory → the session name follows it; Tab edits the name.
package ui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"

	"pier/internal/discover"
	"pier/internal/newsess"
	"pier/internal/tmux"
)

var (
	stylePrompt = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	stylePick   = lipgloss.NewStyle().Bold(true)
	styleOpen   = lipgloss.NewStyle().Faint(true)
	styleHint   = lipgloss.NewStyle().Faint(true)
	styleNewErr = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

const listTop = 2 // line 0: filter, line 1: blank, then the list

type pickerModel struct {
	home     string
	all      []newsess.Candidate
	filtered []newsess.Candidate
	manual   *newsess.Candidate
	taken    map[string]bool
	query    string
	cursor   int
	scroll   int
	editing  bool
	name     string
	width    int
	height   int
	errMsg   string
}

// RunNew starts the picker TUI (intended for a tmux display-popup).
func RunNew() error {
	if os.Getenv("TMUX") == "" {
		return fmt.Errorf("not inside tmux")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	panes, err := tmux.ListPanes()
	if err != nil {
		return err
	}
	m := pickerModel{
		home:   home,
		all:    newsess.Collect(home, panes),
		taken:  tmux.SessionNames(),
		width:  46,
		height: 18,
	}
	m.refilter()
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err = p.Run()
	return err
}

func (m pickerModel) Init() tea.Cmd { return nil }

func (m *pickerModel) refilter() {
	m.filtered = newsess.Filter(m.all, m.query)
	m.manual = nil
	if len(m.filtered) == 0 {
		m.manual = newsess.Manual(m.home, m.query)
	}
	if n := m.itemCount(); m.cursor >= n {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.errMsg = ""
}

func (m pickerModel) itemCount() int {
	if m.manual != nil {
		return 1
	}
	return len(m.filtered)
}

func (m pickerModel) itemAt(i int) *newsess.Candidate {
	if m.manual != nil {
		if i == 0 {
			return m.manual
		}
		return nil
	}
	if i < 0 || i >= len(m.filtered) {
		return nil
	}
	return &m.filtered[i]
}

func (m pickerModel) visibleRows() int {
	v := m.height - listTop - 2 // hint + spacer at the bottom
	if v < 1 {
		v = 1
	}
	return v
}

func (m *pickerModel) clampScroll() {
	if m.cursor < m.scroll {
		m.scroll = m.cursor
	}
	if m.cursor >= m.scroll+m.visibleRows() {
		m.scroll = m.cursor - m.visibleRows() + 1
	}
	if m.scroll < 0 {
		m.scroll = 0
	}
}

func (m pickerModel) confirm(i int) (tea.Model, tea.Cmd) {
	cand := m.itemAt(i)
	if cand == nil {
		return m, nil
	}
	if cand.Open != "" {
		_ = tmux.SwitchTo(cand.Open)
		return m, tea.Quit
	}
	name := cand.Name
	if m.editing && strings.TrimSpace(m.name) != "" {
		name = newsess.Sanitize(strings.TrimSpace(m.name))
	}
	name = newsess.Unique(name, m.taken)
	if cand.NeedsMkdir {
		if err := os.MkdirAll(cand.Path, 0o755); err != nil {
			mm := m
			mm.errMsg = "mkdir: " + err.Error()
			return mm, nil
		}
	}
	if err := tmux.NewSessionAndSwitch(name, cand.Path); err != nil {
		mm := m
		mm.errMsg = err.Error()
		return mm, nil
	}
	return m, tea.Quit
}

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.MouseMsg:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if m.cursor > 0 {
				m.cursor--
				m.clampScroll()
			}
		case tea.MouseButtonWheelDown:
			if m.cursor < m.itemCount()-1 {
				m.cursor++
				m.clampScroll()
			}
		case tea.MouseButtonLeft:
			if m.editing {
				return m, nil
			}
			i := msg.Y - listTop + m.scroll
			if m.itemAt(i) != nil {
				m.cursor = i
				return m.confirm(i)
			}
		}
		return m, nil

	case tea.KeyMsg:
		if m.editing {
			switch msg.String() {
			case "enter":
				return m.confirm(m.cursor)
			case "esc":
				m.editing = false
				m.name = ""
			case "backspace":
				if len(m.name) > 0 {
					r := []rune(m.name)
					m.name = string(r[:len(r)-1])
				}
			default:
				if msg.Type == tea.KeyRunes {
					m.name += string(msg.Runes)
				}
			}
			return m, nil
		}
		switch msg.String() {
		case "esc", "ctrl+c":
			return m, tea.Quit
		case "enter":
			return m.confirm(m.cursor)
		case "tab":
			if cand := m.itemAt(m.cursor); cand != nil && cand.Open == "" {
				m.editing = true
				m.name = newsess.Unique(cand.Name, m.taken)
			}
		case "up", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
				m.clampScroll()
			}
		case "down", "ctrl+n":
			if m.cursor < m.itemCount()-1 {
				m.cursor++
				m.clampScroll()
			}
		case "backspace":
			if len(m.query) > 0 {
				r := []rune(m.query)
				m.query = string(r[:len(r)-1])
				m.refilter()
			}
		default:
			if msg.Type == tea.KeyRunes {
				m.query += string(msg.Runes)
				m.refilter()
			}
		}
		return m, nil
	}
	return m, nil
}

func (m pickerModel) View() string {
	w := uint(m.width - 1) // ambiguous-width guard, same as the sidebar
	var b strings.Builder
	line := func(s string) {
		b.WriteString(truncate.StringWithTail(s, w, "…"))
		b.WriteString("\n")
	}

	if m.editing {
		cand := m.itemAt(m.cursor)
		path := ""
		if cand != nil {
			path = discover.ShortPath(cand.Path)
		}
		line(" " + styleHint.Render("path: "+path))
		line(" " + stylePrompt.Render("name: ") + m.name + "█")
		line("")
		if m.errMsg != "" {
			line(" " + styleNewErr.Render(m.errMsg))
		}
		line(" " + styleHint.Render("Enter: create · Esc: back"))
		return strings.TrimSuffix(b.String(), "\n")
	}

	line(" " + stylePrompt.Render("> ") + m.query + "█")
	line("")
	shown := 0
	for i := m.scroll; i < m.itemCount() && shown < m.visibleRows(); i++ {
		cand := m.itemAt(i)
		prefix, label := "  ", ""
		switch {
		case cand.Manual && cand.NeedsMkdir:
			label = "+ mkdir & create at " + discover.ShortPath(cand.Path)
		case cand.Manual:
			label = "+ create at " + discover.ShortPath(cand.Path)
		case cand.Open != "":
			label = cand.Name + styleOpen.Render("  · open: "+cand.Open)
		default:
			label = cand.Name + "  " + styleHint.Render(discover.ShortPath(cand.Path))
		}
		if i == m.cursor {
			prefix = "▸ "
			label = stylePick.Render(label)
		}
		line(prefix + label)
		shown++
	}
	if m.itemCount() == 0 {
		line("  " + styleHint.Render("no match — type a ~/ or absolute path"))
	}
	line("")
	if m.errMsg != "" {
		line(" " + styleNewErr.Render(m.errMsg))
	} else {
		line(" " + styleHint.Render("Enter: create · Tab: name · Esc"))
	}
	return strings.TrimSuffix(b.String(), "\n")
}
