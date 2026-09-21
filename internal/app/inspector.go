package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/FMLS/bosk/internal/domain"
)

// Inspector answers questions about workspaces without changing anything.
type Inspector struct {
	cfg        *domain.Config
	git        StatusReader
	registry   Registry
	discoverer RepoDiscoverer
	fs         FileSystem
	paths      PathResolver
}

// NewInspector wires the read-only use cases to their collaborators.
func NewInspector(
	cfg *domain.Config,
	git StatusReader,
	registry Registry,
	discoverer RepoDiscoverer,
	fs FileSystem,
	paths PathResolver,
) *Inspector {
	return &Inspector{cfg: cfg, git: git, registry: registry, discoverer: discoverer, fs: fs, paths: paths}
}

// List returns recorded workspaces.
func (i *Inspector) List() ([]domain.Workspace, error) {
	reg, err := i.registry.Load()
	if err != nil {
		return nil, err
	}
	return reg.Workspaces, nil
}

// Get returns one workspace by name.
func (i *Inspector) Get(name string) (domain.Workspace, error) {
	reg, err := i.registry.Load()
	if err != nil {
		return domain.Workspace{}, err
	}
	ws, ok := reg.Get(name)
	if !ok {
		return domain.Workspace{}, fmt.Errorf("workspace %q not found", name)
	}
	return ws, nil
}

// Resolve returns a named workspace, or the workspace containing the working
// directory when no name is given.
func (i *Inspector) Resolve(name string) (domain.Workspace, error) {
	if name != "" {
		return i.Get(name)
	}
	cwd, err := i.fs.Getwd()
	if err != nil {
		return domain.Workspace{}, err
	}
	reg, err := i.registry.Load()
	if err != nil {
		return domain.Workspace{}, err
	}
	ws, ok := i.containing(reg, cwd)
	if !ok {
		return domain.Workspace{}, fmt.Errorf("not inside a bosk workspace; pass a name")
	}
	return ws, nil
}

// containing finds the most deeply nested workspace that contains path.
func (i *Inspector) containing(reg *domain.Registry, path string) (domain.Workspace, bool) {
	want := i.paths.Normalize(path)
	var best domain.Workspace
	bestLen := -1
	for _, w := range reg.Workspaces {
		root := i.paths.Normalize(w.Path)
		if want == root || strings.HasPrefix(want, root+string(filepath.Separator)) {
			if len(root) > bestLen {
				best = w
				bestLen = len(root)
			}
		}
	}
	if bestLen < 0 {
		return domain.Workspace{}, false
	}
	return best, true
}

// Discovered lists source repositories and the paths skipped as duplicates.
func (i *Inspector) Discovered() ([]domain.Repo, []string) {
	return i.discoverer.Discover(i.cfg.RepoDirs)
}

// Status reports live git status for each worktree. Per-repo failures are
// reported in the row rather than aborting the whole report.
func (i *Inspector) Status(ws domain.Workspace) []domain.RepoStatus {
	out := make([]domain.RepoStatus, 0, len(ws.Repos))
	for _, repo := range ws.Repos {
		out = append(out, i.repoStatus(repo))
	}
	return out
}

func (i *Inspector) repoStatus(repo domain.RepoWorktree) domain.RepoStatus {
	st := domain.RepoStatus{Repo: repo.RepoName, Expected: repo.Branch}

	exists, err := i.fs.Exists(repo.WorktreePath)
	if err != nil || !exists {
		st.Error = "worktree missing"
		return st
	}
	branch, err := i.git.CurrentBranch(repo.WorktreePath)
	if err != nil {
		st.Error = err.Error()
		return st
	}
	if branch == "" {
		branch = "(detached)"
	}
	st.Branch = branch

	dirty, err := i.git.DirtyCount(repo.WorktreePath)
	if err != nil {
		st.Error = err.Error()
		return st
	}
	st.Dirty = dirty

	ahead, behind, err := i.git.AheadBehind(repo.WorktreePath)
	if err != nil {
		st.Error = err.Error()
		return st
	}
	st.Ahead = ahead
	st.Behind = behind
	return st
}
