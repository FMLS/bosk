package cli

import (
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status [name]",
		Short: "Show git status for each repository in a workspace",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			inspector, err := newContainer(cmd).inspector()
			if err != nil {
				return err
			}
			ws, err := inspector.Resolve(firstArg(args))
			if err != nil {
				return err
			}
			return presenterFor(cmd, asJSON).Status(ws, inspector.Status(ws))
		},
	}
	cmd.Flags().BoolVarP(&asJSON, "json", "j", false, "print JSON")
	return cmd
}
