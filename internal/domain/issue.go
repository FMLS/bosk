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
	IssueSharedSourceMissing   IssueKind = "shared_source_missing"
	IssueSharedLinkMissing      IssueKind = "shared_link_missing"
	IssueSharedLinkMismatch     IssueKind = "shared_link_mismatch"
	IssueSharedLinkNotSymlink   IssueKind = "shared_link_not_symlink"
	IssueCodeWorkspaceMissing   IssueKind = "code_workspace_missing"
	IssueCodeWorkspaceOutOfSync IssueKind = "code_workspace_out_of_sync"
	IssueTrashShadowsWorkspace  IssueKind = "trash_shadows_workspace"
	IssueTrashOrphaned          IssueKind = "trash_orphaned"
)

// Issue is one health-check finding. Path carries the subject path in
// structured form; Detail carries an underlying error message. Target is what a
// shared repository link should point at, set only for link findings. Folders
// carries the expected .code-workspace folder list for code-workspace findings.
type Issue struct {
	Kind      IssueKind
	Workspace string
	Repo      string
	Path      string
	Target    string
	Folders   []CodeWorkspaceFolder
	Detail    string
}

// Fixable reports whether Repairer knows a conservative repair for this issue.
func (i Issue) Fixable() bool {
	switch i.Kind {
	case IssueWorkspaceDirMissing, IssueWorktreePathMissing, IssueWorktreeUnregistered, IssueTrashOrphaned,
		IssueSharedLinkMissing, IssueSharedLinkMismatch,
		IssueCodeWorkspaceMissing, IssueCodeWorkspaceOutOfSync:
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
	case IssueSourceRepoMissing, IssueSharedSourceMissing:
		return "source repository missing: " + i.Path
	case IssueWorktreePathMissing:
		return "worktree path missing"
	case IssueWorktreeUnregistered:
		return "worktree is not registered in git"
	case IssueWorktreeProbeFailed:
		return i.Detail
	case IssueSharedLinkMissing:
		return "shared repository link missing: " + i.Path
	case IssueSharedLinkMismatch:
		return "shared repository link points elsewhere: " + i.Path
	case IssueSharedLinkNotSymlink:
		return "shared repository path is not a symlink: " + i.Path
	case IssueCodeWorkspaceMissing:
		return "code-workspace file missing: " + i.Path
	case IssueCodeWorkspaceOutOfSync:
		return "code-workspace file out of sync: " + i.Path
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
	case IssueSourceRepoMissing, IssueSharedSourceMissing:
		return "restore the source clone or bosk delete --force " + i.Workspace
	case IssueSharedLinkMissing, IssueSharedLinkMismatch,
		IssueCodeWorkspaceMissing, IssueCodeWorkspaceOutOfSync:
		return "bosk doctor --fix"
	case IssueSharedLinkNotSymlink:
		return "move or delete that path, then bosk doctor"
	case IssueTrashShadowsWorkspace:
		return "inspect the directory; restore or bosk doctor --fix"
	case IssueTrashOrphaned:
		return "bosk doctor --fix"
	default:
		return ""
	}
}
