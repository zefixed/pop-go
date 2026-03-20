package cff

import (
	"log/slog"
	"pop-go/internal/models"

	"golang.org/x/tools/go/packages"
)

type CFF struct {
	cfg  *models.Config
	log  *slog.Logger
	pkgs []*packages.Package
}

func NewCFF(cfg *models.Config, log *slog.Logger, pkgs []*packages.Package) *CFF {
	return &CFF{
		cfg:  cfg,
		log:  log,
		pkgs: pkgs,
	}
}
