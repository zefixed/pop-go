package cff

import (
	"log/slog"
	"math/rand"
	"pop-go/internal/models"

	"golang.org/x/tools/go/packages"
)

type CFF struct {
	cfg  *models.Config
	log  *slog.Logger
	pkgs []*packages.Package
	r    *rand.Rand
}

func NewCFF(cfg *models.Config, log *slog.Logger, pkgs []*packages.Package, r *rand.Rand) *CFF {
	return &CFF{
		cfg:  cfg,
		log:  log,
		pkgs: pkgs,
		r:    r,
	}
}
