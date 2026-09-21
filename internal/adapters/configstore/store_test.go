package configstore_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/FMLS/bosk/internal/adapters/configstore"
	"github.com/FMLS/bosk/internal/domain"
)

func TestLoadMissingConfigReturnsNil(t *testing.T) {
	store := configstore.NewStore(t.TempDir())

	cfg, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg != nil {
		t.Fatalf("cfg = %#v, want nil", cfg)
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	home := t.TempDir()
	store := configstore.NewStore(home)
	want := &domain.Config{RepoDirs: []string{"/src/one", "/src/two"}, WorkspaceDir: "/ws"}

	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.RepoDirs) != 2 || got.RepoDirs[1] != "/src/two" || got.WorkspaceDir != "/ws" {
		t.Fatalf("got %#v", got)
	}
}

func TestLoadFillsInDefaultWorkspaceDir(t *testing.T) {
	home := t.TempDir()
	store := configstore.NewStore(home)
	if err := os.WriteFile(store.Path(), []byte("repo_dirs:\n  - /src\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WorkspaceDir != store.DefaultWorkspaceDir() {
		t.Fatalf("workspace dir = %s, want %s", cfg.WorkspaceDir, store.DefaultWorkspaceDir())
	}
}

func TestLoadExpandsHomeAndDropsBlanks(t *testing.T) {
	home := t.TempDir()
	store := configstore.NewStore(home)
	if err := os.WriteFile(store.Path(), []byte("repo_dirs:\n  - ~/code\n  - \"\"\n  - /src\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no user home directory")
	}

	cfg, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(userHome, "code"), "/src"}
	if len(cfg.RepoDirs) != len(want) {
		t.Fatalf("repo dirs = %v, want %v", cfg.RepoDirs, want)
	}
	for i := range want {
		if cfg.RepoDirs[i] != want[i] {
			t.Fatalf("repo dirs = %v, want %v", cfg.RepoDirs, want)
		}
	}
}

func TestLoadRejectsBrokenYAML(t *testing.T) {
	home := t.TempDir()
	store := configstore.NewStore(home)
	if err := os.WriteFile(store.Path(), []byte("repo_dirs: [unterminated\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Load(); err == nil {
		t.Fatal("expected a parse error")
	}
}

func TestDefaultHomeHonoursTheEnvironment(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(configstore.HomeEnv, dir)

	if got := configstore.DefaultHome(); got != dir {
		t.Fatalf("DefaultHome = %s, want %s", got, dir)
	}
}

func TestPathsLiveUnderHome(t *testing.T) {
	home := t.TempDir()
	store := configstore.NewStore(home)

	cases := map[string]string{
		store.Path():                filepath.Join(home, "config.yaml"),
		store.StatePath():           filepath.Join(home, "state.json"),
		store.LockPath():            filepath.Join(home, "state.lock"),
		store.DefaultWorkspaceDir(): filepath.Join(home, "workspaces"),
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got %s, want %s", got, want)
		}
	}
	if store.Home() != home {
		t.Errorf("Home = %s, want %s", store.Home(), home)
	}
}
