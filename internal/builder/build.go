package builder

import (
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	pkgfs "pop-go/pkg/fs"
	"time"
)

func (b *Builder) Build() error {
	args := []string{"build"}

	// Adding custom flags
	if len(b.cfg.Builder.Flags) > 0 {
		args = append(args, b.cfg.Builder.Flags...)
	}

	// Adding output (-o) flag
	outputPath, err := filepath.Abs(b.cfg.Builder.OutputPath)
	if err != nil {
		b.log.Error(b.cfg.CurLocale["bld.err.abs"], slog.String("error", err.Error()))
		return err
	}
	if b.cfg.Builder.OutputPath != "" {
		args = append(args, "-o", outputPath)
	}

	// Adding path to main.go
	mainPath, err := b.findMain()
	if err != nil {
		b.log.Error(b.cfg.CurLocale["bld.err.main"], slog.String("error", err.Error()))
		return err
	}
	args = append(args, mainPath)

	// Executing go build command
	cmd := exec.Command("go", args...)

	cmd.Dir = b.cfg.Obfuscator.TargetPath

	env := os.Environ()
	if b.cfg.Builder.GOOS != "" {
		env = append(env, "GOOS="+b.cfg.Builder.GOOS)
	}

	if b.cfg.Builder.GOARCH != "" {
		env = append(env, "GOARCH="+b.cfg.Builder.GOARCH)
	}
	cmd.Env = env

	b.log.Info(b.cfg.CurLocale["bld.info.start"])
	t := time.Now()
	output, err := cmd.CombinedOutput()
	if err != nil {
		b.log.Error(b.cfg.CurLocale["bld.err"], slog.String("error", err.Error()), slog.String("output", string(output)))
		return err
	}

	b.log.Info(b.cfg.CurLocale["bld.info.end"], slog.String("duration", time.Since(t).String()))
	return nil
}

func (b *Builder) findMain() (string, error) {
	snapshot, err := pkgfs.TakeSnapshot(b.cfg.Obfuscator.TargetPath)
	if err != nil {
		return "", err
	}

	for _, file := range snapshot {
		if filepath.Base(file) == "main.go" {
			dir, _ := filepath.Split(file)
			return dir, nil
		}
	}

	return "", errors.New("not found main.go")
}
