// Debug helper: prints what discover sees. Not part of the shipped binary.
package main

import (
	"fmt"

	"pier/internal/discover"
	"pier/internal/tmux"
)

func main() {
	panes, err := tmux.ListPanes()
	if err != nil {
		fmt.Println("ListPanes err:", err)
		return
	}
	for _, p := range panes {
		fmt.Printf("pane=%s sess=%s cmd=%q path=%q sidebar=%v claude=%v\n",
			p.ID, p.Session, p.Command, p.Path, p.Sidebar, discover.IsClaudeCommand(p.Command))
	}
	for _, g := range discover.Group(panes, discover.GitBranch) {
		fmt.Printf("group %s ⎇ %s: %d instance(s)\n", g.Path, g.Branch, len(g.Instances))
	}
}
