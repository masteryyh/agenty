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
	"fmt"
	"log/slog"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/masteryyh/agenty/pkg/cli/theme"
)

type tuiHandler struct {
	level slog.Level
	attrs []slog.Attr
	group string
}

func newTUIHandler(level slog.Level) *tuiHandler {
	return &tuiHandler{level: level}
}

func (h *tuiHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *tuiHandler) Handle(_ context.Context, r slog.Record) error {
	entryStore.add(formatRecordForTUI(r, h.attrs))
	return nil
}

func (h *tuiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	combined := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(combined, h.attrs)
	copy(combined[len(h.attrs):], attrs)
	return &tuiHandler{level: h.level, attrs: combined, group: h.group}
}

func (h *tuiHandler) WithGroup(name string) slog.Handler {
	return &tuiHandler{level: h.level, attrs: h.attrs, group: name}
}

func formatRecordForTUI(r slog.Record, extraAttrs []slog.Attr) string {
	var icon string
	var iconStyle lipgloss.Style
	switch {
	case r.Level >= slog.LevelError:
		icon = "✗"
		iconStyle = theme.Red
	case r.Level >= slog.LevelWarn:
		icon = "⚠"
		iconStyle = theme.Yellow
	default:
		icon = "ℹ"
		iconStyle = theme.Blue
	}

	levelStr := theme.Muted.Render(strings.ToLower(r.Level.String()))
	ts := theme.Muted.Render(r.Time.Format("15:04:05"))
	sep := theme.Muted.Render(" · ")
	msg := theme.White.Render(r.Message)

	var sb strings.Builder
	fmt.Fprintf(&sb, "  %s  %s%s%s%s%s",
		iconStyle.Render(icon),
		levelStr, sep, ts, sep, msg)

	var attrParts []string
	for _, a := range extraAttrs {
		attrParts = append(attrParts, theme.Dim.Render(fmt.Sprintf("%s=%v", a.Key, a.Value)))
	}
	r.Attrs(func(a slog.Attr) bool {
		attrParts = append(attrParts, theme.Dim.Render(fmt.Sprintf("%s=%v", a.Key, a.Value)))
		return true
	})
	if len(attrParts) > 0 {
		sb.WriteString("  " + strings.Join(attrParts, " "))
	}
	sb.WriteString("\n\n")
	return sb.String()
}
