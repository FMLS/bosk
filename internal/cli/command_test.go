package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FMLS/bosk/internal/adapters/configstore"
	"github.com/FMLS/bosk/internal/testsupport"
)

// runCLI drives the real command tree, so these tests cover the wiring in
// container.go as well as the commands themselves.
func runCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	root := newRoot()
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), errOut.String(), err
}

// setupCLI points bosk at a temporary home holding two source repositories.
func setupCLI(t *testing.T) string {
	t.Helper()
	t.Setenv(configstore.HomeEnv, t.TempDir())
	src := t.TempDir()
	testsupport.InitRepo(t, filepath.Join(src, "svc-a"))
	testsupport.InitRepo(t, filepath.Join(src, "svc-b"))
	if _, _, err := runCLI(t, "init", src); err != nil {
		t.Fatal(err)
	}
	return src
}

func TestCommandsWithoutConfigurationExplainInit(t *testing.T) {
	t.Setenv(configstore.HomeEnv, t.TempDir())

	for _, args := range [][]string{{"list"}, {"repos"}, {"status"}, {"doctor"}, {"go"}} {
		_, _, err := runCLI(t, args...)
		if err == nil || !strings.Contains(err.Error(), "bosk init <repo-dir>") {
			t.Errorf("bosk %v: error = %v", args, err)
		}
	}
}

func TestInitReportsWhatItFound(t *testing.T) {
	t.Setenv(configstore.HomeEnv, t.TempDir())
	src := t.TempDir()
	testsupport.InitRepo(t, filepath.Join(src, "svc-a"))

	out, _, err := runCLI(t, "init", src)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "discovered 1 repositories") || !strings.Contains(out, "svc-a") {
		t.Fatalf("output = %q", out)
	}
}

func TestCreateListStatusGoDelete(t *testing.T) {
	setupCLI(t)

	out, _, err := runCLI(t, "create", "-b", "feat/login", "-r", "svc-a,svc-b")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "created workspace feat-login") {
		t.Fatalf("create output = %q", out)
	}

	out, _, err = runCLI(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "feat-login") || !strings.Contains(out, "NAME") {
		t.Fatalf("list output = %q", out)
	}

	out, _, err = runCLI(t, "status", "feat-login", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var status struct {
		Name  string `json:"name"`
		Repos []struct {
			Repo   string `json:"repo"`
			Branch string `json:"branch"`
		} `json:"repos"`
	}
	if err := json.Unmarshal([]byte(out), &status); err != nil {
		t.Fatalf("status JSON: %v\n%s", err, out)
	}
	if status.Name != "feat-login" || len(status.Repos) != 2 || status.Repos[0].Branch != "feat/login" {
		t.Fatalf("status = %#v", status)
	}

	out, _, err = runCLI(t, "go", "feat-login")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(strings.TrimSpace(out), filepath.Join("workspaces", "feat-login")) {
		t.Fatalf("go output = %q", out)
	}

	out, _, err = runCLI(t, "delete", "feat-login", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "deleted workspace feat-login") {
		t.Fatalf("delete output = %q", out)
	}

	out, _, err = runCLI(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no workspaces") {
		t.Fatalf("list output = %q", out)
	}
}

func TestReposListsDiscoveredSources(t *testing.T) {
	setupCLI(t)

	out, _, err := runCLI(t, "repos", "--json")
	if err != nil {
		t.Fatal(err)
	}

	var repos []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(out), &repos); err != nil {
		t.Fatalf("repos JSON: %v\n%s", err, out)
	}
	if len(repos) != 2 {
		t.Fatalf("repos = %#v", repos)
	}
}

func TestDoctorExitsNonZeroForHumansAndZeroForJSON(t *testing.T) {
	setupCLI(t)
	if _, _, err := runCLI(t, "create", "task-1", "-b", "feat/x", "-r", "svc-a"); err != nil {
		t.Fatal(err)
	}
	home := configstore.DefaultHome()
	if err := os.RemoveAll(filepath.Join(home, "workspaces", "task-1")); err != nil {
		t.Fatal(err)
	}

	_, _, err := runCLI(t, "doctor")
	if err == nil || !strings.Contains(err.Error(), "doctor found") {
		t.Fatalf("text mode should fail: %v", err)
	}

	out, _, err := runCLI(t, "doctor", "--json")
	if err != nil {
		t.Fatalf("JSON mode should put findings in the payload, not the exit code: %v", err)
	}
	if !strings.Contains(out, `"kind"`) {
		t.Fatalf("doctor JSON = %q", out)
	}

	out, _, err = runCLI(t, "doctor", "--fix")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "all workspaces healthy") {
		t.Fatalf("doctor --fix output = %q", out)
	}
}

func TestDeleteWithoutYesAbortsOnAnEmptyAnswer(t *testing.T) {
	setupCLI(t)
	if _, _, err := runCLI(t, "create", "task-1", "-b", "feat/x", "-r", "svc-a"); err != nil {
		t.Fatal(err)
	}

	_, _, err := runCLI(t, "delete", "task-1")

	if err == nil || !strings.Contains(err.Error(), "aborted") {
		t.Fatalf("error = %v", err)
	}
	out, _, err := runCLI(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "task-1") {
		t.Fatal("an aborted delete must leave the workspace in place")
	}
}

func TestShellInitPrintsTheWrapper(t *testing.T) {
	out, _, err := runCLI(t, "shell-init")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "command bosk go") {
		t.Fatalf("shell-init output = %q", out)
	}
}

func TestCreateRejectsConflictingSelectionFlags(t *testing.T) {
	setupCLI(t)

	_, _, err := runCLI(t, "create", "-b", "feat/x", "-r", "svc-a", "--all")

	if err == nil || !strings.Contains(err.Error(), "not both") {
		t.Fatalf("error = %v", err)
	}
}
