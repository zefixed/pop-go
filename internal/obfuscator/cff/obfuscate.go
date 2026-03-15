package cff

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/packages"
	"log/slog"
	"pop-go/pkg/util"
	"time"
)

// Obfuscate executes the control flow flattening obfuscation process.
// It iterates through all packages and files, applying the transformation
// to eligible functions while logging progress and errors.
func (f *CFF) Obfuscate() {
	f.log.Info(f.cfg.CurLocale["obf.info.start.cff"])
	t := time.Now()
	for _, pkg := range f.pkgs {
		for _, file := range pkg.Syntax {
			absPath := pkg.Fset.File(file.Pos()).Name()
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
		if err := f.flattenFunction(fn, pkg); err != nil {
			return err
		}
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
			// Channel creation: make(chan T)
			if ident, ok := n.(*ast.CallExpr).Fun.(*ast.Ident); ok && ident.Name == "make" {
				found = true
				return false
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
func (f *CFF) flattenFunction(fn *ast.FuncDecl, pkg *packages.Package) error {
	hoistedVars := make(map[string]bool)
	stateVarName := util.GenerateUniqueName(f.cfg.Obfuscator.Seed)
	originalStmts := fn.Body.List
	typesInfo := pkg.TypesInfo
	currentPkgName := pkg.Name // Get current package name

	varInfoMap := make(map[string]*VarInfo)

	if fn.Type.Params != nil {
		for _, param := range fn.Type.Params.List {
			for _, name := range param.Names {
				varInfoMap[name.Name] = &VarInfo{Name: name.Name, Type: param.Type, IsParam: true}
			}
		}
	}

	for _, stmt := range originalStmts {
		f.collectVarsWithTypes(stmt, varInfoMap, typesInfo, currentPkgName)
	}
	for _, stmt := range originalStmts {
		f.resolveVarTypes(stmt, varInfoMap, typesInfo, currentPkgName)
	}

	var hoistedDecls []ast.Stmt
	for _, v := range varInfoMap {
		if v.IsParam {
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
	for i, stmt := range originalStmts {
		processed := f.convertDefineToAssign(stmt, hoistedVars)
		var trans ast.Stmt
		if i == n-1 {
			trans = &ast.BranchStmt{Tok: token.BREAK}
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
	cases = append(cases, &ast.CaseClause{Body: []ast.Stmt{&ast.BranchStmt{Tok: token.BREAK}}})

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
func (f *CFF) collectVarsWithTypes(stmt ast.Stmt, varMap map[string]*VarInfo, typesInfo *types.Info, currentPkgName string) {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		if s.Tok == token.DEFINE {
			for i, lhs := range s.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok {
					if _, exists := varMap[ident.Name]; !exists {
						var varType ast.Expr

						// Check if RHS is a type assertion: x, ok := expr.(T)
						if i < len(s.Rhs) {
							if ta, ok := s.Rhs[i].(*ast.TypeAssertExpr); ok && ta.Type != nil {
								if i == 0 {
									// First variable gets the asserted type
									varType = ta.Type // Use AST type directly
								} else {
									// Second variable (ok) is always bool
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
									// Each LHS variable gets its corresponding tuple element type
									varType = f.typeToAST(tuple.At(i).Type(), currentPkgName)
									varMap[ident.Name] = &VarInfo{Name: ident.Name, Type: varType, IsParam: false}
									continue
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
						varType = f.inferTypeFromExpr(s.Rhs[rhsIdx], varMap, typesInfo, currentPkgName)
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
									varType = f.typeToAST(tv.Type, currentPkgName)
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
			f.collectVarsWithTypes(inner, varMap, typesInfo, currentPkgName)
		}
	case *ast.IfStmt:
		if s.Init != nil {
			f.collectVarsWithTypes(s.Init, varMap, typesInfo, currentPkgName)
		}
		f.collectVarsWithTypes(s.Body, varMap, typesInfo, currentPkgName)
		if s.Else != nil {
			if elseBlock, ok := s.Else.(*ast.BlockStmt); ok {
				f.collectVarsWithTypes(elseBlock, varMap, typesInfo, currentPkgName)
			}
		}
	case *ast.ForStmt:
		if s.Init != nil {
			if assign, ok := s.Init.(*ast.AssignStmt); ok && assign.Tok == token.DEFINE {
			} else {
				f.collectVarsWithTypes(s.Init, varMap, typesInfo, currentPkgName)
			}
		}
		f.collectVarsWithTypes(s.Body, varMap, typesInfo, currentPkgName)
	case *ast.RangeStmt:
		if s.Body != nil {
			for _, inner := range s.Body.List {
				f.collectVarsWithTypes(inner, varMap, typesInfo, currentPkgName)
			}
		}
	}
}

// resolveVarTypes performs a second pass to refine variable types using
// already collected information from varMap. This handles cases where
// a variable's type depends on another variable declared earlier.
func (f *CFF) resolveVarTypes(stmt ast.Stmt, varMap map[string]*VarInfo, typesInfo *types.Info, currentPkgName string) {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		if s.Tok == token.DEFINE {
			for i, lhs := range s.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok {
					if info, exists := varMap[ident.Name]; exists {
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
									info.Type = f.typeToAST(tuple.At(i).Type(), currentPkgName)
									continue
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
						newType := f.inferTypeFromExpr(s.Rhs[rhsIdx], varMap, typesInfo, currentPkgName)
						info.Type = newType
					}
				}
			}
		}
	case *ast.BlockStmt:
		for _, inner := range s.List {
			f.resolveVarTypes(inner, varMap, typesInfo, currentPkgName)
		}
	case *ast.IfStmt:
		if s.Init != nil {
			f.resolveVarTypes(s.Init, varMap, typesInfo, currentPkgName)
		}
		f.resolveVarTypes(s.Body, varMap, typesInfo, currentPkgName)
		if s.Else != nil {
			if elseBlock, ok := s.Else.(*ast.BlockStmt); ok {
				f.resolveVarTypes(elseBlock, varMap, typesInfo, currentPkgName)
			}
		}
	case *ast.ForStmt:
		if s.Init != nil {
			f.resolveVarTypes(s.Init, varMap, typesInfo, currentPkgName)
		}
		f.resolveVarTypes(s.Body, varMap, typesInfo, currentPkgName)
	}
}

// inferTypeFromExpr infers the Go type of expression by pattern matching
// on AST node types. Handles literals, identifiers (with varMap lookup),
// function calls (make, chan), channel operations, and composite types.
// Returns interface{} as fallback for unknown types.
func (f *CFF) inferTypeFromExpr(expr ast.Expr, varMap map[string]*VarInfo, typesInfo *types.Info, currentPkgName string) ast.Expr {
	if typesInfo != nil {
		if tv, ok := typesInfo.Types[expr]; ok && tv.Type != nil {
			return f.typeToAST(tv.Type, currentPkgName)
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
				return f.typeToAST(tv.Type, currentPkgName)
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
			chanType := f.inferTypeFromExpr(e.X, varMap, typesInfo, currentPkgName)
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
func (f *CFF) inferTypeFromRangeKey(expr ast.Expr, varMap map[string]*VarInfo) ast.Expr {
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
					skip := true
					for _, name := range vs.Names {
						if !hoistedVars[name.Name] {
							skip = false
							break
						}
					}
					if skip {
						continue
					}
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
		if s.Init != nil {
			s.Init = f.convertDefineToAssign(s.Init, hoistedVars)
		}
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

func toExprs(idents []*ast.Ident) []ast.Expr {
	exprs := make([]ast.Expr, len(idents))
	for i, id := range idents {
		exprs[i] = id
	}
	return exprs
}

func (f *CFF) typeToAST(t types.Type, currentPkgName string) ast.Expr {
	switch typ := t.(type) {
	case *types.Basic:
		switch typ.Kind() {
		case types.String:
			return &ast.Ident{Name: "string"}
		case types.Int, types.Int8, types.Int16, types.Int64:
			return &ast.Ident{Name: "int"}
		case types.Int32:
			return &ast.Ident{Name: "rune"}
		case types.Uint, types.Uint16, types.Uint32, types.Uint64, types.Uintptr:
			return &ast.Ident{Name: "uint"}
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
		return &ast.StarExpr{X: f.typeToAST(typ.Elem(), currentPkgName)}
	case *types.Slice:
		elem := f.typeToAST(typ.Elem(), currentPkgName)
		if basic, ok := typ.Elem().(*types.Basic); ok && basic.Kind() == types.Uint8 {
			return &ast.ArrayType{Len: nil, Elt: &ast.Ident{Name: "byte"}}
		}
		return &ast.ArrayType{Len: nil, Elt: elem}
	case *types.Array:
		return &ast.ArrayType{
			Len: &ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", typ.Len())},
			Elt: f.typeToAST(typ.Elem(), currentPkgName),
		}
	case *types.Chan:
		return &ast.ChanType{Value: f.typeToAST(typ.Elem(), currentPkgName)}
	case *types.Map:
		return &ast.MapType{
			Key:   f.typeToAST(typ.Key(), currentPkgName),
			Value: f.typeToAST(typ.Elem(), currentPkgName),
		}
	case *types.Named:
		obj := typ.Obj()
		pkg := obj.Pkg()
		typeName := obj.Name()
		// Skip package qualifier if type is from the same package
		if pkg != nil && pkg.Name() == currentPkgName {
			return &ast.Ident{Name: typeName}
		}
		if pkg != nil {
			return &ast.SelectorExpr{
				X:   &ast.Ident{Name: pkg.Name()},
				Sel: &ast.Ident{Name: typeName},
			}
		}
		return &ast.Ident{Name: typeName}
	case *types.Struct, *types.Interface, *types.Signature:
		return &ast.Ident{Name: "interface{}"}
	case *types.Tuple:
		if typ.Len() > 0 {
			return f.typeToAST(typ.At(0).Type(), currentPkgName)
		}
		return &ast.Ident{Name: "interface{}"}
	}
	return &ast.Ident{Name: "interface{}"}
}

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
