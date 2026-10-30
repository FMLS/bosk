package app_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/FMLS/bosk/internal/app"
	"github.com/FMLS/bosk/internal/domain"
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

// withSvcC gives the harness a third repository, so a workspace created from
// svc-a and svc-b has something left to add.
func withSvcC(h *harness) {
	h.fs.AddDir("/src/svc-c")
	h.git.AddRepo("/src/svc-c")
	h.disc.Repos = append(h.disc.Repos, domain.Repo{Name: "svc-c", Path: "/src/svc-c"})
}

func TestAddAppendsWorktreesToAnExistingWorkspace(t *testing.T) {
	h := newHarness(t)
	withSvcC(h)
	before := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})
	locks := h.reg.Locks

	ws, added, err := h.creator().Add(app.AddOpts{Name: "feat-login", RepoNames: []string{"svc-b", "svc-c"}})
	if err != nil {
		t.Fatal(err)
	}

	if len(added.Repos) != 2 || added.Repos[0].RepoName != "svc-b" || added.Repos[1].RepoName != "svc-c" {
		t.Fatalf("added = %#v", added)
	}
	for _, repo := range added.Repos {
		if repo.Branch != "feat/login" || repo.WorktreePath != "/ws/feat-login/"+repo.RepoName || !repo.CreatedBranch {
			t.Errorf("worktree = %#v", repo)
		}
		if !exists(t, h.fs, repo.WorktreePath) {
			t.Errorf("%s: worktree directory missing", repo.RepoName)
		}
	}
	if len(ws.Repos) != 3 || ws.Repos[0] != before.Repos[0] {
		t.Fatalf("repos = %#v, want svc-a kept first", ws.Repos)
	}
	if ws.CreatedAt != before.CreatedAt {
		t.Errorf("CreatedAt changed to %s", ws.CreatedAt)
	}
	stored, _ := h.reg.Registry.Get("feat-login")
	if len(stored.Repos) != 3 {
		t.Fatalf("registry = %#v", stored)
	}
	if h.reg.Locks == locks {
		t.Error("add should hold the registry lock")
	}
}

func TestAddUsesAnExistingBranchWithoutCreatingIt(t *testing.T) {
	h := newHarness(t)
	h.git.AddRepo("/src/svc-b", "feat/login")
	h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})

	_, added, err := h.creator().Add(app.AddOpts{Name: "feat-login", RepoNames: []string{"svc-b"}})
	if err != nil {
		t.Fatal(err)
	}

	if added.Repos[0].CreatedBranch {
		t.Fatal("branch already existed, so bosk should not claim to have created it")
	}
}

func TestAddAllSkipsRepositoriesAlreadyPresent(t *testing.T) {
	h := newHarness(t)
	withSvcC(h)
	h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})

	_, added, err := h.creator().Add(app.AddOpts{Name: "feat-login", All: true})
	if err != nil {
		t.Fatal(err)
	}

	if len(added.Repos) != 2 || added.Repos[0].RepoName != "svc-b" || added.Repos[1].RepoName != "svc-c" {
		t.Fatalf("added = %#v", added)
	}
}

func TestAddRejectsBadInput(t *testing.T) {
	cases := []struct {
		name  string
		setup func(h *harness)
		opts  app.AddOpts
		want  string
	}{
		{"unknown workspace", nil, app.AddOpts{Name: "nope", RepoNames: []string{"svc-b"}}, `workspace "nope" not found`},
		{"repos and all", nil, app.AddOpts{Name: "feat-login", RepoNames: []string{"svc-b"}, All: true}, "not both"},
		{"no selection", nil, app.AddOpts{Name: "feat-login"}, "select repositories"},
		{"unknown repo", nil, app.AddOpts{Name: "feat-login", RepoNames: []string{"nope"}}, "unknown repositories"},
		{"repo already present", nil, app.AddOpts{Name: "feat-login", RepoNames: []string{"svc-a"}}, `repo "svc-a" is already in workspace "feat-login"`},
		{"all already present", func(h *harness) {
			h.disc.Repos = h.disc.Repos[:1]
		}, app.AddOpts{Name: "feat-login", All: true}, "no repositories to add"},
		{"workspace directory missing", func(h *harness) {
			_ = h.fs.RemoveAll("/ws/feat-login")
		}, app.AddOpts{Name: "feat-login", RepoNames: []string{"svc-b"}}, "workspace directory missing"},
		{"target path exists", func(h *harness) {
			h.fs.AddDir("/ws/feat-login/svc-b")
		}, app.AddOpts{Name: "feat-login", RepoNames: []string{"svc-b"}}, "svc-b: path already exists"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})
			if tc.setup != nil {
				tc.setup(h)
			}
			saves := h.reg.Saves

			_, _, err := h.creator().Add(tc.opts)

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want it to mention %q", err, tc.want)
			}
			if h.reg.Saves != saves {
				t.Error("a rejected add must not touch the registry")
			}
			if got := h.git.Worktrees("/src/svc-b"); len(got) != 0 {
				t.Errorf("svc-b worktrees = %v, want none", got)
			}
		})
	}
}

// assertAddRolledBack checks that a failed add of svc-b and svc-c left the
// workspace exactly as create made it.
func assertAddRolledBack(t *testing.T, h *harness) {
	t.Helper()
	for _, repo := range []string{"/src/svc-b", "/src/svc-c"} {
		if got := h.git.Worktrees(repo); len(got) != 0 {
			t.Errorf("%s worktrees = %v, want the rollback to have removed them", repo, got)
		}
		for _, branch := range h.git.Branches(repo) {
			if branch == "feat/login" {
				t.Errorf("%s: rollback should have deleted the branch it created", repo)
			}
		}
	}
	if !exists(t, h.fs, "/ws/feat-login") || !exists(t, h.fs, "/ws/feat-login/svc-a") {
		t.Error("rollback must keep the workspace directory and its earlier worktrees")
	}
	if got := h.git.Worktrees("/src/svc-a"); len(got) != 1 {
		t.Errorf("svc-a worktrees = %v, want the original one kept", got)
	}
	stored, _ := h.reg.Registry.Get("feat-login")
	if len(stored.Repos) != 1 {
		t.Errorf("registry repos = %#v, want only svc-a", stored.Repos)
	}
}

func TestAddRollsBackWhenALaterRepoConflicts(t *testing.T) {
	h := newHarness(t)
	withSvcC(h)
	h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})
	h.git.CheckOut("/src/svc-c", "/elsewhere/svc-c", "feat/login")

	_, _, err := h.creator().Add(app.AddOpts{Name: "feat-login", RepoNames: []string{"svc-b", "svc-c"}})

	if err == nil || !strings.Contains(err.Error(), "svc-c: branch \"feat/login\" is already checked out") {
		t.Fatalf("error = %v", err)
	}
	// svc-c's checkout elsewhere is not ours to remove; only svc-b is checked.
	if got := h.git.Worktrees("/src/svc-b"); len(got) != 0 {
		t.Errorf("svc-b worktrees = %v, want them rolled back", got)
	}
	if !exists(t, h.fs, "/ws/feat-login/svc-a") {
		t.Error("rollback must keep the earlier worktrees")
	}
}

func TestAddRollsBackWhenAddingAWorktreeFails(t *testing.T) {
	h := newHarness(t)
	withSvcC(h)
	h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})
	h.git.AddErr["/src/svc-c"] = errors.New("worktree add exploded")

	_, _, err := h.creator().Add(app.AddOpts{Name: "feat-login", RepoNames: []string{"svc-b", "svc-c"}})

	if err == nil || !strings.Contains(err.Error(), "svc-c: worktree add exploded") {
		t.Fatalf("error = %v", err)
	}
	assertAddRolledBack(t, h)
}

func TestAddRollsBackWhenTheRegistrySaveFails(t *testing.T) {
	h := newHarness(t)
	withSvcC(h)
	h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})
	h.reg.SaveErr = errors.New("disk full")

	_, _, err := h.creator().Add(app.AddOpts{Name: "feat-login", RepoNames: []string{"svc-b", "svc-c"}})

	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("error = %v", err)
	}
	assertAddRolledBack(t, h)
}
