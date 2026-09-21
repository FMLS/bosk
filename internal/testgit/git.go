package testgit

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Init creates a git repository with an initial commit on main.
func Init(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "init", "-b", "main")
	run(t, dir, "git", "config", "user.email", "bosk@example.com")
	run(t, dir, "git", "config", "user.name", "bosk")
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte(filepath.Base(dir)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "add", "README")
	run(t, dir, "git", "commit", "-m", "init")
}

// Run runs git in dir and returns stdout+stderr.
func Run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return run(t, dir, "git", args...)
}

func run(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
	return string(out)
}
