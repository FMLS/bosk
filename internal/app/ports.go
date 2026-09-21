// Package app holds bosk's use cases. Every outside effect it needs is
// declared here as an interface and supplied by the caller, so the services in
// this package depend on no concrete adapter.
package app

import (
	"time"

	"github.com/FMLS/bosk/internal/domain"
)

// --- git ---------------------------------------------------------------
//
// The interfaces below are split by what each use case actually needs. A
// single git adapter satisfies all of them.

// Fetcher updates remotes.
type Fetcher interface {
	Fetch(repo string) error
}

// BranchInspector answers questions about branches without changing them.
type BranchInspector interface {
	ValidBranch(name string) bool
	BranchExists(repo, branch string) bool
	BranchInUse(repo, branch string) (bool, error)
	StartPoint(repo, base string) (string, error)
}

// BranchWriter deletes local branches.
type BranchWriter interface {
	DeleteBranch(repo, branch string, force bool) error
}

// WorktreeWriter manipulates git worktree registrations.
type WorktreeWriter interface {
	WorktreeAdd(repo, path, branch string) error
	WorktreeAddNew(repo, path, branch, startPoint string) error
	WorktreeRemove(repo, path string, force bool) error
	WorktreePrune(repo string) error
	WorktreeRepair(repo, path string) error
}

// RepoProbe reports whether a path is a git repository.
type RepoProbe interface {
	IsRepo(path string) bool
}

// WorktreeRegistry reports whether git still knows about a worktree.
type WorktreeRegistry interface {
	WorktreeRegistered(repo, path string) (bool, error)
}

// StatusReader reads live state from a worktree.
type StatusReader interface {
	CurrentBranch(path string) (string, error)
	DirtyCount(path string) (int, error)
	AheadBehind(path string) (ahead, behind int, err error)
}

// CreatorGit is the git surface workspace creation needs.
type CreatorGit interface {
	Fetcher
	BranchInspector
	BranchWriter
	WorktreeWriter
}

// RemoverGit is the git surface workspace removal needs.
type RemoverGit interface {
	RepoProbe
	WorktreeRegistry
	StatusReader
	WorktreeWriter
	BranchWriter
}

// DoctorGit is the git surface health checks need.
type DoctorGit interface {
	RepoProbe
	WorktreeRegistry
}

// --- persistence -------------------------------------------------------

// Registry persists the set of workspaces bosk created.
type Registry interface {
	Load() (*domain.Registry, error)
	Save(reg *domain.Registry) error
	// WithLock runs fn while holding an exclusive cross-process lock.
	WithLock(fn func() error) error
}

// ConfigStore persists the global configuration and knows where it lives.
type ConfigStore interface {
	// Load returns (nil, nil) when no config file exists yet.
	Load() (*domain.Config, error)
	Save(cfg *domain.Config) error
	Path() string
	Home() string
	DefaultWorkspaceDir() string
}

// --- filesystem --------------------------------------------------------

// FileSystem is the narrow set of filesystem effects bosk performs. It is
// shaped around bosk's needs rather than mirroring the os package.
type FileSystem interface {
	Exists(path string) (bool, error)
	IsDir(path string) (bool, error)
	MkdirAll(path string) error
	Rename(oldPath, newPath string) error
	RemoveAll(path string) error
	// ListDirs returns the names of the directory entries that are
	// themselves directories.
	ListDirs(path string) ([]string, error)
	// ListEntries returns the names of every entry in a directory.
	ListEntries(path string) ([]string, error)
	Getwd() (string, error)
}

// PathResolver turns user-supplied paths into comparable absolute paths.
type PathResolver interface {
	// Normalize returns an absolute, symlink-resolved path for comparison.
	Normalize(path string) string
	// Resolve expands a leading ~ and makes the path absolute.
	Resolve(path string) (string, error)
}

// --- discovery ---------------------------------------------------------

// RepoDiscoverer finds source repositories under the configured roots. The
// second return value lists paths skipped for having a duplicate name.
type RepoDiscoverer interface {
	Discover(roots []string) ([]domain.Repo, []string)
}

// --- ambient -----------------------------------------------------------

// Clock supplies the current time.
type Clock interface {
	Now() time.Time
}

// Warner reports non-fatal problems to the user.
type Warner interface {
	Warn(msg string)
}
