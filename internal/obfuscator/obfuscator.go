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

	// Filter out test-variant packages which share source files but carry
	// different TypesInfo objects — processing them would produce a second,
	// conflicting renameMap for the same files.
	//   "pkg/foo"                — canonical package           ← keep
	//   "pkg/foo [pkg/foo.test]" — test-binary variant         ← skip
	//   "pkg/foo_test"           — external _test package      ← skip
	seen := make(map[string]bool)
	pkgs := make([]*packages.Package, 0, len(pkgsRaw))
	for _, p := range pkgsRaw {
		if p.PkgPath == "" || seen[p.PkgPath] {
			continue
		}
		// Skip test-variant packages. packages.Load returns packages in several forms:
		//   "pkg/foo"              — the real package  ← keep
		//   "pkg/foo [pkg/foo.test]" — test binary variant of the package ← skip
		//   "pkg/foo_test"         — external test package ← skip
		//
		// Test variants share source files with the real package but have different
		// TypesInfo objects. Processing them would produce a second, conflicting
		// renameMap for the same files, causing "undefined" errors after WriteAll
		// overwrites the file with the second pass's renames.
		if strings.Contains(p.PkgPath, " [") || strings.HasSuffix(p.PkgPath, "_test") {
			continue
		}
		seen[p.PkgPath] = true
		pkgs = append(pkgs, p)
	}

	// Sort packages by PkgPath so the PRNG advances in the same order
	// regardless of how packages.Load ordered them. packages.Load order
	// depends on the go tool's internal parallelism / cache state and can
	// differ between runs (e.g. when "go test" was run beforehand and
	// warmed the cache). Without sorting, --test and --test=false produce
	// different rename maps for the same source → "undefined" build errors.
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].PkgPath < pkgs[j].PkgPath })

	// One rand and one global "used names" set for the entire obfuscation pass.
	// All modules share both so:
	//   • Every GenerateUniqueName call advances the same PRNG — names are
	//     globally unique even with a fixed seed (e.g. --seed 1).
	//   • The used set guarantees no two identifiers collide even if the PRNG
	//     happens to emit the same string twice (e.g. with small seeds on large
	//     codebases where CFF hoists vars from different scopes into one scope).
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
