// Package literals implements string-literal obfuscation. Each string constant
// in the source is encrypted at compile time and replaced with a call to an
// inline decryption function that is injected into the same file. Two
// encryption profiles are available: XOR (easy) and AES-CTR (medium).
package literals

import (
	"math/rand"

	"golang.org/x/tools/go/packages"
	"log/slog"
	"pop-go/internal/models"
)

// ObfuscateLiteralsProfile defines the interface for a string-encryption
// strategy. Implementations must be stateless and deterministic for a given
// seed so that multiple runs with the same seed produce identical output.
type ObfuscateLiteralsProfile interface {
	// GenerateKey derives an encryption key from seed. If seed is 0 the
	// implementation should use a time-based value for uniqueness.
	GenerateKey(seed int64) interface{}

	// DecryptFunction returns the source code of a Go function named funcName
	// that decrypts a string encrypted by EncryptString.
	DecryptFunction(key interface{}, funcName string) string

	// EncryptString encrypts str with key and returns the ciphertext as a
	// Go string literal value (may contain arbitrary bytes).
	EncryptString(str string, key interface{}) string

	// RequiredImports returns the import paths needed by the decrypt function.
	RequiredImports() []string
}

// Literals holds the shared state required by the literal obfuscation pass.
type Literals struct {
	cfg  *models.Config
	log  *slog.Logger
	pkgs []*packages.Package
	r    *rand.Rand
	used map[string]struct{}
}

// NewLiterals returns a Literals pass configured with the given dependencies.
// r and used must be the same instances shared across all obfuscation passes
// to guarantee globally unique generated names.
func NewLiterals(cfg *models.Config, log *slog.Logger, pkgs []*packages.Package, r *rand.Rand, used map[string]struct{}) *Literals {
	return &Literals{
		cfg:  cfg,
		log:  log,
		pkgs: pkgs,
		r:    r,
		used: used,
	}
}
