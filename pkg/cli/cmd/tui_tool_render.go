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

	json "github.com/bytedance/sonic"
	"github.com/muesli/reflow/truncate"
)

const (
	maxToolArgPreview     = 40
	maxToolResultPreview  = 60
	maxToolResultLines    = 5
)

// renderBuiltinToolCallSummary returns a human-readable summary of a tool call's arguments.
// Returns empty string if no meaningful summary can be produced.
func renderBuiltinToolCallSummary(name, argsJSON string) string {
	switch name {
	case "search":
		return renderSearchCallSummary(argsJSON)
	case "run_shell_command":
		return renderShellCallSummary(argsJSON)
	case "read_file":
		return renderReadFileCallSummary(argsJSON)
	case "write_file":
		return renderWriteFileCallSummary(argsJSON)
	case "list_directory":
		return renderListDirCallSummary(argsJSON)
	case "replace_in_file":
		return renderReplaceInFileCallSummary(argsJSON)
	case "save_memory":
		return renderSaveMemoryCallSummary(argsJSON)
	case "todo":
		return renderTodoCallSummary(argsJSON)
	case "update_soul":
		return styleGray.Render("update agent soul")
	default:
		return renderDefaultCallSummary(argsJSON)
	}
}

func renderSearchCallSummary(argsJSON string) string {
	var args struct {
		Searches []struct {
			Channel string `json:"channel"`
			Query   string `json:"query"`
		} `json:"searches"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil || len(args.Searches) == 0 {
		return styleGray.Render("search")
	}
	var parts []string
	for _, s := range args.Searches {
		ch := s.Channel
		switch ch {
		case "knowledge_base":
			ch = "kb"
		case "web_search":
			ch = "web"
		}
		q := s.Query
		if len(q) > maxToolArgPreview {
			q = q[:maxToolArgPreview-3] + "..."
		}
		parts = append(parts, styleGray.Render(ch+": ")+styleToolArgs.Render(`"`+q+`"`))
	}
	return "🔍 " + strings.Join(parts, styleGray.Render(" · "))
}

func renderShellCallSummary(argsJSON string) string {
	var args struct {
		Command string `json:"command"`
		Workdir string `json:"workdir,omitempty"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil || args.Command == "" {
		return styleGray.Render("run command")
	}
	cmd := args.Command
	if len(cmd) > maxToolResultPreview {
		cmd = cmd[:maxToolResultPreview-3] + "..."
	}
	result := styleGray.Render("$ ") + styleContent.Render(cmd)
	if args.Workdir != "" {
		result += styleGray.Render("  in " + args.Workdir)
	}
	return result
}

func truncatePath(p string, maxLen int) string {
	if len(p) <= maxLen {
		return p
	}
	return "..." + p[len(p)-(maxLen-3):]
}

func renderReadFileCallSummary(argsJSON string) string {
	var args struct {
		Path      string `json:"path"`
		StartLine int    `json:"startLine,omitempty"`
		EndLine   int    `json:"endLine,omitempty"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil || args.Path == "" {
		return styleGray.Render("read file")
	}
	result := "📄 " + styleContent.Render(truncatePath(args.Path, 50))
	if args.StartLine > 0 && args.EndLine > 0 {
		result += styleGray.Render(fmt.Sprintf("  lines %d-%d", args.StartLine, args.EndLine))
	} else if args.StartLine > 0 {
		result += styleGray.Render(fmt.Sprintf("  from line %d", args.StartLine))
	} else if args.EndLine > 0 {
		result += styleGray.Render(fmt.Sprintf("  to line %d", args.EndLine))
	}
	return result
}

func renderWriteFileCallSummary(argsJSON string) string {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil || args.Path == "" {
		return styleGray.Render("write file")
	}
	return "📝 " + styleContent.Render(truncatePath(args.Path, 50))
}

func renderListDirCallSummary(argsJSON string) string {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil || args.Path == "" {
		return styleGray.Render("list directory")
	}
	return "📁 " + styleContent.Render(truncatePath(args.Path, 50))
}

func renderReplaceInFileCallSummary(argsJSON string) string {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil || args.Path == "" {
		return styleGray.Render("replace in file")
	}
	return "✏️  " + styleContent.Render(truncatePath(args.Path, 50))
}

func renderSaveMemoryCallSummary(argsJSON string) string {
	var args struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil || args.Content == "" {
		return styleGray.Render("save memory")
	}
	content := args.Content
	if len(content) > 50 {
		content = content[:47] + "..."
	}
	return "🧠 " + styleToolArgs.Render(`"`+content+`"`)
}

func renderTodoCallSummary(argsJSON string) string {
	var args struct {
		Action string   `json:"action"`
		Items  []string `json:"items,omitempty"`
		ID     int      `json:"id,omitempty"`
		Status string   `json:"status,omitempty"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return styleGray.Render("todo")
	}
	switch args.Action {
	case "add":
		return fmt.Sprintf("📋 add %d item(s)", len(args.Items))
	case "update":
		return fmt.Sprintf("📋 update #%d → %s", args.ID, args.Status)
	case "list":
		return "📋 list todos"
	default:
		if args.Action != "" {
			return "📋 " + styleGray.Render(args.Action)
		}
		return styleGray.Render("todo")
	}
}

func renderDefaultCallSummary(argsJSON string) string {
	if argsJSON == "" || argsJSON == "{}" {
		return ""
	}
	display := argsJSON
	if len(display) > maxToolResultPreview {
		display = display[:maxToolResultPreview-3] + "..."
	}
	return styleToolArgs.Render(display)
}

// streamRenderBuiltinToolCallLine renders the complete tool call line for streaming output.
func streamRenderBuiltinToolCallLine(name, argsJSON string) string {
	summary := renderBuiltinToolCallSummary(name, argsJSON)
	line := contentIndent + styleGray.Render("─") + " " + styleToolName.Render(name)
	if summary != "" {
		line += "  " + summary
	}
	maxW := renderWidth
	if maxW < 20 {
		maxW = 20
	}
	return truncate.StringWithTail(line, uint(maxW), "…")
}

// renderBuiltinToolResultLines parses tool result content and returns display lines + more count.
// maxLines controls how many lines are returned; returns (lines, moreCount).
func renderBuiltinToolResultLines(name, content string, maxLines int) ([]string, int) {
	content = strings.TrimRight(stripCR(content), "\n")
	switch name {
	case "search":
		return renderSearchResultLines(content, maxLines)
	case "run_shell_command":
		return renderShellResultLines(content, maxLines)
	default:
		return renderGenericResultLines(content, maxLines)
	}
}

func renderSearchResultLines(content string, maxLines int) ([]string, int) {
	var resp struct {
		KnowledgeBase *struct {
			Results []struct {
				ItemTitle string  `json:"item_title,omitempty"`
				Category  string  `json:"category"`
				Content   string  `json:"content"`
				Score     float64 `json:"score"`
			} `json:"results"`
			Quality string `json:"quality"`
		} `json:"knowledge_base,omitempty"`
		WebSearch *struct {
			Results []struct {
				Title   string `json:"title"`
				URL     string `json:"url"`
				Content string `json:"content"`
			} `json:"results"`
			Quality string `json:"quality"`
		} `json:"web_search,omitempty"`
		OverallQuality string `json:"overall_quality"`
	}

	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return renderGenericResultLines(content, maxLines)
	}

	var lines []string
	totalResults := 0

	if resp.WebSearch != nil {
		totalResults += len(resp.WebSearch.Results)
		for _, r := range resp.WebSearch.Results {
			if len(lines) >= maxLines {
				break
			}
			title := r.Title
			if len(title) > 55 {
				title = title[:52] + "..."
			}
			lines = append(lines, styleGray.Render("[web] ")+title)
			if len(lines) < maxLines && r.URL != "" {
				url := r.URL
				if len(url) > maxToolResultPreview {
					url = url[:maxToolResultPreview-3] + "..."
				}
				lines = append(lines, styleGray.Render("      "+url))
			}
		}
	}

	if resp.KnowledgeBase != nil {
		totalResults += len(resp.KnowledgeBase.Results)
		for _, r := range resp.KnowledgeBase.Results {
			if len(lines) >= maxLines {
				break
			}
			label := r.Category
			if r.ItemTitle != "" {
				label = r.ItemTitle
			}
			if len(label) > 50 {
				label = label[:47] + "..."
			}
			lines = append(lines, styleGray.Render("[kb] ")+label)
		}
	}

	shown := len(lines)
	more := totalResults - shown
	if more < 0 {
		more = 0
	}
	return lines, more
}

func renderShellResultLines(content string, maxLines int) ([]string, int) {
	rawLines := strings.Split(content, "\n")
	var lines []string
	inStdout := false
	stdoutLines := 0
	totalStdoutLines := 0

	for _, l := range rawLines {
		if l == "Stdout:" {
			inStdout = true
			lines = append(lines, styleGray.Render(l))
			continue
		}
		if l == "Stderr:" {
			inStdout = false
		}
		if inStdout {
			totalStdoutLines++
		}
		if len(lines) < maxLines {
			lines = append(lines, l)
			if inStdout {
				stdoutLines++
			}
		}
	}

	more := totalStdoutLines - stdoutLines
	if more < 0 {
		more = 0
	}
	return lines, more
}

func renderGenericResultLines(content string, maxLines int) ([]string, int) {
	lines := strings.Split(content, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) <= maxLines {
		return lines, 0
	}
	return lines[:maxLines], len(lines) - maxLines
}
