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

package command

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/masteryyh/agenty/pkg/cli/theme"
	"github.com/masteryyh/agenty/pkg/utils/termwrap"
)

var (
	styleBold    = theme.Bold
	styleGray    = theme.Muted
	styleCyan    = theme.Cyan
	styleGreen   = theme.Green
	styleRed     = theme.Red
	styleYellow  = theme.Yellow
	styleMagenta = theme.MagentaS
	styleWhite   = theme.White
	styleSep     = theme.Sep
	styleSysMsg  = theme.SysMsg
	styleSysErr  = theme.SysErr
)

const maxToolResultPreview = 60

func RefreshStyles() {
	styleBold = theme.Bold
	styleGray = theme.Muted
	styleCyan = theme.Cyan
	styleGreen = theme.Green
	styleRed = theme.Red
	styleYellow = theme.Yellow
	styleMagenta = theme.MagentaS
	styleWhite = theme.White
	styleSep = theme.Sep
	styleSysMsg = theme.SysMsg
	styleSysErr = theme.SysErr
}

func renderSectionHeader(title string) string {
	sep := styleSep.Render(strings.Repeat("─", 56))
	return fmt.Sprintf("\n  %s\n  %s\n\n", styleBold.Foreground(theme.Colors.Text).Render(title), sep)
}

func renderKV(label, value string, labelWidth int) string {
	return fmt.Sprintf("  %s  %s\n", styleGray.Render(fmt.Sprintf("%-*s", labelWidth, label)), value)
}

func renderTableSeparator() string {
	return "  " + styleSep.Render(strings.Repeat("─", 72)) + "\n"
}

func wrapForDisplay(s string) string {
	return termwrap.WrapLines(s, termwrap.Options{Width: 76})
}

func renderCommandHintsToString(localMode bool) string {
	var buf strings.Builder
	sep := styleSep.Render(strings.Repeat("─", 56))

	fmt.Fprintf(&buf, "\n  %s\n  %s\n\n",
		styleBold.Foreground(theme.Colors.Text).Render("commands"),
		sep)
	for _, cmd := range commands {
		if !CommandVisible(cmd, localMode) {
			continue
		}
		usage := styleCyan.Render(fmt.Sprintf("%-24s", cmd.Usage))
		desc := styleGray.Render(cmd.Description)
		fmt.Fprintf(&buf, "  %s  %s\n", usage, desc)
	}
	tabHint := styleSep.Render(
		"press " + theme.ModelInfo.Render("tab") + " after / to autocomplete",
	)
	fmt.Fprintf(&buf, "\n  %s\n", tabHint)
	return buf.String()
}

func renderMatchingCommandHints(input string, localMode bool) string {
	trimmed := strings.TrimSpace(input)
	matches := MatchingCommands(trimmed, localMode)
	if len(matches) == 0 {
		return fmt.Sprintf("  %s  %s\n",
			styleSysErr.Render("✗"),
			styleSysMsg.Render("unknown command: "+trimmed+"  ·  type /help to see available commands"),
		)
	}

	var buf strings.Builder
	fmt.Fprintf(&buf, "  %s  %s\n",
		styleYellow.Render("?"),
		styleSysMsg.Render("unknown command: "+trimmed+"  ·  did you mean:"))
	for _, cmd := range matches {
		usage := styleCyan.Render(fmt.Sprintf("    %-24s", cmd.Usage))
		desc := styleSysMsg.Render(cmd.Description)
		fmt.Fprintf(&buf, "%s %s\n", usage, desc)
	}
	return buf.String()
}

func padCell(s string, width int) string {
	if s == "" {
		return "-"
	}
	value := s
	if lipgloss.Width(value) > width {
		value = lipgloss.NewStyle().MaxWidth(width).Render(value)
	}
	if padding := width - lipgloss.Width(value); padding > 0 {
		return value + strings.Repeat(" ", padding)
	}
	return value
}
