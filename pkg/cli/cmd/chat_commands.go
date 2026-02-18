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
	"github.com/masteryyh/agenty/pkg/cli/client"
	"github.com/pterm/pterm"
)

type CommandHandler func(c *client.Client, args []string, sessionID uuid.UUID, modelID uuid.UUID) (handled bool, newSessionID uuid.UUID, newModelID uuid.UUID, err error)

var commandRegistry = map[string]CommandHandler{
	"/new":     handleNewCmd,
	"/status":  handleStatusCmd,
	"/history": handleHistoryCmd,
	"/model":   handleModelCmd,
	"/help":    handleHelpCmd,
}

func handleSlashCommand(c *client.Client, input string, sessionID uuid.UUID, modelID uuid.UUID) (handled bool, newSessionID uuid.UUID, newModelID uuid.UUID, err error) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return false, uuid.Nil, uuid.Nil, nil
	}

	command := strings.ToLower(parts[0])

	handler, ok := commandRegistry[command]
	if !ok {
		return false, uuid.Nil, uuid.Nil, nil
	}

	return handler(c, parts[1:], sessionID, modelID)
}

func resolveModel(c *client.Client, modelSpec string) (uuid.UUID, string, error) {
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

func handleNewCmd(c *client.Client, args []string, sessionID uuid.UUID, modelID uuid.UUID) (bool, uuid.UUID, uuid.UUID, error) {
	session, err := c.CreateSession()
	if err != nil {
		return true, uuid.Nil, uuid.Nil, fmt.Errorf("failed to create new session: %w", err)
	}

	clearScreen()
	pterm.Success.Printf("Started new session: %s\n", session.ID)
	fmt.Println()
	pterm.Info.Printf("Type %s to see available commands, %s to exit\n", pterm.FgYellow.Sprint("/help"), pterm.FgYellow.Sprint("/exit"))
	fmt.Println()

	return true, session.ID, uuid.Nil, nil
}

func handleStatusCmd(c *client.Client, args []string, sessionID uuid.UUID, modelID uuid.UUID) (bool, uuid.UUID, uuid.UUID, error) {
	session, err := c.GetSession(sessionID)
	if err != nil {
		return true, uuid.Nil, uuid.Nil, fmt.Errorf("failed to get session: %w", err)
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

	return true, uuid.Nil, uuid.Nil, nil
}

func handleHistoryCmd(c *client.Client, args []string, sessionID uuid.UUID, modelID uuid.UUID) (bool, uuid.UUID, uuid.UUID, error) {
	session, err := c.GetSession(sessionID)
	if err != nil {
		return true, uuid.Nil, uuid.Nil, fmt.Errorf("failed to get session: %w", err)
	}

	if len(session.Messages) == 0 {
		pterm.Info.Println("No message history available")
		return true, uuid.Nil, uuid.Nil, nil
	}

	if err := openHistoryViewer(session.Messages); err != nil {
		pterm.Warning.Printf("Failed to open interactive viewer: %v\n", err)
		pterm.Info.Println("Showing history in console instead...")
		fmt.Println()
		printMessageHistory(session.Messages)
		fmt.Println()
	}

	return true, uuid.Nil, uuid.Nil, nil
}

func handleModelCmd(c *client.Client, args []string, sessionID uuid.UUID, modelID uuid.UUID) (bool, uuid.UUID, uuid.UUID, error) {
	if len(args) == 0 {
		return true, uuid.Nil, uuid.Nil, fmt.Errorf("usage: /model [provider-name/model-name]")
	}

	resolvedID, displayName, err := resolveModel(c, args[0])
	if err != nil {
		return true, uuid.Nil, uuid.Nil, err
	}

	pterm.Success.Printf("Switched to model: %s\n", displayName)
	return true, uuid.Nil, resolvedID, nil
}

func handleHelpCmd(c *client.Client, args []string, sessionID uuid.UUID, modelID uuid.UUID) (bool, uuid.UUID, uuid.UUID, error) {
	PrintCommandHints()
	return true, uuid.Nil, uuid.Nil, nil
}

func clearScreen() {
	fmt.Print("\033[2J\033[H")
}
