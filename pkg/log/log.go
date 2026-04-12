// Package log configures a structured [slog.Logger] from the application
// settings and returns a closer that flushes and releases any open file sink.
package log

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"pop-go/internal/models"
	"strings"
)

// SetupLogger creates and returns a [slog.Logger] whose verbosity and output
// format are controlled by cfg. When file logging is enabled, output is written
// to both stdout and the configured log file. The returned closer must be called
// on shutdown to sync and close the file.
func SetupLogger(cfg *models.Config) (*slog.Logger, func() error, error) {
	level := getLogLevel(strings.TrimSpace(cfg.Log.Level))

	if level == nil {
		return nil, nil, fmt.Errorf("%s: %v", cfg.CurLocale["log.err.invalid.level"], cfg.Log.Level)
	}

	var multiWriter io.Writer
	var closer func() error

	if cfg.Log.EnableFile {
		file, err := os.OpenFile(cfg.Log.File, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", cfg.CurLocale["log.err.open.file"], err)
		}
		closer = func() error {
			if err := file.Sync(); err != nil {
				return err
			}
			return file.Close()
		}
		multiWriter = io.MultiWriter(os.Stdout, file)
	} else {
		multiWriter = io.MultiWriter(os.Stdout)
		closer = func() error {
			return nil
		}
	}

	var handler slog.Handler

	switch strings.TrimSpace(cfg.Log.Type) {
	case "text":
		handler = slog.NewTextHandler(multiWriter, &slog.HandlerOptions{Level: *level})
	case "json":
		handler = slog.NewJSONHandler(multiWriter, &slog.HandlerOptions{Level: *level})
	default:
		return nil, closer, fmt.Errorf("%s: %v", cfg.CurLocale["log.err.invalid.type"], cfg.Log.Type)
	}

	return slog.New(handler), closer, nil
}

// getLogLevel maps a lowercase level string to the corresponding [slog.Level].
// Returns nil for unrecognised values so the caller can produce a meaningful error.
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
