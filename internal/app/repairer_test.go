package app_test

import (
	"strings"
	"testing"

	"github.com/FMLS/bosk/internal/app"
	"github.com/FMLS/bosk/internal/domain"
)

// recordingRemover stands in for the delete use case so repair dispatch can be
// observed without running a real deletion.
type recordingRemover struct {
	deleted []string
	err     error
}

func (r *recordingRemover) Delete(name string, opts app.DeleteOpts) error {
	if r.err != nil {
		return r.err
	}
	if !opts.Force {
		return errUnforced
	}
	r.deleted = append(r.deleted, name)
	return nil
}

var errUnforced = &repairError{"repair should force the delete"}

type repairError struct{ msg string }

func (e *repairError) Error() string { return e.msg }

func TestRepairDeletesEachBrokenWorkspaceOnce(t *testing.T) {
	h := newHarness(t)
	remover := &recordingRemover{}
	repairer := app.NewRepairer(h.cfg, remover, h.fs)

	err := repairer.Repair([]domain.Issue{
		{Kind: domain.IssueWorkspaceDirMissing, Workspace: "task-1", Path: "/ws/task-1"},
		{Kind: domain.IssueWorktreePathMissing, Workspace: "task-1", Repo: "svc-a"},
		{Kind: domain.IssueWorktreeUnregistered, Workspace: "task-1", Repo: "svc-b"},
		{Kind: domain.IssueWorktreePathMissing, Workspace: "task-2", Repo: "svc-a"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(remover.deleted) != 2 || remover.deleted[0] != "task-1" || remover.deleted[1] != "task-2" {
		t.Fatalf("deleted = %v, want each workspace exactly once", remover.deleted)
	}
}

func TestRepairSkipsIssuesItCannotFix(t *testing.T) {
	h := newHarness(t)
	remover := &recordingRemover{}
	repairer := app.NewRepairer(h.cfg, remover, h.fs)

	err := repairer.Repair([]domain.Issue{
		{Kind: domain.IssueSourceRepoMissing, Workspace: "task-1", Repo: "svc-a", Path: "/src/svc-a"},
		{Kind: domain.IssueWorktreeProbeFailed, Workspace: "task-2", Detail: "git broke"},
		{Kind: domain.IssueTrashShadowsWorkspace, Workspace: "task-3", Path: "/ws/.trash/task-3/t"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(remover.deleted) != 0 {
		t.Fatalf("deleted = %v, want nothing", remover.deleted)
	}
}

func TestRepairRemovesOrphanedTrashAndPrunesTheParent(t *testing.T) {
	h := newHarness(t)
	instance := h.trashPath("gone", "20240102T030405Z")
	h.fs.AddDir(instance)
	repairer := app.NewRepairer(h.cfg, &recordingRemover{}, h.fs)

	err := repairer.Repair([]domain.Issue{
		{Kind: domain.IssueTrashOrphaned, Workspace: "gone", Path: instance},
	})
	if err != nil {
		t.Fatal(err)
	}

	if exists(t, h.fs, instance) {
		t.Error("the quarantined copy should be gone")
	}
	if exists(t, h.fs, h.trashPath("gone")) {
		t.Error("the now-empty per-workspace directory should be pruned")
	}
}

func TestRepairKeepsTheParentWhenOtherCopiesRemain(t *testing.T) {
	h := newHarness(t)
	first := h.trashPath("gone", "20240102T030405Z")
	second := h.trashPath("gone", "20240103T030405Z")
	h.fs.AddDir(first)
	h.fs.AddDir(second)
	repairer := app.NewRepairer(h.cfg, &recordingRemover{}, h.fs)

	err := repairer.Repair([]domain.Issue{
		{Kind: domain.IssueTrashOrphaned, Workspace: "gone", Path: first},
	})
	if err != nil {
		t.Fatal(err)
	}

	if !exists(t, h.fs, second) {
		t.Error("the other quarantined copy should survive")
	}
	if !exists(t, h.fs, h.trashPath("gone")) {
		t.Error("the parent directory is still in use")
	}
}

func TestRepairRefusesAPathOutsideTheTrash(t *testing.T) {
	h := newHarness(t)
	h.fs.AddDir("/etc/important")
	repairer := app.NewRepairer(h.cfg, &recordingRemover{}, h.fs)

	err := repairer.Repair([]domain.Issue{
		{Kind: domain.IssueTrashOrphaned, Workspace: "x", Path: "/etc/important"},
	})

	if err == nil || !strings.Contains(err.Error(), "refusing to delete path outside trash") {
		t.Fatalf("error = %v", err)
	}
	if !exists(t, h.fs, "/etc/important") {
		t.Fatal("the directory must be untouched")
	}
}

func TestRepairRefusesATraversalOutOfTheTrash(t *testing.T) {
	h := newHarness(t)
	repairer := app.NewRepairer(h.cfg, &recordingRemover{}, h.fs)

	err := repairer.Repair([]domain.Issue{
		{Kind: domain.IssueTrashOrphaned, Workspace: "x", Path: h.trashPath("..", "..", "etc")},
	})

	if err == nil || !strings.Contains(err.Error(), "refusing to delete path outside trash") {
		t.Fatalf("error = %v", err)
	}
}

func TestRepairAppliesRealFixesEndToEnd(t *testing.T) {
	h := newHarness(t)
	ws := h.mustCreate(t, app.CreateOpts{Name: "task-1", Branch: "feat/x", RepoNames: []string{"svc-a"}})
	if err := h.fs.RemoveAll(ws.Path); err != nil {
		t.Fatal(err)
	}

	issues, err := h.doctor().Diagnose()
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) == 0 {
		t.Fatal("expected doctor to find something")
	}
	if err := h.repairer().Repair(issues); err != nil {
		t.Fatal(err)
	}

	remaining, err := h.doctor().Diagnose()
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("after repair: %#v", remaining)
	}
	if _, ok := h.reg.Registry.Get("task-1"); ok {
		t.Error("the broken workspace should be out of the registry")
	}
}

func TestRepairRebuildsMissingAndOutOfSyncCodeWorkspaceFiles(t *testing.T) {
	h := newHarness(t)
	ws1 := h.mustCreate(t, app.CreateOpts{Name: "task-1", Branch: "feat/a", RepoNames: []string{"svc-a"}})
	ws2 := h.mustCreate(t, app.CreateOpts{Name: "task-2", Branch: "feat/b", RepoNames: []string{"svc-a", "svc-b"}})
	if err := h.fs.RemoveAll(ws1.CodeWorkspacePath()); err != nil {
		t.Fatal(err)
	}
	if err := h.fs.WriteFile(ws2.CodeWorkspacePath(), []byte(`{"folders":[],"settings":{"go.useLanguageServer":true}}`)); err != nil {
		t.Fatal(err)
	}

	issues, err := h.doctor().Diagnose()
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 2 {
		t.Fatalf("issues = %#v", issues)
	}
	if err := h.repairer().Repair(issues); err != nil {
		t.Fatal(err)
	}

	remaining, err := h.doctor().Diagnose()
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("after repair: %#v", remaining)
	}
	data2, err := h.fs.ReadFile(ws2.CodeWorkspacePath())
	if err != nil || !ws2.ValidCodeWorkspace(data2) || !strings.Contains(string(data2), `"go.useLanguageServer": true`) {
		t.Fatalf("repaired task-2 code-workspace = %q, %v", data2, err)
	}
}

func TestRepairDoesNotWriteCodeWorkspaceIntoADeletedWorkspace(t *testing.T) {
	h := newHarness(t)
	remover := &recordingRemover{}
	repairer := app.NewRepairer(h.cfg, remover, h.fs)

	err := repairer.Repair([]domain.Issue{
		{Kind: domain.IssueWorktreePathMissing, Workspace: "task-1", Repo: "svc-a"},
		{
			Kind:      domain.IssueCodeWorkspaceMissing,
			Workspace: "task-1",
			Path:      "/ws/task-1/task-1.code-workspace",
			Folders:   []domain.CodeWorkspaceFolder{{Path: "svc-a"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if exists(t, h.fs, "/ws/task-1/task-1.code-workspace") {
		t.Error("no code-workspace file should be created inside a workspace that is being deleted")
	}
}
