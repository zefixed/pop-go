package obfuscator

import (
	"fmt"
	"go/token"
	"golang.org/x/tools/go/packages"
	"log/slog"
	"pop-go/internal/models"
	"pop-go/internal/obfuscator/literals"
)

type Obfuscator struct {
	cfg      *models.Config
	log      *slog.Logger
	pkgs     []*packages.Package
	literals *literals.Literals
	CritErr  error
}

func NewObfuscator(cfg *models.Config, log *slog.Logger) *Obfuscator {
	obfCfg := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedTypes | packages.NeedImports,
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
		cfg:      cfg,
		log:      log,
		pkgs:     pkgs,
		literals: literals.NewLiterals(cfg, log, pkgs),
	}
}
