package app_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/FMLS/bosk/internal/app"
	"github.com/FMLS/bosk/internal/domain"
)

// withShared adds a repository that workspaces link in rather than check out.
func withShared(h *harness) {
	h.fs.AddDir("/src/lib")
	h.git.AddRepo("/src/lib")
	h.disc.Repos = append(h.disc.Repos, domain.Repo{Name: "lib", Path: "/src/lib"})
}

func createWithShared(t *testing.T, h *harness) domain.Workspace {
	t.Helper()
	return h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}, SharedNames: []string{"lib"}})
}

func readlink(t *testing.T, h *harness, link string) string {
	t.Helper()
	target, err := h.fs.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink(%s) = %v", link, err)
	}
	return target
}

func TestCreateLinksSharedRepositories(t *testing.T) {
	h := newHarness(t)
	withShared(h)

	ws := createWithShared(t, h)

	if len(ws.Repos) != 1 || ws.Repos[0].RepoName != "svc-a" {
		t.Fatalf("repos = %#v", ws.Repos)
	}
	want := domain.SharedRepo{RepoName: "lib", SourceRepo: "/src/lib", LinkPath: "/ws/feat-login/lib"}
	if len(ws.Shared) != 1 || ws.Shared[0] != want {
		t.Fatalf("shared = %#v, want %#v", ws.Shared, want)
	}
	if got := readlink(t, h, want.LinkPath); got != "/src/lib" {
		t.Errorf("link target = %s", got)
	}
	stored, _ := h.reg.Registry.Get("feat-login")
	if len(stored.Shared) != 1 {
		t.Errorf("registry shared = %#v", stored.Shared)
	}
	if got := h.git.Worktrees("/src/lib"); len(got) != 0 {
		t.Errorf("a shared repository must not get a worktree, got %v", got)
	}
	if h.recorded("fetch /src/lib") {
		t.Error("a shared repository must not be fetched")
	}
	for _, branch := range h.git.Branches("/src/lib") {
		if branch == "feat/login" {
			t.Error("a shared repository must not get the workspace branch")
		}
	}
	data, err := h.fs.ReadFile(ws.CodeWorkspacePath())
	if err != nil || !ws.ValidCodeWorkspace(data) {
		t.Fatalf("code-workspace should include both worktree and shared repos: %q, %v", data, err)
	}
}

func TestCreateWithOnlySharedRepositoriesFails(t *testing.T) {
	h := newHarness(t)
	withShared(h)

	_, err := h.creator().Create(app.CreateOpts{Branch: "feat/login", SharedNames: []string{"lib"}})

	if err == nil || !strings.Contains(err.Error(), "at least one worktree repository") {
		t.Fatalf("error = %v", err)
	}
	if exists(t, h.fs, "/ws/feat-login") {
		t.Error("nothing should have been created")
	}
}

func TestCreateRejectsARepoThatIsBothWorktreeAndShared(t *testing.T) {
	h := newHarness(t)

	_, err := h.creator().Create(app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}, SharedNames: []string{"svc-a"}})

	if err == nil || !strings.Contains(err.Error(), "both a worktree repository and a shared repository") {
		t.Fatalf("error = %v", err)
	}
}

func TestCreateRejectsAnUnknownSharedRepository(t *testing.T) {
	h := newHarness(t)

	_, err := h.creator().Create(app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}, SharedNames: []string{"nope"}})

	var missing *domain.MissingReposError
	if !errors.As(err, &missing) || missing.Names[0] != "nope" {
		t.Fatalf("error = %v", err)
	}
}

func TestCreateAllLeavesSharedRepositoriesOutOfTheWorktrees(t *testing.T) {
	h := newHarness(t)

	ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", All: true, SharedNames: []string{"svc-b"}})

	if len(ws.Repos) != 1 || ws.Repos[0].RepoName != "svc-a" {
		t.Fatalf("repos = %#v", ws.Repos)
	}
	if len(ws.Shared) != 1 || ws.Shared[0].RepoName != "svc-b" {
		t.Fatalf("shared = %#v", ws.Shared)
	}
}

func TestCreateRollsBackWhenALinkFails(t *testing.T) {
	h := newHarness(t)
	withShared(h)
	h.fs.LinkErr["/ws/feat-login/lib"] = errors.New("symlink denied")

	_, err := h.creator().Create(app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}, SharedNames: []string{"lib"}})

	if err == nil || !strings.Contains(err.Error(), "lib: symlink denied") {
		t.Fatalf("error = %v", err)
	}
	if got := h.git.Worktrees("/src/svc-a"); len(got) != 0 {
		t.Errorf("worktrees = %v, want them rolled back", got)
	}
	if exists(t, h.fs, "/ws/feat-login") || len(h.reg.Registry.Workspaces) != 0 {
		t.Error("nothing should be left behind")
	}
	if !exists(t, h.fs, "/src/lib") {
		t.Error("the shared source repository must be untouched")
	}
}

func TestCreateRollsBackLinksWhenTheRegistrySaveFails(t *testing.T) {
	h := newHarness(t)
	withShared(h)
	h.reg.SaveErr = errors.New("disk full")

	_, err := h.creator().Create(app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}, SharedNames: []string{"lib"}})

	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("error = %v", err)
	}
	if ok, _ := h.fs.LinkExists("/ws/feat-login/lib"); ok {
		t.Error("the link should have been rolled back")
	}
	if !exists(t, h.fs, "/src/lib") {
		t.Error("the shared source repository must be untouched")
	}
}

func TestAddLinksASharedRepositoryToAnExistingWorkspace(t *testing.T) {
	h := newHarness(t)
	withShared(h)
	h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})

	ws, added, err := h.creator().Add(app.AddOpts{Name: "feat-login", SharedNames: []string{"lib"}})
	if err != nil {
		t.Fatal(err)
	}

	if len(added.Repos) != 0 || len(added.Shared) != 1 || added.Shared[0].RepoName != "lib" {
		t.Fatalf("added = %#v", added)
	}
	if len(ws.Shared) != 1 || len(ws.Repos) != 1 {
		t.Fatalf("workspace = %#v", ws)
	}
	if got := readlink(t, h, "/ws/feat-login/lib"); got != "/src/lib" {
		t.Errorf("link target = %s", got)
	}
	stored, _ := h.reg.Registry.Get("feat-login")
	if len(stored.Shared) != 1 {
		t.Errorf("registry shared = %#v", stored.Shared)
	}
}

func TestAddAllSkipsRepositoriesAlreadyLinked(t *testing.T) {
	h := newHarness(t)
	h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}, SharedNames: []string{"svc-b"}})

	_, _, err := h.creator().Add(app.AddOpts{Name: "feat-login", All: true})

	if err == nil || !strings.Contains(err.Error(), "no repositories to add") {
		t.Fatalf("error = %v, want svc-b (linked) and svc-a (checked out) both skipped", err)
	}
}

func TestAddRequiresSomethingToAdd(t *testing.T) {
	h := newHarness(t)
	h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})

	_, _, err := h.creator().Add(app.AddOpts{Name: "feat-login"})

	if err == nil || !strings.Contains(err.Error(), "--repos, --all or --shared") {
		t.Fatalf("error = %v", err)
	}
}

func TestAddRejectsASharedRepositoryTheWorkspaceAlreadyHolds(t *testing.T) {
	h := newHarness(t)
	withShared(h)
	createWithShared(t, h)

	for _, opts := range []app.AddOpts{
		{Name: "feat-login", SharedNames: []string{"lib"}},
		{Name: "feat-login", SharedNames: []string{"svc-a"}},
		{Name: "feat-login", RepoNames: []string{"lib"}},
	} {
		_, _, err := h.creator().Add(opts)
		if err == nil || !strings.Contains(err.Error(), "already in workspace") {
			t.Errorf("Add(%+v) error = %v", opts, err)
		}
	}
}

func TestAddRejectsADanglingLinkAtTheTargetPath(t *testing.T) {
	h := newHarness(t)
	withShared(h)
	h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})
	h.fs.AddLink("/gone", "/ws/feat-login/lib")

	_, _, err := h.creator().Add(app.AddOpts{Name: "feat-login", SharedNames: []string{"lib"}})

	if err == nil || !strings.Contains(err.Error(), "path already exists") {
		t.Fatalf("error = %v", err)
	}
}

func TestAddRollsBackWorktreesAndLinksOnFailure(t *testing.T) {
	h := newHarness(t)
	withShared(h)
	h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})
	h.reg.SaveErr = errors.New("disk full")

	_, _, err := h.creator().Add(app.AddOpts{Name: "feat-login", RepoNames: []string{"svc-b"}, SharedNames: []string{"lib"}})

	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("error = %v", err)
	}
	if ok, _ := h.fs.LinkExists("/ws/feat-login/lib"); ok {
		t.Error("the link should have been rolled back")
	}
	if got := h.git.Worktrees("/src/svc-b"); len(got) != 0 {
		t.Errorf("svc-b worktrees = %v", got)
	}
	if !exists(t, h.fs, "/ws/feat-login/svc-a") || !exists(t, h.fs, "/src/lib") {
		t.Error("earlier members and the source repository must stay")
	}
}

func TestAddRollsBackWhenALinkFails(t *testing.T) {
	h := newHarness(t)
	withShared(h)
	h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})
	h.fs.LinkErr["/ws/feat-login/lib"] = errors.New("symlink denied")

	_, _, err := h.creator().Add(app.AddOpts{Name: "feat-login", RepoNames: []string{"svc-b"}, SharedNames: []string{"lib"}})

	if err == nil || !strings.Contains(err.Error(), "lib: symlink denied") {
		t.Fatalf("error = %v", err)
	}
	if got := h.git.Worktrees("/src/svc-b"); len(got) != 0 {
		t.Errorf("svc-b worktrees = %v", got)
	}
	if !exists(t, h.fs, "/ws/feat-login") {
		t.Error("an add must keep the workspace directory")
	}
}

// --- remove and delete --------------------------------------------------

func TestRemoveReposDropsASharedLinkAndLeavesTheSource(t *testing.T) {
	h := newHarness(t)
	withShared(h)
	ws := createWithShared(t, h)
	h.fs.AddFile("/src/lib/README")

	updated, removed, err := h.remover().RemoveRepos(ws.Name, []string{"lib"}, app.DeleteOpts{})
	if err != nil {
		t.Fatal(err)
	}

	if len(removed.Shared) != 1 || len(removed.Repos) != 0 {
		t.Fatalf("removed = %#v", removed)
	}
	if len(updated.Shared) != 0 || len(updated.Repos) != 1 {
		t.Fatalf("updated = %#v", updated)
	}
	stored, _ := h.reg.Registry.Get(ws.Name)
	if len(stored.Shared) != 0 {
		t.Errorf("registry shared = %#v", stored.Shared)
	}
	if ok, _ := h.fs.LinkExists("/ws/feat-login/lib"); ok {
		t.Error("the link should be gone")
	}
	if !exists(t, h.fs, "/src/lib/README") {
		t.Error("the shared source repository must be untouched")
	}
	if !exists(t, h.fs, "/ws/feat-login/svc-a") {
		t.Error("the worktree must be left alone")
	}
	if h.recorded("worktree-prune /src/lib") {
		t.Error("a shared repository has no worktree to prune")
	}
}

func TestRemoveReposTakesWorktreesAndSharedTogether(t *testing.T) {
	h := newHarness(t)
	withShared(h)
	ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a", "svc-b"}, SharedNames: []string{"lib"}})

	updated, removed, err := h.remover().RemoveRepos(ws.Name, []string{"svc-b", "lib"}, app.DeleteOpts{})
	if err != nil {
		t.Fatal(err)
	}

	if len(removed.Repos) != 1 || len(removed.Shared) != 1 {
		t.Fatalf("removed = %#v", removed)
	}
	if len(updated.Repos) != 1 || len(updated.Shared) != 0 {
		t.Fatalf("updated = %#v", updated)
	}
}

func TestRemoveReposCannotEmptyTheWorktreeRepositories(t *testing.T) {
	h := newHarness(t)
	withShared(h)
	ws := createWithShared(t, h)

	_, _, err := h.remover().RemoveRepos(ws.Name, []string{"svc-a", "lib"}, app.DeleteOpts{})

	if err == nil || !strings.Contains(err.Error(), "cannot remove every repository") {
		t.Fatalf("error = %v", err)
	}
	if ok, _ := h.fs.LinkExists("/ws/feat-login/lib"); !ok {
		t.Error("a refused remove must not touch the link")
	}
}

func TestRemoveReposListsSharedRepositoriesAsAvailable(t *testing.T) {
	h := newHarness(t)
	withShared(h)
	ws := createWithShared(t, h)

	_, _, err := h.remover().RemoveRepos(ws.Name, []string{"nope"}, app.DeleteOpts{})

	if err == nil || !strings.Contains(err.Error(), "available: svc-a, lib") {
		t.Fatalf("error = %v", err)
	}
}

func TestRemoveReposLeavesAPathThatIsNoLongerASymlink(t *testing.T) {
	h := newHarness(t)
	withShared(h)
	ws := createWithShared(t, h)
	// Someone replaced the link with a real directory holding their own work.
	if err := h.fs.RemoveAll("/ws/feat-login/lib"); err != nil {
		t.Fatal(err)
	}
	h.fs.AddFile("/ws/feat-login/lib/notes.txt")

	if _, _, err := h.remover().RemoveRepos(ws.Name, []string{"lib"}, app.DeleteOpts{}); err != nil {
		t.Fatal(err)
	}

	if !exists(t, h.fs, "/ws/feat-login/lib/notes.txt") {
		t.Error("bosk must not delete a real directory it did not create")
	}
	if !h.warner.Contains("not a symlink") {
		t.Errorf("warnings = %v", h.warner.Messages)
	}
	stored, _ := h.reg.Registry.Get(ws.Name)
	if len(stored.Shared) != 0 {
		t.Errorf("the record should still be dropped, got %#v", stored.Shared)
	}
}

func TestDeleteLeavesTheSharedSourceRepositoryIntact(t *testing.T) {
	h := newHarness(t)
	withShared(h)
	ws := createWithShared(t, h)
	h.fs.AddFile("/src/lib/README")

	if err := h.remover().Delete(ws.Name, app.DeleteOpts{}); err != nil {
		t.Fatal(err)
	}

	if !exists(t, h.fs, "/src/lib/README") {
		t.Fatal("deleting a workspace must never delete a shared source repository")
	}
	if exists(t, h.fs, ws.Path) || len(h.reg.Registry.Workspaces) != 0 {
		t.Error("the workspace should be gone")
	}
	if h.recorded("worktree-prune /src/lib") {
		t.Error("a shared repository has no worktree to prune")
	}
	if exists(t, h.fs, h.trashPath("feat-login")) {
		t.Error("the quarantined copy, link included, should have been removed")
	}
}

func TestDeleteIgnoresAMissingSharedSource(t *testing.T) {
	h := newHarness(t)
	withShared(h)
	ws := createWithShared(t, h)
	h.git.RemoveRepo("/src/lib")

	if err := h.remover().Delete(ws.Name, app.DeleteOpts{}); err != nil {
		t.Fatalf("a vanished shared source should not block delete: %v", err)
	}
}

// --- doctor, repair, status ----------------------------------------------

func TestDoctorFindsNothingWrongWithHealthyLinks(t *testing.T) {
	h := newHarness(t)
	withShared(h)
	createWithShared(t, h)

	issues, err := h.doctor().Diagnose()
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestDoctorReportsASharedSourceThatVanished(t *testing.T) {
	h := newHarness(t)
	withShared(h)
	createWithShared(t, h)
	h.git.RemoveRepo("/src/lib")

	issues, _ := h.doctor().Diagnose()

	issue, ok := findIssue(issues, domain.IssueSharedSourceMissing)
	if !ok {
		t.Fatalf("issues = %#v", issues)
	}
	if issue.Fixable() || issue.Repo != "lib" || issue.Path != "/src/lib" {
		t.Fatalf("issue = %#v", issue)
	}
}

func TestDoctorReportsAMissingLink(t *testing.T) {
	h := newHarness(t)
	withShared(h)
	createWithShared(t, h)
	if err := h.fs.RemoveAll("/ws/feat-login/lib"); err != nil {
		t.Fatal(err)
	}

	issues, _ := h.doctor().Diagnose()

	issue, ok := findIssue(issues, domain.IssueSharedLinkMissing)
	if !ok {
		t.Fatalf("issues = %#v", issues)
	}
	if !issue.Fixable() || issue.Path != "/ws/feat-login/lib" || issue.Target != "/src/lib" {
		t.Fatalf("issue = %#v", issue)
	}
}

func TestDoctorReportsALinkPointingElsewhere(t *testing.T) {
	h := newHarness(t)
	withShared(h)
	createWithShared(t, h)
	h.fs.RemoveAll("/ws/feat-login/lib")
	h.fs.AddLink("/src/svc-b", "/ws/feat-login/lib")

	issues, _ := h.doctor().Diagnose()

	issue, ok := findIssue(issues, domain.IssueSharedLinkMismatch)
	if !ok || !issue.Fixable() {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestDoctorReportsAPathThatIsNotASymlinkAsUnfixable(t *testing.T) {
	h := newHarness(t)
	withShared(h)
	createWithShared(t, h)
	h.fs.RemoveAll("/ws/feat-login/lib")
	h.fs.AddDir("/ws/feat-login/lib")

	issues, _ := h.doctor().Diagnose()

	issue, ok := findIssue(issues, domain.IssueSharedLinkNotSymlink)
	if !ok || issue.Fixable() {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestRepairRebuildsAMissingLink(t *testing.T) {
	h := newHarness(t)
	withShared(h)
	createWithShared(t, h)
	h.fs.RemoveAll("/ws/feat-login/lib")
	issues, _ := h.doctor().Diagnose()

	if err := h.repairer().Repair(issues); err != nil {
		t.Fatal(err)
	}

	if got := readlink(t, h, "/ws/feat-login/lib"); got != "/src/lib" {
		t.Errorf("link target = %s", got)
	}
	if again, _ := h.doctor().Diagnose(); len(again) != 0 {
		t.Errorf("issues after repair = %#v", again)
	}
}

func TestRepairRepointsAMismatchedLink(t *testing.T) {
	h := newHarness(t)
	withShared(h)
	createWithShared(t, h)
	h.fs.RemoveAll("/ws/feat-login/lib")
	h.fs.AddLink("/src/svc-b", "/ws/feat-login/lib")
	issues, _ := h.doctor().Diagnose()

	if err := h.repairer().Repair(issues); err != nil {
		t.Fatal(err)
	}

	if got := readlink(t, h, "/ws/feat-login/lib"); got != "/src/lib" {
		t.Errorf("link target = %s", got)
	}
	if !exists(t, h.fs, "/src/svc-b") {
		t.Error("the wrongly linked repository must be untouched")
	}
}

func TestRepairLeavesASharedSourceProblemAlone(t *testing.T) {
	h := newHarness(t)
	withShared(h)
	createWithShared(t, h)
	h.git.RemoveRepo("/src/lib")
	issues, _ := h.doctor().Diagnose()

	if err := h.repairer().Repair(issues); err != nil {
		t.Fatal(err)
	}

	if _, err := h.inspector().Get("feat-login"); err != nil {
		t.Errorf("a missing shared source must not get the workspace deleted: %v", err)
	}
}

func TestRepairDoesNotRelinkIntoADeletedWorkspace(t *testing.T) {
	h := newHarness(t)
	remover := &recordingRemover{}
	repairer := app.NewRepairer(h.cfg, remover, h.fs)

	err := repairer.Repair([]domain.Issue{
		{Kind: domain.IssueWorkspaceDirMissing, Workspace: "task-1", Path: "/ws/task-1"},
		{Kind: domain.IssueSharedLinkMissing, Workspace: "task-1", Repo: "lib", Path: "/ws/task-1/lib", Target: "/src/lib"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if ok, _ := h.fs.LinkExists("/ws/task-1/lib"); ok {
		t.Error("no link should be created inside a workspace that is being deleted")
	}
}

func TestSharedStatusReportsLinkHealth(t *testing.T) {
	h := newHarness(t)
	withShared(h)
	ws := createWithShared(t, h)

	rows := h.inspector().SharedStatus(ws)
	if len(rows) != 1 || !rows[0].Valid || rows[0].Target != "/src/lib" || rows[0].Error != "" {
		t.Fatalf("healthy rows = %#v", rows)
	}

	h.fs.RemoveAll("/src/lib")
	if rows := h.inspector().SharedStatus(ws); rows[0].Valid || rows[0].Error != "source missing" {
		t.Errorf("dangling rows = %#v", rows)
	}

	h.fs.RemoveAll("/ws/feat-login/lib")
	if rows := h.inspector().SharedStatus(ws); rows[0].Valid || rows[0].Error != "link missing" {
		t.Errorf("missing rows = %#v", rows)
	}

	h.fs.AddLink("/elsewhere", "/ws/feat-login/lib")
	if rows := h.inspector().SharedStatus(ws); rows[0].Valid || !strings.Contains(rows[0].Error, "points at /elsewhere") {
		t.Errorf("mismatch rows = %#v", rows)
	}
}
