package gitops

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// GitError is a failed git subprocess.
type GitError struct {
	Cmd    string
	Stderr string
}

func (e *GitError) Error() string {
	return fmt.Sprintf("git %s: %s", e.Cmd, e.Stderr)
}

var gitEnv []string
var devNull *os.File

func init() {
	env := append([]string{}, os.Environ()...)
	env = append(env, "GIT_TERMINAL_PROMPT=0")
	hasSSH := false
	for _, e := range env {
		if strings.HasPrefix(e, "GIT_SSH_COMMAND=") {
			hasSSH = true
			break
		}
	}
	if !hasSSH {
		env = append(env, "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
	}
	gitEnv = env
	devNull, _ = os.Open(os.DevNull)
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv
	if devNull != nil {
		cmd.Stdin = devNull
	}
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		return output, &GitError{Cmd: strings.Join(args, " "), Stderr: output}
	}
	return output, nil
}

// IsGitRepo reports whether path is a git working copy or bare repo.
func IsGitRepo(path string) bool {
	_, err := runGit(path, "rev-parse", "--git-dir")
	return err == nil
}

// Fetch updates all remotes. Repositories without remotes succeed.
func Fetch(repo string) error {
	_, err := runGit(repo, "fetch", "--all", "--quiet")
	return err
}

// DefaultBranch returns the repo's default branch name.
func DefaultBranch(repo string) (string, error) {
	out, err := runGit(repo, "symbolic-ref", "refs/remotes/origin/HEAD", "--short")
	if err == nil {
		parts := strings.SplitN(out, "/", 2)
		if len(parts) == 2 {
			return parts[1], nil
		}
		return out, nil
	}
	for _, branch := range []string{"main", "master"} {
		if _, err := runGit(repo, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+branch); err == nil {
			return branch, nil
		}
		if _, err := runGit(repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
			return branch, nil
		}
	}
	return "", &GitError{Cmd: "default-branch", Stderr: "could not determine default branch"}
}

// BranchExists reports whether a local branch exists.
func BranchExists(repo, branch string) bool {
	_, err := runGit(repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

// ValidBranch reports whether name is a legal git branch.
func ValidBranch(name string) bool {
	_, err := runGit("", "check-ref-format", "--branch", name)
	return err == nil
}

// StartPoint chooses origin/<base> when present, otherwise the local ref.
func StartPoint(repo, base string) (string, error) {
	if base == "" {
		b, err := DefaultBranch(repo)
		if err != nil {
			return "", err
		}
		base = b
	}
	remote := "origin/" + base
	if _, err := runGit(repo, "rev-parse", "--verify", "--quiet", "refs/remotes/"+remote); err == nil {
		return remote, nil
	}
	if _, err := runGit(repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+base); err == nil {
		return base, nil
	}
	return "", fmt.Errorf("start point %q not found in %s", base, repo)
}

// DeleteBranch deletes a local branch. force uses -D.
func DeleteBranch(repo, branch string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err := runGit(repo, "branch", flag, branch)
	return err
}

// WorktreeAdd checks out an existing branch in a new worktree.
func WorktreeAdd(repo, path, branch string) error {
	_, err := runGit(repo, "worktree", "add", "--", path, branch)
	return err
}

// WorktreeAddNew creates a worktree and a new branch from startPoint.
func WorktreeAddNew(repo, path, branch, startPoint string) error {
	_, err := runGit(repo, "worktree", "add", "-b", branch, "--", path, startPoint)
	return err
}

// WorktreeRemove unregisters and deletes a worktree directory.
func WorktreeRemove(repo, path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, "--", path)
	_, err := runGit(repo, args...)
	return err
}

// WorktreePrune drops registrations whose directories are gone.
func WorktreePrune(repo string) error {
	_, err := runGit(repo, "worktree", "prune", "--expire=now")
	return err
}

// WorktreeRepair fixes worktree gitdir links after a move.
func WorktreeRepair(repo, path string) error {
	_, err := runGit(repo, "worktree", "repair", path)
	return err
}

// Worktree is one `git worktree list` entry.
type Worktree struct {
	Path   string
	Branch string
}

// WorktreeList returns linked worktrees for a source repo.
func WorktreeList(repo string) ([]Worktree, error) {
	out, err := runGit(repo, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var entries []Worktree
	var current Worktree
	flush := func() {
		if current.Path != "" {
			entries = append(entries, current)
		}
		current = Worktree{}
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
func BranchInUse(repo, branch string) (bool, error) {
	entries, err := WorktreeList(repo)
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

// CurrentBranch returns the checked-out branch, or empty when detached.
func CurrentBranch(path string) (string, error) {
	return runGit(path, "branch", "--show-current")
}

// DirtyCount returns the number of changed paths in the worktree.
func DirtyCount(path string) (int, error) {
	out, err := runGit(path, "status", "--porcelain")
	if err != nil {
		return 0, err
	}
	if out == "" {
		return 0, nil
	}
	return len(strings.Split(out, "\n")), nil
}

// AheadBehind returns commits ahead/behind upstream. Missing upstream is 0,0,nil.
func AheadBehind(path string) (ahead, behind int, err error) {
	out, err := runGit(path, "rev-list", "--left-right", "--count", "@{upstream}...HEAD")
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

// WorktreeRegistered reports whether path is a registered worktree of repo.
func WorktreeRegistered(repo, path string) (bool, error) {
	entries, err := WorktreeList(repo)
	if err != nil {
		return false, err
	}
	want := Normalize(path)
	for _, e := range entries {
		if Normalize(e.Path) == want {
			return true, nil
		}
	}
	return false, nil
}

// Normalize returns a comparable absolute path.
func Normalize(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}
