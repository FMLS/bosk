package testsupport

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/FMLS/bosk/internal/domain"
)

// MemFS is an in-memory filesystem covering the operations bosk performs.
type MemFS struct {
	dirs  map[string]bool
	files map[string]bool

	Cwd string

	MkdirErr  map[string]error
	RenameErr map[string]error
	RemoveErr map[string]error
	StatErr   map[string]error
}

// NewMemFS returns an empty filesystem with the given working directory.
func NewMemFS(cwd string) *MemFS {
	fs := &MemFS{
		dirs:      map[string]bool{},
		files:     map[string]bool{},
		Cwd:       cwd,
		MkdirErr:  map[string]error{},
		RenameErr: map[string]error{},
		RemoveErr: map[string]error{},
		StatErr:   map[string]error{},
	}
	if cwd != "" {
		fs.AddDir(cwd)
	}
	return fs
}

// AddDir creates a directory and its parents.
func (fs *MemFS) AddDir(path string) {
	clean := filepath.Clean(path)
	for p := clean; ; p = filepath.Dir(p) {
		fs.dirs[p] = true
		if p == filepath.Dir(p) {
			return
		}
	}
}

// AddFile creates a file and its parent directories.
func (fs *MemFS) AddFile(path string) {
	clean := filepath.Clean(path)
	fs.AddDir(filepath.Dir(clean))
	fs.files[clean] = true
}

// Paths returns every known path, sorted.
func (fs *MemFS) Paths() []string {
	out := make([]string, 0, len(fs.dirs)+len(fs.files))
	for p := range fs.dirs {
		out = append(out, p)
	}
	for p := range fs.files {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func (fs *MemFS) Exists(path string) (bool, error) {
	clean := filepath.Clean(path)
	if err := fs.StatErr[clean]; err != nil {
		return false, err
	}
	return fs.dirs[clean] || fs.files[clean], nil
}

func (fs *MemFS) IsDir(path string) (bool, error) {
	clean := filepath.Clean(path)
	if err := fs.StatErr[clean]; err != nil {
		return false, err
	}
	return fs.dirs[clean], nil
}

func (fs *MemFS) MkdirAll(path string) error {
	clean := filepath.Clean(path)
	if err := fs.MkdirErr[clean]; err != nil {
		return err
	}
	fs.AddDir(clean)
	return nil
}

func (fs *MemFS) Rename(oldPath, newPath string) error {
	from := filepath.Clean(oldPath)
	to := filepath.Clean(newPath)
	if err := fs.RenameErr[from]; err != nil {
		return err
	}
	if !fs.dirs[from] && !fs.files[from] {
		return fmt.Errorf("rename %s: no such file or directory", from)
	}
	move := func(set map[string]bool) {
		for p := range set {
			if p == from || strings.HasPrefix(p, from+string(filepath.Separator)) {
				delete(set, p)
				set[to+strings.TrimPrefix(p, from)] = true
			}
		}
	}
	move(fs.dirs)
	move(fs.files)
	fs.AddDir(filepath.Dir(to))
	return nil
}

func (fs *MemFS) RemoveAll(path string) error {
	clean := filepath.Clean(path)
	if err := fs.RemoveErr[clean]; err != nil {
		return err
	}
	drop := func(set map[string]bool) {
		for p := range set {
			if p == clean || strings.HasPrefix(p, clean+string(filepath.Separator)) {
				delete(set, p)
			}
		}
	}
	drop(fs.dirs)
	drop(fs.files)
	return nil
}

func (fs *MemFS) ListDirs(path string) ([]string, error) {
	return fs.list(path, true)
}

func (fs *MemFS) ListEntries(path string) ([]string, error) {
	return fs.list(path, false)
}

func (fs *MemFS) list(path string, dirsOnly bool) ([]string, error) {
	clean := filepath.Clean(path)
	if !fs.dirs[clean] {
		return nil, fmt.Errorf("open %s: no such file or directory", clean)
	}
	seen := map[string]bool{}
	collect := func(set map[string]bool) {
		for p := range set {
			if filepath.Dir(p) == clean && p != clean {
				seen[filepath.Base(p)] = true
			}
		}
	}
	collect(fs.dirs)
	if !dirsOnly {
		collect(fs.files)
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

func (fs *MemFS) Getwd() (string, error) {
	if fs.Cwd == "" {
		return "", fmt.Errorf("getwd: no working directory")
	}
	return fs.Cwd, nil
}

// MemRegistry keeps the workspace registry in memory. Load hands out a copy so
// that callers mutating the result cannot change stored state without Save,
// matching how the file-backed store behaves.
type MemRegistry struct {
	Registry domain.Registry

	LoadErr error
	SaveErr error
	Locks   int
	Saves   int
}

func (r *MemRegistry) Load() (*domain.Registry, error) {
	if r.LoadErr != nil {
		return nil, r.LoadErr
	}
	out := &domain.Registry{}
	out.Workspaces = append(out.Workspaces, r.Registry.Workspaces...)
	return out, nil
}

func (r *MemRegistry) Save(reg *domain.Registry) error {
	if r.SaveErr != nil {
		return r.SaveErr
	}
	r.Saves++
	r.Registry = domain.Registry{}
	r.Registry.Workspaces = append(r.Registry.Workspaces, reg.Workspaces...)
	return nil
}

func (r *MemRegistry) WithLock(fn func() error) error {
	r.Locks++
	return fn()
}

// FixedClock always reports the same instant.
type FixedClock struct {
	Time time.Time
}

func (c FixedClock) Now() time.Time { return c.Time }

// RecordingWarner collects warnings instead of printing them.
type RecordingWarner struct {
	Messages []string
}

func (w *RecordingWarner) Warn(msg string) { w.Messages = append(w.Messages, msg) }

// Contains reports whether any warning contains substr.
func (w *RecordingWarner) Contains(substr string) bool {
	for _, m := range w.Messages {
		if strings.Contains(m, substr) {
			return true
		}
	}
	return false
}

// CleanResolver resolves paths lexically, so tests never touch the disk.
type CleanResolver struct{}

func (CleanResolver) Normalize(path string) string { return filepath.Clean(path) }

func (CleanResolver) Resolve(path string) (string, error) {
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	return filepath.Join(string(os.PathSeparator), path), nil
}

// MemConfigStore keeps the configuration in memory.
type MemConfigStore struct {
	Config       *domain.Config
	HomeDir      string
	LoadErr      error
	SaveErr      error
	SavedConfigs int
}

func (s *MemConfigStore) Load() (*domain.Config, error) {
	if s.LoadErr != nil {
		return nil, s.LoadErr
	}
	if s.Config == nil {
		return nil, nil
	}
	clone := *s.Config
	clone.RepoDirs = append([]string(nil), s.Config.RepoDirs...)
	return &clone, nil
}

func (s *MemConfigStore) Save(cfg *domain.Config) error {
	if s.SaveErr != nil {
		return s.SaveErr
	}
	s.SavedConfigs++
	clone := *cfg
	clone.RepoDirs = append([]string(nil), cfg.RepoDirs...)
	s.Config = &clone
	return nil
}

func (s *MemConfigStore) Path() string { return filepath.Join(s.HomeDir, "config.yaml") }

func (s *MemConfigStore) Home() string { return s.HomeDir }

func (s *MemConfigStore) DefaultWorkspaceDir() string { return filepath.Join(s.HomeDir, "workspaces") }

// ScriptedRunner replays canned git output keyed by the joined arguments.
type ScriptedRunner struct {
	Responses map[string]string
	Errors    map[string]error
	Calls     []string
}

func NewScriptedRunner() *ScriptedRunner {
	return &ScriptedRunner{Responses: map[string]string{}, Errors: map[string]error{}}
}

func (r *ScriptedRunner) Run(dir string, args ...string) (string, error) {
	key := strings.Join(args, " ")
	r.Calls = append(r.Calls, key)
	if err, ok := r.Errors[key]; ok {
		return r.Responses[key], err
	}
	out, ok := r.Responses[key]
	if !ok {
		return "", fmt.Errorf("unexpected git call: %s", key)
	}
	return out, nil
}
