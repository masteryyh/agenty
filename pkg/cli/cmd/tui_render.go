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

package cmd

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/masteryyh/agenty/pkg/cli/theme"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/muesli/reflow/ansi"
	"github.com/muesli/reflow/wordwrap"
	"github.com/muesli/reflow/wrap"
)

const contentIndent = "    "

var renderWidth = 80

func SetRenderWidth(w int) {
	if w > 8 {
		renderWidth = w
	}
}

func stripCR(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "")
}

var (
	markdownRendererMu    sync.Mutex
	markdownRendererWidth int
	markdownRendererInst  *glamour.TermRenderer

	styleSep             = theme.Sep
	styleAssistantHeader = theme.AssistantHeader
	styleUserHeader      = theme.UserHeader
	styleTimestamp       = theme.Timestamp
	styleModelInfo       = theme.ModelInfo
	styleContent         = theme.Content
	styleReasoning       = theme.Reasoning
	styleReasoningLabel  = theme.ReasoningLabel
	styleToolLabel       = theme.ToolLabel
	styleToolName        = theme.ToolName
	styleToolArgs        = theme.ToolArgs
	styleToolSuccess     = theme.ToolSuccess
	styleToolError       = theme.ToolError
	styleToolResult      = theme.ToolResult
	styleFinalLabel      = theme.FinalLabel
	styleSysMsg          = theme.SysMsg
	styleSysOk           = theme.SysOK
	styleSysErr          = theme.SysErr

	styleBold    = theme.Bold
	styleGray    = theme.Muted
	styleCyan    = theme.Cyan
	styleGreen   = theme.Green
	styleRed     = theme.Red
	styleYellow  = theme.Yellow
	styleMagenta = theme.MagentaS
	styleWhite   = theme.White
	styleReverse = theme.Reverse

	styleBarSep    = theme.BarSep
	styleBarModel  = theme.BarModel
	styleBarThink  = theme.BarThink
	styleHintMuted = theme.HintMuted
	styleStreaming = theme.Streaming

	styleSpinner = theme.Spinner
	styleSpinTxt = theme.SpinnerTxt
)

func renderMarkdown(text string) string {
	w := max(renderWidth-len(contentIndent)-1, 20)

	markdownRendererMu.Lock()
	if markdownRendererInst == nil || markdownRendererWidth != w {
		r, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"),
			glamour.WithWordWrap(w),
		)
		markdownRendererWidth = w
		if err == nil {
			markdownRendererInst = r
		}
	}
	r := markdownRendererInst
	markdownRendererMu.Unlock()

	if r == nil {
		return text
	}

	rendered, err := r.Render(text)
	if err != nil {
		return text
	}

	return strings.Trim(rendered, "\n")
}

func renderSectionHeader(title string) string {
	w := max(renderWidth-4, 20)
	return fmt.Sprintf("  %s\n  %s\n\n", styleBold.Render(title), styleGray.Render(strings.Repeat("─", w)))
}

func renderKV(label, value string, labelWidth int) string {
	return fmt.Sprintf("  %-*s %s\n", labelWidth, styleGray.Render(label), value)
}

func renderTableSeparator() string {
	w := max(renderWidth-4, 20)
	return "  " + styleGray.Render(strings.Repeat("─", w)) + "\n"
}

func renderAssistantHeader(modelName string, t time.Time) string {
	ts := styleTimestamp.Render(t.Format("15:04:05"))
	d := styleSep.Render(theme.Dot)
	arrow := styleAssistantHeader.Render("▸")
	label := styleAssistantHeader.Render("ai")
	if modelName != "" {
		model := styleModelInfo.Render(modelName)
		return "  " + arrow + " " + label + d + model + d + ts
	}
	return "  " + arrow + " " + label + d + ts
}

func renderUserHeader(t time.Time) string {
	ts := styleTimestamp.Render(t.Format("15:04:05"))
	d := styleSep.Render(theme.Dot)
	arrow := styleUserHeader.Render("▸")
	label := styleUserHeader.Render("you")
	return "  " + arrow + " " + label + d + ts
}

func renderReasoningContent(reasoning string) string {
	wrapWidth := max(renderWidth-len(contentIndent)-2, 20)
	var buf strings.Builder
	for _, line := range strings.Split(reasoning, "\n") {
		for _, wl := range strings.Split(wordwrap.String(line, wrapWidth), "\n") {
			if ansi.PrintableRuneWidth(wl) > wrapWidth {
				for _, hw := range strings.Split(wrap.String(wl, wrapWidth), "\n") {
					buf.WriteString(contentIndent + styleReasoning.Render(hw))
					buf.WriteString("\n")
				}
			} else {
				buf.WriteString(contentIndent + styleReasoning.Render(wl))
				buf.WriteString("\n")
			}
		}
	}
	return buf.String()
}

func renderReasoningBlock(reasoning string, show bool) string {
	reasoning = strings.Trim(reasoning, " \n")
	if reasoning == "" {
		return ""
	}
	reasoning = stripCR(reasoning)
	if !show {
		return contentIndent + styleReasoningLabel.Render("thinking") + "\n\n"
	}
	var buf strings.Builder
	buf.WriteString(contentIndent + styleReasoningLabel.Render("thinking:"))
	buf.WriteString("\n")
	buf.WriteString(renderReasoningContent(reasoning))
	buf.WriteString("\n")
	return buf.String()
}

func renderContentBlock(content string) string {
	content = stripCR(content)
	rendered := renderMarkdown(content)

	maxW := max(renderWidth-len(contentIndent), 20)

	var buf strings.Builder
	for _, line := range strings.Split(rendered, "\n") {
		stripped := trimLeadingVisibleSpaces(line, 2)
		// Word-level wrap first (preserves English word boundaries, handles spacing).
		for _, wl := range strings.Split(wordwrap.String(stripped, maxW), "\n") {
			if ansi.PrintableRuneWidth(wl) > maxW {
				// Fallback to character-level hard wrap for CJK text, long URLs,
				// or any other token that wordwrap could not break.
				for _, hw := range strings.Split(wrap.String(wl, maxW), "\n") {
					buf.WriteString(contentIndent + hw + "\n")
				}
			} else {
				buf.WriteString(contentIndent + wl + "\n")
			}
		}
	}
	return buf.String()
}

// trimLeadingVisibleSpaces strips up to n leading visible (non-ANSI) spaces from s,
// preserving any ANSI escape sequences encountered before or between those spaces.
func trimLeadingVisibleSpaces(s string, n int) string {
	stripped := 0
	var kept strings.Builder
	i := 0
	for i < len(s) && stripped < n {
		if s[i] == '\033' && i+1 < len(s) && s[i+1] == '[' {
			// CSI sequence: \033[ ... <final byte 0x40-0x7E>
			j := i + 2
			for j < len(s) && !(s[j] >= 0x40 && s[j] <= 0x7e) {
				j++
			}
			if j < len(s) {
				j++ // include final byte
			}
			kept.WriteString(s[i:j])
			i = j
		} else if s[i] == ' ' {
			stripped++
			i++
		} else {
			break
		}
	}
	return kept.String() + s[i:]
}

func renderUserPlainBlock(content string) string {
	content = stripCR(content)
	wrapWidth := max(renderWidth-len(contentIndent)-2, 20)
	var buf strings.Builder
	for _, line := range strings.Split(content, "\n") {
		for _, wl := range strings.Split(wordwrap.String(line, wrapWidth), "\n") {
			if ansi.PrintableRuneWidth(wl) > wrapWidth {
				for _, hw := range strings.Split(wrap.String(wl, wrapWidth), "\n") {
					buf.WriteString(styleContent.Render(contentIndent+hw) + "\n")
				}
			} else {
				buf.WriteString(styleContent.Render(contentIndent+wl) + "\n")
			}
		}
	}
	return buf.String()
}

func renderToolExecutionBlock(toolCalls []models.ToolCall, toolResults map[string]*models.ChatMessageDto) string {
	var buf strings.Builder
	buf.WriteString(contentIndent + styleToolLabel.Render("🔧 tool execution:"))
	buf.WriteString("\n")

	wrapWidth := renderWidth - 10
	if wrapWidth < 20 {
		wrapWidth = 20
	}

	for i, tc := range toolCalls {
		isLast := i == len(toolCalls)-1

		var namePrefix, contPrefix string
		if isLast {
			namePrefix = contentIndent + styleGray.Render("└─") + " "
			contPrefix = contentIndent + "    "
		} else {
			namePrefix = contentIndent + styleGray.Render("├─") + " "
			contPrefix = contentIndent + styleGray.Render("│") + "   "
		}

		summary := renderBuiltinToolCallSummary(tc.Name, tc.Arguments)
		summaryLine := namePrefix + styleToolName.Render(tc.Name) + "  " + summary
		summaryWrapW := max(renderWidth-4, 20)
		for _, wl := range strings.Split(wordwrap.String(summaryLine, summaryWrapW), "\n") {
			buf.WriteString(wl + "\n")
		}

		if toolResultMsg, ok := toolResults[tc.ID]; ok {
			if toolResultMsg.ToolResult.IsError {
				buf.WriteString(contPrefix + styleToolError.Render("✗ error") + "\n")
			} else {
				buf.WriteString(contPrefix + styleToolSuccess.Render("✓ ok") + "\n")
			}

			resultContent := stripCR(toolResultMsg.ToolResult.Content)
			lines, moreCount := renderBuiltinToolResultLines(toolResultMsg.ToolResult.Name, resultContent, maxToolResultLines)
			for _, line := range lines {
				for _, wl := range strings.Split(wordwrap.String(line, wrapWidth), "\n") {
					if ansi.PrintableRuneWidth(wl) > wrapWidth {
						for _, hw := range strings.Split(wrap.String(wl, wrapWidth), "\n") {
							buf.WriteString(contPrefix + styleToolResult.Render(hw) + "\n")
						}
					} else {
						buf.WriteString(contPrefix + styleToolResult.Render(wl) + "\n")
					}
				}
			}
			if moreCount > 0 {
				buf.WriteString(contPrefix + styleGray.Render(fmt.Sprintf("...(%d more results)", moreCount)) + "\n")
			}
		}
	}

	return buf.String()
}

func renderToolCallingSequence(assistantMsg *models.ChatMessageDto, toolResults map[string]*models.ChatMessageDto, finalResponse *models.ChatMessageDto, showReasoning bool) string {
	var buf strings.Builder

	modelName := ""
	if assistantMsg.Model != nil {
		modelName = assistantMsg.Model.Name
	}
	buf.WriteString(renderAssistantHeader(modelName, assistantMsg.CreatedAt))
	buf.WriteString("\n")
	buf.WriteString(renderReasoningBlock(assistantMsg.ReasoningContent, showReasoning))

	if assistantMsg.Content != "" {
		buf.WriteString(renderContentBlock(assistantMsg.Content))
		buf.WriteString("\n")
	}

	buf.WriteString(renderToolExecutionBlock(assistantMsg.ToolCalls, toolResults))

	if finalResponse != nil && finalResponse.Content != "" {
		buf.WriteString("\n")
		buf.WriteString(contentIndent + styleFinalLabel.Render("📝 final response:"))
		buf.WriteString("\n")
		buf.WriteString(renderContentBlock(finalResponse.Content))
	}

	buf.WriteString("\n")
	return buf.String()
}

func renderSingleMessage(msg *models.ChatMessageDto, showReasoning bool) string {
	var buf strings.Builder

	switch msg.Role {
	case models.RoleUser:
		buf.WriteString(renderUserHeader(msg.CreatedAt))
		buf.WriteString("\n")
		buf.WriteString(renderUserPlainBlock(msg.Content))

	case models.RoleAssistant:
		modelName := ""
		if msg.Model != nil {
			modelName = msg.Model.Name
		}
		buf.WriteString(renderAssistantHeader(modelName, msg.CreatedAt))
		buf.WriteString("\n")
		buf.WriteString(renderReasoningBlock(msg.ReasoningContent, showReasoning))
		if msg.Content != "" {
			buf.WriteString(renderContentBlock(msg.Content))
		}

	case models.RoleTool:
		return ""
	}

	buf.WriteString("\n")
	return buf.String()
}

func renderMessageHistoryToString(messages []models.ChatMessageDto, showReasoning bool) string {
	var buf strings.Builder

	for i := 0; i < len(messages); i++ {
		msg := &messages[i]

		if msg.Role == models.RoleAssistant && len(msg.ToolCalls) > 0 {
			toolResults := make(map[string]*models.ChatMessageDto)
			j := i + 1
			for j < len(messages) && messages[j].Role == models.RoleTool {
				toolMsg := &messages[j]
				if toolMsg.ToolResult != nil {
					toolResults[toolMsg.ToolResult.CallID] = toolMsg
				}
				j++
			}

			var finalResponse *models.ChatMessageDto
			if j < len(messages) && messages[j].Role == models.RoleAssistant && len(messages[j].ToolCalls) == 0 {
				finalResponse = &messages[j]
				j++
			}

			buf.WriteString(renderToolCallingSequence(msg, toolResults, finalResponse, showReasoning))
			i = j - 1
		} else {
			buf.WriteString(renderSingleMessage(msg, showReasoning))
		}
	}

	return buf.String()
}

func renderCommandHintsToString() string {
	var buf strings.Builder
	sep := styleSep.Render(strings.Repeat("─", 56))

	bold := styleBold
	fmt.Fprintf(&buf, "\n  %s\n  %s\n\n",
		bold.Foreground(theme.Colors.Text).Render("commands"),
		sep)
	for _, cmd := range commands {
		usage := styleCyan.Render(fmt.Sprintf("%-24s", cmd.Usage))
		desc := styleGray.Render(cmd.Description)
		fmt.Fprintf(&buf, "  %s  %s\n", usage, desc)
	}
	tabHint := styleSep.Render(
		"press " + styleModelInfo.Render("tab") + " after / to autocomplete",
	)
	fmt.Fprintf(&buf, "\n  %s\n", tabHint)
	return buf.String()
}

func renderMatchingCommandHints(input string) string {
	trimmed := strings.TrimSpace(input)
	matches := matchingCommands(trimmed)
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

func renderStatusMessage(icon, text string) string {
	return fmt.Sprintf("  %s  %s\n\n", icon, styleSysMsg.Render(text))
}

func renderErrorMessage(text string) string {
	return fmt.Sprintf("  %s  %s\n\n", styleSysErr.Render("✗"), styleSysErr.Render(text))
}

func WrapForDisplay(s string) string {
	w := max(renderWidth-4, 20)
	var buf strings.Builder
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) == "" {
			buf.WriteString("\n")
			continue
		}
		for _, wl := range strings.Split(wordwrap.String(line, w), "\n") {
			if ansi.PrintableRuneWidth(wl) > w {
				for _, hw := range strings.Split(wrap.String(wl, w), "\n") {
					buf.WriteString(hw + "\n")
				}
			} else {
				buf.WriteString(wl + "\n")
			}
		}
	}
	return buf.String()
}

