// Package configstore persists the global bosk configuration as YAML and owns
// the layout of the bosk home directory.
package configstore

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/FMLS/bosk/internal/adapters/pathx"
	"github.com/FMLS/bosk/internal/domain"
	"gopkg.in/yaml.v3"
)

// HomeEnv overrides the bosk home directory.
const HomeEnv = "BOSK_HOME"

const dirPerm = 0o755

// DefaultHome returns the bosk data directory for the current environment.
func DefaultHome() string {
	if v := os.Getenv(HomeEnv); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "bosk")
	}
	return filepath.Join(home, ".bosk")
}

// Store reads and writes config.yaml under a fixed home directory.
type Store struct {
	home string
}

// NewStore returns a store rooted at the given home directory.
func NewStore(home string) *Store { return &Store{home: home} }

// Home is the bosk data directory.
func (s *Store) Home() string { return s.home }

// Path is the global config file.
func (s *Store) Path() string { return filepath.Join(s.home, "config.yaml") }

// StatePath is the workspace registry document.
func (s *Store) StatePath() string { return filepath.Join(s.home, "state.json") }

// LockPath is the exclusive lock for mutating state.
func (s *Store) LockPath() string { return filepath.Join(s.home, "state.lock") }

// DefaultWorkspaceDir is used when config omits workspace_dir.
func (s *Store) DefaultWorkspaceDir() string { return filepath.Join(s.home, "workspaces") }

// Load reads config.yaml and resolves its paths. A missing file returns
// (nil, nil) so callers can distinguish "absent" from "empty".
func (s *Store) Load() (*domain.Config, error) {
	data, err := os.ReadFile(s.Path())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var cfg domain.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	dirs := make([]string, 0, len(cfg.RepoDirs))
	for _, d := range cfg.RepoDirs {
		if d == "" {
			continue
		}
		dirs = append(dirs, pathx.Expand(d))
	}
	cfg.RepoDirs = dirs
	cfg.WorkspaceDir = pathx.Expand(cfg.WorkspaceDir)
	if cfg.WorkspaceDir == "" {
		cfg.WorkspaceDir = s.DefaultWorkspaceDir()
	}
	return &cfg, nil
}

// Save writes config atomically.
func (s *Store) Save(cfg *domain.Config) error {
	if err := os.MkdirAll(filepath.Dir(s.Path()), dirPerm); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return writeAtomic(s.Path(), data)
}

func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
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
	return os.Rename(tmpName, path)
}
