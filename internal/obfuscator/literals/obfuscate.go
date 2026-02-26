package literals

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"golang.org/x/tools/go/ast/astutil"
	"log/slog"
	"os"
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
			if err := l.obfuscateFile(absPath, profile); err != nil {
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

func (l *Literals) obfuscateFile(filePath string, profile ObfuscateLiteralsProfile) error {
	// Generate decryption key and function template using seed
	decryptKey := profile.GenerateKey(l.cfg.Obfuscator.Seed)
	decryptFuncName := util.GenerateDecryptFuncName(l.cfg.Obfuscator.Seed)
	decryptFunc := profile.DecryptFunction(decryptKey, decryptFuncName)

	// Parse file into AST for analysis
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return err
	}

	// Check for existing decrypt function
	hasDecrypt := false
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == decryptFuncName {
			hasDecrypt = true
			break
		}
	}

	// Insert generated decrypt function if missing
	if !hasDecrypt {
		fullCode := "package temp\n" + decryptFunc
		extra, err := parser.ParseFile(fset, "", fullCode, 0)
		if err != nil {
			return err
		}
		if len(extra.Decls) == 1 {
			f.Decls = append(f.Decls, extra.Decls[0])
		}

		// Add required imports for some profile
		for _, pkg := range profile.RequiredImports() {
			if !hasImport(f, pkg) {
				addImport(f, pkg)
			}
		}
	}

	// Process string literals for obfuscation
	astutil.Apply(f, nil, func(cursor *astutil.Cursor) bool {
		if lit, ok := cursor.Node().(*ast.BasicLit); ok && lit.Kind == token.STRING {
			// Skip already obfuscated strings
			parent := cursor.Parent()
			if call, ok := parent.(*ast.CallExpr); ok {
				if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == decryptFuncName {
					return true
				}
			}

			// Exclude compiler directives and imports
			if isCompilerDirective(lit.Value) || isImportString(cursor) {
				return true
			}

			// Encrypt string and wrap with decrypt call
			s, _ := strconv.Unquote(lit.Value)
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
		}
		return true
	})

	// Format modified AST and write to file
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		return err
	}
	return os.WriteFile(filePath, buf.Bytes(), 0644)
}

func isCompilerDirective(s string) bool {
	return len(s) > 4 && s[0:4] == `"//g`
}

func isImportString(cursor *astutil.Cursor) bool {
	parent := cursor.Parent()
	if _, ok := parent.(*ast.ImportSpec); ok {
		return true
	}
	return false
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
