package discovery_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/FMLS/bosk/internal/adapters/discovery"
	"github.com/FMLS/bosk/internal/adapters/fsys"
	"github.com/FMLS/bosk/internal/adapters/git"
	"github.com/FMLS/bosk/internal/adapters/pathx"
	"github.com/FMLS/bosk/internal/testsupport"
)

func newScanner() *discovery.Scanner {
	paths := pathx.New()
	client := git.NewClient(git.NewExecRunner(), paths)
	return discovery.NewScanner(client, fsys.New(), paths)
}

func TestDiscoverFindsNestedRepos(t *testing.T) {
	root := t.TempDir()
	testsupport.InitRepo(t, filepath.Join(root, "svc-a"))
	testsupport.InitRepo(t, filepath.Join(root, "team", "svc-b"))
	testsupport.InitRepo(t, filepath.Join(root, "svc-a", "vendor-src"))

	repos, skipped := newScanner().Discover([]string{root})

	if len(skipped) != 0 {
		t.Fatalf("skipped = %v", skipped)
	}
	names := map[string]bool{}
	for _, r := range repos {
		names[r.Name] = true
	}
	if !names["svc-a"] || !names["svc-b"] {
		t.Fatalf("repos = %v", repos)
	}
	if names["vendor-src"] {
		t.Fatal("should not descend into a git repository")
	}
}

func TestDiscoverSkipsDuplicateNames(t *testing.T) {
	root := t.TempDir()
	testsupport.InitRepo(t, filepath.Join(root, "svc-a"))
	testsupport.InitRepo(t, filepath.Join(root, "team", "svc-a"))

	repos, skipped := newScanner().Discover([]string{root})

	if len(repos) != 1 || repos[0].Name != "svc-a" {
		t.Fatalf("repos = %v", repos)
	}
	if len(skipped) != 1 {
		t.Fatalf("skipped = %v", skipped)
	}
}

func TestDiscoverIgnoresHiddenAndVendorDirs(t *testing.T) {
	root := t.TempDir()
	testsupport.InitRepo(t, filepath.Join(root, "node_modules", "hidden-a"))
	testsupport.InitRepo(t, filepath.Join(root, ".cache", "hidden-b"))
	testsupport.InitRepo(t, filepath.Join(root, "visible"))

	repos, _ := newScanner().Discover([]string{root})

	if len(repos) != 1 || repos[0].Name != "visible" {
		t.Fatalf("repos = %v", repos)
	}
}

func TestDiscoverDoesNotFollowSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks needs developer mode on Windows")
	}
	root := t.TempDir()
	real := t.TempDir()
	testsupport.InitRepo(t, filepath.Join(real, "linked"))
	if err := os.Symlink(filepath.Join(real, "linked"), filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	testsupport.InitRepo(t, filepath.Join(root, "plain"))

	repos, _ := newScanner().Discover([]string{root})

	if len(repos) != 1 || repos[0].Name != "plain" {
		t.Fatalf("repos = %v, want a symlink (such as a workspace's shared link) left out", repos)
	}
}
