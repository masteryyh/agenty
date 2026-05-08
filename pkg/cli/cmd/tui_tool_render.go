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
	maxToolArgPreview    = 40
	maxToolResultPreview = 60
	maxToolResultLines   = 5
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

func shortToolCallID(callID string) string {
	if callID == "" {
		return ""
	}
	if len(callID) <= 8 {
		return callID
	}
	return callID[:8]
}

// streamRenderBuiltinToolCallLine renders the complete tool call line for streaming output.
func streamRenderBuiltinToolCallLine(name, argsJSON, callID string) string {
	summary := renderBuiltinToolCallSummary(name, argsJSON)
	line := contentIndent + styleGray.Render("─") + " " + styleToolName.Render(name)
	if id := shortToolCallID(callID); id != "" {
		line += " " + styleGray.Render("["+id+"]")
	}
	if summary != "" {
		line += "  " + summary
	}
	maxW := max(renderWidth, 20)
	return truncate.StringWithTail(line, uint(maxW), "…")
}

func isLineBasedTool(name string) bool {
	switch name {
	case "read_file", "run_shell_command", "list_directory", "write_file", "replace_in_file":
		return true
	}
	return false
}

// renderBuiltinToolResultLines parses tool result content and returns display lines + more count.
// maxLines controls how many lines are returned; returns (lines, moreCount).
func renderBuiltinToolResultLines(name, content string, maxLines int) ([]string, int) {
	content = strings.TrimRight(stripCR(content), "\n")
	if errMsg, ok := parseToolErrorContent(content); ok {
		return renderGenericResultLines(errMsg, maxLines)
	}
	switch name {
	case "search":
		return renderSearchResultLines(content, maxLines)
	case "run_shell_command":
		return renderShellResultLines(content, maxLines)
	case "read_file":
		return renderJSONContentResultLines(content, maxLines)
	case "list_directory":
		return renderListDirectoryResultLines(content, maxLines)
	case "write_file":
		return renderWriteFileResultLines(content, maxLines)
	case "replace_in_file":
		return renderReplaceInFileResultLines(content, maxLines)
	case "todo":
		return renderTodoResultLines(content, maxLines)
	case "find_skill":
		return renderFindSkillResultLines(content, maxLines)
	case "save_memory", "update_soul":
		return renderStatusResultLines(content, maxLines)
	default:
		return renderGenericResultLines(content, maxLines)
	}
}

func parseToolErrorContent(content string) (string, bool) {
	var resp struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(content), &resp); err != nil || resp.Error == "" {
		return "", false
	}
	return resp.Error, true
}

func renderSearchResultLines(content string, maxLines int) ([]string, int) {
	var resp struct {
		KnowledgeBase *struct {
			Results []struct {
				ItemTitle string  `json:"itemTitle,omitempty"`
				Category  string  `json:"category"`
				Content   string  `json:"content"`
				Score     float64 `json:"score"`
			} `json:"results"`
			Quality string `json:"quality"`
		} `json:"knowledgeBase,omitempty"`
		WebSearch *struct {
			Results []struct {
				Title   string `json:"title"`
				URL     string `json:"url"`
				Content string `json:"content"`
			} `json:"results"`
			Quality string `json:"quality"`
		} `json:"webSearch,omitempty"`
		WorkspaceFiles *struct {
			Results []struct {
				RelativePath string `json:"relativePath"`
				Path         string `json:"path"`
				StartLine    int    `json:"startLine"`
			} `json:"results"`
			Quality string `json:"quality"`
		} `json:"workspaceFiles,omitempty"`
		OverallQuality string `json:"overallQuality"`
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

	if resp.WorkspaceFiles != nil {
		totalResults += len(resp.WorkspaceFiles.Results)
		for _, r := range resp.WorkspaceFiles.Results {
			if len(lines) >= maxLines {
				break
			}
			label := r.RelativePath
			if label == "" {
				label = r.Path
			}
			if r.StartLine > 0 {
				label = fmt.Sprintf("%s:%d", label, r.StartLine)
			}
			if len(label) > 55 {
				label = label[:52] + "..."
			}
			lines = append(lines, styleGray.Render("[file] ")+label)
		}
	}

	shown := len(lines)
	more := max(totalResults-shown, 0)
	return lines, more
}

func renderShellResultLines(content string, maxLines int) ([]string, int) {
	var resp struct {
		ExitCode int    `json:"exitCode"`
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
	}
	if err := json.Unmarshal([]byte(content), &resp); err == nil {
		stdoutLines := splitNonEmptyTrailingLines(resp.Stdout)
		stderrLines := splitNonEmptyTrailingLines(resp.Stderr)

		var displayLines []string
		if resp.ExitCode == 0 && len(stdoutLines) > 0 {
			displayLines = stdoutLines
		} else if resp.ExitCode != 0 && len(stderrLines) > 0 {
			displayLines = stderrLines
		} else if resp.ExitCode != 0 && len(stdoutLines) > 0 {
			displayLines = stdoutLines
		} else {
			return []string{styleGray.Render(fmt.Sprintf("command exited with code %d", resp.ExitCode))}, 0
		}

		if len(displayLines) <= maxLines {
			return displayLines, 0
		}
		return displayLines[:maxLines], len(displayLines) - maxLines
	}

	const (
		sectionNone = iota
		sectionStdout
		sectionStderr
	)

	exitCode := 0
	var stdoutLines []string
	var stderrLines []string
	section := sectionNone

	for _, l := range strings.Split(content, "\n") {
		if strings.HasPrefix(l, "Exit Code:") {
			fmt.Sscanf(l, "Exit Code: %d", &exitCode)
			continue
		}
		if l == "Stdout:" {
			section = sectionStdout
			continue
		}
		if l == "Stderr:" {
			section = sectionStderr
			continue
		}
		switch section {
		case sectionStdout:
			stdoutLines = append(stdoutLines, l)
		case sectionStderr:
			stderrLines = append(stderrLines, l)
		}
	}

	trimTrailing := func(ls []string) []string {
		for len(ls) > 0 && strings.TrimSpace(ls[len(ls)-1]) == "" {
			ls = ls[:len(ls)-1]
		}
		return ls
	}
	stdoutLines = trimTrailing(stdoutLines)
	stderrLines = trimTrailing(stderrLines)

	var displayLines []string
	if exitCode == 0 && len(stdoutLines) > 0 {
		displayLines = stdoutLines
	} else if exitCode != 0 && len(stderrLines) > 0 {
		displayLines = stderrLines
	} else if exitCode != 0 && len(stdoutLines) > 0 {
		displayLines = stdoutLines
	} else {
		return []string{styleGray.Render(fmt.Sprintf("command exited with code %d", exitCode))}, 0
	}

	if len(displayLines) <= maxLines {
		return displayLines, 0
	}
	return displayLines[:maxLines], len(displayLines) - maxLines
}

func splitNonEmptyTrailingLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func renderJSONContentResultLines(content string, maxLines int) ([]string, int) {
	var resp struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return renderGenericResultLines(content, maxLines)
	}
	return renderGenericResultLines(resp.Content, maxLines)
}

func renderListDirectoryResultLines(content string, maxLines int) ([]string, int) {
	var resp struct {
		Entries []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"entries"`
	}
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return renderGenericResultLines(content, maxLines)
	}

	lines := make([]string, 0, min(len(resp.Entries), maxLines))
	for _, entry := range resp.Entries {
		if len(lines) >= maxLines {
			break
		}
		lines = append(lines, fmt.Sprintf("[%s] %s", entry.Type, entry.Name))
	}
	return lines, max(len(resp.Entries)-len(lines), 0)
}

func renderWriteFileResultLines(content string, maxLines int) ([]string, int) {
	var resp struct {
		Path         string `json:"path"`
		BytesWritten int    `json:"bytesWritten"`
	}
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return renderGenericResultLines(content, maxLines)
	}
	return renderGenericResultLines(fmt.Sprintf("wrote %d bytes to %s", resp.BytesWritten, resp.Path), maxLines)
}

func renderReplaceInFileResultLines(content string, maxLines int) ([]string, int) {
	var resp struct {
		Path              string `json:"path"`
		StartLine         int    `json:"startLine"`
		EndLine           int    `json:"endLine"`
		BytesWritten      int    `json:"bytesWritten"`
		ReplacedLineCount int    `json:"replacedLineCount"`
	}
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return renderGenericResultLines(content, maxLines)
	}
	return renderGenericResultLines(fmt.Sprintf("replaced lines %d-%d in %s (%d bytes)", resp.StartLine, resp.EndLine, resp.Path, resp.BytesWritten), maxLines)
}

func renderTodoResultLines(content string, maxLines int) ([]string, int) {
	var resp struct {
		Action     string `json:"action"`
		Total      int    `json:"total"`
		Pending    int    `json:"pending"`
		InProgress int    `json:"inProgress"`
		Done       int    `json:"done"`
		Items      []struct {
			ID      int    `json:"id"`
			Content string `json:"content"`
			Status  string `json:"status"`
		} `json:"items"`
		Item *struct {
			ID      int    `json:"id"`
			Content string `json:"content"`
			Status  string `json:"status"`
		} `json:"item"`
	}
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return renderGenericResultLines(content, maxLines)
	}

	var lines []string
	switch resp.Action {
	case "add":
		lines = append(lines, fmt.Sprintf("added %d todo item(s)", len(resp.Items)))
	case "update":
		if resp.Item != nil {
			lines = append(lines, fmt.Sprintf("[%d] %s -> %s", resp.Item.ID, resp.Item.Content, resp.Item.Status))
		}
	case "list":
		lines = append(lines, fmt.Sprintf("%d total, %d pending, %d in_progress, %d done", resp.Total, resp.Pending, resp.InProgress, resp.Done))
	}
	for _, item := range resp.Items {
		if len(lines) >= maxLines {
			break
		}
		lines = append(lines, fmt.Sprintf("[%d] %s %s", item.ID, item.Status, item.Content))
	}
	if len(lines) == 0 {
		return renderGenericResultLines(content, maxLines)
	}
	total := len(resp.Items)
	if resp.Action == "list" {
		total++
	}
	return lines, max(total-len(lines), 0)
}

func renderFindSkillResultLines(content string, maxLines int) ([]string, int) {
	var resp struct {
		Count   int `json:"count"`
		Results []struct {
			Name        string  `json:"name"`
			SkillMDPath string  `json:"skillMdPath"`
			Score       float64 `json:"score,omitempty"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return renderGenericResultLines(content, maxLines)
	}

	lines := make([]string, 0, min(resp.Count, maxLines))
	for _, result := range resp.Results {
		if len(lines) >= maxLines {
			break
		}
		label := result.Name
		if result.SkillMDPath != "" {
			label = fmt.Sprintf("%s  %s", label, result.SkillMDPath)
		}
		lines = append(lines, label)
	}
	if len(lines) == 0 {
		return []string{"no matching skills"}, 0
	}
	return lines, max(resp.Count-len(lines), 0)
}

func renderStatusResultLines(content string, maxLines int) ([]string, int) {
	var resp map[string]any
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return renderGenericResultLines(content, maxLines)
	}
	if errMsg, ok := resp["error"].(string); ok {
		return renderGenericResultLines(errMsg, maxLines)
	}
	if id, ok := resp["id"].(string); ok {
		return renderGenericResultLines("ok: "+id, maxLines)
	}
	return renderGenericResultLines("ok", maxLines)
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
