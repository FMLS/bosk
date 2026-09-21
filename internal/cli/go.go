package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newGoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "go [name]",
		Aliases: []string{"cd"},
		Short:   "Print a workspace path for cd or shell integration",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := loadService()
			if err != nil {
				return err
			}
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			ws, err := svc.Resolve(name)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), ws.Path)
			return nil
		},
	}
	return cmd
}
