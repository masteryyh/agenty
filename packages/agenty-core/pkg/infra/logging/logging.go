package logging

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/masteryyh/agenty-core/pkg/infra/config"
)

type outputFormat uint8

const (
	formatText outputFormat = iota + 1
	formatJSONL
)

type settings struct {
	level  slog.Level
	format outputFormat
	path   string
}

type FileLogger struct {
	*slog.Logger
	file *os.File
	path string
}

func Open() (*FileLogger, error) {
	mgr, err := config.Init()
	if err != nil {
		return nil, fmt.Errorf("logging: load configuration: %w", err)
	}

	settings, err := loadSettings(mgr.Paths().DataDir, mgr.Config().Logging, time.Now())
	if err != nil {
		return nil, err
	}
	return open(settings)
}

func (l *FileLogger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}

	syncErr := l.file.Sync()
	closeErr := l.file.Close()
	return errors.Join(syncErr, closeErr)
}

func loadSettings(dataDir string, logging config.LoggingConfig, now time.Time) (settings, error) {
	level, err := parseLevel(logging.Level)
	if err != nil {
		return settings{}, err
	}
	format, err := parseFormat(logging.Format)
	if err != nil {
		return settings{}, err
	}

	filename := "core.log"
	if format == formatJSONL {
		filename = "core.jsonl"
	}

	return settings{
		level:  level,
		format: format,
		path: filepath.Join(
			dataDir,
			"logs",
			now.Format("2006"),
			now.Format("01"),
			now.Format("02"),
			filename,
		),
	}, nil
}

func parseLevel(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf(
			"logging: invalid %s %q: expected debug, info, warn, or error",
			config.EnvLogLevel,
			value,
		)
	}
}

func parseFormat(value string) (outputFormat, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "text":
		return formatText, nil
	case "jsonl":
		return formatJSONL, nil
	default:
		return 0, fmt.Errorf(
			"logging: invalid %s %q: expected text or jsonl",
			config.EnvLogFormat,
			value,
		)
	}
}

func open(settings settings) (*FileLogger, error) {
	dir := filepath.Dir(settings.path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("logging: create log directory %q: %w", dir, err)
	}

	file, err := os.OpenFile(settings.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return nil, fmt.Errorf("logging: open log file %q: %w", settings.path, err)
	}

	handlerOptions := &slog.HandlerOptions{Level: settings.level}
	var handler slog.Handler
	switch settings.format {
	case formatText:
		handler = slog.NewTextHandler(file, handlerOptions)
	case formatJSONL:
		handler = slog.NewJSONHandler(file, handlerOptions)
	default:
		if closeErr := file.Close(); closeErr != nil {
			return nil, errors.Join(errors.New("logging: unsupported output format"), closeErr)
		}
		return nil, errors.New("logging: unsupported output format")
	}

	return &FileLogger{
		Logger: slog.New(handler),
		file:   file,
		path:   settings.path,
	}, nil
}
