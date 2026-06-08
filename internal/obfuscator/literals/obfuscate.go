package literals

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
	"log/slog"
	"pop-go/pkg/util"
	"strconv"
	"strings"
	"time"
)

// Obfuscate encrypts all string literals in every non-test source file using
// the encryption profile selected by level. For each file a unique decrypt
// function is injected and every plain string literal is replaced with a call
// to it. Test files are skipped to avoid breaking test assertions.
func (l *Literals) Obfuscate(level string) error {
	l.log.Info(l.cfg.CurLocale["obf.info.start.obf.lit"])
	t := time.Now()

	profile := getObfuscationProfile(level)
	if profile == nil {
		return fmt.Errorf(l.cfg.CurLocale["obf.err.lit.profile"], level)
	}

	for _, pkg := range l.pkgs {
		for _, file := range util.SortedSyntax(pkg) {
			absPath := pkg.Fset.File(file.Pos()).Name()
			if strings.HasSuffix(absPath, "_test.go") {
				continue
			}

			l.log.Debug(l.cfg.CurLocale["obf.debug.processing.file"], slog.String("filename", absPath))

			if err := l.obfuscateAST(file, pkg, pkg.Fset, profile, pkg.TypesInfo); err != nil {
				return fmt.Errorf(l.cfg.CurLocale["obf.err.lit"], absPath, err)
			}
		}
	}

	l.log.Info(l.cfg.CurLocale["obf.info.end.obf.lit"], slog.String("duration", time.Since(t).String()))
	return nil
}

// getObfuscationProfile maps a level string to its ObfuscateLiteralsProfile
// implementation. Returns nil for unknown or unimplemented levels.
func getObfuscationProfile(level string) ObfuscateLiteralsProfile {
	switch level {
	case "easy":
		return &XorObfuscateLiteralsProfile{}
	case "medium":
		return &AESObfuscateLiteralsProfile{}
	}
	return nil
}

// obfuscateAST encrypts every eligible string literal in f. A decrypt function
// is injected once per file and each literal is replaced with a call to it.
//
// Literals are skipped in the following cases:
//   - Inside const declarations (must remain compile-time constants).
//   - Struct field tags (stored as *ast.BasicLit but not replaceable with a call).
//   - Import path specifications.
//   - Already-wrapped calls to the decrypt function itself.
//   - Type conversions (e.g. MyType("value")) where Fun is not a *types.Signature.
//   - Compiler directives (strings starting with "//go:").
//   - Empty strings ("") — replacing them breaks named-string-type assignments.
func (l *Literals) obfuscateAST(f *ast.File, pkg *packages.Package, fset *token.FileSet, profile ObfuscateLiteralsProfile, typesInfo *types.Info) error {
	decryptKey := profile.GenerateKey(l.cfg.Obfuscator.Seed)
	decryptFuncName := util.GenerateUniqueName(l.r, l.used)
	decryptFunc := profile.DecryptFunction(decryptKey, decryptFuncName)
	importAliases := fileImportAliases(f)

	hasDecrypt := false
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == decryptFuncName {
			hasDecrypt = true
			break
		}
	}

	if !hasDecrypt {
		fullCode := "package temp\n" + decryptFunc
		extra, err := parser.ParseFile(fset, "", fullCode, 0)
		if err != nil {
			return err
		}
		if len(extra.Decls) == 1 {
			f.Decls = append(f.Decls, extra.Decls[0])
		}

		for _, pkg := range profile.RequiredImports() {
			if !hasImport(f, pkg) {
				addImport(f, pkg)
			}
		}
	}

	astutil.Apply(f, func(cursor *astutil.Cursor) bool {
		if genDecl, ok := cursor.Node().(*ast.GenDecl); ok && genDecl.Tok == token.CONST {
			return false
		}

		if lit, ok := cursor.Node().(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if parent, ok := cursor.Parent().(*ast.Field); ok && parent.Tag == lit {
				return true
			}

			if _, ok := cursor.Parent().(*ast.ImportSpec); ok {
				return true
			}

			parent := cursor.Parent()
			if call, ok := parent.(*ast.CallExpr); ok {
				if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == decryptFuncName {
					return true
				}
				// Skip type conversions: if Fun's type is not a *types.Signature
				// the call is a type conversion, not a function call.
				if typesInfo != nil {
					if tv, ok := typesInfo.Types[call.Fun]; ok {
						if _, isSig := tv.Type.(*types.Signature); !isSig {
							return true
						}
					}
				} else {
					// Without type info, conservatively skip single-argument calls
					// with a bare identifier Fun (e.g. MyType("value")).
					if _, ok := call.Fun.(*ast.Ident); ok && len(call.Args) == 1 {
						return true
					}
				}
			}

			if isCompilerDirective(lit.Value) {
				return true
			}

			s, _ := strconv.Unquote(lit.Value)
			if s == "" {
				return true
			}
			enc := profile.EncryptString(s, decryptKey)
			newLit := &ast.BasicLit{
				ValuePos: lit.ValuePos,
				Kind:     token.STRING,
				Value:    strconv.Quote(enc),
			}
			call := &ast.CallExpr{
				Fun:  &ast.Ident{Name: decryptFuncName},
				Args: []ast.Expr{newLit},
			}

			if convType := namedStringConversionType(lit, pkg.PkgPath, importAliases, typesInfo); convType != nil {
				cursor.Replace(&ast.CallExpr{
					Fun:  convType,
					Args: []ast.Expr{call},
				})
				return false
			}

			cursor.Replace(call)
			return false
		}
		return true
	}, nil)

	return nil
}

func fileImportAliases(f *ast.File) map[string]string {
	aliases := make(map[string]string)
	for _, imp := range f.Imports {
		path := cleanImportPath(imp.Path.Value)
		if imp.Name != nil && imp.Name.Name != "_" && imp.Name.Name != "." {
			aliases[path] = imp.Name.Name
			continue
		}
		parts := strings.Split(path, "/")
		if len(parts) > 0 {
			aliases[path] = parts[len(parts)-1]
		}
	}
	return aliases
}

func namedStringConversionType(lit *ast.BasicLit, currentPkgPath string, importAliases map[string]string, typesInfo *types.Info) ast.Expr {
	if typesInfo == nil {
		return nil
	}
	tv, ok := typesInfo.Types[lit]
	if !ok || tv.Type == nil {
		return nil
	}
	named, ok := tv.Type.(*types.Named)
	if !ok {
		return nil
	}
	if basic, ok := named.Underlying().(*types.Basic); !ok || basic.Kind() != types.String {
		return nil
	}
	obj := named.Obj()
	if obj == nil {
		return nil
	}
	if obj.Pkg() == nil || obj.Pkg().Path() == currentPkgPath {
		return &ast.Ident{Name: obj.Name()}
	}
	localName := obj.Pkg().Name()
	if alias, ok := importAliases[obj.Pkg().Path()]; ok {
		localName = alias
	}
	return &ast.SelectorExpr{
		X:   &ast.Ident{Name: localName},
		Sel: &ast.Ident{Name: obj.Name()},
	}
}

// isCompilerDirective reports whether the quoted string literal value s
// represents a Go compiler directive (e.g. "//go:generate ...").
func isCompilerDirective(s string) bool {
	return len(s) > 4 && s[0:4] == `"//g`
}

// hasImport reports whether file f already imports the package at path.
func hasImport(f *ast.File, path string) bool {
	for _, imp := range f.Imports {
		if cleanImportPath(imp.Path.Value) == path {
			return true
		}
	}
	return false
}

// cleanImportPath strips surrounding double-quotes from the raw import path
// value as it appears in the AST (e.g. `"fmt"` → `fmt`).
func cleanImportPath(s string) string {
	return strings.Trim(s, `"`)
}

// addImport adds an import for path to file f, creating an import declaration
// block if one does not already exist.
func addImport(f *ast.File, path string) {
	importSpec := &ast.ImportSpec{
		Path: &ast.BasicLit{
			Kind:  token.STRING,
			Value: `"` + path + `"`,
		},
	}

	var importDecl *ast.GenDecl
	for _, decl := range f.Decls {
		if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.IMPORT {
			importDecl = gd
			break
		}
	}

	if importDecl == nil {
		importDecl = &ast.GenDecl{
			Tok:   token.IMPORT,
			Specs: []ast.Spec{importSpec},
		}
		f.Decls = append([]ast.Decl{importDecl}, f.Decls...)
	} else {
		importDecl.Specs = append(importDecl.Specs, importSpec)
	}
}
