package app

import (
	"fmt"

	"github.com/FMLS/bosk/internal/domain"
)

// RequireConfig loads the configuration or explains how to initialize bosk.
func RequireConfig(store ConfigStore) (*domain.Config, error) {
	cfg, err := store.Load()
	if err != nil {
		return nil, err
	}
	if cfg == nil || len(cfg.RepoDirs) == 0 {
		return nil, fmt.Errorf("bosk is not initialized. Run: bosk init <repo-dir>")
	}
	return cfg, nil
}
