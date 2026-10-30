// Package domain holds bosk's core types and rules. It performs no I/O and
// imports no other bosk package, so every rule here is testable in isolation.
package domain

import (
	"fmt"
	"time"
)

// RepoWorktree is one repository checkout inside a workspace.
type RepoWorktree struct {
	RepoName      string `json:"repo_name"`
	SourceRepo    string `json:"source_repo"`
	WorktreePath  string `json:"worktree_path"`
	Branch        string `json:"branch"`
	CreatedBranch bool   `json:"created_branch,omitempty"`
}

// SharedRepo is a repository every workspace shares rather than checks out: the
// workspace holds a symlink to the source repository instead of a worktree.
type SharedRepo struct {
	RepoName   string `json:"repo_name"`
	SourceRepo string `json:"source_repo"`
	LinkPath   string `json:"link_path"`
}

// Workspace is a named collection of worktrees sharing a branch name, plus any
// shared repositories linked in alongside them.
type Workspace struct {
	Name      string         `json:"name"`
	Path      string         `json:"path"`
	Branch    string         `json:"branch"`
	CreatedAt string         `json:"created_at"`
	Repos     []RepoWorktree `json:"repos"`
	Shared    []SharedRepo   `json:"shared,omitempty"`
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

// WithoutRepos returns a copy of the workspace with the named repositories
// dropped. The receiver's Repos slice is left untouched.
func (w Workspace) WithoutRepos(names []string) Workspace {
	drop := make(map[string]bool, len(names))
	for _, n := range names {
		drop[n] = true
	}
	kept := make([]RepoWorktree, 0, len(w.Repos))
	for _, repo := range w.Repos {
		if !drop[repo.RepoName] {
			kept = append(kept, repo)
		}
	}
	w.Repos = kept
	return w
}

// HasMember reports whether name is already a repository or a shared
// repository of the workspace. Both live directly under the workspace
// directory, so a name can be used only once.
func (w Workspace) HasMember(name string) bool {
	for _, repo := range w.Repos {
		if repo.RepoName == name {
			return true
		}
	}
	for _, shared := range w.Shared {
		if shared.RepoName == name {
			return true
		}
	}
	return false
}

// WithShared returns a copy of the workspace with the shared repositories
// appended. It refuses a name the workspace already uses. The receiver's Shared
// slice is left untouched.
func (w Workspace) WithShared(added []SharedRepo) (Workspace, error) {
	grown := make([]SharedRepo, len(w.Shared), len(w.Shared)+len(added))
	copy(grown, w.Shared)
	w.Shared = grown
	for _, shared := range added {
		if w.HasMember(shared.RepoName) {
			return Workspace{}, fmt.Errorf("repo %q is already in workspace %q", shared.RepoName, w.Name)
		}
		w.Shared = append(w.Shared, shared)
	}
	return w, nil
}

// WithoutShared returns a copy of the workspace with the named shared
// repositories dropped. The receiver's Shared slice is left untouched.
func (w Workspace) WithoutShared(names []string) Workspace {
	drop := make(map[string]bool, len(names))
	for _, n := range names {
		drop[n] = true
	}
	var kept []SharedRepo
	for _, shared := range w.Shared {
		if !drop[shared.RepoName] {
			kept = append(kept, shared)
		}
	}
	w.Shared = kept
	return w
}

// SharedStatus is the live state of one shared repository link. Unlike a
// worktree it has no branch of its own, so only the link is checked.
type SharedStatus struct {
	Repo   string `json:"repo"`
	Path   string `json:"path"`
	Target string `json:"target"`
	Valid  bool   `json:"valid"`
	Error  string `json:"error,omitempty"`
}
