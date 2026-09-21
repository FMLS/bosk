package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/FMLS/bosk/internal/config"
	"github.com/FMLS/bosk/internal/models"
	"github.com/FMLS/bosk/internal/workspace"
	"github.com/spf13/cobra"
)

var version = "dev"

// Execute runs the bosk CLI.
func Execute() int {
	root := newRoot()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func newRoot() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "bosk",
		Short:         "Multi-repo git worktree workspaces",
		Long:          "bosk groups linked git worktrees into a named workspace so one task can span multiple repositories.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
	}
	cmd.AddCommand(
		newInitCmd(),
		newCreateCmd(),
		newListCmd(),
		newStatusCmd(),
		newDeleteCmd(),
		newDoctorCmd(),
		newGoCmd(),
		newReposCmd(),
		newShellInitCmd(),
	)
	return cmd
}

func loadService() (*workspace.Service, error) {
	cfg, err := config.Require()
	if err != nil {
		return nil, err
	}
	svc := workspace.New(cfg)
	svc.Warn = func(msg string) {
		fmt.Fprintln(os.Stderr, "warning:", msg)
	}
	return svc, nil
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printTable(headers []string, rows [][]string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(headers, "\t"))
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	w.Flush()
}

func splitCSV(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func confirm(r io.Reader, w io.Writer, prompt string) bool {
	fmt.Fprint(w, prompt)
	var answer string
	_, _ = fmt.Fscanln(r, &answer)
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}

func formatWorkspace(ws models.Workspace) []string {
	return []string{ws.Name, ws.Branch, fmt.Sprintf("%d", len(ws.Repos)), ws.Path}
}
