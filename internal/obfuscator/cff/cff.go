package cff

import (
	"golang.org/x/tools/go/packages"
	"log/slog"
	"pop-go/internal/models"
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
