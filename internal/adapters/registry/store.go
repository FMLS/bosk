// Package registry persists the workspace registry as a JSON file guarded by
// an advisory file lock.
package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/FMLS/bosk/internal/domain"
)

const (
	dirPerm  = 0o755
	lockPerm = 0o644
)

// Store reads and writes the registry document. Both paths are supplied by the
// caller, so nothing here depends on where bosk's home happens to be.
type Store struct {
	path     string
	lockPath string
}

// NewStore returns a store for the given document and lock paths.
func NewStore(path, lockPath string) *Store {
	return &Store{path: path, lockPath: lockPath}
}

// Load reads the registry. A missing or empty file is an empty registry.
func (s *Store) Load() (*domain.Registry, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &domain.Registry{}, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return &domain.Registry{}, nil
	}
	var reg domain.Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("corrupt state file (%s). Run: bosk doctor --fix", s.path)
	}
	return &reg, nil
}

// Save writes the registry atomically.
func (s *Store) Save(reg *domain.Registry) error {
	if err := os.MkdirAll(filepath.Dir(s.path), dirPerm); err != nil {
		return err
	}
	data, err := json.MarshalIndent(reg, "", "  ")
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

// WithLock runs fn while holding an exclusive lock on the registry.
func (s *Store) WithLock(fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(s.lockPath), dirPerm); err != nil {
		return err
	}
	f, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_RDWR, lockPerm)
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
