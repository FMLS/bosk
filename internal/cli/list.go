package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List workspaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := loadService()
			if err != nil {
				return err
			}
			workspaces, err := svc.List()
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(workspaces)
			}
			if len(workspaces) == 0 {
				fmt.Fprintln(os.Stdout, "no workspaces")
				return nil
			}
			rows := make([][]string, 0, len(workspaces))
			for _, ws := range workspaces {
				rows = append(rows, formatWorkspace(ws))
			}
			printTable([]string{"NAME", "BRANCH", "REPOS", "PATH"}, rows)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&asJSON, "json", "j", false, "print JSON")
	return cmd
}
