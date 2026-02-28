package renameIdentifiers

import (
	"golang.org/x/tools/go/packages"
	"log/slog"
	"pop-go/internal/models"
)

type RenameIdentifiers struct {
	cfg  *models.Config
	log  *slog.Logger
	pkgs []*packages.Package
}

func NewRenameIdentifiers(cfg *models.Config, log *slog.Logger, pkgs []*packages.Package) *RenameIdentifiers {
	return &RenameIdentifiers{
		cfg:  cfg,
		log:  log,
		pkgs: pkgs,
	}
}
