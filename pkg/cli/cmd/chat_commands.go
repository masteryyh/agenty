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
	"github.com/masteryyh/agenty/pkg/cli/api"
	"github.com/pterm/pterm"
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

type CommandHandler func(c *api.Client, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error)

var commandRegistry = map[string]CommandHandler{
	"/new":     handleNewCmd,
	"/status":  handleStatusCmd,
	"/history": handleHistoryCmd,
	"/model":   handleModelCmd,
	"/think":   handleThinkCmd,
	"/help":    handleHelpCmd,
	"/exit":    handleExitCmd,
	"/agent":   handleAgentCmd,
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

func handleSlashCommand(c *api.Client, input string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	parts := parseSlashInput(input)
	if len(parts) == 0 {
		return CommandResult{}, nil
	}

	command := strings.ToLower(parts[0])

	handler, ok := commandRegistry[command]
	if !ok {
		return CommandResult{}, nil
	}

	return handler(c, parts[1:], sessionID, modelID, agentID, state)
}

func resolveModel(c *api.Client, modelSpec string) (uuid.UUID, string, error) {
	parts := strings.Split(modelSpec, "/")
	if len(parts) != 2 {
		return uuid.Nil, "", fmt.Errorf("invalid format, use: provider-name/model-name")
	}

	providerName := strings.TrimSpace(parts[0])
	modelName := strings.TrimSpace(parts[1])

	providers, err := c.ListProviders(1, 100)
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

	modelsList, err := c.ListModels(1, 100)
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

func handleExitCmd(c *api.Client, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	pterm.Info.Println("Goodbye!")
	return CommandResult{Handled: true, ShouldExit: true}, nil
}

func handleThinkCmd(c *api.Client, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	if len(args) == 0 {
		if state.Thinking {
			if state.ThinkingLevel != "" {
				pterm.Info.Printf("Thinking is %s (level: %s)\n", pterm.FgGreen.Sprint("on"), pterm.FgYellow.Sprint(state.ThinkingLevel))
			} else {
				pterm.Info.Printf("Thinking is %s\n", pterm.FgGreen.Sprint("on"))
			}
		} else {
			pterm.Info.Printf("Thinking is %s\n", pterm.FgRed.Sprint("off"))
		}
		pterm.Info.Println("Usage: /think [off|<level>]")
		return CommandResult{Handled: true}, nil
	}

	arg := strings.ToLower(args[0])
	if arg == "off" {
		state.Thinking = false
		state.ThinkingLevel = ""
		pterm.Success.Println("Thinking disabled")
		return CommandResult{Handled: true}, nil
	}

	supportedLevelsPtr, _ := c.GetModelThinkingLevels(modelID)
	var supportedLevels []string
	if supportedLevelsPtr != nil {
		supportedLevels = *supportedLevelsPtr
	}
	valid := false
	for _, l := range supportedLevels {
		if l == arg {
			valid = true
			break
		}
	}
	if !valid {
		if len(supportedLevels) == 0 {
			pterm.Error.Printf("Model does not support thinking\n")
		} else {
			pterm.Error.Printf("Unknown level: %s. Supported: %s\n", arg, strings.Join(supportedLevels, ", "))
		}
		return CommandResult{Handled: true}, nil
	}

	state.Thinking = true
	state.ThinkingLevel = arg
	pterm.Success.Printf("Thinking enabled (level: %s)\n", pterm.FgYellow.Sprint(arg))
	return CommandResult{Handled: true}, nil
}

func handleNewCmd(c *api.Client, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	currentSession, err := c.GetSession(sessionID)
	if err != nil {
		return CommandResult{Handled: true}, fmt.Errorf("failed to get current session: %w", err)
	}

	session, err := c.CreateSession(currentSession.AgentID)
	if err != nil {
		return CommandResult{Handled: true}, fmt.Errorf("failed to create new session: %w", err)
	}

	clearScreen()
	pterm.Success.Printf("Started new session: %s\n", session.ID)
	fmt.Println()
	pterm.Info.Printf("Type %s to see available commands, %s to exit\n", pterm.FgYellow.Sprint("/help"), pterm.FgYellow.Sprint("/exit"))
	fmt.Println()

	return CommandResult{Handled: true, NewSessionID: session.ID}, nil
}

func handleStatusCmd(c *api.Client, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	session, err := c.GetSession(sessionID)
	if err != nil {
		return CommandResult{Handled: true}, fmt.Errorf("failed to get session: %w", err)
	}

	modelsList, err := c.ListModels(1, 100)
	var currentModelInfo string
	if err == nil {
		for _, m := range modelsList.Data {
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

	fmt.Println()
	pterm.DefaultHeader.Println("📊 Session Status")
	pterm.Printf("  Session ID: %s\n", pterm.FgCyan.Sprint(session.ID))
	pterm.Printf("  Current Model: %s\n", pterm.FgMagenta.Sprint(currentModelInfo))
	pterm.Printf("  Token Consumed: %s\n", pterm.FgYellow.Sprint(session.TokenConsumed))
	pterm.Printf("  Messages: %s\n", pterm.FgGreen.Sprint(len(session.Messages)))
	pterm.Printf("  Created: %s\n", pterm.FgGray.Sprint(session.CreatedAt.Format("2006-01-02 15:04:05")))
	pterm.Printf("  Updated: %s\n", pterm.FgGray.Sprint(session.UpdatedAt.Format("2006-01-02 15:04:05")))
	fmt.Println()

	return CommandResult{Handled: true}, nil
}

func handleHistoryCmd(c *api.Client, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	session, err := c.GetSession(sessionID)
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

func handleModelCmd(c *api.Client, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	if len(args) == 0 {
		return CommandResult{Handled: true}, fmt.Errorf("usage: /model [provider-name/model-name]")
	}

	resolvedID, displayName, err := resolveModel(c, args[0])
	if err != nil {
		return CommandResult{Handled: true}, err
	}

	pterm.Success.Printf("Switched to model: %s\n", displayName)
	return CommandResult{Handled: true, NewModelID: resolvedID}, nil
}

func handleHelpCmd(c *api.Client, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	PrintCommandHints()
	return CommandResult{Handled: true}, nil
}

func handleAgentCmd(c *api.Client, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	if len(args) == 0 {
		if agentID == uuid.Nil {
			pterm.Info.Println("No agent selected")
		} else {
			agent, err := c.GetAgent(agentID)
			if err != nil {
				pterm.Info.Printf("Current agent ID: %s\n", agentID)
			} else {
				pterm.Info.Printf("Current agent: %s\n", pterm.FgCyan.Sprint(agent.Name))
			}
		}
		pterm.Info.Println("Usage: /agent [agent-name]")
		return CommandResult{Handled: true}, nil
	}

	agentName := strings.TrimSpace(args[0])
	agents, err := c.ListAgents(1, 100)
	if err != nil {
		return CommandResult{Handled: true}, fmt.Errorf("failed to list agents: %w", err)
	}

	var targetAgentID uuid.UUID
	for _, a := range agents.Data {
		if strings.EqualFold(a.Name, agentName) {
			targetAgentID = a.ID
			break
		}
	}
	if targetAgentID == uuid.Nil {
		return CommandResult{Handled: true}, fmt.Errorf("agent '%s' not found", agentName)
	}

	lastSession, err := c.GetLastSessionByAgent(targetAgentID)
	var newSessionID uuid.UUID
	if err == nil && lastSession != nil {
		newSessionID = lastSession.ID
		clearScreen()
		pterm.Success.Printf("Switched to agent: %s\n", pterm.FgCyan.Sprint(agentName))
		pterm.Info.Printf("Resuming last session: %s\n", newSessionID)
		fmt.Println()

		if len(lastSession.Messages) > 0 {
			messageCount := len(lastSession.Messages)
			startIdx := 0
			if messageCount > 10 {
				startIdx = messageCount - 10
				pterm.Info.Printf("Showing last 10 messages (total: %d). Use /history to view all.\n", messageCount)
			}
			pterm.DefaultSection.Println("Previous Messages")
			printMessageHistory(lastSession.Messages[startIdx:])
			fmt.Println()
		}
	} else {
		newSession, err := c.CreateSession(targetAgentID)
		if err != nil {
			return CommandResult{Handled: true}, fmt.Errorf("failed to create session for agent: %w", err)
		}
		newSessionID = newSession.ID
		clearScreen()
		pterm.Success.Printf("Switched to agent: %s\n", pterm.FgCyan.Sprint(agentName))
		pterm.Info.Printf("Started new session: %s\n", newSessionID)
		fmt.Println()
	}

	pterm.Info.Printf("Type %s to see available commands, %s to exit\n", pterm.FgYellow.Sprint("/help"), pterm.FgYellow.Sprint("/exit"))
	fmt.Println()

	return CommandResult{Handled: true, NewAgentID: targetAgentID, NewSessionID: newSessionID}, nil
}

func clearScreen() {
	fmt.Print("\033[2J\033[H")
}
