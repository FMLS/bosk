package app_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/FMLS/bosk/internal/adapters/clock"
	"github.com/FMLS/bosk/internal/adapters/configstore"
	"github.com/FMLS/bosk/internal/adapters/discovery"
	"github.com/FMLS/bosk/internal/adapters/fsys"
	"github.com/FMLS/bosk/internal/adapters/git"
	"github.com/FMLS/bosk/internal/adapters/pathx"
	"github.com/FMLS/bosk/internal/adapters/registry"
	"github.com/FMLS/bosk/internal/app"
	"github.com/FMLS/bosk/internal/domain"
	"github.com/FMLS/bosk/internal/testsupport"
)

// realHarness wires the use cases to the production adapters against real git
// repositories in a temporary directory. These tests are the guard that the
// port interfaces did not change behaviour.
type realHarness struct {
	cfg    *domain.Config
	git    *git.Client
	srcA   string
	srcB   string
	warner *testsupport.RecordingWarner

	creator   *app.Creator
	remover   *app.Remover
	inspector *app.Inspector
	doctor    *app.Doctor
	repairer  *app.Repairer
}

func newRealHarness(t *testing.T) *realHarness {
	t.Helper()

	home := t.TempDir()
	src := t.TempDir()
	srcA := filepath.Join(src, "svc-a")
	srcB := filepath.Join(src, "svc-b")
	testsupport.InitRepo(t, srcA)
	testsupport.InitRepo(t, srcB)

	paths := pathx.New()
	files := fsys.New()
	configStore := configstore.NewStore(home)
	gitClient := git.NewClient(git.NewExecRunner(), paths)
	scanner := discovery.NewScanner(gitClient, files, paths)
	warner := &testsupport.RecordingWarner{}

	res, err := app.NewInitializer(configStore, scanner, files, paths).Init([]string{src})
	if err != nil {
		t.Fatal(err)
	}
	cfg := res.Config
	reg := registry.NewStore(configStore.StatePath(), configStore.LockPath())
	systemClock := clock.New()
	remover := app.NewRemover(cfg, gitClient, reg, files, systemClock, warner)

	return &realHarness{
		cfg:       cfg,
		git:       gitClient,
		srcA:      srcA,
		srcB:      srcB,
		warner:    warner,
		creator:   app.NewCreator(cfg, gitClient, reg, scanner, files, systemClock, warner),
		remover:   remover,
		inspector: app.NewInspector(cfg, gitClient, reg, scanner, files, paths),
		doctor:    app.NewDoctor(cfg, gitClient, reg, files),
		repairer:  app.NewRepairer(cfg, remover, files),
	}
}

func TestIntegrationCreateStatusDelete(t *testing.T) {
	h := newRealHarness(t)

	ws, err := h.creator.Create(app.CreateOpts{
		Branch:    "feat/login",
		RepoNames: []string{"svc-a", "svc-b"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ws.Name != "feat-login" {
		t.Fatalf("name = %s", ws.Name)
	}
	if len(ws.Repos) != 2 {
		t.Fatalf("repos = %d", len(ws.Repos))
	}
	cwData, err := os.ReadFile(ws.CodeWorkspacePath())
	if err != nil || !ws.ValidCodeWorkspace(cwData) {
		t.Fatalf("code-workspace = %q, %v", cwData, err)
	}
	for _, repo := range ws.Repos {
		if _, err := os.Stat(repo.WorktreePath); err != nil {
			t.Fatal(err)
		}
		cur, err := h.git.CurrentBranch(repo.WorktreePath)
		if err != nil {
			t.Fatal(err)
		}
		if cur != "feat/login" {
			t.Fatalf("%s branch = %s", repo.RepoName, cur)
		}
	}

	listed, err := h.inspector.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatalf("list = %d", len(listed))
	}
	statuses := h.inspector.Status(ws)
	if len(statuses) != 2 || statuses[0].Dirty != 0 {
		t.Fatalf("status = %#v", statuses)
	}

	if err := os.WriteFile(filepath.Join(ws.Repos[0].WorktreePath, "README"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := h.remover.Delete(ws.Name, app.DeleteOpts{}); err == nil {
		t.Fatal("expected a dirty worktree to block delete")
	}
	if err := h.remover.Delete(ws.Name, app.DeleteOpts{Force: true, DeleteBranch: true, ForceBranch: true}); err != nil {
		t.Fatal(err)
	}
	if h.git.BranchExists(h.srcA, "feat/login") {
		t.Fatal("the branch bosk created should be deleted")
	}
	if _, err := h.inspector.Get(ws.Name); err == nil {
		t.Fatal("workspace should be gone")
	}
	if _, err := os.Stat(ws.Path); !os.IsNotExist(err) {
		t.Fatalf("workspace directory should be gone, stat err = %v", err)
	}
}

func TestIntegrationCreateRollsBackOnConflict(t *testing.T) {
	h := newRealHarness(t)
	if err := h.git.WorktreeAddNew(h.srcB, filepath.Join(t.TempDir(), "other"), "feat/login", "main"); err != nil {
		t.Fatal(err)
	}

	_, err := h.creator.Create(app.CreateOpts{
		Branch:    "feat/login",
		RepoNames: []string{"svc-a", "svc-b"},
	})
	if err == nil {
		t.Fatal("expected a conflict")
	}

	listed, err := h.inspector.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("rolled back state still has %#v", listed)
	}
	if h.git.BranchExists(h.srcA, "feat/login") {
		t.Fatal("the rolled-back branch should not remain in svc-a")
	}
	if _, err := os.Stat(filepath.Join(h.cfg.WorkspaceDir, "feat-login")); !os.IsNotExist(err) {
		t.Fatalf("workspace directory should be gone, stat err = %v", err)
	}
}

func TestIntegrationDoctorFindsAndFixesAMissingDirectory(t *testing.T) {
	h := newRealHarness(t)
	ws, err := h.creator.Create(app.CreateOpts{Branch: "feat/x", RepoNames: []string{"svc-a"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(ws.Path); err != nil {
		t.Fatal(err)
	}

	issues, err := h.doctor.Diagnose()
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) == 0 {
		t.Fatal("expected doctor issues")
	}
	if err := h.repairer.Repair(issues); err != nil {
		t.Fatal(err)
	}

	issues, err = h.doctor.Diagnose()
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("after fix: %#v", issues)
	}
}

func TestIntegrationResolveFromTheWorkingDirectory(t *testing.T) {
	h := newRealHarness(t)
	ws, err := h.creator.Create(app.CreateOpts{Name: "task-1", Branch: "feat/task-1", RepoNames: []string{"svc-a"}})
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(ws.Repos[0].WorktreePath); err != nil {
		t.Fatal(err)
	}

	got, err := h.inspector.Resolve("")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "task-1" {
		t.Fatalf("resolved %s", got.Name)
	}
}

func TestIntegrationCreateRequiresASelection(t *testing.T) {
	h := newRealHarness(t)

	if _, err := h.creator.Create(app.CreateOpts{Branch: "feat/x"}); err == nil {
		t.Fatal("expected an error")
	}

	ws, err := h.creator.Create(app.CreateOpts{Branch: "feat/all", All: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(ws.Repos) != 2 {
		t.Fatalf("all repos = %d", len(ws.Repos))
	}
}

func TestIntegrationDeleteLeavesNoTrashBehind(t *testing.T) {
	h := newRealHarness(t)
	ws, err := h.creator.Create(app.CreateOpts{Name: "task-1", Branch: "feat/t", RepoNames: []string{"svc-a"}})
	if err != nil {
		t.Fatal(err)
	}

	if err := h.remover.Delete(ws.Name, app.DeleteOpts{}); err != nil {
		t.Fatal(err)
	}

	trash := filepath.Join(h.cfg.WorkspaceDir, ".trash")
	entries, err := os.ReadDir(trash)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("trash should be empty, holds %v", entries)
	}

	issues, err := h.doctor.Diagnose()
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("a clean delete should leave nothing for doctor: %#v", issues)
	}
}

func TestIntegrationWorktreeUnregisteredIsReportedAndFixable(t *testing.T) {
	h := newRealHarness(t)
	ws, err := h.creator.Create(app.CreateOpts{Name: "task-1", Branch: "feat/t", RepoNames: []string{"svc-a"}})
	if err != nil {
		t.Fatal(err)
	}
	// Detach the worktree from git without removing the directory.
	testsupport.RunGit(t, h.srcA, "worktree", "remove", "--force", ws.Repos[0].WorktreePath)
	if err := os.MkdirAll(ws.Repos[0].WorktreePath, 0o755); err != nil {
		t.Fatal(err)
	}

	issues, err := h.doctor.Diagnose()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, issue := range issues {
		if issue.Kind == domain.IssueWorktreeUnregistered {
			found = true
		}
	}
	if !found {
		t.Fatalf("issues = %#v", issues)
	}

	if err := h.repairer.Repair(issues); err != nil {
		t.Fatal(err)
	}
	if _, err := h.inspector.Get("task-1"); err == nil {
		t.Fatal("the repaired workspace should be out of the registry")
	}
}

func TestIntegrationAddExtendsAWorkspace(t *testing.T) {
	h := newRealHarness(t)
	ws, err := h.creator.Create(app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})
	if err != nil {
		t.Fatal(err)
	}

	ws, added, err := h.creator.Add(app.AddOpts{Name: ws.Name, RepoNames: []string{"svc-b"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(added.Repos) != 1 || len(ws.Repos) != 2 {
		t.Fatalf("added = %#v, repos = %#v", added, ws.Repos)
	}
	cur, err := h.git.CurrentBranch(added.Repos[0].WorktreePath)
	if err != nil {
		t.Fatal(err)
	}
	if cur != "feat/login" {
		t.Fatalf("svc-b branch = %s", cur)
	}
	statuses := h.inspector.Status(ws)
	if len(statuses) != 2 || statuses[1].Repo != "svc-b" || statuses[1].Error != "" {
		t.Fatalf("status = %#v", statuses)
	}

	if err := h.remover.Delete(ws.Name, app.DeleteOpts{DeleteBranch: true}); err != nil {
		t.Fatal(err)
	}
	if h.git.BranchExists(h.srcB, "feat/login") {
		t.Fatal("delete should remove the branch add created")
	}
	if _, err := os.Stat(ws.Path); !os.IsNotExist(err) {
		t.Fatalf("workspace directory should be gone, stat err = %v", err)
	}
}

func TestIntegrationRemoveReposDropsOneWorktree(t *testing.T) {
	h := newRealHarness(t)
	ws, err := h.creator.Create(app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a", "svc-b"}})
	if err != nil {
		t.Fatal(err)
	}
	gone := ws.Repos[1].WorktreePath

	ws, removed, err := h.remover.RemoveRepos(ws.Name, []string{"svc-b"}, app.DeleteOpts{DeleteBranch: true})
	if err != nil {
		t.Fatal(err)
	}

	if len(removed.Repos) != 1 || len(ws.Repos) != 1 || ws.Repos[0].RepoName != "svc-a" {
		t.Fatalf("removed = %#v, repos = %#v", removed, ws.Repos)
	}
	if _, err := os.Stat(gone); !os.IsNotExist(err) {
		t.Fatalf("worktree directory should be gone, stat err = %v", err)
	}
	if h.git.BranchExists(h.srcB, "feat/login") {
		t.Error("--delete-branch should remove the branch bosk created")
	}
	if !h.git.BranchExists(h.srcA, "feat/login") {
		t.Error("the repo that stays keeps its branch")
	}
	if statuses := h.inspector.Status(ws); len(statuses) != 1 || statuses[0].Error != "" {
		t.Fatalf("status = %#v", statuses)
	}
	issues, err := h.doctor.Diagnose()
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("doctor issues = %#v", issues)
	}
}
