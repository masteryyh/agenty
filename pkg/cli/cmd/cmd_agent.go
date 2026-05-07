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

	"github.com/charmbracelet/huh"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/models"
)

func handleAgentCmd(b backend.Backend, bridge *UIBridge, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	if len(args) > 0 {
		return switchToAgent(b, bridge, args[0])
	}

	for {
		agents, err := b.ListAgents(1, 100)
		if err != nil {
			return CommandResult{Handled: true}, fmt.Errorf("failed to list agents: %w", err)
		}

		if len(agents.Data) == 0 {
			bridge.Warning("No agents found")
			res, err := bridge.ShowList("Agents", []string{"(no agents)"}, "a add  ·  Esc back")
			if err != nil {
				return CommandResult{Handled: true}, err
			}
			if res.Action == ListActionAdd {
				if err := doCreateAgent(b, bridge); err != nil && !errors.Is(err, ErrCancelled) {
					bridge.Error("Failed to create agent: %v", err)
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
			items[i] = fmt.Sprintf("%s%s%s", a.Name, marker, styleGray.Render(current))
		}

		res, err := bridge.ShowListWithCursorAndActions("Agents  "+styleGray.Render("(select to switch)"), items, listHints, 0, nil, agentDeleteConfirm(agents.Data))
		if err != nil {
			return CommandResult{Handled: true}, err
		}

		switch res.Action {
		case ListActionSelect:
			target := agents.Data[res.Index]
			return switchToAgent(b, bridge, target.Name)

		case ListActionAdd:
			if err := doCreateAgent(b, bridge); err != nil && !errors.Is(err, ErrCancelled) {
				bridge.Error("Failed to create agent: %v", err)
			}
			continue

		case ListActionEdit:
			if err := doUpdateAgent(b, bridge, agents.Data[res.Index]); err != nil && !errors.Is(err, ErrCancelled) {
				bridge.Error("Failed to update agent: %v", err)
			}
			continue

		case ListActionDelete:
			target := agents.Data[res.Index]
			if err := b.DeleteAgent(target.ID); err != nil {
				bridge.Error("Failed to delete agent: %v", err)
			} else {
				bridge.Success("Agent deleted: %s", target.Name)
			}
			continue

		case ListActionCancel:
			return CommandResult{Handled: true}, nil
		}
	}
}

func agentDeleteConfirm(agentList []models.AgentDto) func(idx int) string {
	return func(idx int) string {
		if idx < 0 || idx >= len(agentList) {
			return ""
		}
		return fmt.Sprintf("Delete agent '%s'? This will also delete all sessions, messages and memories.", agentList[idx].Name)
	}
}

func switchToAgent(b backend.Backend, bridge *UIBridge, agentName string) (CommandResult, error) {
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

	result := CommandResult{Handled: true, NewAgentID: targetAgentID, NewAgentName: agentName}

	lastSession, err := b.GetLastSessionByAgent(targetAgentID)
	if err == nil && lastSession != nil {
		result.NewSessionID = lastSession.ID
		result.SessionMessages = lastSession.Messages
		result.TokenConsumed = lastSession.TokenConsumed
		modelID, modelName, modelErr := resolveInitialChatModel(b, targetAgentID, lastSession, true)
		if modelErr == nil {
			result.NewModelID = modelID
			result.NewModelName = modelName
			chatState := chatStateForSession(b, modelID, lastSession, true)
			result.NewChatState = &chatState
		}
		sessionDesc := fmt.Sprintf("resumed · %d messages", len(lastSession.Messages))
		bridge.Printf("  %-10s %s", styleGray.Render("Agent"), styleCyan.Render(agentName))
		bridge.Printf("  %-10s %s  %s\n", styleGray.Render("Session"),
			styleGray.Render(lastSession.ID.String()[:8]+"…"),
			styleGray.Render("("+sessionDesc+")"))
	} else {
		newSession, err := b.CreateSession(targetAgentID)
		if err != nil {
			return CommandResult{Handled: true}, fmt.Errorf("failed to create session for agent: %w", err)
		}
		result.NewSessionID = newSession.ID
		modelID, modelName, modelErr := resolveInitialChatModel(b, targetAgentID, newSession, false)
		if modelErr == nil {
			result.NewModelID = modelID
			result.NewModelName = modelName
			chatState := chatStateForSession(b, modelID, newSession, false)
			result.NewChatState = &chatState
		}
		bridge.Printf("  %-10s %s", styleGray.Render("Agent"), styleCyan.Render(agentName))
		bridge.Printf("  %-10s %s  %s\n", styleGray.Render("Session"),
			styleGray.Render(newSession.ID.String()[:8]+"…"),
			styleGray.Render("(new)"))
	}

	bridge.Printf("  %s\n", styleGray.Render(fmt.Sprintf("Type %s for commands  ·  %s to quit",
		styleWhite.Render("/help"), styleWhite.Render("/exit"))))

	return result, nil
}

func doCreateAgent(b backend.Backend, bridge *UIBridge) error {
	var name, soul string
	isDefault := false

	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Name").Value(&name),
		huh.NewInput().Title("Soul").Placeholder("system prompt, leave blank for default").Value(&soul),
		huh.NewSelect[bool]().Title("Default agent").
			Options(huh.NewOption("No", false), huh.NewOption("Yes", true)).
			Value(&isDefault),
	))

	submitted, err := bridge.ShowValidatedHuhForm(form, func() error {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("Name is required.")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if !submitted {
		return ErrCancelled
	}

	modelIDs, err := selectAgentModels(b, bridge, nil)
	if err != nil {
		return err
	}

	agent, err := b.CreateAgent(&models.CreateAgentDto{
		Name:      name,
		Soul:      &soul,
		IsDefault: isDefault,
		ModelIDs:  modelIDs,
	})
	if err != nil {
		return err
	}
	bridge.Success("Agent created: %s (%s)", agent.Name, agent.ID)
	return nil
}

func doUpdateAgent(b backend.Backend, bridge *UIBridge, target models.AgentDto) error {
	newName := target.Name
	newSoul := target.Soul
	newIsDefault := target.IsDefault

	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Name").Value(&newName),
		huh.NewInput().Title("Soul").Placeholder("system prompt").Value(&newSoul),
		huh.NewSelect[bool]().Title("Default agent").
			Options(huh.NewOption("No", false), huh.NewOption("Yes", true)).
			Value(&newIsDefault),
	))

	submitted, err := bridge.ShowValidatedHuhForm(form, func() error {
		if strings.TrimSpace(newName) == "" {
			return fmt.Errorf("Name is required.")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if !submitted {
		return ErrCancelled
	}

	reconfigure, err := bridge.ShowConfirm("Reconfigure models for this agent?")
	if err != nil {
		return err
	}

	dto := &models.UpdateAgentDto{
		Name:      &newName,
		Soul:      &newSoul,
		IsDefault: &newIsDefault,
	}

	if reconfigure {
		var currentIDs []uuid.UUID
		for _, m := range target.Models {
			currentIDs = append(currentIDs, m.ID)
		}
		modelIDs, err := selectAgentModels(b, bridge, currentIDs)
		if err != nil && !errors.Is(err, ErrCancelled) {
			return err
		}
		if err == nil {
			dto.ModelIDs = &modelIDs
		}
	}

	if newName == target.Name && newSoul == target.Soul && newIsDefault == target.IsDefault && dto.ModelIDs == nil {
		bridge.Info("No changes detected, skipping update")
		return nil
	}

	if err := b.UpdateAgent(target.ID, dto); err != nil {
		return err
	}
	bridge.Success("Agent updated: %s", newName)
	return nil
}

func selectAgentModels(b backend.Backend, bridge *UIBridge, currentIDs []uuid.UUID) ([]uuid.UUID, error) {
	allModels, err := b.ListModels(1, 500)
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}

	var chatModels []models.ModelDto
	for _, m := range allModels.Data {
		if !m.EmbeddingModel && m.Provider != nil && m.Provider.APIKeyCensored != "<not set>" {
			chatModels = append(chatModels, m)
		}
	}
	if len(chatModels) == 0 {
		bridge.Warning("No chat models from configured providers found")
		return nil, ErrCancelled
	}

	labels := make([]string, len(chatModels))
	idToIdx := make(map[uuid.UUID]int, len(chatModels))
	for i, m := range chatModels {
		if m.Provider != nil {
			labels[i] = m.Provider.Name + "/" + m.Name
		} else {
			labels[i] = m.Name
		}
		idToIdx[m.ID] = i
	}

	defaultIndices := make([]int, 0, len(currentIDs))
	for _, id := range currentIDs {
		if idx, ok := idToIdx[id]; ok {
			defaultIndices = append(defaultIndices, idx)
		}
	}

	selectedIndices, err := bridge.ShowMultiSelect("Select Models  "+styleGray.Render("(★ primary, numbers = fallback order)"), labels, defaultIndices)
	if err != nil {
		return nil, err
	}
	if selectedIndices == nil {
		return nil, ErrCancelled
	}

	ids := make([]uuid.UUID, len(selectedIndices))
	for i, idx := range selectedIndices {
		ids[i] = chatModels[idx].ID
	}
	return ids, nil
}
