// The new-session picker: runs inside a tmux popup (`pier new`) or
// standalone in a plain terminal (bare `pier` outside tmux — the caller
// attaches to whatever was picked). Running sessions are listed first
// (Enter jumps back into one, ctrl+s opens a terminal session in its
// directory), then directories: pick one → the session name follows it;
// Tab edits the name. Enter starts Claude Code there, ctrl+s a terminal
// (plain shell).
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

	"pier/internal/claude"
	"pier/internal/discover"
	"pier/internal/newsess"
	"pier/internal/resume"
	"pier/internal/state"
	"pier/internal/tmux"
)

var (
	stylePrompt = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	stylePick   = lipgloss.NewStyle().Bold(true)
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

// what a terminal session's main pane runs
const cmdShell = "" // empty: tmux's default shell

// tmux effects behind function vars: tests intercept them — the real
// thing talks to a live server.
var (
	tmuxNewSwitch   = tmux.NewSessionAndSwitch
	tmuxNewDetached = tmux.NewSessionDetached
	tmuxSwitchTo    = tmux.SwitchTo
)

type pickerRowKind int

const (
	prRestoreAll pickerRowKind = iota // "↻ restore all" action row
	prOpen                            // a running session — Enter jumps there
	prSep                             // blank divider between the groups; the cursor skips it
	prDir                             // a directory candidate (incl. the manual one)
)

type pickerRow struct {
	kind pickerRowKind
	open *newsess.Open      // prOpen
	cand *newsess.Candidate // prDir
}

type pickerModel struct {
	home       string
	insideTmux bool
	all        []newsess.Candidate        // directory candidates (open paths excluded)
	opens      []newsess.Open             // running sessions, sidebar order
	states     map[string]state.PaneState // icons for the open rows
	filtered   []newsess.Candidate
	manual     *newsess.Candidate
	rows       []pickerRow
	taken      map[string]bool
	casualties map[string]resume.Casualty // by cleaned cwd
	query      string
	cursor     int // index into rows
	scroll     int
	onResume   bool // focus sits on the cursor row's ↻ resume button
	telegram   bool // ^T: start claude with the telegram channel attached
	editing    bool
	name       string
	attach     string // standalone: session to attach to after the TUI exits
	width      int
	height     int
	errMsg     string
}

// RunNew runs the picker TUI — in a tmux display-popup or standalone — and
// returns the session to attach to. That is "" when nothing was picked, and
// always "" inside tmux, where the picker switches the client itself.
func RunNew() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	insideTmux := os.Getenv("TMUX") != ""
	panes, err := tmux.ListPanes()
	if err != nil {
		if insideTmux {
			return "", err
		}
		panes = nil // no tmux server yet — nothing is open, that's fine
	}
	exclude := ""
	if insideTmux {
		exclude = tmux.CurrentSession() // jumping to where we already are is a no-op
	}
	casualties := map[string]resume.Casualty{}
	for _, c := range resume.List(resume.Dir()) {
		casualties[c.CWD] = c
	}
	m := pickerModel{
		home:       home,
		insideTmux: insideTmux,
		all:        newsess.Collect(home, panes),
		opens:      newsess.OpenSessions(panes, exclude),
		states:     state.ReadAll(state.Dir()),
		taken:      tmux.SessionNames(),
		casualties: casualties,
		width:      46,
		height:     18,
	}
	m.refilter()
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	if fm, ok := final.(pickerModel); ok {
		return fm.attach, nil
	}
	return "", nil
}

func (m pickerModel) Init() tea.Cmd { return nil }

// refilter applies the query to both groups and rebuilds the rows:
// [restore all] → open sessions → divider → directories. The cursor lands on
// the first open session (Enter = back into it), else the first directory —
// never the restore-all row: a reflexive Enter at boot must not resume every
// casualty at once (it stays one ↑ away).
func (m *pickerModel) refilter() {
	m.filtered = newsess.Filter(m.all, m.query)
	opens := newsess.FilterOpen(m.opens, m.query)
	m.manual = nil
	if len(m.filtered) == 0 && len(opens) == 0 {
		m.manual = newsess.Manual(m.home, m.query)
	}
	m.rows = m.rows[:0]
	if m.query == "" && len(m.casualties) > 0 {
		m.rows = append(m.rows, pickerRow{kind: prRestoreAll})
	}
	for i := range opens {
		m.rows = append(m.rows, pickerRow{kind: prOpen, open: &opens[i]})
	}
	dirs := m.filtered
	if m.manual != nil {
		dirs = []newsess.Candidate{*m.manual}
	}
	if len(opens) > 0 && len(dirs) > 0 {
		m.rows = append(m.rows, pickerRow{kind: prSep})
	}
	for i := range dirs {
		m.rows = append(m.rows, pickerRow{kind: prDir, cand: &dirs[i]})
	}
	m.cursor = 0
	for i, r := range m.rows {
		if r.kind == prOpen || r.kind == prDir {
			m.cursor = i
			break
		}
	}
	m.scroll = 0
	m.onResume = false
	m.errMsg = ""
}

func (m pickerModel) rowAt(i int) *pickerRow {
	if i < 0 || i >= len(m.rows) {
		return nil
	}
	return &m.rows[i]
}

// casualtyFor returns the dead session waiting in a directory row's path.
func (m pickerModel) casualtyFor(r *pickerRow) (resume.Casualty, bool) {
	if r == nil || r.kind != prDir || r.cand.Manual {
		return resume.Casualty{}, false
	}
	c, ok := m.casualties[filepath.Clean(r.cand.Path)]
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

// move steps the cursor by delta, hopping over divider rows; past either end
// it stays put.
func (m *pickerModel) move(delta int) {
	i := m.cursor + delta
	for i >= 0 && i < len(m.rows) && m.rows[i].kind == prSep {
		i += delta
	}
	if i < 0 || i >= len(m.rows) {
		return
	}
	m.cursor = i
	m.onResume = false
	m.clampScroll()
}

// launch creates the session — switching to it inside tmux, handing the
// name back for an attach when standalone.
func (m pickerModel) launch(name, path, command string) (tea.Model, tea.Cmd) {
	if m.insideTmux {
		if err := tmuxNewSwitch(name, path, command); err != nil {
			m.errMsg = err.Error()
			return m, nil
		}
	} else {
		if err := tmuxNewDetached(name, path, command); err != nil {
			m.errMsg = err.Error()
			return m, nil
		}
		m.attach = name
	}
	return m, tea.Quit
}

// jump leaves the picker for a running session: switch the client inside
// tmux, else hand the name back for the caller to attach.
func (m pickerModel) jump(session string) (tea.Model, tea.Cmd) {
	if m.insideTmux {
		_ = tmuxSwitchTo(session)
		return m, tea.Quit
	}
	m.attach = session
	return m, tea.Quit
}

func (m pickerModel) confirm(i int, shell bool) (tea.Model, tea.Cmd) {
	r := m.rowAt(i)
	if r == nil {
		return m, nil
	}
	switch r.kind {
	case prRestoreAll:
		if shell {
			return m, nil
		}
		return m.restoreAll()
	case prOpen:
		if !shell {
			return m.jump(r.open.Session)
		}
		// ^S means "terminal at this row's path" on every row — the cursor
		// boots on an open row, and a jump here instead of a terminal reads
		// as the key doing nothing (or worse, hopping sessions).
		name := newsess.Unique(newsess.Sanitize(r.open.Session), m.taken)
		return m.launch(name, r.open.Path, cmdShell)
	case prSep:
		return m, nil
	}
	cand := r.cand
	command := claude.Cmd(m.telegram)
	name := cand.Name
	resumeSID := ""
	if !shell && m.onResume {
		// The ↻ button: continue the dead conversation, under its old
		// session name when we know it.
		if c, ok := m.casualtyFor(r); ok {
			command = claude.ResumeCmd(m.telegram, c.SID)
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
		if err := tmuxNewDetached(name, c.CWD, claude.ResumeCmd(m.telegram, c.SID)); err != nil {
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
	return m.jump(newest)
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
			m.move(-1)
		case tea.MouseButtonWheelDown:
			m.move(1)
		case tea.MouseButtonLeft:
			if m.editing {
				return m, nil
			}
			i := msg.Y - listTop + m.scroll
			r := m.rowAt(i)
			if r == nil || r.kind == prSep {
				return m, nil
			}
			m.cursor = i
			_, has := m.casualtyFor(r)
			m.onResume = has && msg.X >= m.width-1-ansi.PrintableRuneWidth(btnResume)
			return m.confirm(i, false)
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
			if _, ok := m.casualtyFor(m.rowAt(m.cursor)); ok {
				m.onResume = true
			}
		case "left":
			m.onResume = false
		case "tab":
			if r := m.rowAt(m.cursor); r != nil && r.kind == prDir {
				m.editing = true
				m.onResume = false
				m.name = newsess.Unique(r.cand.Name, m.taken)
			}
		case "up", "ctrl+p":
			m.move(-1)
		case "down", "ctrl+n":
			m.move(1)
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

// dirLine renders a directory row, reserving the ↻ resume button's columns
// when the directory holds a casualty so it never gets truncated away.
func (m pickerModel) dirLine(r *pickerRow, cur bool, w uint) string {
	cand := r.cand
	prefix, label := "  ", ""
	switch {
	case cand.Manual && cand.NeedsMkdir:
		label = "+ mkdir & create at " + discover.ShortPath(cand.Path)
	case cand.Manual:
		label = "+ create at " + discover.ShortPath(cand.Path)
	default:
		label = cand.Name + "  " + styleHint.Render(discover.ShortPath(cand.Path))
	}
	onBtn := cur && m.onResume
	if cur {
		prefix = "▸ "
		if !onBtn {
			label = stylePick.Render(label)
		}
	}
	if _, has := m.casualtyFor(r); has {
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
	return prefix + label
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
		path := ""
		if r := m.rowAt(m.cursor); r != nil && r.kind == prDir {
			path = discover.ShortPath(r.cand.Path)
		}
		line(" " + styleHint.Render("path: "+path))
		line(" " + stylePrompt.Render("name: ") + m.name + "█")
		line("")
		if m.errMsg != "" {
			line(" " + styleNewErr.Render(m.errMsg))
		}
		line(" " + styleHint.Render("Enter: create · ^S: terminal · Esc: back"))
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
	for i := m.scroll; i < len(m.rows) && shown < m.visibleRows(); i++ {
		r := &m.rows[i]
		cur := i == m.cursor
		switch r.kind {
		case prRestoreAll:
			label := fmt.Sprintf("↻ restore all (%d)", len(m.casualties))
			if cur {
				line("▸ " + stylePick.Render(label))
			} else {
				line("  " + styleResume.Render(label))
			}
		case prOpen:
			// Styles are applied per segment, not nested: the icon's color
			// reset would otherwise cut the cursor's bold short.
			_, icon := paneIcon(r.open.Pane, m.states)
			prefix, name := "  ", r.open.Session
			if cur {
				prefix, name = "▸ ", stylePick.Render(name)
			}
			line(prefix + icon + " " + name + "  " + styleHint.Render(discover.ShortPath(r.open.Path)))
		case prSep:
			line("")
		case prDir:
			line(m.dirLine(r, cur, w))
		}
		shown++
	}
	if len(m.rows) == 0 {
		line("  " + styleHint.Render("no match — type a ~/ or absolute path"))
	}
	line("")
	if m.errMsg != "" {
		line(" " + styleNewErr.Render(m.errMsg))
	} else if len(m.casualties) > 0 {
		line(" " + styleHint.Render("Enter: new · → resume · ^S · ^T tg · Esc"))
	} else {
		line(" " + styleHint.Render("Enter: create · ^S: terminal · Tab: name"))
	}
	return strings.TrimSuffix(b.String(), "\n")
}
