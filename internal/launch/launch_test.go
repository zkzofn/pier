package launch

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		args []string
		want Kind
	}{
		{nil, KindLaunch},
		{[]string{"-r"}, KindPassthrough},
		{[]string{"--continue"}, KindPassthrough},
		{[]string{"-p", "hi"}, KindPassthrough},
		{[]string{"-v"}, KindVersion}, {[]string{"--version"}, KindVersion}, {[]string{"version"}, KindVersion},
		{[]string{"-h"}, KindHelp}, {[]string{"--help"}, KindHelp}, {[]string{"help"}, KindHelp},
		{[]string{"setup"}, KindSubcommand}, {[]string{"new"}, KindSubcommand}, {[]string{"alias", "cl"}, KindSubcommand},
		{[]string{"reap"}, KindSubcommand}, {[]string{"upgrade"}, KindSubcommand}, {[]string{"hook", "stop"}, KindSubcommand},
		{[]string{"setpu"}, KindUnknown}, {[]string{"mcp"}, KindUnknown},
	}
	for _, c := range cases {
		if got := Classify(c.args); got != c.want {
			t.Errorf("Classify(%v) = %v, want %v", c.args, got, c.want)
		}
	}
}
