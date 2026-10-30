package fsys

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func linkFixture(t *testing.T) (dir, source, link string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks needs developer mode on Windows")
	}
	dir = t.TempDir()
	source = filepath.Join(dir, "source")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "keep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, source, filepath.Join(dir, "link")
}

func TestSymlinkAndReadlink(t *testing.T) {
	_, source, link := linkFixture(t)
	fs := New()

	if err := fs.Symlink(source, link); err != nil {
		t.Fatal(err)
	}

	got, err := fs.Readlink(link)
	if err != nil || got != source {
		t.Fatalf("Readlink = %q, %v", got, err)
	}
	if err := fs.Symlink(source, link); err == nil {
		t.Error("linking over an existing path should fail")
	}
}

func TestLinkExistsSeesADanglingLinkExistsDoesNot(t *testing.T) {
	dir, _, link := linkFixture(t)
	fs := New()
	if err := os.Symlink(filepath.Join(dir, "gone"), link); err != nil {
		t.Fatal(err)
	}

	if ok, err := fs.LinkExists(link); err != nil || !ok {
		t.Errorf("LinkExists = %v, %v; want true", ok, err)
	}
	if ok, err := fs.Exists(link); err != nil || ok {
		t.Errorf("Exists = %v, %v; want false for a dangling link", ok, err)
	}
	if ok, err := fs.LinkExists(filepath.Join(dir, "nothing")); err != nil || ok {
		t.Errorf("LinkExists(absent) = %v, %v", ok, err)
	}
}

func TestListDirsIgnoresSymlinksButListEntriesKeepsThem(t *testing.T) {
	dir, source, link := linkFixture(t)
	fs := New()
	if err := os.Symlink(source, link); err != nil {
		t.Fatal(err)
	}

	dirs, err := fs.ListDirs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 || dirs[0] != "source" {
		t.Errorf("ListDirs = %v, want only the real directory", dirs)
	}
	entries, err := fs.ListEntries(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("ListEntries = %v, want both", entries)
	}
}

func TestRemoveAllOnALinkLeavesTheTarget(t *testing.T) {
	_, source, link := linkFixture(t)
	fs := New()
	if err := os.Symlink(source, link); err != nil {
		t.Fatal(err)
	}

	if err := fs.RemoveAll(link); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(source, "keep.txt")); err != nil {
		t.Fatalf("the link target lost its contents: %v", err)
	}
	if ok, _ := fs.LinkExists(link); ok {
		t.Error("the link should be gone")
	}
}

func TestRemoveAllOnADirectoryHoldingALinkLeavesTheTarget(t *testing.T) {
	dir, source, _ := linkFixture(t)
	fs := New()
	holder := filepath.Join(dir, "holder")
	if err := os.Mkdir(holder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, filepath.Join(holder, "lib")); err != nil {
		t.Fatal(err)
	}

	if err := fs.RemoveAll(holder); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(source, "keep.txt")); err != nil {
		t.Fatalf("removing a directory followed a link into its target: %v", err)
	}
}

func TestWriteAndReadFile(t *testing.T) {
	fs := New()
	path := filepath.Join(t.TempDir(), "test.code-workspace")

	if err := fs.WriteFile(path, []byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	got, err := fs.ReadFile(path)
	if err != nil || string(got) != "hello\n" {
		t.Fatalf("ReadFile = %q, %v", got, err)
	}
}
