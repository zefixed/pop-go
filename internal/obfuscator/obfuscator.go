// Package obfuscator coordinates all obfuscation passes over the target
// project. It loads the project's packages with full type information once and
// distributes the shared AST and TypesInfo to each pass.
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
	"sort"
	"strings"
)

// Obfuscator holds the loaded packages and all enabled transformation passes.
// CritErr is set by NewObfuscator when package loading fails; callers must
// check it before calling Obfuscate.
type Obfuscator struct {
	cfg               *models.Config
	log               *slog.Logger
	pkgs              []*packages.Package
	literals          *literals.Literals
	renameIdentifiers *renameIdentifiers.RenameIdentifiers
	controlFlow       *cff.CFF
	CritErr           error
}

// NewObfuscator loads all packages from cfg.Obfuscator.TargetPath with full
// syntax and type information, filters out test-variant and external-test
// packages, sorts them deterministically, and initialises every obfuscation
// pass with a shared PRNG and global name-collision set.
//
// Package filtering rules:
//   - "pkg/foo"                — canonical package             → kept
//   - "pkg/foo [pkg/foo.test]" — test-binary variant           → skipped
//   - "pkg/foo_test"           — external _test package        → skipped
//
// Test-variant packages are skipped here because processing them alongside
// their canonical counterparts would produce a second, conflicting rename map
// for the same source files. Internal test files are handled separately by the
// renameIdentifiers pass via directory scanning.
//
// Packages are sorted by import path so the PRNG advances in the same order
// on every run regardless of the non-deterministic order returned by
// packages.Load (which depends on the Go toolchain's cache state).
func NewObfuscator(cfg *models.Config, log *slog.Logger) *Obfuscator {
	obfCfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedCompiledGoFiles |
			packages.NeedImports |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedSyntax,
		Fset: token.NewFileSet(),
		Dir:  cfg.Obfuscator.TargetPath,
	}

	pkgsRaw, err := packages.Load(obfCfg, "./...")
	if err != nil {
		return &Obfuscator{
			CritErr: fmt.Errorf("%s: %w", cfg.CurLocale["obf.err.load.pkg"], err),
		}
	}

	seen := make(map[string]bool)
	pkgs := make([]*packages.Package, 0, len(pkgsRaw))
	for _, p := range pkgsRaw {
		if p.PkgPath == "" || seen[p.PkgPath] {
			continue
		}
		if strings.Contains(p.PkgPath, " [") || strings.HasSuffix(p.PkgPath, "_test") {
			continue
		}
		seen[p.PkgPath] = true
		pkgs = append(pkgs, p)
	}

	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].PkgPath < pkgs[j].PkgPath })

	// A single PRNG and a global used-names set are shared across all passes so
	// that every GenerateUniqueName call advances the same sequence. This ensures
	// names are globally unique even with a fixed seed and prevents collisions
	// between variables hoisted into the same scope by CFF.
	r := util.NewRand(cfg.Obfuscator.Seed)
	used := make(map[string]struct{})

	return &Obfuscator{
		cfg:               cfg,
		log:               log,
		pkgs:              pkgs,
		literals:          literals.NewLiterals(cfg, log, pkgs, r, used),
		renameIdentifiers: renameIdentifiers.NewRenameIdentifiers(cfg, log, pkgs, r, used),
		controlFlow:       cff.NewCFF(cfg, log, pkgs, r, used),
	}
}
