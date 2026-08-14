// The new-session picker: runs inside a tmux popup (`pier new`) or
// standalone in a plain terminal (`cl` outside tmux — attaches on exit).
// Pick a directory → the session name follows it; Tab edits the name.
// Enter starts Claude Code there, ctrl+s a plain shell.
//
// Directories where a session died (crash, power loss, OS shutdown) carry a
// ↻ resume button: →/Enter resumes that conversation, plain Enter starts
// blank and leaves the record alone. With any casualties present, a
// "restore all" row on top brings back the whole pre-shutdown layout.
package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/reflow/ansi"
	"github.com/muesli/reflow/truncate"

	"pier/internal/discover"
	"pier/internal/newsess"
	"pier/internal/resume"
	"pier/internal/tmux"
)

var (
	stylePrompt = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	stylePick   = lipgloss.NewStyle().Bold(true)
	styleOpen   = lipgloss.NewStyle().Faint(true)
	styleHint   = lipgloss.NewStyle().Faint(true)
	styleNewErr = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styleResume = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleTg     = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
)

// tmux draws East-Asian-ambiguous runes (…, ↻, ▸) one cell wide regardless
// of locale, but go-runewidth — the ruler behind reflow's truncate and
// PrintableRuneWidth — switches them to two cells under CJK LANGs. Pin the
// ruler to what tmux renders so padding and truncation stay cell-accurate.
func init() { runewidth.DefaultCondition.EastAsianWidth = false }

const listTop = 2 // line 0: filter, line 1: blank, then the list

// resume button faces; same cell width, so focus never shifts the row
const (
	btnResume      = " ↻ resume "
	btnResumeFocus = "[↻ resume]"
)

// what the new session's main pane runs
const cmdShell = "" // empty: tmux's default shell

type pickerModel struct {
	home       string
	insideTmux bool
	all        []newsess.Candidate
	filtered   []newsess.Candidate
	manual     *newsess.Candidate
	taken      map[string]bool
	casualties map[string]resume.Casualty // by cleaned cwd
	query      string
	cursor     int
	scroll     int
	onResume   bool // focus sits on the cursor row's ↻ resume button
	telegram   bool // ^T: start claude with the telegram channel attached
	editing    bool
	name       string
	attach     string // standalone: session to exec-attach after the TUI exits
	width      int
	height     int
	errMsg     string
}

// RunNew starts the picker TUI — in a tmux display-popup or standalone.
func RunNew() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	insideTmux := os.Getenv("TMUX") != ""
	panes, err := tmux.ListPanes()
	if err != nil {
		if insideTmux {
			return err
		}
		panes = nil // no tmux server yet — nothing is open, that's fine
	}
	casualties := map[string]resume.Casualty{}
	for _, c := range resume.List(resume.Dir()) {
		casualties[c.CWD] = c
	}
	m := pickerModel{
		home:       home,
		insideTmux: insideTmux,
		all:        newsess.Collect(home, panes),
		taken:      tmux.SessionNames(),
		casualties: casualties,
		width:      46,
		height:     18,
	}
	m.refilter()
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	final, err := p.Run()
	if err != nil {
		return err
	}
	if fm, ok := final.(pickerModel); ok && fm.attach != "" {
		return tmux.AttachExec(fm.attach)
	}
	return nil
}

func (m pickerModel) Init() tea.Cmd { return nil }

func (m *pickerModel) refilter() {
	m.filtered = newsess.Filter(m.all, m.query)
	m.manual = nil
	if len(m.filtered) == 0 {
		m.manual = newsess.Manual(m.home, m.query)
	}
	// Land on the first directory, never the restore-all row: a reflexive
	// Enter at boot must not resume every casualty at once (it stays one ↑
	// away).
	m.cursor = 0
	if m.restoreRow() && m.itemCount() > 1 {
		m.cursor = 1
	}
	m.scroll = 0
	m.onResume = false
	m.errMsg = ""
}

// restoreRow reports whether the "restore all" row is shown (list top).
func (m pickerModel) restoreRow() bool {
	return m.query == "" && m.manual == nil && len(m.casualties) > 0
}

// isRestoreAll reports whether list index i is the restore-all row.
func (m pickerModel) isRestoreAll(i int) bool {
	return m.restoreRow() && i == 0
}

func (m pickerModel) itemCount() int {
	n := len(m.filtered)
	if m.manual != nil {
		n = 1
	}
	if m.restoreRow() {
		n++
	}
	return n
}

func (m pickerModel) itemAt(i int) *newsess.Candidate {
	if m.restoreRow() {
		if i == 0 {
			return nil // the restore-all row is not a directory
		}
		i--
	}
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

// casualtyFor returns the dead session waiting in cand's directory, if any.
func (m pickerModel) casualtyFor(cand *newsess.Candidate) (resume.Casualty, bool) {
	if cand == nil || cand.Manual || cand.Open != "" {
		return resume.Casualty{}, false
	}
	c, ok := m.casualties[filepath.Clean(cand.Path)]
	return c, ok
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

// claudeCmd is what a new Claude Code pane runs, ^T state included.
func (m pickerModel) claudeCmd() string {
	if m.telegram {
		return "claude --channels plugin:telegram@claude-plugins-official"
	}
	return "claude"
}

// launch creates the session — switching to it inside tmux, deferring an
// exec-attach when standalone.
func (m pickerModel) launch(name, path, command string) (tea.Model, tea.Cmd) {
	if m.insideTmux {
		if err := tmux.NewSessionAndSwitch(name, path, command); err != nil {
			m.errMsg = err.Error()
			return m, nil
		}
	} else {
		if err := tmux.NewSessionDetached(name, path, command); err != nil {
			m.errMsg = err.Error()
			return m, nil
		}
		m.attach = name
	}
	return m, tea.Quit
}

func (m pickerModel) confirm(i int, shell bool) (tea.Model, tea.Cmd) {
	if m.isRestoreAll(i) {
		if shell {
			return m, nil
		}
		return m.restoreAll()
	}
	cand := m.itemAt(i)
	if cand == nil {
		return m, nil
	}
	if cand.Open != "" {
		if m.insideTmux {
			_ = tmux.SwitchTo(cand.Open)
			return m, tea.Quit
		}
		m.attach = cand.Open
		return m, tea.Quit
	}

	command := m.claudeCmd()
	name := cand.Name
	resumeSID := ""
	if !shell && m.onResume {
		// The ↻ button: continue the dead conversation, under its old
		// session name when we know it.
		if c, ok := m.casualtyFor(cand); ok {
			command += " --resume " + c.SID
			resumeSID = c.SID
			if c.Session != "" {
				name = c.Session
			}
		}
	}
	if shell {
		command = cmdShell
	}
	if m.editing && strings.TrimSpace(m.name) != "" {
		name = strings.TrimSpace(m.name)
	}
	name = newsess.Unique(newsess.Sanitize(name), m.taken)

	if cand.NeedsMkdir {
		if err := os.MkdirAll(cand.Path, 0o755); err != nil {
			m.errMsg = "mkdir: " + err.Error()
			return m, nil
		}
	}
	mm, cmd := m.launch(name, cand.Path, command)
	if resumeSID != "" && mm.(pickerModel).errMsg == "" {
		// Only an actual resume retires the record; starting blank (plain
		// Enter) leaves the offer around until it expires.
		resume.Consume(resume.Dir(), resumeSID)
	}
	return mm, cmd
}

// restoreAll recreates every casualty as its own session (old name when
// recorded, else the directory name) and jumps to the most recent one.
// Failed ones keep their records and show up again next time.
func (m pickerModel) restoreAll() (tea.Model, tea.Cmd) {
	cs := make([]resume.Casualty, 0, len(m.casualties))
	for _, c := range m.casualties {
		cs = append(cs, c)
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].LastActive.After(cs[j].LastActive) })

	dir := resume.Dir()
	newest := ""
	var failed []string
	for _, c := range cs {
		base := c.Session
		if base == "" {
			base = filepath.Base(c.CWD)
		}
		name := newsess.Unique(newsess.Sanitize(base), m.taken)
		if err := tmux.NewSessionDetached(name, c.CWD, m.claudeCmd()+" --resume "+c.SID); err != nil {
			failed = append(failed, filepath.Base(c.CWD))
			continue
		}
		m.taken[name] = true
		resume.Consume(dir, c.SID)
		if newest == "" {
			newest = name
		}
	}
	if newest == "" {
		m.errMsg = "restore failed: " + strings.Join(failed, ", ")
		return m, nil
	}
	if m.insideTmux {
		_ = tmux.SwitchTo(newest)
	} else {
		m.attach = newest
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
				m.onResume = false
				m.clampScroll()
			}
		case tea.MouseButtonWheelDown:
			if m.cursor < m.itemCount()-1 {
				m.cursor++
				m.onResume = false
				m.clampScroll()
			}
		case tea.MouseButtonLeft:
			if m.editing {
				return m, nil
			}
			i := msg.Y - listTop + m.scroll
			if m.isRestoreAll(i) {
				m.cursor = i
				return m.restoreAll()
			}
			if cand := m.itemAt(i); cand != nil {
				m.cursor = i
				_, has := m.casualtyFor(cand)
				m.onResume = has && msg.X >= m.width-1-ansi.PrintableRuneWidth(btnResume)
				return m.confirm(i, false)
			}
		}
		return m, nil

	case tea.KeyMsg:
		if m.editing {
			switch msg.String() {
			case "enter":
				return m.confirm(m.cursor, false)
			case "ctrl+s":
				return m.confirm(m.cursor, true)
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
			return m.confirm(m.cursor, false)
		case "ctrl+s":
			return m.confirm(m.cursor, true)
		case "ctrl+t":
			m.telegram = !m.telegram
		case "right":
			if _, ok := m.casualtyFor(m.itemAt(m.cursor)); ok {
				m.onResume = true
			}
		case "left":
			m.onResume = false
		case "tab":
			if cand := m.itemAt(m.cursor); cand != nil && cand.Open == "" {
				m.editing = true
				m.onResume = false
				m.name = newsess.Unique(cand.Name, m.taken)
			}
		case "up", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
				m.onResume = false
				m.clampScroll()
			}
		case "down", "ctrl+n":
			if m.cursor < m.itemCount()-1 {
				m.cursor++
				m.onResume = false
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
		// reflow reserves the tail's cells before comparing, so it would trim
		// rows that fit exactly; only truncate genuine overflow.
		if ansi.PrintableRuneWidth(s) > int(w) {
			s = truncate.StringWithTail(s, w, "…")
		}
		b.WriteString(s)
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
		line(" " + styleHint.Render("Enter: create · ^S: shell · Esc: back"))
		return strings.TrimSuffix(b.String(), "\n")
	}

	head := " " + stylePrompt.Render("> ") + m.query + "█"
	if m.telegram {
		badge := styleTg.Render("tg✓")
		pad := int(w) - ansi.PrintableRuneWidth(head) - ansi.PrintableRuneWidth(badge) - 1
		if pad > 0 {
			head += strings.Repeat(" ", pad) + badge
		}
	}
	line(head)
	line("")
	shown := 0
	for i := m.scroll; i < m.itemCount() && shown < m.visibleRows(); i++ {
		if m.isRestoreAll(i) {
			label := fmt.Sprintf("↻ restore all (%d)", len(m.casualties))
			if i == m.cursor {
				line("▸ " + stylePick.Render(label))
			} else {
				line("  " + styleResume.Render(label))
			}
			shown++
			continue
		}
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
		onBtn := i == m.cursor && m.onResume
		if i == m.cursor {
			prefix = "▸ "
			if !onBtn {
				label = stylePick.Render(label)
			}
		}
		if _, has := m.casualtyFor(cand); has {
			// Reserve the button's columns so it never gets truncated away.
			btn := btnResume
			if onBtn {
				btn = btnResumeFocus
			}
			avail := int(w) - ansi.PrintableRuneWidth(prefix) - ansi.PrintableRuneWidth(btn) - 1
			if avail < 1 {
				avail = 1
			}
			if ansi.PrintableRuneWidth(label) > avail {
				label = truncate.StringWithTail(label, uint(avail), "…")
			}
			pad := avail - ansi.PrintableRuneWidth(label) + 1
			if pad < 1 {
				pad = 1
			}
			styled := styleResume.Render(btn)
			if onBtn {
				styled = stylePick.Reverse(true).Render(btn)
			}
			label += strings.Repeat(" ", pad) + styled
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
	} else if len(m.casualties) > 0 {
		line(" " + styleHint.Render("Enter: new · → resume · ^S · ^T tg · Esc"))
	} else {
		line(" " + styleHint.Render("Enter: create · ^S: shell · Tab: name · Esc"))
	}
	return strings.TrimSuffix(b.String(), "\n")
}
