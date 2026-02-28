package renameIdentifiers

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/ast/astutil"
	"log/slog"
	"pop-go/pkg/util"
	"strings"
	"time"
)

// A workaround to prevent the obfuscation of package names
// that were not originally present in the source code but were injected by the obfuscator.
// Need to fix this in the future.
var standardPackageNames = map[string]bool{
	"aes": true, "cipher": true,
}

func (i *RenameIdentifiers) Obfuscate() error {
	i.log.Info(i.cfg.CurLocale["obf.info.start.ren.ids"])
	t := time.Now()

	for _, pkg := range i.pkgs {
		if pkg.TypesInfo == nil {
			i.log.Warn(i.cfg.CurLocale["obf.warn.pkg.types"],
				slog.String("pkg", pkg.PkgPath),
				slog.String("name", pkg.Name))
			continue
		}

		for _, file := range pkg.Syntax {
			absPath := pkg.Fset.File(file.Pos()).Name()
			i.log.Debug(i.cfg.CurLocale["obf.debug.processing.file"], slog.String("filename", absPath))

			// Modifying the AST
			if err := i.obfuscateAST(file, pkg.TypesInfo, pkg.PkgPath); err != nil {
				return fmt.Errorf(i.cfg.CurLocale["obf.err.ren.ids"], absPath, err)
			}
		}
	}

	i.log.Info(i.cfg.CurLocale["obf.info.end.ren.ids"], slog.String("duration", time.Since(t).String()))
	return nil
}

func (i *RenameIdentifiers) obfuscateAST(f *ast.File, typesInfo *types.Info, currentPkgPath string) error {
	renameMap := make(map[string]string)
	processed := make(map[token.Pos]bool)

	// Collecting all the names of the imported packages
	importedNames := make(map[string]bool)
	for _, imp := range f.Imports {
		if imp.Name != nil {
			if imp.Name.Name != "_" && imp.Name.Name != "." {
				importedNames[imp.Name.Name] = true
			}
		} else {
			path := strings.Trim(imp.Path.Value, `"`)
			parts := strings.Split(path, "/")
			importedNames[parts[len(parts)-1]] = true
		}
	}

	// Collect all the positions of the identifiers in SelectorExpr.X (for example, pkg in pkg.Func)
	selectorPositions := make(map[token.Pos]bool)
	ast.Inspect(f, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				selectorPositions[ident.Pos()] = true
			}
		}
		return true
	})

	// First pass: collecting identifiers for renaming
	ast.Inspect(f, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok || ident == nil || processed[ident.Pos()] {
			return true
		}
		processed[ident.Pos()] = true

		if !shouldRename(ident, f, typesInfo, currentPkgPath, importedNames, selectorPositions) {
			return true
		}

		name := ident.Name
		if _, exists := renameMap[name]; !exists {
			renameMap[name] = util.GenerateUniqueName(i.cfg.Obfuscator.Seed)
		}

		return true
	})

	// Second pass: replacing IDs
	astutil.Apply(f, nil, func(cursor *astutil.Cursor) bool {
		ident, ok := cursor.Node().(*ast.Ident)
		if !ok || ident == nil {
			return true
		}

		if !shouldRename(ident, f, typesInfo, currentPkgPath, importedNames, selectorPositions) {
			return true
		}

		if newName, exists := renameMap[ident.Name]; exists {
			ident.Name = newName
		}

		return true
	})

	return nil
}

func shouldRename(ident *ast.Ident, file *ast.File, typesInfo *types.Info, currentPkgPath string, importedNames map[string]bool, selectorPositions map[token.Pos]bool) bool {
	name := ident.Name
	// Fast path: Check conditions that don't require external data first
	// Skip exported identifiers early as they are common and easy to check
	if ast.IsExported(name) {
		return false
	}

	// Skip special identifiers like blank identifier, init, main
	if name == "_" || name == "init" || name == "main" {
		return false
	}

	// Skip built-in types and functions early before accessing complex data structures
	if isBuiltinType(name) {
		return false
	}

	// Skip the package name in the file declaration if this identifier matches it
	if file.Name != nil && file.Name.Pos() == ident.Pos() {
		return false
	}

	// Handle identifiers that appear on the left side of selectors (e.g., pkg.Func)
	// These are likely package names and should not be renamed
	if selectorPositions[ident.Pos()] {
		// Check against known imported names in this file
		if importedNames[name] {
			return false
		}
		// Check against standard library package names
		if standardPackageNames[name] {
			return false
		}
		// Use type information to confirm if this is a package name
		if typesInfo != nil {
			if obj := typesInfo.ObjectOf(ident); obj != nil {
				if _, ok := obj.(*types.PkgName); ok {
					return false
				}
			}
		}
	}

	// Perform additional checks only if type information is available
	if typesInfo != nil {
		obj := typesInfo.ObjectOf(ident)
		if obj == nil {
			// If no object information is available, we cannot make a decision based on types,
			// so proceed with renaming unless already excluded above
			return true
		}

		// Skip embedded fields or unqualified identifiers that have no associated package
		if obj.Pkg() == nil {
			return false
		}

		// Avoid renaming package names identified through type information
		// This check was duplicated earlier; now consolidated here
		if _, ok := obj.(*types.PkgName); ok {
			return false
		}

		// Skip identifiers originating from standard library packages
		// Compare package paths to determine origin
		if isStandardPackagePath(obj.Pkg().Path(), currentPkgPath) {
			return false
		}
	}

	// If all checks pass, this identifier can be safely renamed
	return true
}

func isBuiltinType(name string) bool {
	builtins := map[string]bool{
		"bool": true, "byte": true, "complex64": true, "complex128": true,
		"error": true, "float32": true, "float64": true, "int": true,
		"int8": true, "int16": true, "int32": true, "int64": true,
		"rune": true, "string": true, "uint": true, "uint8": true,
		"uint16": true, "uint32": true, "uint64": true, "uintptr": true,
		"append": true, "cap": true, "close": true, "complex": true,
		"copy": true, "delete": true, "imag": true, "len": true,
		"make": true, "new": true, "panic": true, "print": true,
		"println": true, "real": true, "recover": true,
		"true": true, "false": true, "iota": true, "nil": true,
	}
	return builtins[name]
}

func isStandardPackagePath(path, currentPkgPath string) bool {
	if path == "" || path == currentPkgPath {
		return false
	}
	if strings.Contains(path, ".") {
		return false
	}
	if strings.HasPrefix(path, "vendor/") {
		return false
	}
	return true
}
