package app_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/FMLS/bosk/internal/app"
)

func TestListReturnsRecordedWorkspaces(t *testing.T) {
	h := newHarness(t)
	h.mustCreate(t, app.CreateOpts{Name: "one", Branch: "feat/one", RepoNames: []string{"svc-a"}})
	h.mustCreate(t, app.CreateOpts{Name: "two", Branch: "feat/two", RepoNames: []string{"svc-b"}})

	list, err := h.inspector().List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Name != "one" || list[1].Name != "two" {
		t.Fatalf("list = %#v", list)
	}
}

func TestGetUnknownWorkspace(t *testing.T) {
	h := newHarness(t)

	_, err := h.inspector().Get("nope")

	if err == nil || !strings.Contains(err.Error(), `workspace "nope" not found`) {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveByName(t *testing.T) {
	h := newHarness(t)
	h.mustCreate(t, app.CreateOpts{Name: "task-1", Branch: "feat/x", RepoNames: []string{"svc-a"}})

	ws, err := h.inspector().Resolve("task-1")
	if err != nil {
		t.Fatal(err)
	}
	if ws.Name != "task-1" {
		t.Fatalf("resolved %s", ws.Name)
	}
}

func TestResolveFromTheWorkingDirectory(t *testing.T) {
	h := newHarness(t)
	ws := h.mustCreate(t, app.CreateOpts{Name: "task-1", Branch: "feat/x", RepoNames: []string{"svc-a"}})
	h.fs.Cwd = ws.Repos[0].WorktreePath + "/deep/inside"

	got, err := h.inspector().Resolve("")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "task-1" {
		t.Fatalf("resolved %s", got.Name)
	}
}

func TestResolvePrefersTheDeepestWorkspace(t *testing.T) {
	h := newHarness(t)
	h.mustCreate(t, app.CreateOpts{Name: "outer", Branch: "feat/outer", RepoNames: []string{"svc-a"}})
	// A workspace nested inside another one, which the registry allows.
	h.cfg.WorkspaceDir = "/ws/outer"
	h.mustCreate(t, app.CreateOpts{Name: "inner", Branch: "feat/inner", RepoNames: []string{"svc-b"}})
	h.fs.Cwd = "/ws/outer/inner/svc-b"

	got, err := h.inspector().Resolve("")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "inner" {
		t.Fatalf("resolved %s, want the innermost workspace", got.Name)
	}
}

func TestResolveOutsideAnyWorkspace(t *testing.T) {
	h := newHarness(t)
	h.mustCreate(t, app.CreateOpts{Name: "task-1", Branch: "feat/x", RepoNames: []string{"svc-a"}})
	h.fs.Cwd = "/somewhere/else"

	_, err := h.inspector().Resolve("")

	if err == nil || !strings.Contains(err.Error(), "not inside a bosk workspace") {
		t.Fatalf("error = %v", err)
	}
}

func TestStatusReportsCleanWorktrees(t *testing.T) {
	h := newHarness(t)
	ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a", "svc-b"}})
	h.git.AheadPaths[ws.Repos[0].WorktreePath] = 2
	h.git.BehindPaths[ws.Repos[0].WorktreePath] = 1
	h.git.DirtyPaths[ws.Repos[1].WorktreePath] = 3

	rows := h.inspector().Status(ws)

	if len(rows) != 2 {
		t.Fatalf("rows = %#v", rows)
	}
	if rows[0].Branch != "feat/login" || rows[0].Ahead != 2 || rows[0].Behind != 1 || rows[0].Error != "" {
		t.Fatalf("row[0] = %#v", rows[0])
	}
	if rows[1].Dirty != 3 {
		t.Fatalf("row[1] = %#v", rows[1])
	}
}

func TestStatusReportsAMissingWorktree(t *testing.T) {
	h := newHarness(t)
	ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})
	if err := h.fs.RemoveAll(ws.Repos[0].WorktreePath); err != nil {
		t.Fatal(err)
	}

	rows := h.inspector().Status(ws)

	if rows[0].Error != "worktree missing" {
		t.Fatalf("row = %#v", rows[0])
	}
}

func TestStatusReportsGitErrorsPerRepo(t *testing.T) {
	h := newHarness(t)
	ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a", "svc-b"}})
	h.git.StatusErr[ws.Repos[0].WorktreePath] = errors.New("git exploded")

	rows := h.inspector().Status(ws)

	if rows[0].Error != "git exploded" {
		t.Fatalf("row[0] = %#v", rows[0])
	}
	if rows[1].Error != "" {
		t.Fatalf("one broken repo must not spoil the rest: %#v", rows[1])
	}
}

func TestStatusLabelsADetachedHead(t *testing.T) {
	h := newHarness(t)
	ws := h.mustCreate(t, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})
	// Drop the worktree registration so the fake reports no current branch.
	if err := h.git.WorktreeRemove("/src/svc-a", ws.Repos[0].WorktreePath, true); err != nil {
		t.Fatal(err)
	}
	h.fs.AddDir(ws.Repos[0].WorktreePath)

	rows := h.inspector().Status(ws)

	if rows[0].Branch != "(detached)" {
		t.Fatalf("row = %#v", rows[0])
	}
}

func TestDiscoveredPassesThroughSkips(t *testing.T) {
	h := newHarness(t)
	h.disc.Skipped = []string{"/src/team/svc-a"}

	repos, skipped := h.inspector().Discovered()

	if len(repos) != 2 || len(skipped) != 1 {
		t.Fatalf("repos = %v skipped = %v", repos, skipped)
	}
}
