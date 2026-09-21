package registry_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FMLS/bosk/internal/adapters/registry"
	"github.com/FMLS/bosk/internal/domain"
)

func newStore(t *testing.T) (*registry.Store, string) {
	t.Helper()
	home := t.TempDir()
	return registry.NewStore(filepath.Join(home, "state.json"), filepath.Join(home, "state.lock")), home
}

func TestSaveThenLoad(t *testing.T) {
	store, home := newStore(t)

	var reg domain.Registry
	ws := domain.NewWorkspace("feat-login", filepath.Join(home, "workspaces", "feat-login"), "feat/login", time.Now())
	ws.Repos = []domain.RepoWorktree{{RepoName: "svc-a", Branch: "feat/login"}}
	reg.Upsert(ws)
	if err := store.Save(&reg); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	got, ok := loaded.Get("feat-login")
	if !ok || got.Branch != "feat/login" || len(got.Repos) != 1 {
		t.Fatalf("got %#v ok=%v", got, ok)
	}
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	store, _ := newStore(t)

	reg, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(reg.Workspaces) != 0 {
		t.Fatalf("workspaces = %v", reg.Workspaces)
	}
}

func TestLoadEmptyFileIsEmpty(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.json")
	if err := os.WriteFile(path, []byte("  \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := registry.NewStore(path, filepath.Join(home, "state.lock"))

	reg, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(reg.Workspaces) != 0 {
		t.Fatalf("workspaces = %v", reg.Workspaces)
	}
}

func TestLoadCorruptFileExplainsTheFix(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := registry.NewStore(path, filepath.Join(home, "state.lock"))

	_, err := store.Load()
	if err == nil {
		t.Fatal("expected an error")
	}
	if want := "bosk doctor --fix"; !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want it to mention %q", err, want)
	}
}

func TestWithLockRunsTheFunction(t *testing.T) {
	store, _ := newStore(t)

	ran := false
	if err := store.WithLock(func() error {
		ran = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Fatal("fn should have run under the lock")
	}
}
