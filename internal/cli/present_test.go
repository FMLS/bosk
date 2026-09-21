package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/FMLS/bosk/internal/domain"
	"github.com/spf13/cobra"
)

func newCapturedCommand() (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	cmd := &cobra.Command{}
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	return cmd, out, errOut
}

func TestTextPresenterRendersAnIssueTable(t *testing.T) {
	cmd, out, _ := newCapturedCommand()
	issues := []domain.Issue{
		{Kind: domain.IssueWorkspaceDirMissing, Workspace: "task-1", Path: "/ws/task-1"},
		{Kind: domain.IssueSourceRepoMissing, Workspace: "task-1", Repo: "svc-a", Path: "/src/svc-a"},
	}

	if err := presenterFor(cmd, false).Issues(issues); err != nil {
		t.Fatal(err)
	}

	got := out.String()
	for _, want := range []string{
		"WORKSPACE", "ISSUE", "ACTION",
		"workspace directory missing: /ws/task-1",
		"bosk delete --force task-1",
		"source repository missing: /src/svc-a",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
}

func TestTextPresenterReportsAHealthyRegistry(t *testing.T) {
	cmd, out, _ := newCapturedCommand()

	if err := presenterFor(cmd, false).Issues(nil); err != nil {
		t.Fatal(err)
	}

	if got := strings.TrimSpace(out.String()); got != "all workspaces healthy" {
		t.Fatalf("output = %q", got)
	}
}

func TestJSONPresenterCarriesTheIssueKind(t *testing.T) {
	cmd, out, _ := newCapturedCommand()
	issues := []domain.Issue{
		{Kind: domain.IssueTrashOrphaned, Workspace: "gone", Path: "/ws/.trash/gone/t"},
	}

	if err := presenterFor(cmd, true).Issues(issues); err != nil {
		t.Fatal(err)
	}

	var decoded []map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if len(decoded) != 1 {
		t.Fatalf("decoded = %#v", decoded)
	}
	row := decoded[0]
	if row["kind"] != string(domain.IssueTrashOrphaned) {
		t.Errorf("kind = %v", row["kind"])
	}
	if row["issue"] != "leftover trash: /ws/.trash/gone/t" {
		t.Errorf("issue = %v", row["issue"])
	}
	if row["fixable"] != true {
		t.Errorf("fixable = %v", row["fixable"])
	}
}

func TestPresenterMachineReadability(t *testing.T) {
	cmd, _, _ := newCapturedCommand()

	if presenterFor(cmd, false).MachineReadable() {
		t.Error("the table presenter is for humans")
	}
	if !presenterFor(cmd, true).MachineReadable() {
		t.Error("the JSON presenter is for programs")
	}
}

func TestTextPresenterOnEmptyCollections(t *testing.T) {
	cmd, out, _ := newCapturedCommand()
	presenter := presenterFor(cmd, false)

	if err := presenter.Workspaces(nil); err != nil {
		t.Fatal(err)
	}
	if err := presenter.Repos(nil); err != nil {
		t.Fatal(err)
	}

	got := out.String()
	if !strings.Contains(got, "no workspaces") || !strings.Contains(got, "no repositories found") {
		t.Fatalf("output = %q", got)
	}
}

func TestSplitCSV(t *testing.T) {
	cases := map[string][]string{
		"":                nil,
		"   ":             nil,
		"a":               {"a"},
		"a,b":             {"a", "b"},
		" a , b ,, c ":    {"a", "b", "c"},
		"trailing,comma,": {"trailing", "comma"},
	}
	for input, want := range cases {
		got := splitCSV(input)
		if len(got) != len(want) {
			t.Errorf("splitCSV(%q) = %v, want %v", input, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("splitCSV(%q) = %v, want %v", input, got, want)
				break
			}
		}
	}
}

func TestFirstArg(t *testing.T) {
	if got := firstArg(nil); got != "" {
		t.Errorf("firstArg(nil) = %q", got)
	}
	if got := firstArg([]string{"name"}); got != "name" {
		t.Errorf("firstArg = %q", got)
	}
}
