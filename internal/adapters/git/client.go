package git

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/FMLS/bosk/internal/domain"
)

// PathNormalizer produces comparable absolute paths.
type PathNormalizer interface {
	Normalize(path string) string
}

// Client implements bosk's git ports on top of a CommandRunner.
type Client struct {
	runner CommandRunner
	paths  PathNormalizer
}

// NewClient wires a client to a runner and a path normalizer.
func NewClient(runner CommandRunner, paths PathNormalizer) *Client {
	return &Client{runner: runner, paths: paths}
}

// IsRepo reports whether path is a git working copy or bare repo.
func (c *Client) IsRepo(path string) bool {
	_, err := c.runner.Run(path, "rev-parse", "--git-dir")
	return err == nil
}

// Fetch updates all remotes. Repositories without remotes succeed.
func (c *Client) Fetch(repo string) error {
	_, err := c.runner.Run(repo, "fetch", "--all", "--quiet")
	return err
}

// DefaultBranch returns the repo's default branch name.
func (c *Client) DefaultBranch(repo string) (string, error) {
	out, err := c.runner.Run(repo, "symbolic-ref", "refs/remotes/origin/HEAD", "--short")
	if err == nil {
		parts := strings.SplitN(out, "/", 2)
		if len(parts) == 2 {
			return parts[1], nil
		}
		return out, nil
	}
	for _, branch := range []string{"main", "master"} {
		if _, err := c.runner.Run(repo, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+branch); err == nil {
			return branch, nil
		}
		if _, err := c.runner.Run(repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
			return branch, nil
		}
	}
	return "", &CommandError{Cmd: "default-branch", Output: "could not determine default branch"}
}

// BranchExists reports whether a local branch exists.
func (c *Client) BranchExists(repo, branch string) bool {
	_, err := c.runner.Run(repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

// ValidBranch reports whether name is a legal git branch.
func (c *Client) ValidBranch(name string) bool {
	_, err := c.runner.Run("", "check-ref-format", "--branch", name)
	return err == nil
}

// StartPoint chooses origin/<base> when present, otherwise the local ref.
func (c *Client) StartPoint(repo, base string) (string, error) {
	if base == "" {
		b, err := c.DefaultBranch(repo)
		if err != nil {
			return "", err
		}
		base = b
	}
	remote := "origin/" + base
	if _, err := c.runner.Run(repo, "rev-parse", "--verify", "--quiet", "refs/remotes/"+remote); err == nil {
		return remote, nil
	}
	if _, err := c.runner.Run(repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+base); err == nil {
		return base, nil
	}
	return "", fmt.Errorf("start point %q not found in %s", base, repo)
}

// DeleteBranch deletes a local branch. force uses -D.
func (c *Client) DeleteBranch(repo, branch string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err := c.runner.Run(repo, "branch", flag, branch)
	return err
}

// WorktreeAdd checks out an existing branch in a new worktree.
func (c *Client) WorktreeAdd(repo, path, branch string) error {
	_, err := c.runner.Run(repo, "worktree", "add", "--", path, branch)
	return err
}

// WorktreeAddNew creates a worktree and a new branch from startPoint.
func (c *Client) WorktreeAddNew(repo, path, branch, startPoint string) error {
	_, err := c.runner.Run(repo, "worktree", "add", "-b", branch, "--", path, startPoint)
	return err
}

// WorktreeRemove unregisters and deletes a worktree directory.
func (c *Client) WorktreeRemove(repo, path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, "--", path)
	_, err := c.runner.Run(repo, args...)
	return err
}

// WorktreePrune drops registrations whose directories are gone.
func (c *Client) WorktreePrune(repo string) error {
	_, err := c.runner.Run(repo, "worktree", "prune", "--expire=now")
	return err
}

// WorktreeRepair fixes worktree gitdir links after a move.
func (c *Client) WorktreeRepair(repo, path string) error {
	_, err := c.runner.Run(repo, "worktree", "repair", path)
	return err
}

// WorktreeList returns linked worktrees for a source repo.
func (c *Client) WorktreeList(repo string) ([]domain.Worktree, error) {
	out, err := c.runner.Run(repo, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var entries []domain.Worktree
	var current domain.Worktree
	flush := func() {
		if current.Path != "" {
			entries = append(entries, current)
		}
		current = domain.Worktree{}
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			flush()
			continue
		}
		switch {
		case strings.HasPrefix(line, "worktree "):
			current.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "branch refs/heads/"):
			current.Branch = strings.TrimPrefix(line, "branch refs/heads/")
		}
	}
	flush()
	return entries, nil
}

// BranchInUse reports whether any worktree has branch checked out.
func (c *Client) BranchInUse(repo, branch string) (bool, error) {
	entries, err := c.WorktreeList(repo)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if e.Branch == branch {
			return true, nil
		}
	}
	return false, nil
}

// WorktreeRegistered reports whether path is a registered worktree of repo.
func (c *Client) WorktreeRegistered(repo, path string) (bool, error) {
	entries, err := c.WorktreeList(repo)
	if err != nil {
		return false, err
	}
	want := c.paths.Normalize(path)
	for _, e := range entries {
		if c.paths.Normalize(e.Path) == want {
			return true, nil
		}
	}
	return false, nil
}

// CurrentBranch returns the checked-out branch, or empty when detached.
func (c *Client) CurrentBranch(path string) (string, error) {
	return c.runner.Run(path, "branch", "--show-current")
}

// DirtyCount returns the number of changed paths in the worktree.
func (c *Client) DirtyCount(path string) (int, error) {
	out, err := c.runner.Run(path, "status", "--porcelain")
	if err != nil {
		return 0, err
	}
	if out == "" {
		return 0, nil
	}
	return len(strings.Split(out, "\n")), nil
}

// AheadBehind returns commits ahead/behind upstream. A missing upstream is
// reported as 0, 0, nil.
func (c *Client) AheadBehind(path string) (ahead, behind int, err error) {
	out, err := c.runner.Run(path, "rev-list", "--left-right", "--count", "@{upstream}...HEAD")
	if err != nil {
		return 0, 0, nil
	}
	parts := strings.Fields(out)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("unexpected rev-list output: %s", out)
	}
	behind, err1 := strconv.Atoi(parts[0])
	ahead, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("parsing rev-list counts: %s", out)
	}
	return ahead, behind, nil
}
