// Package app wires together all obfuscation stages and drives a single
// end-to-end obfuscation run: loading configuration, copying the target
// project to a temporary directory, running pre- and post-obfuscation tests,
// applying each transformation pass, and finally compiling the result.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"pop-go/internal/builder"
	"pop-go/internal/models"
	"pop-go/internal/obfuscator"
	"pop-go/internal/tester"
	"pop-go/pkg/fs"
	pkglog "pop-go/pkg/log"
	"syscall"

	"gopkg.in/yaml.v3"
)

// Run executes the full obfuscation pipeline for the given configuration.
// It loads locale strings, initialises the logger, creates a working temp
// directory, optionally runs pre- and post-obfuscation tests, applies all
// enabled obfuscation passes, and builds the final binary.
func Run(cfg *models.Config) error {
	_, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	locale, err := loadLocales(cfg)
	if err != nil {
		return fmt.Errorf(fmt.Sprintf("error loading locales: %v", err))
	}
	cfg.CurLocale = *locale

	log, closer, err := pkglog.SetupLogger(cfg)
	if err != nil {
		return fmt.Errorf(fmt.Sprintf("%s: %v", cfg.CurLocale["app.err.setup.logger"], err))
	}
	defer closer()

	log.Info(fmt.Sprintf("%s %v v%v", cfg.CurLocale["app.info.starting"], cfg.App.Name, cfg.App.Version), slog.String("log_level", cfg.Log.Level))
	log.Debug(cfg.CurLocale["app.debug.message.enabled"])

	dir, err := makeTempDir(cfg)
	if err != nil {
		return err
	}
	log.Debug(cfg.CurLocale["app.debug.create.temp.dir"], slog.String("path", dir))

	if cfg.App.RemoveTemp {
		defer func(path string) {
			err = os.RemoveAll(path)
			if err != nil {
				log.Debug(cfg.CurLocale["app.err.delete.temp.dir"], slog.String("path", path))
			} else {
				log.Debug(cfg.CurLocale["app.debug.delete.temp.dir"], slog.String("path", path))
			}
		}(dir)
	}

	// Run tests against the original source before any transformation so that
	// a pre-existing test failure is reported clearly as a user error.
	if cfg.App.Test {
		t := tester.NewTester(cfg, log)
		if err = t.RunTests(cfg.Obfuscator.TargetPath, cfg.CurLocale["tst.stage.pre"]); err != nil {
			stop()
			return fmt.Errorf(cfg.CurLocale["app.err.shutdown"])
		}
	}

	err = fs.CopyDir(cfg.Obfuscator.TargetPath, dir)
	if err != nil {
		return fmt.Errorf(fmt.Sprintf("%s: %s", cfg.CurLocale["app.err.copy.temp.dir"], err))
	}
	cfg.Obfuscator.TargetPath = dir

	obf := obfuscator.NewObfuscator(cfg, log)
	if obf.CritErr != nil {
		stop()
		log.Error(obf.CritErr.Error())
		return fmt.Errorf(cfg.CurLocale["app.err.shutdown"])
	}
	err = obf.Obfuscate()
	if err != nil {
		log.Error(err.Error())
		return fmt.Errorf(cfg.CurLocale["app.err.shutdown"])
	}

	// Post-obfuscation tests run with -vet=off because obfuscation legitimately
	// produces non-constant format strings and other patterns that vet rejects.
	if cfg.App.Test {
		t := tester.NewTester(cfg, log)
		if err = t.RunTests(cfg.Obfuscator.TargetPath, cfg.CurLocale["tst.stage.post"], "-vet=off"); err != nil {
			stop()
			return fmt.Errorf(cfg.CurLocale["app.err.shutdown"])
		}
	}

	err = builder.NewBuilder(cfg, log).Build()
	if err != nil {
		stop()
		return fmt.Errorf(cfg.CurLocale["app.err.shutdown"])
	}

	stop()
	log.Info(cfg.CurLocale["app.info.shutdown"])

	return nil
}

// loadLocales reads the YAML locale file matching cfg.App.Lang from the
// ./locales directory and returns the resulting string map.
func loadLocales(cfg *models.Config) (*map[string]string, error) {
	locale := make(map[string]string)
	data, err := os.ReadFile(fmt.Sprintf("locales/%s.yaml", cfg.App.Lang))
	if err != nil {
		return nil, err
	}

	if err = yaml.Unmarshal(data, &locale); err != nil {
		return nil, err
	}

	return &locale, nil
}

// makeTempDir creates and returns a working directory for the obfuscation pass.
// If cfg.App.TempDir is empty a unique directory is created under the OS temp
// path; otherwise the specified path is created with os.MkdirAll.
func makeTempDir(cfg *models.Config) (string, error) {
	var dir string
	var err error
	if cfg.App.TempDir == "" {
		dir, err = os.MkdirTemp(os.TempDir(), "")
	} else {
		dir = cfg.App.TempDir
		err = os.MkdirAll("./"+cfg.App.TempDir, 0700)
	}

	if err != nil {
		return "", fmt.Errorf("%s: %w", cfg.CurLocale["app.err.create.temp.dir"], err)
	}

	return dir, nil
}
