package cli

import (
	"github.com/spf13/cobra"
)

func newGoCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "go [name]",
		Aliases: []string{"cd"},
		Short:   "Print a workspace path for cd or shell integration",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			inspector, err := newContainer(cmd).inspector()
			if err != nil {
				return err
			}
			ws, err := inspector.Resolve(firstArg(args))
			if err != nil {
				return err
			}
			return presenterFor(cmd, false).Path(ws.Path)
		},
	}
}
