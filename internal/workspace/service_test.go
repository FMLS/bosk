package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/FMLS/bosk/internal/config"
	"github.com/FMLS/bosk/internal/gitops"
	"github.com/FMLS/bosk/internal/models"
	"github.com/FMLS/bosk/internal/testgit"
)

func setup(t *testing.T) (*Service, string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("BOSK_HOME", home)
	src := t.TempDir()
	a := filepath.Join(src, "svc-a")
	b := filepath.Join(src, "svc-b")
	testgit.Init(t, a)
	testgit.Init(t, b)
	cfg, err := config.Init([]string{src})
	if err != nil {
		t.Fatal(err)
	}
	return New(cfg), a, b
}

func TestCreateStatusDelete(t *testing.T) {
	svc, a, _ := setup(t)
	ws, err := svc.Create(CreateOpts{
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
	for _, repo := range ws.Repos {
		if _, err := os.Stat(repo.WorktreePath); err != nil {
			t.Fatal(err)
		}
		cur, err := gitops.CurrentBranch(repo.WorktreePath)
		if err != nil {
			t.Fatal(err)
		}
		if cur != "feat/login" {
			t.Fatalf("%s branch = %s", repo.RepoName, cur)
		}
	}

	listed, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatalf("list = %d", len(listed))
	}
	st := svc.Status(ws)
	if len(st) != 2 || st[0].Dirty != 0 {
		t.Fatalf("status = %#v", st)
	}

	if err := os.WriteFile(filepath.Join(ws.Repos[0].WorktreePath, "README"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ws.Name, DeleteOpts{}); err == nil {
		t.Fatal("expected dirty delete to fail")
	}
	if err := svc.Delete(ws.Name, DeleteOpts{Force: true, DeleteBranch: true, ForceBranch: true}); err != nil {
		t.Fatal(err)
	}
	if gitops.BranchExists(a, "feat/login") {
		t.Fatal("created branch should be deleted")
	}
	if _, err := svc.Get(ws.Name); err == nil {
		t.Fatal("workspace should be gone")
	}
}

func TestCreateRollbackOnConflict(t *testing.T) {
	svc, a, b := setup(t)
	if err := gitops.WorktreeAddNew(b, filepath.Join(t.TempDir(), "other"), "feat/login", "main"); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Create(CreateOpts{
		Branch:    "feat/login",
		RepoNames: []string{"svc-a", "svc-b"},
	})
	if err == nil {
		t.Fatal("expected conflict")
	}
	listed, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("rolled back state still has %#v", listed)
	}
	if gitops.BranchExists(a, "feat/login") {
		t.Fatal("rolled-back branch should not remain in svc-a")
	}
}

func TestDoctorMissingDirectory(t *testing.T) {
	svc, _, _ := setup(t)
	ws, err := svc.Create(CreateOpts{Branch: "feat/x", RepoNames: []string{"svc-a"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(ws.Path); err != nil {
		t.Fatal(err)
	}
	issues, err := svc.Doctor()
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) == 0 {
		t.Fatal("expected doctor issues")
	}
	if err := svc.Fix(issues); err != nil {
		t.Fatal(err)
	}
	issues, err = svc.Doctor()
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("after fix: %#v", issues)
	}
}

func TestResolveFromPath(t *testing.T) {
	svc, _, _ := setup(t)
	ws, err := svc.Create(CreateOpts{Name: "task-1", Branch: "feat/task-1", RepoNames: []string{"svc-a"}})
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
	got, err := svc.Resolve("")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "task-1" {
		t.Fatalf("resolved %s", got.Name)
	}
}

func TestCreateRequiresSelection(t *testing.T) {
	svc, _, _ := setup(t)
	if _, err := svc.Create(CreateOpts{Branch: "feat/x"}); err == nil {
		t.Fatal("expected error")
	}
	ws, err := svc.Create(CreateOpts{Branch: "feat/all", All: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(ws.Repos) != 2 {
		t.Fatalf("all repos = %d", len(ws.Repos))
	}
}

func TestModelsFindRepo(t *testing.T) {
	ws := models.NewWorkspace("n", "/tmp/n", "b")
	ws.Repos = []models.RepoWorktree{{RepoName: "svc-a"}}
	if ws.FindRepo("svc-a") == nil || ws.FindRepo("nope") != nil {
		t.Fatal("FindRepo")
	}
}
