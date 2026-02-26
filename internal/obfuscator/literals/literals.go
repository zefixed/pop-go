package literals

import (
	"golang.org/x/tools/go/packages"
	"log/slog"
	"pop-go/internal/models"
)

type ObfuscateLiteralsProfile interface {
	GenerateKey(seed int64) interface{}
	DecryptFunction(key interface{}, funcName string) string
	EncryptString(str string, key interface{}) string
	RequiredImports() []string
}

type Literals struct {
	cfg  *models.Config
	log  *slog.Logger
	pkgs []*packages.Package
}

func NewLiterals(cfg *models.Config, log *slog.Logger, pkgs []*packages.Package) *Literals {
	return &Literals{
		cfg:  cfg,
		log:  log,
		pkgs: pkgs,
	}
}
