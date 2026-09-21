package testsupport

import (
	"fmt"
	"sort"
	"sync"

	"github.com/FMLS/bosk/internal/domain"
)

// FakeGit is an in-memory stand-in for the git adapter. It keeps just enough
// state (repositories, branches, worktrees) to exercise the use cases, and
// exposes injectable failures for the paths that are hard to provoke with a
// real repository.
type FakeGit struct {
	mu sync.Mutex

	repos map[string]*fakeRepo

	// Injectable failures, keyed by repository path unless noted.
	FetchErr        map[string]error
	AddErr          map[string]error
	PruneErr        map[string]error
	RegisteredErr   map[string]error
	InvalidBranches map[string]bool
	NoStartPoint    map[string]bool

	// Worktree state keyed by worktree path.
	DirtyPaths  map[string]int
	AheadPaths  map[string]int
	BehindPaths map[string]int
	StatusErr   map[string]error

	// Files, when set, makes the fake create and delete worktree
	// directories the way real git does.
	Files FakeFS

	// Calls records mutating operations in order, for assertions.
	Calls []string
}

// FakeFS is the slice of a filesystem the fake git needs.
type FakeFS interface {
	MkdirAll(path string) error
	RemoveAll(path string) error
}

type fakeRepo struct {
	branches  map[string]bool
	worktrees map[string]string // worktree path -> branch
}

// NewFakeGit returns an empty fake with no repositories.
func NewFakeGit() *FakeGit {
	return &FakeGit{
		repos:           map[string]*fakeRepo{},
		FetchErr:        map[string]error{},
		AddErr:          map[string]error{},
		PruneErr:        map[string]error{},
		RegisteredErr:   map[string]error{},
		InvalidBranches: map[string]bool{},
		NoStartPoint:    map[string]bool{},
		DirtyPaths:      map[string]int{},
		AheadPaths:      map[string]int{},
		BehindPaths:     map[string]int{},
		StatusErr:       map[string]error{},
	}
}

// AddRepo registers a source repository with the given local branches.
func (g *FakeGit) AddRepo(path string, branches ...string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	repo := &fakeRepo{branches: map[string]bool{"main": true}, worktrees: map[string]string{}}
	for _, b := range branches {
		repo.branches[b] = true
	}
	g.repos[path] = repo
}

// RemoveRepo makes a source repository look deleted, as if the user removed
// the clone behind bosk's back.
func (g *FakeGit) RemoveRepo(path string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.repos, path)
}

// CheckOut registers an existing worktree, as if another workspace owned it.
func (g *FakeGit) CheckOut(repo, path, branch string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	r := g.repos[repo]
	if r == nil {
		return
	}
	r.branches[branch] = true
	r.worktrees[path] = branch
}

// Worktrees returns the registered worktree paths of a repo, sorted.
func (g *FakeGit) Worktrees(repo string) []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	r := g.repos[repo]
	if r == nil {
		return nil
	}
	paths := make([]string, 0, len(r.worktrees))
	for p := range r.worktrees {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// Branches returns the local branches of a repo, sorted.
func (g *FakeGit) Branches(repo string) []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	r := g.repos[repo]
	if r == nil {
		return nil
	}
	names := make([]string, 0, len(r.branches))
	for b := range r.branches {
		names = append(names, b)
	}
	sort.Strings(names)
	return names
}

func (g *FakeGit) record(format string, args ...any) {
	g.Calls = append(g.Calls, fmt.Sprintf(format, args...))
}

// --- ports --------------------------------------------------------------

func (g *FakeGit) IsRepo(path string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.repos[path] != nil
}

func (g *FakeGit) Fetch(repo string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.record("fetch %s", repo)
	return g.FetchErr[repo]
}

func (g *FakeGit) ValidBranch(name string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return name != "" && !g.InvalidBranches[name]
}

func (g *FakeGit) BranchExists(repo, branch string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	r := g.repos[repo]
	return r != nil && r.branches[branch]
}

func (g *FakeGit) BranchInUse(repo, branch string) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	r := g.repos[repo]
	if r == nil {
		return false, fmt.Errorf("no such repo %s", repo)
	}
	for _, b := range r.worktrees {
		if b == branch {
			return true, nil
		}
	}
	return false, nil
}

func (g *FakeGit) StartPoint(repo, base string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.NoStartPoint[repo] {
		return "", fmt.Errorf("start point not found in %s", repo)
	}
	if base == "" {
		base = "main"
	}
	return base, nil
}

func (g *FakeGit) DeleteBranch(repo, branch string, force bool) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.record("delete-branch %s %s force=%v", repo, branch, force)
	r := g.repos[repo]
	if r == nil {
		return fmt.Errorf("no such repo %s", repo)
	}
	if !r.branches[branch] {
		return fmt.Errorf("branch %s not found", branch)
	}
	delete(r.branches, branch)
	return nil
}

func (g *FakeGit) WorktreeAdd(repo, path, branch string) error {
	return g.addWorktree(repo, path, branch, false)
}

func (g *FakeGit) WorktreeAddNew(repo, path, branch, startPoint string) error {
	return g.addWorktree(repo, path, branch, true)
}

func (g *FakeGit) addWorktree(repo, path, branch string, create bool) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.record("worktree-add %s %s %s new=%v", repo, path, branch, create)
	if err := g.AddErr[repo]; err != nil {
		return err
	}
	r := g.repos[repo]
	if r == nil {
		return fmt.Errorf("no such repo %s", repo)
	}
	if create {
		r.branches[branch] = true
	}
	r.worktrees[path] = branch
	if g.Files != nil {
		_ = g.Files.MkdirAll(path)
	}
	return nil
}

func (g *FakeGit) WorktreeRemove(repo, path string, force bool) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.record("worktree-remove %s %s force=%v", repo, path, force)
	r := g.repos[repo]
	if r == nil {
		return fmt.Errorf("no such repo %s", repo)
	}
	delete(r.worktrees, path)
	if g.Files != nil {
		_ = g.Files.RemoveAll(path)
	}
	return nil
}

func (g *FakeGit) WorktreePrune(repo string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.record("worktree-prune %s", repo)
	return g.PruneErr[repo]
}

func (g *FakeGit) WorktreeRepair(repo, path string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.record("worktree-repair %s %s", repo, path)
	return nil
}

func (g *FakeGit) WorktreeRegistered(repo, path string) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.RegisteredErr[repo]; err != nil {
		return false, err
	}
	r := g.repos[repo]
	if r == nil {
		return false, fmt.Errorf("no such repo %s", repo)
	}
	_, ok := r.worktrees[path]
	return ok, nil
}

func (g *FakeGit) CurrentBranch(path string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.StatusErr[path]; err != nil {
		return "", err
	}
	for _, r := range g.repos {
		if b, ok := r.worktrees[path]; ok {
			return b, nil
		}
	}
	return "", nil
}

func (g *FakeGit) DirtyCount(path string) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.StatusErr[path]; err != nil {
		return 0, err
	}
	return g.DirtyPaths[path], nil
}

func (g *FakeGit) AheadBehind(path string) (int, int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.StatusErr[path]; err != nil {
		return 0, 0, err
	}
	return g.AheadPaths[path], g.BehindPaths[path], nil
}

// FakeDiscoverer returns a canned repository list.
type FakeDiscoverer struct {
	Repos   []domain.Repo
	Skipped []string
}

func (d FakeDiscoverer) Discover([]string) ([]domain.Repo, []string) {
	return d.Repos, d.Skipped
}
