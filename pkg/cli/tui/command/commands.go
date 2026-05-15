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

package command

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/cli/chatstate"
	"github.com/masteryyh/agenty/pkg/models"
)

type ChatState = chatstate.ChatState

type CommandResult struct {
	Handled         bool
	NewSessionID    uuid.UUID
	NewModelID      uuid.UUID
	NewAgentID      uuid.UUID
	NewModelName    string
	NewAgentName    string
	NewChatState    *ChatState
	SessionMessages []models.ChatMessageDto
	TokenConsumed   int64
	ShouldExit      bool
}

var ErrCancelled = errors.New("user cancelled")

type ListAction int

const (
	ListActionSelect ListAction = iota
	ListActionAdd
	ListActionEdit
	ListActionDelete
	ListActionCancel
)

type ListResult struct {
	Action ListAction
	Index  int
}

type Bridge interface {
	ShowList(title string, items []string, hints string, subtitle ...string) (*ListResult, error)
	ShowListWithCursor(title string, items []string, hints string, cursor int, subtitle ...string) (*ListResult, error)
	ShowListWithCursorAndValidate(title string, items []string, hints string, cursor int, validate func(action ListAction, idx int) error, subtitle ...string) (*ListResult, error)
	ShowListWithCursorAndActions(title string, items []string, hints string, cursor int, validate func(action ListAction, idx int) error, deleteConfirm func(idx int) string, subtitle ...string) (*ListResult, error)
	ShowHuhForm(form *huh.Form) (bool, error)
	ShowValidatedHuhForm(form *huh.Form, validate func() error) (bool, error)
	ShowSettingsEditor(backend backend.Backend, settings *models.SystemSettingsDto) error
	ShowConfirm(message string) (bool, error)
	ShowMultiSelect(title string, options []string, defaultIndices []int) ([]int, error)
	ShowLogViewer()
	Info(format string, args ...any)
	Warning(format string, args ...any)
	Success(format string, args ...any)
	Error(format string, args ...any)
	Print(text string)
	Println(text string)
	Printf(format string, args ...any)
	PrintHistory(messages []models.ChatMessageDto)
	PrintCommandHints(localMode bool)
}

type CommandHandler func(b backend.Backend, bridge Bridge, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error)

func ParseSlashInput(input string) []string {
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

func ResolveModel(b backend.Backend, modelSpec string) (uuid.UUID, string, error) {
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
			if !chatstate.ModelSwitchable(m) {
				return uuid.Nil, "", fmt.Errorf("model '%s/%s' is not a configured chat model", m.Provider.Name, m.Name)
			}
			return m.ID, fmt.Sprintf("%s/%s", m.Provider.Name, m.Name), nil
		}
	}

	return uuid.Nil, "", fmt.Errorf("model '%s' not found in provider '%s'", modelName, providerName)
}

const ListHints = "↑/↓ navigate  ·  Enter select  ·  a add  ·  e edit  ·  Ctrl+D delete  ·  Esc back"

func Handler(name string) CommandHandler {
	switch name {
	case "/new":
		return handleNewCmd
	case "/status":
		return handleStatusCmd
	case "/history":
		return handleHistoryCmd
	case "/model":
		return handleModelCmd
	case "/compact":
		return handleCompactCmd
	case "/think":
		return handleThinkCmd
	case "/help":
		return handleHelpCmd
	case "/exit":
		return handleExitCmd
	case "/agent":
		return handleAgentCmd
	case "/provider":
		return handleProviderCmd
	case "/mcp":
		return handleMCPCmd
	case "/settings":
		return handleSettingsCmd
	case "/memory":
		return handleMemoryCmd
	case "/cwd":
		return handleCwdCmd
	case "/skill":
		return handleSkillCmd
	case "/logs":
		return handleLogsCmd
	}
	return nil
}
