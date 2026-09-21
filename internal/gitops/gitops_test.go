package gitops

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/FMLS/bosk/internal/testgit"
)

func TestWorktreeAddAndStatus(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "app")
	testgit.Init(t, repo)

	if !IsGitRepo(repo) {
		t.Fatal("expected git repo")
	}
	branch, err := DefaultBranch(repo)
	if err != nil {
		t.Fatal(err)
	}
	if branch != "main" {
		t.Fatalf("default branch = %s", branch)
	}

	wt := filepath.Join(root, "feat")
	if err := WorktreeAddNew(repo, wt, "feat/login", "main"); err != nil {
		t.Fatal(err)
	}
	if !BranchExists(repo, "feat/login") {
		t.Fatal("branch should exist")
	}
	inUse, err := BranchInUse(repo, "feat/login")
	if err != nil {
		t.Fatal(err)
	}
	if !inUse {
		t.Fatal("branch should be in use")
	}
	cur, err := CurrentBranch(wt)
	if err != nil {
		t.Fatal(err)
	}
	if cur != "feat/login" {
		t.Fatalf("current branch = %s", cur)
	}
	dirty, err := DirtyCount(wt)
	if err != nil {
		t.Fatal(err)
	}
	if dirty != 0 {
		t.Fatalf("dirty = %d", dirty)
	}

	if err := os.WriteFile(filepath.Join(wt, "README"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirty, err = DirtyCount(wt)
	if err != nil {
		t.Fatal(err)
	}
	if dirty == 0 {
		t.Fatal("expected dirty worktree")
	}

	if err := WorktreeRemove(repo, wt, true); err != nil {
		t.Fatal(err)
	}
	if err := DeleteBranch(repo, "feat/login", true); err != nil {
		t.Fatal(err)
	}
}

func TestValidBranch(t *testing.T) {
	if !ValidBranch("feat/login") {
		t.Fatal("feat/login should be valid")
	}
	if ValidBranch("feat login") {
		t.Fatal("spaces should be invalid")
	}
}
