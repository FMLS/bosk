package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/FMLS/bosk/internal/domain"
)

// DeleteOpts controls workspace removal.
type DeleteOpts struct {
	Force        bool
	DeleteBranch bool
	ForceBranch  bool
}

// WorkspaceRemover deletes a recorded workspace.
type WorkspaceRemover interface {
	Delete(name string, opts DeleteOpts) error
}

// Remover takes a workspace's worktrees and registry entry away.
type Remover struct {
	git      RemoverGit
	registry Registry
	fs       FileSystem
	clock    Clock
	warner   Warner
	trash    trashKeeper
}

// NewRemover wires the delete use case to its collaborators.
func NewRemover(
	cfg *domain.Config,
	git RemoverGit,
	registry Registry,
	fs FileSystem,
	clock Clock,
	warner Warner,
) *Remover {
	return &Remover{
		git:      git,
		registry: registry,
		fs:       fs,
		clock:    clock,
		warner:   warner,
		trash:    newTrashKeeper(cfg, fs),
	}
}

// Delete removes a workspace's worktrees and registry entry. The workspace
// directory is quarantined first so that a failure part-way through can put it
// back.
func (r *Remover) Delete(name string, opts DeleteOpts) error {
	return r.registry.WithLock(func() error {
		reg, err := r.registry.Load()
		if err != nil {
			return err
		}
		ws, ok := reg.Get(name)
		if !ok {
			return fmt.Errorf("workspace %q not found", name)
		}

		if !opts.Force {
			if err := r.preflight(ws); err != nil {
				return err
			}
		}

		// A workspace directory that is absent or unreadable has nothing to
		// quarantine; the registry entry still needs clearing.
		trashPath := ""
		isDir, statErr := r.fs.IsDir(ws.Path)
		if statErr == nil && isDir {
			trashPath, err = r.quarantine(ws)
			if err != nil {
				return err
			}
		}
		restore := func() { r.restore(ws, trashPath) }

		if err := r.pruneWorktrees(ws); err != nil {
			restore()
			return err
		}

		reg.Remove(name)
		if err := r.registry.Save(reg); err != nil {
			restore()
			return err
		}

		if opts.DeleteBranch {
			r.deleteBranches(ws, opts.ForceBranch)
		}

		if trashPath != "" {
			if err := r.trash.remove(trashPath); err != nil {
				r.warner.Warn("workspace moved to " + trashPath + "; remove it manually")
			}
		}
		return nil
	})
}

// preflight refuses to delete a workspace whose worktrees have drifted from
// what bosk recorded.
func (r *Remover) preflight(ws domain.Workspace) error {
	for _, repo := range ws.Repos {
		exists, err := r.fs.Exists(repo.WorktreePath)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if !r.git.IsRepo(repo.SourceRepo) {
			return fmt.Errorf("%s: source repository missing: %s", repo.RepoName, repo.SourceRepo)
		}
		ok, err := r.git.WorktreeRegistered(repo.SourceRepo, repo.WorktreePath)
		if err != nil {
			return fmt.Errorf("%s: %w", repo.RepoName, err)
		}
		if !ok {
			return fmt.Errorf("%s: worktree is not registered in git; use --force", repo.RepoName)
		}
		cur, err := r.git.CurrentBranch(repo.WorktreePath)
		if err != nil {
			return fmt.Errorf("%s: %w", repo.RepoName, err)
		}
		if cur != repo.Branch {
			return fmt.Errorf("%s: checked out %q, expected %q; use --force", repo.RepoName, cur, repo.Branch)
		}
		dirty, err := r.git.DirtyCount(repo.WorktreePath)
		if err != nil {
			return fmt.Errorf("%s: %w", repo.RepoName, err)
		}
		if dirty > 0 {
			return fmt.Errorf("%s: has uncommitted changes; use --force", repo.RepoName)
		}
	}
	return nil
}

func (r *Remover) pruneWorktrees(ws domain.Workspace) error {
	for _, repo := range ws.Repos {
		if !r.git.IsRepo(repo.SourceRepo) {
			continue
		}
		if err := r.git.WorktreePrune(repo.SourceRepo); err != nil {
			return fmt.Errorf("%s: %w", repo.RepoName, err)
		}
	}
	return nil
}

func (r *Remover) deleteBranches(ws domain.Workspace, force bool) {
	for _, repo := range ws.Repos {
		if !repo.CreatedBranch || !r.git.IsRepo(repo.SourceRepo) {
			continue
		}
		if err := r.git.DeleteBranch(repo.SourceRepo, repo.Branch, force); err != nil {
			r.warner.Warn(fmt.Sprintf("could not delete branch %s in %s: %v", repo.Branch, repo.RepoName, err))
		}
	}
}

// quarantine moves the workspace directory to .trash/<workspace>/<timestamp>.
func (r *Remover) quarantine(ws domain.Workspace) (string, error) {
	dir := r.trash.dirFor(ws.Name)
	if err := r.fs.MkdirAll(dir); err != nil {
		return "", err
	}
	dest, err := r.freeName(dir)
	if err != nil {
		return "", err
	}
	if err := r.fs.Rename(ws.Path, dest); err != nil {
		return "", fmt.Errorf("quarantining workspace: %w", err)
	}
	return dest, nil
}

// freeName picks a timestamped directory that does not exist yet, so deleting
// the same workspace twice within a second cannot clobber the first copy.
func (r *Remover) freeName(dir string) (string, error) {
	stamp := r.clock.Now().UTC().Format("20060102T150405Z")
	candidate := filepath.Join(dir, stamp)
	for i := 1; ; i++ {
		exists, err := r.fs.Exists(candidate)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
		candidate = filepath.Join(dir, fmt.Sprintf("%s-%d", stamp, i))
	}
}

// restore puts a quarantined workspace back and re-links its worktrees.
func (r *Remover) restore(ws domain.Workspace, trashPath string) {
	if trashPath == "" {
		return
	}
	_ = r.fs.Rename(trashPath, ws.Path)
	for _, repo := range ws.Repos {
		_ = r.git.WorktreeRepair(repo.SourceRepo, repo.WorktreePath)
	}
}

// RemoveRepos takes the named repositories out of a workspace, leaving the rest
// in place. A name may be a worktree repository or a shared one. Like Delete,
// each worktree is quarantined first so that a failure part-way through can put
// it back; a shared repository is just a link, which is dropped last.
func (r *Remover) RemoveRepos(name string, repoNames []string, opts DeleteOpts) (domain.Workspace, Members, error) {
	var (
		updated domain.Workspace
		removed Members
	)
	err := r.registry.WithLock(func() error {
		reg, err := r.registry.Load()
		if err != nil {
			return err
		}
		ws, ok := reg.Get(name)
		if !ok {
			return fmt.Errorf("workspace %q not found", name)
		}
		removed, err = selectWorkspaceRepos(ws, repoNames)
		if err != nil {
			return err
		}
		sub := domain.Workspace{Name: ws.Name, Path: ws.Path, Branch: ws.Branch, Repos: removed.Repos}

		if !opts.Force {
			if err := r.preflight(sub); err != nil {
				return err
			}
		}

		trashPath, moved, err := r.quarantineRepos(sub)
		if err != nil {
			return err
		}
		restore := func() { r.restoreRepos(sub, moved) }

		if err := r.pruneWorktrees(sub); err != nil {
			restore()
			return err
		}

		updated = ws.WithoutRepos(repoNamesOf(removed.Repos)).WithoutShared(sharedNamesOf(removed.Shared))
		reg.Upsert(updated)
		if err := r.registry.Save(reg); err != nil {
			restore()
			return err
		}

		if opts.DeleteBranch {
			r.deleteBranches(sub, opts.ForceBranch)
		}

		r.removeLinks(removed.Shared)

		if trashPath != "" {
			if err := r.trash.remove(trashPath); err != nil {
				r.warner.Warn("worktrees moved to " + trashPath + "; remove it manually")
			}
		}
		return nil
	})
	if err != nil {
		return domain.Workspace{}, Members{}, err
	}
	return updated, removed, nil
}

// selectWorkspaceRepos matches names against the repositories recorded in the
// workspace, not against discovery, so a repository that has since left
// repo_dirs can still be removed. A name is unique across worktree and shared
// repositories, so it matches at most one. The result keeps the workspace's
// order. Only worktree repositories count toward "cannot remove every
// repository": a workspace needs one for its branch to mean anything.
func selectWorkspaceRepos(ws domain.Workspace, names []string) (Members, error) {
	if len(names) == 0 {
		return Members{}, fmt.Errorf("no repositories selected; pass --repos")
	}
	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[n] = true
	}
	var selected Members
	for _, repo := range ws.Repos {
		if want[repo.RepoName] {
			selected.Repos = append(selected.Repos, repo)
			delete(want, repo.RepoName)
		}
	}
	for _, shared := range ws.Shared {
		if want[shared.RepoName] {
			selected.Shared = append(selected.Shared, shared)
			delete(want, shared.RepoName)
		}
	}
	if len(want) > 0 {
		var unknown []string
		for _, n := range names {
			if want[n] {
				unknown = append(unknown, n)
				delete(want, n)
			}
		}
		available := append(repoNamesOf(ws.Repos), sharedNamesOf(ws.Shared)...)
		return Members{}, fmt.Errorf("repositories not in workspace %s: %s (available: %s)",
			ws.Name, strings.Join(unknown, ", "), strings.Join(available, ", "))
	}
	if len(selected.Repos) == len(ws.Repos) {
		return Members{}, fmt.Errorf("cannot remove every repository from workspace %s; use: bosk delete %s", ws.Name, ws.Name)
	}
	return selected, nil
}

func sharedNamesOf(shared []domain.SharedRepo) []string {
	names := make([]string, len(shared))
	for i, s := range shared {
		names[i] = s.RepoName
	}
	return names
}

// removeLinks drops the symlinks of shared repositories. It deletes only a path
// that is still a symlink, never a real directory someone put there since, and
// never follows the link: the source repository stays untouched.
func (r *Remover) removeLinks(shared []domain.SharedRepo) {
	for _, s := range shared {
		if _, err := r.fs.Readlink(s.LinkPath); err != nil {
			if exists, _ := r.fs.LinkExists(s.LinkPath); exists {
				r.warner.Warn(s.RepoName + ": " + s.LinkPath + " is not a symlink; left in place")
			}
			continue
		}
		if err := r.fs.RemoveAll(s.LinkPath); err != nil {
			r.warner.Warn(fmt.Sprintf("could not remove link %s: %v", s.LinkPath, err))
		}
	}
}

func repoNamesOf(repos []domain.RepoWorktree) []string {
	names := make([]string, len(repos))
	for i, repo := range repos {
		names[i] = repo.RepoName
	}
	return names
}

// movedWorktree records where a quarantined worktree came from.
type movedWorktree struct{ from, to string }

// quarantineRepos moves each existing worktree to
// .trash/<workspace>/<timestamp>/<repo>. It returns the timestamped directory
// ("" when nothing needed moving) and undoes its own moves on failure.
func (r *Remover) quarantineRepos(ws domain.Workspace) (string, []movedWorktree, error) {
	dest := ""
	var moved []movedWorktree
	fail := func(err error) (string, []movedWorktree, error) {
		r.undoMoves(moved)
		return "", nil, err
	}
	for _, repo := range ws.Repos {
		exists, err := r.fs.Exists(repo.WorktreePath)
		if err != nil {
			return fail(err)
		}
		if !exists {
			continue
		}
		if dest == "" {
			dir := r.trash.dirFor(ws.Name)
			if err := r.fs.MkdirAll(dir); err != nil {
				return fail(err)
			}
			if dest, err = r.freeName(dir); err != nil {
				return fail(err)
			}
			if err := r.fs.MkdirAll(dest); err != nil {
				return fail(err)
			}
		}
		to := filepath.Join(dest, repo.RepoName)
		if err := r.fs.Rename(repo.WorktreePath, to); err != nil {
			return fail(fmt.Errorf("%s: quarantining worktree: %w", repo.RepoName, err))
		}
		moved = append(moved, movedWorktree{from: repo.WorktreePath, to: to})
	}
	return dest, moved, nil
}

func (r *Remover) undoMoves(moved []movedWorktree) {
	for _, m := range moved {
		_ = r.fs.Rename(m.to, m.from)
	}
}

// restoreRepos puts quarantined worktrees back and re-links them.
func (r *Remover) restoreRepos(ws domain.Workspace, moved []movedWorktree) {
	r.undoMoves(moved)
	for _, repo := range ws.Repos {
		_ = r.git.WorktreeRepair(repo.SourceRepo, repo.WorktreePath)
	}
}
