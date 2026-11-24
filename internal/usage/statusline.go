package usage

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const maxPayloadBytes = 1 << 20

// RunStatusLine reads an agent statusLine JSON payload from stdin, updates
// pier's local usage cache, and optionally chains to wrapCmd so existing user
// statusLine scripts keep working unchanged.
func RunStatusLine(agentName, wrapCmd, dir string, stdin io.Reader, stdout, stderr io.Writer) error {
	return runStatusLine(agentName, wrapCmd, dir, stdin, stdout, stderr, time.Now())
}

func runStatusLine(agentName, wrapCmd, dir string, stdin io.Reader, stdout, stderr io.Writer, now time.Time) error {
	payload, _ := io.ReadAll(io.LimitReader(stdin, maxPayloadBytes))
	if os.Getenv("PIER_DEBUG_STATUSLINE") != "" && agentName != "" && len(payload) > 0 {
		if os.MkdirAll(dir, 0o755) == nil {
			_ = os.WriteFile(filepath.Join(dir, agentName+"-raw.json"), payload, 0o600)
		}
	}

	var snap Snapshot
	var ok bool
	switch agentName {
	case "claude":
		snap, ok = ParseClaude(payload, now)
	case "agy":
		snap, ok = ParseAgy(payload, now)
	}
	if ok {
		// Early ticks in a fresh session may omit quota/rate_limits while the
		// CLI finishes initializing; preserve recent cached buckets instead of
		// erasing them.
		if len(snap.Buckets) == 0 {
			if prev, hasPrev := LoadFile(dir, snap.Agent); hasPrev && len(prev.Buckets) > 0 &&
				now.Unix()-prev.UpdatedAt <= int64(expireAfter.Seconds()) {
				snap.Buckets = prev.Buckets
				snap.UpdatedAt = prev.UpdatedAt
			}
		}
		_ = Save(dir, snap)
	}

	if wrapCmd == "" {
		return nil
	}
	cmd := exec.Command("sh", "-c", wrapCmd)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}
