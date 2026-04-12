// Package builder compiles the obfuscated project by invoking the Go toolchain.
package builder

import (
	"log/slog"
	"pop-go/internal/models"
)

// Builder wraps the build configuration and logger needed to compile
// the obfuscated project with the Go toolchain.
type Builder struct {
	cfg *models.Config
	log *slog.Logger
}

// NewBuilder returns a Builder configured with cfg and log.
func NewBuilder(cfg *models.Config, log *slog.Logger) *Builder {
	return &Builder{
		cfg: cfg,
		log: log,
	}
}
