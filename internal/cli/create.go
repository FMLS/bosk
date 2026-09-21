package cli

import (
	"fmt"

	"github.com/FMLS/bosk/internal/workspace"
	"github.com/spf13/cobra"
)

func newCreateCmd() *cobra.Command {
	var (
		branch string
		repos  string
		all    bool
	)
	cmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a workspace of linked git worktrees",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := loadService()
			if err != nil {
				return err
			}
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			ws, err := svc.Create(workspace.CreateOpts{
				Name:      name,
				Branch:    branch,
				RepoNames: splitCSV(repos),
				All:       all,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "created workspace %s\n", ws.Name)
			for _, repo := range ws.Repos {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s\t%s\n", repo.RepoName, repo.WorktreePath)
			}
			fmt.Fprintln(cmd.OutOrStdout(), ws.Path)
			return nil
		},
	}
	cmd.Flags().StringVarP(&branch, "branch", "b", "", "branch name shared by each repository")
	cmd.Flags().StringVarP(&repos, "repos", "r", "", "comma-separated repository names")
	cmd.Flags().BoolVar(&all, "all", false, "include every discovered repository")
	return cmd
}
