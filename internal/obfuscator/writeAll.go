package obfuscator

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"golang.org/x/tools/go/packages"
	"log/slog"
	"os"
	"sort"
)

func (o *Obfuscator) WriteAll() error {
	o.log.Info(o.cfg.CurLocale["obf.info.writing.files"])

	sortedPkgs := make([]*packages.Package, len(o.pkgs))
	copy(sortedPkgs, o.pkgs)
	sort.Slice(sortedPkgs, func(a, b int) bool {
		return sortedPkgs[a].PkgPath < sortedPkgs[b].PkgPath
	})

	for _, pkg := range sortedPkgs {
		if pkg.TypesInfo == nil {
			continue
		}

		sortedFiles := make([]*ast.File, len(pkg.Syntax))
		copy(sortedFiles, pkg.Syntax)
		sort.Slice(sortedFiles, func(a, b int) bool {
			return pkg.Fset.File(sortedFiles[a].Pos()).Name() < pkg.Fset.File(sortedFiles[b].Pos()).Name()
		})

		for _, file := range sortedFiles {
			filePath := pkg.Fset.File(file.Pos()).Name()
			o.log.Debug(o.cfg.CurLocale["obf.debug.writing.file"], slog.String("filename", filePath))

			var buf bytes.Buffer
			if err := format.Node(&buf, pkg.Fset, file); err != nil {
				return fmt.Errorf(o.cfg.CurLocale["obf.err.fmt.node"], filePath, err)
			}
			if err := os.WriteFile(filePath, buf.Bytes(), 0644); err != nil {
				return fmt.Errorf(o.cfg.CurLocale["obf.err.write.file"], filePath, err)
			}
		}
	}
	return nil
}
