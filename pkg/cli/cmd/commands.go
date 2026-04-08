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

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
)

type ChatState struct {
	Thinking      bool
	ThinkingLevel string
}

type CommandResult struct {
	Handled      bool
	NewSessionID uuid.UUID
	NewModelID   uuid.UUID
	NewAgentID   uuid.UUID
	ShouldExit   bool
}

type CommandHandler func(b backend.Backend, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error)

var commandRegistry = map[string]CommandHandler{
	"/new":        handleNewCmd,
	"/status":     handleStatusCmd,
	"/history":    handleHistoryCmd,
	"/model":      handleModelCmd,
	"/think":      handleThinkCmd,
	"/help":       handleHelpCmd,
	"/exit":       handleExitCmd,
	"/agent":      handleAgentCmd,
	"/provider":   handleProviderCmd,
	"/mcp":        handleMCPCmd,
	"/settings":   handleSettingsCmd,
	"/memory":     handleMemoryCmd,
}

func parseSlashInput(input string) []string {
	var parts []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	for _, r := range input {
		switch {
		case r == '\'' && !inDouble:
			inSingle = !inSingle
		case r == '"' && !inSingle:
			inDouble = !inDouble
		case (r == ' ' || r == '\t') && !inSingle && !inDouble:
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

func handleSlashCommand(b backend.Backend, input string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	parts := parseSlashInput(input)
	if len(parts) == 0 {
		return CommandResult{}, nil
	}

	command := strings.ToLower(parts[0])

	handler, ok := commandRegistry[command]
	if !ok {
		return CommandResult{}, nil
	}

	return handler(b, parts[1:], sessionID, modelID, agentID, state)
}

func resolveModel(b backend.Backend, modelSpec string) (uuid.UUID, string, error) {
	parts := strings.Split(modelSpec, "/")
	if len(parts) != 2 {
		return uuid.Nil, "", fmt.Errorf("invalid format, use: provider-name/model-name")
	}

	providerName := strings.TrimSpace(parts[0])
	modelName := strings.TrimSpace(parts[1])

	providers, err := b.ListProviders(1, 100)
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("failed to list providers: %w", err)
	}

	var providerID uuid.UUID
	for _, p := range providers.Data {
		if strings.EqualFold(p.Name, providerName) {
			providerID = p.ID
			break
		}
	}
	if providerID == uuid.Nil {
		return uuid.Nil, "", fmt.Errorf("provider '%s' not found", providerName)
	}

	modelsList, err := b.ListModels(1, 100)
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("failed to list models: %w", err)
	}

	for _, m := range modelsList.Data {
		if m.Provider != nil && m.Provider.ID == providerID && strings.EqualFold(m.Name, modelName) {
			return m.ID, fmt.Sprintf("%s/%s", m.Provider.Name, m.Name), nil
		}
	}

	return uuid.Nil, "", fmt.Errorf("model '%s' not found in provider '%s'", modelName, providerName)
}

func clearScreen() {
	fmt.Print("\033[2J\033[H")
}

var listHints = "↑/↓ navigate  ·  Enter select  ·  a add  ·  e edit  ·  Ctrl+D delete  ·  Esc back"
