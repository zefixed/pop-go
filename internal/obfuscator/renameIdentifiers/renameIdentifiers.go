package renameIdentifiers

import (
	"math/rand"

	"golang.org/x/tools/go/packages"
	"log/slog"
	"pop-go/internal/models"
)

type RenameIdentifiers struct {
	cfg  *models.Config
	log  *slog.Logger
	pkgs []*packages.Package
	r    *rand.Rand
}

func NewRenameIdentifiers(cfg *models.Config, log *slog.Logger, pkgs []*packages.Package, r *rand.Rand) *RenameIdentifiers {
	return &RenameIdentifiers{
		cfg:  cfg,
		log:  log,
		pkgs: pkgs,
		r:    r,
	}
}
