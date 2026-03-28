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

	// Shared rand from the obfuscator — guarantees all modules in one pass
	// advance the same PRNG, so names are unique even across modules.
	r := i.r

	// Secondary map for anonymous struct fields: (structTypeString + "." + fieldName) → newName.
	anonFieldNames := make(map[string]string)

	// Secondary map for methods: originalMethodName → newName.
	// Interface methods and their implementing struct methods are different types.Object
	// entries, but they must receive the SAME renamed identifier. Without this map,
	// renaming "available" on the interface and "available" on the struct independently
	// would produce different names, breaking the interface satisfaction check.
	methodNames := make(map[string]string)

	for _, file := range pkg.Syntax {
		typeSwitchVars := collectTypeSwitchVars(file)

		processed := make(map[token.Pos]bool)
		ast.Inspect(file, func(n ast.Node) bool {
			ident, ok := n.(*ast.Ident)
			if !ok || ident == nil || processed[ident.Pos()] {
				return true
			}
			processed[ident.Pos()] = true

			if typeSwitchVars[ident.Pos()] {
				return true
			}

			if !shouldRename(ident, file, pkg.TypesInfo, currentPkgPath, importedNames) {
				return true
			}

			obj := pkg.TypesInfo.ObjectOf(ident)
			if obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == currentPkgPath {
				if _, exists := renameMap[obj]; !exists {
					// For fields of anonymous structs, use canonical key for consistency.
					if field, ok := obj.(*types.Var); ok && field.IsField() {
						if canonicalKey := anonStructFieldKey(field, pkg.TypesInfo); canonicalKey != "" {
							if existing, ok := anonFieldNames[canonicalKey]; ok {
								renameMap[obj] = existing
							} else {
								newName := util.GenerateUniqueName(r)
								renameMap[obj] = newName
								anonFieldNames[canonicalKey] = newName
							}
							return true
						}
					}

					// For methods (interface or struct), use the original method name as
					// a grouping key so that all methods named e.g. "available" in this
					// package receive the same obfuscated name. This ensures interface
					// methods and their implementing struct methods stay in sync.
					if fn, ok := obj.(*types.Func); ok && fn.Type().(*types.Signature).Recv() != nil {
						origName := ident.Name
						if existing, ok := methodNames[origName]; ok {
							renameMap[obj] = existing
						} else {
							newName := util.GenerateUniqueName(r)
							renameMap[obj] = newName
							methodNames[origName] = newName
						}
						return true
					}

					renameMap[obj] = util.GenerateUniqueName(r)
				}
			}

			return true
		})
	}

	return renameMap
}

// anonStructFieldKey returns a canonical string key for a field of an anonymous struct type.
// The key is "<structTypeString>.<fieldName>", e.g. "struct{ip string; domain string}.ip".
// Returns "" if the field belongs to a named (non-anonymous) struct — those are keyed by
// their types.Object directly, which is already unique and stable.
func anonStructFieldKey(field *types.Var, typesInfo *types.Info) string {
	if typesInfo == nil {
		return ""
	}
	// Walk all types in the package to find the anonymous struct containing this field.
	for expr, tv := range typesInfo.Types {
		st, ok := tv.Type.(*types.Struct)
		if !ok {
			continue
		}
		// Only anonymous structs — named structs are handled by their object key.
		if _, isNamed := expr.(*ast.StructType); !isNamed {
			continue
		}
		for fi := 0; fi < st.NumFields(); fi++ {
			if st.Field(fi) == field {
				// Found the anonymous struct containing this field.
				return types.TypeString(st, nil) + "." + field.Name()
			}
		}
	}
	return ""
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
