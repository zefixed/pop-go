package obfuscator

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"log/slog"
	"os"
	"time"
)

// DeleteComments removes all doc-comments and inline comments from every
// source file in the loaded packages. Each file is re-formatted with
// go/format after stripping and written back to its path in the temp directory.
func (o *Obfuscator) DeleteComments() {
	o.log.Info(o.cfg.CurLocale["obf.info.start.del.comm"])
	t := time.Now()

	for _, pkg := range o.pkgs {
		if len(pkg.Errors) > 0 {
			for _, e := range pkg.Errors {
				o.log.Error(fmt.Sprintf("%s: %s", o.cfg.CurLocale["obf.err.pkg"], e.Error()))
			}
			continue
		}

		for _, file := range pkg.Syntax {
			filename := pkg.Fset.File(file.Pos()).Name()
			o.log.Debug(o.cfg.CurLocale["obf.debug.processing.file"], slog.String("filename", filename))

			ast.Inspect(file, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.File:
					x.Doc = nil
					x.Comments = nil
				case *ast.FuncDecl:
					x.Doc = nil
				case *ast.GenDecl:
					x.Doc = nil
				case *ast.TypeSpec:
					x.Doc = nil
				case *ast.Field:
					x.Doc = nil
					x.Comment = nil
				case *ast.ValueSpec:
					x.Doc = nil
				}
				return true
			})

			var buf bytes.Buffer
			if err := format.Node(&buf, pkg.Fset, file); err != nil {
				o.log.Error(fmt.Sprintf("%s: %s", o.cfg.CurLocale["obf.err.format"], err.Error()))
				continue
			}

			if err := os.WriteFile(filename, buf.Bytes(), 0644); err != nil {
				o.log.Error(fmt.Sprintf("%s: %s", o.cfg.CurLocale["obf.err.write"], err.Error()))
				continue
			}
		}
	}

	o.log.Info(o.cfg.CurLocale["obf.info.end.del.comm"], slog.String("duration", time.Since(t).String()))
}
