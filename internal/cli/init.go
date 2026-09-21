package cli

import (
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init <dir> [dir...]",
		Short: "Register directories that contain git repositories",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newContainer(cmd)
			res, err := c.initializer().Init(args)
			if err != nil {
				return err
			}
			for _, p := range res.Skipped {
				c.warner.Warn("skipped duplicate repo name: " + p)
			}
			return presenterFor(cmd, false).Initialized(res)
		},
	}
}
