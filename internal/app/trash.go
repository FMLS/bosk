package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/FMLS/bosk/internal/domain"
)

// trashDirName holds quarantined workspace directories, laid out as
// <workspace>/<timestamp>/ so that a workspace name is always a whole path
// segment and never has to be parsed back out of a directory name.
const trashDirName = ".trash"

func trashRoot(cfg *domain.Config) string {
	return filepath.Join(cfg.WorkspaceDir, trashDirName)
}

// trashKeeper owns the layout of the trash directory: where a quarantined copy
// goes, and how it is removed without leaving debris behind.
type trashKeeper struct {
	root string
	fs   FileSystem
}

func newTrashKeeper(cfg *domain.Config, fs FileSystem) trashKeeper {
	return trashKeeper{root: trashRoot(cfg), fs: fs}
}

// dirFor is where a workspace's quarantined copies live.
func (k trashKeeper) dirFor(workspace string) string {
	return filepath.Join(k.root, workspace)
}

// remove deletes one quarantined copy, refusing any path that is not inside
// the trash directory, and drops the per-workspace directory once its last
// copy is gone. Leaving the empty directory behind would make doctor report it
// as leftover trash forever.
func (k trashKeeper) remove(path string) error {
	clean := filepath.Clean(path)
	if clean != k.root && !strings.HasPrefix(clean, k.root+string(filepath.Separator)) {
		return fmt.Errorf("refusing to delete path outside trash: %s", path)
	}
	// A quarantined workspace may hold symlinks to shared repositories.
	// RemoveAll deletes a symlink itself and never descends into its target, so
	// the source repository survives. Do not replace this with a recursive walk
	// that follows links.
	if err := k.fs.RemoveAll(clean); err != nil {
		return err
	}

	parent := filepath.Dir(clean)
	if filepath.Dir(parent) != k.root {
		return nil
	}
	entries, err := k.fs.ListEntries(parent)
	if err != nil || len(entries) > 0 {
		return nil
	}
	return k.fs.RemoveAll(parent)
}
