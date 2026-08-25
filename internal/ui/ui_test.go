package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"pier/internal/state"
)

func writeTranscript(t *testing.T, lines string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// The keys popup is 46 wide with no truncation — every rendered line must
// fit (45 usable, matching the ambiguous-width guard used elsewhere).
func TestKeysContentFitsPopup(t *testing.T) {
	for _, line := range strings.Split(keysContent(), "\n") {
		if w := lipgloss.Width(line); w > 45 {
			t.Errorf("line %d cols wide, over 45: %q", w, line)
		}
	}
	// The popup is 29 tall (2 for the border): the content must fit too.
	if n := strings.Count(keysContent(), "\n") + 1; n > 27 {
		t.Errorf("keys content is %d lines, over 27", n)
	}
}

func TestTitleFromTranscriptPrefersAITitle(t *testing.T) {
	path := writeTranscript(t, `{"type":"user","message":{"role":"user","content":"첫 질문"}}
{"type":"ai-title","aiTitle":"세션 요약 제목"}
`)
	if got := titleFromTranscript(path); got != "세션 요약 제목" {
		t.Errorf("got %q, want the ai-title", got)
	}
}

func TestTitleFromTranscriptFallsBackToFirstPrompt(t *testing.T) {
	path := writeTranscript(t, `{"type":"last-prompt","leafUuid":"x"}
{"type":"file-history-snapshot","messageId":"m"}
{"type":"user","isMeta":true,"message":{"role":"user","content":"meta noise"}}
{"type":"user","message":{"role":"user","content":"<teammate-message teammate_id=\"team-lead\">\n조사해줘: hooks 문서 확인\n</teammate-message>"}}
{"type":"user","message":{"role":"user","content":"두번째 질문"}}
`)
	if got := titleFromTranscript(path); got != "조사해줘: hooks 문서 확인" {
		t.Errorf("got %q, want the first real user message with tags stripped", got)
	}
}

func TestTitleFromTranscriptBlockContent(t *testing.T) {
	path := writeTranscript(t, `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"x"},{"type":"text","text":"블록 형태 질문"}]}}
`)
	if got := titleFromTranscript(path); got != "블록 형태 질문" {
		t.Errorf("got %q, want text extracted from block content", got)
	}
}

func TestTitleFromTranscriptEmptySession(t *testing.T) {
	// A freshly /clear-ed session: records exist but no user prompt yet.
	path := writeTranscript(t, `{"type":"last-prompt","leafUuid":"x"}
{"type":"mode","mode":"normal"}
`)
	if got := titleFromTranscript(path); got != "" {
		t.Errorf("got %q, want empty for a promptless session", got)
	}
}

func TestKeysContentSaysTerminal(t *testing.T) {
	c := keysContent()
	if strings.Contains(c, "shell") {
		t.Errorf("cheatsheet still says shell:\n%s", c)
	}
	if !strings.Contains(c, "terminal session") {
		t.Error("cheatsheet should describe ^S / $ as a terminal session")
	}
}

// The cheatsheet must teach the picker's mental model — keys live inside
// the popup and act on the ▸ row — and cover ^T, or nobody finds it.
func TestKeysContentTeachesRowModel(t *testing.T) {
	c := keysContent()
	if !strings.Contains(c, "keys act on the ▸ row") {
		t.Error("picker section should say keys act on the ▸ row")
	}
	if !strings.Contains(c, "^T") {
		t.Error("picker section should list ^T (telegram)")
	}
}

func TestSidebarUpdateRow(t *testing.T) {
	m := Model{cursor: -1, width: 30, height: 40, states: map[string]state.PaneState{}, titles: map[string]string{}}
	m.buildRows()
	last := m.rows[len(m.rows)-1]
	if last.kind != rowHelp {
		t.Fatalf("without a known update the last row is help, got %v", last.kind)
	}

	m.latest = "1.1.0"
	m.buildRows()
	last = m.rows[len(m.rows)-1]
	if last.kind != rowUpdate || last.text != "1.1.0" {
		t.Fatalf("update row missing: %+v", last)
	}
	v := m.View()
	if !strings.Contains(v, "↑ 1.1.0 · pier upgrade") {
		t.Errorf("view should name the version and the command:\n%s", v)
	}
	for _, line := range strings.Split(v, "\n") {
		if w := lipgloss.Width(line); w > 29 {
			t.Errorf("line %d cols wide, over the sidebar's 29: %q", w, line)
		}
	}
}
