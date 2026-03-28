package util

import (
	"go/ast"
	"sort"

	"golang.org/x/tools/go/packages"
)

// SortedSyntax returns the AST files of pkg sorted by their absolute path.
// packages.Load does NOT guarantee a stable file order — it depends on the
// go tool's internal cache/parallelism state. If files are processed in
// different orders across runs (e.g. because "go test" was run beforehand
// and warmed the build cache), the PRNG produces different names for the
// same identifiers, causing "undefined" or "field not found" errors.
//
// Always use SortedSyntax instead of ranging over pkg.Syntax directly when
// the loop body calls GenerateUniqueName.
func SortedSyntax(pkg *packages.Package) []*ast.File {
	files := make([]*ast.File, len(pkg.Syntax))
	copy(files, pkg.Syntax)
	sort.Slice(files, func(i, j int) bool {
		pi := pkg.Fset.File(files[i].Pos()).Name()
		pj := pkg.Fset.File(files[j].Pos()).Name()
		return pi < pj
	})
	return files
}
