package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/FMLS/bosk/internal/config"
	"github.com/FMLS/bosk/internal/gitops"
	"github.com/FMLS/bosk/internal/models"
)

// File is the on-disk workspace registry.
type File struct {
	Workspaces []models.Workspace `json:"workspaces"`
}

// Store persists workspace records under ~/.bosk.
type Store struct {
	path     string
	lockPath string
}

// New returns a store using the default bosk home.
func New() *Store {
	return &Store{path: config.StatePath(), lockPath: config.LockPath()}
}

// Load reads the registry. A missing file is an empty list.
func (s *Store) Load() (*File, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &File{}, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return &File{}, nil
	}
	var file File
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("corrupt state file (%s). Run: bosk doctor --fix", s.path)
	}
	return &file, nil
}

// Save writes the registry atomically.
func (s *Store) Save(file *File) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".state-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, s.path)
}

// WithLock runs fn with an exclusive lock on the registry.
func (s *Store) WithLock(fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(s.lockPath), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("locking state: %w", err)
	}
	defer f.Close()
	if err := lockExclusive(f); err != nil {
		return fmt.Errorf("locking state: %w", err)
	}
	defer unlockExclusive(f)
	return fn()
}

// Get returns a workspace by name.
func (f *File) Get(name string) (models.Workspace, bool) {
	for _, w := range f.Workspaces {
		if w.Name == name {
			return w, true
		}
	}
	return models.Workspace{}, false
}

// Upsert replaces or appends a workspace record.
func (f *File) Upsert(ws models.Workspace) {
	for i, w := range f.Workspaces {
		if w.Name == ws.Name {
			f.Workspaces[i] = ws
			return
		}
	}
	f.Workspaces = append(f.Workspaces, ws)
}

// Remove deletes a workspace record by name.
func (f *File) Remove(name string) bool {
	for i, w := range f.Workspaces {
		if w.Name == name {
			f.Workspaces = append(f.Workspaces[:i], f.Workspaces[i+1:]...)
			return true
		}
	}
	return false
}

// ByPath finds the workspace that contains path.
func (f *File) ByPath(path string) (models.Workspace, bool) {
	want := gitops.Normalize(path)
	var best models.Workspace
	bestLen := -1
	for _, w := range f.Workspaces {
		root := gitops.Normalize(w.Path)
		if want == root || strings.HasPrefix(want, root+string(os.PathSeparator)) {
			if len(root) > bestLen {
				best = w
				bestLen = len(root)
			}
		}
	}
	if bestLen < 0 {
		return models.Workspace{}, false
	}
	return best, true
}
