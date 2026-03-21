package literals

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/ast/astutil"
	"log/slog"
	"pop-go/pkg/util"
	"strconv"
	"strings"
	"time"
)

func (l *Literals) Obfuscate(level string) error {
	l.log.Info(l.cfg.CurLocale["obf.info.start.obf.lit"])
	t := time.Now()

	// Getting obfuscation profile
	profile := getObfuscationProfile(level)
	if profile == nil {
		return fmt.Errorf(l.cfg.CurLocale["obf.err.lit.profile"], level)
	}

	for _, pkg := range l.pkgs {
		for _, file := range pkg.Syntax {
			// Getting absolute path of file
			absPath := pkg.Fset.File(file.Pos()).Name()

			l.log.Debug(l.cfg.CurLocale["obf.debug.processing.file"], slog.String("filename", absPath))

			// Obfuscate literals in file with given obfuscation profile
			if err := l.obfuscateAST(file, pkg.Fset, profile, pkg.TypesInfo); err != nil {
				return fmt.Errorf(l.cfg.CurLocale["obf.err.lit"], absPath, err)
			}
		}
	}

	l.log.Info(l.cfg.CurLocale["obf.info.end.obf.lit"], slog.String("duration", time.Since(t).String()))
	return nil
}

func getObfuscationProfile(level string) ObfuscateLiteralsProfile {
	switch level {
	case "easy":
		return &XorObfuscateLiteralsProfile{}
	case "medium":
		return &AESObfuscateLiteralsProfile{}
	case "hard":
		return nil
	}
	return nil
}

func (l *Literals) obfuscateAST(f *ast.File, fset *token.FileSet, profile ObfuscateLiteralsProfile, typesInfo *types.Info) error {
	decryptKey := profile.GenerateKey(l.cfg.Obfuscator.Seed)
	decryptFuncName := util.GenerateUniqueName(l.cfg.Obfuscator.Seed)
	decryptFunc := profile.DecryptFunction(decryptKey, decryptFuncName)

	// Проверка на наличие функции дешифровки
	hasDecrypt := false
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == decryptFuncName {
			hasDecrypt = true
			break
		}
	}

	// Добавляем функцию дешифровки при необходимости
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

	// Process literals in pre-handler to allow safe node replacement.
	// Return false after replacement to skip traversing children of the new node.
	astutil.Apply(f, func(cursor *astutil.Cursor) bool {
		// Skip const declarations entirely — const values must be compile-time
		// constants, and function calls are not allowed there.
		if genDecl, ok := cursor.Node().(*ast.GenDecl); ok && genDecl.Tok == token.CONST {
			return false
		}

		if lit, ok := cursor.Node().(*ast.BasicLit); ok && lit.Kind == token.STRING {
			// Skip struct tags: they are stored as *ast.BasicLit in ast.Field.Tag,
			// which is not an ast.Expr field and cannot be replaced with CallExpr.
			if parent, ok := cursor.Parent().(*ast.Field); ok && parent.Tag == lit {
				return true
			}

			// Skip import paths: they are also BasicLit but not replaceable with CallExpr
			if _, ok := cursor.Parent().(*ast.ImportSpec); ok {
				return true
			}

			parent := cursor.Parent()
			if call, ok := parent.(*ast.CallExpr); ok {
				if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == decryptFuncName {
					return true
				}
				// Skip literals used as arguments to type conversions (e.g. TestType("value")).
				// A type conversion has exactly one argument and Fun refers to a type, not a function.
				// We use typesInfo to distinguish: if Fun's type is a *types.Signature, it's a
				// function call (safe to obfuscate). If it's any other type, it's a conversion.
				if typesInfo != nil {
					if tv, ok := typesInfo.Types[call.Fun]; ok {
						if _, isSig := tv.Type.(*types.Signature); !isSig {
							// Fun is a type, not a function — this is a type conversion, skip.
							return true
						}
					}
				} else {
					// No type info: conservatively skip single-arg calls with bare Ident Fun
					// (e.g. MyType("value")) but allow SelectorExpr calls (fmt.Printf etc.)
					if _, ok := call.Fun.(*ast.Ident); ok && len(call.Args) == 1 {
						return true
					}
				}
			}

			if isCompilerDirective(lit.Value) {
				return true
			}

			s, _ := strconv.Unquote(lit.Value)
			// Skip empty strings — obfuscating "" produces DecryptFunc("") which
			// returns string and breaks assignments to named string types (e.g. type TestType string).
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

			cursor.Replace(call)
			return false
		}
		return true
	}, nil)

	return nil
}

func isCompilerDirective(s string) bool {
	return len(s) > 4 && s[0:4] == `"//g`
}

func hasImport(f *ast.File, path string) bool {
	// Check if package is already imported
	for _, imp := range f.Imports {
		if cleanImportPath(imp.Path.Value) == path {
			return true
		}
	}
	return false
}

func cleanImportPath(s string) string {
	// Clean import path from quotes
	return strings.Trim(s, `"`)
}

func addImport(f *ast.File, path string) {
	importSpec := &ast.ImportSpec{
		Path: &ast.BasicLit{
			Kind:  token.STRING,
			Value: `"` + path + `"`,
		},
	}

	// Find or create import declaration
	var importDecl *ast.GenDecl
	for _, decl := range f.Decls {
		if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.IMPORT {
			importDecl = gd
			break
		}
	}

	// Add new import to file
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
