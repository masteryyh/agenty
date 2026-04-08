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
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	json "github.com/bytedance/sonic"
	"github.com/charmbracelet/glamour"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/pterm/pterm"
)

func printMessageHistory(messages []models.ChatMessageDto) {
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
			if j < len(messages) && messages[j].Role == models.RoleAssistant {
				finalResponse = &messages[j]
				j++
			}

			printToolCallingSequence(msg, toolResults, finalResponse)
			i = j - 1
		} else {
			printMessage(msg)
		}
	}
}

func writeAssistantHeader(w io.Writer, msg *models.ChatMessageDto) {
	modelInfo := ""
	if msg.Model != nil {
		modelInfo = fmt.Sprintf(" (%s)", msg.Model.Name)
	}
	fmt.Fprintln(w, pterm.FgGreen.Sprintf("🤖 Assistant%s [%s]:", modelInfo, msg.CreatedAt.Format("15:04:05")))
}

func writeReasoningBlock(w io.Writer, reasoning string) {
	reasoning = strings.Trim(reasoning, " \n")
	if reasoning == "" {
		return
	}
	fmt.Fprintln(w, pterm.FgLightBlue.Sprint("  💭 Reasoning:"))
	for line := range strings.SplitSeq(reasoning, "\n") {
		fmt.Fprintln(w, pterm.FgGray.Sprint("  "+line))
	}
	fmt.Fprintln(w)
}

func writeRenderedContent(w io.Writer, content string) {
	rendered := renderMarkdown(content)
	for line := range strings.SplitSeq(rendered, "\n") {
		fmt.Fprintln(w, "  "+line)
	}
}

func writeToolCalls(w io.Writer, toolCalls []models.ToolCall) {
	fmt.Fprintln(w, pterm.FgYellow.Sprint("  🔧 Tool Calls:"))
	for _, tc := range toolCalls {
		fmt.Fprintf(w, "    • %s", pterm.FgYellow.Sprint(tc.Name))
		var args map[string]any
		if err := json.Unmarshal([]byte(tc.Arguments), &args); err == nil {
			argsStr, _ := json.MarshalIndent(args, "      ", "  ")
			fmt.Fprintln(w, pterm.FgGray.Sprintf("\n      %s", string(argsStr)))
		} else {
			fmt.Fprintln(w)
		}
	}
}

func writeToolResult(w io.Writer, msg *models.ChatMessageDto) {
	fmt.Fprintln(w, pterm.FgMagenta.Sprintf("🛠️  Tool Result [%s]:", msg.CreatedAt.Format("15:04:05")))
	if msg.ToolResult == nil {
		return
	}
	if msg.ToolResult.IsError {
		fmt.Fprintln(w, pterm.FgRed.Sprintf("  ❌ %s (Error)", msg.ToolResult.Name))
	} else {
		fmt.Fprintln(w, pterm.FgGreen.Sprintf("  ✅ %s", msg.ToolResult.Name))
	}
	contentPreview := msg.ToolResult.Content
	if len(contentPreview) > 200 {
		contentPreview = contentPreview[:200] + "..."
	}
	fmt.Fprintln(w, pterm.FgGray.Sprint("  "+contentPreview))
}

func printToolCallingSequence(assistantMsg *models.ChatMessageDto, toolResults map[string]*models.ChatMessageDto, finalResponse *models.ChatMessageDto) {
	w := os.Stdout

	writeAssistantHeader(w, assistantMsg)
	writeReasoningBlock(w, assistantMsg.ReasoningContent)

	if assistantMsg.Content != "" {
		writeRenderedContent(w, assistantMsg.Content)
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, pterm.FgYellow.Sprint("  🔧 Tool Execution:"))

	for tcIdx, tc := range assistantMsg.ToolCalls {
		isLast := tcIdx == len(assistantMsg.ToolCalls)-1
		prefix := "  ├─"
		if isLast {
			prefix = "  └─"
		}

		fmt.Fprintf(w, "%s %s ", pterm.FgYellow.Sprint(prefix), pterm.FgCyan.Sprint(tc.Name))

		var args map[string]any
		if err := json.Unmarshal([]byte(tc.Arguments), &args); err == nil {
			argsStr, _ := json.Marshal(args)
			argDisplay := string(argsStr)
			if len(argDisplay) > 80 {
				argDisplay = argDisplay[:77] + "..."
			}
			fmt.Fprintln(w, pterm.FgGray.Sprint(argDisplay))
		} else {
			fmt.Fprintln(w)
		}

		if toolResultMsg, ok := toolResults[tc.ID]; ok {
			resultPrefix := "    "
			if !isLast {
				resultPrefix = "  │ "
			}

			if toolResultMsg.ToolResult.IsError {
				fmt.Fprintf(w, "%s%s\n", resultPrefix, pterm.FgRed.Sprint("❌ Error"))
			} else {
				fmt.Fprintf(w, "%s%s\n", resultPrefix, pterm.FgGreen.Sprint("✅ Success"))
			}
			contentPreview := toolResultMsg.ToolResult.Content
			if len(contentPreview) > 100 {
				contentPreview = contentPreview[:97] + "..."
			}
			fmt.Fprintf(w, "%s%s\n", resultPrefix, pterm.FgGray.Sprint(contentPreview))
		}
	}

	if finalResponse != nil && finalResponse.Content != "" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, pterm.FgGreen.Sprint("  📝 Final Response:"))
		writeRenderedContent(w, finalResponse.Content)
	}

	fmt.Fprintln(w)
}

func printMessage(msg *models.ChatMessageDto) {
	w := os.Stdout

	switch msg.Role {
	case models.RoleUser:
		fmt.Fprintln(w, pterm.FgCyan.Sprintf("👤 User [%s]:", msg.CreatedAt.Format("15:04:05")))
		writeRenderedContent(w, msg.Content)

	case models.RoleAssistant:
		writeAssistantHeader(w, msg)
		writeReasoningBlock(w, msg.ReasoningContent)
		if msg.Content != "" {
			writeRenderedContent(w, msg.Content)
		}
		if len(msg.ToolCalls) > 0 {
			writeToolCalls(w, msg.ToolCalls)
		}

	case models.RoleTool:
		writeToolResult(w, msg)
	}

	fmt.Fprintln(w)
}

func openHistoryViewer(messages []models.ChatMessageDto) error {
	var buf strings.Builder
	buf.WriteString(pterm.FgCyan.Sprint("=== Chat History ===") + "\n\n")

	for i, msg := range messages {
		buf.WriteString(pterm.FgGray.Sprintf("--- Message %d/%d ---\n", i+1, len(messages)))

		switch msg.Role {
		case models.RoleUser:
			fmt.Fprintln(&buf, pterm.FgCyan.Sprintf("👤 User [%s]:", msg.CreatedAt.Format("15:04:05")))
			writeRenderedContent(&buf, msg.Content)
			buf.WriteString("\n")

		case models.RoleAssistant:
			writeAssistantHeader(&buf, &msg)
			writeReasoningBlock(&buf, msg.ReasoningContent)
			if msg.Content != "" {
				writeRenderedContent(&buf, msg.Content)
			}
			if len(msg.ToolCalls) > 0 {
				buf.WriteString("\n")
				writeToolCalls(&buf, msg.ToolCalls)
			}
			buf.WriteString("\n")

		case models.RoleTool:
			writeToolResult(&buf, &msg)
			buf.WriteString("\n")
		}
		buf.WriteString("\n")
	}

	if lessPath, err := exec.LookPath("less"); err == nil {
		cmd := exec.Command(lessPath, "-R")
		cmd.Stdin = strings.NewReader(buf.String())
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	if morePath, err := exec.LookPath("more"); err == nil {
		cmd := exec.Command(morePath)
		cmd.Stdin = strings.NewReader(buf.String())
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	return fmt.Errorf("no pager available (less or more)")
}

var (
	markdownRenderer     *glamour.TermRenderer
	markdownRendererOnce sync.Once
)

func printStreamText(text string, color pterm.Color, indent string, atLineStart *bool) {
	if text == "" {
		return
	}
	parts := strings.Split(text, "\n")
	for i, part := range parts {
		if i > 0 {
			fmt.Println()
			*atLineStart = true
		}
		if *atLineStart && part != "" {
			fmt.Print(indent)
			*atLineStart = false
		}
		fmt.Print(color.Sprint(part))
	}
}

func renderMarkdown(text string) string {
	markdownRendererOnce.Do(func() {
		r, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(100),
		)
		if err == nil {
			markdownRenderer = r
		}
	})

	if markdownRenderer == nil {
		return text
	}

	rendered, err := markdownRenderer.Render(text)
	if err != nil {
		return text
	}

	return strings.TrimSpace(rendered)
}
