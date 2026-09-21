package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/FMLS/bosk/internal/adapters/clock"
	"github.com/FMLS/bosk/internal/adapters/configstore"
	"github.com/FMLS/bosk/internal/adapters/discovery"
	"github.com/FMLS/bosk/internal/adapters/fsys"
	"github.com/FMLS/bosk/internal/adapters/git"
	"github.com/FMLS/bosk/internal/adapters/pathx"
	"github.com/FMLS/bosk/internal/adapters/registry"
	"github.com/FMLS/bosk/internal/app"
	"github.com/FMLS/bosk/internal/domain"
	"github.com/spf13/cobra"
)

// container is bosk's composition root: the one place that knows which
// concrete adapter backs each port. Commands ask it for a use case and never
// construct a dependency themselves.
type container struct {
	config     *configstore.Store
	git        *git.Client
	registry   *registry.Store
	discoverer *discovery.Scanner
	fs         fsys.OS
	clock      clock.System
	paths      pathx.Resolver
	warner     app.Warner
	confirmer  Confirmer

	cfg *domain.Config
}

// newContainer assembles the adapters. Loading configuration is deferred, so
// `bosk init` can run before one exists.
func newContainer(cmd *cobra.Command) *container {
	paths := pathx.New()
	files := fsys.New()
	configStore := configstore.NewStore(configstore.DefaultHome())
	gitClient := git.NewClient(git.NewExecRunner(), paths)
	return &container{
		config:     configStore,
		git:        gitClient,
		registry:   registry.NewStore(configStore.StatePath(), configStore.LockPath()),
		discoverer: discovery.NewScanner(gitClient, files, paths),
		fs:         files,
		clock:      clock.New(),
		paths:      paths,
		warner:     newWarner(cmd.ErrOrStderr()),
		confirmer:  newTerminalConfirmer(os.Stdin, cmd.ErrOrStderr()),
	}
}

// settings loads the configuration once, failing when bosk is uninitialized.
func (c *container) settings() (*domain.Config, error) {
	if c.cfg == nil {
		cfg, err := app.RequireConfig(c.config)
		if err != nil {
			return nil, err
		}
		c.cfg = cfg
	}
	return c.cfg, nil
}

func (c *container) initializer() *app.Initializer {
	return app.NewInitializer(c.config, c.discoverer, c.fs, c.paths)
}

func (c *container) inspector() (*app.Inspector, error) {
	cfg, err := c.settings()
	if err != nil {
		return nil, err
	}
	return app.NewInspector(cfg, c.git, c.registry, c.discoverer, c.fs, c.paths), nil
}

func (c *container) creator() (*app.Creator, error) {
	cfg, err := c.settings()
	if err != nil {
		return nil, err
	}
	return app.NewCreator(cfg, c.git, c.registry, c.discoverer, c.fs, c.clock, c.warner), nil
}

func (c *container) remover() (*app.Remover, error) {
	cfg, err := c.settings()
	if err != nil {
		return nil, err
	}
	return app.NewRemover(cfg, c.git, c.registry, c.fs, c.clock, c.warner), nil
}

func (c *container) doctor() (*app.Doctor, error) {
	cfg, err := c.settings()
	if err != nil {
		return nil, err
	}
	return app.NewDoctor(cfg, c.git, c.registry, c.fs), nil
}

func (c *container) repairer() (*app.Repairer, error) {
	cfg, err := c.settings()
	if err != nil {
		return nil, err
	}
	remover, err := c.remover()
	if err != nil {
		return nil, err
	}
	return app.NewRepairer(cfg, remover, c.fs), nil
}

// warner reports non-fatal problems on stderr.
type warner struct {
	out io.Writer
}

func newWarner(out io.Writer) warner { return warner{out: out} }

func (w warner) Warn(msg string) {
	fmt.Fprintln(w.out, "warning:", msg)
}
