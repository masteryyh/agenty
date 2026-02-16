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
	{Name: "/help", Description: "Show available commands", Usage: "/help"},
	{Name: "/exit", Description: "Quit the chat", Usage: "/exit"},
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
				return sorted
			})
			items = append(items, readline.PcItem(cmd.Name, dynamic))
		} else {
			items = append(items, readline.PcItem(cmd.Name))
		}
	}
	return readline.NewPrefixCompleter(items...)
}

type HintPainter struct{}

func NewHintPainter() *HintPainter {
	return &HintPainter{}
}

func (p *HintPainter) Paint(line []rune, pos int) []rune {
	input := string(line)
	trimmed := strings.TrimSpace(input)

	if !strings.HasPrefix(trimmed, "/") {
		return line
	}

	inlineHint := p.findInlineHint(trimmed)
	panelLines := p.buildPanel(trimmed)

	if inlineHint == "" && len(panelLines) == 0 {
		return line
	}

	var buf strings.Builder
	buf.WriteString(input)
	buf.WriteString("\033[s")

	if inlineHint != "" {
		buf.WriteString("\033[90m")
		buf.WriteString(inlineHint)
		buf.WriteString("\033[0m")
	}

	for _, pl := range panelLines {
		buf.WriteString("\n\033[2K")
		buf.WriteString(pl)
	}

	buf.WriteString("\033[u")
	return []rune(buf.String())
}

func (p *HintPainter) findInlineHint(input string) string {
	lower := strings.ToLower(input)

	for _, cmd := range commands {
		if cmd.ArgCompleter != nil && strings.HasPrefix(lower, strings.ToLower(cmd.Name)+" ") {
			return p.findArgHint(input, cmd.Name, cmd.ArgCompleter)
		}
	}

	for _, cmd := range commands {
		if strings.HasPrefix(strings.ToLower(cmd.Name), lower) && strings.ToLower(cmd.Name) != lower {
			return cmd.Name[len(input):] + "  " + cmd.Description
		}
	}

	for _, cmd := range commands {
		if strings.ToLower(cmd.Name) == lower {
			return "  " + cmd.Description
		}
	}

	return ""
}

func (p *HintPainter) findArgHint(input, cmdName string, completer func() []string) string {
	prefix := strings.TrimSpace(input[len(cmdName):])
	items := completer()

	for _, item := range items {
		if prefix == "" {
			return item
		}
		if strings.HasPrefix(strings.ToLower(item), strings.ToLower(prefix)) && !strings.EqualFold(item, prefix) {
			return item[len(prefix):]
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
	prefix := strings.TrimSpace(input[len(cmdName):])
	items := completer()

	var lines []string
	for _, item := range items {
		if prefix == "" || strings.HasPrefix(strings.ToLower(item), strings.ToLower(prefix)) {
			lines = append(lines, fmt.Sprintf("  \033[36m%s\033[0m", item))
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
