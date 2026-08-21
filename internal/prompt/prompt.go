// Package prompt holds the small line prompts the first-run wizard and the
// update check use. Input and output are injectable so tests drive them.
package prompt

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// Prompter asks questions on Out and reads answers from its input.
type Prompter struct {
	in  *bufio.Reader
	Out io.Writer
}

// New makes a Prompter over the given streams.
func New(in io.Reader, out io.Writer) *Prompter {
	return &Prompter{in: bufio.NewReader(in), Out: out}
}

// Std is a Prompter on the process's stdin/stdout.
func Std() *Prompter { return New(os.Stdin, os.Stdout) }

// YesNo asks a yes/no question; Enter (or EOF) picks def, junk re-asks.
func (p *Prompter) YesNo(question string, def bool) bool {
	hint := "[y/N]"
	if def {
		hint = "[Y/n]"
	}
	for {
		fmt.Fprintf(p.Out, "%s %s ", question, hint)
		line, err := p.in.ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "":
			return def
		case "y", "yes":
			return true
		case "n", "no":
			return false
		}
		if err != nil {
			return def
		}
	}
}

// Line asks for a free-form answer; Enter (or EOF) picks def.
func (p *Prompter) Line(question, def string) string {
	if def != "" {
		fmt.Fprintf(p.Out, "%s [%s]: ", question, def)
	} else {
		fmt.Fprintf(p.Out, "%s: ", question)
	}
	line, _ := p.in.ReadString('\n')
	if ans := strings.TrimSpace(line); ans != "" {
		return ans
	}
	return def
}

// IsTerminal reports whether stdin and stdout are both terminals — the
// wizard and the update prompt only make sense then.
func IsTerminal() bool {
	return isCharDevice(os.Stdin) && isCharDevice(os.Stdout)
}

func isCharDevice(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
