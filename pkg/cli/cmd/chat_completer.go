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
	"sort"
	"strings"

	"github.com/chzyer/readline"
	"github.com/pterm/pterm"
)

type Command struct {
	Name         string
	Description  string
	Usage        string
	ArgCompleter func() []string
}

var commands = []Command{
	{Name: "/new", Description: "Start a new chat session", Usage: "/new"},
	{Name: "/status", Description: "Show current session status", Usage: "/status"},
	{Name: "/history", Description: "Browse message history", Usage: "/history"},
	{Name: "/model", Description: "Switch to a different model", Usage: "/model [provider/model]"},
	{
		Name:        "/think",
		Description: "Set thinking mode",
		Usage:       "/think [off|<level>]",
	},
	{Name: "/help", Description: "Show available commands", Usage: "/help"},
	{Name: "/exit", Description: "Quit the chat", Usage: "/exit"},
	{Name: "/agent", Description: "Switch to a different agent", Usage: "/agent [agent-name]"},
}

func SetArgCompleter(cmdName string, completer func() []string) {
	for i := range commands {
		if commands[i].Name == cmdName {
			commands[i].ArgCompleter = completer
			return
		}
	}
}

func NewChatCompleter() readline.AutoCompleter {
	items := make([]readline.PrefixCompleterInterface, 0, len(commands))
	for _, cmd := range commands {
		if cmd.ArgCompleter != nil {
			completer := cmd.ArgCompleter
			dynamic := readline.PcItemDynamic(func(line string) []string {
				vals := completer()
				if len(vals) == 0 {
					return nil
				}
				sorted := append([]string(nil), vals...)
				sort.Strings(sorted)
				result := make([]string, len(sorted))
				for i, v := range sorted {
					if strings.Contains(v, " ") {
						result[i] = `"` + v + `"`
					} else {
						result[i] = v
					}
				}
				return result
			})
			items = append(items, readline.PcItem(cmd.Name, dynamic))
		} else {
			items = append(items, readline.PcItem(cmd.Name))
		}
	}
	return readline.NewPrefixCompleter(items...)
}

type HintPainter struct {
	promptWidth    int
	lastHadHint    bool
	lastPanelLines int
}

func stripANSIWidth(s string) int {
	inEscape := false
	width := 0
	for _, r := range s {
		if r == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEscape = false
			}
			continue
		}
		width++
	}
	return width
}

func NewHintPainter(prompt string) *HintPainter {
	return &HintPainter{promptWidth: stripANSIWidth(prompt)}
}

func (p *HintPainter) ClearInlineHint(inputLen int, w io.Writer) {
	if !p.lastHadHint && p.lastPanelLines == 0 {
		return
	}
	col := p.promptWidth + inputLen + 1
	fmt.Fprintf(w, "\033[1A\033[%dG\033[J\n", col)
	p.lastHadHint = false
	p.lastPanelLines = 0
}

func (p *HintPainter) Paint(line []rune, pos int) []rune {
	input := string(line)
	trimmed := strings.TrimSpace(input)

	if !strings.HasPrefix(trimmed, "/") {
		p.lastHadHint = false
		p.lastPanelLines = 0
		return line
	}

	inlineHint := p.findInlineHint(trimmed)
	panelLines := p.buildPanel(trimmed)

	if inlineHint == "" && len(panelLines) == 0 {
		p.lastHadHint = false
		p.lastPanelLines = 0
		return line
	}

	p.lastHadHint = inlineHint != ""
	p.lastPanelLines = len(panelLines)

	var buf strings.Builder
	buf.WriteString(input)

	if inlineHint != "" {
		buf.WriteString(inlineHint)
	}

	for _, pl := range panelLines {
		buf.WriteString("\n\033[2K")
		buf.WriteString(pl)
	}

	if len(panelLines) > 0 {
		fmt.Fprintf(&buf, "\033[%dA", len(panelLines))
		buf.WriteString("\r")
		moveRight := p.promptWidth + len(line)
		if moveRight > 0 {
			fmt.Fprintf(&buf, "\033[%dC", moveRight)
		}
	}

	return []rune(buf.String())
}

func (p *HintPainter) findInlineHint(input string) string {
	lower := strings.ToLower(input)

	for _, cmd := range commands {
		if cmd.ArgCompleter != nil && strings.HasPrefix(lower, strings.ToLower(cmd.Name)+" ") {
			hint := p.findArgHint(input, cmd.Name, cmd.ArgCompleter)
			if hint == "" {
				return ""
			}
			return "\033[90m" + hint + "\033[0m"
		}
	}

	for _, cmd := range commands {
		if strings.HasPrefix(strings.ToLower(cmd.Name), lower) && strings.ToLower(cmd.Name) != lower {
			suffix := cmd.Name[len(input):]
			return "\033[90m" + suffix + "\033[0m\033[37m  " + cmd.Description + "\033[0m"
		}
	}

	for _, cmd := range commands {
		if strings.ToLower(cmd.Name) == lower {
			return "\033[37m  " + cmd.Description + "\033[0m"
		}
	}

	return ""
}

func (p *HintPainter) findArgHint(input, cmdName string, completer func() []string) string {
	rawPrefix := strings.TrimSpace(input[len(cmdName):])
	items := completer()

	quoteChar := rune(0)
	prefix := rawPrefix
	if len(rawPrefix) > 0 && (rawPrefix[0] == '"' || rawPrefix[0] == '\'') {
		quoteChar = rune(rawPrefix[0])
		prefix = rawPrefix[1:]
	}

	for _, item := range items {
		itemHasSpace := strings.Contains(item, " ")
		if prefix == "" && quoteChar == 0 {
			if itemHasSpace {
				return `"` + item + `"`
			}
			return item
		}
		if strings.HasPrefix(strings.ToLower(item), strings.ToLower(prefix)) && !strings.EqualFold(item, prefix) {
			suffix := item[len(prefix):]
			if quoteChar != 0 {
				return suffix + string(quoteChar)
			}
			if itemHasSpace {
				return ""
			}
			return suffix
		}
	}

	return ""
}

func (p *HintPainter) buildPanel(input string) []string {
	lower := strings.ToLower(input)

	for _, cmd := range commands {
		if cmd.ArgCompleter != nil && strings.HasPrefix(lower, strings.ToLower(cmd.Name)+" ") {
			return p.buildArgPanel(input, cmd.Name, cmd.ArgCompleter)
		}
	}

	var lines []string
	for _, cmd := range commands {
		if strings.HasPrefix(strings.ToLower(cmd.Name), lower) {
			lines = append(lines, fmt.Sprintf("  \033[36m%-14s\033[0m \033[90m%s\033[0m", cmd.Name, cmd.Description))
		}
	}

	return lines
}

func (p *HintPainter) buildArgPanel(input, cmdName string, completer func() []string) []string {
	rawPrefix := strings.TrimSpace(input[len(cmdName):])
	items := completer()

	matchPrefix := rawPrefix
	if len(matchPrefix) > 0 && (matchPrefix[0] == '"' || matchPrefix[0] == '\'') {
		matchPrefix = matchPrefix[1:]
	}

	var lines []string
	for _, item := range items {
		if matchPrefix == "" || strings.HasPrefix(strings.ToLower(item), strings.ToLower(matchPrefix)) {
			display := item
			if strings.Contains(item, " ") {
				display = `"` + item + `"`
			}
			lines = append(lines, fmt.Sprintf("  \033[36m%s\033[0m", display))
			if len(lines) >= 8 {
				break
			}
		}
	}

	return lines
}

func matchingCommands(input string) []Command {
	trimmed := strings.ToLower(strings.TrimSpace(input))
	if trimmed == "" || !strings.HasPrefix(trimmed, "/") {
		return nil
	}

	matches := make([]Command, 0, len(commands))
	for _, cmd := range commands {
		if strings.HasPrefix(strings.ToLower(cmd.Name), trimmed) {
			matches = append(matches, cmd)
		}
	}

	return matches
}

func PrintMatchingCommandHints(input string) {
	trimmed := strings.TrimSpace(input)
	matches := matchingCommands(trimmed)
	if len(matches) == 0 {
		pterm.Warning.Printf("Unknown command: %s\n", trimmed)
		pterm.Info.Println("Type /help to see available commands")
		return
	}

	pterm.Warning.Printf("Unknown command: %s\n", trimmed)
	pterm.Info.Println("Did you mean:")
	for _, cmd := range matches {
		pterm.Printf("  %s  %s\n",
			pterm.FgCyan.Sprint(fmt.Sprintf("%-24s", cmd.Usage)),
			pterm.FgLightWhite.Sprint(cmd.Description))
	}
	pterm.Println()
}

func PrintCommandHints() {
	pterm.Info.Println("Commands")
	pterm.Println()
	for _, cmd := range commands {
		pterm.Printf("  %s  %s\n",
			pterm.FgCyan.Sprint(fmt.Sprintf("%-24s", cmd.Usage)),
			pterm.FgLightWhite.Sprint(cmd.Description))
	}
	pterm.Println()
	pterm.DefaultBasicText.Printf("Type %s then press %s to see completions\n",
		pterm.FgYellow.Sprint("/"),
		pterm.FgYellow.Sprint("TAB"))
}
