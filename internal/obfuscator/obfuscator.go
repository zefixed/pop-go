package obfuscator

import (
	"fmt"
	"go/token"
	"golang.org/x/tools/go/packages"
	"log/slog"
	"pop-go/internal/models"
	"pop-go/internal/obfuscator/literals"
	"pop-go/internal/obfuscator/renameIdentifiers"
)

type Obfuscator struct {
	cfg               *models.Config
	log               *slog.Logger
	pkgs              []*packages.Package
	literals          *literals.Literals
	renameIdentifiers *renameIdentifiers.RenameIdentifiers
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

	return &Obfuscator{
		cfg:               cfg,
		log:               log,
		pkgs:              pkgs,
		literals:          literals.NewLiterals(cfg, log, pkgs),
		renameIdentifiers: renameIdentifiers.NewRenameIdentifiers(cfg, log, pkgs),
	}
}
