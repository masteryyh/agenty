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
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/cli/theme"
	"github.com/masteryyh/agenty/pkg/models"
	"golang.org/x/term"
)

func startChat(b backend.Backend, isLocal bool) error {
	initialized, err := b.IsInitialized()
	if err != nil {
		return fmt.Errorf("failed to check initialization status: %w", err)
	}
	if !initialized {
		if err := RunWizardTUI(b); err != nil {
			return fmt.Errorf("setup wizard failed: %w", err)
		}
	}

	agents, err := b.ListAgents(1, 100)
	if err != nil {
		return fmt.Errorf("failed to list agents: %w", err)
	}
	if len(agents.Data) == 0 {
		defaultSoul := ""
		_, err := b.CreateAgent(&models.CreateAgentDto{
			Name:      "default",
			Soul:      &defaultSoul,
			IsDefault: true,
		})
		if err != nil {
			return fmt.Errorf("failed to create default agent: %w", err)
		}
		agents, err = b.ListAgents(1, 100)
		if err != nil {
			return fmt.Errorf("failed to list agents: %w", err)
		}
	}

	var agentID uuid.UUID
	var agentName string
	if len(agents.Data) == 1 {
		agentID = agents.Data[0].ID
		agentName = agents.Data[0].Name
	} else {
		var defaultAgent *models.AgentDto
		for i := range agents.Data {
			if agents.Data[i].IsDefault {
				defaultAgent = &agents.Data[i]
				break
			}
		}
		if defaultAgent != nil {
			agentID = defaultAgent.ID
			agentName = defaultAgent.Name
		} else {
			agentID = agents.Data[0].ID
			agentName = agents.Data[0].Name
		}
	}

	var sessionID uuid.UUID
	var session *models.ChatSessionDto
	sessionRestored := false

	lastSession, err := b.GetLastSessionByAgent(agentID)
	if err == nil && lastSession != nil {
		sessionID = lastSession.ID
		session = lastSession
		sessionRestored = true
	} else {
		session, err = b.CreateSession(agentID)
		if err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}
		sessionID = session.ID
	}

	if isLocal {
		if cwd, err := os.Getwd(); err == nil {
			var agentsMD *string
			for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
				candidate := filepath.Join(cwd, name)
				if data, readErr := os.ReadFile(candidate); readErr == nil {
					content := string(data)
					agentsMD = &content
					break
				}
			}
			if setErr := b.SetSessionCwd(sessionID, &cwd, agentsMD); setErr == nil {
				session.Cwd = &cwd
			}
		}
	}

	modelID, modelInfo, err := resolveInitialChatModel(b, agentID, session, sessionRestored)
	if err != nil {
		return err
	}
	chatState := chatStateForSession(b, modelID, session, sessionRestored)
	chatState.LocalMode = isLocal

	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		SetRenderWidth(w)
	}

	theme.InitTheme(theme.DetectDarkBackground())
	refreshRenderStyles()

	bridge := newUIBridge()
	model := newChatModel(b, bridge, sessionID, modelID, agentID, modelInfo, agentName, session.Messages, chatState)

	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	bridge.program = p

	_, err = p.Run()
	bridge.Close()

	theme.RestoreTerminal()

	return err
}

func resolveInitialChatModel(b backend.Backend, agentID uuid.UUID, session *models.ChatSessionDto, restored bool) (uuid.UUID, string, error) {
	if restored && session != nil && session.LastUsedModel != uuid.Nil {
		if model, ok := configuredModelByID(b, session.LastUsedModel); ok {
			return model.ID, modelDisplayName(model), nil
		}
	}

	if agent, err := b.GetAgent(agentID); err == nil && agent != nil && len(agent.Models) > 0 {
		if session != nil && session.LastUsedModel != uuid.Nil {
			for _, m := range agent.Models {
				if m.ID == session.LastUsedModel && modelConfigured(m) {
					return m.ID, modelDisplayName(m), nil
				}
			}
		}
		for _, m := range agent.Models {
			if modelConfigured(m) {
				return m.ID, modelDisplayName(m), nil
			}
		}
	}

	if defaultModel, err := b.GetDefaultModel(); err == nil && defaultModel != nil && modelConfigured(*defaultModel) {
		return defaultModel.ID, modelDisplayName(*defaultModel), nil
	}

	modelsList, err := b.ListModels(1, 100)
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("failed to list models: %w", err)
	}
	for _, m := range modelsList.Data {
		if modelConfigured(m) {
			return m.ID, modelDisplayName(m), nil
		}
	}
	return uuid.Nil, "", fmt.Errorf("no models available, use /model to create one")
}

func configuredModelByID(b backend.Backend, modelID uuid.UUID) (models.ModelDto, bool) {
	modelsList, err := b.ListModels(1, 100)
	if err != nil {
		return models.ModelDto{}, false
	}
	for _, m := range modelsList.Data {
		if m.ID == modelID && modelConfigured(m) {
			return m, true
		}
	}
	return models.ModelDto{}, false
}

func modelConfigured(model models.ModelDto) bool {
	return model.Provider != nil && model.Provider.APIKeyCensored != "<not set>"
}
