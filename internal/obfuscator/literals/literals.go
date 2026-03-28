package literals

import (
	"math/rand"

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
	r    *rand.Rand
	used map[string]struct{}
}

func NewLiterals(cfg *models.Config, log *slog.Logger, pkgs []*packages.Package, r *rand.Rand, used map[string]struct{}) *Literals {
	return &Literals{
		cfg:  cfg,
		log:  log,
		pkgs: pkgs,
		r:    r,
		used: used,
	}
}
