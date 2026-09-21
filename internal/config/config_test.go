package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitLoadSave(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BOSK_HOME", home)
	repoDir := filepath.Join(t.TempDir(), "src")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg, err := Init([]string{repoDir})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.RepoDirs) != 1 {
		t.Fatalf("repo dirs = %v", cfg.RepoDirs)
	}
	if cfg.WorkspaceDir != DefaultWorkspaceDir() {
		t.Fatalf("workspace dir = %s", cfg.WorkspaceDir)
	}

	loaded, err := Require()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RepoDirs[0] != cfg.RepoDirs[0] {
		t.Fatalf("loaded %#v", loaded)
	}

	extra := filepath.Join(t.TempDir(), "more")
	if err := os.MkdirAll(extra, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg2, err := Init([]string{repoDir, extra})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg2.RepoDirs) != 2 {
		t.Fatalf("merged repo dirs = %v", cfg2.RepoDirs)
	}
}

func TestRequireMissing(t *testing.T) {
	t.Setenv("BOSK_HOME", t.TempDir())
	if _, err := Require(); err == nil {
		t.Fatal("expected error")
	}
}
