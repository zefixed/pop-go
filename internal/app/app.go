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
	"strings"
	"syscall"
)

func Run(cfg *models.Config) {
	// Global context
	_, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	locale, err := loadLocales(cfg)
	if err != nil {
		fmt.Println(fmt.Sprintf("error loading locales: %v", err))
		return
	}
	cfg.CurLocale = *locale

	// Logger setup
	log, err := setupLogger(cfg)
	if err != nil {
		fmt.Println(fmt.Sprintf("%s: %v", cfg.CurLocale["err.setup.logger"], err))
		return
	}

	// Starting message
	log.Info(fmt.Sprintf("%s %v v%v", cfg.CurLocale["starting"], cfg.App.Name, cfg.App.Version), slog.String("log_level", cfg.Log.Level))
	log.Debug(cfg.CurLocale["debug.message.enabled"])

	// Making temporary directory
	dir, err := makeTempDir(cfg)
	if err != nil {
		log.Error(err.Error())
		return
	}

	log.Debug(cfg.CurLocale["create.temp.dir"], slog.String("path", dir))
	defer func(path string) {
		os.RemoveAll(path)
		log.Debug(cfg.CurLocale["delete.temp.dir"], slog.String("path", path))
	}(dir)

	// Copying project to temp dir
	err = fs.CopyDir(cfg.Obfuscator.TargetPath, dir)
	if err != nil {
		log.Error(fmt.Sprintf("%s: %s", cfg.CurLocale["err.copy.temp.dir"], err))
		return
	}
	cfg.Obfuscator.TargetPath = dir

	// Starting obfuscation
	obf := obfuscator.NewObfuscator(cfg, log)
	obf.Obfuscate()
	if obf.CritErr != nil {
		stop()
		log.Info(cfg.CurLocale["err.shutdown"])
		return
	}

	// Start of building
	err = builder.NewBuilder(cfg, log).Build()
	if err != nil {
		stop()
		log.Info(cfg.CurLocale["err.shutdown"])
		return
	}

	stop()
	log.Info(cfg.CurLocale["shutdown"])
}

func setupLogger(cfg *models.Config) (*slog.Logger, error) {
	var log *slog.Logger
	var err error

	var handler slog.Handler
	level := getLogLevel(strings.TrimSpace(cfg.Log.Level))

	if level == nil {
		return nil, fmt.Errorf("%s: %v", cfg.CurLocale["invalid.log.level"], cfg.Log.Level)
	}

	switch strings.TrimSpace(cfg.Log.Type) {
	case "text":
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: *level})
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: *level})
	default:
		return nil, fmt.Errorf("%s: %v", cfg.CurLocale["invalid.log.type"], cfg.Log.Type)
	}

	log = slog.New(handler)
	return log, err
}

func getLogLevel(level string) *slog.Level {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		return nil
	}
	return &lvl
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
	if cfg.App.TempFolder == "" {
		dir, err = os.MkdirTemp("/tmp", "")
	} else {
		dir = cfg.App.TempFolder
		err = os.MkdirAll("./"+cfg.App.TempFolder, 0700)
	}

	if err != nil {
		return "", fmt.Errorf("%s: %w", cfg.CurLocale["err.temp.dir"], err)
	}

	return dir, nil
}
