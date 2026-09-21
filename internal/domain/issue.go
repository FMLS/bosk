package domain

// IssueKind classifies a health-check finding. Repairs dispatch on this value
// so that changing a message can never change what gets deleted.
type IssueKind string

const (
	IssueWorkspaceDirMissing   IssueKind = "workspace_dir_missing"
	IssueSourceRepoMissing     IssueKind = "source_repo_missing"
	IssueWorktreePathMissing   IssueKind = "worktree_path_missing"
	IssueWorktreeUnregistered  IssueKind = "worktree_unregistered"
	IssueWorktreeProbeFailed   IssueKind = "worktree_probe_failed"
	IssueTrashShadowsWorkspace IssueKind = "trash_shadows_workspace"
	IssueTrashOrphaned         IssueKind = "trash_orphaned"
)

// Issue is one health-check finding. Path carries the subject path in
// structured form; Detail carries an underlying error message.
type Issue struct {
	Kind      IssueKind
	Workspace string
	Repo      string
	Path      string
	Detail    string
}

// Fixable reports whether Repairer knows a conservative repair for this issue.
func (i Issue) Fixable() bool {
	switch i.Kind {
	case IssueWorkspaceDirMissing, IssueWorktreePathMissing, IssueWorktreeUnregistered, IssueTrashOrphaned:
		return true
	default:
		return false
	}
}

// Message renders the finding for humans.
func (i Issue) Message() string {
	switch i.Kind {
	case IssueWorkspaceDirMissing:
		return "workspace directory missing: " + i.Path
	case IssueSourceRepoMissing:
		return "source repository missing: " + i.Path
	case IssueWorktreePathMissing:
		return "worktree path missing"
	case IssueWorktreeUnregistered:
		return "worktree is not registered in git"
	case IssueWorktreeProbeFailed:
		return i.Detail
	case IssueTrashShadowsWorkspace:
		return "quarantined copy left in " + i.Path
	case IssueTrashOrphaned:
		return "leftover trash: " + i.Path
	default:
		return i.Detail
	}
}

// SuggestedAction renders the recommended next step, empty when there is none.
func (i Issue) SuggestedAction() string {
	switch i.Kind {
	case IssueWorkspaceDirMissing, IssueWorktreePathMissing, IssueWorktreeUnregistered:
		return "bosk delete --force " + i.Workspace
	case IssueSourceRepoMissing:
		return "restore the source clone or bosk delete --force " + i.Workspace
	case IssueTrashShadowsWorkspace:
		return "inspect the directory; restore or bosk doctor --fix"
	case IssueTrashOrphaned:
		return "bosk doctor --fix"
	default:
		return ""
	}
}
