// Package ui renders the pier sidebar TUI (bubbletea).
package ui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fsnotify/fsnotify"
	"github.com/muesli/reflow/truncate"

	"pier/internal/agent"
	"pier/internal/discover"
	"pier/internal/onboard"
	"pier/internal/resume"
	"pier/internal/state"
	"pier/internal/tmux"
	"pier/internal/update"
)

// Version is pier's own version, set by main — the sidebar compares it
// against the Homebrew tap to show the update row.
var Version string

// updateCheckEvery bounds how often a sidebar looks at the update cache;
// the network fetch behind it is capped separately (once a day, shared
// between every sidebar through the cache file).
const updateCheckEvery = time.Hour

const pollInterval = 2 * time.Second

var (
	styleHeader = lipgloss.NewStyle().Faint(true)
	stylePath   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	styleBranch = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleCursor = lipgloss.NewStyle().Reverse(true)
	// Current session: full-row highlight so it pops at a glance.
	styleCurrent = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("30"))
	styleDim    = lipgloss.NewStyle().Faint(true)
	styleErr    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styleUpdate = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))

	iconStyles = map[string]lipgloss.Style{
		state.Working:    lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		state.Waiting:    lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		state.Permission: lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
	}
	icons = map[string]string{
		state.Working:    "●",
		state.Waiting:    "○",
		state.Permission: "!",
	}
)

// paneIcon returns a pane's status glyph and its styled rendering: "$" for a
// terminal pane (no AI agent — any recorded state is stale), else the recorded
// state's glyph, "·" when unknown. Shared by the sidebar rows and the
// picker's open-session rows so both show the same thing.
func paneIcon(p tmux.Pane, states map[string]state.PaneState, custom ...[]agent.Spec) (glyph, styled string) {
	var specs []agent.Spec
	if len(custom) > 0 {
		specs = custom[0]
	}
	if !discover.IsAgentCommand(p.Command, specs) {
		return "$", styleDim.Render("$")
	}
	st := states[p.ID].State
	glyph = "·"
	if ic, ok := icons[st]; ok {
		glyph = ic
	}
	if s, ok := iconStyles[st]; ok {
		return glyph, s.Render(glyph)
	}
	return glyph, styleDim.Render(glyph)
}

type rowKind int

const (
	rowBlank rowKind = iota
	rowHeader
	rowItem
	rowNew    // the "+ new session" action line
	rowHelp   // the "? help" action line
	rowInfo   // plain informational text
	rowUpdate // "a newer pier is out" notice
)

type row struct {
	kind rowKind
	text string // rowInfo only
	wt   discover.Worktree
	pane tmux.Pane
	item int // index into items when kind == rowItem, else -1
}

type (
	tickMsg    time.Time
	watchMsg   struct{}
	updateMsg  struct{ latest string }
	refreshMsg struct {
		groups []discover.Worktree
		states map[string]state.PaneState
		panes  []tmux.Pane
		custom []agent.Spec
		err    error
	}
)

// Model is the sidebar bubbletea model.
type Model struct {
	paneID       string // our own pane id ($TMUX_PANE)
	session      string // session this sidebar lives in
	groups       []discover.Worktree
	states       map[string]state.PaneState
	titles       map[string]string // session id → title
	customAgents []agent.Spec
	rows         []row
	items        []tmux.Pane
	multiAgent   bool
	cursor       int
	scroll       int
	width        int
	height       int
	err          error
	watcher      *fsnotify.Watcher
	// latest is the newer version the Homebrew tap carries, "" when we are
	// current (or never checked).
	latest    string
	lastCheck time.Time
}

// Run starts the sidebar TUI. Must run inside a tmux pane.
func Run() error {
	if os.Getenv("TMUX") == "" {
		return fmt.Errorf("not inside tmux")
	}
	dir := state.Dir()
	_ = os.MkdirAll(dir, 0o755)
	watcher, err := fsnotify.NewWatcher()
	if err == nil {
		_ = watcher.Add(dir)
		defer watcher.Close()
	}
	m := Model{
		paneID:  os.Getenv("TMUX_PANE"),
		cursor:  -1, // hidden until the first j/k — mouse is the primary path
		session: tmux.CurrentSession(),
		states:  map[string]state.PaneState{},
		titles:  map[string]string{},
		watcher: watcher,
		width:   30,
		height:  40,
	}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err = p.Run()
	return err
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(refresh, tick(), m.waitWatch(), checkUpdate)
}

// checkUpdate reports a newer pier from the Homebrew tap, reusing the
// launcher's once-a-day cache — so a sidebar costs a file read, not a
// request. PIER_NO_UPDATE_CHECK silences it like everywhere else.
func checkUpdate() tea.Msg {
	if Version == "" || os.Getenv("PIER_NO_UPDATE_CHECK") != "" {
		return updateMsg{}
	}
	url := update.FormulaURL
	if u := os.Getenv("PIER_UPDATE_URL"); u != "" {
		url = u
	}
	latest, _ := update.Check(update.CachePath(), url, Version, time.Now(), func(u string) (string, error) {
		return update.Fetch(u, 2*time.Second)
	})
	if !update.Newer(latest, Version) {
		return updateMsg{}
	}
	return updateMsg{latest: latest}
}

func refresh() tea.Msg {
	panes, err := tmux.ListPanes()
	if err != nil {
		return refreshMsg{err: err}
	}
	live := map[string]bool{}
	for _, p := range panes {
		live[p.ID] = true
	}
	dir := state.Dir()
	state.Cleanup(dir, live)
	cfg, _ := onboard.Load(onboard.ConfigPath())
	return refreshMsg{
		groups: discover.GroupWithCustom(panes, discover.GitBranch, cfg.Agents),
		states: state.ReadAll(dir),
		panes:  panes,
		custom: cfg.Agents,
	}
}

func tick() tea.Cmd {
	return tea.Tick(pollInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) waitWatch() tea.Cmd {
	if m.watcher == nil {
		return nil
	}
	w := m.watcher
	return func() tea.Msg {
		select {
		case <-w.Events:
		case <-w.Errors:
		}
		return watchMsg{}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.scroll > m.maxScroll() {
			m.scroll = m.maxScroll()
		}
		// Multi-client attach reflows can squeeze this pane; snap back to the
		// fixed width. No resize loop: an unchanged size emits no new event.
		if pane := os.Getenv("TMUX_PANE"); pane != "" && msg.Width != tmux.SidebarWidth {
			tmux.RestoreWidth(pane)
		}
		// A frame drawn during a transient width can wrap and leave ghost
		// lines the renderer can't diff away — wipe them.
		return m, tea.ClearScreen

	case tickMsg:
		// The sidebar doubles as the machine's heartbeat: after a shutdown
		// the file's frozen mtime tells resume-pick when the system was last
		// alive (throttled inside to one touch a minute).
		resume.HeartbeatTick(resume.Dir())
		cmds := []tea.Cmd{refresh, tick()}
		if time.Since(m.lastCheck) >= updateCheckEvery {
			m.lastCheck = time.Now()
			cmds = append(cmds, checkUpdate)
		}
		return m, tea.Batch(cmds...)

	case watchMsg:
		return m, tea.Batch(refresh, m.waitWatch())

	case updateMsg:
		if msg.latest != m.latest {
			m.latest = msg.latest
			m.buildRows()
		}
		return m, nil

	case refreshMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		// The sidebar must never keep a session alive on its own: when every
		// other pane in our session is gone, exit so the session dies with us.
		if m.paneID != "" && m.session != "" && len(msg.panes) > 0 {
			alone := true
			for _, p := range msg.panes {
				if p.Session == m.session && p.ID != m.paneID {
					alone = false
					break
				}
			}
			if alone {
				return m, tea.Quit
			}
		}
		m.err = nil
		m.customAgents = msg.custom
		m.groups = msg.groups
		m.states = msg.states
		m.loadTitles()
		m.buildRows()
		if m.cursor >= 0 {
			m.clampScroll()
		} else if max := m.maxScroll(); m.scroll > max {
			m.scroll = max
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			if m.cursor < len(m.items)-1 {
				m.cursor++ // from the hidden -1 this lands on the first item
				m.clampScroll()
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
				m.clampScroll()
			}
		case "g":
			if len(m.items) > 0 {
				m.cursor = 0
				m.clampScroll()
			}
		case "enter":
			m.jump(m.cursor)
		case "n":
			return m, m.openPopup(tmux.OpenNewPopup)
		case "?":
			return m, m.openPopup(tmux.OpenKeysPopup)
		case "r":
			return m, refresh
		}
		return m, nil

	case tea.MouseMsg:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if m.scroll > 0 {
				m.scroll--
			}
			return m, nil
		case tea.MouseButtonWheelDown:
			if max := m.maxScroll(); m.scroll < max {
				m.scroll++
			}
			return m, nil
		case tea.MouseButtonLeft:
		default:
			return m, nil
		}
		if msg.Y <= 0 {
			return m, nil
		}
		idx := 1 + m.scroll + (msg.Y - 1)
		if idx < 0 || idx >= len(m.rows) {
			return m, nil
		}
		r := m.rows[idx]
		switch r.kind {
		case rowItem:
			m.cursor = r.item
			m.jump(r.item)
		case rowHeader:
			// jump to the worktree's first instance
			if len(r.wt.Instances) > 0 {
				m.jumpPane(r.wt.Instances[0].ID)
			}
		case rowNew:
			return m, m.openPopup(tmux.OpenNewPopup)
		case rowHelp:
			return m, m.openPopup(tmux.OpenKeysPopup)
		}
		return m, nil
	}
	return m, nil
}

func (m *Model) jump(item int) {
	if item < 0 || item >= len(m.items) {
		return
	}
	m.jumpPane(m.items[item].ID)
}

func (m *Model) jumpPane(paneID string) {
	if err := tmux.Jump(paneID); err != nil {
		m.err = err
	}
}

// loadTitles resolves Claude session ids seen in states to a display title.
func (m *Model) loadTitles() {
	for _, st := range m.states {
		if st.SessionID == "" || st.CWD == "" {
			continue
		}
		if _, ok := m.titles[st.SessionID]; ok {
			continue
		}
		if t := sessionTitle(st.CWD, st.SessionID); t != "" {
			m.titles[st.SessionID] = t
		}
	}
}

func (m *Model) buildRows() {
	m.rows = m.rows[:0]
	m.items = m.items[:0]
	seenAgents := map[string]bool{}
	m.rows = append(m.rows, row{kind: rowBlank, item: -1}) // header line rendered in View
	for _, wt := range m.groups {
		m.rows = append(m.rows, row{kind: rowHeader, wt: wt, item: -1})
		for _, p := range wt.Instances {
			m.items = append(m.items, p)
			m.rows = append(m.rows, row{kind: rowItem, wt: wt, pane: p, item: len(m.items) - 1})
			if ag := discover.AgentNameWithCustom(p.Command, m.customAgents); ag != "" {
				seenAgents[ag] = true
			}
		}
		m.rows = append(m.rows, row{kind: rowBlank, item: -1})
	}
	m.multiAgent = len(seenAgents) > 1
	if len(m.items) == 0 {
		m.rows = append(m.rows,
			row{kind: rowInfo, text: " no sessions", item: -1},
			row{kind: rowBlank, item: -1})
	}
	m.rows = append(m.rows, row{kind: rowNew, item: -1})
	m.rows = append(m.rows, row{kind: rowHelp, item: -1})
	if m.latest != "" {
		m.rows = append(m.rows, row{kind: rowUpdate, text: m.latest, item: -1})
	}
	if m.cursor >= len(m.items) {
		m.cursor = len(m.items) - 1 // may become -1 (hidden) when empty
	}
}

func (m Model) visibleBodyRows() int {
	h := m.height - 1 // line 0 is the fixed header
	if m.err != nil {
		h--
	}
	if h < 1 {
		return 1
	}
	return h
}

func (m Model) maxScroll() int {
	bodyLen := len(m.rows) - 1
	vis := m.visibleBodyRows()
	if bodyLen <= vis {
		return 0
	}
	return bodyLen - vis
}

func (m *Model) clampScroll() {
	if m.scroll < 0 {
		m.scroll = 0
	}
	if max := m.maxScroll(); m.scroll > max {
		m.scroll = max
	}
	if m.cursor < 0 {
		return
	}
	rowIdx := -1
	for i, r := range m.rows {
		if r.kind == rowItem && r.item == m.cursor {
			rowIdx = i
			break
		}
	}
	if rowIdx <= 0 {
		return
	}
	bodyIdx := rowIdx - 1
	vis := m.visibleBodyRows()
	if bodyIdx < m.scroll {
		m.scroll = bodyIdx
	} else if bodyIdx >= m.scroll+vis {
		m.scroll = bodyIdx - vis + 1
	}
	if max := m.maxScroll(); m.scroll > max {
		m.scroll = max
	}
}

// openPopup launches a pier popup (picker, keys) on the interacting client
// and refreshes when it closes.
func (m Model) openPopup(open func(self, session string) error) tea.Cmd {
	self, err := os.Executable()
	if err != nil {
		self = "pier"
	}
	session := m.session
	return func() tea.Msg {
		_ = open(self, session)
		return refresh()
	}
}

// itemLabel names an instance row: the prompt currently being worked on,
// falling back to a transcript-derived title — the ai-title, or the first
// user message, which for a subagent is the task it was assigned. A known
// session with neither is a fresh or /clear-ed conversation — deliberately
// blank. Shell instances and panes without any recorded Claude state fall
// back to the tmux session name.
func (m Model) itemLabel(p tmux.Pane, dup bool) string {
	if st, ok := m.states[p.ID]; ok && st.SessionID != "" && discover.IsAgentCommand(p.Command, m.customAgents) {
		// re-clean stored labels: state written by older binaries may hold
		// raw wrapper tags.
		if l := state.Label(st.LastPrompt); l != "" {
			return l
		}
		if st.Fresh {
			return "" // startup//clear: deliberately blank, no fallbacks
		}
		if t := m.titles[st.SessionID]; t != "" {
			return t
		}
		return ""
	}
	label := p.Session
	if dup {
		label += ":" + p.WindowIdx + "." + p.PaneIdx
	}
	return label
}

func shortAgentTag(ag string) string {
	switch ag {
	case "claude":
		return "cl"
	case "agy":
		return "agy"
	case "codex":
		return "cdx"
	case "grok":
		return "grk"
	default:
		if r := []rune(ag); len(r) > 3 {
			return string(r[:3])
		}
		return ag
	}
}

func (m Model) View() string {
	// Render against the fixed sidebar width, never a transient pane size —
	// an over-wide line wraps in the 30-col pane and leaves ghost artifacts.
	// The -1 guards ambiguous-width glyphs (●, ⎇) that some terminals draw
	// wider than measured.
	width := m.width
	if width > tmux.SidebarWidth {
		width = tmux.SidebarWidth
	}
	w := uint(width - 1)
	var b strings.Builder
	writeLine := func(s string) {
		b.WriteString(truncate.StringWithTail(s, w, "…"))
		b.WriteString("\n")
	}

	// rows[0] is the reserved header line
	writeLine(styleHeader.Render(" Pier — sessions"))

	var body []row
	if len(m.rows) > 1 {
		body = m.rows[1:] // first refresh may not have arrived yet
	}
	start := m.scroll
	if start < 0 {
		start = 0
	}
	if start > len(body) {
		start = len(body)
	}
	end := start + m.visibleBodyRows()
	if end > len(body) {
		end = len(body)
	}
	for _, r := range body[start:end] {
		switch r.kind {
		case rowBlank:
			writeLine("")
		case rowNew:
			writeLine("  " + styleBranch.Render("+") + styleDim.Render(" new session"))
		case rowHelp:
			writeLine("  " + styleBranch.Render("?") + styleDim.Render(" help"))
		case rowInfo:
			writeLine(styleDim.Render(r.text))
		case rowUpdate:
			// Instructional, not clickable: `pier upgrade` is the command,
			// and a row that looks pressable but isn't reads as broken.
			writeLine("  " + styleUpdate.Render("↑ "+r.text) + styleDim.Render(" · pier upgrade"))
		case rowHeader:
			name := filepath.Base(r.wt.Path)
			writeLine(" " + stylePath.Render(name) + " " + styleBranch.Render("⎇ "+r.wt.Branch))
		case rowItem:
			dup := false
			for _, other := range r.wt.Instances {
				if other.ID != r.pane.ID && other.Session == r.pane.Session {
					dup = true
				}
			}
			icon, iconR := paneIcon(r.pane, m.states, m.customAgents)
			label := m.itemLabel(r.pane, dup)
			tagPrefix, plainPrefix := "", ""
			if m.multiAgent {
				if ag := discover.AgentNameWithCustom(r.pane.Command, m.customAgents); ag != "" {
					tag := shortAgentTag(ag) + ":"
					tagPrefix = styleDim.Render(tag) + " "
					plainPrefix = tag + " "
				}
			}
			line := "  " + iconR + " " + tagPrefix + label
			if r.pane.Session == m.session {
				// Full-width highlighted row with an edge bar. The icon is
				// folded into the row style: nested renders would reset the
				// background mid-line.
				line = styleCurrent.Width(int(w)).Render("▌ " + icon + " " + plainPrefix + label)
			}
			if r.item == m.cursor {
				line = styleCursor.Render(strings.TrimRight(line, " "))
			}
			writeLine(line)
		}
	}

	if m.err != nil {
		writeLine(styleErr.Render(truncateErr(m.err)))
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func truncateErr(err error) string {
	s := err.Error()
	if len(s) > 120 {
		s = s[:120]
	}
	return " " + s
}

// projectSlug converts a working directory to Claude Code's project dir slug
// (observed rule: '/' and '.' both become '-').
func projectSlug(cwd string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '.' {
			return '-'
		}
		return r
	}, cwd)
}

// sessionTitle derives a display title from a Claude Code session transcript
// or Antigravity CLI conversation metadata cache.
func sessionTitle(cwd, sessionID string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(home, ".claude", "projects", projectSlug(cwd), sessionID+".jsonl")
	if _, err := os.Stat(path); err != nil {
		// hook cwd can drift from the session's project dir; fall back to a glob
		matches, _ := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", sessionID+".jsonl"))
		if len(matches) > 0 {
			path = matches[0]
		} else {
			path = ""
		}
	}
	if path != "" {
		if t := titleFromTranscript(path); t != "" {
			return t
		}
	}
	return agySessionTitle(filepath.Join(home, ".gemini", "antigravity-cli", "cache", "conversation_metadata.json"), sessionID)
}

func agySessionTitle(metaPath, sessionID string) string {
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return ""
	}
	var meta struct {
		Conversations map[string]struct {
			Summary struct {
				Title   string `json:"Title"`
				Preview string `json:"Preview"`
			} `json:"summary"`
		} `json:"conversations"`
	}
	if json.Unmarshal(data, &meta) != nil {
		return ""
	}
	conv, ok := meta.Conversations[sessionID]
	if !ok {
		return ""
	}
	if t := strings.TrimSpace(conv.Summary.Title); t != "" {
		return t
	}
	return state.Label(conv.Summary.Preview)
}

func titleFromTranscript(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	firstPrompt := ""
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for i := 0; i < 40 && sc.Scan(); i++ {
		var rec struct {
			Type        string `json:"type"`
			AITitle     string `json:"aiTitle"`
			IsMeta      bool   `json:"isMeta"`
			IsSidechain bool   `json:"isSidechain"`
			Message     struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(sc.Bytes(), &rec) != nil {
			continue
		}
		if rec.Type == "ai-title" && rec.AITitle != "" {
			return rec.AITitle // preferred: Claude's own summary
		}
		if firstPrompt != "" || rec.Type != "user" || rec.IsMeta || rec.IsSidechain || rec.Message.Role != "user" {
			continue
		}
		firstPrompt = state.Label(userText(rec.Message.Content))
	}
	return firstPrompt
}

// userText extracts the text of a user record's content, which is either a
// plain string or a block array mixing text with tool results.
func userText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var b strings.Builder
	for _, bl := range blocks {
		if bl.Type == "text" {
			b.WriteString(bl.Text)
			b.WriteString("\n")
		}
	}
	return b.String()
}
