package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

const shellInit = `# bosk shell integration — add to ~/.zshrc or ~/.bashrc:
#   eval "$(bosk shell-init)"
bosk() {
  if [ "$1" = "go" ] || [ "$1" = "cd" ]; then
    shift
    local dest
    dest="$(command bosk go "$@")" || return $?
    [ -n "$dest" ] || return 0
    cd "$dest" || return $?
    return 0
  fi
  command bosk "$@"
}
`

func newShellInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "shell-init",
		Short: "Print shell integration so `bosk go` can change directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			// A fixed snippet, not a rendered result: there is nothing for a
			// presenter to decide.
			fmt.Fprint(cmd.OutOrStdout(), shellInit)
			return nil
		},
	}
}
