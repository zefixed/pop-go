package cff

import (
	"fmt"
	"go/ast"
	"go/token"
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

			if err := f.flattenFile(file, pkg.Fset); err != nil {
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
func (f *CFF) flattenFile(file *ast.File, fset *token.FileSet) error {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || len(fn.Body.List) < 2 {
			continue
		}

		// Skip functions with concurrency primitives to prevent deadlocks
		if f.hasConcurrency(fn.Body.List) {
			f.log.Debug("skipping function with concurrency", slog.String("function", fn.Name.Name))
			continue
		}

		if err := f.flattenFunction(fn); err != nil {
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
func (f *CFF) flattenFunction(fn *ast.FuncDecl) error {
	stateVarName := util.GenerateUniqueName(f.cfg.Obfuscator.Seed)
	originalStmts := fn.Body.List

	// Phase 1: Collect variables with type information for hoisting
	varInfoMap := make(map[string]*VarInfo)

	// Register function parameters with their types (marked as IsParam=true)
	if fn.Type.Params != nil {
		for _, param := range fn.Type.Params.List {
			for _, name := range param.Names {
				varInfoMap[name.Name] = &VarInfo{
					Name:    name.Name,
					Type:    param.Type,
					IsParam: true,
				}
			}
		}
	}

	// First pass: collect variables from short declarations (:=) and explicit var statements
	for _, stmt := range originalStmts {
		f.collectVarsWithTypes(stmt, varInfoMap)
	}

	// Second pass: refine variable types using already collected information
	for _, stmt := range originalStmts {
		f.resolveVarTypes(stmt, varInfoMap)
	}

	// Phase 2: Generate hoisted variable declarations (excluding parameters)
	var hoistedDecls []ast.Stmt
	for _, varInfo := range varInfoMap {
		if varInfo.IsParam {
			continue
		}

		varDecl := &ast.DeclStmt{
			Decl: &ast.GenDecl{
				Tok: token.VAR,
				Specs: []ast.Spec{
					&ast.ValueSpec{
						Names: []*ast.Ident{{Name: varInfo.Name}},
						Type:  varInfo.Type,
					},
				},
			},
		}
		hoistedDecls = append(hoistedDecls, varDecl)
	}

	// Phase 3: Build the state machine dispatcher (switch inside for-loop)
	var caseClauses []ast.Stmt
	n := len(originalStmts)

	for i, stmt := range originalStmts {
		// Convert short declarations (:=) to assignments (=) for hoisted variables
		processedStmt := f.convertDefineToAssign(stmt)

		// Determine transition: either next state or break for final statement
		var transition ast.Stmt
		if i == n-1 {
			transition = &ast.BranchStmt{Tok: token.BREAK}
		} else {
			transition = &ast.AssignStmt{
				Tok: token.ASSIGN,
				Lhs: []ast.Expr{&ast.Ident{Name: stateVarName}},
				Rhs: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", i+1)}},
			}
		}

		// Assemble case body: processed statement + state transition
		caseBody := []ast.Stmt{processedStmt, transition}

		caseBlock := &ast.BlockStmt{
			List: caseBody,
		}

		caseClause := &ast.CaseClause{
			List: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", i)}},
			Body: []ast.Stmt{caseBlock},
		}
		caseClauses = append(caseClauses, caseClause)
	}

	// Add default case as safety fallback to exit the dispatcher loop
	defaultCase := &ast.CaseClause{
		Body: []ast.Stmt{&ast.BranchStmt{Tok: token.BREAK}},
	}
	caseClauses = append(caseClauses, defaultCase)

	// Phase 4: Assemble the new function body
	finalBody := []ast.Stmt{}
	finalBody = append(finalBody, hoistedDecls...)

	// Initialize state variable: state := 0
	stateInit := &ast.AssignStmt{
		Tok: token.DEFINE,
		Lhs: []ast.Expr{&ast.Ident{Name: stateVarName}},
		Rhs: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "0"}},
	}
	finalBody = append(finalBody, stateInit)

	// Create the dispatcher: for { switch state { ... } }
	dispatcherLoop := &ast.ForStmt{
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.SwitchStmt{
					Tag: &ast.Ident{Name: stateVarName},
					Body: &ast.BlockStmt{
						List: caseClauses,
					},
				},
			},
		},
	}
	finalBody = append(finalBody, dispatcherLoop)

	// Replace the original function body with the flattened version
	fn.Body.List = finalBody

	return nil
}

// collectVarsWithTypes recursively traverses statements to collect variables
// declared with short declaration syntax (:=). Infers types from RHS expressions
// and stores them in varMap for later hoisting. Handles nested blocks, if/for/range.
func (f *CFF) collectVarsWithTypes(stmt ast.Stmt, varMap map[string]*VarInfo) {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		if s.Tok == token.DEFINE {
			for i, lhs := range s.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok {
					if _, exists := varMap[ident.Name]; !exists {
						varType := f.inferTypeFromExpr(s.Rhs[i], varMap)
						varMap[ident.Name] = &VarInfo{
							Name:    ident.Name,
							Type:    varType,
							IsParam: false,
						}
					}
				}
			}
		}
	case *ast.BlockStmt:
		for _, inner := range s.List {
			f.collectVarsWithTypes(inner, varMap)
		}
	case *ast.IfStmt:
		if s.Init != nil {
			f.collectVarsWithTypes(s.Init, varMap)
		}
		f.collectVarsWithTypes(s.Body, varMap)
		if s.Else != nil {
			if elseBlock, ok := s.Else.(*ast.BlockStmt); ok {
				f.collectVarsWithTypes(elseBlock, varMap)
			}
		}
	case *ast.ForStmt:
		if s.Init != nil {
			f.collectVarsWithTypes(s.Init, varMap)
		}
		f.collectVarsWithTypes(s.Body, varMap)
	case *ast.RangeStmt:
		if s.Key != nil {
			if ident, ok := s.Key.(*ast.Ident); ok && s.Tok == token.DEFINE {
				if _, exists := varMap[ident.Name]; !exists {
					varMap[ident.Name] = &VarInfo{
						Name:    ident.Name,
						Type:    f.inferTypeFromRangeKey(s.X, varMap),
						IsParam: false,
					}
				}
			}
		}
		if s.Value != nil {
			if ident, ok := s.Value.(*ast.Ident); ok && s.Tok == token.DEFINE {
				if _, exists := varMap[ident.Name]; !exists {
					varMap[ident.Name] = &VarInfo{
						Name:    ident.Name,
						Type:    f.inferTypeFromRangeValue(s.X, varMap),
						IsParam: false,
					}
				}
			}
		}
		f.collectVarsWithTypes(s.Body, varMap)
	}
}

// resolveVarTypes performs a second pass to refine variable types using
// already collected information from varMap. This handles cases where
// a variable's type depends on another variable declared earlier.
func (f *CFF) resolveVarTypes(stmt ast.Stmt, varMap map[string]*VarInfo) {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		if s.Tok == token.DEFINE {
			for i, lhs := range s.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok {
					if info, exists := varMap[ident.Name]; exists {
						newType := f.inferTypeFromExpr(s.Rhs[i], varMap)
						info.Type = newType
					}
				}
			}
		}
	case *ast.BlockStmt:
		for _, inner := range s.List {
			f.resolveVarTypes(inner, varMap)
		}
	case *ast.IfStmt:
		if s.Init != nil {
			f.resolveVarTypes(s.Init, varMap)
		}
		f.resolveVarTypes(s.Body, varMap)
		if s.Else != nil {
			if elseBlock, ok := s.Else.(*ast.BlockStmt); ok {
				f.resolveVarTypes(elseBlock, varMap)
			}
		}
	case *ast.ForStmt:
		if s.Init != nil {
			f.resolveVarTypes(s.Init, varMap)
		}
		f.resolveVarTypes(s.Body, varMap)
	}
}

// inferTypeFromExpr infers the Go type of an expression by pattern matching
// on AST node types. Handles literals, identifiers (with varMap lookup),
// function calls (make, chan), channel operations, and composite types.
// Returns interface{} as fallback for unknown types.
func (f *CFF) inferTypeFromExpr(expr ast.Expr, varMap map[string]*VarInfo) ast.Expr {
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
		// Lookup type from collected variable information
		if info, exists := varMap[e.Name]; exists {
			return info.Type
		}
		return &ast.Ident{Name: "string"}
	case *ast.CallExpr:
		if ident, ok := e.Fun.(*ast.Ident); ok {
			switch ident.Name {
			case "make":
				// Handle make(chan T) -> chan T
				if len(e.Args) > 0 {
					argType := f.inferTypeFromExpr(e.Args[0], varMap)
					if chanType, ok := argType.(*ast.ChanType); ok {
						return chanType
					}
					return &ast.ChanType{Value: argType}
				}
			case "chan":
				if len(e.Args) > 0 {
					return &ast.ChanType{Value: f.inferTypeFromExpr(e.Args[0], varMap)}
				}
			}
		}
		return &ast.Ident{Name: "string"}
	case *ast.UnaryExpr:
		// Handle address-of: &Struct{} -> *Struct
		if e.Op == token.AND {
			if comp, ok := e.X.(*ast.CompositeLit); ok {
				return &ast.StarExpr{X: comp.Type}
			}
		}
		// Handle receive: <-ch -> element type of channel
		if e.Op == token.ARROW {
			chanType := f.inferTypeFromExpr(e.X, varMap)
			if ct, ok := chanType.(*ast.ChanType); ok {
				return ct.Value
			}
		}
	case *ast.CompositeLit:
		return e.Type
	case *ast.BinaryExpr:
		return f.inferTypeFromExpr(e.X, varMap)
	case *ast.ArrayType:
		// ArrayType with Len==nil represents a slice
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

// inferTypeFromRangeKey infers the type of a range loop's key variable.
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

// inferTypeFromRangeValue infers the type of a range loop's value variable.
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
func (f *CFF) convertDefineToAssign(stmt ast.Stmt) ast.Stmt {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		if s.Tok == token.DEFINE {
			newStmt := &ast.AssignStmt{
				Lhs:    s.Lhs,
				Tok:    token.ASSIGN,
				TokPos: s.TokPos,
				Rhs:    s.Rhs,
			}
			return newStmt
		}
	case *ast.BlockStmt:
		newList := make([]ast.Stmt, len(s.List))
		for i, inner := range s.List {
			newList[i] = f.convertDefineToAssign(inner)
		}
		return &ast.BlockStmt{List: newList}
	case *ast.IfStmt:
		if s.Init != nil {
			s.Init = f.convertDefineToAssign(s.Init)
		}
		s.Body = f.convertDefineToAssign(s.Body).(*ast.BlockStmt)
		if s.Else != nil {
			if elseBlock, ok := s.Else.(*ast.BlockStmt); ok {
				s.Else = f.convertDefineToAssign(elseBlock)
			}
		}
		return s
	case *ast.ForStmt:
		if s.Init != nil {
			s.Init = f.convertDefineToAssign(s.Init)
		}
		s.Body = f.convertDefineToAssign(s.Body).(*ast.BlockStmt)
		return s
	case *ast.RangeStmt:
		if s.Tok == token.DEFINE {
			s.Tok = token.ASSIGN
		}
		s.Body = f.convertDefineToAssign(s.Body).(*ast.BlockStmt)
		return s
	}
	return stmt
}
