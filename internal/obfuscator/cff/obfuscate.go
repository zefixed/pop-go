package cff

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"log/slog"
	"pop-go/pkg/util"
	"strings"
	"time"

	"golang.org/x/tools/go/packages"
)

// Obfuscate executes the control flow flattening obfuscation process.
// It iterates through all packages and files, applying the transformation
// to eligible functions while logging progress and errors.
func (f *CFF) Obfuscate() {
	f.log.Info(f.cfg.CurLocale["obf.info.start.cff"])
	t := time.Now()
	for _, pkg := range f.pkgs {
		for _, file := range util.SortedSyntax(pkg) {
			absPath := pkg.Fset.File(file.Pos()).Name()
			if strings.HasSuffix(absPath, "_test.go") {
				continue
			}
			f.log.Debug(f.cfg.CurLocale["obf.debug.processing.file"], slog.String("filename", absPath))
			if err := f.flattenFile(file, pkg); err != nil {
				f.log.Error(f.cfg.CurLocale["obf.err.cff"], slog.String("file", absPath), slog.Any("error", err))
				continue
			}
		}
	}
	f.log.Info(f.cfg.CurLocale["obf.info.end.cff"], slog.String("duration", time.Since(t).String()))
}

// flattenFile processes a Go AST file and applies control flow flattening
// to eligible functions. Functions containing concurrency primitives (goroutines,
// channels) are skipped to avoid synchronization issues.
func (f *CFF) flattenFile(file *ast.File, pkg *packages.Package) error {
	// Build import alias map for THIS file only.
	// Must be per-file because different files in the same package may import
	// the same package under different aliases (or no alias at all). Using a
	// package-wide map would cause CFF to emit the wrong qualifier in files
	// that don't use the alias (e.g. stdhttp.Flusher in a file that imports
	// "net/http" without an alias, causing "undefined: stdhttp").
	filePkgAliases := make(map[string]string) // importPath → localName
	for _, imp := range file.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		if imp.Name != nil && imp.Name.Name != "_" && imp.Name.Name != "." {
			filePkgAliases[importPath] = imp.Name.Name
		}
	}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || len(fn.Body.List) < 2 {
			continue
		}
		if f.hasConcurrency(fn.Body.List) {
			f.log.Debug("skipping function with concurrency", slog.String("function", fn.Name.Name))
			continue
		}
		if f.hasAnonymousStructTypeAssertion(fn.Body.List) {
			f.log.Debug("skipping function with anonymous struct type assertion", slog.String("function", fn.Name.Name))
			continue
		}
		if f.hasLocalConst(fn.Body.List) {
			f.log.Debug("skipping function with local const declaration", slog.String("function", fn.Name.Name))
			continue
		}
		if f.hasLocalTypeDecl(fn.Body.List) {
			f.log.Debug("skipping function with local type declaration", slog.String("function", fn.Name.Name))
			continue
		}
		if f.hasGoto(fn.Body.List) {
			f.log.Debug("skipping function with goto statement", slog.String("function", fn.Name.Name))
			continue
		}

		imports := make(map[string]string)
		if err := f.flattenFunction(fn, pkg, filePkgAliases, imports); err != nil {
			return err
		}
		addImportsToFile(file, imports)
	}
	return nil
}

// hasConcurrency checks if a list of statements contains any concurrency
// primitives such as goroutines (go statements) or channel operations.
// Returns true if concurrency is detected, false otherwise.
func (f *CFF) hasConcurrency(stmts []ast.Stmt) bool {
	for _, stmt := range stmts {
		if f.containsGoStmt(stmt) || f.containsChannelOp(stmt) {
			return true
		}
	}
	return false
}

// containsGoStmt recursively checks if a statement contains a goroutine
// launch (go statement). Handles nested blocks, if/for statements.
func (f *CFF) containsGoStmt(stmt ast.Stmt) bool {
	switch s := stmt.(type) {
	case *ast.GoStmt:
		return true
	case *ast.BlockStmt:
		for _, inner := range s.List {
			if f.containsGoStmt(inner) {
				return true
			}
		}
	case *ast.IfStmt:
		if s.Init != nil && f.containsGoStmt(s.Init) {
			return true
		}
		if f.containsGoStmt(s.Body) {
			return true
		}
		if s.Else != nil {
			if elseBlock, ok := s.Else.(*ast.BlockStmt); ok {
				if f.containsGoStmt(elseBlock) {
					return true
				}
			}
		}
	case *ast.ForStmt:
		if s.Init != nil && f.containsGoStmt(s.Init) {
			return true
		}
		if f.containsGoStmt(s.Body) {
			return true
		}
	}
	return false
}

// containsChannelOp recursively checks if a statement contains channel
// operations: send (ch <- val), receive (<-ch), or channel creation (make(chan T)).
// Uses ast.Inspect for comprehensive traversal of nested expressions.
func (f *CFF) containsChannelOp(stmt ast.Stmt) bool {
	found := false
	ast.Inspect(stmt, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.UnaryExpr:
			// Receive operation: <-ch
			if u, ok := n.(*ast.UnaryExpr); ok && u.Op == token.ARROW {
				found = true
				return false
			}
		case *ast.SendStmt:
			// Send operation: ch <- value
			found = true
			return false
		case *ast.CallExpr:
			// Channel creation: make(chan T) — but NOT make([]T) or make(map[K]V).
			// Check that the first argument is a chan type expression.
			call, ok := n.(*ast.CallExpr)
			if !ok {
				break
			}
			ident, ok := call.Fun.(*ast.Ident)
			if !ok || ident.Name != "make" {
				break
			}
			if len(call.Args) > 0 {
				if _, isChan := call.Args[0].(*ast.ChanType); isChan {
					found = true
					return false
				}
			}
		}
		return true
	})
	return found
}

// VarInfo stores metadata about a variable for hoisting during
// control flow flattening. Tracks name, inferred type, and whether
// the variable is a function parameter (which should not be re-declared).
type VarInfo struct {
	Name    string
	Type    ast.Expr
	IsParam bool
}

// flattenFunction transforms a function body into a flattened control flow
// structure using a state machine pattern. The original statements are
// converted into switch cases controlled by a state variable within a
// for-loop dispatcher. Variable declarations are hoisted to function scope
// to maintain visibility across case boundaries.
func (f *CFF) flattenFunction(fn *ast.FuncDecl, pkg *packages.Package, pkgAliases map[string]string, imports map[string]string) error {
	hoistedVars := make(map[string]bool)
	stateVarName := util.GenerateUniqueName(f.r, f.used)
	originalStmts := fn.Body.List
	typesInfo := pkg.TypesInfo
	currentPkgPath := pkg.Types.Path() // Use full import path for accurate type comparison

	// Pre-build position → type map from typesInfo.Defs.
	// Using token.Pos as key avoids pointer-equality issues that can cause
	// typesInfo.Defs[ident] to miss entries (e.g. when AST nodes are reused
	// or the map is keyed by a different pointer than the one we hold).
	posToType := make(map[token.Pos]types.Type)
	if typesInfo != nil && typesInfo.Defs != nil {
		for defIdent, obj := range typesInfo.Defs {
			if obj != nil && obj.Type() != nil {
				posToType[defIdent.Pos()] = obj.Type()
			}
		}
	}

	// Build a map from original declaration position → current name in the AST.
	// This is necessary because renameIdentifiers may have already run and
	// renamed type names and struct field names in-place. typesInfo still
	// holds the original names, so we must resolve current names from the AST
	// to avoid generating hoisted declarations with stale (undefined) type names.
	//
	// IMPORTANT: skip idents with Pos() == token.NoPos (== 0). Synthetic idents
	// created by CFF (state variable names) have no position. Built-in types like
	// `error` also have obj.Pos() == 0, so if we stored a synthetic name at key 0,
	// typeToAST would rename `error` to the state variable name.
	currentNames := make(map[token.Pos]string)
	for _, file := range util.SortedSyntax(pkg) {
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.TypeSpec:
				if node.Name != nil && node.Name.Pos() != token.NoPos {
					currentNames[node.Name.Pos()] = node.Name.Name
				}
			case *ast.Field:
				for _, name := range node.Names {
					if name.Pos() != token.NoPos {
						currentNames[name.Pos()] = name.Name
					}
				}
			case *ast.Ident:
				if node.Pos() != token.NoPos {
					currentNames[node.Pos()] = node.Name
				}
			}
			return true
		})
	}

	// Build a map from import path → local alias used in this package's files.
	// When a package is imported with an alias (e.g. stdhttp "net/http"), typeToAST
	// must use that alias rather than the canonical pkg.Name(), otherwise the generated
	// hoisted var declaration would reference an undefined identifier.
	// NOTE: pkgAliases is now passed in from flattenFile (per-file) to avoid applying
	// an alias from one file to all other files in the same package.

	varInfoMap := make(map[string]*VarInfo)

	if fn.Type.Params != nil {
		for _, param := range fn.Type.Params.List {
			for _, name := range param.Names {
				varInfoMap[name.Name] = &VarInfo{Name: name.Name, Type: param.Type, IsParam: true}
			}
		}
	}

	// Named return values are implicitly declared at function scope — exactly
	// like parameters. If CFF hoists them again as "var x T" a redeclaration
	// compile error occurs. Mark them IsParam=true so the hoisting loop skips them.
	if fn.Type.Results != nil {
		for _, result := range fn.Type.Results.List {
			for _, name := range result.Names {
				if name.Name != "_" {
					varInfoMap[name.Name] = &VarInfo{Name: name.Name, Type: result.Type, IsParam: true}
				}
			}
		}
	}

	for _, stmt := range originalStmts {
		f.collectVarsWithTypes(stmt, varInfoMap, typesInfo, posToType, currentPkgPath, currentNames, pkgAliases, imports)
	}
	for _, stmt := range originalStmts {
		f.resolveVarTypes(stmt, varInfoMap, typesInfo, posToType, currentPkgPath, currentNames, pkgAliases, imports)
	}

	var hoistedDecls []ast.Stmt
	for _, v := range varInfoMap {
		if v.IsParam {
			continue
		}
		// Skip blank identifier — it cannot be declared as a named variable
		// and any import added for its type would be flagged as unused.
		if v.Name == "_" {
			continue
		}
		hoistedVars[v.Name] = true
		hoistedDecls = append(hoistedDecls, &ast.DeclStmt{
			Decl: &ast.GenDecl{
				Tok: token.VAR,
				Specs: []ast.Spec{
					&ast.ValueSpec{Names: []*ast.Ident{{Name: v.Name}}, Type: v.Type},
				},
			},
		})
	}

	var cases []ast.Stmt
	n := len(originalStmts)
	// Build a zero-value return statement for use as the terminal transition.
	// A plain `return` without values is only valid for void functions.
	// For functions with return types we must return zero values to satisfy the compiler.
	// (This return is always unreachable in practice — it only follows the last real
	// statement which is itself a return — but the compiler still type-checks it.)
	// Use types.Type-aware builder so interface return types (error, cipher.AEAD, etc.)
	// produce nil instead of the invalid composite literal T{}.
	var terminalReturn *ast.ReturnStmt
	if typesInfo != nil {
		if obj := typesInfo.Defs[fn.Name]; obj != nil {
			if sig, ok := obj.Type().(*types.Signature); ok {
				terminalReturn = f.buildZeroReturnFromSig(sig, currentPkgPath, currentNames, pkgAliases, imports)
			}
		}
	}
	if terminalReturn == nil {
		terminalReturn = f.buildZeroReturn(fn.Type.Results)
	}

	for i, stmt := range originalStmts {
		processed := f.convertDefineToAssign(stmt, hoistedVars)
		var trans ast.Stmt
		if i == n-1 {
			trans = terminalReturn
		} else {
			trans = &ast.AssignStmt{
				Tok: token.ASSIGN,
				Lhs: []ast.Expr{&ast.Ident{Name: stateVarName}},
				Rhs: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", i+1)}},
			}
		}
		cases = append(cases, &ast.CaseClause{
			List: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", i)}},
			Body: []ast.Stmt{&ast.BlockStmt{List: []ast.Stmt{processed, trans}}},
		})
	}
	cases = append(cases, &ast.CaseClause{Body: []ast.Stmt{terminalReturn}})

	finalBody := append(hoistedDecls, &ast.AssignStmt{
		Tok: token.DEFINE,
		Lhs: []ast.Expr{&ast.Ident{Name: stateVarName}},
		Rhs: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "0"}},
	})
	finalBody = append(finalBody, &ast.ForStmt{
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.SwitchStmt{
			Tag:  &ast.Ident{Name: stateVarName},
			Body: &ast.BlockStmt{List: cases},
		}}},
	})
	fn.Body.List = finalBody
	return nil
}

// collectVarsWithTypes recursively traverses statements to collect variables
// declared with short declaration syntax (:=). Infers types from RHS expressions
// and stores them in varMap for later hoisting. Handles nested blocks, if/for/range.
func (f *CFF) collectVarsWithTypes(stmt ast.Stmt, varMap map[string]*VarInfo, typesInfo *types.Info, posToType map[token.Pos]types.Type, currentPkgPath string, currentNames map[token.Pos]string, pkgAliases map[string]string, imports map[string]string) {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		if s.Tok == token.DEFINE {
			for i, lhs := range s.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok {
					// Skip blank identifier — it can't be hoisted as a named var
					if ident.Name == "_" {
						continue
					}
					if _, exists := varMap[ident.Name]; !exists {
						var varType ast.Expr

						// Most reliable: look up by token position (avoids pointer-equality
						// issues with typesInfo.Defs[ident] direct lookup).
						if typ, found := posToType[ident.Pos()]; found && typ != nil {
							// For types that contain anonymous structs, typeToAST reconstructs
							// field names from *types.Struct which may not match the renamed names
							// in the current AST (multiple anonymous structs with the same shape
							// get different renamed field names). Use inferTypeFromExpr instead,
							// which reads the type directly from the RHS AST node.
							if containsAnonymousStruct(typ) {
								rhsIdx := i
								if rhsIdx >= len(s.Rhs) {
									rhsIdx = len(s.Rhs) - 1
								}
								if rhsIdx >= 0 {
									varType = f.inferTypeFromExpr(s.Rhs[rhsIdx], varMap, typesInfo, currentPkgPath, currentNames, pkgAliases, imports)
									varMap[ident.Name] = &VarInfo{Name: ident.Name, Type: varType, IsParam: false}
									continue
								}
							}
							varType = f.typeToAST(typ, currentPkgPath, currentNames, pkgAliases, imports)
							varMap[ident.Name] = &VarInfo{Name: ident.Name, Type: varType, IsParam: false}
							continue
						}

						// Fallback: direct Defs lookup
						if typesInfo != nil {
							if obj := typesInfo.Defs[ident]; obj != nil && obj.Type() != nil {
								if containsAnonymousStruct(obj.Type()) {
									rhsIdx := i
									if rhsIdx >= len(s.Rhs) {
										rhsIdx = len(s.Rhs) - 1
									}
									if rhsIdx >= 0 {
										varType = f.inferTypeFromExpr(s.Rhs[rhsIdx], varMap, typesInfo, currentPkgPath, currentNames, pkgAliases, imports)
										varMap[ident.Name] = &VarInfo{Name: ident.Name, Type: varType, IsParam: false}
										continue
									}
								}
								varType = f.typeToAST(obj.Type(), currentPkgPath, currentNames, pkgAliases, imports)
								varMap[ident.Name] = &VarInfo{Name: ident.Name, Type: varType, IsParam: false}
								continue
							}
						}

						// Check if RHS is a type assertion: x, ok := expr.(T)
						if i < len(s.Rhs) {
							if ta, ok := s.Rhs[i].(*ast.TypeAssertExpr); ok && ta.Type != nil {
								if i == 0 {
									varType = ta.Type
								} else {
									varType = &ast.Ident{Name: "bool"}
								}
								varMap[ident.Name] = &VarInfo{Name: ident.Name, Type: varType, IsParam: false}
								continue
							}
						}

						// For multi-value function returns, use types.Info to get correct types per index
						if len(s.Lhs) > 1 && len(s.Rhs) == 1 && typesInfo != nil {
							if tv, ok := typesInfo.Types[s.Rhs[0]]; ok && tv.Type != nil {
								if tuple, ok := tv.Type.(*types.Tuple); ok && tuple.Len() == len(s.Lhs) {
									varType = f.typeToAST(tuple.At(i).Type(), currentPkgPath, currentNames, pkgAliases, imports)
									varMap[ident.Name] = &VarInfo{Name: ident.Name, Type: varType, IsParam: false}
									continue
								}
							}
							if callExpr, ok := s.Rhs[0].(*ast.CallExpr); ok {
								if funTV, ok := typesInfo.Types[callExpr.Fun]; ok && funTV.Type != nil {
									if sig, ok := funTV.Type.(*types.Signature); ok {
										results := sig.Results()
										if results != nil && results.Len() == len(s.Lhs) {
											varType = f.typeToAST(results.At(i).Type(), currentPkgPath, currentNames, pkgAliases, imports)
											varMap[ident.Name] = &VarInfo{Name: ident.Name, Type: varType, IsParam: false}
											continue
										}
									}
								}
							}
						}

						// Fallback: infer type from the corresponding RHS expression
						rhsIdx := i
						if rhsIdx >= len(s.Rhs) {
							rhsIdx = len(s.Rhs) - 1
						}
						if rhsIdx < 0 {
							continue
						}
						varType = f.inferTypeFromExpr(s.Rhs[rhsIdx], varMap, typesInfo, currentPkgPath, currentNames, pkgAliases, imports)
						varMap[ident.Name] = &VarInfo{Name: ident.Name, Type: varType, IsParam: false}
					}
				}
			}
		}
	case *ast.DeclStmt:
		if gen, ok := s.Decl.(*ast.GenDecl); ok && gen.Tok == token.VAR {
			for _, spec := range gen.Specs {
				if valueSpec, ok := spec.(*ast.ValueSpec); ok {
					for _, name := range valueSpec.Names {
						if _, exists := varMap[name.Name]; !exists {
							varType := valueSpec.Type
							if varType == nil && len(valueSpec.Values) > 0 && typesInfo != nil {
								if tv, ok := typesInfo.Types[valueSpec.Values[0]]; ok && tv.Type != nil {
									varType = f.typeToAST(tv.Type, currentPkgPath, currentNames, pkgAliases, imports)
								}
							}
							if varType == nil {
								varType = &ast.Ident{Name: "interface{}"}
							}
							varMap[name.Name] = &VarInfo{Name: name.Name, Type: varType, IsParam: false}
						}
					}
				}
			}
		}
	case *ast.BlockStmt:
		for _, inner := range s.List {
			f.collectVarsWithTypes(inner, varMap, typesInfo, posToType, currentPkgPath, currentNames, pkgAliases, imports)
		}
	case *ast.IfStmt:
		if s.Init != nil {
			f.collectVarsWithTypes(s.Init, varMap, typesInfo, posToType, currentPkgPath, currentNames, pkgAliases, imports)
		}
		f.collectVarsWithTypes(s.Body, varMap, typesInfo, posToType, currentPkgPath, currentNames, pkgAliases, imports)
		if s.Else != nil {
			if elseBlock, ok := s.Else.(*ast.BlockStmt); ok {
				f.collectVarsWithTypes(elseBlock, varMap, typesInfo, posToType, currentPkgPath, currentNames, pkgAliases, imports)
			}
		}
	case *ast.ForStmt:
		if s.Init != nil {
			if assign, ok := s.Init.(*ast.AssignStmt); ok && assign.Tok == token.DEFINE {
			} else {
				f.collectVarsWithTypes(s.Init, varMap, typesInfo, posToType, currentPkgPath, currentNames, pkgAliases, imports)
			}
		}
		f.collectVarsWithTypes(s.Body, varMap, typesInfo, posToType, currentPkgPath, currentNames, pkgAliases, imports)
	case *ast.RangeStmt:
		if s.Body != nil {
			for _, inner := range s.Body.List {
				f.collectVarsWithTypes(inner, varMap, typesInfo, posToType, currentPkgPath, currentNames, pkgAliases, imports)
			}
		}
	}
}

// resolveVarTypes performs a second pass to refine variable types using
// already collected information from varMap. This handles cases where
// a variable's type depends on another variable declared earlier.
func (f *CFF) resolveVarTypes(stmt ast.Stmt, varMap map[string]*VarInfo, typesInfo *types.Info, posToType map[token.Pos]types.Type, currentPkgPath string, currentNames map[token.Pos]string, pkgAliases map[string]string, imports map[string]string) {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		if s.Tok == token.DEFINE {
			for i, lhs := range s.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok {
					// Skip blank identifier
					if ident.Name == "_" {
						continue
					}
					if info, exists := varMap[ident.Name]; exists {
						// Most reliable: look up by token position.
						if typ, found := posToType[ident.Pos()]; found && typ != nil {
							if containsAnonymousStruct(typ) {
								rhsIdx := i
								if rhsIdx >= len(s.Rhs) {
									rhsIdx = len(s.Rhs) - 1
								}
								if rhsIdx >= 0 {
									info.Type = f.inferTypeFromExpr(s.Rhs[rhsIdx], varMap, typesInfo, currentPkgPath, currentNames, pkgAliases, imports)
									continue
								}
							}
							info.Type = f.typeToAST(typ, currentPkgPath, currentNames, pkgAliases, imports)
							continue
						}

						// Fallback: direct Defs lookup
						if typesInfo != nil {
							if obj := typesInfo.Defs[ident]; obj != nil && obj.Type() != nil {
								if containsAnonymousStruct(obj.Type()) {
									rhsIdx := i
									if rhsIdx >= len(s.Rhs) {
										rhsIdx = len(s.Rhs) - 1
									}
									if rhsIdx >= 0 {
										info.Type = f.inferTypeFromExpr(s.Rhs[rhsIdx], varMap, typesInfo, currentPkgPath, currentNames, pkgAliases, imports)
										continue
									}
								}
								info.Type = f.typeToAST(obj.Type(), currentPkgPath, currentNames, pkgAliases, imports)
								continue
							}
						}

						// Check if RHS is a type assertion: x, ok := expr.(T)
						if i < len(s.Rhs) {
							if ta, ok := s.Rhs[i].(*ast.TypeAssertExpr); ok && ta.Type != nil {
								if i == 0 {
									info.Type = ta.Type
								} else {
									info.Type = &ast.Ident{Name: "bool"}
								}
								continue
							}
						}

						// For multi-value function returns, use types.Info to get correct types per index
						if len(s.Lhs) > 1 && len(s.Rhs) == 1 && typesInfo != nil {
							if tv, ok := typesInfo.Types[s.Rhs[0]]; ok && tv.Type != nil {
								if tuple, ok := tv.Type.(*types.Tuple); ok && tuple.Len() == len(s.Lhs) {
									info.Type = f.typeToAST(tuple.At(i).Type(), currentPkgPath, currentNames, pkgAliases, imports)
									continue
								}
							}
							if callExpr, ok := s.Rhs[0].(*ast.CallExpr); ok {
								if funTV, ok := typesInfo.Types[callExpr.Fun]; ok && funTV.Type != nil {
									if sig, ok := funTV.Type.(*types.Signature); ok {
										results := sig.Results()
										if results != nil && results.Len() == len(s.Lhs) {
											info.Type = f.typeToAST(results.At(i).Type(), currentPkgPath, currentNames, pkgAliases, imports)
											continue
										}
									}
								}
							}
						}

						// For multi-value: all reliable sources exhausted, never call
						// inferTypeFromExpr (it can't distinguish per-index return types
						// and would overwrite a correctly resolved type with interface{}).
						if len(s.Lhs) > 1 && len(s.Rhs) == 1 {
							continue
						}
						rhsIdx := i
						if rhsIdx >= len(s.Rhs) {
							rhsIdx = len(s.Rhs) - 1
						}
						if rhsIdx < 0 {
							continue
						}
						newType := f.inferTypeFromExpr(s.Rhs[rhsIdx], varMap, typesInfo, currentPkgPath, currentNames, pkgAliases, imports)
						// Only update if we got something concrete, never degrade to interface{}.
						if !isInterfaceAny(newType) || isInterfaceAny(info.Type) {
							info.Type = newType
						}
					}
				}
			}
		}
	case *ast.BlockStmt:
		for _, inner := range s.List {
			f.resolveVarTypes(inner, varMap, typesInfo, posToType, currentPkgPath, currentNames, pkgAliases, imports)
		}
	case *ast.IfStmt:
		if s.Init != nil {
			f.resolveVarTypes(s.Init, varMap, typesInfo, posToType, currentPkgPath, currentNames, pkgAliases, imports)
		}
		f.resolveVarTypes(s.Body, varMap, typesInfo, posToType, currentPkgPath, currentNames, pkgAliases, imports)
		if s.Else != nil {
			if elseBlock, ok := s.Else.(*ast.BlockStmt); ok {
				f.resolveVarTypes(elseBlock, varMap, typesInfo, posToType, currentPkgPath, currentNames, pkgAliases, imports)
			}
		}
	case *ast.ForStmt:
		if s.Init != nil {
			f.resolveVarTypes(s.Init, varMap, typesInfo, posToType, currentPkgPath, currentNames, pkgAliases, imports)
		}
		f.resolveVarTypes(s.Body, varMap, typesInfo, posToType, currentPkgPath, currentNames, pkgAliases, imports)
	}
}

// inferTypeFromExpr infers the Go type of expression by pattern matching
// on AST node types. Handles literals, identifiers (with varMap lookup),
// function calls (make, chan), channel operations, and composite types.
// Returns interface{} as fallback for unknown types.
func (f *CFF) inferTypeFromExpr(expr ast.Expr, varMap map[string]*VarInfo, typesInfo *types.Info, currentPkgPath string, currentNames map[token.Pos]string, pkgAliases map[string]string, imports map[string]string) ast.Expr {
	if typesInfo != nil {
		if tv, ok := typesInfo.Types[expr]; ok && tv.Type != nil {
			return f.typeToAST(tv.Type, currentPkgPath, currentNames, pkgAliases, imports)
		}
	}
	switch e := expr.(type) {
	case *ast.BasicLit:
		switch e.Kind {
		case token.STRING:
			return &ast.Ident{Name: "string"}
		case token.INT:
			return &ast.Ident{Name: "int"}
		case token.FLOAT:
			return &ast.Ident{Name: "float64"}
		case token.CHAR:
			return &ast.Ident{Name: "byte"}
		}
	case *ast.Ident:
		if info, exists := varMap[e.Name]; exists {
			return info.Type
		}
		return &ast.Ident{Name: "string"}
	case *ast.CallExpr:
		if typesInfo != nil {
			if tv, ok := typesInfo.Types[e]; ok && tv.Type != nil {
				return f.typeToAST(tv.Type, currentPkgPath, currentNames, pkgAliases, imports)
			}
		}
		return &ast.Ident{Name: "interface{}"}
	case *ast.UnaryExpr:
		if e.Op == token.AND {
			if comp, ok := e.X.(*ast.CompositeLit); ok {
				return &ast.StarExpr{X: comp.Type}
			}
		}
		if e.Op == token.ARROW {
			chanType := f.inferTypeFromExpr(e.X, varMap, typesInfo, currentPkgPath, currentNames, pkgAliases, imports)
			if ct, ok := chanType.(*ast.ChanType); ok {
				return ct.Value
			}
		}
	case *ast.CompositeLit:
		return e.Type
	case *ast.ArrayType:
		if e.Len == nil {
			return &ast.ArrayType{Len: nil, Elt: e.Elt}
		}
		return e
	case *ast.ChanType:
		return e
	case *ast.StarExpr:
		return e
	}
	return &ast.Ident{Name: "interface{}"}
}

// inferTypeFromRangeKey infers the type of range loop's key variable.
// For slices/arrays/maps, the key is typically int (index).
func (f *CFF) inferTypeFromRangeKey(expr ast.Expr) ast.Expr {
	switch expr.(type) {
	case *ast.Ident:
		return &ast.Ident{Name: "int"}
	case *ast.ArrayType:
		return &ast.Ident{Name: "int"}
	}
	return &ast.Ident{Name: "interface{}"}
}

// inferTypeFromRangeValue infers the type of range loop's value variable.
// Looks up the container type in varMap and extracts the element type.
func (f *CFF) inferTypeFromRangeValue(expr ast.Expr, varMap map[string]*VarInfo) ast.Expr {
	switch e := expr.(type) {
	case *ast.Ident:
		if info, exists := varMap[e.Name]; exists {
			return f.getElementType(info.Type)
		}
		return &ast.Ident{Name: "string"}
	case *ast.ArrayType:
		return e.Elt
	}
	return &ast.Ident{Name: "interface{}"}
}

// getElementType extracts the element type from container types:
// arrays/slices return Elt, channels return Value.
func (f *CFF) getElementType(expr ast.Expr) ast.Expr {
	switch e := expr.(type) {
	case *ast.ArrayType:
		return e.Elt
	case *ast.ChanType:
		return e.Value
	}
	return &ast.Ident{Name: "interface{}"}
}

// convertDefineToAssign recursively converts short variable declarations
// (:=) to assignments (=) in preparation for hoisting. This ensures
// variables declared in one case remain accessible in subsequent cases.
// Handles nested blocks, if/for/range statements recursively.
func (f *CFF) convertDefineToAssign(stmt ast.Stmt, hoistedVars map[string]bool) ast.Stmt {
	isAnonymousStructAssertion := func(expr ast.Expr) bool {
		if ta, ok := expr.(*ast.TypeAssertExpr); ok && ta.Type != nil {
			_, isStruct := ta.Type.(*ast.StructType)
			return isStruct
		}
		return false
	}

	switch s := stmt.(type) {
	case *ast.DeclStmt:
		if gen, ok := s.Decl.(*ast.GenDecl); ok && gen.Tok == token.VAR {
			var assigns []ast.Stmt
			for _, spec := range gen.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok {
					// If there are no values, the declaration is pure type annotation
					// (e.g. "var x int"). If all names are hoisted, we already emitted
					// "var x int" at the top — no assignment needed, skip entirely.
					if len(vs.Values) == 0 {
						allHoisted := true
						for _, name := range vs.Names {
							if !hoistedVars[name.Name] {
								allHoisted = false
								break
							}
						}
						if allHoisted {
							continue
						}
					}
					// If there ARE values (e.g. "var bestSpeed = speed"), always emit
					// the assignment — even if the variable is hoisted we still need to
					// run the initializer expression.
					if len(vs.Values) > 0 {
						assigns = append(assigns, &ast.AssignStmt{
							Lhs: toExprs(vs.Names), Tok: token.ASSIGN, TokPos: gen.TokPos, Rhs: vs.Values,
						})
					}
				}
			}
			if len(assigns) == 1 {
				return assigns[0]
			}
			if len(assigns) > 1 {
				return &ast.BlockStmt{List: assigns}
			}
			return &ast.EmptyStmt{}
		}
	case *ast.AssignStmt:
		if s.Tok == token.DEFINE {
			// Preserve := for type assertions to anonymous structs
			for _, rhs := range s.Rhs {
				if isAnonymousStructAssertion(rhs) {
					return stmt
				}
			}
			return &ast.AssignStmt{Lhs: s.Lhs, Tok: token.ASSIGN, TokPos: s.TokPos, Rhs: s.Rhs}
		}
	case *ast.BlockStmt:
		res := make([]ast.Stmt, len(s.List))
		for i, inner := range s.List {
			res[i] = f.convertDefineToAssign(inner, hoistedVars)
		}
		return &ast.BlockStmt{List: res}
	case *ast.IfStmt:
		if s.Init != nil {
			s.Init = f.convertDefineToAssign(s.Init, hoistedVars)
		}
		s.Body = f.convertDefineToAssign(s.Body, hoistedVars).(*ast.BlockStmt)
		if s.Else != nil {
			if eb, ok := s.Else.(*ast.BlockStmt); ok {
				s.Else = f.convertDefineToAssign(eb, hoistedVars)
			}
		}
		return s
	case *ast.ForStmt:
		s.Body = f.convertDefineToAssign(s.Body, hoistedVars).(*ast.BlockStmt)
		return s
	case *ast.RangeStmt:
		// Preserve := in range loops
		if s.Body != nil {
			s.Body = f.convertDefineToAssign(s.Body, hoistedVars).(*ast.BlockStmt)
		}
		return s
	}
	return stmt
}

// toExprs converts a slice of *ast.Ident pointers to a slice of ast.Expr
// values, as required by ast.AssignStmt.Lhs and similar fields.
func toExprs(idents []*ast.Ident) []ast.Expr {
	exprs := make([]ast.Expr, len(idents))
	for i, id := range idents {
		exprs[i] = id
	}
	return exprs
}

// buildZeroReturnFromSig builds a zero-value return using the type checker's
// types.Signature. Unlike buildZeroReturn (AST-only), this correctly handles
// interface types (error, cipher.AEAD, io.Reader, …) by returning nil instead
// of the invalid composite literal T{}.
func (f *CFF) buildZeroReturnFromSig(sig *types.Signature, currentPkgPath string, currentNames map[token.Pos]string, pkgAliases map[string]string, imports map[string]string) *ast.ReturnStmt {
	results := sig.Results()
	if results == nil || results.Len() == 0 {
		return &ast.ReturnStmt{}
	}
	var retVals []ast.Expr
	for i := 0; i < results.Len(); i++ {
		retVals = append(retVals, f.zeroValueForTypesType(results.At(i).Type(), currentPkgPath, currentNames, pkgAliases, imports))
	}
	return &ast.ReturnStmt{Results: retVals}
}

// zeroValueForTypesType returns the zero-value AST expression for a types.Type.
// It correctly returns nil for any interface type (including named interfaces
// such as error or cipher.AEAD), and delegates to typeToAST for struct/named types.
func (f *CFF) zeroValueForTypesType(t types.Type, currentPkgPath string, currentNames map[token.Pos]string, pkgAliases map[string]string, imports map[string]string) ast.Expr {
	switch typ := t.(type) {
	case *types.Basic:
		switch typ.Kind() {
		case types.Bool:
			return &ast.Ident{Name: "false"}
		case types.String:
			return &ast.BasicLit{Kind: token.STRING, Value: `""`}
		case types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
			types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64,
			types.Uintptr, types.Float32, types.Float64,
			types.Complex64, types.Complex128:
			return &ast.BasicLit{Kind: token.INT, Value: "0"}
		}
		return &ast.BasicLit{Kind: token.INT, Value: "0"}
	case *types.Pointer, *types.Slice, *types.Map, *types.Chan, *types.Signature:
		return &ast.Ident{Name: "nil"}
	case *types.Interface:
		// All interfaces (error, io.Reader, cipher.AEAD, …) → nil.
		return &ast.Ident{Name: "nil"}
	case *types.Named:
		// Check underlying first — a named interface (e.g. type MyErr interface{…})
		// must also return nil.
		if _, isIface := typ.Underlying().(*types.Interface); isIface {
			return &ast.Ident{Name: "nil"}
		}
		// Named struct or alias → zero composite literal using the AST type expression.
		return &ast.CompositeLit{Type: f.typeToAST(t, currentPkgPath, currentNames, pkgAliases, imports)}
	case *types.Struct:
		return &ast.CompositeLit{Type: f.typeToAST(t, currentPkgPath, currentNames, pkgAliases, imports)}
	case *types.Alias:
		return f.zeroValueForTypesType(types.Unalias(typ), currentPkgPath, currentNames, pkgAliases, imports)
	}
	return &ast.Ident{Name: "nil"}
}

// buildZeroReturn constructs a return statement with zero values for each
// result type in the function signature. For void functions returns `return`.
// This is needed as the terminal transition in the CFF state machine because
// a plain `return` without values is invalid in non-void functions.
func (f *CFF) buildZeroReturn(results *ast.FieldList) *ast.ReturnStmt {
	if results == nil || len(results.List) == 0 {
		return &ast.ReturnStmt{}
	}
	var retVals []ast.Expr
	for _, field := range results.List {
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		for j := 0; j < count; j++ {
			retVals = append(retVals, zeroValueForType(field.Type))
		}
	}
	return &ast.ReturnStmt{Results: retVals}
}

// zeroValueForType returns an AST expression for the zero value of the given
// type expression. Used to build valid return statements in non-void functions.
func zeroValueForType(t ast.Expr) ast.Expr {
	switch typ := t.(type) {
	case *ast.Ident:
		switch typ.Name {
		case "bool":
			return &ast.Ident{Name: "false"}
		case "string":
			return &ast.BasicLit{Kind: token.STRING, Value: `""`}
		case "int", "int8", "int16", "int32", "int64",
			"uint", "uint8", "uint16", "uint32", "uint64",
			"uintptr", "byte", "rune", "float32", "float64",
			"complex64", "complex128":
			return &ast.BasicLit{Kind: token.INT, Value: "0"}
		default:
			// Named type — return zero composite literal: TypeName{}
			return &ast.CompositeLit{Type: t}
		}
	case *ast.StarExpr:
		return &ast.Ident{Name: "nil"}
	case *ast.ArrayType, *ast.MapType, *ast.ChanType, *ast.FuncType:
		return &ast.Ident{Name: "nil"}
	case *ast.InterfaceType:
		return &ast.Ident{Name: "nil"}
	case *ast.SelectorExpr:
		// Qualified type from another package (e.g. time.Duration, http.Handler)
		return &ast.CompositeLit{Type: t}
	case *ast.StructType:
		return &ast.CompositeLit{Type: t}
	}
	return &ast.Ident{Name: "nil"}
}

// typeToAST converts a types.Type to its corresponding ast.Expr representation.
// Named types from the current package are emitted as bare identifiers; types
// from other packages are emitted as selector expressions (pkg.Type) and their
// import paths are recorded in imports for later injection. Anonymous structs
// and interfaces fall back to struct{} and interface{} literals respectively.
// Post-rename type names are resolved through currentNames (token.Pos → name)
// so that the generated AST uses obfuscated names consistently.
func (f *CFF) typeToAST(t types.Type, currentPkgPath string, currentNames map[token.Pos]string, pkgAliases map[string]string, imports map[string]string) ast.Expr {
	switch typ := t.(type) {
	case *types.Basic:
		switch typ.Kind() {
		case types.String:
			return &ast.Ident{Name: "string"}
		case types.Int:
			return &ast.Ident{Name: "int"}
		case types.Int8:
			return &ast.Ident{Name: "int8"}
		case types.Int16:
			return &ast.Ident{Name: "int16"}
		case types.Int64:
			return &ast.Ident{Name: "int64"}
		case types.Int32:
			return &ast.Ident{Name: "rune"}
		case types.Uint:
			return &ast.Ident{Name: "uint"}
		case types.Uint16:
			return &ast.Ident{Name: "uint16"}
		case types.Uint32:
			return &ast.Ident{Name: "uint32"}
		case types.Uint64:
			return &ast.Ident{Name: "uint64"}
		case types.Uintptr:
			return &ast.Ident{Name: "uintptr"}
		case types.Uint8:
			return &ast.Ident{Name: "byte"}
		case types.Float32, types.Float64:
			return &ast.Ident{Name: "float64"}
		case types.Bool:
			return &ast.Ident{Name: "bool"}
		case types.UnsafePointer:
			return &ast.Ident{Name: "unsafe.Pointer"}
		default:
			return &ast.Ident{Name: "interface{}"}
		}
	case *types.Pointer:
		return &ast.StarExpr{X: f.typeToAST(typ.Elem(), currentPkgPath, currentNames, pkgAliases, imports)}
	case *types.Slice:
		elem := f.typeToAST(typ.Elem(), currentPkgPath, currentNames, pkgAliases, imports)
		if basic, ok := typ.Elem().(*types.Basic); ok && basic.Kind() == types.Uint8 {
			return &ast.ArrayType{Len: nil, Elt: &ast.Ident{Name: "byte"}}
		}
		return &ast.ArrayType{Len: nil, Elt: elem}
	case *types.Array:
		return &ast.ArrayType{
			Len: &ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", typ.Len())},
			Elt: f.typeToAST(typ.Elem(), currentPkgPath, currentNames, pkgAliases, imports),
		}
	case *types.Chan:
		return &ast.ChanType{Value: f.typeToAST(typ.Elem(), currentPkgPath, currentNames, pkgAliases, imports)}
	case *types.Map:
		return &ast.MapType{
			Key:   f.typeToAST(typ.Key(), currentPkgPath, currentNames, pkgAliases, imports),
			Value: f.typeToAST(typ.Elem(), currentPkgPath, currentNames, pkgAliases, imports),
		}
	case *types.Alias:
		// Go 1.22+ represents explicit type aliases as *types.Alias.
		// types.Unalias returns the *types.Named (or other) that the alias points to,
		// preserving the named type identity (e.g. os.FileInfo, not the underlying interface).
		rhs := types.Unalias(typ)
		return f.typeToAST(rhs, currentPkgPath, currentNames, pkgAliases, imports)
	case *types.Named:
		obj := typ.Obj()
		pkg := obj.Pkg()
		// Use current (post-rename) name from AST; fall back to original from type checker.
		// Guard against NoPos (== 0): built-in types (error, comparable, etc.) and
		// synthetic CFF idents both have Pos() == 0 — looking up currentNames[0] would
		// return the last synthetic state-variable name, corrupting the type.
		typeName := obj.Name()
		if obj.Pos() != token.NoPos {
			if currentName, ok := currentNames[obj.Pos()]; ok {
				typeName = currentName
			}
		}
		if pkg != nil && pkg.Path() == currentPkgPath {
			return &ast.Ident{Name: typeName}
		}
		if pkg != nil {
			// Use the local alias if the package was imported with one (e.g. stdhttp "net/http").
			localName := pkg.Name()
			if alias, ok := pkgAliases[pkg.Path()]; ok {
				localName = alias
			}
			if imports != nil {
				imports[localName] = pkg.Path()
			}
			return &ast.SelectorExpr{
				X:   &ast.Ident{Name: localName},
				Sel: &ast.Ident{Name: typeName},
			}
		}
		return &ast.Ident{Name: typeName}
	case *types.Signature:
		return f.signatureToAST(typ, currentPkgPath, currentNames, pkgAliases, imports)
	case *types.Struct:
		// Build an anonymous struct AST node with current (post-rename) field names.
		fields := &ast.FieldList{}
		for i := 0; i < typ.NumFields(); i++ {
			field := typ.Field(i)
			fieldName := field.Name()
			if field.Pos() != token.NoPos {
				if currentName, ok := currentNames[field.Pos()]; ok {
					fieldName = currentName
				}
			}
			fields.List = append(fields.List, &ast.Field{
				Names: []*ast.Ident{{Name: fieldName}},
				Type:  f.typeToAST(field.Type(), currentPkgPath, currentNames, pkgAliases, imports),
			})
		}
		return &ast.StructType{Fields: fields}
	case *types.Interface:
		return &ast.Ident{Name: "interface{}"}
	case *types.Tuple:
		if typ.Len() > 0 {
			return f.typeToAST(typ.At(0).Type(), currentPkgPath, currentNames, pkgAliases, imports)
		}
		return &ast.Ident{Name: "interface{}"}
	}
	return &ast.Ident{Name: "interface{}"}
}

// signatureToAST converts a *types.Signature to an *ast.FuncType suitable for
// use in a var declaration. Parameter and result types are resolved recursively
// via typeToAST.
func (f *CFF) signatureToAST(sig *types.Signature, currentPkgPath string, currentNames map[token.Pos]string, pkgAliases map[string]string, imports map[string]string) *ast.FuncType {
	params := &ast.FieldList{}
	if sig.Params() != nil {
		for i := 0; i < sig.Params().Len(); i++ {
			param := sig.Params().At(i)
			params.List = append(params.List, &ast.Field{
				Type: f.typeToAST(param.Type(), currentPkgPath, currentNames, pkgAliases, imports),
			})
		}
	}

	var results *ast.FieldList
	if sig.Results() != nil && sig.Results().Len() > 0 {
		results = &ast.FieldList{}
		for i := 0; i < sig.Results().Len(); i++ {
			result := sig.Results().At(i)
			results.List = append(results.List, &ast.Field{
				Type: f.typeToAST(result.Type(), currentPkgPath, currentNames, pkgAliases, imports),
			})
		}
	}

	return &ast.FuncType{
		Params:  params,
		Results: results,
	}
}

// hasLocalConst checks if a list of statements contains any local const
// declarations. Such functions cannot be flattened because const values
// are only visible within the block they're declared in — splitting them
// into separate switch cases makes them undefined in subsequent cases.
func (f *CFF) hasLocalConst(stmts []ast.Stmt) bool {
	for _, stmt := range stmts {
		found := false
		ast.Inspect(stmt, func(n ast.Node) bool {
			if found {
				return false
			}
			if decl, ok := n.(*ast.GenDecl); ok && decl.Tok == token.CONST {
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

// hasLocalTypeDecl checks if a list of statements contains any local type
// declarations (e.g. "type Alias MyStruct"). Such types are only visible
// within their declaring block and would become undefined after CFF splits
// the function body into separate switch cases.
func (f *CFF) hasLocalTypeDecl(stmts []ast.Stmt) bool {
	for _, stmt := range stmts {
		found := false
		ast.Inspect(stmt, func(n ast.Node) bool {
			if found {
				return false
			}
			if decl, ok := n.(*ast.GenDecl); ok && decl.Tok == token.TYPE {
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

// hasGoto checks if a list of statements contains any goto statements or
// labeled statements. After CFF splits the body into switch cases, goto
// targets (labels) end up inside case blocks, and Go does not allow goto
// to jump into a block — which causes a compile error.
func (f *CFF) hasGoto(stmts []ast.Stmt) bool {
	for _, stmt := range stmts {
		found := false
		ast.Inspect(stmt, func(n ast.Node) bool {
			if found {
				return false
			}
			switch n.(type) {
			case *ast.BranchStmt:
				if b, ok := n.(*ast.BranchStmt); ok && b.Tok == token.GOTO {
					found = true
					return false
				}
			case *ast.LabeledStmt:
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

// hasAnonymousStructTypeAssertion reports whether any statement in stmts
// contains a type assertion to an anonymous struct (e.g. v.(struct{ x int })).
// Such assertions prevent CFF from converting the surrounding := to = because
// the asserted type would need to be re-stated in the hoisted var declaration.
func (f *CFF) hasAnonymousStructTypeAssertion(stmts []ast.Stmt) bool {
	for _, stmt := range stmts {
		found := false
		ast.Inspect(stmt, func(n ast.Node) bool {
			if found {
				return false
			}
			if ta, ok := n.(*ast.TypeAssertExpr); ok && ta.Type != nil {
				if _, isStruct := ta.Type.(*ast.StructType); isStruct {
					found = true
					return false
				}
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

// addImportsToFile adds any import paths in imports that are not already
// present in file. Entries in imports map a local package name to its import
// path (e.g. "http" → "net/http"). A parenthesised import block is created
// if the file does not already contain one.
func addImportsToFile(file *ast.File, imports map[string]string) {
	if len(imports) == 0 {
		return
	}

	// Collect already-present import paths to avoid duplicates.
	existing := make(map[string]bool)
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		existing[path] = true
	}

	// Find or create the import GenDecl.
	var importDecl *ast.GenDecl
	for _, decl := range file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.IMPORT {
			importDecl = gen
			break
		}
	}
	if importDecl == nil {
		importDecl = &ast.GenDecl{Tok: token.IMPORT, Lparen: 1}
		file.Decls = append([]ast.Decl{importDecl}, file.Decls...)
	}
	if importDecl.Lparen == 0 {
		// Enable parenthesised form so multiple specs can be appended.
		importDecl.Lparen = 1
	}

	for _, importPath := range imports {
		if existing[importPath] {
			continue
		}
		importDecl.Specs = append(importDecl.Specs, &ast.ImportSpec{
			Path: &ast.BasicLit{Kind: token.STRING, Value: `"` + importPath + `"`},
		})
		file.Imports = append(file.Imports, &ast.ImportSpec{
			Path: &ast.BasicLit{Kind: token.STRING, Value: `"` + importPath + `"`},
		})
		existing[importPath] = true
	}
}

// isInterfaceAny reports whether expr represents the untyped interface{}
// (the fallback type used when real type resolution fails).
func isInterfaceAny(expr ast.Expr) bool {
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name == "interface{}"
	}
	return false
}

// containsAnonymousStruct reports whether t is or contains an anonymous struct type.
// For such types, typeToAST cannot reliably reconstruct renamed field names because
// multiple anonymous structs with the same shape each get distinct renamed names.
// The caller should use inferTypeFromExpr (AST-based) instead.
func containsAnonymousStruct(t types.Type) bool {
	switch typ := t.(type) {
	case *types.Struct:
		return true
	case *types.Slice:
		return containsAnonymousStruct(typ.Elem())
	case *types.Array:
		return containsAnonymousStruct(typ.Elem())
	case *types.Pointer:
		return containsAnonymousStruct(typ.Elem())
	case *types.Named:
		// Named types are fine — they have a stable declaration position.
		return false
	}
	return false
}
