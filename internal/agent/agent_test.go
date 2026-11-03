package agent

import (
	"errors"
	"slices"
	"testing"
)

func TestBuiltinsExcludeGemini(t *testing.T) {
	var names []string
	for _, b := range Builtins {
		names = append(names, b.Name)
	}
	want := []string{"claude", "agy", "codex", "grok"}
	if !slices.Equal(names, want) {
		t.Fatalf("Builtins = %v, want %v", names, want)
	}
}

func TestAvailableFiltersByLookPathAndFallsBack(t *testing.T) {
	installed := map[string]bool{"claude": true, "agy": true}
	look := func(bin string) (string, error) {
		if installed[bin] {
			return "/usr/local/bin/" + bin, nil
		}
		return "", errors.New("not found")
	}

	got := Available(nil, look)
	if len(got) != 2 || got[0].Name != "claude" || got[1].Name != "agy" {
		t.Fatalf("Available = %+v, want [claude agy]", got)
	}

	// Custom override of builtin + custom extra agent
	custom := []Spec{
		{Name: "agy", Cmd: "agy --dangerously-skip-permissions"},
		{Name: "mybot", Cmd: "claude --model opus"},
	}
	got = Available(custom, look)
	if len(got) != 3 || got[1].Command() != "agy --dangerously-skip-permissions" || got[2].Name != "mybot" {
		t.Fatalf("Available with custom = %+v", got)
	}

	// Nothing installed -> fallback to first builtin (claude)
	empty := Available(nil, func(string) (string, error) { return "", errors.New("none") })
	if len(empty) != 1 || empty[0].Name != "claude" {
		t.Fatalf("Fallback = %+v, want [claude]", empty)
	}
}

func TestResolve(t *testing.T) {
	avail := []Spec{{Name: "claude", Cmd: "claude"}, {Name: "agy", Cmd: "agy"}}
	if got := Resolve("agy", avail); got != 1 {
		t.Errorf("Resolve(agy) = %d, want 1", got)
	}
	if got := Resolve("unknown", avail); got != 0 {
		t.Errorf("Resolve(unknown) = %d, want 0", got)
	}
}
