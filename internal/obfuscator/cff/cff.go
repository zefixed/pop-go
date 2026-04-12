// Package cff implements control-flow flattening (CFF), a structural
// obfuscation technique that transforms function bodies into state-machine
// dispatchers. Each original top-level statement becomes a case in a switch
// driven by a synthetic state variable inside an infinite for-loop, making
// the original control flow opaque to static analysis.
package cff

import (
	"log/slog"
	"math/rand"
	"pop-go/internal/models"

	"golang.org/x/tools/go/packages"
)

// CFF holds the shared state required by the control-flow flattening pass.
type CFF struct {
	cfg  *models.Config
	log  *slog.Logger
	pkgs []*packages.Package
	r    *rand.Rand
	used map[string]struct{}
}

// NewCFF returns a CFF pass configured with the given dependencies. r and used
// must be the same instances shared across all obfuscation passes to guarantee
// globally unique generated names.
func NewCFF(cfg *models.Config, log *slog.Logger, pkgs []*packages.Package, r *rand.Rand, used map[string]struct{}) *CFF {
	return &CFF{
		cfg:  cfg,
		log:  log,
		pkgs: pkgs,
		r:    r,
		used: used,
	}
}
