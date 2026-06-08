package renameIdentifiers

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
	"log/slog"
	"os"
	"path/filepath"
	"pop-go/pkg/util"
	"strings"
	"time"
)

// standardPackageNames lists package names injected by the literals pass
// (e.g. "aes", "cipher") that must not be renamed even though they appear as
// unexported identifiers in the import-alias namespace.
// TODO: replace with proper detection of injected import aliases.
var standardPackageNames = map[string]bool{
	"aes": true, "cipher": true,
}

// Obfuscate renames every eligible unexported identifier across all loaded
// packages. For each package a rename map is built from the non-test source
// files, applied to the in-memory AST, and then propagated to internal test
// files via renameTestFiles.
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

		// Build package-level rename map for consistent renaming across all files.
		renameMap := i.buildRenameMap(pkg, pkg.PkgPath)

		for _, file := range util.SortedSyntax(pkg) {
			absPath := pkg.Fset.File(file.Pos()).Name()
			i.log.Debug(i.cfg.CurLocale["obf.debug.processing.file"], slog.String("filename", absPath))

			if err := i.applyRenameMap(file, pkg.TypesInfo, renameMap); err != nil {
				return fmt.Errorf(i.cfg.CurLocale["obf.err.ren.ids"], absPath, err)
			}
		}

		// Rename identifiers in _test.go files that belong to this package.
		// Test files are not in pkg.Syntax when packages are loaded without Tests mode,
		// so we find them by scanning the package directory and apply a name-based rename.
		if err := i.renameTestFiles(pkg, renameMap); err != nil {
			return err
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

	for _, file := range util.SortedSyntax(pkg) {
		if strings.HasSuffix(pkg.Fset.File(file.Pos()).Name(), "_test.go") {
			continue
		}
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

			obj := canonicalObject(pkg.TypesInfo.ObjectOf(ident))
			if obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == currentPkgPath {

				// ── Embedded (anonymous) field synchronisation ───────────────────
				// For `*runner` or `runner` anonymous fields the single AST ident is
				// in TypesInfo TWICE:
				//   Defs[ident] → *types.Var  (the promoted struct field)
				//   Uses[ident] → *types.TypeName  (the referenced type)
				// ObjectOf prefers Defs → obj = Var. Without special-casing, the
				// TypeName (a different types.Object) would get a DIFFERENT random
				// name. tcp.go would embed `*X` while runner.go defines `type Y` →
				// "undefined: X".
				// Fix: when we see an anonymous-field Var, retrieve the TypeName from
				// Uses and ensure both always share one name.
				if varObj, isVar := obj.(*types.Var); isVar && varObj.Anonymous() {
					typeNameObj := pkg.TypesInfo.Uses[ident]
					if typeNameObj != nil &&
						typeNameObj.Pkg() != nil &&
						typeNameObj.Pkg().Path() == currentPkgPath {
						existingVar, varHasName := renameMap[obj]
						existingType, typeHasName := renameMap[typeNameObj]
						switch {
						case varHasName && typeHasName:
							// Both already assigned — nothing to do.
						case varHasName:
							renameMap[typeNameObj] = existingVar
						case typeHasName:
							renameMap[obj] = existingType
						default:
							newName := util.GenerateUniqueName(r, i.used)
							renameMap[obj] = newName
							renameMap[typeNameObj] = newName
						}
						return true
					}
				}
				// ─────────────────────────────────────────────────────────────────

				if _, exists := renameMap[obj]; !exists {
					// For fields of anonymous structs, use canonical key for consistency.
					if field, ok := obj.(*types.Var); ok && field.IsField() {
						if canonicalKey := anonStructFieldKey(field, pkg.TypesInfo); canonicalKey != "" {
							if existing, ok := anonFieldNames[canonicalKey]; ok {
								renameMap[obj] = existing
							} else {
								newName := util.GenerateUniqueName(r, i.used)
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
							newName := util.GenerateUniqueName(r, i.used)
							renameMap[obj] = newName
							methodNames[origName] = newName
						}
						return true
					}

					renameMap[obj] = util.GenerateUniqueName(r, i.used)
				}
			}

			return true
		})
	}

	// ── Embedded-type post-pass ─────────────────────────────────────────────
	// An embedded field like *stop has TWO distinct types.Object:
	//   Var   (the promoted field, keyed by Defs[ident] of the embed line)
	//   TypeName (the type, keyed by Defs[ident] of the type declaration)
	//
	// The Uses-based sync in the main loop can miss some cases, e.g. when the
	// identifier is in a selector expression (s.stop) where Uses returns the
	// Var itself, not the TypeName. This post-pass ensures both always share
	// one name by resolving Var.Type() → Named.Obj() → TypeName directly.
	for varObj, varName := range renameMap {
		v, ok := varObj.(*types.Var)
		if !ok || !v.Anonymous() {
			continue
		}
		typ := v.Type()
		if ptr, isPtr := typ.(*types.Pointer); isPtr {
			typ = ptr.Elem()
		}
		named, ok := typ.(*types.Named)
		if !ok {
			continue
		}
		tnObj := named.Obj()
		if tnObj == nil || tnObj.Pkg() == nil || tnObj.Pkg().Path() != currentPkgPath {
			continue
		}
		existing, exists := renameMap[tnObj]
		switch {
		case !exists:
			// TypeName not yet renamed — give it the same name as the Var.
			renameMap[tnObj] = varName
		case existing == varName:
			// Already in sync.
		default:
			// Names diverged — the type DECLARATION is authoritative.
			renameMap[varObj] = existing
		}
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

// collectTypeSwitchVars returns the set of source positions occupied by type
// switch variables — both the declaration site and every use within the switch
// body. Variables bound by a type switch (e.g. "v" in "switch v := x.(type)")
// implicitly change their type in each case clause. Renaming them would
// produce conflicting declarations, so the rename pass skips all of them.
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

// applyRenameMap renames every identifier in f whose resolved types.Object
// appears as a key in renameMap. The walk uses astutil.Apply so that cursor
// replacements are applied immediately; returning true continues the traversal.
func (i *RenameIdentifiers) applyRenameMap(f *ast.File, typesInfo *types.Info, renameMap map[types.Object]string) error {
	posRenameMap := make(map[token.Pos]string, len(renameMap))
	for obj, newName := range renameMap {
		if obj != nil && obj.Pos() != token.NoPos {
			posRenameMap[obj.Pos()] = newName
		}
	}

	astutil.Apply(f, nil, func(cursor *astutil.Cursor) bool {
		ident, ok := cursor.Node().(*ast.Ident)
		if !ok || ident == nil {
			return true
		}

		obj := canonicalObject(typesInfo.ObjectOf(ident))
		if obj != nil {
			if newName, exists := renameMap[obj]; exists {
				ident.Name = newName
			} else if newName, exists := posRenameMap[obj.Pos()]; exists {
				// Generic instantiations can materialize fresh field objects for uses in
				// keyed composite literals and selectors. Their declaration position stays
				// stable, so fall back to Pos() to keep the use-site name in sync.
				ident.Name = newName
			}
		}

		return true
	})

	return nil
}

func canonicalObject(obj types.Object) types.Object {
	switch o := obj.(type) {
	case *types.Var:
		return o.Origin()
	case *types.Func:
		return o.Origin()
	default:
		return obj
	}
}

// renameTestFiles finds all internal _test.go files in pkg's directory
// (those with "package <pkg.Name>", not "package <pkg.Name>_test")
// and renames identifiers according to renameMap.
//
// Because packages.Load without the Tests flag does not include test files in
// pkg.Syntax, we cannot use pkg.TypesInfo to resolve test-file identifiers.
// Instead we build a name-based lookup from the package-scope rename map and
// apply it to test files parsed fresh. This correctly handles the common case
// of test files calling unexported package-level functions, types, and variables.
//
// Limitation: unexported struct fields that share the same base name across
// multiple structs and were renamed to different identifiers cannot be
// distinguished without type info; in that case the name is removed from the
// lookup map and left unrenamed.
func (i *RenameIdentifiers) renameTestFiles(pkg *packages.Package, renameMap map[types.Object]string) error {
	if len(pkg.GoFiles) == 0 {
		return nil
	}

	// ── Build name-based lookup ────────────────────────────────────────────
	// We intentionally include ALL renamed objects (not only package-scope).
	// Methods and struct fields are also accessible from internal test files.
	// Conflict resolution: if the same original name maps to two different new
	// names (e.g., field x in struct A → xa, field x in struct B → xb), we
	// remove the entry to avoid silently producing an incorrect rename.
	type nameEntry struct {
		newName   string
		conflicts bool
	}
	entries := make(map[string]*nameEntry)
	for obj, newName := range renameMap {
		origName := obj.Name()
		if e, ok := entries[origName]; ok {
			if e.newName != newName {
				e.conflicts = true
			}
		} else {
			entries[origName] = &nameEntry{newName: newName}
		}
	}
	nameMap := make(map[string]string, len(entries))
	for origName, e := range entries {
		if !e.conflicts {
			nameMap[origName] = e.newName
		}
	}
	if len(nameMap) == 0 {
		return nil
	}

	structFieldMap := i.buildStructFieldRenameMap(pkg, renameMap)

	// ── Scan directory for internal _test.go files ─────────────────────────
	dir := filepath.Dir(pkg.GoFiles[0])

	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		// Not critical — just skip
		return nil
	}

	fset := token.NewFileSet()

	for _, de := range dirEntries {
		if de.IsDir() || !strings.HasSuffix(de.Name(), "_test.go") {
			continue
		}

		testPath := filepath.Join(dir, de.Name())

		f, err := parser.ParseFile(fset, testPath, nil, parser.ParseComments)
		if err != nil {
			i.log.Warn("failed to parse test file",
				slog.String("file", testPath),
				slog.String("error", err.Error()))
			continue
		}

		// Only process internal test files (same package name).
		// External test files (package foo_test) can only access exported
		// identifiers which we never rename, so they need no changes.
		if f.Name == nil || f.Name.Name != pkg.Name {
			continue
		}

		i.log.Debug(i.cfg.CurLocale["obf.debug.processing.file"], slog.String("filename", testPath))

		i.applyNameRenameMap(f, nameMap)
		i.applyStructFieldRenameMap(f, structFieldMap)

		var buf bytes.Buffer
		if err := format.Node(&buf, fset, f); err != nil {
			i.log.Warn("failed to format test file",
				slog.String("file", testPath),
				slog.String("error", err.Error()))
			continue
		}

		if err := os.WriteFile(testPath, buf.Bytes(), 0644); err != nil {
			return fmt.Errorf("failed to write test file %s: %w", testPath, err)
		}
	}

	return nil
}

func (i *RenameIdentifiers) buildStructFieldRenameMap(pkg *packages.Package, renameMap map[types.Object]string) map[string]map[string]string {
	typeMaps := make(map[string]map[string]string)

	for _, file := range util.SortedSyntax(pkg) {
		ast.Inspect(file, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return true
			}

			typeNames := []string{ts.Name.Name}
			if obj := pkg.TypesInfo.Defs[ts.Name]; obj != nil {
				typeNames[0] = obj.Name()
				if newName, ok := renameMap[obj]; ok && newName != obj.Name() {
					typeNames = append(typeNames, newName)
				}
			}

			fieldRenames := make(map[string]string)
			for _, field := range st.Fields.List {
				for _, name := range field.Names {
					if name == nil {
						continue
					}
					obj := canonicalObject(pkg.TypesInfo.Defs[name])
					if obj == nil {
						continue
					}
					if newName, ok := renameMap[obj]; ok && newName != obj.Name() {
						fieldRenames[obj.Name()] = newName
					}
				}
			}

			if len(fieldRenames) == 0 {
				return true
			}

			for _, typeName := range typeNames {
				typeMaps[typeName] = fieldRenames
			}
			return true
		})
	}

	return typeMaps
}

// applyNameRenameMap walks the AST of f and renames any unexported identifier
// whose name appears in nameMap.  It skips the package declaration, exported
// identifiers, builtin names, and the blank identifier.
func (i *RenameIdentifiers) applyNameRenameMap(f *ast.File, nameMap map[string]string) {
	pkgNamePos := token.NoPos
	if f.Name != nil {
		pkgNamePos = f.Name.Pos()
	}

	astutil.Apply(f, nil, func(cursor *astutil.Cursor) bool {
		ident, ok := cursor.Node().(*ast.Ident)
		if !ok || ident == nil {
			return true
		}

		// Skip package declaration
		if ident.Pos() == pkgNamePos {
			return true
		}

		// Never rename exported, blank, or special identifiers
		name := ident.Name
		if ast.IsExported(name) || name == "_" || name == "init" || name == "main" {
			return true
		}

		if isBuiltinType(name) {
			return true
		}

		if newName, exists := nameMap[name]; exists {
			ident.Name = newName
		}

		return true
	})
}

func (i *RenameIdentifiers) applyStructFieldRenameMap(f *ast.File, structFieldMap map[string]map[string]string) {
	ast.Inspect(f, func(n ast.Node) bool {
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}

		typeName := compositeLitTypeName(cl.Type)
		if typeName == "" {
			return true
		}

		fieldMap, ok := structFieldMap[typeName]
		if !ok {
			return true
		}

		for _, elt := range cl.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			if newName, ok := fieldMap[key.Name]; ok {
				key.Name = newName
			}
		}

		return true
	})
}

func compositeLitTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return t.Sel.Name
	case *ast.IndexExpr:
		return compositeLitTypeName(t.X)
	case *ast.IndexListExpr:
		return compositeLitTypeName(t.X)
	default:
		return ""
	}
}

// collectImportedNames returns the set of local package names introduced by
// the import declarations of files. Both aliased imports (import foo "pkg")
// and non-aliased imports (where the local name equals the last path segment)
// are included. These names must not be renamed because they are references to
// external packages, not declarations in the current package.
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

// shouldRename reports whether ident is eligible for renaming. An identifier
// is skipped when it is exported, is a blank identifier, is a builtin name, is
// the package declaration itself, refers to an imported package name, belongs
// to a different package, or belongs to a standard-library package path.
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

// isBuiltinType reports whether name is a predeclared Go identifier: a builtin
// type, builtin function, or predeclared constant. Such identifiers must never
// be renamed because they are not user-defined declarations.
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

// isStandardPackagePath reports whether path is a standard-library import path.
// Standard-library paths contain no dots, are non-empty, and do not start with
// "vendor/". The currentPkgPath check avoids misidentifying the package under
// obfuscation as a stdlib package.
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
