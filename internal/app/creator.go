package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/FMLS/bosk/internal/domain"
)

// CreateOpts is input for workspace creation.
type CreateOpts struct {
	Name      string
	Branch    string
	RepoNames []string
	All       bool
}

// Creator provisions a workspace of linked worktrees.
type Creator struct {
	cfg        *domain.Config
	git        CreatorGit
	registry   Registry
	discoverer RepoDiscoverer
	fs         FileSystem
	clock      Clock
	warner     Warner
}

// NewCreator wires the create use case to its collaborators.
func NewCreator(
	cfg *domain.Config,
	git CreatorGit,
	registry Registry,
	discoverer RepoDiscoverer,
	fs FileSystem,
	clock Clock,
	warner Warner,
) *Creator {
	return &Creator{cfg: cfg, git: git, registry: registry, discoverer: discoverer, fs: fs, clock: clock, warner: warner}
}

// Create provisions worktrees and records the workspace.
func (c *Creator) Create(opts CreateOpts) (domain.Workspace, error) {
	name, branch, err := c.resolveNames(opts)
	if err != nil {
		return domain.Workspace{}, err
	}
	selected, err := c.selectRepos(opts)
	if err != nil {
		return domain.Workspace{}, err
	}

	wsPath := filepath.Join(c.cfg.WorkspaceDir, name)
	var result domain.Workspace
	err = c.registry.WithLock(func() error {
		ws, err := c.provision(name, branch, wsPath, selected)
		if err != nil {
			return err
		}
		result = ws
		return nil
	})
	return result, err
}

// resolveNames derives the workspace name and branch from the flags and
// validates both before anything touches the disk.
func (c *Creator) resolveNames(opts CreateOpts) (name, branch string, err error) {
	branch = strings.TrimSpace(opts.Branch)
	name = strings.TrimSpace(opts.Name)
	if branch == "" && name == "" {
		return "", "", fmt.Errorf("workspace name or --branch is required")
	}
	if branch == "" {
		branch = name
	}
	if name == "" {
		name = domain.NameFromBranch(branch)
	}
	if err := domain.ValidateWorkspaceName(name); err != nil {
		return "", "", err
	}
	if !c.git.ValidBranch(branch) {
		return "", "", fmt.Errorf("invalid branch name %q", branch)
	}
	return name, branch, nil
}

func (c *Creator) selectRepos(opts CreateOpts) ([]domain.Repo, error) {
	if opts.All && len(opts.RepoNames) > 0 {
		return nil, fmt.Errorf("use either --repos or --all, not both")
	}
	found, skipped := c.discoverer.Discover(c.cfg.RepoDirs)
	for _, p := range skipped {
		c.warner.Warn("skipped duplicate repo name: " + p)
	}
	if len(found) == 0 {
		return nil, fmt.Errorf("no git repositories found under repo_dirs")
	}

	var selected []domain.Repo
	var err error
	switch {
	case opts.All:
		selected = found
	case len(opts.RepoNames) > 0:
		selected, err = domain.SelectRepos(found, opts.RepoNames)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("select repositories with --repos or --all")
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("no repositories selected")
	}
	return selected, nil
}

// provision runs under the registry lock.
func (c *Creator) provision(name, branch, wsPath string, selected []domain.Repo) (domain.Workspace, error) {
	reg, err := c.registry.Load()
	if err != nil {
		return domain.Workspace{}, err
	}
	if _, exists := reg.Get(name); exists {
		return domain.Workspace{}, fmt.Errorf("workspace %q already exists", name)
	}
	if isDir, err := c.fs.IsDir(wsPath); err == nil && isDir {
		return domain.Workspace{}, fmt.Errorf("workspace directory already exists: %s", wsPath)
	}
	if err := c.fs.MkdirAll(wsPath); err != nil {
		return domain.Workspace{}, err
	}

	c.fetchAll(selected)

	rollback := newRollbackLog(c.git, c.fs, wsPath)
	ws := domain.NewWorkspace(name, wsPath, branch, c.clock.Now())
	for _, repo := range selected {
		worktree, err := c.addWorktree(repo, wsPath, branch)
		if err != nil {
			rollback.undo()
			return domain.Workspace{}, fmt.Errorf("%s: %w", repo.Name, err)
		}
		rollback.record(createdWorktree{
			source:        repo.Path,
			path:          worktree.WorktreePath,
			branch:        branch,
			createdBranch: worktree.CreatedBranch,
		})
		ws.Repos = append(ws.Repos, worktree)
	}

	reg.Upsert(ws)
	if err := c.registry.Save(reg); err != nil {
		rollback.undo()
		return domain.Workspace{}, err
	}
	return ws, nil
}

// addWorktree checks out branch for one repository, creating the branch from
// the repo's default start point when it does not exist yet.
func (c *Creator) addWorktree(repo domain.Repo, wsPath, branch string) (domain.RepoWorktree, error) {
	inUse, err := c.git.BranchInUse(repo.Path, branch)
	if err != nil {
		return domain.RepoWorktree{}, err
	}
	if inUse {
		return domain.RepoWorktree{}, fmt.Errorf("branch %q is already checked out in another worktree", branch)
	}

	wtPath := filepath.Join(wsPath, repo.Name)
	createdBranch := false
	if c.git.BranchExists(repo.Path, branch) {
		err = c.git.WorktreeAdd(repo.Path, wtPath, branch)
	} else {
		start, startErr := c.git.StartPoint(repo.Path, "")
		if startErr != nil {
			return domain.RepoWorktree{}, startErr
		}
		err = c.git.WorktreeAddNew(repo.Path, wtPath, branch, start)
		createdBranch = true
	}
	if err != nil {
		return domain.RepoWorktree{}, err
	}
	return domain.RepoWorktree{
		RepoName:      repo.Name,
		SourceRepo:    repo.Path,
		WorktreePath:  wtPath,
		Branch:        branch,
		CreatedBranch: createdBranch,
	}, nil
}

// fetchAll refreshes every selected repository in parallel. A failed fetch is a
// warning, not an error: creating from stale refs still works.
func (c *Creator) fetchAll(repos []domain.Repo) {
	var wg sync.WaitGroup
	for _, repo := range repos {
		wg.Add(1)
		go func(name, path string) {
			defer wg.Done()
			if err := c.git.Fetch(path); err != nil {
				c.warner.Warn(fmt.Sprintf("fetch %s failed: %v", name, err))
			}
		}(repo.Name, repo.Path)
	}
	wg.Wait()
}
