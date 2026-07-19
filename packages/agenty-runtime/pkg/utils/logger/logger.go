package logger

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
)

var (
	mu      sync.Mutex
	logFile *os.File
)

type multiHandler struct {
	handlers []slog.Handler
}

func (h *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, r.Level) {
			if err := handler.Handle(ctx, r.Clone()); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (h *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		handlers[i] = handler.WithAttrs(attrs)
	}
	return &multiHandler{handlers: handlers}
}

func (h *multiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		handlers[i] = handler.WithGroup(name)
	}
	return &multiHandler{handlers: handlers}
}

func Init(debug bool, logFilePath string) error {
	mu.Lock()
	defer mu.Unlock()

	if logFilePath == "" {
		logFilePath = "agenty.log"
	}

	f, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0640)
	if err != nil {
		return fmt.Errorf("failed to open log file %q: %w", logFilePath, err)
	}

	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}

	opts := &slog.HandlerOptions{Level: level}

	handlers := []slog.Handler{
		slog.NewJSONHandler(f, opts),
	}
	handlers = append(handlers, slog.NewTextHandler(os.Stdout, opts))

	slog.SetDefault(slog.New(&multiHandler{handlers: handlers}))

	if logFile != nil {
		_ = logFile.Sync()
		_ = logFile.Close()
	}
	logFile = f

	return nil
}

func Close() {
	mu.Lock()
	defer mu.Unlock()

	if logFile != nil {
		_ = logFile.Sync()
		_ = logFile.Close()
		logFile = nil
	}
}
