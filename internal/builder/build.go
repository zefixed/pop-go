package builder

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	pkgfs "pop-go/pkg/fs"
	"strings"
	"time"
)

// Build compiles the obfuscated project by executing "go build" with the flags,
// output path, GOOS, and GOARCH values from the configuration. The working
// directory is set to the obfuscated project's root (cfg.Obfuscator.TargetPath).
func (b *Builder) Build() error {
	args := []string{"build"}

	if len(b.cfg.Builder.Flags) > 0 {
		args = append(args, normalizeBuildFlags(b.cfg.Builder.Flags, b.cfg.Obfuscator.Seed)...)
	} else {
		args = append(args, "-ldflags", buildIDFlagValue(b.cfg.Obfuscator.Seed), "-buildvcs=false")
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
	mainArg, err := filepath.Rel(b.cfg.Obfuscator.TargetPath, mainPath)
	if err != nil {
		b.log.Error(b.cfg.CurLocale["bld.err.main"], slog.String("error", err.Error()))
		return err
	}
	mainArg = filepath.ToSlash(mainArg)
	if mainArg == "." {
		args = append(args, ".")
	} else {
		args = append(args, "./"+mainArg)
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
		b.log.Error(b.cfg.CurLocale["bld.err"], slog.String("error", err.Error()), slog.String("output", string(output)))
		return err
	}

	b.log.Info(b.cfg.CurLocale["bld.info.end"], slog.String("duration", time.Since(t).String()))
	return nil
}

func normalizeBuildFlags(flags []string, seed int64) []string {
	args := make([]string, 0, len(flags)+3)
	ldflagsSeen := false

	for i := 0; i < len(flags); i++ {
		flag := flags[i]
		switch {
		case strings.HasPrefix(flag, "-buildvcs="):
			continue
		case flag == "-buildvcs":
			if i+1 < len(flags) {
				i++
			}
			continue
		case flag == "-ldflags":
			ldflagsSeen = true
			value := ""
			if i+1 < len(flags) {
				value = flags[i+1]
				i++
			}
			args = append(args, "-ldflags", mergeLDFlags(value, buildIDFlagValue(seed)))
		case strings.HasPrefix(flag, "-ldflags="):
			ldflagsSeen = true
			value := strings.TrimPrefix(flag, "-ldflags=")
			args = append(args, "-ldflags="+mergeLDFlags(value, buildIDFlagValue(seed)))
		default:
			args = append(args, flag)
		}
	}

	if !ldflagsSeen {
		args = append(args, "-ldflags", buildIDFlagValue(seed))
	}

	return append(args, "-buildvcs=false")
}

func mergeLDFlags(existing string, buildID string) string {
	fields := strings.Fields(existing)
	merged := make([]string, 0, len(fields)+1)

	for i := 0; i < len(fields); i++ {
		switch {
		case fields[i] == "-buildid":
			if i+1 < len(fields) {
				i++
			}
			continue
		case strings.HasPrefix(fields[i], "-buildid="):
			continue
		default:
			merged = append(merged, fields[i])
		}
	}

	merged = append(merged, buildID)
	return strings.TrimSpace(strings.Join(merged, " "))
}

func buildIDFlagValue(seed int64) string {
	if seed == 0 {
		return fmt.Sprintf("-buildid=pop-go-rand-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("-buildid=pop-go-seed-%d", seed)
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
