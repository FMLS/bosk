// Package domain holds bosk's core types and rules. It performs no I/O and
// imports no other bosk package, so every rule here is testable in isolation.
package domain

import "time"

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

// NewWorkspace builds a workspace record stamped with the given time.
func NewWorkspace(name, path, branch string, createdAt time.Time) Workspace {
	return Workspace{
		Name:      name,
		Path:      path,
		Branch:    branch,
		CreatedAt: createdAt.UTC().Format(time.RFC3339),
		Repos:     nil,
	}
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

// Worktree is one linked worktree reported by git.
type Worktree struct {
	Path   string
	Branch string
}

// Config is the global bosk configuration (~/.bosk/config.yaml).
type Config struct {
	RepoDirs     []string `yaml:"repo_dirs" json:"repo_dirs"`
	WorkspaceDir string   `yaml:"workspace_dir" json:"workspace_dir"`
}
