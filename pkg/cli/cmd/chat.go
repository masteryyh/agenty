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

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/cli/theme"
	"github.com/masteryyh/agenty/pkg/models"
	"golang.org/x/term"
)

func startChat(b backend.Backend, isLocal bool) error {
	if isLocal {
		registerCommand(
			Command{Name: "/logs", Description: "View debug logs", Usage: "/logs"},
			handleLogsCmd,
		)
	}

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

	lastSession, err := b.GetLastSessionByAgent(agentID)
	if err == nil && lastSession != nil {
		sessionID = lastSession.ID
		session = lastSession
	} else {
		session, err = b.CreateSession(agentID)
		if err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}
		sessionID = session.ID
	}

	var modelID uuid.UUID
	var modelInfo string

	agent, agentErr := b.GetAgent(agentID)
	if agentErr == nil && agent != nil && len(agent.Models) > 0 {
		if session.LastUsedModel != uuid.Nil {
			for _, m := range agent.Models {
				if m.ID == session.LastUsedModel && m.Provider != nil && m.Provider.APIKeyCensored != "<not set>" {
					modelID = m.ID
					modelInfo = modelDisplayName(m)
					break
				}
			}
		}
		if modelID == uuid.Nil {
			for _, m := range agent.Models {
				if m.Provider != nil && m.Provider.APIKeyCensored != "<not set>" {
					modelID = m.ID
					modelInfo = modelDisplayName(m)
					break
				}
			}
		}
	}

	if modelID == uuid.Nil {
		defaultModel, err := b.GetDefaultModel()
		if err == nil && defaultModel != nil {
			modelID = defaultModel.ID
			modelInfo = modelDisplayName(*defaultModel)
		} else {
			modelsList, err := b.ListModels(1, 1)
			if err != nil {
				return fmt.Errorf("failed to list models: %w", err)
			}
			if len(modelsList.Data) > 0 {
				modelID = modelsList.Data[0].ID
				modelInfo = modelDisplayName(modelsList.Data[0])
			} else {
				return fmt.Errorf("no models available, use /model to create one")
			}
		}
	}

	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		SetRenderWidth(w)
	}

	bridge := newUIBridge()
	model := newChatModel(b, bridge, sessionID, modelID, agentID, modelInfo, agentName, session.Messages)

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
