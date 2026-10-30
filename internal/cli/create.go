package cli

import (
	"github.com/FMLS/bosk/internal/app"
	"github.com/spf13/cobra"
)

func newCreateCmd() *cobra.Command {
	var (
		branch string
		repos  string
		shared string
		all    bool
	)
	cmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a workspace of linked git worktrees",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			creator, err := newContainer(cmd).creator()
			if err != nil {
				return err
			}
			ws, err := creator.Create(app.CreateOpts{
				Name:        firstArg(args),
				Branch:      branch,
				RepoNames:   splitCSV(repos),
				SharedNames: splitCSV(shared),
				All:         all,
			})
			if err != nil {
				return err
			}
			return presenterFor(cmd, false).Created(ws)
		},
	}
	cmd.Flags().StringVarP(&branch, "branch", "b", "", "branch name shared by each repository")
	cmd.Flags().StringVarP(&repos, "repos", "r", "", "comma-separated repository names")
	cmd.Flags().StringVar(&shared, "shared", "", "comma-separated repositories to link in instead of checking out")
	cmd.Flags().BoolVar(&all, "all", false, "include every discovered repository (except --shared ones)")
	return cmd
}
