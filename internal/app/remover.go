package app

import (
	"fmt"
	"path/filepath"

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
