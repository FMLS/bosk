// Package git adapts bosk's git ports onto the git command line.
package git

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// CommandRunner executes a git invocation and returns its combined output.
// Client depends on this rather than os/exec so that output parsing can be
// tested without a real repository.
type CommandRunner interface {
	Run(dir string, args ...string) (string, error)
}

// CommandError is a failed git subprocess.
type CommandError struct {
	Cmd    string
	Output string
}

func (e *CommandError) Error() string {
	return fmt.Sprintf("git %s: %s", e.Cmd, e.Output)
}

// ExecRunner runs git as a subprocess with prompting disabled.
type ExecRunner struct {
	env []string
}

// NewExecRunner captures the process environment once, with the settings that
// keep git from blocking on a credential or host-key prompt.
func NewExecRunner() *ExecRunner {
	env := append([]string{}, os.Environ()...)
	env = append(env, "GIT_TERMINAL_PROMPT=0")
	hasSSH := false
	for _, e := range env {
		if strings.HasPrefix(e, "GIT_SSH_COMMAND=") {
			hasSSH = true
			break
		}
	}
	if !hasSSH {
		env = append(env, "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
	}
	return &ExecRunner{env: env}
}

func (r *ExecRunner) Run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = r.env
	// A nil Stdin gives the child the null device, so git can never consume
	// the caller's terminal input.
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		return output, &CommandError{Cmd: strings.Join(args, " "), Output: output}
	}
	return output, nil
}
