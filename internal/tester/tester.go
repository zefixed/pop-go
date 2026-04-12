// Package tester runs "go test" against the target project and reports
// failures to the caller. It is invoked twice by the pipeline: once before
// any obfuscation to confirm the baseline passes, and once after to verify
// that the transformations did not break existing tests.
package tester

import (
	"log/slog"
	"pop-go/internal/models"
)

// Tester executes "go test" in a given directory and reports the outcome
// through the configured logger.
type Tester struct {
	cfg *models.Config
	log *slog.Logger
}

// NewTester returns a Tester configured with cfg and log.
func NewTester(cfg *models.Config, log *slog.Logger) *Tester {
	return &Tester{
		cfg: cfg,
		log: log,
	}
}
