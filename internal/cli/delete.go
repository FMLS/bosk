package cli

import (
	"fmt"

	"github.com/FMLS/bosk/internal/app"
	"github.com/spf13/cobra"
)

func newDeleteCmd() *cobra.Command {
	var (
		yes          bool
		force        bool
		deleteBranch bool
		forceBranch  bool
	)
	cmd := &cobra.Command{
		Use:     "delete <name>",
		Aliases: []string{"rm"},
		Short:   "Delete a workspace and its worktrees",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newContainer(cmd)
			inspector, err := c.inspector()
			if err != nil {
				return err
			}
			name := args[0]
			ws, err := inspector.Get(name)
			if err != nil {
				return err
			}

			if !yes {
				prompt := fmt.Sprintf("Delete workspace %s (%d repos) at %s? [y/N] ", ws.Name, len(ws.Repos), ws.Path)
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
			if err := remover.Delete(name, app.DeleteOpts{
				Force:        force,
				DeleteBranch: deleteBranch,
				ForceBranch:  forceBranch,
			}); err != nil {
				return err
			}
			return presenterFor(cmd, false).Deleted(name)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "do not prompt for confirmation")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "delete even if worktrees are dirty or drifted")
	cmd.Flags().BoolVar(&deleteBranch, "delete-branch", false, "delete local branches created by bosk")
	cmd.Flags().BoolVar(&forceBranch, "force-delete-branch", false, "use git branch -D when deleting branches")
	return cmd
}
