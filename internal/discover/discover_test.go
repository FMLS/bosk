package discover

import (
	"path/filepath"
	"testing"

	"github.com/FMLS/bosk/internal/testgit"
)

func TestReposFindsNestedGit(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "svc-a")
	b := filepath.Join(root, "team", "svc-b")
	nested := filepath.Join(root, "svc-a", "vendor-src")
	testgit.Init(t, a)
	testgit.Init(t, b)
	testgit.Init(t, nested)

	repos, skipped := Repos([]string{root})
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

func TestByName(t *testing.T) {
	root := t.TempDir()
	testgit.Init(t, filepath.Join(root, "svc-a"))
	testgit.Init(t, filepath.Join(root, "svc-b"))
	repos, _ := Repos([]string{root})
	selected, err := ByName(repos, []string{"svc-b", "svc-a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 2 || selected[0].Name != "svc-b" {
		t.Fatalf("order = %v", selected)
	}
	if _, err := ByName(repos, []string{"missing"}); err == nil {
		t.Fatal("expected missing error")
	}
}
