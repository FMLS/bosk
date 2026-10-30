package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/FMLS/bosk/internal/testsupport"
)

// setupSharedCLI is setupCLI plus a third repository, "lib", to link in.
func setupSharedCLI(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks needs developer mode on Windows")
	}
	src := setupCLI(t)
	lib := filepath.Join(src, "lib")
	testsupport.InitRepo(t, lib)
	if err := os.WriteFile(filepath.Join(lib, "keep.txt"), []byte("precious\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return lib
}

func workspaceDir(t *testing.T, name string) string {
	t.Helper()
	out, _, err := runCLI(t, "go", name)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out)
}

func TestCreateWithSharedStatusRemoveAndDelete(t *testing.T) {
	lib := setupSharedCLI(t)

	out, _, err := runCLI(t, "create", "-b", "feat/login", "-r", "svc-a", "--shared", "lib")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "lib") || !strings.Contains(out, "(shared)") {
		t.Fatalf("create output = %q", out)
	}
	ws := workspaceDir(t, "feat-login")
	if target, err := os.Readlink(filepath.Join(ws, "lib")); err != nil || target != lib {
		t.Fatalf("link = %q, %v; want %s", target, err, lib)
	}

	out, _, err = runCLI(t, "list")
	if err != nil || !strings.Contains(out, "SHARED") {
		t.Fatalf("list output = %q, %v", out, err)
	}

	out, _, err = runCLI(t, "status", "feat-login")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "SHARED") || !strings.Contains(out, "ok") {
		t.Fatalf("status output = %q", out)
	}
	out, _, err = runCLI(t, "status", "feat-login", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var status struct {
		Shared []struct {
			Repo  string `json:"repo"`
			Valid bool   `json:"valid"`
		} `json:"shared"`
	}
	if err := json.Unmarshal([]byte(out), &status); err != nil {
		t.Fatalf("status JSON: %v\n%s", err, out)
	}
	if len(status.Shared) != 1 || status.Shared[0].Repo != "lib" || !status.Shared[0].Valid {
		t.Fatalf("status shared = %#v", status.Shared)
	}

	out, _, err = runCLI(t, "remove", "feat-login", "-r", "lib", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "lib (shared)") {
		t.Fatalf("remove output = %q", out)
	}
	if _, err := os.Lstat(filepath.Join(ws, "lib")); !os.IsNotExist(err) {
		t.Errorf("link should be gone: %v", err)
	}

	if _, _, err := runCLI(t, "add", "feat-login", "--shared", "lib"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(ws, "lib", "keep.txt")); err != nil {
		t.Fatalf("add --shared should link the source: %v", err)
	}
	if _, _, err := runCLI(t, "delete", "feat-login", "--yes"); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(lib, "keep.txt")); err != nil || string(data) != "precious\n" {
		t.Fatalf("delete damaged the shared source: %q, %v", data, err)
	}
}

func TestCreateSharedFlagErrors(t *testing.T) {
	setupSharedCLI(t)

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"only shared", []string{"create", "-b", "feat/x", "--shared", "lib"}, "at least one worktree repository"},
		{"both ways", []string{"create", "-b", "feat/x", "-r", "svc-a", "--shared", "svc-a"}, "both a worktree repository and a shared repository"},
		{"unknown", []string{"create", "-b", "feat/x", "-r", "svc-a", "--shared", "nope"}, "unknown repositories: nope"},
	}
	for _, tc := range cases {
		_, _, err := runCLI(t, tc.args...)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error = %v, want %q", tc.name, err, tc.want)
		}
	}
}

func TestAddNeedsAtLeastOneSelection(t *testing.T) {
	setupSharedCLI(t)
	if _, _, err := runCLI(t, "create", "task-1", "-b", "feat/x", "-r", "svc-a"); err != nil {
		t.Fatal(err)
	}

	_, _, err := runCLI(t, "add", "task-1")

	if err == nil || !strings.Contains(err.Error(), "--repos, --all or --shared") {
		t.Fatalf("error = %v", err)
	}
}

func TestRemoveRefusesToEmptyTheWorktreeRepositories(t *testing.T) {
	setupSharedCLI(t)
	if _, _, err := runCLI(t, "create", "task-1", "-b", "feat/x", "-r", "svc-a", "--shared", "lib"); err != nil {
		t.Fatal(err)
	}

	_, _, err := runCLI(t, "remove", "task-1", "-r", "svc-a", "--yes")

	if err == nil || !strings.Contains(err.Error(), "cannot remove every repository") {
		t.Fatalf("error = %v", err)
	}
}
