package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNameFromBranch(t *testing.T) {
	cases := map[string]string{
		"feat/login":     "feat-login",
		"fix\\windows":   "fix-windows",
		"with space":     "with-space",
		"scope:thing":    "scope-thing",
		"already-plain":  "already-plain",
		"a/b c:d\\e":     "a-b-c-d-e",
		"":               "",
		"feat/a/b/c/dir": "feat-a-b-c-dir",
	}
	for branch, want := range cases {
		if got := NameFromBranch(branch); got != want {
			t.Errorf("NameFromBranch(%q) = %q, want %q", branch, got, want)
		}
	}
}

func TestValidateWorkspaceName(t *testing.T) {
	valid := []string{"feat-login", "task.1", "a", "-dash"}
	for _, name := range valid {
		if err := ValidateWorkspaceName(name); err != nil {
			t.Errorf("ValidateWorkspaceName(%q) = %v, want nil", name, err)
		}
	}
	invalid := []string{"", ".", "..", "a/b", `a\b`, "x..y", "../escape"}
	for _, name := range invalid {
		if err := ValidateWorkspaceName(name); err == nil {
			t.Errorf("ValidateWorkspaceName(%q) = nil, want error", name)
		}
	}
}

func TestSelectReposPreservesRequestOrder(t *testing.T) {
	repos := []Repo{{Name: "svc-a"}, {Name: "svc-b"}, {Name: "svc-c"}}

	got, err := SelectRepos(repos, []string{"svc-c", "svc-a", "svc-c", ""})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "svc-c" || got[1].Name != "svc-a" {
		t.Fatalf("SelectRepos = %v", got)
	}
}

func TestSelectReposReportsMissing(t *testing.T) {
	repos := []Repo{{Name: "svc-a"}}

	_, err := SelectRepos(repos, []string{"nope", "svc-a"})
	var missing *MissingReposError
	if !errors.As(err, &missing) {
		t.Fatalf("error = %v (%T), want *MissingReposError", err, err)
	}
	if len(missing.Names) != 1 || missing.Names[0] != "nope" {
		t.Fatalf("missing = %v", missing.Names)
	}
	if got := missing.Error(); got != "unknown repositories: nope (available: svc-a)" {
		t.Fatalf("message = %q", got)
	}
}

func TestRegistryUpsertAndRemove(t *testing.T) {
	var reg Registry
	reg.Upsert(NewWorkspace("a", "/ws/a", "main", time.Now()))
	reg.Upsert(NewWorkspace("b", "/ws/b", "main", time.Now()))
	reg.Upsert(Workspace{Name: "a", Path: "/ws/a2", Branch: "dev"})

	if len(reg.Workspaces) != 2 {
		t.Fatalf("workspaces = %d, want 2", len(reg.Workspaces))
	}
	got, ok := reg.Get("a")
	if !ok || got.Path != "/ws/a2" || got.Branch != "dev" {
		t.Fatalf("Get(a) = %#v ok=%v", got, ok)
	}
	if !reg.Remove("a") || reg.Remove("a") {
		t.Fatal("Remove should succeed once and then report missing")
	}
	if len(reg.Workspaces) != 1 || reg.Workspaces[0].Name != "b" {
		t.Fatalf("workspaces = %#v", reg.Workspaces)
	}
}

func TestNewWorkspaceStampsUTC(t *testing.T) {
	at := time.Date(2024, 3, 1, 12, 30, 0, 0, time.FixedZone("CST", 8*3600))

	ws := NewWorkspace("n", "/ws/n", "feat/x", at)

	if ws.CreatedAt != "2024-03-01T04:30:00Z" {
		t.Fatalf("CreatedAt = %s", ws.CreatedAt)
	}
}

func TestIssueRenderingAndFixability(t *testing.T) {
	cases := []struct {
		issue   Issue
		fixable bool
		message string
		action  string
	}{
		{
			issue:   Issue{Kind: IssueWorkspaceDirMissing, Workspace: "w", Path: "/ws/w"},
			fixable: true,
			message: "workspace directory missing: /ws/w",
			action:  "bosk delete --force w",
		},
		{
			issue:   Issue{Kind: IssueSourceRepoMissing, Workspace: "w", Repo: "svc-a", Path: "/src/svc-a"},
			fixable: false,
			message: "source repository missing: /src/svc-a",
			action:  "restore the source clone or bosk delete --force w",
		},
		{
			issue:   Issue{Kind: IssueWorktreePathMissing, Workspace: "w", Repo: "svc-a"},
			fixable: true,
			message: "worktree path missing",
			action:  "bosk delete --force w",
		},
		{
			issue:   Issue{Kind: IssueWorktreeUnregistered, Workspace: "w", Repo: "svc-a"},
			fixable: true,
			message: "worktree is not registered in git",
			action:  "bosk delete --force w",
		},
		{
			issue:   Issue{Kind: IssueWorktreeProbeFailed, Workspace: "w", Detail: "git blew up"},
			fixable: false,
			message: "git blew up",
			action:  "",
		},
		{
			issue:   Issue{Kind: IssueTrashShadowsWorkspace, Workspace: "w", Path: "/ws/.trash/w/t"},
			fixable: false,
			message: "quarantined copy left in /ws/.trash/w/t",
			action:  "inspect the directory; restore or bosk doctor --fix",
		},
		{
			issue:   Issue{Kind: IssueTrashOrphaned, Workspace: "w", Path: "/ws/.trash/w/t"},
			fixable: true,
			message: "leftover trash: /ws/.trash/w/t",
			action:  "bosk doctor --fix",
		},
	}
	for _, tc := range cases {
		if got := tc.issue.Fixable(); got != tc.fixable {
			t.Errorf("%s Fixable = %v, want %v", tc.issue.Kind, got, tc.fixable)
		}
		if got := tc.issue.Message(); got != tc.message {
			t.Errorf("%s Message = %q, want %q", tc.issue.Kind, got, tc.message)
		}
		if got := tc.issue.SuggestedAction(); got != tc.action {
			t.Errorf("%s SuggestedAction = %q, want %q", tc.issue.Kind, got, tc.action)
		}
	}
}

func TestWithoutReposDropsNamedReposWithoutMutatingTheOriginal(t *testing.T) {
	ws := Workspace{Name: "w", Repos: []RepoWorktree{{RepoName: "a"}, {RepoName: "b"}, {RepoName: "c"}}}

	got := ws.WithoutRepos([]string{"b", "missing"})

	if len(got.Repos) != 2 || got.Repos[0].RepoName != "a" || got.Repos[1].RepoName != "c" {
		t.Fatalf("got = %#v", got.Repos)
	}
	if len(ws.Repos) != 3 || ws.Repos[1].RepoName != "b" {
		t.Errorf("original was mutated: %#v", ws.Repos)
	}
}

func TestWorkspaceHasMemberCoversReposAndShared(t *testing.T) {
	ws := Workspace{
		Name:   "w",
		Repos:  []RepoWorktree{{RepoName: "a"}},
		Shared: []SharedRepo{{RepoName: "lib"}},
	}

	for _, name := range []string{"a", "lib"} {
		if !ws.HasMember(name) {
			t.Errorf("HasMember(%q) = false", name)
		}
	}
	if ws.HasMember("other") {
		t.Error(`HasMember("other") = true`)
	}
}

func TestWithSharedAppendsWithoutTouchingTheReceiver(t *testing.T) {
	ws := Workspace{Name: "w", Repos: []RepoWorktree{{RepoName: "a"}}, Shared: []SharedRepo{{RepoName: "x"}}}

	got, err := ws.WithShared([]SharedRepo{{RepoName: "lib"}})
	if err != nil {
		t.Fatal(err)
	}

	if len(got.Shared) != 2 || got.Shared[1].RepoName != "lib" {
		t.Fatalf("shared = %#v", got.Shared)
	}
	if len(ws.Shared) != 1 {
		t.Fatalf("receiver changed: %#v", ws.Shared)
	}
}

func TestWithSharedRefusesANameTheWorkspaceUses(t *testing.T) {
	ws := Workspace{Name: "w", Repos: []RepoWorktree{{RepoName: "a"}}, Shared: []SharedRepo{{RepoName: "x"}}}

	for _, name := range []string{"a", "x"} {
		if _, err := ws.WithShared([]SharedRepo{{RepoName: name}}); err == nil {
			t.Errorf("WithShared(%q) = nil, want error", name)
		}
	}
	if _, err := ws.WithShared([]SharedRepo{{RepoName: "n"}, {RepoName: "n"}}); err == nil {
		t.Error("the same new name twice should be refused")
	}
}

func TestWithoutSharedDropsOnlyTheNamedLinks(t *testing.T) {
	ws := Workspace{Shared: []SharedRepo{{RepoName: "x"}, {RepoName: "y"}}}

	got := ws.WithoutShared([]string{"x"})

	if len(got.Shared) != 1 || got.Shared[0].RepoName != "y" {
		t.Fatalf("shared = %#v", got.Shared)
	}
	if len(ws.Shared) != 2 {
		t.Fatalf("receiver changed: %#v", ws.Shared)
	}
}

func TestValidateDisjoint(t *testing.T) {
	if err := ValidateDisjoint([]string{"a", "b"}, []string{"c"}); err != nil {
		t.Errorf("disjoint sets: %v", err)
	}
	if err := ValidateDisjoint([]string{"a", "b"}, []string{"b"}); err == nil {
		t.Error("overlapping sets should be refused")
	}
	if err := ValidateDisjoint(nil, nil); err != nil {
		t.Errorf("empty sets: %v", err)
	}
}
