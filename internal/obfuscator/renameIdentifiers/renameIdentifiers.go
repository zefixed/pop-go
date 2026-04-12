// Package renameIdentifiers renames every unexported identifier in the target
// project's source files (and their internal test files) to randomly generated
// names, making the code harder to reverse-engineer without affecting exported
// API surfaces or standard-library names.
package renameIdentifiers

import (
	"math/rand"

	"golang.org/x/tools/go/packages"
	"log/slog"
	"pop-go/internal/models"
)

// RenameIdentifiers holds the shared state required by the rename pass.
type RenameIdentifiers struct {
	cfg  *models.Config
	log  *slog.Logger
	pkgs []*packages.Package
	r    *rand.Rand
	used map[string]struct{}
}

// NewRenameIdentifiers returns a RenameIdentifiers pass configured with the
// given dependencies. r and used must be the same instances shared across all
// obfuscation passes to guarantee globally unique generated names.
func NewRenameIdentifiers(cfg *models.Config, log *slog.Logger, pkgs []*packages.Package, r *rand.Rand, used map[string]struct{}) *RenameIdentifiers {
	return &RenameIdentifiers{
		cfg:  cfg,
		log:  log,
		pkgs: pkgs,
		r:    r,
		used: used,
	}
}
