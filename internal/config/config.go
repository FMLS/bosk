package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/FMLS/bosk/internal/models"
	"gopkg.in/yaml.v3"
)

const homeEnv = "BOSK_HOME"

// Home returns the bosk data directory.
func Home() string {
	if v := os.Getenv(homeEnv); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "bosk")
	}
	return filepath.Join(home, ".bosk")
}

// Path is the global config file.
func Path() string {
	return filepath.Join(Home(), "config.yaml")
}

// StatePath is the workspace registry.
func StatePath() string {
	return filepath.Join(Home(), "state.json")
}

// LockPath is the exclusive lock for mutating state.
func LockPath() string {
	return filepath.Join(Home(), "state.lock")
}

// DefaultWorkspaceDir is used when config omits workspace_dir.
func DefaultWorkspaceDir() string {
	return filepath.Join(Home(), "workspaces")
}

// Load reads config.yaml. A missing file returns (nil, nil).
func Load() (*models.Config, error) {
	data, err := os.ReadFile(Path())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var cfg models.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	cfg.RepoDirs = expandAll(cfg.RepoDirs)
	cfg.WorkspaceDir = expand(cfg.WorkspaceDir)
	if cfg.WorkspaceDir == "" {
		cfg.WorkspaceDir = DefaultWorkspaceDir()
	}
	return &cfg, nil
}

// Require loads config or explains how to initialize.
func Require() (*models.Config, error) {
	cfg, err := Load()
	if err != nil {
		return nil, err
	}
	if cfg == nil || len(cfg.RepoDirs) == 0 {
		return nil, fmt.Errorf("bosk is not initialized. Run: bosk init <repo-dir>")
	}
	return cfg, nil
}

// Save writes config atomically.
func Save(cfg *models.Config) error {
	if err := os.MkdirAll(filepath.Dir(Path()), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return writeAtomic(Path(), data)
}

// Init registers repository roots and ensures data directories exist.
func Init(dirs []string) (*models.Config, error) {
	if len(dirs) == 0 {
		return nil, fmt.Errorf("at least one repository directory is required")
	}
	cfg, err := Load()
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		cfg = &models.Config{
			WorkspaceDir: DefaultWorkspaceDir(),
		}
	}
	existing := make(map[string]bool, len(cfg.RepoDirs))
	for _, d := range cfg.RepoDirs {
		existing[d] = true
	}
	for _, d := range dirs {
		abs, err := filepath.Abs(expand(d))
		if err != nil {
			return nil, fmt.Errorf("resolving %s: %w", d, err)
		}
		info, err := os.Stat(abs)
		if err != nil {
			return nil, fmt.Errorf("directory does not exist: %s", abs)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("not a directory: %s", abs)
		}
		if !existing[abs] {
			cfg.RepoDirs = append(cfg.RepoDirs, abs)
			existing[abs] = true
		}
	}
	if cfg.WorkspaceDir == "" {
		cfg.WorkspaceDir = DefaultWorkspaceDir()
	}
	if err := os.MkdirAll(Home(), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.WorkspaceDir, 0o755); err != nil {
		return nil, err
	}
	if err := Save(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func expand(p string) string {
	if p == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}

func expandAll(dirs []string) []string {
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		if d == "" {
			continue
		}
		out = append(out, expand(d))
	}
	return out
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
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
