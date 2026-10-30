package app_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/FMLS/bosk/internal/app"
	"github.com/FMLS/bosk/internal/domain"
	"github.com/FMLS/bosk/internal/testsupport"
)

// sharedRealHarness adds a third repository, "lib", with a file in it so the
// tests can tell whether anything touched the source.
func sharedRealHarness(t *testing.T) (*realHarness, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks needs developer mode on Windows")
	}
	h := newRealHarness(t)
	lib := filepath.Join(filepath.Dir(h.srcA), "lib")
	testsupport.InitRepo(t, lib)
	if err := os.WriteFile(filepath.Join(lib, "keep.txt"), []byte("precious\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return h, lib
}

func assertSourceIntact(t *testing.T, lib string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(lib, "keep.txt"))
	if err != nil || string(data) != "precious\n" {
		t.Fatalf("shared source repository was damaged: %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(lib, ".git")); err != nil {
		t.Fatalf("shared source repository lost its .git: %v", err)
	}
}

func TestIntegrationCreateWithSharedThenDeleteKeepsTheSource(t *testing.T) {
	h, lib := sharedRealHarness(t)

	ws, err := h.creator.Create(app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}, SharedNames: []string{"lib"}})
	if err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(ws.Path, "lib")
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected %s to be a symlink: %v, %v", link, info, err)
	}
	if target, _ := os.Readlink(link); target != lib {
		t.Fatalf("link target = %s, want %s", target, lib)
	}
	if _, err := os.Stat(filepath.Join(link, "keep.txt")); err != nil {
		t.Fatalf("the link should reach the source repository: %v", err)
	}
	if rows := h.inspector.SharedStatus(ws); len(rows) != 1 || !rows[0].Valid {
		t.Fatalf("shared status = %#v", rows)
	}
	if issues, err := h.doctor.Diagnose(); err != nil || len(issues) != 0 {
		t.Fatalf("doctor = %#v, %v", issues, err)
	}

	if err := h.remover.Delete(ws.Name, app.DeleteOpts{}); err != nil {
		t.Fatal(err)
	}

	assertSourceIntact(t, lib)
	if _, err := os.Lstat(ws.Path); !os.IsNotExist(err) {
		t.Errorf("workspace directory should be gone: %v", err)
	}
	if issues, err := h.doctor.Diagnose(); err != nil || len(issues) != 0 {
		t.Errorf("no trash should be left behind: %#v, %v", issues, err)
	}
}

func TestIntegrationRemoveSharedKeepsTheSource(t *testing.T) {
	h, lib := sharedRealHarness(t)
	ws, err := h.creator.Create(app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}, SharedNames: []string{"lib"}})
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := h.remover.RemoveRepos(ws.Name, []string{"lib"}, app.DeleteOpts{}); err != nil {
		t.Fatal(err)
	}

	assertSourceIntact(t, lib)
	if _, err := os.Lstat(filepath.Join(ws.Path, "lib")); !os.IsNotExist(err) {
		t.Errorf("link should be gone: %v", err)
	}
}

func TestIntegrationAddSharedToAnExistingWorkspace(t *testing.T) {
	h, lib := sharedRealHarness(t)
	ws := mustCreateReal(t, h, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}})

	ws, added, err := h.creator.Add(app.AddOpts{Name: ws.Name, SharedNames: []string{"lib"}})
	if err != nil {
		t.Fatal(err)
	}

	if len(added.Shared) != 1 || len(ws.Shared) != 1 {
		t.Fatalf("added = %#v, ws = %#v", added, ws)
	}
	if target, _ := os.Readlink(filepath.Join(ws.Path, "lib")); target != lib {
		t.Errorf("link target = %s", target)
	}
}

func TestIntegrationDoctorRepairsAMovedSharedSourceOnceRestored(t *testing.T) {
	h, lib := sharedRealHarness(t)
	ws := mustCreateReal(t, h, app.CreateOpts{Branch: "feat/login", RepoNames: []string{"svc-a"}, SharedNames: []string{"lib"}})
	moved := lib + ".moved"
	if err := os.Rename(lib, moved); err != nil {
		t.Fatal(err)
	}

	issues, err := h.doctor.Diagnose()
	if err != nil {
		t.Fatal(err)
	}
	if issue, ok := findIssue(issues, domain.IssueSharedSourceMissing); !ok || issue.Fixable() {
		t.Fatalf("issues = %#v", issues)
	}
	if err := h.repairer.Repair(issues); err != nil {
		t.Fatal(err)
	}
	if _, err := h.inspector.Get(ws.Name); err != nil {
		t.Fatalf("repair must not delete the workspace: %v", err)
	}

	if err := os.Rename(moved, lib); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(ws.Path, "lib")); err != nil {
		t.Fatal(err)
	}
	issues, _ = h.doctor.Diagnose()
	if _, ok := findIssue(issues, domain.IssueSharedLinkMissing); !ok {
		t.Fatalf("issues = %#v", issues)
	}
	if err := h.repairer.Repair(issues); err != nil {
		t.Fatal(err)
	}
	if issues, _ := h.doctor.Diagnose(); len(issues) != 0 {
		t.Fatalf("issues after repair = %#v", issues)
	}
}

func mustCreateReal(t *testing.T, h *realHarness, opts app.CreateOpts) domain.Workspace {
	t.Helper()
	ws, err := h.creator.Create(opts)
	if err != nil {
		t.Fatal(err)
	}
	return ws
}
