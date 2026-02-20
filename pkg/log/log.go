package log

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"pop-go/internal/models"
	"strings"
)

func SetupLogger(cfg *models.Config) (*slog.Logger, func() error, error) {
	var log *slog.Logger
	var err error

	var handler slog.Handler
	level := getLogLevel(strings.TrimSpace(cfg.Log.Level))

	if level == nil {
		return nil, nil, fmt.Errorf("%s: %v", cfg.CurLocale["log.err.invalid.level"], cfg.Log.Level)
	}

	file, err := os.OpenFile(cfg.Log.File, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", cfg.CurLocale["log.err.open.file"], err)
	}

	closer := func() error {
		if err := file.Sync(); err != nil {
			return err
		}
		return file.Close()
	}

	multiWriter := io.MultiWriter(os.Stdout, file)

	switch strings.TrimSpace(cfg.Log.Type) {
	case "text":
		handler = slog.NewTextHandler(multiWriter, &slog.HandlerOptions{Level: *level})
	case "json":
		handler = slog.NewJSONHandler(multiWriter, &slog.HandlerOptions{Level: *level})
	default:
		return nil, closer, fmt.Errorf("%s: %v", cfg.CurLocale["log.err.invalid.type"], cfg.Log.Type)
	}

	log = slog.New(handler)
	return log, closer, nil
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
