package cli

import (
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List workspaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			inspector, err := newContainer(cmd).inspector()
			if err != nil {
				return err
			}
			workspaces, err := inspector.List()
			if err != nil {
				return err
			}
			return presenterFor(cmd, asJSON).Workspaces(workspaces)
		},
	}
	cmd.Flags().BoolVarP(&asJSON, "json", "j", false, "print JSON")
	return cmd
}
