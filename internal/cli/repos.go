package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newReposCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "repos",
		Short: "List discovered source repositories",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := loadService()
			if err != nil {
				return err
			}
			repos, skipped := svc.Discovered()
			for _, p := range skipped {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: skipped duplicate repo name: %s\n", p)
			}
			if asJSON {
				return printJSON(repos)
			}
			if len(repos) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no repositories found")
				return nil
			}
			rows := make([][]string, 0, len(repos))
			for _, r := range repos {
				rows = append(rows, []string{r.Name, r.Path})
			}
			printTable([]string{"NAME", "PATH"}, rows)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&asJSON, "json", "j", false, "print JSON")
	return cmd
}
