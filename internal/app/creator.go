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
	// SharedNames are repositories linked into the workspace rather than
	// checked out as worktrees.
	SharedNames []string
	All         bool
}

// AddOpts is input for adding repositories to an existing workspace.
type AddOpts struct {
	Name        string
	RepoNames   []string
	SharedNames []string
	All         bool
}

// Members is what an add or remove attempt changed in a workspace: the
// worktree repositories and the shared repository links.
type Members struct {
	Repos  []domain.RepoWorktree
	Shared []domain.SharedRepo
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
	selected, shared, err := c.selectRepos(opts.RepoNames, opts.SharedNames, opts.All)
	if err != nil {
		return domain.Workspace{}, err
	}
	if len(selected) == 0 {
		if len(shared) > 0 {
			return domain.Workspace{}, fmt.Errorf("a workspace needs at least one worktree repository; select one with --repos or --all")
		}
		return domain.Workspace{}, fmt.Errorf("select repositories with --repos or --all")
	}

	wsPath := filepath.Join(c.cfg.WorkspaceDir, name)
	var result domain.Workspace
	err = c.registry.WithLock(func() error {
		ws, err := c.provision(name, branch, wsPath, selected, shared)
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

// selectRepos resolves the flags against the discovered repositories. It
// returns the repositories to check out and the ones to link; either may be
// empty, and each caller decides which combinations it accepts. With all, the
// worktree set is every discovered repository except the shared ones.
func (c *Creator) selectRepos(repoNames, sharedNames []string, all bool) (worktrees, shared []domain.Repo, err error) {
	if all && len(repoNames) > 0 {
		return nil, nil, fmt.Errorf("use either --repos or --all, not both")
	}
	if err := domain.ValidateDisjoint(repoNames, sharedNames); err != nil {
		return nil, nil, err
	}
	found, skipped := c.discoverer.Discover(c.cfg.RepoDirs)
	for _, p := range skipped {
		c.warner.Warn("skipped duplicate repo name: " + p)
	}
	if len(found) == 0 {
		return nil, nil, fmt.Errorf("no git repositories found under repo_dirs")
	}

	shared, err = domain.SelectRepos(found, sharedNames)
	if err != nil {
		return nil, nil, err
	}
	switch {
	case all:
		worktrees = withoutRepos(found, shared)
	case len(repoNames) > 0:
		worktrees, err = domain.SelectRepos(found, repoNames)
		if err != nil {
			return nil, nil, err
		}
	}
	return worktrees, shared, nil
}

// withoutRepos returns repos minus the ones in drop, keeping order.
func withoutRepos(repos, drop []domain.Repo) []domain.Repo {
	dropped := make(map[string]bool, len(drop))
	for _, r := range drop {
		dropped[r.Name] = true
	}
	var out []domain.Repo
	for _, r := range repos {
		if !dropped[r.Name] {
			out = append(out, r)
		}
	}
	return out
}

// provision runs under the registry lock.
func (c *Creator) provision(name, branch, wsPath string, selected, shared []domain.Repo) (domain.Workspace, error) {
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
	worktrees, err := c.addAll(selected, wsPath, branch, rollback)
	if err != nil {
		return domain.Workspace{}, err
	}
	links, err := c.linkAll(shared, wsPath, rollback)
	if err != nil {
		return domain.Workspace{}, err
	}
	ws.Repos = worktrees
	ws.Shared = links
	if err := writeCodeWorkspace(c.fs, ws); err != nil {
		rollback.undo()
		return domain.Workspace{}, err
	}

	reg.Upsert(ws)
	if err := c.registry.Save(reg); err != nil {
		rollback.undo()
		return domain.Workspace{}, err
	}
	return ws, nil
}

// Add provisions worktrees and shared links for more repositories in an
// existing workspace, on the branch the workspace already uses. A failure
// removes only what this attempt added; the workspace directory and its earlier
// members stay.
func (c *Creator) Add(opts AddOpts) (domain.Workspace, Members, error) {
	selected, shared, err := c.selectRepos(opts.RepoNames, opts.SharedNames, opts.All)
	if err != nil {
		return domain.Workspace{}, Members{}, err
	}
	if len(selected) == 0 && len(shared) == 0 {
		return domain.Workspace{}, Members{}, fmt.Errorf("select repositories with --repos, --all or --shared")
	}

	var (
		result domain.Workspace
		added  Members
	)
	err = c.registry.WithLock(func() error {
		ws, members, err := c.extend(opts, selected, shared)
		if err != nil {
			return err
		}
		result, added = ws, members
		return nil
	})
	return result, added, err
}

// extend runs under the registry lock.
func (c *Creator) extend(opts AddOpts, selected, shared []domain.Repo) (domain.Workspace, Members, error) {
	reg, err := c.registry.Load()
	if err != nil {
		return domain.Workspace{}, Members{}, err
	}
	ws, ok := reg.Get(opts.Name)
	if !ok {
		return domain.Workspace{}, Members{}, fmt.Errorf("workspace %q not found", opts.Name)
	}
	if isDir, err := c.fs.IsDir(ws.Path); err != nil || !isDir {
		return domain.Workspace{}, Members{}, fmt.Errorf("workspace directory missing: %s (run bosk doctor)", ws.Path)
	}

	toAdd, err := newRepos(ws, selected, opts.All)
	if err != nil {
		return domain.Workspace{}, Members{}, err
	}
	if err := checkShared(ws, shared); err != nil {
		return domain.Workspace{}, Members{}, err
	}
	if len(toAdd) == 0 && len(shared) == 0 {
		return domain.Workspace{}, Members{}, fmt.Errorf("no repositories to add")
	}
	// Lstat, not stat: a dangling symlink at the target path still blocks us.
	for _, repo := range append(append([]domain.Repo(nil), toAdd...), shared...) {
		path := filepath.Join(ws.Path, repo.Name)
		if exists, err := c.fs.LinkExists(path); err != nil {
			return domain.Workspace{}, Members{}, err
		} else if exists {
			return domain.Workspace{}, Members{}, fmt.Errorf("%s: path already exists: %s", repo.Name, path)
		}
	}

	c.fetchAll(toAdd)

	rollback := newRollbackLog(c.git, c.fs, "")
	worktrees, err := c.addAll(toAdd, ws.Path, ws.Branch, rollback)
	if err != nil {
		return domain.Workspace{}, Members{}, err
	}
	links, err := c.linkAll(shared, ws.Path, rollback)
	if err != nil {
		return domain.Workspace{}, Members{}, err
	}
	origWS := ws
	origWS.Repos = append([]domain.RepoWorktree(nil), ws.Repos...)
	origWS.Shared = append([]domain.SharedRepo(nil), ws.Shared...)

	ws.Repos = append(ws.Repos, worktrees...)
	if ws, err = ws.WithShared(links); err != nil {
		rollback.undo()
		return domain.Workspace{}, Members{}, err
	}
	if err := writeCodeWorkspace(c.fs, ws); err != nil {
		rollback.undo()
		return domain.Workspace{}, Members{}, err
	}

	reg.Upsert(ws)
	if err := c.registry.Save(reg); err != nil {
		_ = writeCodeWorkspace(c.fs, origWS)
		rollback.undo()
		return domain.Workspace{}, Members{}, err
	}
	return ws, Members{Repos: worktrees, Shared: links}, nil
}

// newRepos drops repositories the workspace already holds. Naming one
// explicitly is an error; --all just skips it.
func newRepos(ws domain.Workspace, selected []domain.Repo, all bool) ([]domain.Repo, error) {
	var out []domain.Repo
	for _, repo := range selected {
		if !ws.HasMember(repo.Name) {
			out = append(out, repo)
			continue
		}
		if !all {
			return nil, fmt.Errorf("repo %q is already in workspace %q", repo.Name, ws.Name)
		}
	}
	return out, nil
}

// checkShared rejects a shared repository the workspace already holds, whether as
// a worktree or as a link.
func checkShared(ws domain.Workspace, shared []domain.Repo) error {
	for _, repo := range shared {
		if ws.HasMember(repo.Name) {
			return fmt.Errorf("repo %q is already in workspace %q", repo.Name, ws.Name)
		}
	}
	return nil
}

// linkAll links each shared repository into the workspace directory, recording
// each in rollback. On a failure it undoes the attempt before returning.
func (c *Creator) linkAll(repos []domain.Repo, wsPath string, rollback *rollbackLog) ([]domain.SharedRepo, error) {
	var out []domain.SharedRepo
	for _, repo := range repos {
		link := filepath.Join(wsPath, repo.Name)
		if err := c.fs.Symlink(repo.Path, link); err != nil {
			rollback.undo()
			return nil, fmt.Errorf("%s: %w", repo.Name, err)
		}
		rollback.recordLink(link)
		out = append(out, domain.SharedRepo{RepoName: repo.Name, SourceRepo: repo.Path, LinkPath: link})
	}
	return out, nil
}

// addAll adds one worktree per repository, recording each in rollback. On a
// failure it undoes the attempt before returning.
func (c *Creator) addAll(repos []domain.Repo, wsPath, branch string, rollback *rollbackLog) ([]domain.RepoWorktree, error) {
	var out []domain.RepoWorktree
	for _, repo := range repos {
		worktree, err := c.addWorktree(repo, wsPath, branch)
		if err != nil {
			rollback.undo()
			return nil, fmt.Errorf("%s: %w", repo.Name, err)
		}
		rollback.record(createdWorktree{
			source:        repo.Path,
			path:          worktree.WorktreePath,
			branch:        branch,
			createdBranch: worktree.CreatedBranch,
		})
		out = append(out, worktree)
	}
	return out, nil
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

// writeCodeWorkspace writes or updates the .code-workspace file for ws,
// preserving any non-folders top-level keys already in the file.
func writeCodeWorkspace(fs FileSystem, ws domain.Workspace) error {
	return writeCodeWorkspaceFolders(fs, ws.CodeWorkspacePath(), ws.CodeWorkspaceFolders())
}

func writeCodeWorkspaceFolders(fs FileSystem, path string, folders []domain.CodeWorkspaceFolder) error {
	existing, _ := fs.ReadFile(path)
	data, err := domain.RenderCodeWorkspace(folders, existing)
	if err != nil {
		return err
	}
	return fs.WriteFile(path, data)
}
