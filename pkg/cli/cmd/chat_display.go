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
	"bytes"
	"fmt"
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

func printToolCallingSequence(assistantMsg *models.ChatMessageDto, toolResults map[string]*models.ChatMessageDto, finalResponse *models.ChatMessageDto) {
	modelInfo := ""
	if assistantMsg.Model != nil {
		modelInfo = fmt.Sprintf(" (%s)", assistantMsg.Model.Name)
	}

	pterm.Println(pterm.FgGreen.Sprintf("🤖 Assistant%s [%s]:", modelInfo, assistantMsg.CreatedAt.Format("15:04:05")))

	if assistantMsg.ProviderSpecifics != nil && assistantMsg.ProviderSpecifics.KimiReasoningContent != "" {
		pterm.Println(pterm.FgLightBlue.Sprint("  💭 Reasoning:"))
		renderedReasoning := renderMarkdown(assistantMsg.ProviderSpecifics.KimiReasoningContent)
		for line := range strings.SplitSeq(renderedReasoning, "\n") {
			pterm.Println(pterm.FgLightWhite.Sprint("  " + line))
		}
		fmt.Println()
	}

	if assistantMsg.Content != "" {
		renderedContent := renderMarkdown(assistantMsg.Content)
		for line := range strings.SplitSeq(renderedContent, "\n") {
			pterm.Println("  " + line)
		}
		fmt.Println()
	}

	pterm.Println(pterm.FgYellow.Sprint("  🔧 Tool Execution:"))

	for tcIdx, tc := range assistantMsg.ToolCalls {
		isLast := tcIdx == len(assistantMsg.ToolCalls)-1
		prefix := "  ├─"
		if isLast {
			prefix = "  └─"
		}

		pterm.Printf("%s %s ", pterm.FgYellow.Sprint(prefix), pterm.FgCyan.Sprint(tc.Name))

		var args map[string]any
		if err := json.Unmarshal([]byte(tc.Arguments), &args); err == nil {
			argsStr, _ := json.Marshal(args)
			argDisplay := string(argsStr)
			if len(argDisplay) > 80 {
				argDisplay = argDisplay[:77] + "..."
			}
			pterm.Println(pterm.FgGray.Sprint(argDisplay))
		} else {
			pterm.Println()
		}

		if toolResultMsg, ok := toolResults[tc.ID]; ok {
			resultPrefix := "    "
			if !isLast {
				resultPrefix = "  │ "
			}

			if toolResultMsg.ToolResult.IsError {
				pterm.Printf("%s%s\n", resultPrefix, pterm.FgRed.Sprint("❌ Error"))
				contentPreview := toolResultMsg.ToolResult.Content
				if len(contentPreview) > 100 {
					contentPreview = contentPreview[:97] + "..."
				}
				pterm.Printf("%s%s\n", resultPrefix, pterm.FgGray.Sprint(contentPreview))
			} else {
				pterm.Printf("%s%s\n", resultPrefix, pterm.FgGreen.Sprint("✅ Success"))
				contentPreview := toolResultMsg.ToolResult.Content
				if len(contentPreview) > 100 {
					contentPreview = contentPreview[:97] + "..."
				}
				pterm.Printf("%s%s\n", resultPrefix, pterm.FgGray.Sprint(contentPreview))
			}
		}
	}

	if finalResponse != nil && finalResponse.Content != "" {
		fmt.Println()
		pterm.Println(pterm.FgGreen.Sprint("  📝 Final Response:"))
		renderedContent := renderMarkdown(finalResponse.Content)
		for line := range strings.SplitSeq(renderedContent, "\n") {
			pterm.Println("  " + line)
		}
	}

	fmt.Println()
}

func printMessage(msg *models.ChatMessageDto) {
	switch msg.Role {
	case models.RoleUser:
		pterm.Println(pterm.FgCyan.Sprintf("👤 User [%s]:", msg.CreatedAt.Format("15:04:05")))
		rendered := renderMarkdown(msg.Content)
		for line := range strings.SplitSeq(rendered, "\n") {
			pterm.Println("  " + line)
		}

	case models.RoleAssistant:
		modelInfo := ""
		if msg.Model != nil {
			modelInfo = fmt.Sprintf(" (%s)", msg.Model.Name)
		}
		pterm.Println(pterm.FgGreen.Sprintf("🤖 Assistant%s [%s]:", modelInfo, msg.CreatedAt.Format("15:04:05")))

		if msg.ProviderSpecifics != nil && msg.ProviderSpecifics.KimiReasoningContent != "" {
			pterm.Println(pterm.FgLightBlue.Sprint("  💭 Reasoning:"))
			rendered := renderMarkdown(msg.ProviderSpecifics.KimiReasoningContent)
			for line := range strings.SplitSeq(rendered, "\n") {
				pterm.Println(pterm.FgLightWhite.Sprint("  " + line))
			}
			fmt.Println()
		}

		if msg.Content != "" {
			rendered := renderMarkdown(msg.Content)
			for line := range strings.SplitSeq(rendered, "\n") {
				pterm.Println("  " + line)
			}
		}

		if len(msg.ToolCalls) > 0 {
			pterm.Println(pterm.FgYellow.Sprint("  🔧 Tool Calls:"))
			for _, tc := range msg.ToolCalls {
				pterm.Printf("    • %s", pterm.FgYellow.Sprint(tc.Name))

				var args map[string]any
				if err := json.Unmarshal([]byte(tc.Arguments), &args); err == nil {
					argsStr, _ := json.MarshalIndent(args, "      ", "  ")
					pterm.Println(pterm.FgGray.Sprintf("\n      %s", string(argsStr)))
				} else {
					pterm.Println()
				}
			}
		}

	case models.RoleTool:
		pterm.Println(pterm.FgMagenta.Sprintf("🛠️  Tool Result [%s]:", msg.CreatedAt.Format("15:04:05")))
		if msg.ToolResult != nil {
			if msg.ToolResult.IsError {
				pterm.Println(pterm.FgRed.Sprintf("  ❌ %s (Error)", msg.ToolResult.Name))
			} else {
				pterm.Println(pterm.FgGreen.Sprintf("  ✅ %s", msg.ToolResult.Name))
			}
			contentPreview := msg.ToolResult.Content
			if len(contentPreview) > 200 {
				contentPreview = contentPreview[:200] + "..."
			}
			pterm.Println(pterm.FgGray.Sprint("  " + contentPreview))
		}
	}

	fmt.Println()
}

func openHistoryViewer(messages []models.ChatMessageDto) error {
	var buf bytes.Buffer
	buf.WriteString(pterm.FgCyan.Sprint("=== Chat History ===") + "\n\n")

	for i, msg := range messages {
		buf.WriteString(pterm.FgGray.Sprintf("--- Message %d/%d ---\n", i+1, len(messages)))

		switch msg.Role {
		case models.RoleUser:
			buf.WriteString(pterm.FgCyan.Sprintf("👤 User [%s]:\n", msg.CreatedAt.Format("15:04:05")))
			renderedContent := renderMarkdown(msg.Content)
			for line := range strings.SplitSeq(renderedContent, "\n") {
				buf.WriteString("  " + line + "\n")
			}
			buf.WriteString("\n")

		case models.RoleAssistant:
			modelInfo := ""
			if msg.Model != nil {
				modelInfo = fmt.Sprintf(" (%s)", msg.Model.Name)
			}
			buf.WriteString(pterm.FgGreen.Sprintf("🤖 Assistant%s [%s]:\n", modelInfo, msg.CreatedAt.Format("15:04:05")))

			if msg.ProviderSpecifics != nil && msg.ProviderSpecifics.KimiReasoningContent != "" {
				buf.WriteString(pterm.FgLightBlue.Sprint("  💭 Reasoning:\n"))
				renderedReasoning := renderMarkdown(msg.ProviderSpecifics.KimiReasoningContent)
				for line := range strings.SplitSeq(renderedReasoning, "\n") {
					buf.WriteString(pterm.FgLightWhite.Sprint("  " + line + "\n"))
				}
				buf.WriteString("\n")
			}

			if msg.Content != "" {
				renderedContent := renderMarkdown(msg.Content)
				for line := range strings.SplitSeq(renderedContent, "\n") {
					buf.WriteString("  " + line + "\n")
				}
			}

			if len(msg.ToolCalls) > 0 {
				buf.WriteString("\n" + pterm.FgYellow.Sprint("  🔧 Tool Calls:\n"))
				for _, tc := range msg.ToolCalls {
					buf.WriteString(pterm.FgYellow.Sprintf("    • %s\n", tc.Name))
					var args map[string]any
					if err := json.Unmarshal([]byte(tc.Arguments), &args); err == nil {
						argsStr, _ := json.MarshalIndent(args, "      ", "  ")
						buf.WriteString(pterm.FgGray.Sprintf("      %s\n", string(argsStr)))
					}
				}
			}
			buf.WriteString("\n")

		case models.RoleTool:
			buf.WriteString(pterm.FgMagenta.Sprintf("🛠️  Tool Result [%s]:\n", msg.CreatedAt.Format("15:04:05")))
			if msg.ToolResult != nil {
				if msg.ToolResult.IsError {
					buf.WriteString(pterm.FgRed.Sprintf("  ❌ %s (Error)\n", msg.ToolResult.Name))
				} else {
					buf.WriteString(pterm.FgGreen.Sprintf("  ✅ %s\n", msg.ToolResult.Name))
				}
				for line := range strings.SplitSeq(msg.ToolResult.Content, "\n") {
					buf.WriteString(pterm.FgGray.Sprint("  " + line + "\n"))
				}
				buf.WriteString("\n")
			}
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
