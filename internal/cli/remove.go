package cli

import (
	"fmt"
	"strings"

	"github.com/FMLS/bosk/internal/app"
	"github.com/spf13/cobra"
)

// There is deliberately no "rm" alias: that already belongs to delete.
func newRemoveCmd() *cobra.Command {
	var (
		repos        string
		yes          bool
		force        bool
		deleteBranch bool
		forceBranch  bool
	)
	cmd := &cobra.Command{
		Use:   "remove [name]",
		Short: "Remove repositories from an existing workspace",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			names := splitCSV(repos)
			if len(names) == 0 {
				return fmt.Errorf("no repositories selected; pass --repos")
			}
			c := newContainer(cmd)
			inspector, err := c.inspector()
			if err != nil {
				return err
			}
			ws, err := inspector.Resolve(firstArg(args))
			if err != nil {
				return err
			}

			if !yes {
				prompt := fmt.Sprintf("Remove %s from workspace %s? [y/N] ", strings.Join(names, ", "), ws.Name)
				ok, err := c.confirmer.Confirm(prompt)
				if err != nil {
					return err
				}
				if !ok {
					return fmt.Errorf("aborted")
				}
			}

			remover, err := c.remover()
			if err != nil {
				return err
			}
			ws, removed, err := remover.RemoveRepos(ws.Name, names, app.DeleteOpts{
				Force:        force,
				DeleteBranch: deleteBranch,
				ForceBranch:  forceBranch,
			})
			if err != nil {
				return err
			}
			return presenterFor(cmd, false).Removed(ws, removed)
		},
	}
	cmd.Flags().StringVarP(&repos, "repos", "r", "", "comma-separated repository names to remove (worktree or shared)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "do not prompt for confirmation")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "remove even if worktrees are dirty or drifted")
	cmd.Flags().BoolVar(&deleteBranch, "delete-branch", false, "delete local branches created by bosk")
	cmd.Flags().BoolVar(&forceBranch, "force-delete-branch", false, "use git branch -D when deleting branches")
	return cmd
}
