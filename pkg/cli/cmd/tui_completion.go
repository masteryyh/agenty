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
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
)

const maxCompletionVisible = 8

type completionModel struct {
	items   []string
	visible bool
	idx     int
	mode    completionMode
	argCmd  *Command
	allArgs []string
}

func (c *completionModel) dismiss() {
	c.visible = false
	c.mode = completeCmdMode
	c.argCmd = nil
	c.allArgs = nil
}

func (c *completionModel) height() int {
	if !c.visible || len(c.items) == 0 {
		return 0
	}
	n := min(len(c.items), maxCompletionVisible)
	return n
}

func (c *completionModel) handleTab(inputValue string, b backend.Backend, modelID uuid.UUID) (newInput string, inputChanged bool, cmd tea.Cmd) {
	if strings.HasPrefix(inputValue, "/") {
		spaceIdx := strings.Index(inputValue, " ")
		if spaceIdx > 0 {
			cmdName := strings.ToLower(strings.TrimSpace(inputValue[:spaceIdx]))
			argPrefix := inputValue[spaceIdx+1:]
			found := findCommand(cmdName)
			if found != nil && len(found.Args) > 0 {
				c.mode = completeArgMode
				if c.argCmd != found || len(c.allArgs) == 0 {
					c.argCmd = found
					c.allArgs = nil
					c.items = nil
					c.visible = false
					return "", false, c.fetchArgCompletions(found, argPrefix, b, modelID)
				}
				filtered := filterByPrefix(c.allArgs, argPrefix)
				switch len(filtered) {
				case 0:
					c.visible = false
				case 1:
					c.argCmd = nil
					c.allArgs = nil
					c.items = nil
					c.visible = false
					return found.Name + " " + filtered[0], true, nil
				default:
					c.items = filtered
					c.visible = true
					cap := len(c.items)
					if cap > maxCompletionVisible {
						cap = maxCompletionVisible
					}
					c.idx = (c.idx + 1) % cap
				}
				return "", false, nil
			}
		}
	}

	c.mode = completeCmdMode
	c.argCmd = nil
	c.allArgs = nil

	if inputValue != "" && !strings.HasPrefix(inputValue, "/") {
		return "", false, nil
	}

	if c.visible && len(c.items) > 0 {
		if len(c.items) == 1 {
			val := c.items[0] + " "
			c.visible = false
			c.items = nil
			c.idx = 0
			return val, true, nil
		}
		cap := len(c.items)
		if cap > maxCompletionVisible {
			cap = maxCompletionVisible
		}
		c.idx = (c.idx + 1) % cap
		return "", false, nil
	}

	c.recompute(inputValue)
	switch len(c.items) {
	case 0:
		c.visible = false
	case 1:
		if strings.HasPrefix(inputValue, "/") {
			val := c.items[0] + " "
			c.items = nil
			c.visible = false
			c.idx = 0
			return val, true, nil
		}
		c.visible = false
		c.idx = 0
	default:
		c.visible = true
		c.idx = 0
	}
	return "", false, nil
}

func (c *completionModel) handleEnterSelection() (selectedInput string, ok bool) {
	if !c.visible || len(c.items) == 0 {
		return "", false
	}
	selected := c.items[c.idx]
	if c.mode == completeArgMode && c.argCmd != nil {
		fullInput := c.argCmd.Name + " " + selected
		c.mode = completeCmdMode
		c.argCmd = nil
		c.allArgs = nil
		c.visible = false
		return fullInput, true
	}
	c.mode = completeCmdMode
	c.argCmd = nil
	c.allArgs = nil
	c.visible = false
	return selected, true
}

func (c *completionModel) handleArgMsg(msg argCompletionMsg) {
	if c.argCmd != nil && c.argCmd.Name == msg.cmdName {
		c.allArgs = msg.args
		c.items = filterByPrefix(msg.args, msg.prefix)
		c.idx = 0
		c.visible = len(c.items) > 0
	}
}

func (c *completionModel) updateLive(inputValue string) {
	if !strings.HasPrefix(inputValue, "/") {
		c.mode = completeCmdMode
		c.argCmd = nil
		c.allArgs = nil
		c.visible = false
		return
	}

	spaceIdx := strings.Index(inputValue, " ")
	if spaceIdx > 0 {
		cmdName := strings.ToLower(strings.TrimSpace(inputValue[:spaceIdx]))
		found := findCommand(cmdName)
		if found != nil && len(found.Args) > 0 && c.argCmd == found && len(c.allArgs) > 0 {
			argPrefix := inputValue[spaceIdx+1:]
			c.items = filterByPrefix(c.allArgs, argPrefix)
			c.idx = 0
			c.visible = len(c.items) > 0
			c.mode = completeArgMode
		} else {
			c.visible = false
		}
		return
	}

	if !strings.Contains(inputValue, " ") {
		c.mode = completeCmdMode
		c.argCmd = nil
		c.allArgs = nil
		c.recompute(inputValue)
		if len(c.items) > 0 {
			c.visible = true
			c.idx = 0
		} else {
			c.visible = false
		}
		return
	}

	c.visible = false
}

func (c *completionModel) recompute(inputValue string) {
	input := strings.ToLower(strings.TrimSpace(inputValue))
	c.items = nil
	for _, cmd := range commands {
		if strings.HasPrefix(strings.ToLower(cmd.Name), input) {
			c.items = append(c.items, cmd.Name)
		}
	}
}

func (c *completionModel) fetchArgCompletions(cmd *Command, prefix string, b backend.Backend, modelID uuid.UUID) tea.Cmd {
	if len(cmd.Args) == 0 || cmd.Args[0].Completer == nil {
		return nil
	}
	completer := cmd.Args[0].Completer
	cmdName := cmd.Name
	return func() tea.Msg {
		args := completer(b, modelID)
		return argCompletionMsg{cmdName: cmdName, args: args, prefix: prefix}
	}
}

func (c *completionModel) render() string {
	var buf strings.Builder
	n := len(c.items)
	if n > maxCompletionVisible {
		n = maxCompletionVisible
	}
	for i := 0; i < n; i++ {
		item := c.items[i]
		if c.mode == completeArgMode {
			if i == c.idx {
				cursor := styleCyan.Render("❯ ")
				val := styleWhite.Bold(true).Render(item)
				buf.WriteString(cursor)
				buf.WriteString(val)
			} else {
				buf.WriteString(styleModelInfo.Render("  " + item))
			}
		} else {
			cmd := findCommand(item)
			argsRef := ""
			desc := ""
			if cmd != nil {
				usageParts := strings.SplitN(cmd.Usage, " ", 2)
				if len(usageParts) == 2 {
					argsRef = " " + styleReasoningLabel.Render(usageParts[1])
				}
				desc = "  " + styleBarSep.Render(cmd.Description)
			}
			if i == c.idx {
				cursor := styleCyan.Render("❯ ")
				name := styleWhite.Bold(true).Render(item)
				buf.WriteString(cursor)
				buf.WriteString(name)
				buf.WriteString(argsRef)
				buf.WriteString(desc)
			} else {
				buf.WriteString(styleModelInfo.Render("  " + item))
				buf.WriteString(argsRef)
				buf.WriteString(desc)
			}
		}
		if i < n-1 {
			buf.WriteString("\n")
		}
	}
	return buf.String()
}
