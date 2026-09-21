package cli

import (
	"github.com/spf13/cobra"
)

func newReposCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "repos",
		Short: "List discovered source repositories",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newContainer(cmd)
			inspector, err := c.inspector()
			if err != nil {
				return err
			}
			repos, skipped := inspector.Discovered()
			for _, p := range skipped {
				c.warner.Warn("skipped duplicate repo name: " + p)
			}
			return presenterFor(cmd, asJSON).Repos(repos)
		},
	}
	cmd.Flags().BoolVarP(&asJSON, "json", "j", false, "print JSON")
	return cmd
}
