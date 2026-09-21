package app

import (
	"fmt"

	"github.com/FMLS/bosk/internal/domain"
)

// InitResult describes what `bosk init` established.
type InitResult struct {
	Config     *domain.Config
	ConfigPath string
	Repos      []domain.Repo
	Skipped    []string
}

// Initializer registers repository roots and prepares the bosk home.
type Initializer struct {
	store      ConfigStore
	discoverer RepoDiscoverer
	fs         FileSystem
	paths      PathResolver
}

// NewInitializer wires the init use case to its collaborators.
func NewInitializer(store ConfigStore, discoverer RepoDiscoverer, fs FileSystem, paths PathResolver) *Initializer {
	return &Initializer{store: store, discoverer: discoverer, fs: fs, paths: paths}
}

// Init adds repository roots to the config, creates the data directories, and
// reports what it can discover under the resulting roots.
func (i *Initializer) Init(dirs []string) (InitResult, error) {
	if len(dirs) == 0 {
		return InitResult{}, fmt.Errorf("at least one repository directory is required")
	}
	cfg, err := i.store.Load()
	if err != nil {
		return InitResult{}, err
	}
	if cfg == nil {
		cfg = &domain.Config{WorkspaceDir: i.store.DefaultWorkspaceDir()}
	}

	known := make(map[string]bool, len(cfg.RepoDirs))
	for _, d := range cfg.RepoDirs {
		known[d] = true
	}
	for _, dir := range dirs {
		abs, err := i.validateRoot(dir)
		if err != nil {
			return InitResult{}, err
		}
		if !known[abs] {
			cfg.RepoDirs = append(cfg.RepoDirs, abs)
			known[abs] = true
		}
	}

	if cfg.WorkspaceDir == "" {
		cfg.WorkspaceDir = i.store.DefaultWorkspaceDir()
	}
	if err := i.fs.MkdirAll(i.store.Home()); err != nil {
		return InitResult{}, err
	}
	if err := i.fs.MkdirAll(cfg.WorkspaceDir); err != nil {
		return InitResult{}, err
	}
	if err := i.store.Save(cfg); err != nil {
		return InitResult{}, err
	}

	repos, skipped := i.discoverer.Discover(cfg.RepoDirs)
	return InitResult{
		Config:     cfg,
		ConfigPath: i.store.Path(),
		Repos:      repos,
		Skipped:    skipped,
	}, nil
}

func (i *Initializer) validateRoot(dir string) (string, error) {
	abs, err := i.paths.Resolve(dir)
	if err != nil {
		return "", fmt.Errorf("resolving %s: %w", dir, err)
	}
	exists, err := i.fs.Exists(abs)
	if err != nil || !exists {
		return "", fmt.Errorf("directory does not exist: %s", abs)
	}
	isDir, err := i.fs.IsDir(abs)
	if err != nil || !isDir {
		return "", fmt.Errorf("not a directory: %s", abs)
	}
	return abs, nil
}
