package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTranscript(t *testing.T, lines string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
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
