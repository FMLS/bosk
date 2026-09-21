package app_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/FMLS/bosk/internal/app"
	"github.com/FMLS/bosk/internal/domain"
	"github.com/FMLS/bosk/internal/testsupport"
)

// harness wires the use cases to in-memory doubles. Nothing here touches the
// disk or runs git, which is the point of the port interfaces.
type harness struct {
	cfg    *domain.Config
	git    *testsupport.FakeGit
	reg    *testsupport.MemRegistry
	fs     *testsupport.MemFS
	disc   testsupport.FakeDiscoverer
	clock  testsupport.FixedClock
	warner *testsupport.RecordingWarner
	paths  testsupport.CleanResolver
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	fs := testsupport.NewMemFS("/home/user")
	fs.AddDir("/ws")
	fs.AddDir("/src/svc-a")
	fs.AddDir("/src/svc-b")

	git := testsupport.NewFakeGit()
	git.Files = fs
	git.AddRepo("/src/svc-a")
	git.AddRepo("/src/svc-b")

	return &harness{
		cfg: &domain.Config{RepoDirs: []string{"/src"}, WorkspaceDir: "/ws"},
		git: git,
		reg: &testsupport.MemRegistry{},
		fs:  fs,
		disc: testsupport.FakeDiscoverer{Repos: []domain.Repo{
			{Name: "svc-a", Path: "/src/svc-a"},
			{Name: "svc-b", Path: "/src/svc-b"},
		}},
		clock:  testsupport.FixedClock{Time: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)},
		warner: &testsupport.RecordingWarner{},
	}
}

func (h *harness) creator() *app.Creator {
	return app.NewCreator(h.cfg, h.git, h.reg, h.disc, h.fs, h.clock, h.warner)
}

func (h *harness) remover() *app.Remover {
	return app.NewRemover(h.cfg, h.git, h.reg, h.fs, h.clock, h.warner)
}

func (h *harness) inspector() *app.Inspector {
	return app.NewInspector(h.cfg, h.git, h.reg, h.disc, h.fs, h.paths)
}

func (h *harness) doctor() *app.Doctor {
	return app.NewDoctor(h.cfg, h.git, h.reg, h.fs)
}

func (h *harness) repairer() *app.Repairer {
	return app.NewRepairer(h.cfg, h.remover(), h.fs)
}

// mustCreate builds a workspace and fails the test if that does not work.
func (h *harness) mustCreate(t *testing.T, opts app.CreateOpts) domain.Workspace {
	t.Helper()
	ws, err := h.creator().Create(opts)
	if err != nil {
		t.Fatalf("Create(%+v) = %v", opts, err)
	}
	return ws
}

func (h *harness) trashPath(parts ...string) string {
	return filepath.Join(append([]string{"/ws", ".trash"}, parts...)...)
}

// recorded reports whether the fake git saw an operation.
func (h *harness) recorded(call string) bool {
	for _, c := range h.git.Calls {
		if c == call {
			return true
		}
	}
	return false
}

func exists(t *testing.T, fs *testsupport.MemFS, path string) bool {
	t.Helper()
	ok, err := fs.Exists(path)
	if err != nil {
		t.Fatalf("Exists(%s) = %v", path, err)
	}
	return ok
}
