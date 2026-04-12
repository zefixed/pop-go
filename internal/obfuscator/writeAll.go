package obfuscator

import (
	"bytes"
	"fmt"
	"go/format"
	"log/slog"
	"os"
)

// WriteAll serialises every modified AST file back to disk using go/format.
// It iterates over all canonical packages and writes the formatted source of
// each file in pkg.Syntax to its original absolute path in the temp directory.
func (o *Obfuscator) WriteAll() error {
	o.log.Info(o.cfg.CurLocale["obf.info.writing.files"])
	for _, pkg := range o.pkgs {
		if pkg.TypesInfo == nil {
			continue
		}

		for _, file := range pkg.Syntax {
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
