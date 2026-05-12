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
	"strings"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
)

func handleNewCmd(b backend.Backend, bridge *UIBridge, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	currentSession, err := b.GetSession(sessionID)
	if err != nil {
		return CommandResult{Handled: true}, fmt.Errorf("failed to get current session: %w", err)
	}

	session, err := b.CreateSession(currentSession.AgentID)
	if err != nil {
		return CommandResult{Handled: true}, fmt.Errorf("failed to create new session: %w", err)
	}

	bridge.Printf("  %s  %s\n", styleGray.Render("Session"), styleGray.Render(session.ID.String()[:8]+"…  (new)"))
	bridge.Printf("  %s\n", styleGray.Render(fmt.Sprintf("Type %s for commands  ·  %s to quit",
		styleWhite.Render("/help"), styleWhite.Render("/exit"))))

	if state.LocalMode {
		if cwd, cwdErr := os.Getwd(); cwdErr == nil {
			var agentsMD *string
			for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
				candidate := filepath.Join(cwd, name)
				if data, readErr := os.ReadFile(candidate); readErr == nil {
					content := string(data)
					agentsMD = &content
					break
				}
			}
			_ = b.SetSessionCwd(session.ID, &cwd, agentsMD)
		}
	}

	result := CommandResult{Handled: true, NewSessionID: session.ID}
	if modelID, modelName, modelErr := resolveInitialChatModel(b, currentSession.AgentID, session, false); modelErr == nil {
		result.NewModelID = modelID
		result.NewModelName = modelName
		chatState := chatStateForSession(b, modelID, session, false)
		result.NewChatState = &chatState
	}
	return result, nil
}

func handleStatusCmd(b backend.Backend, bridge *UIBridge, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	session, err := b.GetSession(sessionID)
	if err != nil {
		return CommandResult{Handled: true}, fmt.Errorf("failed to get session: %w", err)
	}

	var currentModelInfo string
	if allModels, err := b.ListModels(1, 100); err == nil {
		for _, m := range allModels.Data {
			if m.ID == modelID {
				if m.Provider != nil {
					currentModelInfo = fmt.Sprintf("%s/%s", m.Provider.Name, m.Name)
				} else {
					currentModelInfo = m.Name
				}
				break
			}
		}
	}
	if currentModelInfo == "" {
		currentModelInfo = modelID.String()
	}

	thinkStatus := styleGray.Render("off")
	if state.Thinking {
		if state.ThinkingLevel != "" {
			thinkStatus = styleGreen.Render("on") + styleGray.Render(fmt.Sprintf("  (%s)", state.ThinkingLevel))
		} else {
			thinkStatus = styleGreen.Render("on")
		}
	}

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(renderSectionHeader("Session Status"))
	sb.WriteString(renderKV("Session", styleCyan.Render(session.ID.String()), 16))
	sb.WriteString(renderKV("Model", styleMagenta.Render(currentModelInfo), 16))
	sb.WriteString(renderKV("Thinking", thinkStatus, 16))
	sb.WriteString(renderKV("Session tokens", styleYellow.Render(fmt.Sprint(session.TokenConsumed)), 16))
	sb.WriteString(renderKV("Messages", styleGreen.Render(fmt.Sprint(len(session.Messages))), 16))
	if session.Cwd != nil {
		sb.WriteString(renderKV("Working dir", styleGray.Render(*session.Cwd), 16))
	}
	sb.WriteString(renderKV("Created", styleGray.Render(session.CreatedAt.Format("2006-01-02 15:04:05")), 16))
	sb.WriteString(renderKV("Updated", styleGray.Render(session.UpdatedAt.Format("2006-01-02 15:04:05")), 16))

	bridge.Print(sb.String())

	return CommandResult{Handled: true}, nil
}

func handleHistoryCmd(b backend.Backend, bridge *UIBridge, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	session, err := b.GetSession(sessionID)
	if err != nil {
		return CommandResult{Handled: true}, fmt.Errorf("failed to get session: %w", err)
	}

	if len(session.Messages) == 0 {
		bridge.Info("No message history available")
		return CommandResult{Handled: true}, nil
	}

	bridge.Print(renderMessageHistoryToString(session.Messages, false))

	return CommandResult{Handled: true}, nil
}
