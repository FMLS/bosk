package cli

import (
	"github.com/FMLS/bosk/internal/app"
	"github.com/spf13/cobra"
)

func newAddCmd() *cobra.Command {
	var (
		repos  string
		shared string
		all    bool
	)
	cmd := &cobra.Command{
		Use:   "add [name]",
		Short: "Add repositories to an existing workspace",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newContainer(cmd)
			inspector, err := c.inspector()
			if err != nil {
				return err
			}
			ws, err := inspector.Resolve(firstArg(args))
			if err != nil {
				return err
			}
			creator, err := c.creator()
			if err != nil {
				return err
			}
			ws, added, err := creator.Add(app.AddOpts{
				Name:        ws.Name,
				RepoNames:   splitCSV(repos),
				SharedNames: splitCSV(shared),
				All:         all,
			})
			if err != nil {
				return err
			}
			return presenterFor(cmd, false).Added(ws, added)
		},
	}
	cmd.Flags().StringVarP(&repos, "repos", "r", "", "comma-separated repository names")
	cmd.Flags().StringVar(&shared, "shared", "", "comma-separated repositories to link in instead of checking out")
	cmd.Flags().BoolVar(&all, "all", false, "include every discovered repository not yet in the workspace (except --shared ones)")
	return cmd
}
