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

// Build compiles the obfuscated project by executing "go build" with the flags,
// output path, GOOS, and GOARCH values from the configuration. The working
// directory is set to the obfuscated project's root (cfg.Obfuscator.TargetPath).
func (b *Builder) Build() error {
	args := []string{"build"}

	if len(b.cfg.Builder.Flags) > 0 {
		args = append(args, b.cfg.Builder.Flags...)
	}

	outputPath, err := filepath.Abs(b.cfg.Builder.OutputPath)
	if err != nil {
		b.log.Error(b.cfg.CurLocale["bld.err.abs"], slog.String("error", err.Error()))
		return err
	}

	if b.cfg.Builder.OutputPath != "" {
		info, statErr := os.Stat(outputPath)
		if statErr == nil && info.IsDir() {
			binaryName := b.cfg.Builder.BinaryName
			if binaryName == "" {
				binaryName = filepath.Base(b.cfg.Obfuscator.TargetPath)
			}
			outputPath = filepath.Join(outputPath, binaryName)
		}
		args = append(args, "-o", outputPath)
	}

	mainPath, err := b.findMain()
	if err != nil {
		b.log.Error(b.cfg.CurLocale["bld.err.main"], slog.String("error", err.Error()))
		return err
	}
	args = append(args, mainPath)

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

// findMain walks the obfuscated project tree and returns the directory that
// contains a file named "main.go". Returns an error if no such file exists.
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
