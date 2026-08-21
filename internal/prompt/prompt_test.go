package prompt

import (
	"bytes"
	"strings"
	"testing"
)

func TestYesNo(t *testing.T) {
	cases := []struct {
		in   string
		def  bool
		want bool
	}{
		{"y\n", false, true}, {"YES\n", false, true}, {"n\n", true, false}, {"no\n", true, false},
		{"\n", true, true}, {"\n", false, false}, // Enter -> default
		{"maybe\nn\n", true, false}, // re-asks on junk
		{"", true, true},            // EOF -> default
	}
	for _, c := range cases {
		var out bytes.Buffer
		p := New(strings.NewReader(c.in), &out)
		if got := p.YesNo("ok?", c.def); got != c.want {
			t.Errorf("YesNo(%q, def=%v) = %v, want %v", c.in, c.def, got, c.want)
		}
		if !strings.Contains(out.String(), "ok? [") {
			t.Errorf("question not printed: %q", out.String())
		}
	}
	var out bytes.Buffer
	New(strings.NewReader("\n"), &out).YesNo("q", true)
	if !strings.Contains(out.String(), "[Y/n]") {
		t.Errorf("default yes should show [Y/n]: %q", out.String())
	}
}

func TestLine(t *testing.T) {
	var out bytes.Buffer
	if got := New(strings.NewReader("  zz \n"), &out).Line("alias name", "cl"); got != "zz" {
		t.Errorf("Line = %q, want zz (trimmed)", got)
	}
	if !strings.Contains(out.String(), "alias name [cl]: ") {
		t.Errorf("prompt should show the default: %q", out.String())
	}
	if got := New(strings.NewReader("\n"), &out).Line("alias name", "cl"); got != "cl" {
		t.Errorf("Enter should pick the default, got %q", got)
	}
	if got := New(strings.NewReader(""), &out).Line("x", ""); got != "" {
		t.Errorf("EOF with no default -> empty, got %q", got)
	}
}
