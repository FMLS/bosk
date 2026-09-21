package state

import (
	"path/filepath"
	"testing"

	"github.com/FMLS/bosk/internal/models"
)

func TestSaveLoadAndByPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BOSK_HOME", home)
	store := New()

	var saved File
	ws := models.NewWorkspace("feat-login", filepath.Join(home, "workspaces", "feat-login"), "feat/login")
	ws.Repos = []models.RepoWorktree{{RepoName: "svc-a"}}
	saved.Upsert(ws)
	if err := store.Save(&saved); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	got, ok := loaded.Get("feat-login")
	if !ok || got.Branch != "feat/login" {
		t.Fatalf("got %#v", got)
	}

	inside := filepath.Join(ws.Path, "svc-a")
	found, ok := loaded.ByPath(inside)
	if !ok || found.Name != "feat-login" {
		t.Fatalf("ByPath = %#v ok=%v", found, ok)
	}
}
