package obfuscator

import (
	"fmt"
	"go/token"
	"golang.org/x/tools/go/packages"
	"log/slog"
	"pop-go/internal/models"
	"pop-go/internal/obfuscator/cff"
	"pop-go/internal/obfuscator/literals"
	"pop-go/internal/obfuscator/renameIdentifiers"
	"pop-go/pkg/util"
)

type Obfuscator struct {
	cfg               *models.Config
	log               *slog.Logger
	pkgs              []*packages.Package
	literals          *literals.Literals
	renameIdentifiers *renameIdentifiers.RenameIdentifiers
	controlFlow       *cff.CFF
	CritErr           error
}

func NewObfuscator(cfg *models.Config, log *slog.Logger) *Obfuscator {
	obfCfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedCompiledGoFiles |
			packages.NeedImports |
			packages.NeedDeps |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedSyntax,
		Fset: token.NewFileSet(),
		Dir:  cfg.Obfuscator.TargetPath,
	}

	pkgs, err := packages.Load(obfCfg, "./...")
	if err != nil {
		return &Obfuscator{
			CritErr: fmt.Errorf("%s: %w", cfg.CurLocale["obf.err.load.pkg"], err),
		}
	}

	// One rand for the entire obfuscation pass.
	// All modules share it so every generated name advances the same PRNG —
	// names are globally unique even when seed is fixed (e.g. --seed 1).
	r := util.NewRand(cfg.Obfuscator.Seed)

	return &Obfuscator{
		cfg:               cfg,
		log:               log,
		pkgs:              pkgs,
		literals:          literals.NewLiterals(cfg, log, pkgs, r),
		renameIdentifiers: renameIdentifiers.NewRenameIdentifiers(cfg, log, pkgs, r),
		controlFlow:       cff.NewCFF(cfg, log, pkgs, r),
	}
}
