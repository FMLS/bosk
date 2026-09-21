package cli

import (
	"fmt"
	"os"

	"github.com/FMLS/bosk/internal/workspace"
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
			svc, err := loadService()
			if err != nil {
				return err
			}
			name := args[0]
			ws, err := svc.Get(name)
			if err != nil {
				return err
			}
			if !yes {
				if !isTTY() {
					return fmt.Errorf("refusing to delete without --yes in non-interactive mode")
				}
				prompt := fmt.Sprintf("Delete workspace %s (%d repos) at %s? [y/N] ", ws.Name, len(ws.Repos), ws.Path)
				if !confirm(os.Stdin, os.Stderr, prompt) {
					return fmt.Errorf("aborted")
				}
			}
			if err := svc.Delete(name, workspace.DeleteOpts{
				Force:        force,
				DeleteBranch: deleteBranch,
				ForceBranch:  forceBranch,
			}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "deleted workspace %s\n", name)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "do not prompt for confirmation")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "delete even if worktrees are dirty or drifted")
	cmd.Flags().BoolVar(&deleteBranch, "delete-branch", false, "delete local branches created by bosk")
	cmd.Flags().BoolVar(&forceBranch, "force-delete-branch", false, "use git branch -D when deleting branches")
	return cmd
}

func isTTY() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
