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

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
)

type ArgCompleter struct {
	Placeholder string
	Completer   func(b backend.Backend, modelID uuid.UUID) []string
}

type Command struct {
	Name        string
	Description string
	Usage       string
	LocalOnly   bool
	Hidden      bool
	Args        []ArgCompleter
}

var commands = []Command{
	{Name: "/new", Description: "Start a new chat session", Usage: "/new"},
	{Name: "/status", Description: "Show current session status", Usage: "/status"},
	{Name: "/history", Description: "Print current session history", Usage: "/history"},
	{
		Name:        "/model",
		Description: "Manage and switch models",
		Usage:       "/model [provider/model]",
		Args: []ArgCompleter{{
			Placeholder: "provider/model",
			Completer: func(b backend.Backend, _ uuid.UUID) []string {
				result, err := b.ListModels(1, 100)
				if err != nil {
					return nil
				}
				names := make([]string, 0, len(result.Data))
				for _, m := range result.Data {
					if modelSwitchable(m) {
						names = append(names, m.Provider.Name+"/"+m.Name)
					}
				}
				return names
			},
		}},
	},
	{Name: "/provider", Description: "Manage model providers", Usage: "/provider"},
	{Name: "/mcp", Description: "Manage MCP servers", Usage: "/mcp"},
	{
		Name:        "/agent",
		Description: "Manage and switch agents",
		Usage:       "/agent [agent-name]",
		Args: []ArgCompleter{{
			Placeholder: "agent-name",
			Completer: func(b backend.Backend, _ uuid.UUID) []string {
				result, err := b.ListAgents(1, 100)
				if err != nil {
					return nil
				}
				names := make([]string, 0, len(result.Data))
				for _, a := range result.Data {
					names = append(names, a.Name)
				}
				return names
			},
		}},
	},
	{
		Name:        "/settings",
		Description: "Edit system settings",
		Usage:       "/settings [show|edit]",
		Args: []ArgCompleter{{
			Placeholder: "show|edit",
			Completer: func(_ backend.Backend, _ uuid.UUID) []string {
				return []string{"show", "edit"}
			},
		}},
	},
	{Name: "/memory", Description: "View agent long-term memories", Usage: "/memory"},
	{
		Name:        "/cwd",
		Description: "Set or show current working directory (reads AGENTS.md)",
		Usage:       "/cwd [<path>|clear]",
	},
	{
		Name:        "/skill",
		Description: "View available skills",
		Usage:       "/skill",
	},
	{
		Name:        "/think",
		Description: "Set thinking mode",
		Usage:       "/think [off|<level>]",
		Args: []ArgCompleter{{
			Placeholder: "off|level",
			Completer: func(b backend.Backend, modelID uuid.UUID) []string {
				result := []string{"off"}
				if modelID != uuid.Nil {
					levels, err := b.GetModelThinkingLevels(modelID)
					if err == nil && levels != nil {
						result = append(result, *levels...)
					}
				}
				return result
			},
		}},
	},
	{Name: "/logs", Description: "View debug logs", Usage: "/logs", LocalOnly: true},
	{Name: "/help", Description: "Show available commands", Usage: "/help"},
	{Name: "/exit", Description: "Quit the chat", Usage: "/exit"},
}

func matchingCommands(input string, localMode bool) []Command {
	trimmed := strings.ToLower(strings.TrimSpace(input))
	if trimmed == "" || !strings.HasPrefix(trimmed, "/") {
		return nil
	}

	matches := make([]Command, 0, len(commands))
	for _, cmd := range commands {
		if commandVisible(cmd, localMode) && strings.HasPrefix(strings.ToLower(cmd.Name), trimmed) {
			matches = append(matches, cmd)
		}
	}

	return matches
}

func filterByPrefix(items []string, prefix string) []string {
	if prefix == "" {
		return items
	}
	prefix = strings.ToLower(prefix)
	result := make([]string, 0, len(items))
	for _, item := range items {
		if strings.HasPrefix(strings.ToLower(item), prefix) {
			result = append(result, item)
		}
	}
	return result
}

func commandVisible(cmd Command, localMode bool) bool {
	return !cmd.Hidden && (!cmd.LocalOnly || localMode)
}
