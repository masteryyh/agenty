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
	"github.com/pterm/pterm"
)

func handleNewCmd(b backend.Backend, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	currentSession, err := b.GetSession(sessionID)
	if err != nil {
		return CommandResult{Handled: true}, fmt.Errorf("failed to get current session: %w", err)
	}

	session, err := b.CreateSession(currentSession.AgentID)
	if err != nil {
		return CommandResult{Handled: true}, fmt.Errorf("failed to create new session: %w", err)
	}

	clearScreen()
	fmt.Printf("  %s  %s\n\n", pterm.FgGray.Sprint("Session"), pterm.FgGray.Sprint(session.ID.String()[:8]+"…  (new)"))
	fmt.Printf("  %s\n\n", pterm.FgGray.Sprintf("Type %s for commands  ·  %s to quit",
		pterm.FgWhite.Sprint("/help"), pterm.FgWhite.Sprint("/exit")))

	return CommandResult{Handled: true, NewSessionID: session.ID}, nil
}

func handleStatusCmd(b backend.Backend, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
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

	thinkStatus := pterm.FgGray.Sprint("off")
	if state.Thinking {
		if state.ThinkingLevel != "" {
			thinkStatus = pterm.FgGreen.Sprint("on") + pterm.FgGray.Sprintf("  (%s)", state.ThinkingLevel)
		} else {
			thinkStatus = pterm.FgGreen.Sprint("on")
		}
	}

	fmt.Println()
	fmt.Printf("  %s\n  %s\n\n", pterm.Bold.Sprint("Session Status"), pterm.FgGray.Sprint(strings.Repeat("─", 56)))
	fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Session"), pterm.FgCyan.Sprint(session.ID))
	fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Model"), pterm.FgMagenta.Sprint(currentModelInfo))
	fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Thinking"), thinkStatus)
	fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Tokens used"), pterm.FgYellow.Sprint(session.TokenConsumed))
	fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Messages"), pterm.FgGreen.Sprint(len(session.Messages)))
	fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Created"), pterm.FgGray.Sprint(session.CreatedAt.Format("2006-01-02 15:04:05")))
	fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Updated"), pterm.FgGray.Sprint(session.UpdatedAt.Format("2006-01-02 15:04:05")))

	fmt.Println()

	return CommandResult{Handled: true}, nil
}

func handleHistoryCmd(b backend.Backend, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	session, err := b.GetSession(sessionID)
	if err != nil {
		return CommandResult{Handled: true}, fmt.Errorf("failed to get session: %w", err)
	}

	if len(session.Messages) == 0 {
		pterm.Info.Println("No message history available")
		return CommandResult{Handled: true}, nil
	}

	if err := openHistoryViewer(session.Messages); err != nil {
		pterm.Warning.Printf("Failed to open interactive viewer: %v\n", err)
		pterm.Info.Println("Showing history in console instead...")
		fmt.Println()
		printMessageHistory(session.Messages)
		fmt.Println()
	}

	return CommandResult{Handled: true}, nil
}
