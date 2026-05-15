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

package chatstate

import (
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/models"
)

type ChatState struct {
	Thinking      bool
	ThinkingLevel string
	HistoryOffset int
	LocalMode     bool
}

func FindDefaultAgent(agents []models.AgentDto) *models.AgentDto {
	for i := range agents {
		if agents[i].IsDefault {
			return &agents[i]
		}
	}
	for i := range agents {
		if agents[i].Name == "default" {
			return &agents[i]
		}
	}
	return nil
}

func ModelDisplayName(m models.ModelDto) string {
	if m.Provider != nil {
		return m.Provider.Name + "/" + m.Name
	}
	return m.Name
}

func ModelConfigured(model models.ModelDto) bool {
	return model.Provider != nil && model.Provider.APIKeyCensored != "<not set>"
}

func ModelProviderConfigured(model models.ModelDto) bool {
	return model.Provider != nil && model.Provider.APIKeyCensored != "<not set>"
}

func ModelSwitchable(model models.ModelDto) bool {
	return !model.EmbeddingModel && ModelProviderConfigured(model)
}

func ResolveInitialChatModel(b backend.Backend, agentID uuid.UUID, session *models.ChatSessionDto, restored bool) (uuid.UUID, string, error) {
	if restored && session != nil && session.LastUsedModel != uuid.Nil {
		if model, ok := configuredModelByID(b, session.LastUsedModel); ok {
			return model.ID, ModelDisplayName(model), nil
		}
	}

	if agent, err := b.GetAgent(agentID); err == nil && agent != nil && len(agent.Models) > 0 {
		if session != nil && session.LastUsedModel != uuid.Nil {
			for _, model := range agent.Models {
				if model.ID == session.LastUsedModel && ModelConfigured(model) {
					return model.ID, ModelDisplayName(model), nil
				}
			}
		}
		for _, model := range agent.Models {
			if ModelConfigured(model) {
				return model.ID, ModelDisplayName(model), nil
			}
		}
	}

	if defaultModel, err := b.GetDefaultModel(); err == nil && defaultModel != nil && ModelConfigured(*defaultModel) {
		return defaultModel.ID, ModelDisplayName(*defaultModel), nil
	}

	modelsList, err := b.ListModels(1, 100)
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("failed to list models: %w", err)
	}
	for _, model := range modelsList.Data {
		if ModelConfigured(model) {
			return model.ID, ModelDisplayName(model), nil
		}
	}
	return uuid.Nil, "", fmt.Errorf("no models available, use /model to create one")
}

func DefaultChatState(b backend.Backend, modelID uuid.UUID) ChatState {
	levelsPtr, err := b.GetModelThinkingLevels(modelID)
	if err != nil || levelsPtr == nil || len(*levelsPtr) == 0 {
		return ChatState{}
	}

	level := (*levelsPtr)[0]
	for _, candidate := range []string{"medium", "high", "adaptive", "on"} {
		if slices.Contains(*levelsPtr, candidate) {
			level = candidate
			return ChatState{Thinking: true, ThinkingLevel: level}
		}
	}
	return ChatState{Thinking: true, ThinkingLevel: level}
}

func ChatStateForSession(b backend.Backend, modelID uuid.UUID, session *models.ChatSessionDto, restored bool) ChatState {
	if !restored || session == nil || session.LastUsedThinkingLevel == nil {
		return DefaultChatState(b, modelID)
	}
	return ChatStateFromThinkingLevel(b, modelID, *session.LastUsedThinkingLevel)
}

func ChatStateFromThinkingLevel(b backend.Backend, modelID uuid.UUID, level string) ChatState {
	if level == "" {
		return ChatState{}
	}
	levelsPtr, err := b.GetModelThinkingLevels(modelID)
	if err != nil || levelsPtr == nil || len(*levelsPtr) == 0 {
		return ChatState{}
	}
	if slices.Contains(*levelsPtr, level) {
		return ChatState{Thinking: true, ThinkingLevel: level}
	}
	return DefaultChatState(b, modelID)
}

func configuredModelByID(b backend.Backend, modelID uuid.UUID) (models.ModelDto, bool) {
	modelsList, err := b.ListModels(1, 100)
	if err != nil {
		return models.ModelDto{}, false
	}
	for _, model := range modelsList.Data {
		if model.ID == modelID && ModelConfigured(model) {
			return model, true
		}
	}
	return models.ModelDto{}, false
}
