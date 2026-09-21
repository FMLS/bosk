package models

import "time"

// Config is the global bosk configuration (~/.bosk/config.yaml).
type Config struct {
	RepoDirs     []string `yaml:"repo_dirs" json:"repo_dirs"`
	WorkspaceDir string   `yaml:"workspace_dir" json:"workspace_dir"`
}

// RepoWorktree is one repository checkout inside a workspace.
type RepoWorktree struct {
	RepoName      string `json:"repo_name"`
	SourceRepo    string `json:"source_repo"`
	WorktreePath  string `json:"worktree_path"`
	Branch        string `json:"branch"`
	CreatedBranch bool   `json:"created_branch,omitempty"`
}

// Workspace is a named collection of worktrees sharing a branch name.
type Workspace struct {
	Name      string         `json:"name"`
	Path      string         `json:"path"`
	Branch    string         `json:"branch"`
	CreatedAt string         `json:"created_at"`
	Repos     []RepoWorktree `json:"repos"`
}

// NewWorkspace builds a workspace record with a UTC timestamp.
func NewWorkspace(name, path, branch string) Workspace {
	return Workspace{
		Name:      name,
		Path:      path,
		Branch:    branch,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Repos:     nil,
	}
}

// FindRepo returns the worktree record for a repo name.
func (w *Workspace) FindRepo(name string) *RepoWorktree {
	for i := range w.Repos {
		if w.Repos[i].RepoName == name {
			return &w.Repos[i]
		}
	}
	return nil
}

// RepoNames returns repository names in record order.
func (w *Workspace) RepoNames() []string {
	names := make([]string, len(w.Repos))
	for i, r := range w.Repos {
		names[i] = r.RepoName
	}
	return names
}

// Repo is a discovered source repository.
type Repo struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// RepoStatus is live git status for one worktree.
type RepoStatus struct {
	Repo     string `json:"repo"`
	Branch   string `json:"branch"`
	Expected string `json:"expected"`
	Dirty    int    `json:"dirty"`
	Ahead    int    `json:"ahead"`
	Behind   int    `json:"behind"`
	Error    string `json:"error,omitempty"`
}

// DoctorIssue is one health-check finding.
type DoctorIssue struct {
	Workspace string `json:"workspace"`
	Repo      string `json:"repo,omitempty"`
	Issue     string `json:"issue"`
	Action    string `json:"suggested_action"`
	Fixable   bool   `json:"fixable"`
}
