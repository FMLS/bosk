package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/FMLS/bosk/internal/app"
	"github.com/FMLS/bosk/internal/domain"
	"github.com/spf13/cobra"
)

// Presenter renders command results. Commands call it without knowing whether
// the user asked for a table or for JSON.
type Presenter interface {
	Initialized(res app.InitResult) error
	Created(ws domain.Workspace) error
	Deleted(name string) error
	Workspaces(list []domain.Workspace) error
	Repos(repos []domain.Repo) error
	Status(ws domain.Workspace, rows []domain.RepoStatus) error
	Issues(issues []domain.Issue) error
	Path(path string) error
	// MachineReadable reports whether the output is meant for a program. The
	// doctor command uses it to decide whether findings become a non-zero
	// exit status or just payload.
	MachineReadable() bool
}

// presenterFor picks a renderer for one command invocation.
func presenterFor(cmd *cobra.Command, asJSON bool) Presenter {
	if asJSON {
		return jsonPresenter{out: cmd.OutOrStdout()}
	}
	return textPresenter{out: cmd.OutOrStdout()}
}

// --- text ---------------------------------------------------------------

type textPresenter struct {
	out io.Writer
}

func (p textPresenter) MachineReadable() bool { return false }

func (p textPresenter) Initialized(res app.InitResult) error {
	fmt.Fprintf(p.out, "initialized %s\n", res.ConfigPath)
	fmt.Fprintf(p.out, "workspace dir: %s\n", res.Config.WorkspaceDir)
	fmt.Fprintf(p.out, "discovered %d repositories\n", len(res.Repos))
	for _, r := range res.Repos {
		fmt.Fprintf(p.out, "  %s\t%s\n", r.Name, r.Path)
	}
	return nil
}

func (p textPresenter) Created(ws domain.Workspace) error {
	fmt.Fprintf(p.out, "created workspace %s\n", ws.Name)
	for _, repo := range ws.Repos {
		fmt.Fprintf(p.out, "  %s\t%s\n", repo.RepoName, repo.WorktreePath)
	}
	fmt.Fprintln(p.out, ws.Path)
	return nil
}

func (p textPresenter) Deleted(name string) error {
	fmt.Fprintf(p.out, "deleted workspace %s\n", name)
	return nil
}

func (p textPresenter) Workspaces(list []domain.Workspace) error {
	if len(list) == 0 {
		fmt.Fprintln(p.out, "no workspaces")
		return nil
	}
	rows := make([][]string, 0, len(list))
	for _, ws := range list {
		rows = append(rows, []string{ws.Name, ws.Branch, strconv.Itoa(len(ws.Repos)), ws.Path})
	}
	p.table([]string{"NAME", "BRANCH", "REPOS", "PATH"}, rows)
	return nil
}

func (p textPresenter) Repos(repos []domain.Repo) error {
	if len(repos) == 0 {
		fmt.Fprintln(p.out, "no repositories found")
		return nil
	}
	rows := make([][]string, 0, len(repos))
	for _, r := range repos {
		rows = append(rows, []string{r.Name, r.Path})
	}
	p.table([]string{"NAME", "PATH"}, rows)
	return nil
}

func (p textPresenter) Status(ws domain.Workspace, statuses []domain.RepoStatus) error {
	fmt.Fprintf(p.out, "%s  %s\n", ws.Name, ws.Branch)
	rows := make([][]string, 0, len(statuses))
	for _, st := range statuses {
		rows = append(rows, []string{
			st.Repo,
			st.Branch,
			strconv.Itoa(st.Dirty),
			strconv.Itoa(st.Ahead),
			strconv.Itoa(st.Behind),
			st.Error,
		})
	}
	p.table([]string{"REPO", "BRANCH", "DIRTY", "AHEAD", "BEHIND", "ERROR"}, rows)
	return nil
}

func (p textPresenter) Issues(issues []domain.Issue) error {
	if len(issues) == 0 {
		fmt.Fprintln(p.out, "all workspaces healthy")
		return nil
	}
	rows := make([][]string, 0, len(issues))
	for _, issue := range issues {
		rows = append(rows, []string{issue.Workspace, issue.Repo, issue.Message(), issue.SuggestedAction()})
	}
	p.table([]string{"WORKSPACE", "REPO", "ISSUE", "ACTION"}, rows)
	return nil
}

func (p textPresenter) Path(path string) error {
	fmt.Fprintln(p.out, path)
	return nil
}

func (p textPresenter) table(headers []string, rows [][]string) {
	w := tabwriter.NewWriter(p.out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(headers, "\t"))
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	w.Flush()
}

// --- json ---------------------------------------------------------------

type jsonPresenter struct {
	out io.Writer
}

func (p jsonPresenter) MachineReadable() bool { return true }

func (p jsonPresenter) Initialized(res app.InitResult) error {
	return p.encode(map[string]any{
		"config":        res.ConfigPath,
		"workspace_dir": res.Config.WorkspaceDir,
		"repos":         res.Repos,
	})
}

func (p jsonPresenter) Created(ws domain.Workspace) error { return p.encode(ws) }

func (p jsonPresenter) Deleted(name string) error {
	return p.encode(map[string]any{"deleted": name})
}

func (p jsonPresenter) Workspaces(list []domain.Workspace) error { return p.encode(list) }

func (p jsonPresenter) Repos(repos []domain.Repo) error { return p.encode(repos) }

func (p jsonPresenter) Status(ws domain.Workspace, statuses []domain.RepoStatus) error {
	return p.encode(map[string]any{
		"name":   ws.Name,
		"branch": ws.Branch,
		"path":   ws.Path,
		"repos":  statuses,
	})
}

// issueJSON is the wire shape of a finding. Kind is the field a script should
// branch on; the rendered message and action stay for humans reading the log.
type issueJSON struct {
	Workspace string           `json:"workspace"`
	Repo      string           `json:"repo,omitempty"`
	Issue     string           `json:"issue"`
	Action    string           `json:"suggested_action"`
	Fixable   bool             `json:"fixable"`
	Kind      domain.IssueKind `json:"kind"`
}

func (p jsonPresenter) Issues(issues []domain.Issue) error {
	var out []issueJSON
	for _, issue := range issues {
		out = append(out, issueJSON{
			Workspace: issue.Workspace,
			Repo:      issue.Repo,
			Issue:     issue.Message(),
			Action:    issue.SuggestedAction(),
			Fixable:   issue.Fixable(),
			Kind:      issue.Kind,
		})
	}
	return p.encode(out)
}

func (p jsonPresenter) Path(path string) error {
	return p.encode(map[string]any{"path": path})
}

func (p jsonPresenter) encode(v any) error {
	enc := json.NewEncoder(p.out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
