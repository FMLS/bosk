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
}

func newRollbackLog(git CreatorGit, fs FileSystem, root string) *rollbackLog {
	return &rollbackLog{git: git, fs: fs, root: root}
}

func (l *rollbackLog) record(item createdWorktree) {
	l.items = append(l.items, item)
}

// undo removes the recorded worktrees in reverse order, drops any branch this
// attempt created, and deletes the workspace directory. Errors are ignored:
// the caller is already returning the failure that triggered the rollback.
func (l *rollbackLog) undo() {
	for i := len(l.items) - 1; i >= 0; i-- {
		item := l.items[i]
		_ = l.git.WorktreeRemove(item.source, item.path, true)
		if item.createdBranch {
			_ = l.git.DeleteBranch(item.source, item.branch, true)
		}
	}
	_ = l.fs.RemoveAll(l.root)
}
