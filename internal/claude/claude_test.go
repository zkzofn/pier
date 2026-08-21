package claude

import "testing"

func TestCmd(t *testing.T) {
	if got := Cmd(false); got != "claude" {
		t.Errorf("Cmd(false) = %q", got)
	}
	if got := Cmd(true); got != "claude --channels plugin:telegram@claude-plugins-official" {
		t.Errorf("Cmd(true) = %q", got)
	}
	if got := ResumeCmd(false, "abc-123"); got != "claude --resume abc-123" {
		t.Errorf("ResumeCmd = %q", got)
	}
	if got := ResumeCmd(true, "abc-123"); got != "claude --channels plugin:telegram@claude-plugins-official --resume abc-123" {
		t.Errorf("ResumeCmd(telegram) = %q", got)
	}
}
