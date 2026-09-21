package cli

import (
	"fmt"

	"github.com/FMLS/bosk/internal/config"
	"github.com/FMLS/bosk/internal/discover"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init <dir> [dir...]",
		Short: "Register directories that contain git repositories",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Init(args)
			if err != nil {
				return err
			}
			repos, skipped := discover.Repos(cfg.RepoDirs)
			fmt.Fprintf(cmd.OutOrStdout(), "initialized %s\n", config.Path())
			fmt.Fprintf(cmd.OutOrStdout(), "workspace dir: %s\n", cfg.WorkspaceDir)
			fmt.Fprintf(cmd.OutOrStdout(), "discovered %d repositories\n", len(repos))
			for _, r := range repos {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s\t%s\n", r.Name, r.Path)
			}
			for _, p := range skipped {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: skipped duplicate repo name: %s\n", p)
			}
			return nil
		},
	}
}
