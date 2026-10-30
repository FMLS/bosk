package app

// createdWorktree is one worktree a create attempt brought into existence.
type createdWorktree struct {
	source        string
	path          string
	branch        string
	createdBranch bool
}

// rollbackLog records what a create attempt has built so a later failure can
// undo exactly that much, and nothing a previous attempt left behind.
type rollbackLog struct {
	git   CreatorGit
	fs    FileSystem
	root  string
	items []createdWorktree
	links []string
}

func newRollbackLog(git CreatorGit, fs FileSystem, root string) *rollbackLog {
	return &rollbackLog{git: git, fs: fs, root: root}
}

func (l *rollbackLog) record(item createdWorktree) {
	l.items = append(l.items, item)
}

// recordLink notes a shared repository link this attempt created.
func (l *rollbackLog) recordLink(path string) {
	l.links = append(l.links, path)
}

// undo removes the recorded links and then the recorded worktrees in reverse
// order, drops any branch this attempt created, and deletes the workspace
// directory unless root is empty. An add attempt leaves root empty because the
// workspace predates it. Errors are ignored: the caller is already returning
// the failure that triggered the rollback.
func (l *rollbackLog) undo() {
	// RemoveAll on a symbolic link removes the link and leaves its target, the
	// shared source repository, alone.
	for i := len(l.links) - 1; i >= 0; i-- {
		_ = l.fs.RemoveAll(l.links[i])
	}
	for i := len(l.items) - 1; i >= 0; i-- {
		item := l.items[i]
		_ = l.git.WorktreeRemove(item.source, item.path, true)
		if item.createdBranch {
			_ = l.git.DeleteBranch(item.source, item.branch, true)
		}
	}
	if l.root != "" {
		_ = l.fs.RemoveAll(l.root)
	}
}
