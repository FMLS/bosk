package app_test

import (
	"errors"
	"testing"

	"github.com/FMLS/bosk/internal/app"
	"github.com/FMLS/bosk/internal/domain"
)

func findIssue(issues []domain.Issue, kind domain.IssueKind) (domain.Issue, bool) {
	for _, issue := range issues {
		if issue.Kind == kind {
			return issue, true
		}
	}
	return domain.Issue{}, false
}

func TestDoctorFindsNothingWrongWithAHealthyWorkspace(t *testing.T) {
	h := newHarness(t)
	h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a", "svc-b"}})

	issues, err := h.doctor().Diagnose()
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestDoctorReportsAMissingWorkspaceDirectory(t *testing.T) {
	h := newHarness(t)
	ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})
	if err := h.fs.RemoveAll(ws.Path); err != nil {
		t.Fatal(err)
	}

	issues, err := h.doctor().Diagnose()
	if err != nil {
		t.Fatal(err)
	}

	issue, ok := findIssue(issues, domain.IssueWorkspaceDirMissing)
	if !ok {
		t.Fatalf("issues = %#v", issues)
	}
	if issue.Workspace != ws.Name || issue.Path != ws.Path || !issue.Fixable() {
		t.Fatalf("issue = %#v", issue)
	}
	if _, ok := findIssue(issues, domain.IssueWorktreePathMissing); !ok {
		t.Error("the worktrees under it should be reported too")
	}
}

func TestDoctorReportsAMissingSourceRepoAsUnfixable(t *testing.T) {
	h := newHarness(t)
	h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})
	h.git.RemoveRepo("/src/svc-a")

	issues, err := h.doctor().Diagnose()
	if err != nil {
		t.Fatal(err)
	}

	issue, ok := findIssue(issues, domain.IssueSourceRepoMissing)
	if !ok {
		t.Fatalf("issues = %#v", issues)
	}
	if issue.Fixable() {
		t.Fatal("bosk must not offer to delete a workspace whose source clone vanished")
	}
	if issue.Path != "/src/svc-a" {
		t.Fatalf("issue = %#v", issue)
	}
}

func TestDoctorReportsAnUnregisteredWorktree(t *testing.T) {
	h := newHarness(t)
	ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})
	worktree := ws.Repos[0].WorktreePath
	if err := h.git.WorktreeRemove("/src/svc-a", worktree, true); err != nil {
		t.Fatal(err)
	}
	h.fs.AddDir(worktree) // the directory survives, only the registration is gone

	issues, err := h.doctor().Diagnose()
	if err != nil {
		t.Fatal(err)
	}

	issue, ok := findIssue(issues, domain.IssueWorktreeUnregistered)
	if !ok {
		t.Fatalf("issues = %#v", issues)
	}
	if issue.Repo != "svc-a" || !issue.Fixable() {
		t.Fatalf("issue = %#v", issue)
	}
}

func TestDoctorReportsAFailedProbe(t *testing.T) {
	h := newHarness(t)
	h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})
	h.git.RegisteredErr["/src/svc-a"] = errors.New("git blew up")

	issues, err := h.doctor().Diagnose()
	if err != nil {
		t.Fatal(err)
	}

	issue, ok := findIssue(issues, domain.IssueWorktreeProbeFailed)
	if !ok {
		t.Fatalf("issues = %#v", issues)
	}
	if issue.Message() != "git blew up" || issue.Fixable() {
		t.Fatalf("issue = %#v", issue)
	}
}

func TestDoctorReportsOrphanedTrash(t *testing.T) {
	h := newHarness(t)
	h.fs.AddDir(h.trashPath("gone", "20240102T030405Z"))

	issues, err := h.doctor().Diagnose()
	if err != nil {
		t.Fatal(err)
	}

	issue, ok := findIssue(issues, domain.IssueTrashOrphaned)
	if !ok {
		t.Fatalf("issues = %#v", issues)
	}
	if issue.Workspace != "gone" {
		t.Fatalf("issue = %#v", issue)
	}
	if issue.Path != h.trashPath("gone", "20240102T030405Z") {
		t.Fatalf("path = %s", issue.Path)
	}
	if !issue.Fixable() {
		t.Fatal("orphaned trash should be removable")
	}
}

func TestDoctorWillNotOfferToDeleteTrashOfALiveWorkspace(t *testing.T) {
	h := newHarness(t)
	h.mustCreate(t, app.CreateOpts{Name: "feat-login", Branch: "feat/login", RepoNames: []string{"svc-a"}})
	h.fs.AddDir(h.trashPath("feat-login", "20240102T030405Z"))

	issues, err := h.doctor().Diagnose()
	if err != nil {
		t.Fatal(err)
	}

	issue, ok := findIssue(issues, domain.IssueTrashShadowsWorkspace)
	if !ok {
		t.Fatalf("issues = %#v", issues)
	}
	if issue.Fixable() {
		t.Fatal("a quarantined copy of a live workspace must not be auto-deleted")
	}
}

func TestDoctorHandlesFlatTrashFromAnOlderBosk(t *testing.T) {
	h := newHarness(t)
	h.mustCreate(t, app.CreateOpts{Name: "feat-login", Branch: "feat/login", RepoNames: []string{"svc-a"}})
	// The old layout was ".trash/<workspace>-<timestamp>" with no nesting.
	h.fs.AddDir(h.trashPath("feat-login-20240101T000000Z"))
	h.fs.AddDir(h.trashPath("long-gone-20240101T000000Z"))

	issues, err := h.doctor().Diagnose()
	if err != nil {
		t.Fatal(err)
	}

	shadow, ok := findIssue(issues, domain.IssueTrashShadowsWorkspace)
	if !ok || shadow.Workspace != "feat-login" {
		t.Fatalf("expected the live workspace's old copy to be protected: %#v", issues)
	}
	orphan, ok := findIssue(issues, domain.IssueTrashOrphaned)
	if !ok || orphan.Workspace != "long-gone-20240101T000000Z" {
		t.Fatalf("expected the stale copy to be collectable: %#v", issues)
	}
}

func TestDoctorIgnoresAnAbsentTrashDirectory(t *testing.T) {
	h := newHarness(t)

	issues, err := h.doctor().Diagnose()
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestDoctorReportsAMissingCodeWorkspaceFile(t *testing.T) {
	h := newHarness(t)
	ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})
	if err := h.fs.RemoveAll(ws.CodeWorkspacePath()); err != nil {
		t.Fatal(err)
	}

	issues, err := h.doctor().Diagnose()
	if err != nil {
		t.Fatal(err)
	}

	issue, ok := findIssue(issues, domain.IssueCodeWorkspaceMissing)
	if !ok {
		t.Fatalf("issues = %#v", issues)
	}
	if !issue.Fixable() || issue.Path != ws.CodeWorkspacePath() {
		t.Fatalf("issue = %#v", issue)
	}
}

func TestDoctorReportsAnOutOfSyncCodeWorkspaceFile(t *testing.T) {
	h := newHarness(t)
	ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a", "svc-b"}})
	if err := h.fs.WriteFile(ws.CodeWorkspacePath(), []byte(`{"folders":[{"path":"svc-a"}]}`)); err != nil {
		t.Fatal(err)
	}

	issues, err := h.doctor().Diagnose()
	if err != nil {
		t.Fatal(err)
	}

	issue, ok := findIssue(issues, domain.IssueCodeWorkspaceOutOfSync)
	if !ok {
		t.Fatalf("issues = %#v", issues)
	}
	if !issue.Fixable() || issue.Path != ws.CodeWorkspacePath() {
		t.Fatalf("issue = %#v", issue)
	}
}
