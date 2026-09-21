package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// errNotInteractive is returned when there is no terminal to ask.
var errNotInteractive = errors.New("refusing to delete without --yes in non-interactive mode")

// Confirmer asks the user to approve a destructive action.
type Confirmer interface {
	// Confirm returns an error when no one can be asked, so callers can tell
	// "the user said no" from "there is no user".
	Confirm(prompt string) (bool, error)
}

// terminalConfirmer prompts on a terminal and reads a yes/no answer.
type terminalConfirmer struct {
	in  *os.File
	out io.Writer
}

func newTerminalConfirmer(in *os.File, out io.Writer) terminalConfirmer {
	return terminalConfirmer{in: in, out: out}
}

func (c terminalConfirmer) Confirm(prompt string) (bool, error) {
	if !isCharDevice(c.in) {
		return false, errNotInteractive
	}
	fmt.Fprint(c.out, prompt)
	var answer string
	_, _ = fmt.Fscanln(c.in, &answer)
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
}

func isCharDevice(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
