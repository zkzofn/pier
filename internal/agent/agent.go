// Package agent defines the AI CLI platforms pier can launch and recognize.
// Like internal/claude, it imports nothing from pier so any package can
// depend on it without cycles.
package agent

import "strings"

// Spec defines an AI CLI platform pier can launch and recognize.
type Spec struct {
	Name string `json:"name"` // short id shown in UI & config: "claude", "agy", "codex", "grok"
	Cmd  string `json:"cmd"`  // command line to run (defaults to Name)
}

// Bin returns the executable token (first word of Cmd).
func (s Spec) Bin() string {
	fields := strings.Fields(s.Cmd)
	if len(fields) == 0 {
		return s.Name
	}
	return fields[0]
}

// Command returns the shell command to run for this spec.
func (s Spec) Command() string {
	if strings.TrimSpace(s.Cmd) != "" {
		return strings.TrimSpace(s.Cmd)
	}
	return s.Name
}

// Builtins lists the supported AI CLI presets in picker cycle order.
var Builtins = []Spec{
	{Name: "claude", Cmd: "claude"},
	{Name: "agy", Cmd: "agy"},
	{Name: "codex", Cmd: "codex"},
	{Name: "grok", Cmd: "grok"},
}

// All merges Builtins with user-defined custom specs: a custom spec with the
// same Name overrides the builtin's Cmd in place; new names are appended.
func All(custom []Spec) []Spec {
	out := make([]Spec, len(Builtins))
	copy(out, Builtins)
	byName := make(map[string]int, len(out))
	for i, s := range out {
		byName[s.Name] = i
	}
	for _, c := range custom {
		name := strings.TrimSpace(c.Name)
		if name == "" {
			continue
		}
		cmd := strings.TrimSpace(c.Cmd)
		if cmd == "" {
			cmd = name
		}
		spec := Spec{Name: name, Cmd: cmd}
		if idx, ok := byName[name]; ok {
			out[idx] = spec
		} else {
			byName[name] = len(out)
			out = append(out, spec)
		}
	}
	return out
}

// Available returns the specs from All(custom) whose binary is found by
// lookPath. When nothing is found on PATH, it falls back to [claude] so
// callers always have at least one entry.
func Available(custom []Spec, lookPath func(string) (string, error)) []Spec {
	all := All(custom)
	var avail []Spec
	for _, s := range all {
		if lookPath != nil {
			if _, err := lookPath(s.Bin()); err == nil {
				avail = append(avail, s)
			}
		}
	}
	if len(avail) == 0 {
		return []Spec{all[0]}
	}
	return avail
}

// Resolve returns the index of preferred in avail, or 0 when not found.
func Resolve(preferred string, avail []Spec) int {
	for i, s := range avail {
		if s.Name == preferred {
			return i
		}
	}
	return 0
}

// Find looks up name in All(custom).
func Find(name string, custom []Spec) (Spec, bool) {
	for _, s := range All(custom) {
		if s.Name == name {
			return s, true
		}
	}
	return Spec{}, false
}

// IsBuiltinBin reports whether cmd matches one of the built-in agent binary
// names ("claude", "agy", "codex", "grok").
func IsBuiltinBin(cmd string) bool {
	for _, s := range Builtins {
		if cmd == s.Bin() {
			return true
		}
	}
	return false
}
