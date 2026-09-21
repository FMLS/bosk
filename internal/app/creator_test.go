package app_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/FMLS/bosk/internal/app"
)

func TestCreateProvisionsWorktreesAndRecordsWorkspace(t *testing.T) {
	h := newHarness(t)

	ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a", "svc-b"}})

	if ws.Name != "feat-login" || ws.Path != "/ws/feat-login" || ws.Branch != "feat/login" {
		t.Fatalf("workspace = %#v", ws)
	}
	if ws.CreatedAt != "2024-01-02T03:04:05Z" {
		t.Fatalf("CreatedAt = %s, want the injected clock's time", ws.CreatedAt)
	}
	if len(ws.Repos) != 2 {
		t.Fatalf("repos = %#v", ws.Repos)
	}
	for _, repo := range ws.Repos {
		if !repo.CreatedBranch {
			t.Errorf("%s: expected bosk to have created the branch", repo.RepoName)
		}
		if !exists(t, h.fs, repo.WorktreePath) {
			t.Errorf("%s: worktree directory missing", repo.RepoName)
		}
	}
	stored, ok := h.reg.Registry.Get("feat-login")
	if !ok || len(stored.Repos) != 2 {
		t.Fatalf("registry = %#v", h.reg.Registry)
	}
	if h.reg.Locks == 0 {
		t.Error("create should hold the registry lock")
	}
}

func TestCreateUsesAnExistingBranchWithoutCreatingIt(t *testing.T) {
	h := newHarness(t)
	h.git.AddRepo("/src/svc-a", "feat/login")

	ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})

	if ws.Repos[0].CreatedBranch {
		t.Fatal("branch already existed, so bosk should not claim to have created it")
	}
}

func TestCreateAcceptsAnExplicitName(t *testing.T) {
	h := newHarness(t)

	ws := h.mustCreate(t, app.CreateOpts{Name: "task-1", Branch: "feat/login", RepoNames: []string{"svc-a"}})

	if ws.Name != "task-1" || ws.Path != "/ws/task-1" {
		t.Fatalf("workspace = %#v", ws)
	}
}

func TestCreateFallsBackToNameAsBranch(t *testing.T) {
	h := newHarness(t)

	ws := h.mustCreate(t, app.CreateOpts{Name: "hotfix", All: true})

	if ws.Branch != "hotfix" {
		t.Fatalf("branch = %s, want the workspace name", ws.Branch)
	}
}

func TestCreateRejectsBadInput(t *testing.T) {
	cases := []struct {
		name string
		opts app.CreateOpts
		want string
	}{
		{"no name and no branch", app.CreateOpts{}, "workspace name or --branch is required"},
		{"repos and all", app.CreateOpts{Branch: "b", RepoNames: []string{"svc-a"}, All: true}, "not both"},
		{"no selection", app.CreateOpts{Branch: "b"}, "select repositories"},
		{"unknown repo", app.CreateOpts{Branch: "b", RepoNames: []string{"nope"}}, "unknown repositories"},
		{"path separator in name", app.CreateOpts{Name: "a/b", All: true}, "path separators"},
		{"dot dot in name", app.CreateOpts{Name: "..", All: true}, "invalid workspace name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)

			_, err := h.creator().Create(tc.opts)

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want it to mention %q", err, tc.want)
			}
			if h.reg.Saves != 0 {
				t.Error("a rejected create must not touch the registry")
			}
		})
	}
}

func TestCreateRejectsAnInvalidBranchName(t *testing.T) {
	h := newHarness(t)
	h.git.InvalidBranches["bad branch"] = true

	_, err := h.creator().Create(app.CreateOpts{Branch: "bad branch", All: true})

	if err == nil || !strings.Contains(err.Error(), "invalid branch name") {
		t.Fatalf("error = %v", err)
	}
}

func TestCreateRejectsAnExistingWorkspace(t *testing.T) {
	h := newHarness(t)
	h.mustCreate(t, app.CreateOpts{Name: "task-1", Branch: "feat/a", RepoNames: []string{"svc-a"}})

	_, err := h.creator().Create(app.CreateOpts{Name: "task-1", Branch: "feat/b", RepoNames: []string{"svc-b"}})

	if err == nil || !strings.Contains(err.Error(), `workspace "task-1" already exists`) {
		t.Fatalf("error = %v", err)
	}
}

func TestCreateRejectsAnExistingDirectory(t *testing.T) {
	h := newHarness(t)
	h.fs.AddDir("/ws/task-1")

	_, err := h.creator().Create(app.CreateOpts{Name: "task-1", Branch: "feat/a", RepoNames: []string{"svc-a"}})

	if err == nil || !strings.Contains(err.Error(), "workspace directory already exists") {
		t.Fatalf("error = %v", err)
	}
}

func TestCreateRollsBackWhenALaterRepoConflicts(t *testing.T) {
	h := newHarness(t)
	// svc-b already has the branch checked out somewhere else, so the second
	// repository in the list fails after the first has succeeded.
	h.git.CheckOut("/src/svc-b", "/elsewhere/svc-b", "feat/login")

	_, err := h.creator().Create(app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a", "svc-b"}})

	if err == nil || !strings.Contains(err.Error(), "already checked out") {
		t.Fatalf("error = %v", err)
	}
	if len(h.reg.Registry.Workspaces) != 0 {
		t.Errorf("registry should be untouched, got %#v", h.reg.Registry.Workspaces)
	}
	if got := h.git.Worktrees("/src/svc-a"); len(got) != 0 {
		t.Errorf("svc-a worktrees = %v, want the rollback to have removed them", got)
	}
	for _, branch := range h.git.Branches("/src/svc-a") {
		if branch == "feat/login" {
			t.Error("rollback should have deleted the branch it created")
		}
	}
	if exists(t, h.fs, "/ws/feat-login") {
		t.Error("rollback should have removed the workspace directory")
	}
}

func TestCreateKeepsABranchItDidNotCreate(t *testing.T) {
	h := newHarness(t)
	h.git.AddRepo("/src/svc-a", "feat/login")
	h.git.CheckOut("/src/svc-b", "/elsewhere/svc-b", "feat/login")

	_, err := h.creator().Create(app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a", "svc-b"}})

	if err == nil {
		t.Fatal("expected the conflict to fail the create")
	}
	if !h.git.BranchExists("/src/svc-a", "feat/login") {
		t.Fatal("rollback must not delete a pre-existing branch")
	}
}

func TestCreateRollsBackWhenTheRegistrySaveFails(t *testing.T) {
	h := newHarness(t)
	h.reg.SaveErr = errors.New("disk full")

	_, err := h.creator().Create(app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})

	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("error = %v", err)
	}
	if got := h.git.Worktrees("/src/svc-a"); len(got) != 0 {
		t.Errorf("worktrees = %v, want them rolled back", got)
	}
	if exists(t, h.fs, "/ws/feat-login") {
		t.Error("workspace directory should be gone")
	}
}

func TestCreateRollsBackWhenAddingAWorktreeFails(t *testing.T) {
	h := newHarness(t)
	h.git.AddErr["/src/svc-b"] = errors.New("worktree add exploded")

	_, err := h.creator().Create(app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a", "svc-b"}})

	if err == nil || !strings.Contains(err.Error(), "svc-b: worktree add exploded") {
		t.Fatalf("error = %v", err)
	}
	if got := h.git.Worktrees("/src/svc-a"); len(got) != 0 {
		t.Errorf("svc-a worktrees = %v, want them rolled back", got)
	}
}

func TestCreateFailsWhenNoStartPointExists(t *testing.T) {
	h := newHarness(t)
	h.git.NoStartPoint["/src/svc-a"] = true

	_, err := h.creator().Create(app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})

	if err == nil || !strings.Contains(err.Error(), "start point not found") {
		t.Fatalf("error = %v", err)
	}
}

func TestCreateWarnsButProceedsWhenFetchFails(t *testing.T) {
	h := newHarness(t)
	h.git.FetchErr["/src/svc-a"] = errors.New("network down")

	ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})

	if len(ws.Repos) != 1 {
		t.Fatalf("repos = %#v", ws.Repos)
	}
	if !h.warner.Contains("fetch svc-a failed") {
		t.Fatalf("warnings = %v", h.warner.Messages)
	}
}

func TestCreateWarnsAboutDuplicateRepoNames(t *testing.T) {
	h := newHarness(t)
	h.disc.Skipped = []string{"/src/team/svc-a"}

	h.mustCreate(t, app.CreateOpts{Branch: "feat/login", All: true})

	if !h.warner.Contains("skipped duplicate repo name: /src/team/svc-a") {
		t.Fatalf("warnings = %v", h.warner.Messages)
	}
}

func TestCreateFailsWhenNothingIsDiscovered(t *testing.T) {
	h := newHarness(t)
	h.disc.Repos = nil

	_, err := h.creator().Create(app.CreateOpts{Branch: "feat/login", All: true})

	if err == nil || !strings.Contains(err.Error(), "no git repositories found") {
		t.Fatalf("error = %v", err)
	}
}

func TestCreateAllTakesEveryRepository(t *testing.T) {
	h := newHarness(t)

	ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/all", All: true})

	if len(ws.Repos) != 2 || ws.Repos[0].RepoName != "svc-a" || ws.Repos[1].RepoName != "svc-b" {
		t.Fatalf("repos = %#v", ws.Repos)
	}
}
