package app

import (
	"context"
	"fmt"
	"gopkg.in/yaml.v3"
	"log/slog"
	"os"
	"os/signal"
	"pop-go/internal/builder"
	"pop-go/internal/models"
	"pop-go/internal/obfuscator"
	"pop-go/pkg/fs"
	pkglog "pop-go/pkg/log"
	"syscall"
)

func Run(cfg *models.Config) error {
	fmt.Println(fmt.Sprintf("%+v", cfg))

	// Global context
	_, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Loading locales from ./locales by "lang" from config
	locale, err := loadLocales(cfg)
	if err != nil {
		return fmt.Errorf(fmt.Sprintf("error loading locales: %v", err))
	}
	cfg.CurLocale = *locale

	// Logger setup
	log, closer, err := pkglog.SetupLogger(cfg)
	if err != nil {
		return fmt.Errorf(fmt.Sprintf("%s: %v", cfg.CurLocale["app.err.setup.logger"], err))
	}
	defer closer()

	// Starting message
	log.Info(fmt.Sprintf("%s %v v%v", cfg.CurLocale["app.info.starting"], cfg.App.Name, cfg.App.Version), slog.String("log_level", cfg.Log.Level))
	log.Debug(cfg.CurLocale["app.debug.message.enabled"])

	// Making temporary directory
	dir, err := makeTempDir(cfg)
	if err != nil {
		return err
	}
	log.Debug(cfg.CurLocale["app.debug.create.temp.dir"], slog.String("path", dir))

	// Deleting temp dir if flag is set
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

	// Copying project to temp dir
	err = fs.CopyDir(cfg.Obfuscator.TargetPath, dir)
	if err != nil {
		return fmt.Errorf(fmt.Sprintf("%s: %s", cfg.CurLocale["app.err.copy.temp.dir"], err))
	}
	cfg.Obfuscator.TargetPath = dir

	// Starting obfuscation
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

	// Start of building
	err = builder.NewBuilder(cfg, log).Build()
	if err != nil {
		stop()
		return fmt.Errorf(cfg.CurLocale["app.err.shutdown"])
	}

	stop()
	log.Info(cfg.CurLocale["app.info.shutdown"])

	return nil
}

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

func makeTempDir(cfg *models.Config) (string, error) {
	var dir string
	var err error
	if cfg.App.TempDir == "" {
		dir, err = os.MkdirTemp("/tmp", "")
	} else {
		dir = cfg.App.TempDir
		err = os.MkdirAll("./"+cfg.App.TempDir, 0700)
	}

	if err != nil {
		return "", fmt.Errorf("%s: %w", cfg.CurLocale["app.err.create.temp.dir"], err)
	}

	return dir, nil
}
