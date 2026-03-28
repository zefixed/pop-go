package tester

import (
	"fmt"
	"log/slog"
	"os/exec"
	"time"
)

// RunTests executes `go test [extraFlags] ./...` in the specified directory.
// stage is a human-readable label used in log messages (e.g. "pre-obfuscation" / "post-obfuscation").
// extraFlags are inserted between "test" and "./..." (e.g. "-vet=off").
func (t *Tester) RunTests(dir string, stage string, extraFlags ...string) error {
	t.log.Info(
		t.cfg.CurLocale["tst.info.start"],
		slog.String("stage", stage),
		slog.String("path", dir),
	)

	start := time.Now()

	args := append([]string{"test"}, extraFlags...)
	args = append(args, "./...")
	cmd := exec.Command("go", args...)
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.log.Error(
			t.cfg.CurLocale["tst.err.failed"],
			slog.String("stage", stage),
			slog.String("output", string(output)),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("%s [%s]: %w\n%s",
			t.cfg.CurLocale["tst.err.failed"], stage, err, string(output))
	}

	t.log.Info(
		t.cfg.CurLocale["tst.info.end"],
		slog.String("stage", stage),
		slog.String("duration", time.Since(start).String()),
	)

	if len(output) > 0 {
		t.log.Debug(
			t.cfg.CurLocale["tst.debug.output"],
			slog.String("stage", stage),
			slog.String("output", string(output)),
		)
	}

	return nil
}
