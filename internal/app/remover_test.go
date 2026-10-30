package app_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/FMLS/bosk/internal/app"
)

func TestDeleteRemovesTheWorkspace(t *testing.T) {
	h := newHarness(t)
	ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a", "svc-b"}})

	if err := h.remover().Delete(ws.Name, app.DeleteOpts{}); err != nil {
		t.Fatal(err)
	}

	if _, ok := h.reg.Registry.Get(ws.Name); ok {
		t.Error("registry entry should be gone")
	}
	if exists(t, h.fs, ws.Path) {
		t.Error("workspace directory should be gone")
	}
	if exists(t, h.fs, h.trashPath("feat-login", "20240102T030405Z")) {
		t.Error("the quarantined copy should have been removed on success")
	}
	// An empty per-workspace directory left in the trash would make doctor
	// report leftover trash after every successful delete.
	if exists(t, h.fs, h.trashPath("feat-login")) {
		t.Errorf("the trash should be tidy, paths = %v", h.fs.Paths())
	}
	if !h.recorded("worktree-prune /src/svc-a") || !h.recorded("worktree-prune /src/svc-b") {
		t.Errorf("expected both repos to be pruned, calls = %v", h.git.Calls)
	}
}

func TestDeleteQuarantinesUnderTheWorkspaceName(t *testing.T) {
	h := newHarness(t)
	ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})
	// Make the final cleanup fail so the quarantined copy stays put.
	quarantine := h.trashPath("feat-login", "20240102T030405Z")
	h.fs.RemoveErr[quarantine] = errors.New("busy")

	if err := h.remover().Delete(ws.Name, app.DeleteOpts{}); err != nil {
		t.Fatal(err)
	}

	if !exists(t, h.fs, quarantine) {
		t.Fatalf("expected %s to exist, paths = %v", quarantine, h.fs.Paths())
	}
	if !h.warner.Contains("remove it manually") {
		t.Errorf("warnings = %v", h.warner.Messages)
	}
}

func TestDeletePreflightRefusals(t *testing.T) {
	cases := []struct {
		name  string
		setup func(h *harness, worktree string)
		want  string
	}{
		{
			name:  "uncommitted changes",
			setup: func(h *harness, wt string) { h.git.DirtyPaths[wt] = 2 },
			want:  "has uncommitted changes; use --force",
		},
		{
			name:  "drifted branch",
			setup: func(h *harness, wt string) { h.git.CheckOut("/src/svc-a", wt, "other") },
			want:  `checked out "other", expected "feat/login"; use --force`,
		},
		{
			name: "worktree no longer registered",
			setup: func(h *harness, wt string) {
				_ = h.git.WorktreeRemove("/src/svc-a", wt, true)
				h.fs.AddDir(wt)
			},
			want: "worktree is not registered in git; use --force",
		},
		{
			name:  "probe failure",
			setup: func(h *harness, wt string) { h.git.RegisteredErr["/src/svc-a"] = errors.New("git broke") },
			want:  "svc-a: git broke",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})
			tc.setup(h, ws.Repos[0].WorktreePath)

			err := h.remover().Delete(ws.Name, app.DeleteOpts{})

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want it to mention %q", err, tc.want)
			}
			if _, ok := h.reg.Registry.Get(ws.Name); !ok {
				t.Error("a refused delete must leave the registry entry alone")
			}
		})
	}
}

func TestDeletePreflightRejectsAMissingSourceRepo(t *testing.T) {
	h := newHarness(t)
	ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})
	h.git.RemoveRepo("/src/svc-a")

	err := h.remover().Delete(ws.Name, app.DeleteOpts{})

	if err == nil || !strings.Contains(err.Error(), "source repository missing") {
		t.Fatalf("error = %v", err)
	}
}

func TestDeletePreflightAllowsAMissingWorktree(t *testing.T) {
	h := newHarness(t)
	ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})
	if err := h.fs.RemoveAll(ws.Repos[0].WorktreePath); err != nil {
		t.Fatal(err)
	}

	if err := h.remover().Delete(ws.Name, app.DeleteOpts{}); err != nil {
		t.Fatalf("a worktree that is already gone should not block delete: %v", err)
	}
}

func TestDeleteForceSkipsPreflight(t *testing.T) {
	h := newHarness(t)
	ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})
	h.git.DirtyPaths[ws.Repos[0].WorktreePath] = 5

	if err := h.remover().Delete(ws.Name, app.DeleteOpts{Force: true}); err != nil {
		t.Fatal(err)
	}
	if _, ok := h.reg.Registry.Get(ws.Name); ok {
		t.Error("registry entry should be gone")
	}
}

func TestDeleteRestoresTheWorkspaceWhenPruneFails(t *testing.T) {
	h := newHarness(t)
	ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})
	h.git.PruneErr["/src/svc-a"] = errors.New("prune refused")

	err := h.remover().Delete(ws.Name, app.DeleteOpts{})

	if err == nil || !strings.Contains(err.Error(), "prune refused") {
		t.Fatalf("error = %v", err)
	}
	if !exists(t, h.fs, ws.Path) {
		t.Errorf("workspace should be restored, paths = %v", h.fs.Paths())
	}
	if _, ok := h.reg.Registry.Get(ws.Name); !ok {
		t.Error("registry entry should survive a failed delete")
	}
	if !h.recorded("worktree-repair /src/svc-a " + ws.Repos[0].WorktreePath) {
		t.Errorf("restore should repair the worktree links, calls = %v", h.git.Calls)
	}
}

func TestDeleteRestoresTheWorkspaceWhenSaveFails(t *testing.T) {
	h := newHarness(t)
	ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})
	h.reg.SaveErr = errors.New("disk full")

	err := h.remover().Delete(ws.Name, app.DeleteOpts{})

	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("error = %v", err)
	}
	if !exists(t, h.fs, ws.Path) {
		t.Error("workspace should be restored")
	}
	if _, ok := h.reg.Registry.Get(ws.Name); !ok {
		t.Error("registry entry should survive a failed delete")
	}
}

func TestDeleteBranchOnlyTouchesBranchesBoskCreated(t *testing.T) {
	h := newHarness(t)
	h.git.AddRepo("/src/svc-b", "feat/login") // pre-existing branch in svc-b
	ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a", "svc-b"}})

	if err := h.remover().Delete(ws.Name, app.DeleteOpts{DeleteBranch: true}); err != nil {
		t.Fatal(err)
	}

	if h.git.BranchExists("/src/svc-a", "feat/login") {
		t.Error("the branch bosk created should be deleted")
	}
	if !h.git.BranchExists("/src/svc-b", "feat/login") {
		t.Error("a pre-existing branch must be left alone")
	}
}

func TestDeleteBranchFailureIsOnlyAWarning(t *testing.T) {
	h := newHarness(t)
	ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})
	// Drop the branch behind bosk's back so the delete attempt fails.
	if err := h.git.DeleteBranch("/src/svc-a", "feat/login", true); err != nil {
		t.Fatal(err)
	}
	h.git.Calls = nil

	if err := h.remover().Delete(ws.Name, app.DeleteOpts{Force: true, DeleteBranch: true}); err != nil {
		t.Fatalf("a failed branch delete must not fail the whole operation: %v", err)
	}
	if !h.warner.Contains("could not delete branch feat/login in svc-a") {
		t.Errorf("warnings = %v", h.warner.Messages)
	}
}

func TestDeleteLeavesNothingForDoctorToReport(t *testing.T) {
	h := newHarness(t)
	ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})

	if err := h.remover().Delete(ws.Name, app.DeleteOpts{}); err != nil {
		t.Fatal(err)
	}

	issues, err := h.doctor().Diagnose()
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("a clean delete should leave a healthy registry, got %#v", issues)
	}
}

func TestDeleteUnknownWorkspace(t *testing.T) {
	h := newHarness(t)

	err := h.remover().Delete("nope", app.DeleteOpts{})

	if err == nil || !strings.Contains(err.Error(), `workspace "nope" not found`) {
		t.Fatalf("error = %v", err)
	}
}

func TestRemoveReposTakesOnlyTheNamedRepos(t *testing.T) {
	h := newHarness(t)
	ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a", "svc-b"}})
	gone, kept := ws.Repos[1].WorktreePath, ws.Repos[0].WorktreePath
	locks, saves := h.reg.Locks, h.reg.Saves

	updated, removed, err := h.remover().RemoveRepos(ws.Name, []string{"svc-b"}, app.DeleteOpts{})
	if err != nil {
		t.Fatal(err)
	}

	if len(removed.Repos) != 1 || removed.Repos[0].RepoName != "svc-b" {
		t.Fatalf("removed = %#v", removed)
	}
	if len(updated.Repos) != 1 || updated.Repos[0].RepoName != "svc-a" {
		t.Fatalf("updated repos = %#v", updated.Repos)
	}
	stored, _ := h.reg.Registry.Get(ws.Name)
	if len(stored.Repos) != 1 || stored.Repos[0].RepoName != "svc-a" {
		t.Errorf("registry repos = %#v", stored.Repos)
	}
	if h.reg.Locks != locks+1 || h.reg.Saves != saves+1 {
		t.Errorf("locks %d->%d, saves %d->%d", locks, h.reg.Locks, saves, h.reg.Saves)
	}
	if exists(t, h.fs, gone) {
		t.Error("the removed worktree should be gone")
	}
	if !exists(t, h.fs, kept) {
		t.Error("the other worktree must be left alone")
	}
	if !h.recorded("worktree-prune /src/svc-b") || h.recorded("worktree-prune /src/svc-a") {
		t.Errorf("only svc-b should be pruned, calls = %v", h.git.Calls)
	}
	if exists(t, h.fs, h.trashPath("feat-login")) {
		t.Errorf("the trash should be tidy, paths = %v", h.fs.Paths())
	}
	if !h.git.BranchExists("/src/svc-b", "feat/login") {
		t.Error("the local branch is kept unless asked")
	}
}

func TestRemoveReposDeletesOnlyBranchesBoskCreated(t *testing.T) {
	h := newHarness(t)
	withSvcC(h)
	h.git.AddRepo("/src/svc-b", "feat/login") // pre-existing branch in svc-b
	h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a", "svc-b", "svc-c"}})

	if _, _, err := h.remover().RemoveRepos("feat-login", []string{"svc-a", "svc-b"}, app.DeleteOpts{DeleteBranch: true}); err != nil {
		t.Fatal(err)
	}

	if h.git.BranchExists("/src/svc-a", "feat/login") {
		t.Error("the branch bosk created should be deleted")
	}
	if !h.git.BranchExists("/src/svc-b", "feat/login") {
		t.Error("a pre-existing branch must be left alone")
	}
	if !h.git.BranchExists("/src/svc-c", "feat/login") {
		t.Error("a repo that stays in the workspace keeps its branch")
	}
}

func TestRemoveReposRefusals(t *testing.T) {
	cases := []struct {
		name      string
		workspace string
		repos     []string
		setup     func(h *harness, worktree string)
		want      string
	}{
		{name: "unknown workspace", workspace: "nope", repos: []string{"svc-a"}, want: `workspace "nope" not found`},
		{name: "nothing selected", workspace: "feat-login", want: "no repositories selected"},
		{name: "unknown repo", workspace: "feat-login", repos: []string{"svc-z"}, want: "repositories not in workspace feat-login: svc-z (available: svc-a, svc-b)"},
		{name: "every repo", workspace: "feat-login", repos: []string{"svc-a", "svc-b"}, want: "use: bosk delete feat-login"},
		{
			name: "uncommitted changes", workspace: "feat-login", repos: []string{"svc-b"},
			setup: func(h *harness, wt string) { h.git.DirtyPaths[wt] = 1 },
			want:  "svc-b: has uncommitted changes; use --force",
		},
		{
			name: "drifted branch", workspace: "feat-login", repos: []string{"svc-b"},
			setup: func(h *harness, wt string) { h.git.CheckOut("/src/svc-b", wt, "other") },
			want:  `checked out "other", expected "feat/login"; use --force`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a", "svc-b"}})
			if tc.setup != nil {
				tc.setup(h, ws.Repos[1].WorktreePath)
			}
			saves := h.reg.Saves

			_, _, err := h.remover().RemoveRepos(tc.workspace, tc.repos, app.DeleteOpts{})

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want it to mention %q", err, tc.want)
			}
			if h.reg.Saves != saves {
				t.Error("a refused remove must not touch the registry")
			}
			stored, _ := h.reg.Registry.Get(ws.Name)
			if len(stored.Repos) != 2 {
				t.Errorf("repos = %#v", stored.Repos)
			}
			if !exists(t, h.fs, ws.Repos[1].WorktreePath) {
				t.Error("a refused remove must leave the worktree alone")
			}
		})
	}
}

func TestRemoveReposForceBypassesPreflight(t *testing.T) {
	h := newHarness(t)
	ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a", "svc-b"}})
	h.git.DirtyPaths[ws.Repos[1].WorktreePath] = 3

	if _, _, err := h.remover().RemoveRepos(ws.Name, []string{"svc-b"}, app.DeleteOpts{Force: true}); err != nil {
		t.Fatal(err)
	}

	stored, _ := h.reg.Registry.Get(ws.Name)
	if len(stored.Repos) != 1 {
		t.Errorf("repos = %#v", stored.Repos)
	}
}

func TestRemoveReposRestoresTheWorktreeWhenSaveFails(t *testing.T) {
	h := newHarness(t)
	ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a", "svc-b"}})
	wt := ws.Repos[1].WorktreePath
	h.reg.SaveErr = errors.New("disk full")

	_, _, err := h.remover().RemoveRepos(ws.Name, []string{"svc-b"}, app.DeleteOpts{})

	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("error = %v", err)
	}
	if !exists(t, h.fs, wt) {
		t.Error("the worktree should be restored")
	}
	stored, _ := h.reg.Registry.Get(ws.Name)
	if len(stored.Repos) != 2 {
		t.Errorf("repos = %#v", stored.Repos)
	}
	if !h.recorded("worktree-repair /src/svc-b " + wt) {
		t.Errorf("restore should repair the worktree links, calls = %v", h.git.Calls)
	}
}

func TestRemoveReposRestoresTheWorktreeWhenPruneFails(t *testing.T) {
	h := newHarness(t)
	ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a", "svc-b"}})
	h.git.PruneErr["/src/svc-b"] = errors.New("prune failed")

	_, _, err := h.remover().RemoveRepos(ws.Name, []string{"svc-b"}, app.DeleteOpts{})

	if err == nil || !strings.Contains(err.Error(), "prune failed") {
		t.Fatalf("error = %v", err)
	}
	if !exists(t, h.fs, ws.Repos[1].WorktreePath) {
		t.Error("the worktree should be restored")
	}
}

func TestRemoveReposWarnsWhenTheQuarantinedCopyCannotBeDeleted(t *testing.T) {
	h := newHarness(t)
	ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a", "svc-b"}})
	quarantine := h.trashPath("feat-login", "20240102T030405Z")
	h.fs.RemoveErr[quarantine] = errors.New("busy")

	if _, _, err := h.remover().RemoveRepos(ws.Name, []string{"svc-b"}, app.DeleteOpts{}); err != nil {
		t.Fatal(err)
	}

	if !exists(t, h.fs, h.trashPath("feat-login", "20240102T030405Z", "svc-b")) {
		t.Fatalf("paths = %v", h.fs.Paths())
	}
	if !h.warner.Contains("remove it manually") {
		t.Errorf("warnings = %v", h.warner.Messages)
	}
}
