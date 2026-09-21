// Package cli wires bosk's use cases to cobra commands. It is the only package
// that names a concrete adapter.
package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var version = "dev"

// Execute runs the bosk CLI.
func Execute() int {
	root := newRoot()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func newRoot() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "bosk",
		Short:         "Multi-repo git worktree workspaces",
		Long:          "bosk groups linked git worktrees into a named workspace so one task can span multiple repositories.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
	}
	cmd.AddCommand(
		newInitCmd(),
		newCreateCmd(),
		newListCmd(),
		newStatusCmd(),
		newDeleteCmd(),
		newDoctorCmd(),
		newGoCmd(),
		newReposCmd(),
		newShellInitCmd(),
	)
	return cmd
}

// splitCSV parses a comma-separated flag value, dropping blanks.
func splitCSV(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// firstArg returns the optional positional argument.
func firstArg(args []string) string {
	if len(args) == 1 {
		return args[0]
	}
	return ""
}
