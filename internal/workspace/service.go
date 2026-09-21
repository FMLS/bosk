package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/FMLS/bosk/internal/discover"
	"github.com/FMLS/bosk/internal/gitops"
	"github.com/FMLS/bosk/internal/models"
	"github.com/FMLS/bosk/internal/state"
)

// Service orchestrates workspace lifecycle.
type Service struct {
	cfg   *models.Config
	store *state.Store
	Warn  func(string)
}

// New constructs a service for the given config.
func New(cfg *models.Config) *Service {
	return &Service{cfg: cfg, store: state.New()}
}

func (s *Service) warn(msg string) {
	if s.Warn != nil {
		s.Warn(msg)
	}
}

// List returns recorded workspaces.
func (s *Service) List() ([]models.Workspace, error) {
	file, err := s.store.Load()
	if err != nil {
		return nil, err
	}
	return file.Workspaces, nil
}

// Get returns one workspace by name.
func (s *Service) Get(name string) (models.Workspace, error) {
	file, err := s.store.Load()
	if err != nil {
		return models.Workspace{}, err
	}
	ws, ok := file.Get(name)
	if !ok {
		return models.Workspace{}, fmt.Errorf("workspace %q not found", name)
	}
	return ws, nil
}

// Resolve returns a named workspace, or the workspace containing cwd.
func (s *Service) Resolve(name string) (models.Workspace, error) {
	if name != "" {
		return s.Get(name)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return models.Workspace{}, err
	}
	file, err := s.store.Load()
	if err != nil {
		return models.Workspace{}, err
	}
	ws, ok := file.ByPath(cwd)
	if !ok {
		return models.Workspace{}, fmt.Errorf("not inside a bosk workspace; pass a name")
	}
	return ws, nil
}

// Discovered lists source repositories.
func (s *Service) Discovered() ([]models.Repo, []string) {
	return discover.Repos(s.cfg.RepoDirs)
}

// CreateOpts is input for workspace creation.
type CreateOpts struct {
	Name      string
	Branch    string
	RepoNames []string
	All       bool
}

type createdItem struct {
	source        string
	path          string
	branch        string
	createdBranch bool
}

// Create provisions worktrees and records the workspace.
func (s *Service) Create(opts CreateOpts) (models.Workspace, error) {
	branch := strings.TrimSpace(opts.Branch)
	name := strings.TrimSpace(opts.Name)
	if branch == "" && name == "" {
		return models.Workspace{}, fmt.Errorf("workspace name or --branch is required")
	}
	if branch == "" {
		branch = name
	}
	if name == "" {
		name = nameFromBranch(branch)
	}
	if err := validateWorkspaceName(name); err != nil {
		return models.Workspace{}, err
	}
	if !gitops.ValidBranch(branch) {
		return models.Workspace{}, fmt.Errorf("invalid branch name %q", branch)
	}
	if opts.All && len(opts.RepoNames) > 0 {
		return models.Workspace{}, fmt.Errorf("use either --repos or --all, not both")
	}

	found, skipped := s.Discovered()
	for _, p := range skipped {
		s.warn("skipped duplicate repo name: " + p)
	}
	if len(found) == 0 {
		return models.Workspace{}, fmt.Errorf("no git repositories found under repo_dirs")
	}

	var selected []models.Repo
	var err error
	switch {
	case opts.All:
		selected = found
	case len(opts.RepoNames) > 0:
		selected, err = discover.ByName(found, opts.RepoNames)
		if err != nil {
			return models.Workspace{}, err
		}
	default:
		return models.Workspace{}, fmt.Errorf("select repositories with --repos or --all")
	}
	if len(selected) == 0 {
		return models.Workspace{}, fmt.Errorf("no repositories selected")
	}

	wsPath := filepath.Join(s.cfg.WorkspaceDir, name)
	var result models.Workspace
	err = s.store.WithLock(func() error {
		file, err := s.store.Load()
		if err != nil {
			return err
		}
		if _, exists := file.Get(name); exists {
			return fmt.Errorf("workspace %q already exists", name)
		}
		if info, err := os.Stat(wsPath); err == nil && info.IsDir() {
			return fmt.Errorf("workspace directory already exists: %s", wsPath)
		}

		if err := os.MkdirAll(wsPath, 0o755); err != nil {
			return err
		}

		s.fetchAll(selected)

		var created []createdItem
		rollback := func() {
			for i := len(created) - 1; i >= 0; i-- {
				item := created[i]
				_ = gitops.WorktreeRemove(item.source, item.path, true)
				if item.createdBranch {
					_ = gitops.DeleteBranch(item.source, item.branch, true)
				}
			}
			_ = os.RemoveAll(wsPath)
		}

		ws := models.NewWorkspace(name, wsPath, branch)
		for _, repo := range selected {
			inUse, err := gitops.BranchInUse(repo.Path, branch)
			if err != nil {
				rollback()
				return fmt.Errorf("%s: %w", repo.Name, err)
			}
			if inUse {
				rollback()
				return fmt.Errorf("%s: branch %q is already checked out in another worktree", repo.Name, branch)
			}
			wtPath := filepath.Join(wsPath, repo.Name)
			createdBranch := false
			var addErr error
			if gitops.BranchExists(repo.Path, branch) {
				addErr = gitops.WorktreeAdd(repo.Path, wtPath, branch)
			} else {
				start, err := gitops.StartPoint(repo.Path, "")
				if err != nil {
					rollback()
					return fmt.Errorf("%s: %w", repo.Name, err)
				}
				addErr = gitops.WorktreeAddNew(repo.Path, wtPath, branch, start)
				createdBranch = true
			}
			if addErr != nil {
				rollback()
				return fmt.Errorf("%s: %w", repo.Name, addErr)
			}
			created = append(created, createdItem{
				source:        repo.Path,
				path:          wtPath,
				branch:        branch,
				createdBranch: createdBranch,
			})
			ws.Repos = append(ws.Repos, models.RepoWorktree{
				RepoName:      repo.Name,
				SourceRepo:    repo.Path,
				WorktreePath:  wtPath,
				Branch:        branch,
				CreatedBranch: createdBranch,
			})
		}

		file.Upsert(ws)
		if err := s.store.Save(file); err != nil {
			rollback()
			return err
		}
		result = ws
		return nil
	})
	return result, err
}

func (s *Service) fetchAll(repos []models.Repo) {
	var wg sync.WaitGroup
	for _, repo := range repos {
		wg.Add(1)
		go func(name, path string) {
			defer wg.Done()
			if err := gitops.Fetch(path); err != nil {
				s.warn(fmt.Sprintf("fetch %s failed: %v", name, err))
			}
		}(repo.Name, repo.Path)
	}
	wg.Wait()
}

func nameFromBranch(branch string) string {
	r := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-")
	return r.Replace(branch)
}

func validateWorkspaceName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid workspace name %q", name)
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("workspace name must not contain path separators")
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("invalid workspace name %q", name)
	}
	return nil
}

// DeleteOpts controls workspace removal.
type DeleteOpts struct {
	Force        bool
	DeleteBranch bool
	ForceBranch  bool
}

const trashDirName = ".trash"

// Delete removes a workspace's worktrees and registry entry.
func (s *Service) Delete(name string, opts DeleteOpts) error {
	return s.store.WithLock(func() error {
		file, err := s.store.Load()
		if err != nil {
			return err
		}
		ws, ok := file.Get(name)
		if !ok {
			return fmt.Errorf("workspace %q not found", name)
		}

		if !opts.Force {
			if err := s.preflight(ws); err != nil {
				return err
			}
		}

		trashPath := ""
		if info, err := os.Stat(ws.Path); err == nil && info.IsDir() {
			trashPath, err = s.quarantine(ws)
			if err != nil {
				return err
			}
		}

		restore := func() {
			if trashPath == "" {
				return
			}
			_ = os.Rename(trashPath, ws.Path)
			for _, repo := range ws.Repos {
				_ = gitops.WorktreeRepair(repo.SourceRepo, repo.WorktreePath)
			}
		}

		var pruneErr error
		for _, repo := range ws.Repos {
			if !gitops.IsGitRepo(repo.SourceRepo) {
				continue
			}
			if err := gitops.WorktreePrune(repo.SourceRepo); err != nil {
				pruneErr = fmt.Errorf("%s: %w", repo.RepoName, err)
				break
			}
		}
		if pruneErr != nil {
			restore()
			return pruneErr
		}

		file.Remove(name)
		if err := s.store.Save(file); err != nil {
			restore()
			return err
		}

		if opts.DeleteBranch {
			for _, repo := range ws.Repos {
				if !repo.CreatedBranch || !gitops.IsGitRepo(repo.SourceRepo) {
					continue
				}
				if err := gitops.DeleteBranch(repo.SourceRepo, repo.Branch, opts.ForceBranch); err != nil {
					s.warn(fmt.Sprintf("could not delete branch %s in %s: %v", repo.Branch, repo.RepoName, err))
				}
			}
		}

		if trashPath != "" {
			if err := os.RemoveAll(trashPath); err != nil {
				s.warn("workspace moved to " + trashPath + "; remove it manually")
			}
		}
		return nil
	})
}

func (s *Service) preflight(ws models.Workspace) error {
	for _, repo := range ws.Repos {
		if _, err := os.Stat(repo.WorktreePath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if !gitops.IsGitRepo(repo.SourceRepo) {
			return fmt.Errorf("%s: source repository missing: %s", repo.RepoName, repo.SourceRepo)
		}
		ok, err := gitops.WorktreeRegistered(repo.SourceRepo, repo.WorktreePath)
		if err != nil {
			return fmt.Errorf("%s: %w", repo.RepoName, err)
		}
		if !ok {
			return fmt.Errorf("%s: worktree is not registered in git; use --force", repo.RepoName)
		}
		cur, err := gitops.CurrentBranch(repo.WorktreePath)
		if err != nil {
			return fmt.Errorf("%s: %w", repo.RepoName, err)
		}
		if cur != repo.Branch {
			return fmt.Errorf("%s: checked out %q, expected %q; use --force", repo.RepoName, cur, repo.Branch)
		}
		dirty, err := gitops.DirtyCount(repo.WorktreePath)
		if err != nil {
			return fmt.Errorf("%s: %w", repo.RepoName, err)
		}
		if dirty > 0 {
			return fmt.Errorf("%s: has uncommitted changes; use --force", repo.RepoName)
		}
	}
	return nil
}

func (s *Service) quarantine(ws models.Workspace) (string, error) {
	trashRoot := filepath.Join(s.cfg.WorkspaceDir, trashDirName)
	if err := os.MkdirAll(trashRoot, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(trashRoot, ws.Name)
	if _, err := os.Stat(dest); err == nil {
		dest = dest + "-" + strings.ReplaceAll(ws.CreatedAt, ":", "")
	}
	if err := os.Rename(ws.Path, dest); err != nil {
		return "", fmt.Errorf("quarantining workspace: %w", err)
	}
	return dest, nil
}

// Status reports live git status for each worktree.
func (s *Service) Status(ws models.Workspace) []models.RepoStatus {
	out := make([]models.RepoStatus, 0, len(ws.Repos))
	for _, repo := range ws.Repos {
		st := models.RepoStatus{
			Repo:     repo.RepoName,
			Expected: repo.Branch,
		}
		if _, err := os.Stat(repo.WorktreePath); err != nil {
			st.Error = "worktree missing"
			out = append(out, st)
			continue
		}
		branch, err := gitops.CurrentBranch(repo.WorktreePath)
		if err != nil {
			st.Error = err.Error()
			out = append(out, st)
			continue
		}
		if branch == "" {
			branch = "(detached)"
		}
		st.Branch = branch
		dirty, err := gitops.DirtyCount(repo.WorktreePath)
		if err != nil {
			st.Error = err.Error()
			out = append(out, st)
			continue
		}
		st.Dirty = dirty
		ahead, behind, err := gitops.AheadBehind(repo.WorktreePath)
		if err != nil {
			st.Error = err.Error()
			out = append(out, st)
			continue
		}
		st.Ahead = ahead
		st.Behind = behind
		out = append(out, st)
	}
	return out
}

// Doctor inspects recorded workspaces against git and the filesystem.
func (s *Service) Doctor() ([]models.DoctorIssue, error) {
	file, err := s.store.Load()
	if err != nil {
		return nil, err
	}
	var issues []models.DoctorIssue
	for _, ws := range file.Workspaces {
		if _, err := os.Stat(ws.Path); err != nil {
			issues = append(issues, models.DoctorIssue{
				Workspace: ws.Name,
				Issue:     "workspace directory missing: " + ws.Path,
				Action:    "bosk delete --force " + ws.Name,
				Fixable:   true,
			})
		}
		for _, repo := range ws.Repos {
			if !gitops.IsGitRepo(repo.SourceRepo) {
				issues = append(issues, models.DoctorIssue{
					Workspace: ws.Name,
					Repo:      repo.RepoName,
					Issue:     "source repository missing: " + repo.SourceRepo,
					Action:    "restore the source clone or bosk delete --force " + ws.Name,
				})
				continue
			}
			if _, err := os.Stat(repo.WorktreePath); err != nil {
				issues = append(issues, models.DoctorIssue{
					Workspace: ws.Name,
					Repo:      repo.RepoName,
					Issue:     "worktree path missing",
					Action:    "bosk delete --force " + ws.Name,
					Fixable:   true,
				})
				continue
			}
			ok, err := gitops.WorktreeRegistered(repo.SourceRepo, repo.WorktreePath)
			if err != nil {
				issues = append(issues, models.DoctorIssue{
					Workspace: ws.Name,
					Repo:      repo.RepoName,
					Issue:     err.Error(),
				})
				continue
			}
			if !ok {
				issues = append(issues, models.DoctorIssue{
					Workspace: ws.Name,
					Repo:      repo.RepoName,
					Issue:     "worktree is not registered in git",
					Action:    "bosk delete --force " + ws.Name,
					Fixable:   true,
				})
			}
		}
	}
	issues = append(issues, s.trashIssues(file)...)
	return issues, nil
}

func (s *Service) trashIssues(file *state.File) []models.DoctorIssue {
	trashRoot := filepath.Join(s.cfg.WorkspaceDir, trashDirName)
	entries, err := os.ReadDir(trashRoot)
	if err != nil {
		return nil
	}
	live := make(map[string]bool)
	for _, w := range file.Workspaces {
		live[w.Name] = true
	}
	var issues []models.DoctorIssue
	for _, e := range entries {
		name := e.Name()
		base := name
		if i := strings.Index(name, "-20"); i > 0 {
			base = name[:i]
		}
		if live[base] {
			issues = append(issues, models.DoctorIssue{
				Workspace: base,
				Issue:     "quarantined copy left in " + filepath.Join(trashRoot, name),
				Action:    "inspect the directory; restore or bosk doctor --fix",
			})
			continue
		}
		issues = append(issues, models.DoctorIssue{
			Workspace: name,
			Issue:     "leftover trash: " + filepath.Join(trashRoot, name),
			Action:    "bosk doctor --fix",
			Fixable:   true,
		})
	}
	return issues
}

// Fix applies conservative repairs for doctor findings.
func (s *Service) Fix(issues []models.DoctorIssue) error {
	seen := make(map[string]bool)
	for _, issue := range issues {
		if !issue.Fixable {
			continue
		}
		if strings.HasPrefix(issue.Issue, "leftover trash:") {
			path := strings.TrimPrefix(issue.Issue, "leftover trash: ")
			if err := removeTrashPath(s.cfg.WorkspaceDir, path); err != nil {
				return err
			}
			continue
		}
		if issue.Workspace == "" || seen[issue.Workspace] {
			continue
		}
		if strings.Contains(issue.Issue, "missing") || strings.Contains(issue.Issue, "not registered") {
			seen[issue.Workspace] = true
			if err := s.Delete(issue.Workspace, DeleteOpts{Force: true}); err != nil {
				return err
			}
		}
	}
	return nil
}

func removeTrashPath(workspaceDir, path string) error {
	trashRoot := filepath.Join(workspaceDir, trashDirName)
	clean := filepath.Clean(path)
	if clean != trashRoot && !strings.HasPrefix(clean, trashRoot+string(os.PathSeparator)) {
		return fmt.Errorf("refusing to delete path outside trash: %s", path)
	}
	return os.RemoveAll(clean)
}
