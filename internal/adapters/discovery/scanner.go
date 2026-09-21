// Package discovery finds source repositories under the configured roots.
package discovery

import (
	"path/filepath"
	"strings"

	"github.com/FMLS/bosk/internal/domain"
)

// DefaultMaxDepth is how far below a root the scanner looks for repositories.
const DefaultMaxDepth = 3

var skipNames = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"target":       true,
	".bosk":        true,
}

// RepoProbe reports whether a path is a git repository.
type RepoProbe interface {
	IsRepo(path string) bool
}

// DirLister lists the subdirectories of a directory.
type DirLister interface {
	ListDirs(path string) ([]string, error)
}

// PathNormalizer produces comparable absolute paths.
type PathNormalizer interface {
	Normalize(path string) string
}

// Scanner walks configured roots looking for git repositories.
type Scanner struct {
	probe    RepoProbe
	dirs     DirLister
	paths    PathNormalizer
	maxDepth int
}

// NewScanner wires a scanner to its collaborators.
func NewScanner(probe RepoProbe, dirs DirLister, paths PathNormalizer) *Scanner {
	return &Scanner{probe: probe, dirs: dirs, paths: paths, maxDepth: DefaultMaxDepth}
}

type node struct {
	path  string
	depth int
}

// Discover returns unique git repositories plus the paths skipped for having a
// duplicate directory name. Shallower paths win.
func (s *Scanner) Discover(roots []string) ([]domain.Repo, []string) {
	seenName := make(map[string]bool)
	seenPath := make(map[string]bool)
	var repos []domain.Repo
	var skipped []string

	for _, root := range roots {
		queue := []node{{path: s.paths.Normalize(root), depth: 0}}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			if seenPath[cur.path] {
				continue
			}
			if s.probe.IsRepo(cur.path) {
				seenPath[cur.path] = true
				name := filepath.Base(cur.path)
				if seenName[name] {
					skipped = append(skipped, cur.path)
					continue
				}
				seenName[name] = true
				repos = append(repos, domain.Repo{Name: name, Path: cur.path})
				continue
			}
			if cur.depth >= s.maxDepth {
				continue
			}
			names, err := s.dirs.ListDirs(cur.path)
			if err != nil {
				continue
			}
			for _, name := range names {
				if skipNames[name] || strings.HasPrefix(name, ".") {
					continue
				}
				queue = append(queue, node{path: filepath.Join(cur.path, name), depth: cur.depth + 1})
			}
		}
	}
	return repos, skipped
}
