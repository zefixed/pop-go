package builder

import (
	"log/slog"
	"os"
	"os/exec"
	"time"
)

func (b *Builder) Build() error {
	args := []string{"build"}

	if len(b.cfg.Builder.Flags) > 0 {
		args = append(args, b.cfg.Builder.Flags...)
	}

	if b.cfg.Builder.OutputPath != "" {
		args = append(args, "-o", b.cfg.Builder.OutputPath)
	}

	if b.cfg.Obfuscator.TargetPath != "" {
		args = append(args, b.cfg.Obfuscator.TargetPath)
	}

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
		b.log.Error(
			b.cfg.CurLocale["bld.err"],
			slog.String("error", err.Error()),
			slog.String("output", string(output)),
		)
		return err
	}

	b.log.Info(b.cfg.CurLocale["bld.info.end"], slog.String("duration", time.Since(t).String()))
	return nil
}
