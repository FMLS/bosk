package app

import (
	"path/filepath"
	"strings"

	"github.com/FMLS/bosk/internal/domain"
)

// Doctor compares the registry against git and the filesystem. It only reads;
// applying repairs is Repairer's job.
type Doctor struct {
	cfg      *domain.Config
	git      DoctorGit
	registry Registry
	fs       FileSystem
}

// NewDoctor wires the health check to its collaborators.
func NewDoctor(cfg *domain.Config, git DoctorGit, registry Registry, fs FileSystem) *Doctor {
	return &Doctor{cfg: cfg, git: git, registry: registry, fs: fs}
}

// Diagnose reports every finding across recorded workspaces and the trash.
func (d *Doctor) Diagnose() ([]domain.Issue, error) {
	reg, err := d.registry.Load()
	if err != nil {
		return nil, err
	}
	var issues []domain.Issue
	for _, ws := range reg.Workspaces {
		issues = append(issues, d.workspaceIssues(ws)...)
	}
	return append(issues, d.trashIssues(reg)...), nil
}

func (d *Doctor) workspaceIssues(ws domain.Workspace) []domain.Issue {
	var issues []domain.Issue
	if exists, err := d.fs.Exists(ws.Path); err != nil || !exists {
		issues = append(issues, domain.Issue{
			Kind:      domain.IssueWorkspaceDirMissing,
			Workspace: ws.Name,
			Path:      ws.Path,
		})
	}
	for _, repo := range ws.Repos {
		if issue, ok := d.repoIssue(ws, repo); ok {
			issues = append(issues, issue)
		}
	}
	return issues
}

func (d *Doctor) repoIssue(ws domain.Workspace, repo domain.RepoWorktree) (domain.Issue, bool) {
	base := domain.Issue{Workspace: ws.Name, Repo: repo.RepoName}

	if !d.git.IsRepo(repo.SourceRepo) {
		base.Kind = domain.IssueSourceRepoMissing
		base.Path = repo.SourceRepo
		return base, true
	}
	if exists, err := d.fs.Exists(repo.WorktreePath); err != nil || !exists {
		base.Kind = domain.IssueWorktreePathMissing
		base.Path = repo.WorktreePath
		return base, true
	}
	registered, err := d.git.WorktreeRegistered(repo.SourceRepo, repo.WorktreePath)
	if err != nil {
		base.Kind = domain.IssueWorktreeProbeFailed
		base.Path = repo.WorktreePath
		base.Detail = err.Error()
		return base, true
	}
	if !registered {
		base.Kind = domain.IssueWorktreeUnregistered
		base.Path = repo.WorktreePath
		return base, true
	}
	return domain.Issue{}, false
}

// trashIssues walks .trash/<workspace>/<timestamp>. Entries belonging to a live
// workspace are reported but not offered for automatic removal.
func (d *Doctor) trashIssues(reg *domain.Registry) []domain.Issue {
	root := trashRoot(d.cfg)
	names, err := d.fs.ListDirs(root)
	if err != nil {
		return nil
	}
	live := make(map[string]bool, len(reg.Workspaces))
	for _, w := range reg.Workspaces {
		live[w.Name] = true
	}

	var issues []domain.Issue
	for _, name := range names {
		dir := filepath.Join(root, name)
		instances, err := d.fs.ListDirs(dir)
		if err != nil || len(instances) == 0 {
			// A directory written by an older bosk, which quarantined to a
			// flat ".trash/<workspace>-<timestamp>".
			workspace, isLive := matchLegacyTrash(live, name)
			issues = append(issues, trashIssue(workspace, dir, isLive))
			continue
		}
		for _, instance := range instances {
			issues = append(issues, trashIssue(name, filepath.Join(dir, instance), live[name]))
		}
	}
	return issues
}

func trashIssue(workspace, path string, live bool) domain.Issue {
	kind := domain.IssueTrashOrphaned
	if live {
		kind = domain.IssueTrashShadowsWorkspace
	}
	return domain.Issue{Kind: kind, Workspace: workspace, Path: path}
}

// matchLegacyTrash maps a flat trash directory name back to a live workspace,
// preferring the longest match. Ambiguity resolves toward "live", because
// reporting a stale copy is cheaper than deleting one that is still wanted.
func matchLegacyTrash(live map[string]bool, name string) (string, bool) {
	if live[name] {
		return name, true
	}
	best := ""
	for workspace := range live {
		if strings.HasPrefix(name, workspace+"-") && len(workspace) > len(best) {
			best = workspace
		}
	}
	if best != "" {
		return best, true
	}
	return name, false
}
