package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status [name]",
		Short: "Show git status for each repository in a workspace",
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
			ws, err := svc.Resolve(name)
			if err != nil {
				return err
			}
			rows := svc.Status(ws)
			if asJSON {
				return printJSON(map[string]any{
					"name":   ws.Name,
					"branch": ws.Branch,
					"path":   ws.Path,
					"repos":  rows,
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s  %s\n", ws.Name, ws.Branch)
			table := make([][]string, 0, len(rows))
			for _, row := range rows {
				errCol := row.Error
				table = append(table, []string{
					row.Repo,
					row.Branch,
					strconv.Itoa(row.Dirty),
					strconv.Itoa(row.Ahead),
					strconv.Itoa(row.Behind),
					errCol,
				})
			}
			printTable([]string{"REPO", "BRANCH", "DIRTY", "AHEAD", "BEHIND", "ERROR"}, table)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&asJSON, "json", "j", false, "print JSON")
	return cmd
}
