package git_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/FMLS/bosk/internal/adapters/git"
	"github.com/FMLS/bosk/internal/adapters/pathx"
	"github.com/FMLS/bosk/internal/testsupport"
)

func realClient() *git.Client {
	return git.NewClient(git.NewExecRunner(), pathx.New())
}

func TestWorktreeAddAndStatus(t *testing.T) {
	client := realClient()
	root := t.TempDir()
	repo := filepath.Join(root, "app")
	testsupport.InitRepo(t, repo)

	if !client.IsRepo(repo) {
		t.Fatal("expected git repo")
	}
	branch, err := client.DefaultBranch(repo)
	if err != nil {
		t.Fatal(err)
	}
	if branch != "main" {
		t.Fatalf("default branch = %s", branch)
	}

	wt := filepath.Join(root, "feat")
	if err := client.WorktreeAddNew(repo, wt, "feat/login", "main"); err != nil {
		t.Fatal(err)
	}
	if !client.BranchExists(repo, "feat/login") {
		t.Fatal("branch should exist")
	}
	inUse, err := client.BranchInUse(repo, "feat/login")
	if err != nil {
		t.Fatal(err)
	}
	if !inUse {
		t.Fatal("branch should be in use")
	}
	registered, err := client.WorktreeRegistered(repo, wt)
	if err != nil {
		t.Fatal(err)
	}
	if !registered {
		t.Fatal("worktree should be registered")
	}
	cur, err := client.CurrentBranch(wt)
	if err != nil {
		t.Fatal(err)
	}
	if cur != "feat/login" {
		t.Fatalf("current branch = %s", cur)
	}
	dirty, err := client.DirtyCount(wt)
	if err != nil {
		t.Fatal(err)
	}
	if dirty != 0 {
		t.Fatalf("dirty = %d", dirty)
	}

	if err := os.WriteFile(filepath.Join(wt, "README"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirty, err = client.DirtyCount(wt)
	if err != nil {
		t.Fatal(err)
	}
	if dirty == 0 {
		t.Fatal("expected dirty worktree")
	}

	if err := client.WorktreeRemove(repo, wt, true); err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteBranch(repo, "feat/login", true); err != nil {
		t.Fatal(err)
	}
}

func TestValidBranch(t *testing.T) {
	client := realClient()
	if !client.ValidBranch("feat/login") {
		t.Fatal("feat/login should be valid")
	}
	if client.ValidBranch("feat login") {
		t.Fatal("spaces should be invalid")
	}
}

// The parsing tests below use a scripted runner, so they cover output shapes
// that are awkward to produce with a real repository.

func TestWorktreeListParsesPorcelain(t *testing.T) {
	runner := testsupport.NewScriptedRunner()
	runner.Responses["worktree list --porcelain"] = "" +
		"worktree /src/app\nHEAD abc\nbranch refs/heads/main\n" +
		"\n" +
		"worktree /ws/feat/app\nHEAD def\nbranch refs/heads/feat/login\n" +
		"\n" +
		"worktree /ws/detached/app\nHEAD 123\ndetached\n"
	client := git.NewClient(runner, pathx.New())

	entries, err := client.WorktreeList("/src/app")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries = %#v", entries)
	}
	if entries[1].Path != "/ws/feat/app" || entries[1].Branch != "feat/login" {
		t.Fatalf("entry[1] = %#v", entries[1])
	}
	if entries[2].Branch != "" {
		t.Fatalf("detached worktree should have no branch: %#v", entries[2])
	}
}

func TestAheadBehindParsesCounts(t *testing.T) {
	runner := testsupport.NewScriptedRunner()
	runner.Responses["rev-list --left-right --count @{upstream}...HEAD"] = "3\t7"
	client := git.NewClient(runner, pathx.New())

	ahead, behind, err := client.AheadBehind("/ws/app")
	if err != nil {
		t.Fatal(err)
	}
	if ahead != 7 || behind != 3 {
		t.Fatalf("ahead=%d behind=%d, want 7/3", ahead, behind)
	}
}

func TestAheadBehindTreatsMissingUpstreamAsZero(t *testing.T) {
	runner := testsupport.NewScriptedRunner()
	key := "rev-list --left-right --count @{upstream}...HEAD"
	runner.Errors[key] = errors.New("no upstream")
	client := git.NewClient(runner, pathx.New())

	ahead, behind, err := client.AheadBehind("/ws/app")
	if err != nil || ahead != 0 || behind != 0 {
		t.Fatalf("got %d/%d err=%v, want 0/0 nil", ahead, behind, err)
	}
}

func TestAheadBehindRejectsUnparseableOutput(t *testing.T) {
	runner := testsupport.NewScriptedRunner()
	runner.Responses["rev-list --left-right --count @{upstream}...HEAD"] = "not numbers here"
	client := git.NewClient(runner, pathx.New())

	if _, _, err := client.AheadBehind("/ws/app"); err == nil {
		t.Fatal("expected a parse error")
	}
}

func TestDefaultBranchFallsBackThroughRefs(t *testing.T) {
	runner := testsupport.NewScriptedRunner()
	runner.Errors["symbolic-ref refs/remotes/origin/HEAD --short"] = errors.New("no origin/HEAD")
	runner.Errors["rev-parse --verify --quiet refs/remotes/origin/main"] = errors.New("missing")
	runner.Errors["rev-parse --verify --quiet refs/heads/main"] = errors.New("missing")
	runner.Errors["rev-parse --verify --quiet refs/remotes/origin/master"] = errors.New("missing")
	runner.Responses["rev-parse --verify --quiet refs/heads/master"] = "abc123"
	client := git.NewClient(runner, pathx.New())

	branch, err := client.DefaultBranch("/src/app")
	if err != nil {
		t.Fatal(err)
	}
	if branch != "master" {
		t.Fatalf("branch = %s, want master", branch)
	}
}

func TestDefaultBranchStripsRemotePrefix(t *testing.T) {
	runner := testsupport.NewScriptedRunner()
	runner.Responses["symbolic-ref refs/remotes/origin/HEAD --short"] = "origin/trunk"
	client := git.NewClient(runner, pathx.New())

	branch, err := client.DefaultBranch("/src/app")
	if err != nil || branch != "trunk" {
		t.Fatalf("branch = %q err = %v", branch, err)
	}
}

func TestStartPointPrefersRemote(t *testing.T) {
	runner := testsupport.NewScriptedRunner()
	runner.Responses["rev-parse --verify --quiet refs/remotes/origin/dev"] = "abc"
	client := git.NewClient(runner, pathx.New())

	start, err := client.StartPoint("/src/app", "dev")
	if err != nil || start != "origin/dev" {
		t.Fatalf("start = %q err = %v", start, err)
	}
}

func TestDirtyCountCountsLines(t *testing.T) {
	runner := testsupport.NewScriptedRunner()
	runner.Responses["status --porcelain"] = " M a.go\n?? b.go\n M c.go"
	client := git.NewClient(runner, pathx.New())

	dirty, err := client.DirtyCount("/ws/app")
	if err != nil || dirty != 3 {
		t.Fatalf("dirty = %d err = %v", dirty, err)
	}
}

func TestCommandErrorMessage(t *testing.T) {
	err := &git.CommandError{Cmd: "worktree add", Output: "fatal: boom"}
	if got := err.Error(); got != "git worktree add: fatal: boom" {
		t.Fatalf("Error() = %q", got)
	}
}
