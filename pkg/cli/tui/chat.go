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

package tui

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/cli/chatstate"
	"github.com/masteryyh/agenty/pkg/cli/theme"
	slcmd "github.com/masteryyh/agenty/pkg/cli/tui/command"
	"github.com/masteryyh/agenty/pkg/cli/tui/wizard"
	"github.com/masteryyh/agenty/pkg/models"
	"golang.org/x/term"
)

func StartChat(b backend.Backend, isLocal bool) error {
	initialized, err := b.IsInitialized()
	if err != nil {
		return fmt.Errorf("failed to check initialization status: %w", err)
	}
	if !initialized {
		if err := wizard.Run(b); err != nil {
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

	modelID, modelInfo, err := chatstate.ResolveInitialChatModel(b, agentID, session, sessionRestored)
	if err != nil {
		return err
	}
	chatState := chatstate.ChatStateForSession(b, modelID, session, sessionRestored)
	chatState.LocalMode = isLocal

	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		SetRenderWidth(w)
	}

	theme.InitTheme(theme.DetectDarkBackground())
	refreshRenderStyles()
	slcmd.RefreshStyles()

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
