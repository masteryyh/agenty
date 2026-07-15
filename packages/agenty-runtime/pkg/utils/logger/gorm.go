/*
Copyright © 2026 masteryyh <yyh991013@163.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package logger

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm/logger"
)

type GormSlogLogger struct {
	level          logger.LogLevel
	slowThreshold  time.Duration
	ignoreNotFound bool
}

func NewGormLogger(debug bool) logger.Interface {
	lvl := logger.Warn
	if debug {
		lvl = logger.Info
	}
	return &GormSlogLogger{
		level:          lvl,
		slowThreshold:  200 * time.Millisecond,
		ignoreNotFound: true,
	}
}

func (l *GormSlogLogger) LogMode(level logger.LogLevel) logger.Interface {
	copy := *l
	copy.level = level
	return &copy
}

func (l *GormSlogLogger) Info(ctx context.Context, msg string, data ...any) {
	if l.level >= logger.Info {
		slog.InfoContext(ctx, fmt.Sprintf(msg, data...), "source", "gorm")
	}
}

func (l *GormSlogLogger) Warn(ctx context.Context, msg string, data ...any) {
	if l.level >= logger.Warn {
		slog.WarnContext(ctx, fmt.Sprintf(msg, data...), "source", "gorm")
	}
}

func (l *GormSlogLogger) Error(ctx context.Context, msg string, data ...any) {
	if l.level >= logger.Error {
		slog.ErrorContext(ctx, fmt.Sprintf(msg, data...), "source", "gorm")
	}
}

func (l *GormSlogLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.level <= logger.Silent {
		return
	}
	elapsed := time.Since(begin)
	sql, rows := fc()
	switch {
	case err != nil && l.level >= logger.Error && !(errors.Is(err, logger.ErrRecordNotFound) && l.ignoreNotFound):
		slog.ErrorContext(ctx, "gorm query error", "error", err, "elapsed", elapsed, "rows", rows, "sql", sql)
	case elapsed > l.slowThreshold && l.slowThreshold > 0 && l.level >= logger.Warn:
		slog.WarnContext(ctx, "gorm slow query", "elapsed", elapsed, "rows", rows, "sql", sql)
	case l.level >= logger.Info:
		slog.DebugContext(ctx, "gorm query", "elapsed", elapsed, "rows", rows, "sql", sql)
	}
}
