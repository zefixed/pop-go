package obfuscator

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"golang.org/x/tools/go/ast/astutil"
	"log/slog"
	"math/rand"
	"os"
	"strconv"
	"time"
)

func (o *Obfuscator) ObfuscateLiteralsEasy() {
	o.log.Info(o.cfg.CurLocale["obf.info.start.obf.lit"])
	t := time.Now()

	for _, pkg := range o.pkgs {
		for _, file := range pkg.Syntax {
			absPath := pkg.Fset.File(file.Pos()).Name()
			o.log.Debug(o.cfg.CurLocale["obf.debug.processing.file"], slog.String("filename", absPath))
			if err := o.obfuscateFile(absPath); err != nil {
				o.CritErr = err
				o.log.Error("Obfuscation failed", slog.String("file", absPath), slog.Any("err", err))
			}
		}
	}

	o.log.Info(o.cfg.CurLocale["obf.info.end.obf.lit"], slog.String("duration", time.Since(t).String()))
}

func xorString(s string, key byte) string {
	b := []byte(s)
	for i := range b {
		b[i] ^= key
	}
	return string(b)
}

func (o *Obfuscator) obfuscateFile(filePath string) error {
	decryptKey := o.generateDecryptKey()
	const decryptFuncTemplate = `
	func decrypt(s string) string {
		b := []byte(s)
		for i := range b {
			b[i] ^= %d
		}
		return string(b)
	}
	`

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return err
	}

	hasDecrypt := false
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "decrypt" {
			hasDecrypt = true
			break
		}
	}

	if !hasDecrypt {
		code := fmt.Sprintf(decryptFuncTemplate, decryptKey)
		fullCode := "package temp\n" + code
		extra, err := parser.ParseFile(fset, "", fullCode, 0)
		if err != nil {
			return err
		}
		if len(extra.Decls) == 1 {
			f.Decls = append(f.Decls, extra.Decls[0])
		}
	}

	astutil.Apply(f, nil, func(cursor *astutil.Cursor) bool {
		if lit, ok := cursor.Node().(*ast.BasicLit); ok && lit.Kind == token.STRING {
			parent := cursor.Parent()
			if call, ok := parent.(*ast.CallExpr); ok {
				if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "decrypt" {
					return true
				}
			}

			if isCompilerDirective(lit.Value) || isImportString(lit, cursor) {
				return true
			}

			s, _ := strconv.Unquote(lit.Value)
			enc := xorString(s, decryptKey)
			newLit := &ast.BasicLit{
				ValuePos: lit.ValuePos,
				Kind:     token.STRING,
				Value:    strconv.Quote(enc),
			}
			call := &ast.CallExpr{
				Fun:  &ast.Ident{Name: "decrypt"},
				Args: []ast.Expr{newLit},
			}

			cursor.Replace(call)
		}
		return true
	})

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		return err
	}
	return os.WriteFile(filePath, buf.Bytes(), 0644)
}

func isCompilerDirective(s string) bool {
	return len(s) > 4 && s[0:4] == `"//g`
}

func isImportString(lit *ast.BasicLit, cursor *astutil.Cursor) bool {
	parent := cursor.Parent()
	if _, ok := parent.(*ast.ImportSpec); ok {
		return true
	}
	return false
}

func (o *Obfuscator) generateDecryptKey() byte {
	if o.cfg.Obfuscator.Seed == 0 {
		o.cfg.Obfuscator.Seed = rand.Int63()
	}
	localRand := rand.New(rand.NewSource(o.cfg.Obfuscator.Seed))
	return byte(localRand.Intn(256))
}
