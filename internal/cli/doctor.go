package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	var (
		asJSON bool
		fix    bool
	)
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Compare recorded workspaces with git and the filesystem",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := loadService()
			if err != nil {
				return err
			}
			issues, err := svc.Doctor()
			if err != nil {
				return err
			}
			if fix {
				if err := svc.Fix(issues); err != nil {
					return err
				}
				issues, err = svc.Doctor()
				if err != nil {
					return err
				}
			}
			if asJSON {
				return printJSON(issues)
			}
			if len(issues) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "all workspaces healthy")
				return nil
			}
			rows := make([][]string, 0, len(issues))
			for _, issue := range issues {
				rows = append(rows, []string{issue.Workspace, issue.Repo, issue.Issue, issue.Action})
			}
			printTable([]string{"WORKSPACE", "REPO", "ISSUE", "ACTION"}, rows)
			if !fix {
				return fmt.Errorf("doctor found %d issue(s)", len(issues))
			}
			if len(issues) > 0 {
				return fmt.Errorf("doctor still reports %d issue(s)", len(issues))
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&asJSON, "json", "j", false, "print JSON")
	cmd.Flags().BoolVar(&fix, "fix", false, "apply conservative repairs")
	return cmd
}
