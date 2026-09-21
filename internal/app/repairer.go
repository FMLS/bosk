package app

import (
	"github.com/FMLS/bosk/internal/domain"
)

// Repairer applies the conservative repairs Doctor's findings allow. It
// dispatches on Issue.Kind, so wording changes can never alter what it deletes.
type Repairer struct {
	remover WorkspaceRemover
	trash   trashKeeper
}

// NewRepairer wires the repair use case to its collaborators.
func NewRepairer(cfg *domain.Config, remover WorkspaceRemover, fs FileSystem) *Repairer {
	return &Repairer{remover: remover, trash: newTrashKeeper(cfg, fs)}
}

// Repair fixes what it can and stops at the first repair that fails.
func (r *Repairer) Repair(issues []domain.Issue) error {
	dropped := make(map[string]bool)
	for _, issue := range issues {
		if !issue.Fixable() {
			continue
		}
		switch issue.Kind {
		case domain.IssueTrashOrphaned:
			if err := r.trash.remove(issue.Path); err != nil {
				return err
			}
		case domain.IssueWorkspaceDirMissing, domain.IssueWorktreePathMissing, domain.IssueWorktreeUnregistered:
			if issue.Workspace == "" || dropped[issue.Workspace] {
				continue
			}
			dropped[issue.Workspace] = true
			if err := r.remover.Delete(issue.Workspace, DeleteOpts{Force: true}); err != nil {
				return err
			}
		}
	}
	return nil
}
