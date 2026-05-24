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
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/cli/chatstate"
	"github.com/masteryyh/agenty/pkg/models"
)

func defaultChatState(b backend.Backend, modelID uuid.UUID) ChatState {
	return chatstate.DefaultChatState(b, modelID)
}

func chatStateForSession(b backend.Backend, modelID uuid.UUID, session *models.ChatSessionDto, restored bool) ChatState {
	return chatstate.ChatStateForSession(b, modelID, session, restored)
}

func chatStateFromThinkingLevel(b backend.Backend, modelID uuid.UUID, level string) ChatState {
	return chatstate.ChatStateFromThinkingLevel(b, modelID, level)
}

func modelDisplayName(model models.ModelDto) string {
	return chatstate.ModelDisplayName(model)
}

func modelConfigured(model models.ModelDto) bool {
	return chatstate.ModelConfigured(model)
}

func findDefaultAgent(agents []models.AgentDto) *models.AgentDto {
	return chatstate.FindDefaultAgent(agents)
}

func resolveInitialChatModel(b backend.Backend, agentID uuid.UUID, session *models.ChatSessionDto, restored bool) (uuid.UUID, string, error) {
	return chatstate.ResolveInitialChatModel(b, agentID, session, restored)
}
