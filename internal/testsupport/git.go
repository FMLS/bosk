// Package testsupport provides the fixtures and fakes bosk's tests share.
package testsupport

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// InitRepo creates a git repository with an initial commit on main.
func InitRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	RunGit(t, dir, "init", "-b", "main")
	RunGit(t, dir, "config", "user.email", "bosk@example.com")
	RunGit(t, dir, "config", "user.name", "bosk")
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte(filepath.Base(dir)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	RunGit(t, dir, "add", "README")
	RunGit(t, dir, "commit", "-m", "init")
}

// RunGit runs git in dir and returns stdout+stderr, failing the test on error.
func RunGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}
