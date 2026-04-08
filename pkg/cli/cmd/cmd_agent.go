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
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/cli/ui"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/pterm/pterm"
)

func handleAgentCmd(b backend.Backend, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	if len(args) > 0 {
		return switchToAgent(b, args[0])
	}

	for {
		agents, err := b.ListAgents(1, 100)
		if err != nil {
			return CommandResult{Handled: true}, fmt.Errorf("failed to list agents: %w", err)
		}

		if len(agents.Data) == 0 {
			pterm.Warning.Println("No agents found")
			res, err := ui.ShowList("Agents", []string{"(no agents)"}, "a add  ·  Esc back")
			if err != nil {
				return CommandResult{Handled: true}, err
			}
			if res.Action == ui.ListActionAdd {
				if err := doCreateAgent(b); err != nil && !errors.Is(err, ui.ErrCancelled) {
					pterm.Error.Printf("Failed to create agent: %v\n", err)
				}
				continue
			}
			return CommandResult{Handled: true}, nil
		}

		items := make([]string, len(agents.Data))
		for i, a := range agents.Data {
			marker := ""
			if a.IsDefault {
				marker = " [default]"
			}
			current := ""
			if a.ID == agentID {
				current = " ← current"
			}
			items[i] = fmt.Sprintf("%s%s%s", a.Name, marker, pterm.FgGray.Sprint(current))
		}

		res, err := ui.ShowList("Agents  "+pterm.FgGray.Sprint("(select to switch)"), items, listHints)
		if err != nil {
			return CommandResult{Handled: true}, err
		}

		switch res.Action {
		case ui.ListActionSelect:
			target := agents.Data[res.Index]
			return switchToAgent(b, target.Name)

		case ui.ListActionAdd:
			if err := doCreateAgent(b); err != nil && !errors.Is(err, ui.ErrCancelled) {
				pterm.Error.Printf("Failed to create agent: %v\n", err)
			}
			continue

		case ui.ListActionEdit:
			if err := doUpdateAgent(b, agents.Data[res.Index]); err != nil && !errors.Is(err, ui.ErrCancelled) {
				pterm.Error.Printf("Failed to update agent: %v\n", err)
			}
			continue

		case ui.ListActionDelete:
			target := agents.Data[res.Index]
			confirmed, err := ui.ShowConfirm(fmt.Sprintf("Delete agent '%s'? This will also delete all sessions, messages and memories.", target.Name))
			if err != nil {
				return CommandResult{Handled: true}, err
			}
			if confirmed {
				if err := b.DeleteAgent(target.ID); err != nil {
					pterm.Error.Printf("Failed to delete agent: %v\n", err)
				} else {
					pterm.Success.Printf("Agent deleted: %s\n", target.Name)
				}
			}
			continue

		case ui.ListActionCancel:
			return CommandResult{Handled: true}, nil
		}
	}
}

func switchToAgent(b backend.Backend, agentName string) (CommandResult, error) {
	agentName = strings.TrimSpace(agentName)
	agents, err := b.ListAgents(1, 100)
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

	lastSession, err := b.GetLastSessionByAgent(targetAgentID)
	var newSessionID uuid.UUID
	if err == nil && lastSession != nil {
		newSessionID = lastSession.ID
		sessionDesc := fmt.Sprintf("resumed · %d messages", len(lastSession.Messages))
		clearScreen()
		fmt.Printf("  %-10s %s\n", pterm.FgGray.Sprint("Agent"), pterm.FgCyan.Sprint(agentName))
		fmt.Printf("  %-10s %s  %s\n\n", pterm.FgGray.Sprint("Session"),
			pterm.FgGray.Sprint(newSessionID.String()[:8]+"…"),
			pterm.FgGray.Sprint("("+sessionDesc+")"))

		if len(lastSession.Messages) > 0 {
			messageCount := len(lastSession.Messages)
			startIdx := 0
			if messageCount > 10 {
				startIdx = messageCount - 10
				fmt.Printf("  %s\n\n", pterm.FgGray.Sprintf("Showing last 10 of %d messages  ·  /history to view all", messageCount))
			}
			fmt.Printf("  %s\n  %s\n\n", pterm.Bold.Sprint("Chat History"), pterm.FgGray.Sprint(strings.Repeat("─", 56)))
			printMessageHistory(lastSession.Messages[startIdx:])
			fmt.Println()
		}
	} else {
		newSession, err := b.CreateSession(targetAgentID)
		if err != nil {
			return CommandResult{Handled: true}, fmt.Errorf("failed to create session for agent: %w", err)
		}
		newSessionID = newSession.ID
		clearScreen()
		fmt.Printf("  %-10s %s\n", pterm.FgGray.Sprint("Agent"), pterm.FgCyan.Sprint(agentName))
		fmt.Printf("  %-10s %s  %s\n\n", pterm.FgGray.Sprint("Session"),
			pterm.FgGray.Sprint(newSessionID.String()[:8]+"…"),
			pterm.FgGray.Sprint("(new)"))
	}

	fmt.Printf("  %s\n\n", pterm.FgGray.Sprintf("Type %s for commands  ·  %s to quit",
		pterm.FgWhite.Sprint("/help"), pterm.FgWhite.Sprint("/exit")))

	return CommandResult{Handled: true, NewAgentID: targetAgentID, NewSessionID: newSessionID}, nil
}

func doCreateAgent(b backend.Backend) error {
	fields := []*ui.FormField{
		ui.TextField("Name", "", true),
		ui.TextField("Soul", "", false),
		ui.ToggleField("Default agent", false),
	}
	fields[1].Placeholder = "system prompt, leave blank for default"

	submitted, err := ui.ShowForm("Create Agent", fields)
	if err != nil {
		return err
	}
	if !submitted {
		return ui.ErrCancelled
	}

	name := fields[0].Value
	soul := fields[1].Value
	isDefault := fields[2].BoolValue()

	agent, err := b.CreateAgent(&models.CreateAgentDto{
		Name:      name,
		Soul:      &soul,
		IsDefault: isDefault,
	})
	if err != nil {
		return err
	}
	pterm.Success.Printf("Agent created: %s (%s)\n", agent.Name, agent.ID)
	return nil
}

func doUpdateAgent(b backend.Backend, target models.AgentDto) error {
	fields := []*ui.FormField{
		ui.TextField("Name", target.Name, true),
		ui.TextField("Soul", target.Soul, false),
		ui.ToggleField("Default agent", target.IsDefault),
	}
	fields[1].Placeholder = "system prompt"

	submitted, err := ui.ShowForm("Update Agent", fields)
	if err != nil {
		return err
	}
	if !submitted {
		return ui.ErrCancelled
	}

	newName := fields[0].Value
	newSoul := fields[1].Value
	newIsDefault := fields[2].BoolValue()

	if newName == target.Name && newSoul == target.Soul && newIsDefault == target.IsDefault {
		pterm.Info.Println("No changes detected, skipping update")
		return nil
	}

	if err := b.UpdateAgent(target.ID, &models.UpdateAgentDto{
		Name:      &newName,
		Soul:      &newSoul,
		IsDefault: &newIsDefault,
	}); err != nil {
		return err
	}
	pterm.Success.Printf("Agent updated: %s\n", newName)
	return nil
}
