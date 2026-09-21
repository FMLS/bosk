package discover

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/FMLS/bosk/internal/gitops"
	"github.com/FMLS/bosk/internal/models"
)

const maxDepth = 3

var skipNames = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"target":       true,
	".bosk":        true,
}

// Repos walks configured roots and returns unique git repositories.
// Shallower paths win when two repositories share a directory name.
func Repos(roots []string) ([]models.Repo, []string) {
	type node struct {
		path  string
		depth int
	}
	seenName := make(map[string]bool)
	seenPath := make(map[string]bool)
	var repos []models.Repo
	var skipped []string

	for _, root := range roots {
		root = gitops.Normalize(root)
		queue := []node{{path: root, depth: 0}}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			if seenPath[cur.path] {
				continue
			}
			if gitops.IsGitRepo(cur.path) {
				seenPath[cur.path] = true
				name := filepath.Base(cur.path)
				if seenName[name] {
					skipped = append(skipped, cur.path)
					continue
				}
				seenName[name] = true
				repos = append(repos, models.Repo{Name: name, Path: cur.path})
				continue
			}
			if cur.depth >= maxDepth {
				continue
			}
			entries, err := os.ReadDir(cur.path)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				if skipNames[e.Name()] || strings.HasPrefix(e.Name(), ".") {
					continue
				}
				next := filepath.Join(cur.path, e.Name())
				queue = append(queue, node{path: next, depth: cur.depth + 1})
			}
		}
	}
	return repos, skipped
}

// ByName returns repositories requested by name, in request order.
func ByName(repos []models.Repo, names []string) ([]models.Repo, error) {
	index := make(map[string]models.Repo, len(repos))
	for _, r := range repos {
		index[r.Name] = r
	}
	var out []models.Repo
	var missing []string
	seen := make(map[string]bool)
	for _, name := range names {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		r, ok := index[name]
		if !ok {
			missing = append(missing, name)
			continue
		}
		out = append(out, r)
	}
	if len(missing) > 0 {
		available := make([]string, 0, len(repos))
		for _, r := range repos {
			available = append(available, r.Name)
		}
		return nil, &MissingError{Names: missing, Available: available}
	}
	return out, nil
}

// MissingError lists unknown repository names.
type MissingError struct {
	Names     []string
	Available []string
}

func (e *MissingError) Error() string {
	msg := "unknown repositories: " + strings.Join(e.Names, ", ")
	if len(e.Available) > 0 {
		msg += " (available: " + strings.Join(e.Available, ", ") + ")"
	}
	return msg
}
