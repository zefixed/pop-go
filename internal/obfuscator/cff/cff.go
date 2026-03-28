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
	used map[string]struct{}
}

func NewCFF(cfg *models.Config, log *slog.Logger, pkgs []*packages.Package, r *rand.Rand, used map[string]struct{}) *CFF {
	return &CFF{
		cfg:  cfg,
		log:  log,
		pkgs: pkgs,
		r:    r,
		used: used,
	}
}
