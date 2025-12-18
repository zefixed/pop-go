package obfuscator

import (
	"fmt"
	"go/token"
	"golang.org/x/tools/go/packages"
	"log/slog"
	"pop-go/internal/models"
)

type Obfuscator struct {
	cfg     *models.Config
	log     *slog.Logger
	pkgs    []*packages.Package
	CritErr error
}

func NewObfuscator(cfg *models.Config, log *slog.Logger) *Obfuscator {
	obfCfg := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedTypes | packages.NeedImports,
		Fset: token.NewFileSet(),
		Dir:  cfg.Obfuscator.TargetPath,
	}

	pkgs, err := packages.Load(obfCfg, "./...")
	if err != nil {
		log.Error(fmt.Sprintf("%s: %s", cfg.CurLocale["obf.err.load.pkg"], err.Error()))
	}

	return &Obfuscator{
		cfg:  cfg,
		log:  log,
		pkgs: pkgs,
	}
}
