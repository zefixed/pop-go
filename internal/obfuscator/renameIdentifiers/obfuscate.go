package renameIdentifiers

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
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

		// Build package-level rename map for consistent renaming across all files
		renameMap := i.buildRenameMap(pkg, pkg.PkgPath)

		for _, file := range pkg.Syntax {
			absPath := pkg.Fset.File(file.Pos()).Name()
			i.log.Debug(i.cfg.CurLocale["obf.debug.processing.file"], slog.String("filename", absPath))

			if err := i.applyRenameMap(file, pkg.TypesInfo, renameMap); err != nil {
				return fmt.Errorf(i.cfg.CurLocale["obf.err.ren.ids"], absPath, err)
			}
		}
	}

	i.log.Info(i.cfg.CurLocale["obf.info.end.ren.ids"], slog.String("duration", time.Since(t).String()))
	return nil
}

func (i *RenameIdentifiers) buildRenameMap(pkg *packages.Package, currentPkgPath string) map[types.Object]string {
	renameMap := make(map[types.Object]string)
	importedNames := i.collectImportedNames(pkg.Syntax)

	for _, file := range pkg.Syntax {
		// Collect type switch variables to exclude them from renaming
		typeSwitchVars := collectTypeSwitchVars(file)

		processed := make(map[token.Pos]bool)
		ast.Inspect(file, func(n ast.Node) bool {
			ident, ok := n.(*ast.Ident)
			if !ok || ident == nil || processed[ident.Pos()] {
				return true
			}
			processed[ident.Pos()] = true

			// Skip type switch variables - they have special scoping and type behavior
			if typeSwitchVars[ident.Pos()] {
				return true
			}

			if !shouldRename(ident, file, pkg.TypesInfo, currentPkgPath, importedNames) {
				return true
			}

			obj := pkg.TypesInfo.ObjectOf(ident)
			if obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == currentPkgPath {
				if _, exists := renameMap[obj]; !exists {
					renameMap[obj] = util.GenerateUniqueName(i.cfg.Obfuscator.Seed)
				}
			}

			return true
		})
	}

	return renameMap
}

// collectTypeSwitchVars collects all positions of type switch variables
// including the declaration and all uses within case clauses
func collectTypeSwitchVars(file *ast.File) map[token.Pos]bool {
	vars := make(map[token.Pos]bool)

	ast.Inspect(file, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSwitchStmt)
		if !ok || ts.Assign == nil {
			return true
		}

		// Get the type switch variable from the assignment
		var typeSwitchVar *ast.Ident
		if assign, ok := ts.Assign.(*ast.AssignStmt); ok && len(assign.Lhs) > 0 {
			if ident, ok := assign.Lhs[0].(*ast.Ident); ok {
				typeSwitchVar = ident
				vars[ident.Pos()] = true
			}
		}

		if typeSwitchVar == nil {
			return true
		}

		// Collect all uses of this variable within the type switch body
		ast.Inspect(ts.Body, func(inner ast.Node) bool {
			if ident, ok := inner.(*ast.Ident); ok && ident.Name == typeSwitchVar.Name {
				vars[ident.Pos()] = true
			}
			return true
		})

		return true
	})

	return vars
}

func (i *RenameIdentifiers) applyRenameMap(f *ast.File, typesInfo *types.Info, renameMap map[types.Object]string) error {
	astutil.Apply(f, nil, func(cursor *astutil.Cursor) bool {
		ident, ok := cursor.Node().(*ast.Ident)
		if !ok || ident == nil {
			return true
		}

		obj := typesInfo.ObjectOf(ident)
		if obj != nil {
			if newName, exists := renameMap[obj]; exists {
				ident.Name = newName
			}
		}

		return true
	})

	return nil
}

func (i *RenameIdentifiers) collectImportedNames(files []*ast.File) map[string]bool {
	importedNames := make(map[string]bool)
	for _, f := range files {
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
	}
	return importedNames
}

func shouldRename(ident *ast.Ident, file *ast.File, typesInfo *types.Info, currentPkgPath string, importedNames map[string]bool) bool {
	name := ident.Name
	if ast.IsExported(name) {
		return false
	}

	if name == "_" || name == "init" || name == "main" {
		return false
	}

	if isBuiltinType(name) {
		return false
	}

	if file.Name != nil && file.Name.Pos() == ident.Pos() {
		return false
	}

	if typesInfo != nil {
		obj := typesInfo.ObjectOf(ident)
		if obj == nil {
			return true
		}

		if _, ok := obj.(*types.PkgName); ok {
			return false
		}

		if obj.Pkg() == nil {
			return false
		}

		if obj.Pkg().Path() != currentPkgPath {
			return false
		}

		if isStandardPackagePath(obj.Pkg().Path(), currentPkgPath) {
			return false
		}
	}

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
