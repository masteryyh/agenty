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

package tui

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/masteryyh/agenty/pkg/cli/theme"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/termwrap"
)

const contentIndent = "    "

var renderWidth = 80

func SetRenderWidth(w int) {
	if w > 8 {
		renderWidth = w
	}
}

func stripCR(s string) string {
	return termwrap.StripCR(s)
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

	styleSpinner   = theme.Spinner
	styleSpinTxt   = theme.SpinnerTxt
	styleClipboard = lipgloss.NewStyle().Foreground(theme.Colors.Highlight).Bold(true)
)

func renderMarkdown(text string) string {
	w := max(renderWidth-len(contentIndent)-1, 20)

	markdownRendererMu.Lock()
	if markdownRendererInst == nil || markdownRendererWidth != w {
		glamourStyle := "dark"
		if !theme.IsDark {
			glamourStyle = "light"
		}
		r, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle(glamourStyle),
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
	return renderWrappedLines(reasoning, wrapOptions{
		Width:  renderWidth - len(contentIndent) - 2,
		Indent: contentIndent,
		Style:  styleReasoning,
	})
}

type wrapOptions = termwrap.Options

func renderWrappedLines(text string, opts wrapOptions) string {
	return termwrap.WrapLines(text, opts)
}

func formatReasoningLabel(duration time.Duration) string {
	if duration <= 0 {
		return "thinking:"
	}
	return fmt.Sprintf("thinking: (%.1fs)", duration.Seconds())
}

func renderReasoningBlock(reasoning string, duration time.Duration, show bool) string {
	reasoning = strings.Trim(reasoning, " \n")
	if reasoning == "" {
		return ""
	}
	reasoning = stripCR(reasoning)
	if !show {
		return contentIndent + styleReasoningLabel.Render(formatReasoningLabel(duration)) + "\n\n"
	}
	var buf strings.Builder
	buf.WriteString(contentIndent)
	buf.WriteString(styleReasoningLabel.Render(formatReasoningLabel(duration)))
	buf.WriteString("\n")
	buf.WriteString(renderReasoningContent(reasoning))
	buf.WriteString("\n")
	return buf.String()
}

func renderContentBlock(content string) string {
	content = stripCR(content)
	rendered := normalizeContentMarkdownANSI(renderMarkdown(content), styleContent)
	return renderWrappedLines(rendered, wrapOptions{
		Width:                    renderWidth - len(contentIndent),
		Indent:                   contentIndent,
		Style:                    styleContent,
		TrimLeadingVisibleSpaces: 2,
	})
}

func normalizeContentMarkdownANSI(text string, baseStyle lipgloss.Style) string {
	baseSeq := styleOpenSequence(baseStyle)
	if !strings.Contains(text, "\x1b[") {
		return text
	}

	var buf strings.Builder
	for len(text) > 0 {
		start := strings.Index(text, "\x1b[")
		if start == -1 {
			buf.WriteString(text)
			break
		}
		buf.WriteString(text[:start])
		text = text[start:]

		end := strings.IndexByte(text, 'm')
		if end == -1 {
			buf.WriteString(text)
			break
		}

		params := text[2:end]
		if replacement, ok := normalizeContentSGR(params, baseSeq); ok {
			buf.WriteString(replacement)
		} else {
			buf.WriteString(text[:end+1])
		}
		text = text[end+1:]
	}
	return buf.String()
}

func styleOpenSequence(style lipgloss.Style) string {
	const marker = "x"
	rendered := style.Render(marker)
	idx := strings.Index(rendered, marker)
	if idx == -1 {
		return ""
	}
	return rendered[:idx]
}

func normalizeContentSGR(params, baseSeq string) (string, bool) {
	if params == "" {
		return "\x1b[0m" + baseSeq, true
	}

	parts := strings.Split(params, ";")
	kept := make([]string, 0, len(parts))
	reset := false
	for i := 0; i < len(parts); i++ {
		code, err := strconv.Atoi(parts[i])
		if err != nil {
			return "", false
		}
		switch {
		case code == 0:
			reset = true
		case isANSIColorCode(code):
			continue
		case code == 38 || code == 48:
			i = skipExtendedColorParams(parts, i)
		default:
			kept = append(kept, parts[i])
		}
	}

	var buf strings.Builder
	if reset {
		buf.WriteString("\x1b[0m")
		buf.WriteString(baseSeq)
	}
	if len(kept) > 0 {
		buf.WriteString("\x1b[")
		buf.WriteString(strings.Join(kept, ";"))
		buf.WriteString("m")
	}
	return buf.String(), true
}

func isANSIColorCode(code int) bool {
	return (code >= 30 && code <= 37) ||
		code == 39 ||
		(code >= 40 && code <= 47) ||
		code == 49 ||
		(code >= 90 && code <= 97) ||
		(code >= 100 && code <= 107)
}

func skipExtendedColorParams(parts []string, i int) int {
	if i+1 >= len(parts) {
		return i
	}
	mode, err := strconv.Atoi(parts[i+1])
	if err != nil {
		return i
	}
	switch mode {
	case 5:
		return min(i+2, len(parts)-1)
	case 2:
		return min(i+4, len(parts)-1)
	default:
		return i + 1
	}
}

func renderUserPlainBlock(content string) string {
	return renderWrappedLines(content, wrapOptions{
		Width:  renderWidth - len(contentIndent) - 2,
		Indent: contentIndent,
		Style:  styleContent,
	})
}

func renderToolExecutionBlock(toolCalls []models.ToolCall, toolResults map[string]*models.ChatMessageDto) string {
	var buf strings.Builder
	buf.WriteString(contentIndent)
	buf.WriteString(styleToolLabel.Render("🔧 tool execution:"))
	buf.WriteString("\n")

	wrapWidth := max(renderWidth-10, 20)

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
		buf.WriteString(renderWrappedLines(summaryLine, wrapOptions{Width: summaryWrapW}))

		if toolResultMsg, ok := toolResults[tc.ID]; ok {
			if toolResultMsg.ToolResult.IsError {
				buf.WriteString(contPrefix)
				buf.WriteString(styleToolError.Render("✗ error"))
				buf.WriteString("\n")
			} else {
				buf.WriteString(contPrefix)
				buf.WriteString(styleToolSuccess.Render("✓ ok"))
				buf.WriteString("\n")
			}

			resultContent := stripCR(toolResultMsg.ToolResult.Content)
			lines, moreCount := renderBuiltinToolResultLines(toolResultMsg.ToolResult.Name, resultContent, maxToolResultLines)
			for _, line := range lines {
				buf.WriteString(renderWrappedLines(line, wrapOptions{
					Width:  wrapWidth,
					Indent: contPrefix,
					Style:  styleToolResult,
				}))
			}
			if moreCount > 0 {
				moreLabel := "more results"
				if isLineBasedTool(tc.Name) {
					moreLabel = "more lines"
				}
				buf.WriteString(contPrefix)
				buf.WriteString(styleGray.Render(fmt.Sprintf("...(%d %s)", moreCount, moreLabel)))
				buf.WriteString("\n")
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
	buf.WriteString(renderReasoningBlock(assistantMsg.ReasoningContent, time.Duration(assistantMsg.ReasoningDurationMillis)*time.Millisecond, showReasoning))

	if assistantMsg.Content != "" {
		buf.WriteString(renderContentBlock(assistantMsg.Content))
		buf.WriteString("\n")
	}

	buf.WriteString(renderToolExecutionBlock(assistantMsg.ToolCalls, toolResults))

	if finalResponse != nil {
		if finalResponse.ReasoningContent != "" {
			buf.WriteString("\n")
			buf.WriteString(renderReasoningBlock(finalResponse.ReasoningContent, time.Duration(finalResponse.ReasoningDurationMillis)*time.Millisecond, showReasoning))
		}
		if finalResponse.Content != "" {
			buf.WriteString("\n")
			buf.WriteString(renderContentBlock(finalResponse.Content))
		}
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
		buf.WriteString(renderReasoningBlock(msg.ReasoningContent, time.Duration(msg.ReasoningDurationMillis)*time.Millisecond, showReasoning))
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

func renderCommandHintsToString(localMode bool) string {
	var buf strings.Builder
	sep := styleSep.Render(strings.Repeat("─", 56))

	bold := styleBold
	fmt.Fprintf(&buf, "\n  %s\n  %s\n\n",
		bold.Foreground(theme.Colors.Text).Render("commands"),
		sep)
	for _, cmd := range commands {
		if !commandVisible(cmd, localMode) {
			continue
		}
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

func renderMatchingCommandHints(input string, localMode bool) string {
	trimmed := strings.TrimSpace(input)
	matches := matchingCommands(trimmed, localMode)
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
	return renderWrappedLines(s, wrapOptions{Width: renderWidth - 4})
}

func refreshRenderStyles() {
	styleSep = theme.Sep
	styleAssistantHeader = theme.AssistantHeader
	styleUserHeader = theme.UserHeader
	styleTimestamp = theme.Timestamp
	styleModelInfo = theme.ModelInfo
	styleContent = theme.Content
	styleReasoning = theme.Reasoning
	styleReasoningLabel = theme.ReasoningLabel
	styleToolLabel = theme.ToolLabel
	styleToolName = theme.ToolName
	styleToolArgs = theme.ToolArgs
	styleToolSuccess = theme.ToolSuccess
	styleToolError = theme.ToolError
	styleToolResult = theme.ToolResult
	styleFinalLabel = theme.FinalLabel
	styleSysMsg = theme.SysMsg
	styleSysOk = theme.SysOK
	styleSysErr = theme.SysErr

	styleBold = theme.Bold
	styleGray = theme.Muted
	styleCyan = theme.Cyan
	styleGreen = theme.Green
	styleRed = theme.Red
	styleYellow = theme.Yellow
	styleMagenta = theme.MagentaS
	styleWhite = theme.White
	styleReverse = theme.Reverse

	styleBarSep = theme.BarSep
	styleBarModel = theme.BarModel
	styleBarThink = theme.BarThink
	styleHintMuted = theme.HintMuted
	styleStreaming = theme.Streaming

	styleSpinner = theme.Spinner
	styleSpinTxt = theme.SpinnerTxt
	styleClipboard = lipgloss.NewStyle().Foreground(theme.Colors.Highlight).Bold(true)

	markdownRendererMu.Lock()
	markdownRendererInst = nil
	markdownRendererMu.Unlock()
}
