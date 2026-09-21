package app_test

import (
	"strings"
	"testing"

	"github.com/FMLS/bosk/internal/app"
	"github.com/FMLS/bosk/internal/domain"
	"github.com/FMLS/bosk/internal/testsupport"
)

func newInitializer(t *testing.T) (*app.Initializer, *testsupport.MemConfigStore, *testsupport.MemFS) {
	t.Helper()
	fs := testsupport.NewMemFS("/home/user")
	fs.AddDir("/src")
	fs.AddDir("/more")
	store := &testsupport.MemConfigStore{HomeDir: "/home/user/.bosk"}
	disc := testsupport.FakeDiscoverer{Repos: []domain.Repo{{Name: "svc-a", Path: "/src/svc-a"}}}
	return app.NewInitializer(store, disc, fs, testsupport.CleanResolver{}), store, fs
}

func TestInitRegistersARootAndCreatesTheDataDirectories(t *testing.T) {
	init, store, fs := newInitializer(t)

	res, err := init.Init([]string{"/src"})
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Config.RepoDirs) != 1 || res.Config.RepoDirs[0] != "/src" {
		t.Fatalf("repo dirs = %v", res.Config.RepoDirs)
	}
	if res.Config.WorkspaceDir != store.DefaultWorkspaceDir() {
		t.Fatalf("workspace dir = %s", res.Config.WorkspaceDir)
	}
	if res.ConfigPath != store.Path() {
		t.Fatalf("config path = %s", res.ConfigPath)
	}
	if len(res.Repos) != 1 {
		t.Fatalf("repos = %v", res.Repos)
	}
	if ok, _ := fs.IsDir(store.Home()); !ok {
		t.Error("bosk home should exist")
	}
	if ok, _ := fs.IsDir(res.Config.WorkspaceDir); !ok {
		t.Error("workspace dir should exist")
	}
	if store.SavedConfigs != 1 {
		t.Errorf("saves = %d, want 1", store.SavedConfigs)
	}
}

func TestInitMergesRootsWithoutDuplicating(t *testing.T) {
	init, _, _ := newInitializer(t)
	if _, err := init.Init([]string{"/src"}); err != nil {
		t.Fatal(err)
	}

	res, err := init.Init([]string{"/src", "/more"})
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Config.RepoDirs) != 2 {
		t.Fatalf("repo dirs = %v", res.Config.RepoDirs)
	}
}

func TestInitRejectsBadRoots(t *testing.T) {
	cases := []struct {
		name string
		dirs []string
		want string
	}{
		{"no directories", nil, "at least one repository directory is required"},
		{"missing directory", []string{"/nope"}, "directory does not exist"},
		{"a file, not a directory", []string{"/src/file.txt"}, "not a directory"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			init, store, fs := newInitializer(t)
			fs.AddFile("/src/file.txt")

			_, err := init.Init(tc.dirs)

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want it to mention %q", err, tc.want)
			}
			if store.SavedConfigs != 0 {
				t.Error("a rejected init must not write config")
			}
		})
	}
}

func TestRequireConfigExplainsHowToInitialize(t *testing.T) {
	store := &testsupport.MemConfigStore{HomeDir: "/home/user/.bosk"}

	_, err := app.RequireConfig(store)

	if err == nil || !strings.Contains(err.Error(), "bosk init <repo-dir>") {
		t.Fatalf("error = %v", err)
	}
}

func TestRequireConfigRejectsAConfigWithoutRoots(t *testing.T) {
	store := &testsupport.MemConfigStore{HomeDir: "/home/user/.bosk", Config: &domain.Config{WorkspaceDir: "/ws"}}

	_, err := app.RequireConfig(store)

	if err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("error = %v", err)
	}
}

func TestRequireConfigReturnsTheConfig(t *testing.T) {
	store := &testsupport.MemConfigStore{
		HomeDir: "/home/user/.bosk",
		Config:  &domain.Config{RepoDirs: []string{"/src"}, WorkspaceDir: "/ws"},
	}

	cfg, err := app.RequireConfig(store)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WorkspaceDir != "/ws" {
		t.Fatalf("cfg = %#v", cfg)
	}
}
